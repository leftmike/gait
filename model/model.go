package model

import (
	"context"
	"encoding/json"
	"fmt"
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

func (st StepType) String() string {
	switch st {
	case PromptStep:
		return "PromptStep"
	case ModelResponseStep:
		return "ModelResponseStep"
	case ReasoningStep:
		return "ReasoningStep"
	case ToolCallStep:
		return "ToolCallStep"
	case ToolOutputStep:
		return "ToolOutputStep"
	default:
		return fmt.Sprintf("StepType(%d)", st)
	}
}

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
