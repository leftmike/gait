package filesys

import (
	"reflect"
	"testing"
)

type filterTestCase struct {
	op       string
	dir      string
	filename string
	writable bool
	ok       bool
	lst      []FilterPath
	fail     bool
}

func testFilter(t *testing.T, cases []filterTestCase) {
	t.Helper()

	ffs := NewFs(nil)
	for _, c := range cases {
		switch c.op {
		case "AddDir":
			err := ffs.AddDir(c.dir, c.writable)
			if c.fail {
				if err == nil {
					t.Errorf("AddDir(%s) did not fail", c.dir)
				}
			} else if err != nil {
				t.Errorf("AddDir(%s) failed with %s", c.dir, err)
			}

		case "RemoveDir":
			ffs.RemoveDir(c.dir)

		case "ListDirs":
			lst := ffs.ListDirs()
			if !reflect.DeepEqual(lst, c.lst) {
				t.Errorf("ListDirs() got %v want %v", lst, c.lst)
			}

		case "AddFilename":
			err := ffs.AddFilename(c.filename, c.writable)
			if c.fail {
				if err == nil {
					t.Errorf("AddFilename(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("AddFilename(%s) failed with %s", c.filename, err)
			}

		case "RemoveFilename":
			ffs.RemoveFilename(c.filename)

		case "ListFilenames":
			lst := ffs.ListFilenames()
			if !reflect.DeepEqual(lst, c.lst) {
				t.Errorf("ListFilenames() got %v want %v", lst, c.lst)
			}

		case "accessibleDir":
			writable, ok := ffs.accessibleDir(c.dir)
			if writable != c.writable || ok != c.ok {
				t.Errorf("accessibleDir(%s) got (%v, %v) want (%v, %v)", c.dir,
					writable, ok, c.writable, c.ok)
			}

		case "accessibleFilename":
			writable, ok := ffs.accessibleFilename(c.filename)
			if writable != c.writable || ok != c.ok {
				t.Errorf("accessibleFilename(%s) got (%v, %v) want (%v, %v)", c.filename,
					writable, ok, c.writable, c.ok)
			}

		default:
			t.Fatalf("unexpected op: %s", c.op)
		}
	}
}

func TestFilterDirs(t *testing.T) {
	testFilter(t, []filterTestCase{
		{op: "AddDir", dir: "home", fail: true},
		{op: "AddDir", dir: "home/mike", fail: true},
		{op: "AddDir", dir: "/home"},
		{op: "AddDir", dir: "/home/mike", writable: true},
		{op: "AddDir", dir: "/usr", writable: true},
		{
			op: "ListDirs",
			lst: []FilterPath{
				{"/home/mike", true},
				{"/home", false},
				{"/usr", true},
			},
		},
		{op: "AddDir", dir: "/usr", writable: false},
		{
			op: "ListDirs",
			lst: []FilterPath{
				{"/home/mike", true},
				{"/home", false},
				{"/usr", false},
			},
		},
		{op: "RemoveDir", dir: "home"},
		{op: "RemoveDir", dir: "home/mike"},
		{
			op: "ListDirs",
			lst: []FilterPath{
				{"/home/mike", true},
				{"/home", false},
				{"/usr", false},
			},
		},
		{op: "RemoveDir", dir: "/home"},
		{
			op: "ListDirs",
			lst: []FilterPath{
				{"/home/mike", true},
				{"/usr", false},
			},
		},
		{op: "RemoveDir", dir: "/home"},
		{
			op: "ListDirs",
			lst: []FilterPath{
				{"/home/mike", true},
				{"/usr", false},
			},
		},
		{op: "RemoveDir", dir: "/home/mike"},
		{op: "RemoveDir", dir: "/usr"},
		{op: "ListDirs", lst: []FilterPath{}},
	})
}

func TestFilterFilenames(t *testing.T) {
	testFilter(t, []filterTestCase{
		{op: "AddFilename", filename: "home/mike/README.md", fail: true},
		{op: "AddFilename", filename: "home/mike/list.txt", fail: true},
		{op: "AddFilename", filename: "/home/mike/README.md"},
		{op: "AddFilename", filename: "/home/mike/list.txt", writable: true},
		{op: "AddFilename", filename: "/usr/hosts.txt", writable: true},
		{
			op: "ListFilenames",
			lst: []FilterPath{
				{"/home/mike/README.md", false},
				{"/home/mike/list.txt", true},
				{"/usr/hosts.txt", true},
			},
		},
		{op: "AddFilename", filename: "/usr/hosts.txt", writable: false},
		{
			op: "ListFilenames",
			lst: []FilterPath{
				{"/home/mike/README.md", false},
				{"/home/mike/list.txt", true},
				{"/usr/hosts.txt", false},
			},
		},
		{op: "RemoveFilename", filename: "home/README.md"},
		{op: "RemoveFilename", filename: "home/mike/list.txt"},
		{
			op: "ListFilenames",
			lst: []FilterPath{
				{"/home/mike/README.md", false},
				{"/home/mike/list.txt", true},
				{"/usr/hosts.txt", false},
			},
		},
		{op: "RemoveFilename", filename: "/home/mike/list.txt"},
		{
			op: "ListFilenames",
			lst: []FilterPath{
				{"/home/mike/README.md", false},
				{"/usr/hosts.txt", false},
			},
		},
		{op: "RemoveFilename", filename: "/home/mike/list.txt"},
		{
			op: "ListFilenames",
			lst: []FilterPath{
				{"/home/mike/README.md", false},
				{"/usr/hosts.txt", false},
			},
		},
		{op: "RemoveFilename", filename: "/home/mike/README.md"},
		{op: "RemoveFilename", filename: "/usr/hosts.txt"},
		{op: "ListFilenames"},
	})
}

func TestAccessibleDir(t *testing.T) {
	testFilter(t, []filterTestCase{
		{op: "AddDir", dir: "/home/mike", writable: true},
		{op: "AddDir", dir: "/home"},
		{op: "AddDir", dir: "/home/john/src", writable: true},
		{
			op: "ListDirs",
			lst: []FilterPath{
				{"/home/john/src", true},
				{"/home/mike", true},
				{"/home", false},
			},
		},
		{op: "accessibleDir", dir: "/etc", writable: false, ok: false},
		{op: "accessibleDir", dir: "/home", writable: false, ok: true},
		{op: "accessibleDir", dir: "/home/mike", writable: true, ok: true},
		{op: "accessibleDir", dir: "/home/mike/bin", writable: true, ok: true},
		{op: "accessibleDir", dir: "/home/mike/src", writable: true, ok: true},
		{op: "accessibleDir", dir: "/home/john", writable: false, ok: true},
		{op: "accessibleDir", dir: "/home/john/bin", writable: false, ok: true},
		{op: "accessibleDir", dir: "/home/john/src", writable: true, ok: true},
		{op: "accessibleDir", dir: "/home/fred", writable: false, ok: true},
		{op: "accessibleDir", dir: "/home/fred/bin", writable: false, ok: true},
		{op: "accessibleDir", dir: "/home/fred/src", writable: false, ok: true},
		{op: "AddFilename", filename: "/home/fred/src", writable: true},
		{op: "accessibleDir", dir: "/home/fred/src", writable: false, ok: true},
		{op: "AddFilename", filename: "/usr/bin"},
		{op: "accessibleDir", dir: "/usr/bin", writable: false, ok: false},
	})
}

func TestAccessibleFilename(t *testing.T) {
	testFilter(t, []filterTestCase{
		{op: "AddDir", dir: "/home/mike", writable: true},
		{op: "AddDir", dir: "/home"},
		{op: "AddDir", dir: "/home/john/src", writable: true},
		{
			op: "ListDirs",
			lst: []FilterPath{
				{"/home/john/src", true},
				{"/home/mike", true},
				{"/home", false},
			},
		},
		{op: "AddFilename", filename: "/home/fred/README.md", writable: true},
		{op: "AddFilename", filename: "/etc/passwd"},
		{op: "AddFilename", filename: "/etc/hosts", writable: true},
		{op: "accessibleFilename", filename: "/home/fred/README.md", writable: true, ok: true},
		{op: "accessibleFilename", filename: "/etc/passwd", writable: false, ok: true},
		{op: "accessibleFilename", filename: "/etc/hosts", writable: true, ok: true},
		{op: "accessibleFilename", filename: "/etc/sudoers", writable: false, ok: false},
		{op: "accessibleFilename", filename: "/home/fred/src/README.md", ok: true},
		{op: "accessibleFilename", filename: "/home/john/README.md", writable: false, ok: true},
		{op: "accessibleFilename", filename: "/home/john/src/README.md", writable: true, ok: true},
	})
}

/*
// setupMemFs creates a MemMapFs with a directory tree for testing:
//
//	/home/user/project/src/main.go
//	/home/user/project/src/util.go
//	/home/user/project/docs/readme.md
//	/home/user/project/config.yaml
//	/home/user/other/secret.txt
//	/tmp/scratch.txt
func setupMemFs(t *testing.T) afero.Fs {
	t.Helper()
	base := afero.NewMemMapFs()
	files := map[string]string{
		"/home/user/project/src/main.go":    "package main",
		"/home/user/project/src/util.go":    "package main",
		"/home/user/project/docs/readme.md": "# Readme",
		"/home/user/project/config.yaml":    "key: value",
		"/home/user/other/secret.txt":       "secret",
		"/tmp/scratch.txt":                  "scratch",
	}
	for path, content := range files {
		if err := afero.WriteFile(base, path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return base
}

func TestAllowedDirAccess(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	data, err := afero.ReadFile(fs, "/home/user/project/src/main.go")
	if err != nil {
		t.Fatalf("read allowed file: %v", err)
	}
	if string(data) != "package main" {
		t.Fatalf("unexpected content: %q", data)
	}

	info, err := fs.Stat("/home/user/project/src/main.go")
	if err != nil {
		t.Fatalf("stat allowed file: %v", err)
	}
	if info.Name() != "main.go" {
		t.Fatalf("unexpected name: %s", info.Name())
	}

	info, err = fs.Stat("/home/user/project/src")
	if err != nil {
		t.Fatalf("stat allowed dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestAllowedFileAccess(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddFilename("/home/user/project/config.yaml", true)

	data, err := afero.ReadFile(fs, "/home/user/project/config.yaml")
	if err != nil {
		t.Fatalf("read allowed file: %v", err)
	}
	if string(data) != "key: value" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestDisallowedAccess(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	tests := []struct {
		name string
		path string
	}{
		{"outside dir", "/home/user/other/secret.txt"},
		{"sibling dir", "/home/user/project/docs/readme.md"},
		{"sibling file", "/home/user/project/config.yaml"},
		{"unrelated path", "/tmp/scratch.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fs.Open(tt.path)
			if err == nil {
				t.Fatalf("expected error opening %s", tt.path)
			}
			if !os.IsNotExist(err) {
				t.Fatalf("expected not-exist error, got: %v", err)
			}
		})
	}
}

func TestAncestorTraversal(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)
	fs.AddFilename("/home/user/project/config.yaml", true)

	for _, path := range []string{"/", "/home", "/home/user", "/home/user/project"} {
		_, err := fs.Stat(path)
		if err != nil {
			t.Fatalf("stat ancestor %s: %v", path, err)
		}
	}

	_, err := fs.Stat("/home/user/other")
	if err == nil {
		t.Fatal("expected error for non-ancestor path")
	}
}

func TestFilteredReaddir(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)
	fs.AddFilename("/home/user/project/config.yaml", true)

	f, err := fs.Open("/home/user/project")
	if err != nil {
		t.Fatalf("open ancestor dir: %v", err)
	}
	defer f.Close()

	entries, err := f.Readdir(-1)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	sort.Strings(names)

	expected := []string{"config.yaml", "src"}
	if len(names) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, names)
	}
	for i := range expected {
		if names[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, names)
		}
	}
}

func TestFilteredReaddirnames(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	f, err := fs.Open("/home/user/project")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatalf("readdirnames: %v", err)
	}

	if len(names) != 1 || names[0] != "src" {
		t.Fatalf("expected [src], got %v", names)
	}
}

func TestReaddirWithCount(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)
	fs.AddDir("/home/user/project/docs", true)
	fs.AddFilename("/home/user/project/config.yaml", true)

	f, err := fs.Open("/home/user/project")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var allNames []string
	for {
		entries, err := f.Readdir(1)
		for _, e := range entries {
			allNames = append(allNames, e.Name())
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("readdir: %v", err)
		}
	}

	sort.Strings(allNames)
	expected := []string{"config.yaml", "docs", "src"}
	if len(allNames) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, allNames)
	}
	for i := range expected {
		if allNames[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, allNames)
		}
	}
}

func TestUnfilteredDirListingWithinAllowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	f, err := fs.Open("/home/user/project/src")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	entries, err := f.Readdir(-1)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	sort.Strings(names)

	expected := []string{"main.go", "util.go"}
	if len(names) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, names)
	}
	for i := range expected {
		if names[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, names)
		}
	}
}

func TestCreateInAllowedDir(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	f, err := fs.Create("/home/user/project/src/new.go")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = f.WriteString("package main")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	data, err := afero.ReadFile(fs, "/home/user/project/src/new.go")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "package main" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestCreateDisallowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	_, err := fs.Create("/home/user/project/docs/evil.md")
	if err == nil {
		t.Fatal("expected error creating in disallowed dir")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
}

func TestMkdirAllowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.Mkdir("/home/user/project/src/sub", 0755)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	info, err := fs.Stat("/home/user/project/src/sub")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestMkdirDisallowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.Mkdir("/home/user/project/newdir", 0755)
	if err == nil {
		t.Fatal("expected error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
}

func TestMkdirAllAllowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.MkdirAll("/home/user/project/src/a/b/c", 0755)
	if err != nil {
		t.Fatalf("mkdirall: %v", err)
	}

	info, err := fs.Stat("/home/user/project/src/a/b/c")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestRemoveAllowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.Remove("/home/user/project/src/util.go")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err = fs.Stat("/home/user/project/src/util.go")
	if err == nil {
		t.Fatal("expected file to be removed")
	}
}

func TestRemoveDisallowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.Remove("/home/user/project/config.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
}

func TestRenameAllowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.Rename("/home/user/project/src/util.go", "/home/user/project/src/helpers.go")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}

	_, err = fs.Stat("/home/user/project/src/helpers.go")
	if err != nil {
		t.Fatalf("stat renamed file: %v", err)
	}
}

func TestRenameDisallowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.Rename("/home/user/project/src/main.go", "/tmp/main.go")
	if err == nil {
		t.Fatal("expected error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}

	err = fs.Rename("/tmp/scratch.txt", "/home/user/project/src/scratch.txt")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenFileWriteDisallowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	// Writing to a visible-only ancestor should be denied with ErrPermission.
	_, err := fs.OpenFile("/home/user/project", os.O_RDWR, 0644)
	if err == nil {
		t.Fatal("expected error")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission error, got: %v", err)
	}

	// Writing to an invisible path should be denied with ErrNotExist.
	_, err = fs.OpenFile("/home/user/project/docs/readme.md", os.O_RDWR, 0644)
	if err == nil {
		t.Fatal("expected error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
}

func TestOpenFileReadOnlyAncestor(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	f, err := fs.OpenFile("/home/user/project", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("openfile readonly ancestor: %v", err)
	}
	f.Close()
}

func TestRootDirAsAllowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/", true)

	for _, path := range []string{
		"/home/user/project/src/main.go",
		"/home/user/other/secret.txt",
		"/tmp/scratch.txt",
	} {
		_, err := afero.ReadFile(fs, path)
		if err != nil {
			t.Fatalf("read %s with root allowed: %v", path, err)
		}
	}
}

func TestEmptyAllowLists(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)

	_, err := fs.Open("/home/user/project/src/main.go")
	if err == nil {
		t.Fatal("expected error with empty allow lists")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
}

func TestChtimesAllowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	info, _ := fs.Stat("/home/user/project/src/main.go")
	origMod := info.ModTime()

	newTime := origMod.Add(1000)
	err := fs.Chtimes("/home/user/project/src/main.go", newTime, newTime)
	if err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func TestChtimesDisallowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	info, _ := base.Stat("/tmp/scratch.txt")
	err := fs.Chtimes("/tmp/scratch.txt", info.ModTime(), info.ModTime())
	if err == nil {
		t.Fatal("expected error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
}

func TestChmodAllowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.Chmod("/home/user/project/src/main.go", 0600)
	if err != nil {
		t.Fatalf("chmod: %v", err)
	}
}

func TestChmodDisallowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.Chmod("/tmp/scratch.txt", 0600)
	if err == nil {
		t.Fatal("expected error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
}

func TestName(t *testing.T) {
	fs := NewFs(afero.NewMemMapFs())
	if fs.Name() != "filterFs" {
		t.Fatalf("unexpected name: %s", fs.Name())
	}
}

func TestRemoveAllAllowed(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", true)

	err := fs.RemoveAll("/home/user/project/src")
	if err != nil {
		t.Fatalf("removeall: %v", err)
	}

	_, err = fs.Stat("/home/user/project/src")
	if err == nil {
		t.Fatal("expected dir to be removed")
	}
}

func TestAddDropListDirs(t *testing.T) {
	fs := NewFs(afero.NewMemMapFs())

	if len(fs.ListDirs()) != 0 {
		t.Fatal("expected empty list")
	}

	fs.AddDir("/a", false)
	fs.AddDir("/c", false)
	fs.AddDir("/b", false)
	if got := fs.ListDirs(); len(got) != 3 || got[0] != "/a" || got[1] != "/b" || got[2] != "/c" {
		t.Fatalf("expected [/a /b /c], got %v", got)
	}

	fs.DropDir("/b")
	if got := fs.ListDirs(); len(got) != 2 || got[0] != "/a" || got[1] != "/c" {
		t.Fatalf("expected [/a /c], got %v", got)
	}

	// Drop non-existent path is a no-op.
	fs.DropDir("/nonexistent")
	if len(fs.ListDirs()) != 2 {
		t.Fatal("drop of non-existent path should be a no-op")
	}
}

func TestAddDropListFiles(t *testing.T) {
	fs := NewFs(afero.NewMemMapFs())

	if len(fs.ListFiles()) != 0 {
		t.Fatal("expected empty list")
	}

	fs.AddFilename("/a.txt", false)
	fs.AddFilename("/c.txt", false)
	fs.AddFilename("/b.txt", false)
	if got := fs.ListFiles(); len(got) != 3 || got[0] != "/a.txt" || got[1] != "/b.txt" || got[2] != "/c.txt" {
		t.Fatalf("expected [/a.txt /b.txt /c.txt], got %v", got)
	}

	fs.DropFile("/b.txt")
	if got := fs.ListFiles(); len(got) != 2 || got[0] != "/a.txt" || got[1] != "/c.txt" {
		t.Fatalf("expected [/a.txt /c.txt], got %v", got)
	}

	// Drop non-existent path is a no-op.
	fs.DropFile("/nonexistent")
	if len(fs.ListFiles()) != 2 {
		t.Fatal("drop of non-existent path should be a no-op")
	}
}

func TestReadOnlyDirRead(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", false)

	data, err := afero.ReadFile(fs, "/home/user/project/src/main.go")
	if err != nil {
		t.Fatalf("read in read-only dir: %v", err)
	}
	if string(data) != "package main" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestReadOnlyDirWrite(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddDir("/home/user/project/src", false)

	_, err := fs.Create("/home/user/project/src/new.go")
	if err == nil {
		t.Fatal("expected error creating in read-only dir")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission error, got: %v", err)
	}

	err = fs.Mkdir("/home/user/project/src/sub", 0755)
	if err == nil {
		t.Fatal("expected error mkdir in read-only dir")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission error, got: %v", err)
	}

	err = fs.Remove("/home/user/project/src/util.go")
	if err == nil {
		t.Fatal("expected error removing in read-only dir")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission error, got: %v", err)
	}

	err = fs.Rename("/home/user/project/src/util.go", "/home/user/project/src/helpers.go")
	if err == nil {
		t.Fatal("expected error renaming in read-only dir")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission error, got: %v", err)
	}

	_, err = fs.OpenFile("/home/user/project/src/main.go", os.O_RDWR, 0644)
	if err == nil {
		t.Fatal("expected error opening for write in read-only dir")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission error, got: %v", err)
	}
}

func TestReadOnlyFileRead(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddFilename("/home/user/project/config.yaml", false)

	data, err := afero.ReadFile(fs, "/home/user/project/config.yaml")
	if err != nil {
		t.Fatalf("read read-only file: %v", err)
	}
	if string(data) != "key: value" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestReadOnlyFileWrite(t *testing.T) {
	base := setupMemFs(t)
	fs := NewFs(base)
	fs.AddFilename("/home/user/project/config.yaml", false)

	_, err := fs.OpenFile("/home/user/project/config.yaml", os.O_RDWR, 0644)
	if err == nil {
		t.Fatal("expected error opening read-only file for write")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission error, got: %v", err)
	}

	err = fs.Remove("/home/user/project/config.yaml")
	if err == nil {
		t.Fatal("expected error removing read-only file")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission error, got: %v", err)
	}

	err = fs.Chmod("/home/user/project/config.yaml", 0600)
	if err == nil {
		t.Fatal("expected error chmod on read-only file")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission error, got: %v", err)
	}
}
*/
