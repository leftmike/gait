package model

import (
	"context"
	"time"
)

type Model interface {
	Generate(ctx context.Context, st *State, tools Tools, opts *Options) error
}

type Options struct {
	Verbose bool
	Trace   bool
	Summary bool
}

type ModelInfo struct {
	Name    string
	Created time.Time
}
