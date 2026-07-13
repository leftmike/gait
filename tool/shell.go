// The shell tool, compatible with the shell tool in OpenAI Codex. It runs a
// command supplied as an argv array (e.g. ["bash", "-lc", "ls"]) and returns
// its combined stdout and stderr.
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

type shellArgs struct {
	Command   []string `json:"command" gait:"the command to run as an argv array; for example [\"bash\" \"-lc\" \"ls\"]"`
	Workdir   string   `json:"workdir,omitempty" gait:"the working directory to run the command in; defaults to the current directory"`
	TimeoutMS int      `json:"timeout_ms,omitempty" gait:"the maximum time to wait in milliseconds; defaults to no timeout"`
}

func shell(ctx context.Context, sb *sandbox.Sandbox, buf []byte) (string, error) {
	var args shellArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}
	if len(args.Command) == 0 {
		return "", fmt.Errorf("command must not be empty")
	}

	if args.TimeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx,
			time.Duration(args.TimeoutMS)*time.Millisecond)
		defer cancel()
	}

	out, err := combinedOutput(ctx, sb, args.Workdir, args.Command[0], args.Command[1:]...)

	// A command that runs but exits non-zero is not a tool failure: the model
	// wants to see the output and the exit status, so report them in the result
	// rather than returning an error.
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("%scommand timed out after %d ms", out, args.TimeoutMS), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Sprintf("%sexit status %d", out, exitErr.ExitCode()), nil
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}

var Shell = Tool{
	Name: "shell",
	Description: "Runs a shell command and returns its combined stdout and stderr. " +
		"The command is given as an argv array such as [\"bash\", \"-lc\", \"ls -l\"]; " +
		"it is executed directly rather than through a shell unless you invoke one " +
		"explicitly. Use workdir to set the working directory and timeout_ms to bound " +
		"how long the command may run.",
	Func:   shell,
	Schema: MustToolSchema[shellArgs](),
}
