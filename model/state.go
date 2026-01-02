package model

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
	Content string
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

func (st *State) Prompt(s string) {
	st.appendStep(PromptStep, s)
}
