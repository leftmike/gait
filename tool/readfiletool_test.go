package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Whole file in cat -n format, line numbers starting at 1.
	out := callJSON(t, readFile, readFileArgs{Path: path})
	want := "     1\talpha\n     2\tbeta\n     3\tgamma"
	if out != want {
		t.Fatalf("read_file = %q, want %q", out, want)
	}

	// offset and limit select a range; line numbers reflect real positions.
	out = callJSON(t, readFile, readFileArgs{Path: path, Offset: 2, Limit: 1})
	if out != "     2\tbeta" {
		t.Fatalf("offset/limit = %q", out)
	}

	// Missing file is an error.
	callJSONErr(t, readFile, readFileArgs{Path: filepath.Join(t.TempDir(), "nope")})
}

func TestReadFileNoTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("one\ntwo"), 0644); err != nil {
		t.Fatal(err)
	}

	out := callJSON(t, readFile, readFileArgs{Path: path})
	if out != "     1\tone\n     2\ttwo" {
		t.Fatalf("read_file = %q", out)
	}
}

func TestReadFileEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}

	if out := callJSON(t, readFile, readFileArgs{Path: path}); out != "" {
		t.Fatalf("empty file = %q, want empty", out)
	}
}

func TestReadFileTruncatesLongLine(t *testing.T) {
	long := strings.Repeat("x", readFileMaxLineLen+50)
	out := formatReadFile(long+"\n", 0, 0)
	got := strings.TrimPrefix(out, "     1\t")
	if len([]rune(got)) != readFileMaxLineLen {
		t.Fatalf("truncated line length = %d, want %d", len([]rune(got)), readFileMaxLineLen)
	}
}
