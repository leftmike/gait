package model

import "encoding/json"

type StepType int

const (
	PromptStep StepType = iota
	ModelResponseStep
	ReasoningStep
	ToolCallStep
	ToolOutputStep
)

type Step interface {
	Type() StepType
	Content() string        // Prompt, ModelResponse, Reasoning, and ToolOutput
	Name() string           // ToolCall and ToolOutput
	Input() json.RawMessage // ToolCall
}

/*
type Step struct {
	Type    StepType
	Content string          // Prompt, ModelResponse, Reasoning, and ToolOutput
	Name    string          // ToolCall and ToolOutput
	ID      string          // ToolCall and ToolOutput: optional, depending upon the provider
	Input   json.RawMessage // ToolCall, required
	Args    map[string]any  // ToolCall: optional, depending upon the provider
	IsError bool            // ToolOutput
}
*/

type State struct {
	SystemPrompt string
	Steps        []Step
}

func (st *State) appendStep(step Step) {
	st.Steps = append(st.Steps, step)
}

/*
func (st *State) appendToolCall(name, id string, buf json.RawMessage, args map[string]any) {
	st.Steps = append(st.Steps,
		Step{
			Type:  ToolCallStep,
			Name:  name,
			ID:    id,
			Input: buf,
			Args:  args,
		})
}

func (st *State) appendToolOutput(isError bool, name, id, s string) {
	st.Steps = append(st.Steps,
		Step{
			Type:    ToolOutputStep,
			Name:    name,
			ID:      id,
			Content: s,
			IsError: isError,
		})
}
*/
