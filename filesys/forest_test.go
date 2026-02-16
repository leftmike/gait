package filesys

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type forestTestCase struct {
	op       string
	path     string
	dir      string
	filename string
	resolved string
	writable bool
	ok       bool
	lst      []Tree
	dirs     []string
	fail     bool
}

func filenameToContent(filename string) string {
	return strings.ReplaceAll(filename, "/", " ")
}

func newForestFS(t *testing.T, tmp string, filenames []string) *ForestFS {
	for _, filename := range filenames {
		tmpname := filepath.Join(tmp, filename)
		err := os.MkdirAll(filepath.Dir(tmpname), 0755)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(tmpname, []byte(filenameToContent(filename)),
			0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	return NewForestFS()
}

func testForest(t *testing.T, filenames []string, cases []forestTestCase) {
	t.Helper()

	tmp := t.TempDir()
	ffs := newForestFS(t, tmp, filenames)
	defer ffs.Close()

	for _, c := range cases {
		switch c.op {
		case "AddTree":
			err := ffs.AddTree(filepath.Join(tmp, c.path), c.writable)
			if c.fail {
				if err == nil {
					t.Errorf("AddTree(%s) did not fail", c.path)
				}
			} else if err != nil {
				t.Errorf("AddTree(%s) failed with %s", c.path, err)
			}

		case "RemoveTree":
			ffs.RemoveTree(filepath.Join(tmp, c.path))

		case "ListTrees":
			lst := ffs.ListTrees()
			for i := range c.lst {
				c.lst[i].Path = filepath.Join(tmp, c.lst[i].Path)
			}
			if !reflect.DeepEqual(lst, c.lst) {
				t.Errorf("ListTrees() got %v want %v", lst, c.lst)
			}

		case "resolvePath":
			_, s, ok := ffs.resolvePath(filepath.Join(tmp, c.path))
			if c.resolved != s || c.ok != ok {
				t.Errorf("resolvePath(%s) got (%s, %v) want (%s, %v)", c.path, s, ok,
					c.resolved, c.ok)
			}

		case "ReadFile":
			cnt, err := ffs.ReadFile(filepath.Join(tmp, c.filename))
			if c.fail {
				if err == nil {
					t.Errorf("ReadFile(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("ReadFile(%s) failed with %s", c.filename, err)
			} else {
				want := filenameToContent(c.filename)
				if string(cnt) != want {
					t.Errorf("ReadFile(%s) got %s want %s", c.filename, cnt, want)
				}
			}

		case "WriteFile":
			err := ffs.WriteFile(filepath.Join(tmp, c.filename),
				[]byte(filenameToContent(c.filename)), 0644)
			if c.fail {
				if err == nil {
					t.Errorf("WriteFile(%s) did not fail", c.filename)
				}
			} else if err != nil {
				t.Errorf("WriteFile(%s) failed with %s", c.filename, err)
			}

		case "ReadDir":
			entries, err := ffs.ReadDir(filepath.Join(tmp, c.dir))
			if c.fail {
				if err == nil {
					t.Errorf("ReadDir(%s) did not fail", c.dir)
				}
			} else if err != nil {
				t.Errorf("ReadDir(%s) failed with %s", c.dir, err)
			} else {
				var dirs []string
				for _, e := range entries {
					name := e.Name()
					if e.IsDir() {
						name = name + "/"
					}
					dirs = append(dirs, name)
				}
				sort.Strings(dirs)

				if !reflect.DeepEqual(dirs, c.dirs) {
					t.Errorf("ReadDir(%s) got %v want %v", c.dir, dirs, c.dirs)
				}
			}

		default:
			t.Fatalf("unexpected op: %s", c.op)
		}
	}
}

func TestAddRemoveListTrees(t *testing.T) {
	testForest(t,
		[]string{
			"/usr/usr.txt",
			"/home/home.txt",
			"/etc/etc.txt",
			"/home/mike/home.mike.txt",
			"/home/mikemon/home.mikemon.txt",
			"/home/mik/home.mik.txt",
			"/var/readme.txt",
		},
		[]forestTestCase{
			{op: "AddTree", path: "/usr"},
			{op: "AddTree", path: "/home"},
			{op: "AddTree", path: "/etc/", writable: true},
			{
				op: "ListTrees",
				lst: []Tree{
					{"/home", false},
					{"/etc", true},
					{"/usr", false},
				},
			},
			{op: "AddTree", path: "kernel", fail: true},
			{op: "AddTree", path: "/kernel", fail: true},
			{op: "AddTree", path: "/home/mike", fail: true},
			{op: "AddTree", path: "/home", writable: true, fail: true},
			{
				op: "ListTrees",
				lst: []Tree{
					{"/home", false},
					{"/etc", true},
					{"/usr", false},
				},
			},
			{op: "RemoveTree", path: "/home/mike"},
			{
				op: "ListTrees",
				lst: []Tree{
					{"/home", false},
					{"/etc", true},
					{"/usr", false},
				},
			},
			{op: "RemoveTree", path: "/home"},
			{
				op: "ListTrees",
				lst: []Tree{
					{"/etc", true},
					{"/usr", false},
				},
			},
			{op: "AddTree", path: "/home/mike", writable: true},
			{
				op: "ListTrees",
				lst: []Tree{
					{"/home/mike", true},
					{"/etc", true},
					{"/usr", false},
				},
			},
			{op: "AddTree", path: "/home/mikemon"},
			{op: "AddTree", path: "/home/mik", writable: true},
			{
				op: "ListTrees",
				lst: []Tree{
					{"/home/mikemon", false},
					{"/home/mike", true},
					{"/home/mik", true},
					{"/etc", true},
					{"/usr", false},
				},
			},
			{op: "RemoveTree", path: "/home/mike"},
			{op: "RemoveTree", path: "/kernel"},
			{op: "RemoveTree", path: "kernel"},
			{op: "RemoveTree", path: "/etc"},
			{op: "RemoveTree", path: "/usr"},
			{
				op: "ListTrees",
				lst: []Tree{
					{"/home/mikemon", false},
					{"/home/mik", true},
				},
			},
		})
}

func TestResolvePath(t *testing.T) {
	testForest(t,
		[]string{
			"/usr/usr.txt",
			"/home/home.txt",
			"/etc/etc.txt",
			"/home/mike/home.mike.txt",
			"/home/mikemon/home.mikemon.txt",
			"/home/mik/home.mik.txt",
			"/var/readme.txt",
		},
		[]forestTestCase{
			{op: "AddTree", path: "/usr"},
			{op: "AddTree", path: "/home"},
			{op: "AddTree", path: "/etc/", writable: true},
			{op: "resolvePath", path: "/home", resolved: ".", ok: true},
			{op: "resolvePath", path: "/home/README.md", resolved: "README.md", ok: true},
			{op: "resolvePath", path: "home/README.md", resolved: "README.md", ok: true},
			{op: "resolvePath", path: "/home/mike/.bash", resolved: "mike/.bash", ok: true},
			{op: "resolvePath", path: "/kernel"},
			{op: "resolvePath", path: "/hom"},
			{op: "resolvePath", path: "/homes"},
		})
}

func TestReadFile(t *testing.T) {
	testForest(t,
		[]string{
			"/usr/usr.txt",
			"/home/home.txt",
			"/etc/etc.txt",
			"/home/mike/home.mike.txt",
			"/home/mikemon/home.mikemon.txt",
			"/home/mik/home.mik.txt",
			"/var/readme.txt",
		},
		[]forestTestCase{
			{op: "AddTree", path: "/usr"},
			{op: "AddTree", path: "/home"},
			{op: "AddTree", path: "/etc/", writable: true},
			{op: "ReadFile", filename: "/home/home.txt"},
			{op: "ReadFile", filename: "/var/readme.txt", fail: true},
		})
}

func TestWriteFile(t *testing.T) {
	testForest(t,
		[]string{
			"/usr/usr.txt",
			"/home/home.txt",
			"/etc/etc.txt",
			"/home/mike/mike.txt",
			"/home/mikemon/mikemon.txt",
			"/home/mik/mik.txt",
			"/var/readme.txt",
		},
		[]forestTestCase{
			{op: "AddTree", path: "/usr"},
			{op: "AddTree", path: "/home", writable: true},
			{op: "AddTree", path: "/etc/"},
			{op: "ReadFile", filename: "/home/mike/mike.txt"},
			{op: "ReadFile", filename: "/home/john.txt", fail: true},
			{op: "WriteFile", filename: "/home/john.txt"},
			{op: "ReadFile", filename: "/home/john.txt"},
			{op: "WriteFile", filename: "/var/readme.txt", fail: true},
			{op: "WriteFile", filename: "/kernel/readme.txt", fail: true},
		})
}

func TestReadDir(t *testing.T) {
	testForest(t,
		[]string{
			"/home/home.txt",
			"/home/mike/mike.txt",
			"/home/mikemon/mikemon.txt",
			"/home/mik/mik.txt",
			"/home/mike.txt",
			"/var/readme.txt",
		},
		[]forestTestCase{
			{op: "AddTree", path: "/home", writable: true},
			{op: "ReadDir", dir: "/home/mike", dirs: []string{"mike.txt"}},
			{op: "ReadDir", dir: "/home",
				dirs: []string{"home.txt", "mik/", "mike.txt", "mike/", "mikemon/"}},
			{op: "WriteFile", filename: "/home/john.txt"},
			{op: "ReadDir", dir: "/home",
				dirs: []string{"home.txt", "john.txt", "mik/", "mike.txt", "mike/", "mikemon/"}},
			{op: "WriteFile", filename: "/home/mike/fred.txt"},
			{op: "ReadDir", dir: "/home/mike", dirs: []string{"fred.txt", "mike.txt"}},
			{op: "ReadDir", dir: "/var", fail: true},
			{op: "ReadDir", dir: "/etc", fail: true},
		})
}
