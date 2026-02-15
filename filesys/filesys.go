package filesys

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/afero"
)

type FilterPath struct {
	Path     string
	Writable bool
}

type filterFs struct {
	fs        afero.Fs
	dirs      []FilterPath
	filenames map[string]bool
}

var _ afero.Fs = (*filterFs)(nil)

func NewFs(fs afero.Fs) *filterFs {
	return &filterFs{
		fs:        fs,
		filenames: map[string]bool{},
	}
}

func sortFilterPaths(paths []FilterPath) {
	slices.SortFunc(paths,
		func(fp1, fp2 FilterPath) int {
			return len(fp2.Path) - len(fp1.Path)
		})
}

func (ffs *filterFs) AddDir(dir string, writable bool) error {
	path := filepath.Clean(dir)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("directory must be absolute: %s", dir)
	}

	idx := slices.IndexFunc(ffs.dirs,
		func(fp FilterPath) bool {
			if fp.Path == path {
				return true
			}
			return false
		})
	if idx >= 0 {
		ffs.dirs[idx].Writable = writable
		return nil
	}

	ffs.dirs = append(ffs.dirs, FilterPath{Path: path, Writable: writable})
	sortFilterPaths(ffs.dirs)
	return nil
}

func (ffs *filterFs) RemoveDir(dir string) {
	path := filepath.Clean(dir)
	if filepath.IsAbs(path) {
		ffs.dirs = slices.DeleteFunc(ffs.dirs,
			func(fp FilterPath) bool {
				if fp.Path == path {
					return true
				}
				return false
			})
	}
}

func (ffs *filterFs) ListDirs() []FilterPath {
	return ffs.dirs
}

func (ffs *filterFs) AddFilename(filename string, writable bool) error {
	path := filepath.Clean(filename)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("filename must be absolute: %s", filename)
	}

	ffs.filenames[path] = writable
	return nil
}

func (ffs *filterFs) RemoveFilename(filename string) {
	delete(ffs.filenames, filepath.Clean(filename))
}

func (ffs *filterFs) ListFilenames() []FilterPath {
	var filenames []FilterPath
	for filename, writable := range ffs.filenames {
		filenames = append(filenames, FilterPath{filename, writable})
	}

	sortFilterPaths(filenames)
	return filenames
}

func (ffs *filterFs) accessibleDir(dir string) (writable bool, ok bool) {
	dir = filepath.Clean(dir)
	for _, fp := range ffs.dirs {
		if len(fp.Path) <= len(dir) && strings.HasPrefix(dir, fp.Path) {
			return fp.Writable, true
		}
	}

	return false, false
}

func (ffs *filterFs) accessibleFilename(filename string) (writable bool, ok bool) {
	filename = filepath.Clean(filename)
	if writable, ok := ffs.filenames[filename]; ok {
		return writable, true
	}

	dir := filepath.Dir(filename)
	for _, fp := range ffs.dirs {
		if len(fp.Path) <= len(dir) && strings.HasPrefix(dir, fp.Path) {
			return fp.Writable, true
		}
	}

	return false, false
}

func (ffs *filterFs) Name() string {
	return "filterFs"
}

func (ffs *filterFs) Create(filename string) (afero.File, error) {
	writable, ok := ffs.accessibleFilename(filename)
	if !ok {
		return nil, &os.PathError{Op: "create", Path: filename, Err: os.ErrNotExist}
	} else if !writable {
		return nil, &os.PathError{Op: "create", Path: filename, Err: os.ErrPermission}
	}

	return ffs.fs.Create(filename)
}

func (ffs *filterFs) Mkdir(dir string, perm os.FileMode) error {
	writable, ok := ffs.accessibleDir(dir)
	if !ok {
		return &os.PathError{Op: "mkdir", Path: dir, Err: os.ErrNotExist}
	} else if !writable {
		return &os.PathError{Op: "mkdir", Path: dir, Err: os.ErrPermission}
	}

	return ffs.fs.Mkdir(dir, perm)
}

