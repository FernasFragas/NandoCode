//go:build unix

package eval

import (
	"os/exec"
	"syscall"
)

// configureProcessTree runs the command in its own process group and kills the
// whole group on cancellation, so grandchildren (such as compiled test
// binaries) do not outlive a timed-out command.
func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
