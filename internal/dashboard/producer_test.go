package dashboard

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/roadmap"
)

func TestParseTimeout(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty defaults to 5s", "", 5 * time.Second},
		{"valid passthrough", "10s", 10 * time.Second},
		{"floored at 1s (0s)", "0s", 1 * time.Second},
		{"floored at 1s (100ms)", "100ms", 1 * time.Second},
		{"ceiled at 30s", "999s", 30 * time.Second},
		{"garbage defaults to 5s", "not-a-duration", 5 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseTimeout(tc.in))
		})
	}
}

func TestSegmentSpec_Form(t *testing.T) {
	form, err := SegmentSpec{ID: "a", Builtin: "git"}.Form()
	require.NoError(t, err)
	assert.Equal(t, "builtin", form)

	form, err = SegmentSpec{ID: "b", Exec: []string{"printf", "x"}}.Form()
	require.NoError(t, err)
	assert.Equal(t, "exec", form)

	form, err = SegmentSpec{ID: "c", Shell: "printf x"}.Form()
	require.NoError(t, err)
	assert.Equal(t, "shell", form)

	_, err = SegmentSpec{ID: "zero"}.Form()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `segment "zero" must set exactly one of builtin/exec/shell`)

	_, err = SegmentSpec{ID: "two", Builtin: "git", Shell: "x"}.Form()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `segment "two" must set exactly one of builtin/exec/shell`)
}

func TestRunSegment_ExecSuccess(t *testing.T) {
	db := newDashboardTestDB(t)
	spec := SegmentSpec{ID: "e", Exec: []string{"printf", "%s", `{"v":1,"type":"tile","section":"x","label":"l","value":"v"}`}}

	stdout, stderr, err := RunSegment(context.Background(), db, spec, Context{Schema: 1})
	require.NoError(t, err)
	assert.Empty(t, stderr)

	res, perr := Parse(strings.NewReader(string(stdout)))
	require.NoError(t, perr)
	require.Len(t, res.Fragments, 1)
	tile, ok := res.Fragments[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "l", tile.Label)
}

func TestRunSegment_ShellSuccess(t *testing.T) {
	db := newDashboardTestDB(t)
	spec := SegmentSpec{ID: "s", Shell: `printf '{"v":1,"type":"note","text":"hi"}'`}

	stdout, _, err := RunSegment(context.Background(), db, spec, Context{Schema: 1})
	require.NoError(t, err)

	res, perr := Parse(strings.NewReader(string(stdout)))
	require.NoError(t, perr)
	require.Len(t, res.Fragments, 1)
	note, ok := res.Fragments[0].(Note)
	require.True(t, ok)
	assert.Equal(t, "hi", note.Text)
}

func TestRunSegment_StdinDelivery(t *testing.T) {
	db := newDashboardTestDB(t)
	inv := Context{Schema: 1, Project: "acme-corp", Now: "2026-07-08T00:00:00Z"}
	spec := SegmentSpec{ID: "cat", Shell: "cat"}

	stdout, _, err := RunSegment(context.Background(), db, spec, inv)
	require.NoError(t, err)

	want, merr := json.Marshal(inv)
	require.NoError(t, merr)
	assert.Equal(t, want, stdout)
}

func TestRunSegment_CmdDir(t *testing.T) {
	db := newDashboardTestDB(t)
	repo := t.TempDir()
	spec := SegmentSpec{ID: "pwd", Shell: "pwd"}

	stdout, _, err := RunSegment(context.Background(), db, spec, Context{Schema: 1, Repo: repo})
	require.NoError(t, err)

	gotDir, gerr := filepath.EvalSymlinks(strings.TrimSpace(string(stdout)))
	require.NoError(t, gerr)
	wantDir, werr := filepath.EvalSymlinks(repo)
	require.NoError(t, werr)
	assert.Equal(t, wantDir, gotDir)
}

func TestRunSegment_NonzeroExit(t *testing.T) {
	db := newDashboardTestDB(t)
	spec := SegmentSpec{ID: "f", Exec: []string{"false"}}

	stdout, _, err := RunSegment(context.Background(), db, spec, Context{Schema: 1})
	require.Error(t, err)
	assert.Nil(t, stdout)
	assert.Contains(t, err.Error(), "producer failed")
}

func TestRunSegment_Timeout(t *testing.T) {
	db := newDashboardTestDB(t)
	spec := SegmentSpec{ID: "slow", Shell: "sleep 5", Timeout: "100ms"}

	start := time.Now()
	_, _, err := RunSegment(context.Background(), db, spec, Context{Schema: 1})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.Less(t, elapsed, 2*time.Second)
}

