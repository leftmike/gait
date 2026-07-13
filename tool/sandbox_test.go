package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leftmike/sandbox"
)

func TestCheckReadWrite(t *testing.T) {
	sb := &sandbox.Sandbox{
		FSP: &sandbox.FSPolicy{
			Read:    []string{"/read"},
			Write:   []string{"/write"},
			Execute: []string{"/execute"},
		},
	}

	cases := []struct {
		path  string
		read  bool
		write bool
	}{
		{path: "/read", read: true},
		{path: "/read/file.txt", read: true},
		{path: "/read/dir/file.txt", read: true},
		{path: "/readme.txt"},
		{path: "/write", read: true, write: true},
		{path: "/write/file.txt", read: true, write: true},
		{path: "/execute/bin", read: true},
		{path: "/other/file.txt"},
		{path: "/"},
	}

	for _, c := range cases {
		err := checkRead(sb, c.path)
		if c.read != (err == nil) {
			t.Errorf("checkRead(%q) = %v; want allowed: %t", c.path, err, c.read)
		}
		err = checkWrite(sb, c.path)
		if c.write != (err == nil) {
			t.Errorf("checkWrite(%q) = %v; want allowed: %t", c.path, err, c.write)
		}
	}

	// No sandbox means unrestricted access.
	err := checkRead(nil, "/other/file.txt")
	if err != nil {
		t.Errorf("checkRead(nil) = %v; want nil", err)
	}
	err = checkWrite(nil, "/other/file.txt")
	if err != nil {
		t.Errorf("checkWrite(nil) = %v; want nil", err)
	}
}

func TestSandboxFileTools(t *testing.T) {
	allowed := t.TempDir()
	denied := t.TempDir()

	for _, dir := range []string{allowed, denied} {
		err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	sb := &sandbox.Sandbox{
		FSP: &sandbox.FSPolicy{
			Write: []string{allowed},
		},
	}

	call := func(fn ToolFunc, v any) (string, error) {
		t.Helper()
		buf, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return fn(context.Background(), sb, buf)
	}

	out, err := call(readFile, readFileArgs{Path: filepath.Join(allowed, "file.txt")})
	if err != nil {
		t.Errorf("read_file(allowed) failed with %s", err)
	} else if !strings.Contains(out, "hello") {
		t.Errorf("read_file(allowed) = %q; want it to contain %q", out, "hello")
	}
	if _, err := call(readFile, readFileArgs{Path: filepath.Join(denied, "file.txt")}); err == nil {
		t.Errorf("read_file(denied) did not fail")
	}

	_, err = call(writeFile,
		writeFileArgs{Path: filepath.Join(allowed, "new.txt"), Content: "new\n"})
	if err != nil {
		t.Errorf("write_file(allowed) failed with %s", err)
	}
	_, err = call(writeFile,
		writeFileArgs{Path: filepath.Join(denied, "new.txt"), Content: "new\n"})
	if err == nil {
		t.Errorf("write_file(denied) did not fail")
	}

	_, err = call(editFile, editFileArgs{Path: filepath.Join(allowed, "file.txt"),
		OldString: "hello", NewString: "goodbye"})
	if err != nil {
		t.Errorf("edit_file(allowed) failed with %s", err)
	}
	_, err = call(editFile, editFileArgs{Path: filepath.Join(denied, "file.txt"),
		OldString: "hello", NewString: "goodbye"})
	if err == nil {
		t.Errorf("edit_file(denied) did not fail")
	}

	out, err = call(glob, globArgs{Pattern: "*.txt", Path: denied})
	if err != nil {
		t.Errorf("glob(denied) failed with %s", err)
	} else if out != "No files found." {
		t.Errorf("glob(denied) = %q; want no files", out)
	}

	out, err = call(grep, grepArgs{Pattern: "hello", Path: denied})
	if err != nil {
		t.Errorf("grep(denied dir) failed with %s", err)
	} else if out != "No matches found." {
		t.Errorf("grep(denied dir) = %q; want no matches", out)
	}
	if _, err := call(grep,
		grepArgs{Pattern: "hello", Path: filepath.Join(denied, "file.txt")}); err == nil {

		t.Errorf("grep(denied file) did not fail")
	}
}
