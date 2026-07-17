package tool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/sandbox"
	"golang.org/x/sys/unix"
)

// pathsAllow reports whether path is one of paths or is contained in one of
// them, mirroring how sandbox.FSPolicy matches paths.
func pathsAllow(paths []string, path string) bool {
	path += "/"
	for _, allow := range paths {
		if !strings.HasSuffix(allow, "/") {
			allow += "/"
		}
		if strings.HasPrefix(path, allow) {
			return true
		}
	}
	return false
}

// checkRead returns an error if the sandbox's filesystem policy denies reading
// path. Paths granted write or execute access may be read as well.
func checkRead(sb *sandbox.Sandbox, path string) error {
	if sb == nil || sb.FSP == nil {
		return nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if pathsAllow(sb.FSP.Read, abs) || pathsAllow(sb.FSP.Write, abs) ||
		pathsAllow(sb.FSP.Execute, abs) {

		return nil
	}
	return fmt.Errorf("sandbox: read access denied: %s", path)
}

// checkWrite returns an error if the sandbox's filesystem policy denies
// writing path; writing includes creating, truncating, and removing it.
func checkWrite(sb *sandbox.Sandbox, path string) error {
	if sb == nil || sb.FSP == nil {
		return nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if pathsAllow(sb.FSP.Write, abs) {
		return nil
	}
	return fmt.Errorf("sandbox: write access denied: %s", path)
}

// combinedOutput runs a command in dir and returns its combined stdout and
// stderr, executing it inside the sandbox when one is provided.
func combinedOutput(ctx context.Context, sb *sandbox.Sandbox, dir, name string,
	arg ...string) ([]byte, error) {

	if sb == nil {
		cmd := exec.CommandContext(ctx, name, arg...)
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}

	cmd := sandbox.CommandContext(ctx, name, arg...)
	cmd.Sandbox = sb
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

type sandboxHandler struct {
	log    bool
	ask    bool
	always bool
}

func (sh sandboxHandler) clone(pid uint32, sysnum int, flags uint64) bool {
	// XXX
	return true
}

func (sh sandboxHandler) exec(pid uint32, sysnum int, pathname string, argv []string,
	env []string) bool {

	// XXX
	return true
}

func (sh sandboxHandler) open(pid uint32, sysnum int, pathname string, flags int32, mode uint32,
	resolve uint64) bool {

	// XXX
	return true
}

func (sh sandboxHandler) openFailed(pid uint32, sysnum int, pathname string, err error) {
	// XXX
}

func (sh sandboxHandler) syscall(pid uint32, sysnum int) bool {
	// XXX
	return true
}

func (sh sandboxHandler) failed(pid uint32, sysnum int, err error) {
	// XXX
}

func NewSandbox(sbCfg *config.SandboxConfig) *sandbox.Sandbox {
	if sbCfg == nil {
		return nil
	}

	if sbCfg.Syscalls == "" {
		sbCfg.Syscalls = "yes"
	}
	if sbCfg.FileAccess == "" {
		sbCfg.FileAccess = "yes"
	}
	if sbCfg.Execute == "" {
		sbCfg.Execute = "yes"
	}

	sb := sandbox.Sandbox{
		NoLandlock: sbCfg.NoLandlock,
	}

	if sbCfg.NoLandlock {
		sb.Mode = sandbox.SeccompMode
	} else {
		sb.Mode = sandbox.LandlockMode
	}

	if sbCfg.Syscalls == "all" {
		sb.Filter = sandbox.UserNotifFilterConfig()
	} else {
		sb.Filter = sandbox.DefaultFilterConfig()
	}

	if sbCfg.Syscalls == "ask" || sbCfg.Syscalls == "always" || sbCfg.Log {
		sb.Filter["default"] = sandbox.FilterConfig{Action: unix.SECCOMP_RET_USER_NOTIF}
		sh := sandboxHandler{
			log:    sbCfg.Log,
			ask:    sbCfg.Syscalls == "ask",
			always: sbCfg.Syscalls == "always",
		}
		sb.Syscall = sh.syscall
		sb.Failed = sh.failed
	} else {
		sb.Filter["default"] = sandbox.FilterConfig{Action: unix.SECCOMP_RET_ALLOW}
	}

	if sbCfg.FileAccess != "yes" || sbCfg.Log {
		sh := sandboxHandler{
			log:    sbCfg.Log,
			ask:    sbCfg.FileAccess == "ask",
			always: sbCfg.FileAccess == "always",
		}
		sb.Open = sh.open
		sb.OpenFailed = sh.openFailed
	}

	if sbCfg.Execute != "yes" || sbCfg.Log {
		sh := sandboxHandler{
			log:    sbCfg.Log,
			ask:    sbCfg.Execute == "ask",
			always: sbCfg.Execute == "always",
		}
		sb.Exec = sh.exec
		sb.Clone = sh.clone
	}

	// XXX: policy from config
	sb.FSP = sandbox.DefaultFSPolicy()

	wd, err := os.Getwd()
	if err == nil {
		sb.FSP.Write = append(sb.FSP.Write, wd)
	}

	return &sb
}
