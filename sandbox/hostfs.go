package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type access int

const (
	noAccess access = iota
	readAccess
	writeAccess
)

type rule struct {
	path string
	acc  access
}

// HostFS is the host filesystem. When checked, each path is resolved through its symlinks
// and must be covered by the policy; the most specific entry applies.
type HostFS struct {
	workdir string
	home    string
	checked bool
	rules   []rule
}

// resolveSymlinks resolves the links in the existing prefix of path, keeping the
// components which do not exist yet.
func resolveSymlinks(path string) string {
	var rest string

	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(resolved, rest)
		}

		dir := filepath.Dir(path)
		if dir == path {
			return filepath.Join(path, rest)
		}

		rest = filepath.Join(filepath.Base(path), rest)
		path = dir
	}
}

func expandPath(path, workdir, home string) (string, error) {
	switch {
	case path == "":
		return "", errors.New("sandbox: empty path")
	case path == "~":
		path = home
	case strings.HasPrefix(path, "~/"):
		path = filepath.Join(home, path[2:])
	case strings.HasPrefix(path, "~"):
		return "", fmt.Errorf("sandbox: unsupported path: %s", path)
	case filepath.IsAbs(path):
		path = filepath.Clean(path)
	default:
		path = filepath.Join(workdir, path)
	}

	return path, nil
}

// ExpandPath makes path absolute, relative to workdir and with ~ as home, and resolves
// its symlinks.
func ExpandPath(path, workdir, home string) (string, error) {
	path, err := expandPath(path, workdir, home)
	if err != nil {
		return "", err
	}
	return resolveSymlinks(path), nil
}

func hostDirs(workdir string) (string, string, error) {
	if workdir == "" {
		var err error
		workdir, err = os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("sandbox: %w", err)
		}
	} else if !filepath.IsAbs(workdir) {
		return "", "", fmt.Errorf("sandbox: workdir must be absolute: %s", workdir)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("sandbox: %w", err)
	}

	return resolveSymlinks(filepath.Clean(workdir)), resolveSymlinks(filepath.Clean(home)), nil
}

func UncheckedFS(workdir string) (*HostFS, error) {
	workdir, home, err := hostDirs(workdir)
	if err != nil {
		return nil, err
	}

	return &HostFS{workdir: workdir, home: home}, nil
}

func NewHostFS(p Policy) (*HostFS, error) {
	workdir, home, err := hostDirs(p.Workdir)
	if err != nil {
		return nil, err
	}

	hfs := &HostFS{workdir: workdir, home: home, checked: true}
	seen := map[string]bool{}
	add := func(paths []string, acc access) error {
		for _, path := range paths {
			path, err := ExpandPath(path, workdir, home)
			if err != nil {
				return err
			}
			if seen[path] {
				return fmt.Errorf("sandbox: path specified more than once: %s", path)
			}
			seen[path] = true
			hfs.rules = append(hfs.rules, rule{path, acc})
		}
		return nil
	}

	if err := add(p.ReadOnly, readAccess); err != nil {
		return nil, err
	}
	if err := add(p.ReadWrite, writeAccess); err != nil {
		return nil, err
	}
	if err := add(p.Deny, noAccess); err != nil {
		return nil, err
	}
	if !seen[workdir] {
		hfs.rules = append(hfs.rules, rule{workdir, writeAccess})
	}

	slices.SortFunc(hfs.rules, func(r1, r2 rule) int {
		return len(r2.path) - len(r1.path)
	})
	return hfs, nil
}

func (hfs *HostFS) Workdir() string {
	return hfs.workdir
}

func within(path, dir string) bool {
	return path == dir || dir == "/" || strings.HasPrefix(path, dir+"/")
}

func (hfs *HostFS) access(path string) access {
	if !hfs.checked {
		return writeAccess
	}

	for _, r := range hfs.rules {
		if within(path, r.path) {
			return r.acc
		}
	}
	return noAccess
}

func (hfs *HostFS) checkPath(op, path string, need access) error {
	if hfs.access(path) < need {
		return &fs.PathError{Op: op, Path: path, Err: fs.ErrPermission}
	}
	return nil
}

func (hfs *HostFS) check(op, path string, need access) (string, error) {
	path, err := ExpandPath(path, hfs.workdir, hfs.home)
	if err != nil {
		return "", err
	}
	return path, hfs.checkPath(op, path, need)
}

func (hfs *HostFS) ReadFile(_ context.Context, path string) ([]byte, error) {
	path, err := hfs.check("read", path, readAccess)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (hfs *HostFS) WriteFile(_ context.Context, path string, data []byte,
	perm fs.FileMode) error {

	path, err := hfs.check("write", path, writeAccess)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func (hfs *HostFS) MkdirAll(_ context.Context, path string, perm fs.FileMode) error {
	path, err := hfs.check("mkdir", path, writeAccess)
	if err != nil {
		return err
	}

	var missing []string
	for dir := filepath.Dir(path); ; {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		missing = append(missing, dir)

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	for _, dir := range missing {
		if err := hfs.checkPath("mkdir", dir, writeAccess); err != nil {
			return err
		}
	}

	return os.MkdirAll(path, perm)
}

// Remove checks the path itself rather than what it links to, since that is what is
// removed.
func (hfs *HostFS) Remove(_ context.Context, path string) error {
	path, err := expandPath(path, hfs.workdir, hfs.home)
	if err != nil {
		return err
	}
	path = filepath.Join(resolveSymlinks(filepath.Dir(path)), filepath.Base(path))

	if err := hfs.checkPath("remove", path, writeAccess); err != nil {
		return err
	}
	return os.Remove(path)
}

func (hfs *HostFS) Stat(_ context.Context, path string) (fs.FileInfo, error) {
	path, err := hfs.check("stat", path, readAccess)
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}

// WalkDir skips what cannot be read, rather than failing the walk.
func (hfs *HostFS) WalkDir(ctx context.Context, root string, fn fs.WalkDirFunc) error {
	root, err := hfs.check("walkdir", root, readAccess)
	if err != nil {
		return err
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if hfs.access(path) < readAccess {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		return fn(path, d, err)
	})
}
