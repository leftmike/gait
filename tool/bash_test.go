package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBash(t *testing.T) {
	// Combined stdout and stderr are returned.
	out := callJSON(t, bash, bashArgs{Command: "echo out; echo err 1>&2"})
	if out != "out\nerr\n" {
		t.Fatalf("bash = %q, want %q", out, "out\nerr\n")
	}

	// A non-zero exit status is an error.
	callJSONErr(t, bash, bashArgs{Command: "exit 3"})

	// An empty command is rejected.
	callJSONErr(t, bash, bashArgs{Command: "   "})
}

func TestBashTimeout(t *testing.T) {
	_, err := bashRun(t, bashArgs{Command: "sleep 5", Timeout: 100})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func bashRun(t *testing.T, args bashArgs) (string, error) {
	t.Helper()
	buf, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return bash(t.Context(), nil, buf)
}
