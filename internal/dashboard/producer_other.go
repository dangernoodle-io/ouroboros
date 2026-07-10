//go:build !unix

package dashboard

import "os/exec"

// configureProcessGroup is a no-op on non-Unix platforms; context cancellation
// falls back to os/exec's default single-process kill plus the WaitDelay
// backstop set by the caller.
func configureProcessGroup(cmd *exec.Cmd) {}
