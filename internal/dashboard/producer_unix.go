//go:build unix

package dashboard

import (
	"os"
	"os/exec"
	"syscall"
)

// configureProcessGroup puts the subprocess in its own process group and makes
// context cancellation SIGKILL the entire group (-pgid), so a shell producer's
// grandchildren (e.g. the `sleep` in `sh -c "sleep 5"`) are terminated too,
// rather than surviving to hold the stdout pipe open until they exit on their
// own — which would block cmd.Run past the timeout.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
