package tool

import (
	"strings"
	"testing"
)

func TestShellCommand(t *testing.T) {
	out := callJSON(t, shellCommand, shellCommandArgs{
		Command: []string{"echo", "hello"},
	})
	if out != "hello\n" {
		t.Errorf("got %q, want %q", out, "hello\n")
	}
}

func TestShellCommandWorkdir(t *testing.T) {
	dir := t.TempDir()
	out := callJSON(t, shellCommand, shellCommandArgs{
		Command: []string{"pwd"},
		Workdir: dir,
	})
	if strings.TrimSpace(out) != dir {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), dir)
	}
}

func TestShellCommandExitCode(t *testing.T) {
	out := callJSON(t, shellCommand, shellCommandArgs{
		Command: []string{"sh", "-c", "exit 3"},
	})
	if !strings.Contains(out, "exited with code 3") {
		t.Errorf("got %q, want it to contain %q", out, "exited with code 3")
	}
}

func TestShellCommandEmpty(t *testing.T) {
	callJSONErr(t, shellCommand, shellCommandArgs{})
}

func TestShellCommandMissingBinary(t *testing.T) {
	callJSONErr(t, shellCommand, shellCommandArgs{
		Command: []string{"gait-no-such-binary-xyz"},
	})
}
