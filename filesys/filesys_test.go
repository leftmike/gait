package filesys

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
)

type filterTestCase struct {
	op       string
	dir      string
	filename string
	newname  string
	writable bool
	ok       bool
	lst      []FilterPath
	fail     bool
}

func filenameToContent(filename string) string {
	return strings.ReplaceAll(filename, "/", " ")
}

func memMapFs(t *testing.T, filenames []string) afero.Fs {
	t.Helper()

	if len(filenames) == 0 {
		return nil
	}

	fs := afero.NewMemMapFs()
	for _, filename := range filenames {
		err := afero.WriteFile(fs, filename, []byte(filenameToContent(filename)), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}
	return fs
}

func testFilter(t *testing.T, filenames []string, cases []filterTestCase) {
	t.Helper()

	ffs := NewFs(memMapFs(t, filenames))

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

		case "Open":
			f, err := ffs.Open(c.filename)
			if c.fail {
				if err == nil {
					f.Close()
					t.Errorf("Open(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("Open(%s) failed with %s", c.filename, err)
			} else {
				cnt, err := io.ReadAll(f)
				f.Close()
				if err != nil {
					t.Errorf("Open(%s) ReadAll failed with %s", c.filename, err)
				} else {
					want := filenameToContent(c.filename)
					if string(cnt) != want {
						t.Errorf("Open(%s) got %s want %s", c.filename, cnt, want)
					}
				}

				fi, err := ffs.Stat(c.filename)
				if err != nil {
					t.Errorf("Stat(%s) failed with %s", c.filename, err)
				} else {
					if fi.Name() != filepath.Base(c.filename) {
						t.Errorf("Stat(%s) got %s want %s", c.filename, fi.Name(),
							filepath.Base(c.filename))
					}
					if fi.IsDir() {
						t.Errorf("Stat(%s) got directory", c.filename)
					}
				}
			}

		case "Stat":
			fi, err := ffs.Stat(c.dir)
			if c.fail {
				if err == nil {
					t.Errorf("Stat(%s) did not fail", c.dir)
				}
			} else if err != nil {
				t.Errorf("Stat(%s) failed with %s", c.dir, err)
			} else if !fi.IsDir() {
				t.Errorf("Stat(%s) not directory", c.dir)
			}

		case "Mkdir":
			err := ffs.Mkdir(c.dir, 0755)
			if c.fail {
				if err == nil {
					t.Errorf("Mkdir(%s) did not fail", c.dir)
				}
			} else if err != nil {
				t.Errorf("Mkdir(%s) failed with %s", c.dir, err)
			}

		case "MkdirAll":
			err := ffs.MkdirAll(c.dir, 0755)
			if c.fail {
				if err == nil {
					t.Errorf("MkdirAll(%s) did not fail", c.dir)
				}
			} else if err != nil {
				t.Errorf("MkdirAll(%s) failed with %s", c.dir, err)
			}

		case "Remove":
			err := ffs.Remove(c.filename)
			if c.fail {
				if err == nil {
					t.Errorf("Remove(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("Remove(%s) failed with %s", c.filename, err)
			}

		case "RemoveAll":
			err := ffs.RemoveAll(c.filename)
			if c.fail {
				if err == nil {
					t.Errorf("RemoveAll(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("RemoveAll(%s) failed with %s", c.filename, err)
			}

		case "Rename":
			err := ffs.Rename(c.filename, c.newname)
			if c.fail {
				if err == nil {
					t.Errorf("Rename(%s, %s) did not fail", c.filename, c.newname)
				}
			} else if err != nil {
				t.Errorf("Rename(%s, %s) failed with %s", c.filename, c.newname, err)
			}

		case "Create":
			f, err := ffs.Create(c.filename)
			if c.fail {
				if err == nil {
					f.Close()
					t.Errorf("Create(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("Create(%s) failed with %s", c.filename, err)
			} else {
				want := filenameToContent(c.filename)
				_, err := f.Write([]byte(want))
				f.Close()
				if err != nil {
					t.Errorf("Create(%s) WriteString failed with %s", c.filename, err)
				} else {
					cnt, err := afero.ReadFile(ffs, c.filename)
					if err != nil {
						t.Errorf("Create(%s) ReadFile failed with %s", c.filename, err)
					} else if string(cnt) != want {
						t.Errorf("Create(%s) got %s want %s", c.filename, cnt, want)
					}
				}
			}

		case "Chmod":
			err := ffs.Chmod(c.filename, 0755)
			if c.fail {
				if err == nil {
					t.Errorf("Chmod(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("Chmod(%s) failed with %s", c.filename, err)
			} else {
				fi, err := ffs.Stat(c.filename)
				if err != nil {
					t.Errorf("Stat(%s) failed with %s", c.filename, err)
				} else if fi.Mode().Perm() != os.FileMode(0755) {
					t.Errorf("Chmod(%s) got %v want %v", c.filename,
						fi.Mode().Perm(), os.FileMode(0755))
				}
			}

		case "Chown":
			err := ffs.Chown(c.filename, 1000, 1000)
			if c.fail {
				if err == nil {
					t.Errorf("Chown(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("Chown(%s) failed with %s", c.filename, err)
			}

		case "Chtimes":
			atime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
			mtime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
			err := ffs.Chtimes(c.filename, atime, mtime)
			if c.fail {
				if err == nil {
					t.Errorf("Chtimes(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("Chtimes(%s) failed with %s", c.filename, err)
			} else {
				fi, err := ffs.Stat(c.filename)
				if err != nil {
					t.Errorf("Stat(%s) failed with %s", c.filename, err)
				} else if !fi.ModTime().Equal(mtime) {
					t.Errorf("Chtimes(%s) got %v want %v", c.filename,
						fi.ModTime(), mtime)
				}
			}

		default:
			t.Fatalf("unexpected op: %s", c.op)
		}
	}
}

func TestFilterDirs(t *testing.T) {
	testFilter(t, nil, []filterTestCase{
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
	testFilter(t, nil, []filterTestCase{
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
	testFilter(t, nil, []filterTestCase{
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
	testFilter(t, nil, []filterTestCase{
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

func TestOpenAccess(t *testing.T) {
	testFilter(t, []string{
		"/home/mike/index.html",
		"/home/mike/src/main.go",
		"/home/mike/src/README.md",
		"/home/mike/bin/command.sh",
		"/home/fred/index.html",
		"/home/fred/src/main.go",
		"/home/fred/src/README.md",
		"/home/fred/bin/command.sh",
	}, []filterTestCase{
		{op: "AddDir", dir: "/home/mike"},
		{op: "AddDir", dir: "/home/mike/src", writable: true},
		{op: "AddFilename", filename: "/home/fred/src/README.md"},
		{op: "Open", filename: "/home/mike/index.html"},
		{op: "Open", filename: "/home/mike/src/main.go"},
		{op: "Open", filename: "/home/mike/bin/command.sh"},
		{op: "Open", filename: "/home/fred/index.html", fail: true},
		{op: "Open", filename: "/home/fred/src/README.md"},
		{op: "Open", filename: "/home/fred/src/main.go", fail: true},
		{op: "Open", filename: "/etc/passwd", fail: true},
		{op: "Stat", dir: "/", fail: true},
		{op: "Stat", dir: "/etc", fail: true},
		{op: "Stat", dir: "/home", fail: true},
		// XXX: {op: "Stat", dir: "/home/mike"},
		{op: "Stat", dir: "/home/mike/src"},
		{op: "Stat", dir: "/home/mike/bin"},
		{op: "AddDir", dir: "/home/fred/src"},
		{op: "Open", filename: "/home/fred/index.html", fail: true},
		{op: "Open", filename: "/home/fred/src/README.md"},
		{op: "Open", filename: "/home/fred/src/main.go"},
		{op: "Open", filename: "/etc/passwd", fail: true},
	})
}

func TestMkdir(t *testing.T) {
	testFilter(t, []string{
		"/home/mike/src/main.go",
	}, []filterTestCase{
		{op: "AddDir", dir: "/home/mike", writable: true},
		{op: "AddDir", dir: "/home"},
		{op: "Mkdir", dir: "/home/mike/newdir"},
		{op: "Stat", dir: "/home/mike/newdir"},
		{op: "Mkdir", dir: "/home/newdir", fail: true},
		{op: "Mkdir", dir: "/etc/newdir", fail: true},
		{op: "MkdirAll", dir: "/home/mike/a/b/c"},
		{op: "Stat", dir: "/home/mike/a/b/c"},
		{op: "MkdirAll", dir: "/home/a/b/c", fail: true},
		{op: "MkdirAll", dir: "/etc/a/b/c", fail: true},
	})
}

func TestRemove(t *testing.T) {
	testFilter(t, []string{
		"/home/mike/src/main.go",
		"/home/mike/src/util.go",
		"/home/mike/index.html",
		"/home/fred/src/main.go",
		"/home/fred/index.html",
	}, []filterTestCase{
		{op: "AddDir", dir: "/home/mike/src", writable: true},
		{op: "AddDir", dir: "/home/mike"},
		{op: "AddFilename", filename: "/home/fred/src/main.go", writable: true},
		{op: "AddFilename", filename: "/home/fred/index.html"},
		{op: "Remove", filename: "/home/mike/src/main.go"},
		{op: "Open", filename: "/home/mike/src/main.go", fail: true},
		{op: "Remove", filename: "/home/mike/index.html", fail: true},
		{op: "Remove", filename: "/etc/passwd", fail: true},
		{op: "Remove", filename: "/home/fred/src/main.go"},
		{op: "Open", filename: "/home/fred/src/main.go", fail: true},
		{op: "Remove", filename: "/home/fred/index.html", fail: true},
		{op: "RemoveAll", filename: "/home/mike/src/util.go"},
		{op: "Open", filename: "/home/mike/src/util.go", fail: true},
		{op: "RemoveAll", filename: "/home/mike/index.html", fail: true},
		{op: "RemoveAll", filename: "/etc/passwd", fail: true},
	})
}

func TestRename(t *testing.T) {
	testFilter(t, []string{
		"/home/mike/src/main.go",
		"/home/mike/src/util.go",
		"/home/mike/index.html",
		"/home/fred/src/main.go",
		"/home/fred/index.html",
	}, []filterTestCase{
		{op: "AddDir", dir: "/home/mike/src", writable: true},
		{op: "AddDir", dir: "/home/mike"},
		{op: "AddFilename", filename: "/home/fred/src/main.go", writable: true},
		{op: "AddFilename", filename: "/home/fred/index.html"},
		{op: "Rename", filename: "/home/mike/src/main.go", newname: "/home/mike/src/app.go"},
		{op: "Open", filename: "/home/mike/src/main.go", fail: true},
		{op: "Rename", filename: "/home/mike/index.html", newname: "/home/mike/home.html",
			fail: true},
		{op: "Rename", filename: "/etc/passwd", newname: "/etc/shadow", fail: true},
		{op: "Rename", filename: "/home/fred/src/main.go", newname: "/home/fred/index.html",
			fail: true},
		{op: "Rename", filename: "/home/fred/index.html", newname: "/home/fred/src/main.go",
			fail: true},
		{op: "Rename", filename: "/home/mike/src/util.go", newname: "/etc/util.go", fail: true},
	})
}

func TestCreate(t *testing.T) {
	testFilter(t, []string{
		"/home/mike/src/main.go",
		"/home/mike/index.html",
		"/home/fred/src/main.go",
		"/home/fred/index.html",
	}, []filterTestCase{
		{op: "AddDir", dir: "/home/mike/src", writable: true},
		{op: "AddDir", dir: "/home/mike"},
		{op: "AddFilename", filename: "/home/fred/src/main.go", writable: true},
		{op: "AddFilename", filename: "/home/fred/index.html"},
		{op: "Create", filename: "/home/mike/src/new.go"},
		{op: "Create", filename: "/home/mike/index.html", fail: true},
		{op: "Create", filename: "/etc/passwd", fail: true},
		{op: "Create", filename: "/home/fred/src/main.go"},
		{op: "Create", filename: "/home/fred/index.html", fail: true},
	})
}

func TestChmod(t *testing.T) {
	testFilter(t, []string{
		"/home/mike/src/main.go",
		"/home/mike/index.html",
		"/home/fred/src/main.go",
		"/home/fred/index.html",
	}, []filterTestCase{
		{op: "AddDir", dir: "/home/mike/src", writable: true},
		{op: "AddDir", dir: "/home/mike"},
		{op: "AddFilename", filename: "/home/fred/src/main.go", writable: true},
		{op: "AddFilename", filename: "/home/fred/index.html"},
		{op: "Chmod", filename: "/home/mike/src/main.go"},
		{op: "Chmod", filename: "/home/mike/index.html", fail: true},
		{op: "Chmod", filename: "/etc/passwd", fail: true},
		{op: "Chmod", filename: "/home/fred/src/main.go"},
		{op: "Chmod", filename: "/home/fred/index.html", fail: true},
	})
}

func TestChown(t *testing.T) {
	testFilter(t, []string{
		"/home/mike/src/main.go",
		"/home/mike/index.html",
		"/home/fred/src/main.go",
		"/home/fred/index.html",
	}, []filterTestCase{
		{op: "AddDir", dir: "/home/mike/src", writable: true},
		{op: "AddDir", dir: "/home/mike"},
		{op: "AddFilename", filename: "/home/fred/src/main.go", writable: true},
		{op: "AddFilename", filename: "/home/fred/index.html"},
		{op: "Chown", filename: "/home/mike/src/main.go"},
		{op: "Chown", filename: "/home/mike/index.html", fail: true},
		{op: "Chown", filename: "/etc/passwd", fail: true},
		{op: "Chown", filename: "/home/fred/src/main.go"},
		{op: "Chown", filename: "/home/fred/index.html", fail: true},
	})
}

func TestChtimes(t *testing.T) {
	testFilter(t, []string{
		"/home/mike/src/main.go",
		"/home/mike/index.html",
		"/home/fred/src/main.go",
		"/home/fred/index.html",
	}, []filterTestCase{
		{op: "AddDir", dir: "/home/mike/src", writable: true},
		{op: "AddDir", dir: "/home/mike"},
		{op: "AddFilename", filename: "/home/fred/src/main.go", writable: true},
		{op: "AddFilename", filename: "/home/fred/index.html"},
		{op: "Chtimes", filename: "/home/mike/src/main.go"},
		{op: "Chtimes", filename: "/home/mike/index.html", fail: true},
		{op: "Chtimes", filename: "/etc/passwd", fail: true},
		{op: "Chtimes", filename: "/home/fred/src/main.go"},
		{op: "Chtimes", filename: "/home/fred/index.html", fail: true},
	})
}
