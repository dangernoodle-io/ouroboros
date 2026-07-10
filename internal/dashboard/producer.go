package dashboard

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// defaultTimeout is used when a segment's Timeout is empty or invalid.
const defaultTimeout = 5 * time.Second

// minTimeout/maxTimeout floor/ceil a segment's configured Timeout.
const (
	minTimeout = 1 * time.Second
	maxTimeout = 30 * time.Second
)

// maxStdoutBytes/maxStderrBytes bound how much of an exec/shell producer's
// output RunSegment retains, so a runaway or pathological producer can't
// exhaust memory.
const (
	maxStdoutBytes = 4 << 20  // 4MiB
	maxStderrBytes = 64 << 10 // 64KiB
)

// ParseTimeout parses a segment's Timeout duration string, defaulting to
// defaultTimeout when empty or invalid, and clamping the result to
// [minTimeout, maxTimeout].
func ParseTimeout(s string) time.Duration {
	d := defaultTimeout
	if s != "" {
		if parsed, err := time.ParseDuration(s); err == nil {
			d = parsed
		}
	}
	if d < minTimeout {
		d = minTimeout
	}
	if d > maxTimeout {
		d = maxTimeout
	}
	return d
}

// Form reports which producer form a SegmentSpec declares: exactly one of
// "builtin", "exec", or "shell". It errors when zero or more than one of
// Builtin/Exec/Shell is set.
func (s SegmentSpec) Form() (string, error) {
	count := 0
	form := ""
	if s.Builtin != "" {
		count++
		form = "builtin"
	}
	if len(s.Exec) > 0 {
		count++
		form = "exec"
	}
	if s.Shell != "" {
		count++
		form = "shell"
	}
	if count != 1 {
		return "", fmt.Errorf("segment %q must set exactly one of builtin/exec/shell", s.ID)
	}
	return form, nil
}

// cappedBuffer is a bytes.Buffer that stops appending once it reaches cap
// bytes, while still reporting a successful Write of the full length (so
// io.Copy/cmd.Stdout callers never see a short-write error) — the producer's
// stdout/stderr keeps draining, but RunSegment only retains the first cap
// bytes. Safe for concurrent Write calls (cmd.Run may write from more than
// one internal goroutine when both Stdout and Stderr are set).
type cappedBuffer struct {
	mu  sync.Mutex
	cap int
	buf bytes.Buffer
}

func newCappedBuffer(capBytes int) *cappedBuffer {
	return &cappedBuffer{cap: capBytes}
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if remaining := c.cap - c.buf.Len(); remaining > 0 {
		if len(p) <= remaining {
			c.buf.Write(p)
		} else {
			c.buf.Write(p[:remaining])
		}
	}
	return len(p), nil
}

func (c *cappedBuffer) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]byte, c.buf.Len())
	copy(out, c.buf.Bytes())
	return out
}

func (c *cappedBuffer) String() string {
	return string(c.Bytes())
}

// RunSegment runs one segment spec's producer to completion and returns its
// raw stdout (expected to be the NDJSON fragment contract) plus any stderr
// text, for the caller to log/drop. ctx is the caller's aggregate deadline
// (see dashboard.RefreshTimeout): once it fires, the caller's sequential
// segment loop starts nothing further, and an exec/shell producer already
// in flight is cut, since it shares this ctx as its own subprocess timeout
// bound (along with the spec's own Timeout, whichever is shorter). A
// builtin only checks ctx before running (it is otherwise in-process,
// Providers being expected to be fast and not untrusted external input) —
// if a builtin internally spawns its own subprocess with its own timeout
// (today only the "github" builtin's `gh pr list` call, capped at ~10s
// internally), that inner call is NOT preempted by ctx, so total wall
// clock can still exceed the aggregate deadline by up to that builtin's
// internal cap. Tracked for tightening in the backlog.
func RunSegment(ctx context.Context, db *sql.DB, spec SegmentSpec, inv Context) ([]byte, string, error) {
	form, err := spec.Form()
	if err != nil {
		return nil, "", err
	}

	switch form {
	case "builtin":
		return runBuiltinSegment(ctx, db, spec, inv)
	case "exec":
		return runSubprocessSegment(ctx, spec, inv, spec.Exec[0], spec.Exec[1:]...)
	}
	// form is "shell" — Form() guarantees exactly one of builtin/exec/shell.
	return runSubprocessSegment(ctx, spec, inv, "sh", "-c", spec.Shell)
}

// runBuiltinSegment skips the builtin outright if ctx's aggregate deadline
// has already fired; otherwise it runs the provider with no timeout of its
// own. The Provider signature takes no context.Context, so a builtin that
// spawns its own subprocess (today only "github", via `gh pr list`) is
// bounded solely by that subprocess's own internal timeout, not ctx — it is
// not preempted mid-run when the aggregate deadline fires.
func runBuiltinSegment(ctx context.Context, db *sql.DB, spec SegmentSpec, inv Context) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	prov, ok := Builtin(spec.Builtin)
	if !ok {
		return nil, "", fmt.Errorf("unknown builtin %q", spec.Builtin)
	}
	frags, err := prov(inv, db)
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	if err := Emit(&buf, frags); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "", nil
}

// runSubprocessSegment runs an exec/shell producer: name/args is the argv to
// execute (execve for exec, "sh -c <script>" for shell — never a shell for
// the exec form). The producer runs in the project's repo dir (when set),
// inherits the parent environment (PATH/tool discovery), and receives the
// marshaled Context on stdin.
//
// producer config (a Makefile/git-hook trust boundary), not remote/user
// input, matching the trust-model note in CLAUDE.md.
func runSubprocessSegment(ctx context.Context, spec SegmentSpec, inv Context, name string, args ...string) ([]byte, string, error) {
	timeout := ParseTimeout(spec.Timeout)
	goCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(goCtx, name, args...)
	configureProcessGroup(cmd)
	// Backstop: if a producer's child still holds a pipe open after the group
	// kill, don't let cmd.Run block indefinitely — force-close I/O after a grace.
	cmd.WaitDelay = 2 * time.Second
	if inv.Repo != "" {
		cmd.Dir = inv.Repo
	}

	payload, _ := json.Marshal(inv)
	cmd.Stdin = bytes.NewReader(payload)

	stdout := newCappedBuffer(maxStdoutBytes)
	stderr := newCappedBuffer(maxStderrBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	stderrText := stderr.String()

	if goCtx.Err() == context.DeadlineExceeded {
		return nil, stderrText, fmt.Errorf("timeout after %s", timeout)
	}
	if runErr != nil {
		return nil, stderrText, fmt.Errorf("producer failed: %w (stderr: %s)", runErr, truncateStderr(stderrText))
	}
	return stdout.Bytes(), stderrText, nil
}

// truncateStderr bounds how much stderr text is folded into a producer
// failure's error message, so a chatty failing producer doesn't bloat logs.
func truncateStderr(s string) string {
	s = strings.TrimSpace(s)
	const limit = 500
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}
