package model

import (
	"context"
	"encoding/json"
)

type Options struct {
	Verbose bool
	Summary bool
}

type Model interface {
	Generate(ctx context.Context, s string, tools Tools, opts *Options) (string, error)
}

type Tool struct {
	Description string
	Name        string
	Parameters  map[string]any
	Function    func(args json.RawMessage) string
}

type Tools map[string]Tool
