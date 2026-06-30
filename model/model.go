package model

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leftmike/gait/config"
)

type Model interface {
	EffortLevels() []string
	NewState() State
	Generate(ctx context.Context, st State, tools map[string]Tool, opts *config.Options) error
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
	ThinkingStep
	ToolCallStep
	ToolOutputStep
)

func (st StepType) String() string {
	switch st {
	case PromptStep:
		return "PromptStep"
	case ModelResponseStep:
		return "ModelResponseStep"
	case ThinkingStep:
		return "ThinkingStep"
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
	Content string          // Prompt, ModelResponse, Thinking, and ToolOutput
	Name    string          // ToolCall and ToolOutput
	Input   json.RawMessage // ToolCall
}

type State interface {
	SystemPrompt(s string)
	Prompt(s string)
	Len() int
	Step(n int) Step
	Clear()
	Usage() (int64, int64, int64) // input tokens, output tokens, contextTokens
}
