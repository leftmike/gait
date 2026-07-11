package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func callJSON(t *testing.T, fn func(context.Context, []byte) (string, error), v any) string {
	t.Helper()
	buf, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	out, err := fn(context.Background(), buf)
	if err != nil {
		t.Fatalf("tool returned error: %s", err)
	}
	return out
}

func callJSONErr(t *testing.T, fn func(context.Context, []byte) (string, error), v any) {
	t.Helper()
	buf, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fn(context.Background(), buf); err == nil {
		t.Fatalf("expected error, got none")
	}
}

func TestWriteAndEditFile(t *testing.T) {
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

	callJSON(t, editFile, editFileArgs{Path: path, OldString: "world", NewString: "gait"})
	data, _ = os.ReadFile(path)
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

// TestAddToolsNoPanic ensures every file tool's schema builds (MustToolSchema
// panics on a bad gait tag, e.g. a comma in a description).
func TestAddToolsNoPanic(t *testing.T) {
	var ag Agent
	ag.AddReadFileTool()
	ag.AddWriteFileTool()
	ag.AddEditFileTool()
	ag.AddGlobTool()
	ag.AddGrepTool()
	for _, name := range []string{"read_file", "write_file", "edit_file", "glob", "grep"} {
		if _, ok := ag.Tools[name]; !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}
