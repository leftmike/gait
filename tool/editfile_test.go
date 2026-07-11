package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEditFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")

	callJSON(t, writeFile, writeFileArgs{Path: path, Content: "hello world\n"})
	callJSON(t, editFile, editFileArgs{Path: path, OldString: "world", NewString: "gait"})
	data, _ := os.ReadFile(path)
	if string(data) != "hello gait\n" {
		t.Fatalf("edit failed: %q", string(data))
	}

	// old_string not found.
	callJSONErr(t, editFile, editFileArgs{Path: path, OldString: "nope", NewString: "x"})
	// identical strings.
	callJSONErr(t, editFile, editFileArgs{Path: path, OldString: "a", NewString: "a"})

	// non-unique without replace_all.
	callJSON(t, writeFile, writeFileArgs{Path: path, Content: "a a a\n"})
	callJSONErr(t, editFile, editFileArgs{Path: path, OldString: "a", NewString: "b"})
	// replace_all succeeds.
	callJSON(t, editFile,
		editFileArgs{Path: path, OldString: "a", NewString: "b", ReplaceAll: true})
	data, _ = os.ReadFile(path)
	if string(data) != "b b b\n" {
		t.Fatalf("replace_all failed: %q", string(data))
	}
}

func TestEditBinaryFileRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte{'a', 0, 'b'}, 0644); err != nil {
		t.Fatal(err)
	}
	callJSONErr(t, editFile, editFileArgs{Path: path, OldString: "a", NewString: "x"})
}
