package model

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/llmreg"
)

type Client interface {
	Provider() string
	ProviderName() string
	ListModels() map[string]ModelMetadata
	EffortLevels() []string // XXX: add to ListModels?
	NewModel(mdlCfg config.ModelConfig, tools map[string]Tool) (Model, error)
	NewState() State
	Generate(ctx context.Context, mdl Model, st State, opts *Options) error
}

type ModelMetadata struct {
	Model        string
	Name         string
	Reasoning    bool
	ContextLimit int
	OutputLimit  int
	InputCost    float64
	OutputCost   float64
}

type Model interface {
	SetModelConfig(mdlCfg config.ModelConfig) error
	SetTools(tools map[string]Tool) error
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
	case "google":
		return newGoogleClient(clntCfg.APIKey)
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

func listModels(pvdr llmreg.Provider) map[string]ModelMetadata {
	mmdm := map[string]ModelMetadata{}

	for id, mdl := range pvdr.Models {
		if !mdl.ToolCall || !slices.Contains(mdl.Modalities.Input, "text") ||
			!slices.Contains(mdl.Modalities.Output, "text") {

			continue
		}

		mmdm[id] = ModelMetadata{
			Model:        mdl.ID,
			Name:         mdl.Name,
			Reasoning:    mdl.Reasoning,
			ContextLimit: mdl.Limit.Context,
			OutputLimit:  mdl.Limit.Output,
			InputCost:    mdl.Cost.Input,
			OutputCost:   mdl.Cost.Output,
		}
	}

	return mmdm
}
