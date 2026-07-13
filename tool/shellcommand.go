// The shell_command tool runs a shell command, using the same arguments as the
// shell tool in OpenAI Codex (see codex-rs/core/src/openai_tools.rs): the
// command is an argv array, executed directly rather than through a shell, with
// an optional working directory and timeout.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/leftmike/sandbox"
)

type shellCommandArgs struct {
	Command   []string `json:"command" gait:"the command to execute as an argv array (e.g. [\"ls\" \"-l\"])"`
	Workdir   string   `json:"workdir,omitempty" gait:"the working directory to execute the command in"`
	TimeoutMS int      `json:"timeout_ms,omitempty" gait:"the timeout for the command in milliseconds"`
}

func shellCommand(ctx context.Context, sb *sandbox.Sandbox, buf []byte) (string, error) {
	var args shellCommandArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}
	if len(args.Command) == 0 {
		return "", fmt.Errorf("command must not be empty")
	}

	if args.TimeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(args.TimeoutMS)*time.Millisecond)
		defer cancel()
	}

	out, err := combinedOutput(ctx, sb, args.Workdir, args.Command[0], args.Command[1:]...)

	// A non-zero exit is a normal outcome the model should see, so report the
	// output along with the exit code rather than failing the tool call. A
	// failure to start the command (e.g. a missing binary) is a real error.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Sprintf("%sexited with code %d", out, exitErr.ExitCode()), nil
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}

var ShellCommand = Tool{
	Name: "shell_command",
	Description: "Runs a shell command and returns its combined stdout and stderr. The command " +
		"is given as an argv array and is executed directly, not through a shell, so " +
		"redirection and pipes are not interpreted; run an explicit shell (for example " +
		"[\"bash\", \"-lc\", \"...\"]) if you need them.",
	Func:   shellCommand,
	Schema: MustToolSchema[shellCommandArgs](),
}
