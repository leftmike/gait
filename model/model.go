package model

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leftmike/gait/config"
)

type Client interface {
	// XXX: ListModels
	EffortLevels() []string // XXX: move to Model?
	NewModel(mdlCfg config.ModelConfig, tools map[string]Tool) (Model, error)
	NewState() State
	Generate(ctx context.Context, mdl Model, st State, opts *Options) error
}

type Model interface {
	// XXX: SetTools(tools map[string]Tool)
	// XXX: Set*
}

type Options struct {
	Verbose bool
	Trace   bool
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

func NewClient(clntCfg config.ClientConfig) (Client, error) {
	switch clntCfg.Provider {
	case "anthropic":
		return newAnthropicClient(clntCfg.APIKey)
	case "gemini":
		return newGeminiClient(clntCfg.APIKey)
	case "llamacpp":
		return newLlamaCppClient(clntCfg)
	case "ollama":
		return newOllamaClient(clntCfg)
	case "openai":
		return newOpenAIClient(clntCfg.APIKey)
	default:
		return nil, fmt.Errorf("unknown provider: %s", clntCfg.Provider)
	}
}
