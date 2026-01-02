package model

import (
	"context"
)

type Model interface {
	Generate(ctx context.Context, st *State, tools Tools, opts *Options) error
}

type Options struct {
	Verbose bool
	Summary bool
}
