package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/leftmike/gait/system"
)

const (
	// bashDefaultTimeout is used when no timeout is given, matching Claude
	// Code's bash tool default of 2 minutes.
	bashDefaultTimeout = 120 * time.Second

	// bashMaxTimeout caps the requested timeout, matching Claude Code's limit
	// of 10 minutes.
	bashMaxTimeout = 600 * time.Second

	// bashMaxOutput is the maximum number of bytes of combined output
	// returned; longer output is truncated.
	bashMaxOutput = 1024 * 30
)

type bashArgs struct {
	Command string `json:"command" gait:"the bash command to execute"`
	Timeout int    `json:"timeout,omitempty" gait:"timeout in milliseconds; defaults to 120000 and is capped at 600000"`
}

func bash(ctx context.Context, sb *system.Sandbox, buf []byte) (string, error) {
	var args bashArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Command) == "" {
		return "", fmt.Errorf("command is required")
	}

	timeout := bashDefaultTimeout
	if args.Timeout > 0 {
		timeout = time.Duration(args.Timeout) * time.Millisecond
	}
	if timeout > bashMaxTimeout {
		timeout = bashMaxTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := sb.CombinedOutput(ctx, "", "bash", "-c", args.Command)

	// Report a timeout explicitly rather than as an opaque signal error.
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %s", timeout)
	}

	result := string(out)
	if len(result) > bashMaxOutput {
		result = result[:bashMaxOutput] + "\n... (output truncated)"
	}

	// A non-zero exit status is reported alongside any output the command
	// produced so the model can see both.
	if err != nil {
		if result != "" {
			return "", fmt.Errorf("%s\n%w", result, err)
		}
		return "", err
	}

	return result, nil
}

var Bash = Tool{
	Name: "bash",
	Description: "Executes a bash command and returns its combined stdout and stderr. " +
		"Commands run with a default timeout of 120000 milliseconds (2 minutes), " +
		"which can be raised up to 600000 milliseconds (10 minutes). Output " +
		"longer than 30KB is truncated.",
	Func:   bash,
	Schema: MustToolSchema[bashArgs](),
}
