package sandbox

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func tempDir(t *testing.T) string {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func newHostFS(t *testing.T, p Policy) *HostFS {
	t.Helper()

	hfs, err := NewHostFS(p)
	if err != nil {
		t.Fatal(err)
	}
	return hfs
}

func wantPermission(t *testing.T, err error, what string) {
	t.Helper()

	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("%s = %v, want permission error", what, err)
	}
}

func TestExpandPath(t *testing.T) {
	cases := []struct {
		path string
		want string
		fail bool
	}{
		{path: "/a/b/../c", want: "/a/c"},
		{path: "/", want: "/"},
		{path: "f.txt", want: "/work/f.txt"},
		{path: "./a/f.txt", want: "/work/a/f.txt"},
		{path: "../f.txt", want: "/f.txt"},
		{path: ".", want: "/work"},
		{path: "~", want: "/home/u"},
		{path: "~/f.txt", want: "/home/u/f.txt"},
		{path: "~u/f.txt", fail: true},
		{path: "", fail: true},
	}

	for _, c := range cases {
		got, err := ExpandPath(c.path, "/work", "/home/u")
		if c.fail {
			if err == nil {
				t.Errorf("ExpandPath(%q) did not fail", c.path)
			}
		} else if err != nil {
			t.Errorf("ExpandPath(%q) failed: %s", c.path, err)
		} else if got != c.want {
			t.Errorf("ExpandPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestExpandPathSymlink(t *testing.T) {
	dir := tempDir(t)
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	got, err := ExpandPath("link/new/f.txt", dir, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(target, "new", "f.txt"); got != want {
		t.Errorf("ExpandPath = %q, want %q", got, want)
	}
}

func TestNewHostFSErrors(t *testing.T) {
	dir := tempDir(t)

	policies := []Policy{
		{Workdir: "relative"},
		{Workdir: dir, ReadOnly: []string{"/a"}, Deny: []string{"/a"}},
		{Workdir: dir, ReadWrite: []string{"/a", "/b/../a"}},
		{Workdir: dir, ReadOnly: []string{"~x"}},
	}
	for _, p := range policies {
		if _, err := NewHostFS(p); err == nil {
			t.Errorf("NewHostFS(%+v) did not fail", p)
		}
	}
}

func TestHostFSAccess(t *testing.T) {
	dir := tempDir(t)
	hfs := newHostFS(t, Policy{
		Workdir:   dir,
		ReadOnly:  []string{"/data", "/rw/ro"},
		ReadWrite: []string{"/rw", "/data/rw"},
		Deny:      []string{"secret", "/data/rw/secret"},
	})

	cases := []struct {
		path string
		want access
	}{
		{dir, writeAccess},
		{filepath.Join(dir, "a", "b"), writeAccess},
		{filepath.Join(dir, "secret"), noAccess},
		{filepath.Join(dir, "secret", "f"), noAccess},
		{filepath.Join(dir, "secrets"), writeAccess},
		{"/data", readAccess},
		{"/data/f", readAccess},
		{"/datax", noAccess},
		{"/data/rw/f", writeAccess},
		{"/data/rw/secret", noAccess},
		{"/rw/f", writeAccess},
		{"/rw/ro/f", readAccess},
		{"/", noAccess},
		{"/etc/passwd", noAccess},
	}
	for _, c := range cases {
		if got := hfs.access(c.path); got != c.want {
			t.Errorf("access(%q) = %d, want %d", c.path, got, c.want)
		}
	}
}

func TestHostFSWorkdirOverride(t *testing.T) {
	dir := tempDir(t)
	hfs := newHostFS(t, Policy{Workdir: dir, ReadOnly: []string{"."}})

	if got := hfs.access(filepath.Join(dir, "f")); got != readAccess {
		t.Errorf("access = %d, want read", got)
	}
}

func TestHostFSRoot(t *testing.T) {
	hfs := newHostFS(t, Policy{Workdir: tempDir(t), ReadOnly: []string{"/"}})

	if got := hfs.access("/etc/passwd"); got != readAccess {
		t.Errorf("access = %d, want read", got)
	}
}

func TestHostFSOps(t *testing.T) {
	ctx := context.Background()
	dir := tempDir(t)
	ro := tempDir(t)
	if err := os.WriteFile(filepath.Join(ro, "f.txt"), []byte("ro"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	hfs := newHostFS(t, Policy{Workdir: dir, ReadOnly: []string{ro}, Deny: []string{"secret"}})

	if err := hfs.WriteFile(ctx, "f.txt", []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := hfs.ReadFile(ctx, "f.txt"); err != nil {
		t.Error(err)
	} else if string(data) != "data" {
		t.Errorf("ReadFile = %q, want data", data)
	}
	if fi, err := hfs.Stat(ctx, "f.txt"); err != nil {
		t.Error(err)
	} else if fi.Size() != 4 {
		t.Errorf("Stat size = %d, want 4", fi.Size())
	}
	if err := hfs.Remove(ctx, "f.txt"); err != nil {
		t.Error(err)
	}

	if data, err := hfs.ReadFile(ctx, filepath.Join(ro, "f.txt")); err != nil {
		t.Error(err)
	} else if string(data) != "ro" {
		t.Errorf("ReadFile = %q, want ro", data)
	}
	wantPermission(t, hfs.WriteFile(ctx, filepath.Join(ro, "g.txt"), nil, 0o644), "WriteFile")
	wantPermission(t, hfs.Remove(ctx, filepath.Join(ro, "f.txt")), "Remove")
	wantPermission(t, hfs.MkdirAll(ctx, filepath.Join(ro, "d"), 0o755), "MkdirAll")

	_, err := hfs.ReadFile(ctx, "secret")
	wantPermission(t, err, "ReadFile secret")
	_, err = hfs.Stat(ctx, "secret")
	wantPermission(t, err, "Stat secret")
	wantPermission(t, hfs.WriteFile(ctx, "secret", nil, 0o644), "WriteFile secret")
	_, err = hfs.ReadFile(ctx, "/etc/passwd")
	wantPermission(t, err, "ReadFile outside")
}

func TestHostFSMkdirAllParents(t *testing.T) {
	ctx := context.Background()
	dir := tempDir(t)
	hfs := newHostFS(t, Policy{
		Workdir:   dir,
		ReadOnly:  []string{"a"},
		ReadWrite: []string{"a/b/c"},
	})

	wantPermission(t, hfs.MkdirAll(ctx, "a/b/c", 0o755), "MkdirAll through read-only")
	if _, err := os.Stat(filepath.Join(dir, "a")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("read-only parent was created: %v", err)
	}

	if err := hfs.MkdirAll(ctx, "x/y/z", 0o755); err != nil {
		t.Error(err)
	}
}

func TestHostFSRemoveSymlink(t *testing.T) {
	ctx := context.Background()
	dir := tempDir(t)
	ro := tempDir(t)
	target := filepath.Join(ro, "f.txt")
	if err := os.WriteFile(target, []byte("ro"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	hfs := newHostFS(t, Policy{Workdir: dir, ReadOnly: []string{ro}})

	wantPermission(t, hfs.WriteFile(ctx, "link", nil, 0o644), "WriteFile through link")
	if err := hfs.Remove(ctx, "link"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("link not removed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("target removed: %v", err)
	}
}

func TestHostFSWalkDir(t *testing.T) {
	ctx := context.Background()
	dir := tempDir(t)
	for _, path := range []string{"a/f", "a/secret", "b/f", "hidden/f"} {
		path = filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hfs := newHostFS(t, Policy{Workdir: dir, Deny: []string{"a/secret", "hidden"}})

	var got []string
	err := hfs.WalkDir(ctx, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		got = append(got, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{".", "a", "a/f", "b", "b/f"}
	if !slices.Equal(got, want) {
		t.Errorf("WalkDir = %v, want %v", got, want)
	}

	err = hfs.WalkDir(ctx, "hidden", func(string, fs.DirEntry, error) error { return nil })
	wantPermission(t, err, "WalkDir denied root")
}

func TestUncheckedFS(t *testing.T) {
	hfs, err := UncheckedFS(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hfs.ReadFile(context.Background(), "/etc/passwd"); err != nil {
		t.Errorf("ReadFile = %v", err)
	}
}
