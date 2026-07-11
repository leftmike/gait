package tool

import (
	"strings"
	"testing"
)

func TestShell(t *testing.T) {
	out := callJSON(t, shell, shellArgs{Command: []string{"echo", "hello world"}})
	if out != "hello world\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestShellWorkdir(t *testing.T) {
	dir := t.TempDir()
	out := callJSON(t, shell, shellArgs{Command: []string{"pwd"}, Workdir: dir})
	if strings.TrimSpace(out) != dir {
		t.Fatalf("unexpected output: %q want %q", out, dir)
	}
}

func TestShellExitStatus(t *testing.T) {
	out := callJSON(t, shell,
		shellArgs{Command: []string{"sh", "-c", "echo oops >&2; exit 3"}})
	if !strings.Contains(out, "oops") || !strings.Contains(out, "exit status 3") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestShellTimeout(t *testing.T) {
	out := callJSON(t, shell,
		shellArgs{Command: []string{"sleep", "10"}, TimeoutMS: 50})
	if !strings.Contains(out, "timed out") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestShellEmptyCommand(t *testing.T) {
	callJSONErr(t, shell, shellArgs{})
}
