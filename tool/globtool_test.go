package tool

import (
	"testing"
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
