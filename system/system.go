package system

import (
	"context"
	"os/exec"

	"github.com/leftmike/gait/config"
)

/*
type System struct {
	ReadRules    []PathRule
	ExecuteRules []PathRule
	WriteRules   []PathRule
}

type Action int

const (
	Deny Action = iota
	Allow
	Ask
)

type PathRule struct {
	Path   string
	Action Action
}
*/

type Sandbox struct{}

func NewSandbox(sbCfg *config.SandboxConfig) *Sandbox {
	return &Sandbox{}
}

func (sb *Sandbox) CheckRead(path string) error {
	// XXX
	return nil
}

func (sb *Sandbox) CheckWrite(path string) error {
	// XXX
	return nil
}

func (sb *Sandbox) CombinedOutput(ctx context.Context, dir, name string, arg ...string) ([]byte,
	error) {

	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

/*
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
*/
