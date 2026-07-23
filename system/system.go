package system

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/leftmike/gait/config"
)

type Sandbox struct{}

func NewSandbox(sbCfg *config.SandboxConfig) *Sandbox {
	return &Sandbox{}
}

func (sb *Sandbox) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (sb *Sandbox) WriteFile(path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (sb *Sandbox) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (sb *Sandbox) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (sb *Sandbox) Remove(path string) error {
	return os.Remove(path)
}

func (sb *Sandbox) WalkDir(root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}

// CombinedOutput runs a command in dir and returns its combined stdout and
// stderr.
func (sb *Sandbox) CombinedOutput(ctx context.Context, dir, name string, arg ...string) ([]byte,
	error) {

	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
