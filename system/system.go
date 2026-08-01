package system

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/leftmike/gait/config"
)

type AskFunc func(op, path string) bool

type action int

const (
	deny action = iota
	ask
	allow
)

type dirAction struct {
	dir string
	act action
}

type rwActions struct {
	files map[string]action
	dirs  []dirAction
}

func (rwa *rwActions) addPaths(paths []string, act action) error {
	for _, path := range paths {
		if strings.HasSuffix(path, "/") {
			dir := filepath.Clean(path)
			if dir != "/" {
				dir += "/"
			}
			rwa.dirs = append(rwa.dirs, dirAction{dir, act})
		} else {
			path = filepath.Clean(path)
			if _, ok := rwa.files[path]; ok {
				return fmt.Errorf("sandbox: path specified more than once: %s", path)
			}

			if rwa.files == nil {
				rwa.files = map[string]action{}
			}
			rwa.files[path] = act
		}
	}

	return nil
}

func newRWActions(cfg *config.RWActions) (*rwActions, error) {
	if cfg == nil {
		return nil, nil
	}

	var rwa rwActions
	err := rwa.addPaths(cfg.Allow, allow)
	if err != nil {
		return nil, err
	}
	err = rwa.addPaths(cfg.Ask, ask)
	if err != nil {
		return nil, err
	}
	err = rwa.addPaths(cfg.Deny, deny)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(rwa.dirs, func(da1, da2 dirAction) int {
		return -strings.Compare(da1.dir, da2.dir)
	})

	for i := 1; i < len(rwa.dirs); i += 1 {
		if rwa.dirs[i-1].dir == rwa.dirs[i].dir {
			return nil, fmt.Errorf("sandbox: path specified more than once: %s", rwa.dirs[i].dir)
		}
	}

	return &rwa, nil
}

func (rwa *rwActions) pathAction(path string, dflt action) action {
	if rwa == nil {
		return dflt
	}

	path = filepath.Clean(path)

	act, ok := rwa.files[path]
	if ok {
		return act
	}

	for _, da := range rwa.dirs {
		if strings.HasPrefix(path, da.dir) {
			return da.act
		}
	}

	return dflt
}

type Sandbox struct {
	readActions  *rwActions
	writeActions *rwActions
	dflt         action
	ask          AskFunc
}

func NewSandbox(sbCfg *config.SandboxConfig, ask AskFunc) (*Sandbox, error) {
	if sbCfg == nil {
		return &Sandbox{dflt: allow, ask: ask}, nil
	}

	readActions, err := newRWActions(sbCfg.Read)
	if err != nil {
		return nil, err
	}
	writeActions, err := newRWActions(sbCfg.Write)
	if err != nil {
		return nil, err
	}

	return &Sandbox{
		readActions:  readActions,
		writeActions: writeActions,
		dflt:         deny,
		ask:          ask,
	}, nil
}

// check resolves a path against one set of actions, prompting the user when
// the action is ask.
func (sb *Sandbox) check(rwa *rwActions, op, path string) error {
	switch rwa.pathAction(path, sb.dflt) {
	case allow:
		return nil
	case ask:
		// XXX
		if sb.ask != nil && sb.ask(op, path) {
			return nil
		}
	}

	return &fs.PathError{Op: op, Path: path, Err: fs.ErrPermission}
}

func (sb *Sandbox) CheckRead(op, path string) error {
	return sb.check(sb.readActions, op, path)
}

func (sb *Sandbox) CheckWrite(op, path string) error {
	return sb.check(sb.writeActions, op, path)
}

func (sb *Sandbox) ReadFile(path string) ([]byte, error) {
	if err := sb.CheckRead("read", path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (sb *Sandbox) WriteFile(path string, data []byte, perm fs.FileMode) error {
	if err := sb.CheckWrite("write", path); err != nil {
		return err
	}
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
