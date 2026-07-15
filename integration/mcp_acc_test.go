// Package integration drives the ouroboros MCP server as a real subprocess
// over its stdio wire protocol (initialize -> tools/list -> tools/call),
// exercising the full 4-tool surface (query, kb, backlog, roadmap)
// end-to-end against a hermetic per-test SQLite DB. Opt-in via
// ACC_OUROBOROS=1 (see internal/testutil.SkipUnlessAcc) so the default
// `go test ./...` stays fast and never spawns a subprocess.
package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/testutil"
)

// resolveBinary honors OUROBOROS_ACC_BIN if set, else `go build`s the
// server into a temp path from the repo root. integration/ is one level
// below the repo root, so a single filepath.Dir on this test file's
// directory resolves it.
func resolveBinary(t *testing.T) string {
	t.Helper()

	if bin := os.Getenv("OUROBOROS_ACC_BIN"); bin != "" {
		return bin
	}

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve repo root: runtime.Caller failed")
	repoRoot := filepath.Dir(filepath.Dir(thisFile))

	bin := filepath.Join(t.TempDir(), "ouroboros")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	cmd.Dir = repoRoot
	// CGO_ENABLED=0 for parity with `make build`'s canonical release
	// artifact — a CGO-enabled build here could mask a CGO-only regression.
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build ouroboros server:\n%s", out)
	return bin
}

// harness bundles the live MCP client plus the hermetic environment the
// spawned server (and any CLI-side bootstrap commands) run under.
type harness struct {
	t      *testing.T
	client *client.Client
	bin    string
	env    []string
	dbPath string
}

// newHarness builds/resolves the ouroboros binary, spawns it via the
// explicit "server" subcommand as an MCP stdio server (bare invocation is
// help-only, see internal/cli/root.go) against a hermetic per-test SQLite
// DB, and completes the initialize handshake.
func newHarness(t *testing.T) *harness {
	t.Helper()

	bin := resolveBinary(t)

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "kb.db")
	home := filepath.Join(tmp, "home")
	require.NoError(t, os.MkdirAll(home, 0o755), "create isolated HOME")

	// Hermetic env: REPLACES the child's environment entirely. PROJECT_KB_PATH
	// pins the SQLite DB to this test's tempdir (internal/config.Load's
	// primary env var); HOME isolates from ~/.config/ouroboros/bootstrap.json
	// so a developer's real bootstrap config can never leak in; PATH is
	// copied through in case the server's own runtime needs it (it shouldn't,
	// but a missing PATH is a classic subprocess footgun).
	env := []string{
		"PROJECT_KB_PATH=" + dbPath,
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
	}

	c, err := client.NewStdioMCPClientWithOptions(bin, env, []string{"server"})
	require.NoError(t, err, "spawn ouroboros server")

	// Drain stderr on a goroutine so a full pipe can never deadlock the
	// subprocess; buffer lines and flush via t.Log from Cleanup (t.Log is
	// unsafe to call once the test has finished).
	var stderrWG sync.WaitGroup
	var stderrMu sync.Mutex
	var stderrLines []string
	if stderr, ok := client.GetStderr(c); ok {
		stderrWG.Add(1)
		go func() {
			defer stderrWG.Done()
			scanner := bufio.NewScanner(stderr)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				line := scanner.Text()
				stderrMu.Lock()
				stderrLines = append(stderrLines, line)
				stderrMu.Unlock()
			}
			_, _ = io.Copy(io.Discard, stderr)
		}()
	}
	t.Cleanup(func() {
		_ = c.Close()
		stderrWG.Wait()
		stderrMu.Lock()
		defer stderrMu.Unlock()
		for _, line := range stderrLines {
			t.Log("ouroboros server stderr: " + line)
		}
	})

	// Ouroboros emits no notifications/progress — OnNotification is a no-op
	// registration purely so the transport's dispatcher has a handler wired
	// (see the load-bearing Start-before-Initialize note below).
	c.OnNotification(func(mcp.JSONRPCNotification) {})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// client.NewStdioMCPClientWithOptions only starts the underlying
	// transport; it does NOT wire the notification dispatcher onto handlers
	// registered via OnNotification -- that only happens inside
	// (*client.Client).Start. Start MUST run before Initialize.
	require.NoError(t, c.Start(ctx), "start mcp client transport")

	_, err = c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "ouroboros-mcp-acc", Version: "0.1.0"},
		},
	})
	require.NoError(t, err, "initialize")

	return &harness{t: t, client: c, bin: bin, env: env, dbPath: dbPath}
}

