package model

import (
	"context"
	"encoding/json"
	"time"
)

type Model interface {
	NewState() State
	Generate(ctx context.Context, st State, tools Tools, opts *Options) error
}

type Options struct {
	Verbose bool
	Trace   bool
	Summary bool
}

type ModelInfo struct {
	Name        string
	DisplayName string    // Optional
	Created     time.Time // Optional
}

type StepType int

const (
	PromptStep StepType = iota
	ModelResponseStep
	ReasoningStep
	ToolCallStep
	ToolOutputStep
)

type Step struct {
	Type    StepType
	Content string          // Prompt, ModelResponse, Reasoning, and ToolOutput
	Name    string          // ToolCall and ToolOutput
	Input   json.RawMessage // ToolCall
}

type State interface {
	SystemPrompt(s string)
	Prompt(s string)
	Len() int
	Step(n int) Step
}
