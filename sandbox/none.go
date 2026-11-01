package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/hashicorp/hcl/v2"
)

func init() {
	Register(noneBackend{})
}

type noneBackend struct{}

func (noneBackend) Name() string {
	return "none"
}

func (noneBackend) Available() error {
	return nil
}

func (noneBackend) New(ctx context.Context, p Policy, cfg hcl.Body) (Sandbox, error) {
	hfs, err := UncheckedFS(p.Workdir)
	if err != nil {
		return nil, err
	}
	return &none{hfs: hfs}, nil
}

type none struct {
	hfs *HostFS
}

func (sb *none) Run(ctx context.Context, cmd Cmd) (Result, error) {
	if len(cmd.Args) == 0 {
		return Result{}, errors.New("sandbox: empty command")
	}

	dir, err := sb.hfs.Dir(cmd.Dir)
	if err != nil {
		return Result{}, err
	}

	c := exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
	c.Dir = dir
	if len(cmd.Env) > 0 {
		c.Env = os.Environ()
		for k, v := range cmd.Env {
			c.Env = append(c.Env, k+"="+v)
		}
	}
	return runCombined(c)
}

func (sb *none) FS() FS {
	return sb.hfs
}

func (sb *none) Close(ctx context.Context) error {
	return nil
}

func runCombined(c *exec.Cmd) (Result, error) {
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out
	killGroupOnCancel(c)
	c.WaitDelay = time.Second

	err := c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return Result{Output: out.Bytes(), ExitCode: exitErr.ExitCode()}, nil
	} else if errors.Is(err, exec.ErrWaitDelay) {
		return Result{Output: out.Bytes(), ExitCode: c.ProcessState.ExitCode()}, nil
	} else if err != nil {
		return Result{Output: out.Bytes()}, err
	}
	return Result{Output: out.Bytes()}, nil
}