func (ffs *filterFs) MkdirAll(dir string, perm os.FileMode) error {
	writable, ok := ffs.accessibleDir(dir)
	if !ok {
		return &os.PathError{Op: "mkdir", Path: dir, Err: os.ErrNotExist}
	} else if !writable {
		return &os.PathError{Op: "mkdir", Path: dir, Err: os.ErrPermission}
	}

	return ffs.fs.MkdirAll(dir, perm)
}

func (ffs *filterFs) Open(filename string) (afero.File, error) {
	_, ok := ffs.accessibleFilename(filename)
	if !ok {
		return nil, &os.PathError{Op: "open", Path: filename, Err: os.ErrNotExist}
	}

	return ffs.fs.Open(filename)
}

func (ffs *filterFs) OpenFile(filename string, flag int, perm os.FileMode) (afero.File, error) {
	writable, ok := ffs.accessibleFilename(filename)
	if !ok {
		return nil, &os.PathError{Op: "open", Path: filename, Err: os.ErrNotExist}
	} else if !writable && flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_APPEND|os.O_TRUNC) != 0 {
		return nil, &os.PathError{Op: "open", Path: filename, Err: os.ErrPermission}
	}

	return ffs.fs.OpenFile(filename, flag, perm)
}

func (ffs *filterFs) Remove(filename string) error {
	writable, ok := ffs.accessibleFilename(filename)
	if !ok {
		return &os.PathError{Op: "remove", Path: filename, Err: os.ErrNotExist}
	} else if !writable {
		return &os.PathError{Op: "remove", Path: filename, Err: os.ErrPermission}
	}

	return ffs.fs.Remove(filename)
}

func (ffs *filterFs) RemoveAll(filename string) error {
	writable, ok := ffs.accessibleFilename(filename)
	if !ok {
		return &os.PathError{Op: "remove", Path: filename, Err: os.ErrNotExist}
	} else if !writable {
		return &os.PathError{Op: "remove", Path: filename, Err: os.ErrPermission}
	}

	return ffs.fs.RemoveAll(filename)
}

func (ffs *filterFs) Rename(oldname, newname string) error {
	writable, ok := ffs.accessibleFilename(oldname)
	if !ok {
		return &os.PathError{Op: "rename", Path: oldname, Err: os.ErrNotExist}
	} else if !writable {
		return &os.PathError{Op: "rename", Path: oldname, Err: os.ErrPermission}
	}
	writable, ok = ffs.accessibleFilename(newname)
	if !ok {
		return &os.PathError{Op: "rename", Path: newname, Err: os.ErrNotExist}
	} else if !writable {
		return &os.PathError{Op: "rename", Path: newname, Err: os.ErrPermission}
	}

	return ffs.fs.Rename(oldname, newname)
}

func (ffs *filterFs) Stat(filename string) (os.FileInfo, error) {
	_, ok := ffs.accessibleFilename(filename)
	if !ok {
		return nil, &os.PathError{Op: "stat", Path: filename, Err: os.ErrNotExist}
	}

	return ffs.fs.Stat(filename)
}

func (ffs *filterFs) Chmod(filename string, mode os.FileMode) error {
	writable, ok := ffs.accessibleFilename(filename)
	if !ok {
		return &os.PathError{Op: "chmod", Path: filename, Err: os.ErrNotExist}
	} else if !writable {
		return &os.PathError{Op: "chmod", Path: filename, Err: os.ErrPermission}
	}

	return ffs.fs.Chmod(filename, mode)
}

func (ffs *filterFs) Chown(filename string, uid, gid int) error {
	writable, ok := ffs.accessibleFilename(filename)
	if !ok {
		return &os.PathError{Op: "chown", Path: filename, Err: os.ErrNotExist}
	} else if !writable {
		return &os.PathError{Op: "chown", Path: filename, Err: os.ErrPermission}
	}

	return ffs.fs.Chown(filename, uid, gid)
}

func (ffs *filterFs) Chtimes(filename string, atime time.Time, mtime time.Time) error {
	writable, ok := ffs.accessibleFilename(filename)
	if !ok {
		return &os.PathError{Op: "chtimes", Path: filename, Err: os.ErrNotExist}
	} else if !writable {
		return &os.PathError{Op: "chtimes", Path: filename, Err: os.ErrPermission}
	}

	return ffs.fs.Chtimes(filename, atime, mtime)
}
