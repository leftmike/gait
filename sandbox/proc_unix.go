//go:build unix

package sandbox

import (
	"os/exec"
	"syscall"
)

func killGroupOnCancel(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
