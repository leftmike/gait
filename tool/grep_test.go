package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runGrep(t *testing.T, args grepArgs) string {
	t.Helper()
	buf, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	out, err := grep(context.Background(), testSandbox(), buf)
	if err != nil {
		t.Fatalf("grep error: %s", err)
	}
	return out
}

func TestGrep(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, content string) {
		p := filepath.Join(dir, name)
		err := os.MkdirAll(filepath.Dir(p), 0755)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(p, []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("a.go", "package a\nfunc Foo() {}\nfunc Bar() {}\n")
	mustWrite("b.txt", "foo bar\nFOO BAR\n")
	mustWrite("sub/c.go", "func Foo() {}\n")
	mustWrite("bin", "func Foo\x00() {}")

	// files_with_matches (default), filtered to .go files.
	out := runGrep(t, grepArgs{Pattern: "func Foo", Path: dir, Glob: "*.go"})
	if !strings.Contains(out, "a.go") || !strings.Contains(out, filepath.Join("sub", "c.go")) {
		t.Fatalf("files_with_matches missing expected files: %q", out)
	}
	if strings.Contains(out, "b.txt") {
		t.Fatalf("glob filter leaked non-.go file: %q", out)
	}
	if strings.Contains(out, "bin") {
		t.Fatalf("binary file should be skipped: %q", out)
	}

	// count mode.
	out = runGrep(t, grepArgs{Pattern: "func", Path: dir, Glob: "*.go", OutputMode: "count"})
	if !strings.Contains(out, "a.go:2") {
		t.Fatalf("count mode wrong: %q", out)
	}

	// content mode with line numbers.
	out = runGrep(t, grepArgs{
		Pattern: "Bar", Path: dir, Glob: "*.go", OutputMode: "content", LineNumbers: true,
	})
	if !strings.Contains(out, ":3:func Bar() {}") {
		t.Fatalf("content mode wrong: %q", out)
	}

	// case-insensitive.
	out = runGrep(t, grepArgs{Pattern: "foo", Path: dir, Glob: "*.txt", IgnoreCase: true,
		OutputMode: "count"})
	if !strings.Contains(out, "b.txt:2") {
		t.Fatalf("case-insensitive count wrong: %q", out)
	}

	// no matches.
	out = runGrep(t, grepArgs{Pattern: "zzz-nope", Path: dir})
	if out != "No matches found." {
		t.Fatalf("expected no matches, got: %q", out)
	}
}

func TestGrepMultiline(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "m.txt"), []byte("start\nmiddle\nend\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	out := runGrep(t, grepArgs{
		Pattern: "start.*end", Path: dir, Multiline: true, OutputMode: "files_with_matches",
	})
	if !strings.Contains(out, "m.txt") {
		t.Fatalf("multiline match failed: %q", out)
	}
	// Without multiline, the pattern should not match across lines.
	out = runGrep(t, grepArgs{Pattern: "start.*end", Path: dir})
	if out != "No matches found." {
		t.Fatalf("expected no single-line match, got: %q", out)
	}
}