func TestRunSegment_BuiltinAlreadyCancelledCtx(t *testing.T) {
	db := newDashboardTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	spec := SegmentSpec{ID: "roadmap", Builtin: "roadmap"}
	stdout, stderr, err := RunSegment(ctx, db, spec, Context{Schema: 1, Project: "acme-corp"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, stdout)
	assert.Empty(t, stderr)
}

func TestRunSegment_SubprocessAlreadyCancelledCtx(t *testing.T) {
	db := newDashboardTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	spec := SegmentSpec{ID: "cat", Shell: "cat"}
	_, _, err := RunSegment(ctx, db, spec, Context{Schema: 1})
	require.Error(t, err)
	// The parent is already Canceled (not DeadlineExceeded), so this hits
	// the generic "producer failed" path rather than the timeout
	// classification — either way the subprocess never runs.
	assert.Contains(t, err.Error(), "context canceled")
}

func TestRunSegment_ParentDeadlineShorterThanSpecTimeout(t *testing.T) {
	db := newDashboardTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	spec := SegmentSpec{ID: "slow", Shell: "sleep 5", Timeout: "10s"}

	start := time.Now()
	_, _, err := RunSegment(ctx, db, spec, Context{Schema: 1})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	// bounded by the parent's 100ms deadline, not the spec's 10s timeout
	assert.Less(t, elapsed, 2*time.Second)
}

func TestRunSegment_UnknownBuiltin(t *testing.T) {
	db := newDashboardTestDB(t)
	spec := SegmentSpec{ID: "nope", Builtin: "nope"}

	_, _, err := RunSegment(context.Background(), db, spec, Context{Schema: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown builtin "nope"`)
}

func TestRunSegment_FormError(t *testing.T) {
	db := newDashboardTestDB(t)

	_, _, err := RunSegment(context.Background(), db, SegmentSpec{ID: "zero"}, Context{})
	require.Error(t, err)

	_, _, err = RunSegment(context.Background(), db, SegmentSpec{ID: "two", Builtin: "git", Shell: "x"}, Context{})
	require.Error(t, err)
}

func TestRunSegment_BuiltinRoadmap(t *testing.T) {
	db := newDashboardTestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "task one"})
		return err
	}))

	spec := SegmentSpec{ID: "roadmap", Builtin: "roadmap"}
	stdout, stderr, err := RunSegment(context.Background(), db, spec, Context{Schema: 1, Project: "acme-corp"})
	require.NoError(t, err)
	assert.Empty(t, stderr)

	res, perr := Parse(strings.NewReader(string(stdout)))
	require.NoError(t, perr)
	require.Len(t, res.Fragments, 1)
	tile, ok := res.Fragments[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "now", tile.Label)
	assert.Equal(t, "1", tile.Value)
}

func TestRunSegment_BuiltinProviderError(t *testing.T) {
	db := newDashboardTestDB(t)
	require.NoError(t, db.Close()) // force roadmapSegment's store query to fail

	spec := SegmentSpec{ID: "roadmap", Builtin: "roadmap"}
	_, _, err := RunSegment(context.Background(), db, spec, Context{Schema: 1, Project: "acme-corp"})
	require.Error(t, err)
}

func TestRunSegment_LongStderrTruncatedInFailure(t *testing.T) {
	db := newDashboardTestDB(t)
	long := strings.Repeat("x", 600)
	spec := SegmentSpec{ID: "loud", Shell: `printf '` + long + `' >&2; exit 1`}

	_, stderr, err := RunSegment(context.Background(), db, spec, Context{Schema: 1})
	require.Error(t, err)
	assert.Len(t, stderr, 600)
	assert.Contains(t, err.Error(), "…")
	assert.Less(t, len(err.Error()), 700)
}

func TestCappedBuffer(t *testing.T) {
	cb := newCappedBuffer(5)

	n, err := cb.Write([]byte("abc"))
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	n, err = cb.Write([]byte("defgh"))
	require.NoError(t, err)
	assert.Equal(t, 5, n) // Write still reports full length written

	assert.Equal(t, []byte("abcde"), cb.Bytes())
	assert.Equal(t, "abcde", cb.String())

	// further writes are silently dropped once at cap
	n, err = cb.Write([]byte("more"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, []byte("abcde"), cb.Bytes())
}
