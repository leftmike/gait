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

	pathSlash := path + "/"
	for _, da := range rwa.dirs {
		// A directory covers everything within it as well as the directory
		// itself, so that the directory can be created and removed.
		if strings.HasPrefix(path, da.dir) || pathSlash == da.dir {
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

func (sb *Sandbox) checkRW(rwa *rwActions, op, path string) error {
	switch rwa.pathAction(path, sb.dflt) {
	case allow:
		return nil
	case ask:
		if sb.ask != nil && sb.ask(op, path) {
			return nil
		}
	}

	return &fs.PathError{Op: op, Path: path, Err: fs.ErrPermission}
}

func (sb *Sandbox) ReadFile(path string) ([]byte, error) {
	if err := sb.checkRW(sb.readActions, "read", path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (sb *Sandbox) WriteFile(path string, data []byte, perm fs.FileMode) error {
	if err := sb.checkRW(sb.writeActions, "write", path); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func (sb *Sandbox) MkdirAll(path string, perm fs.FileMode) error {
	if err := sb.checkRW(sb.writeActions, "mkdir", path); err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

func (sb *Sandbox) Stat(path string) (fs.FileInfo, error) {
	if err := sb.checkRW(sb.readActions, "stat", path); err != nil {
		return nil, err
	}
	return os.Stat(path)
}

func (sb *Sandbox) Remove(path string) error {
	if err := sb.checkRW(sb.writeActions, "remove", path); err != nil {
		return err
	}
	return os.Remove(path)
}

func (sb *Sandbox) WalkDir(root string, fn fs.WalkDirFunc) error {
	if err := sb.checkRW(sb.readActions, "walkdir", root); err != nil {
		return err
	}

	return filepath.WalkDir(root,
		func(path string, d fs.DirEntry, err error) error {
			// Denied paths are skipped rather than failing the whole walk, so
			// that a denied file or subdirectory is simply not visible.
			// Anything below root which needs asking is walked without asking;
			// reading it is still checked.
			if sb.readActions.pathAction(path, sb.dflt) == deny {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			return fn(path, d, err)
		})
}

// CombinedOutput runs a command in dir and returns its combined stdout and
// stderr.
func (sb *Sandbox) CombinedOutput(ctx context.Context, dir, name string, arg ...string) ([]byte,
	error) {

	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
