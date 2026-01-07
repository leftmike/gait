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

type Step struct {
	Type    StepType
	Content string          // Prompt, ModelResponse, Reasoning, and ToolOutput
	Name    string          // ToolCall and ToolOutput
	ID      string          // ToolCall and ToolOutput: optional, depending upon the provider
	Input   json.RawMessage // ToolCall
	IsError bool            // ToolOutput
}

type State struct {
	SystemPrompt string
	Steps        []Step
}

func (st *State) appendStep(typ StepType, s string) {
	st.Steps = append(st.Steps,
		Step{
			Type:    typ,
			Content: s,
		})
}

func (st *State) appendToolCall(name, id string, buf json.RawMessage) {
	st.Steps = append(st.Steps,
		Step{
			Type:  ToolCallStep,
			Name:  name,
			ID:    id,
			Input: buf,
		})
}

func (st *State) appendToolOutput(isError bool, id, s string) {
	st.Steps = append(st.Steps,
		Step{
			Type:    ToolOutputStep,
			ID:      id,
			Content: s,
			IsError: isError,
		})
}

func (st *State) Prompt(s string) {
	st.appendStep(PromptStep, s)
}
