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
// text, for the caller to log/drop. A builtin runs in-process with no
// timeout (Providers are expected to be fast and are not untrusted external
// input); exec/shell run as a subprocess bounded by the spec's Timeout.
func RunSegment(db *sql.DB, spec SegmentSpec, inv Context) ([]byte, string, error) {
	form, err := spec.Form()
	if err != nil {
		return nil, "", err
	}

	switch form {
	case "builtin":
		return runBuiltinSegment(db, spec, inv)
	case "exec":
		return runSubprocessSegment(spec, inv, spec.Exec[0], spec.Exec[1:]...)
	}
	// form is "shell" — Form() guarantees exactly one of builtin/exec/shell.
	return runSubprocessSegment(spec, inv, "sh", "-c", spec.Shell)
}

func runBuiltinSegment(db *sql.DB, spec SegmentSpec, inv Context) ([]byte, string, error) {
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
func runSubprocessSegment(spec SegmentSpec, inv Context, name string, args ...string) ([]byte, string, error) {
	timeout := ParseTimeout(spec.Timeout)
	goCtx, cancel := context.WithTimeout(context.Background(), timeout)
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
