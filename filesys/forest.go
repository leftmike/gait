package filesys

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type tree struct {
	path     string
	writable bool
	root     *os.Root // both read only and writable trees
	rofs     fs.FS    // read only trees only
}

type Tree struct {
	Path     string
	Writable bool
}

type ForestFS struct {
	trees []tree
}

func NewForestFS() *ForestFS {
	return &ForestFS{}
}

func (ffs *ForestFS) AddTree(path string, writable bool) error {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute: %s", path)
	}

	for _, tr := range ffs.trees {
		if tr.path == path || strings.HasPrefix(tr.path, path+"/") ||
			strings.HasPrefix(path, tr.path+"/") {

			return fmt.Errorf("path overlaps with existing tree %s: %s", tr.path, path)
		}
	}

	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}

	tr := tree{
		path:     path,
		writable: writable,
		root:     root,
	}
	if !writable {
		tr.rofs = root.FS()
	}

	ffs.trees = append(ffs.trees, tr)
	slices.SortFunc(ffs.trees, func(a, b tree) int {
		if len(b.path) == len(a.path) {
			return strings.Compare(a.path, b.path)
		}
		return len(b.path) - len(a.path)
	})
	return nil
}

func (ffs *ForestFS) RemoveTree(path string) {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		ffs.trees = slices.DeleteFunc(ffs.trees,
			func(tr tree) bool {
				if tr.path == path {
					tr.root.Close()
					return true
				}
				return false
			})
	}
}

func (ffs *ForestFS) ListTrees() []Tree {
	lst := make([]Tree, 0, len(ffs.trees))
	for _, tr := range ffs.trees {
		lst = append(lst, Tree{tr.path, tr.writable})
	}

	return lst
}

func (ffs *ForestFS) Close() error {
	var err error
	for _, tr := range ffs.trees {
		if err == nil {
			err = tr.root.Close()
		} else {
			tr.root.Close()
		}
	}
	ffs.trees = nil

	return err
}

func (ffs *ForestFS) resolvePath(path string) (tree, string, bool) {
	path = filepath.Clean(path)
	for _, tr := range ffs.trees {
		if path == tr.path {
			return tr, ".", true
		}
		if strings.HasPrefix(path, tr.path+"/") {
			return tr, path[len(tr.path)+1:], true
		}
	}

	return tree{}, "", false
}

func (ffs *ForestFS) ReadFile(filename string) ([]byte, error) {
	tr, resolved, ok := ffs.resolvePath(filename)
	if !ok {
		return nil, &os.PathError{Op: "readfile", Path: filename, Err: os.ErrNotExist}
	}

	if tr.rofs != nil {
		return fs.ReadFile(tr.rofs, resolved)
	}
	return tr.root.ReadFile(resolved)
}

func (ffs *ForestFS) WriteFile(filename string, buf []byte, perm os.FileMode) error {
	tr, resolved, ok := ffs.resolvePath(filename)
	if !ok {
		return &os.PathError{Op: "writefile", Path: filename, Err: os.ErrNotExist}
	}

	if !tr.writable {
		return &os.PathError{Op: "writefile", Path: filename, Err: os.ErrPermission}
	}
	return tr.root.WriteFile(resolved, buf, perm)
}

func (ffs *ForestFS) Mkdir(dir string, perm os.FileMode) error {
	tr, resolved, ok := ffs.resolvePath(dir)
	if !ok {
		return &os.PathError{Op: "mkdir", Path: dir, Err: os.ErrNotExist}
	}

	if !tr.writable {
		return &os.PathError{Op: "mkdir", Path: dir, Err: os.ErrPermission}
	}
	return tr.root.Mkdir(resolved, perm)
}

func (ffs *ForestFS) ReadDir(dir string) ([]os.DirEntry, error) {
	tr, resolved, ok := ffs.resolvePath(dir)
	if !ok {
		return nil, &os.PathError{Op: "readdir", Path: dir, Err: os.ErrNotExist}
	}

	if tr.rofs != nil {
		return fs.ReadDir(tr.rofs, resolved)
	}
	dh, err := tr.root.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer dh.Close()
	return dh.ReadDir(-1)
}
