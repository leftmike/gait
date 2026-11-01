package sandbox

import (
	"context"
	"errors"

	"github.com/hashicorp/hcl/v2"
)

var ErrNoCommands = errors.New("sandbox: commands are not allowed")

func init() {
	Register(toolsBackend{})
}

type toolsBackend struct{}

func (toolsBackend) Name() string {
	return "tools"
}

func (toolsBackend) Available() error {
	return nil
}

func (toolsBackend) New(ctx context.Context, p Policy, cfg hcl.Body) (Sandbox, error) {
	if len(p.Env) > 0 {
		return nil, errors.New("sandbox: tools: env requires a backend that runs commands")
	}
	hfs, err := NewHostFS(p)
	if err != nil {
		return nil, err
	}
	return &tools{hfs: hfs}, nil
}

type tools struct {
	hfs *HostFS
}

func (sb *tools) Run(ctx context.Context, cmd Cmd) (Result, error) {
	return Result{}, ErrNoCommands
}

func (sb *tools) FS() FS {
	return sb.hfs
}

func (sb *tools) Close(ctx context.Context) error {
	return nil
}

func (sb *tools) noCommands() {}

// CanRun reports whether sb can run commands.
func CanRun(sb Sandbox) bool {
	_, ok := sb.(interface{ noCommands() })
	return !ok
}