// createProject bootstraps a backlog project row via the CLI, against the
// same hermetic DB the MCP harness points at. The MCP surface has no
// project-create tool (CLI-only, see CLAUDE.md); backlog/roadmap item
// writes require an existing projects row (backlog.GetProjectByName).
func (h *harness) createProject(name string) {
	h.t.Helper()
	cmd := exec.Command(h.bin, "project", "create", name)
	cmd.Env = h.env
	out, err := cmd.CombinedOutput()
	require.NoError(h.t, err, "project create %s:\n%s", name, out)
}

func (h *harness) callTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	h.t.Helper()
	return h.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	})
}

func resultText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := mcp.AsTextContent(c); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

// TestMCPWireAcceptance drives the real MCP stdio wire path
// (initialize -> tools/list -> tools/call) against every registered tool.
// Subtests share one harness/DB and run in declaration order (backlog and
// roadmap items created in earlier subtests are read back in later ones).
func TestMCPWireAcceptance(t *testing.T) {
	testutil.SkipUnlessAcc(t)

	h := newHarness(t)

	// Isolated test project names/data per rules/testing.md (no real PII;
	// example.com-style placeholders).
	const backlogProject = "acc-harness-backlog"
	h.createProject(backlogProject)

	const kbProject = "acc-harness-kb"
	const distinctiveTerm = "quokkawombatxyzzy"

	var seededDocID int64
	var backlogItemID string
	var roadmapItemID float64

	t.Run("tools_list_surface", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		res, err := h.client.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err, "tools/list")

		names := make(map[string]bool, len(res.Tools))
		for _, tool := range res.Tools {
			names[tool.Name] = true
		}

		// Exact-set assertion: fail on any add/remove/rename (schema-drift
		// guard) — the MCP wire surface is intentionally frozen at 4 tools.
		assert.Equal(t, map[string]bool{
			"query":   true,
			"kb":      true,
			"backlog": true,
			"roadmap": true,
		}, names)
	})

	t.Run("kb_write_then_get", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		content := "Acceptance harness seed document, token " + distinctiveTerm + "."

		res, err := h.callTool(ctx, "kb", map[string]any{
			"entries": []map[string]any{
				{
					"type":     "note",
					"project":  kbProject,
					"category": "acceptance",
					"title":    "Acceptance Harness Seed Doc",
					"content":  content,
				},
			},
		})
		require.NoError(t, err, "kb create")
		require.False(t, res.IsError, "kb create returned error: %s", resultText(res))

		var created []struct {
			ID     int64  `json:"id"`
			Action string `json:"action"`
			Title  string `json:"title"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(res)), &created), "parse kb create result: %s", resultText(res))
		require.Len(t, created, 1)
		assert.Equal(t, "created", created[0].Action)
		assert.Equal(t, "Acceptance Harness Seed Doc", created[0].Title)
		require.NotZero(t, created[0].ID)
		seededDocID = created[0].ID

		// Round-trip via get domain=kb ids=[id].
		getRes, err := h.callTool(ctx, "query", map[string]any{
			"domain": "kb",
			"ids":    []any{seededDocID},
		})
		require.NoError(t, err, "get domain=kb")
		require.False(t, getRes.IsError, "get domain=kb returned error: %s", resultText(getRes))

		var docs []struct {
			ID      int64  `json:"id"`
			Type    string `json:"type"`
			Project string `json:"project"`
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(getRes)), &docs), "parse get domain=kb result: %s", resultText(getRes))
		require.Len(t, docs, 1)
		assert.Equal(t, seededDocID, docs[0].ID)
		assert.Equal(t, "note", docs[0].Type)
		assert.Equal(t, kbProject, docs[0].Project)
		assert.Equal(t, "Acceptance Harness Seed Doc", docs[0].Title)
		assert.Equal(t, content, docs[0].Content)
	})

	t.Run("search_finds_seeded_doc", func(t *testing.T) {
		require.NotZero(t, seededDocID, "kb_write_then_get must run first")

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		res, err := h.callTool(ctx, "query", map[string]any{
			"domain": "kb",
			"query":  distinctiveTerm,
		})
		require.NoError(t, err, "search domain=kb")
		require.False(t, res.IsError, "search domain=kb returned error: %s", resultText(res))

		var summaries []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(res)), &summaries), "parse search result: %s", resultText(res))

		found := false
		for _, s := range summaries {
			if s.ID == seededDocID {
				found = true
				break
			}
		}
		assert.True(t, found, "search for distinctive term did not surface seeded doc %d: %s", seededDocID, resultText(res))
	})

	t.Run("backlog_crud", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Create.
		res, err := h.callTool(ctx, "backlog", map[string]any{
			"entries": []map[string]any{
				{
					"project":     backlogProject,
					"priority":    "P2",
					"title":       "Acceptance test item",
					"description": "seeded by the MCP acceptance harness",
				},
			},
		})
		require.NoError(t, err, "backlog create")
		require.False(t, res.IsError, "backlog create returned error: %s", resultText(res))

		var created []struct {
			ID     string `json:"id"`
			Action string `json:"action"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(res)), &created), "parse backlog create result: %s", resultText(res))
		require.Len(t, created, 1)
		assert.Equal(t, "create", created[0].Action)
		require.NotEmpty(t, created[0].ID)
		backlogItemID = created[0].ID

		// List/get by id (verbose to inspect full fields).
		getRes, err := h.callTool(ctx, "query", map[string]any{
			"domain":  "backlog",
			"ids":     []any{backlogItemID},
			"verbose": true,
		})
		require.NoError(t, err, "get domain=backlog")
		require.False(t, getRes.IsError, "get domain=backlog returned error: %s", resultText(getRes))

		var items []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Priority string `json:"priority"`
			Status   string `json:"status"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(getRes)), &items), "parse get domain=backlog result: %s", resultText(getRes))
		require.Len(t, items, 1)
		assert.Equal(t, backlogItemID, items[0].ID)
		assert.Equal(t, "Acceptance test item", items[0].Title)
		assert.Equal(t, "P2", items[0].Priority)
		assert.Equal(t, "open", items[0].Status)

		// Update.
		updRes, err := h.callTool(ctx, "backlog", map[string]any{
			"entries": []map[string]any{
				{"id": backlogItemID, "title": "Acceptance test item (updated)"},
			},
		})
		require.NoError(t, err, "backlog update")
		require.False(t, updRes.IsError, "backlog update returned error: %s", resultText(updRes))

		var updated []struct {
			ID     string `json:"id"`
			Action string `json:"action"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(updRes)), &updated), "parse backlog update result: %s", resultText(updRes))
		require.Len(t, updated, 1)
		assert.Equal(t, "update", updated[0].Action)
		assert.Equal(t, backlogItemID, updated[0].ID)

		// Mark done.
		doneRes, err := h.callTool(ctx, "backlog", map[string]any{
			"entries": []map[string]any{
				{"id": backlogItemID, "status": "done"},
			},
		})
		require.NoError(t, err, "backlog mark done")
		require.False(t, doneRes.IsError, "backlog mark done returned error: %s", resultText(doneRes))

		// Confirm the round trip: title update and status=done both stuck.
		finalRes, err := h.callTool(ctx, "query", map[string]any{
			"domain": "backlog",
			"ids":    []any{backlogItemID},
		})
		require.NoError(t, err, "get domain=backlog (final)")
		require.False(t, finalRes.IsError, "get domain=backlog (final) returned error: %s", resultText(finalRes))

		var final []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(finalRes)), &final), "parse final get result: %s", resultText(finalRes))
		require.Len(t, final, 1)
		assert.Equal(t, "Acceptance test item (updated)", final[0].Title)
		assert.Equal(t, "done", final[0].Status)
	})

	t.Run("roadmap_round_trip", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		const roadmapProject = "acc-harness-roadmap"

		// Add.
		addRes, err := h.callTool(ctx, "roadmap", map[string]any{
			"op":      "add",
			"project": roadmapProject,
			"section": "now",
			"title":   "Acceptance roadmap item",
			"body":    "seeded by the MCP acceptance harness",
		})
		require.NoError(t, err, "roadmap add")
		require.False(t, addRes.IsError, "roadmap add returned error: %s", resultText(addRes))

		var added struct {
			ID      float64 `json:"id"`
			Section string  `json:"section"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(addRes)), &added), "parse roadmap add result: %s", resultText(addRes))
		assert.Equal(t, "now", added.Section)
		require.NotZero(t, added.ID)
		roadmapItemID = added.ID

		// Update.
		updRes, err := h.callTool(ctx, "roadmap", map[string]any{
			"op":      "update",
			"project": roadmapProject,
			"id":      roadmapItemID,
			"title":   "Acceptance roadmap item (updated)",
		})
		require.NoError(t, err, "roadmap update")
		require.False(t, updRes.IsError, "roadmap update returned error: %s", resultText(updRes))

		// Done.
		doneRes, err := h.callTool(ctx, "roadmap", map[string]any{
			"op":      "done",
			"project": roadmapProject,
			"id":      roadmapItemID,
		})
		require.NoError(t, err, "roadmap done")
		require.False(t, doneRes.IsError, "roadmap done returned error: %s", resultText(doneRes))

		var done struct {
			ID      float64 `json:"id"`
			Section string  `json:"section"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(doneRes)), &done), "parse roadmap done result: %s", resultText(doneRes))
		assert.Equal(t, "done", done.Section)

		// Verify the title update stuck and the item landed in done, via a
		// structured get before removing it.
		getRes, err := h.callTool(ctx, "query", map[string]any{
			"domain":   "roadmap",
			"projects": []any{roadmapProject},
		})
		require.NoError(t, err, "get domain=roadmap")
		require.False(t, getRes.IsError, "get domain=roadmap returned error: %s", resultText(getRes))

		var rm struct {
			Sections struct {
				Now  []struct{ ID float64 } `json:"now"`
				Done []struct {
					ID    float64 `json:"id"`
					Title string  `json:"title"`
				} `json:"done"`
			} `json:"sections"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(getRes)), &rm), "parse get domain=roadmap result: %s", resultText(getRes))
		assert.Empty(t, rm.Sections.Now, "item should have moved out of now")
		require.Len(t, rm.Sections.Done, 1)
		assert.Equal(t, roadmapItemID, rm.Sections.Done[0].ID)
		assert.Equal(t, "Acceptance roadmap item (updated)", rm.Sections.Done[0].Title)

		// Remove.
		rmRes, err := h.callTool(ctx, "roadmap", map[string]any{
			"op":      "remove",
			"project": roadmapProject,
			"id":      roadmapItemID,
		})
		require.NoError(t, err, "roadmap remove")
		require.False(t, rmRes.IsError, "roadmap remove returned error: %s", resultText(rmRes))

		var removed struct {
			ID      float64 `json:"id"`
			Removed bool    `json:"removed"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(rmRes)), &removed), "parse roadmap remove result: %s", resultText(rmRes))
		assert.True(t, removed.Removed)
	})

	// Hermetic isolation guard: the harness must only ever touch its own
	// tempdir DB, never the real ~/.local/share/ouroboros/kb.db.
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	realDB := filepath.Join(home, ".local", "share", "ouroboros", "kb.db")
	assert.NotEqual(t, realDB, h.dbPath, "harness DB path must not be the real default DB path")
	tempRoot, err := filepath.EvalSymlinks(os.TempDir())
	require.NoError(t, err)
	dbDir, err := filepath.EvalSymlinks(filepath.Dir(h.dbPath))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(dbDir, tempRoot),
		"harness DB path %q does not resolve under the system tempdir %q", h.dbPath, tempRoot)
}
