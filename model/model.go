package model

import (
	"context"
	"encoding/json"
)

type Model interface {
	Generate(ctx context.Context, st *State, tools Tools, opts *Options) error
}

type Tool struct {
	Description string
	Name        string
	Parameters  map[string]any // XXX: generate based on the Function
	Function    func(args json.RawMessage) string
}

type Tools map[string]Tool

type Options struct {
	Verbose bool
	Summary bool
}
