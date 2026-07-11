package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "hello.txt")

	callJSON(t, writeFile, writeFileArgs{Path: path, Content: "hello world\n"})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}
