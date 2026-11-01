package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/leftmike/gait/sandbox"
)

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
		match   bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "main.js", false},
		{"*.go", "src/main.go", false},
		{"**/*.go", "main.go", true},
		{"**/*.go", "src/main.go", true},
		{"**/*.go", "a/b/c/main.go", true},
		{"**/*.go", "main.js", false},
		{"src/**/*.ts", "src/main.ts", true},
		{"src/**/*.ts", "src/a/b/main.ts", true},
		{"src/**/*.ts", "lib/main.ts", false},
		{"src/*.ts", "src/main.ts", true},
		{"src/*.ts", "src/a/main.ts", false},
		{"**", "a/b/c", true},
		{"**", "a", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/x/y/c", false},
		{"file-?.txt", "file-1.txt", true},
		{"file-?.txt", "file-12.txt", false},
	}

	for _, c := range cases {
		if got := globMatch(c.pattern, c.name); got != c.match {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pattern, c.name, got, c.match)
		}
	}
}

func TestGlob(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "sub/b.go", "c.txt"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	sb, err := sandbox.Open(ctx, "none", sandbox.Policy{Workdir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range []globArgs{
		{Pattern: "**/*.go"},
		{Pattern: "**/*.go", Path: "."},
		{Pattern: "**/*.go", Path: dir},
	} {
		buf, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		out, err := glob(ctx, sb, buf)
		if err != nil {
			t.Fatal(err)
		}

		got := strings.Split(out, "\n")
		sort.Strings(got)
		root := args.Path
		if root == "" {
			root = "."
		}
		want := []string{filepath.Join(root, "a.go"), filepath.Join(root, "sub", "b.go")}
		if !slices.Equal(got, want) {
			t.Errorf("glob(%+v) = %q, want %q", args, got, want)
		}
	}
}
