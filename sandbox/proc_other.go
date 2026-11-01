//go:build !unix

package sandbox

import "os/exec"

func killGroupOnCancel(c *exec.Cmd) {}
