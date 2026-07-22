package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/llmreg"
	"github.com/leftmike/gait/system"
	"github.com/leftmike/gait/tool"
	"github.com/leftmike/gait/util"
)

type openRouterClient struct {
	client *openrouter.OpenRouter
	name   string
	models map[string]ModelMetadata
}

type openRouterModel struct {
	clnt            *openRouterClient
	model           string
	includeThoughts bool
	effort          *components.ChatRequestReasoningEffort
	maxTokens       int64
	contextLimit    int
	inputCost       float64
	outputCost      float64
	tools           map[string]tool.Tool
	sandbox         *system.Sandbox
	toolParams      []components.ChatFunctionTool
}

type openRouterStep struct {
	typ       StepType
	content   string
	name      string
	id        string
	input     string
	reasoning []components.ReasoningDetailUnion
}

type openRouterState struct {
	systemPrompt  string
	steps         []openRouterStep
	inputTokens   int64
	outputTokens  int64
	contextTokens int64
	inputCost     float64 // cents
	outputCost    float64 // cents
}

func newOpenRouterClient(apiKey string) (Client, error) {
	pvdr, err := llmreg.FindProvider("openrouter")
	if err != nil {
		return nil, err
	}

	if apiKey == "" {
		apiKey = os.Getenv("OPENROUTER_API_KEY")
	}

	return &openRouterClient{
		client: openrouter.New(
			openrouter.WithSecurity(apiKey),
			openrouter.WithClient(&http.Client{Timeout: 10 * time.Minute}),
		),
		name:   pvdr.Name,
		models: listModels(pvdr),
	}, nil
}

func (clnt *openRouterClient) Provider() string {
	return "openrouter"
}

func (clnt *openRouterClient) ProviderName() string {
	return clnt.name
}

func (clnt *openRouterClient) ListModels() map[string]ModelMetadata {
	return clnt.models
}

func (clnt *openRouterClient) NewModel(mdlCfg config.ModelConfig) (Model, error) {
	mmd, ok := clnt.models[mdlCfg.Model]
	if !ok {
		return nil, fmt.Errorf("unknown model: %s", mdlCfg.Model)
	}

	maxTokens := int64(mmd.OutputLimit) / 4
	if mdlCfg.MaxTokens > 0 {
		maxTokens = int64(mdlCfg.MaxTokens)
	}
	if mmd.OutputLimit > 0 && maxTokens > int64(mmd.OutputLimit) {
		return nil, fmt.Errorf("max tokens %d exceeds output limit %d for model %s",
			maxTokens, mmd.OutputLimit, mdlCfg.Model)
	}

	var effort *components.ChatRequestReasoningEffort
	switch mdlCfg.Effort {
	case "", "default":
		effort = nil
	case "none":
		effort = components.ChatRequestReasoningEffortNone.ToPointer()
	case "minimal":
		effort = components.ChatRequestReasoningEffortMinimal.ToPointer()
	case "low":
		effort = components.ChatRequestReasoningEffortLow.ToPointer()
	case "medium":
		effort = components.ChatRequestReasoningEffortMedium.ToPointer()
	case "high":
		effort = components.ChatRequestReasoningEffortHigh.ToPointer()
	case "xhigh":
		effort = components.ChatRequestReasoningEffortXhigh.ToPointer()
	case "max":
		effort = components.ChatRequestReasoningEffortMax.ToPointer()
	default:
		return nil, fmt.Errorf(
			"effort must be none, minimal, low, medium, high, xhigh, or max: %s", mdlCfg.Effort)
	}

	return &openRouterModel{
		clnt:            clnt,
		model:           mdlCfg.Model,
		includeThoughts: mdlCfg.IncludeThoughts,
		effort:          effort,
		maxTokens:       maxTokens,
		contextLimit:    mmd.ContextLimit,
		inputCost:       mmd.InputCost,
		outputCost:      mmd.OutputCost,
	}, nil
}

func (clnt *openRouterClient) NewState() State {
	return &openRouterState{}
}

func (mdl *openRouterModel) Generate(ctx context.Context, ast State, opts *Options) error {
	st := ast.(*openRouterState)

	for {
		msgs, txtLen := st.toMessages()

		if opts.Trace {
			fmt.Print("Trace: OpenRouter Chat.Send(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", mdl.model, len(mdl.toolParams), txtLen)
			}
			fmt.Print(") -> ")
		}

		model := mdl.model
		req := components.ChatRequest{
			Model:    &model,
			Messages: msgs,
		}
		if len(mdl.toolParams) > 0 {
			req.Tools = mdl.toolParams
		}
		if mdl.maxTokens > 0 {
			req.MaxCompletionTokens = optionalnullable.From(&mdl.maxTokens)
		}
		if mdl.effort != nil {
			req.ReasoningEffort = optionalnullable.From(mdl.effort)
		}
		if mdl.includeThoughts {
			summary := components.ChatReasoningSummaryVerbosityEnumConcise
			if opts.Verbose {
				summary = components.ChatReasoningSummaryVerbosityEnumDetailed
			}
			req.Reasoning = &components.ChatRequestReasoning{
				Summary: optionalnullable.From(&summary),
			}
		}

		res, err := mdl.clnt.client.Chat.Send(ctx, req, nil)
		if opts.Trace {
			fmt.Print(err)
		}
		if err != nil {
			if opts.Trace {
				fmt.Println()
			}
			return err
		}

		rsp := res.ChatResult
		if rsp == nil {
			if opts.Trace {
				fmt.Println()
			}
			return fmt.Errorf("openrouter: no chat result returned")
		}

		if rsp.Usage != nil {
			st.inputTokens += rsp.Usage.PromptTokens
			st.outputTokens += rsp.Usage.CompletionTokens
			st.contextTokens = rsp.Usage.PromptTokens + rsp.Usage.CompletionTokens
			st.inputCost += float64(rsp.Usage.PromptTokens) / 10_000 * mdl.inputCost
			st.outputCost += float64(rsp.Usage.CompletionTokens) / 10_000 * mdl.outputCost
		}
		if opts.Trace {
			if opts.Verbose && rsp.Usage != nil {
				fmt.Printf(" tokens: input: %d output: %d total: %d", rsp.Usage.PromptTokens,
					rsp.Usage.CompletionTokens, rsp.Usage.TotalTokens)
			}
			fmt.Println()
		}

		if len(rsp.Choices) == 0 {
			return fmt.Errorf("openrouter: no choices returned")
		}
		choice := rsp.Choices[0]
		if choice.FinishReason != nil &&
			*choice.FinishReason == components.ChatFinishReasonEnumLength {

			return fmt.Errorf("max tokens reached: output was truncated")
		}
		msg := choice.Message

		var reasoningText string
		if mdl.includeThoughts {
			if reasoning, ok := msg.Reasoning.Get(); ok && reasoning != nil {
				reasoningText = *reasoning
			}
		}
		if reasoningText != "" || len(msg.ReasoningDetails) > 0 {
			st.steps = append(st.steps, openRouterStep{
				typ:       ThinkingStep,
				content:   reasoningText,
				reasoning: msg.ReasoningDetails,
			})
		}

		if content, ok := msg.Content.Get(); ok && content != nil && content.Str != nil &&
			*content.Str != "" {

			st.steps = append(st.steps, openRouterStep{
				typ:     ModelResponseStep,
				content: *content.Str,
			})
		}

		var toolCalls []components.ChatToolCall
		for _, tc := range msg.ToolCalls {
			st.steps = append(st.steps, openRouterStep{
				typ:   ToolCallStep,
				name:  tc.Function.Name,
				id:    tc.ID,
				input: tc.Function.Arguments,
			})
			toolCalls = append(toolCalls, tc)
		}

		if len(toolCalls) == 0 {
			break
		}

		for _, tc := range toolCalls {
			if opts.Trace {
				fmt.Printf("Trace: calling %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
			}

			out, err := tool.CallTool(ctx, mdl.tools, tc.Function.Name, mdl.sandbox,
				[]byte(tc.Function.Arguments))
			if opts.Trace {
				fmt.Printf("Trace: results from %s() -> (%s, ", tc.Function.Name,
					util.Lines(out, 1, 160))
				fmt.Print(err)
				fmt.Println(")")
			}
			if err != nil {
				out = fmt.Sprintf("error: %s", err)
			}
			st.steps = append(st.steps, openRouterStep{
				typ:     ToolOutputStep,
				name:    tc.Function.Name,
				id:      tc.ID,
				content: out,
			})
		}
	}

	return nil
}

func toOpenRouterTools(tools map[string]tool.Tool) []components.ChatFunctionTool {
	var toolParams []components.ChatFunctionTool
	for _, tl := range tool.Sorted(tools) {
		description := tl.Description
		toolParams = append(toolParams,
			components.CreateChatFunctionToolChatFunctionToolFunction(
				components.ChatFunctionToolFunction{
					Type: components.ChatFunctionToolTypeFunction,
					Function: components.ChatFunctionToolFunctionFunction{
						Name:        tl.Name,
						Description: &description,
						Parameters:  tl.Schema,
					},
				}))
	}

	return toolParams
}

func (mdl *openRouterModel) SetTools(tools map[string]tool.Tool, sb *system.Sandbox) error {
	mdl.tools = tools
	mdl.sandbox = sb
	mdl.toolParams = toOpenRouterTools(tools)
	return nil
}

func (mdl *openRouterModel) ContextLimit() int {
	return mdl.contextLimit
}

func (st *openRouterState) SystemPrompt(s string) {
	st.systemPrompt = s
}

func (st *openRouterState) Prompt(s string) {
	st.steps = append(st.steps, openRouterStep{
		typ:     PromptStep,
		content: s,
	})
}

func (st *openRouterState) Len() int {
	return len(st.steps)
}

func (st *openRouterState) Step(n int) Step {
	step := st.steps[n]
	return Step{
		Type:    step.typ,
		Content: step.content,
		Name:    step.name,
		Input:   json.RawMessage(step.input),
	}
}

func (st *openRouterState) Clear() {
	st.steps = st.steps[:0]
}

func (st *openRouterState) Usage() (int64, int64, int64) {
	return st.inputTokens, st.outputTokens, st.contextTokens
}

func (st *openRouterState) Cost() (float64, float64) {
	return st.inputCost / 100, st.outputCost / 100
}

func openRouterUserMessage(s string) components.ChatMessages {
	return components.CreateChatMessagesUser(components.ChatUserMessage{
		Role:    components.ChatUserMessageRoleUser,
		Content: components.CreateChatUserMessageContentStr(s),
	})
}

func (st *openRouterState) toMessages() ([]components.ChatMessages, int) {
	var msgs []components.ChatMessages
	var txtLen int

	if st.systemPrompt != "" {
		msgs = append(msgs, components.CreateChatMessagesSystem(components.ChatSystemMessage{
			Role:    components.ChatSystemMessageRoleSystem,
			Content: components.CreateChatSystemMessageContentStr(st.systemPrompt),
		}))
		txtLen += len(st.systemPrompt)
	}

	var reasoning []components.ReasoningDetailUnion
	for _, step := range st.steps {
		switch step.typ {
		case PromptStep:
			msgs = append(msgs, openRouterUserMessage(step.content))
			txtLen += len(step.content)
			reasoning = nil

		case ModelResponseStep:
			content := components.CreateChatAssistantMessageContentStr(step.content)
			asst := components.ChatAssistantMessage{
				Role:    components.ChatAssistantMessageRoleAssistant,
				Content: optionalnullable.From(&content),
			}
			if len(reasoning) > 0 {
				asst.ReasoningDetails = reasoning
			}
			msgs = append(msgs, components.CreateChatMessagesAssistant(asst))
			txtLen += len(step.content)
			reasoning = nil

		case ThinkingStep:
			reasoning = step.reasoning

		case ToolCallStep:
			tc := components.ChatToolCall{
				ID:   step.id,
				Type: components.ChatToolCallTypeFunction,
				Function: components.ChatToolCallFunction{
					Name:      step.name,
					Arguments: step.input,
				},
			}
			txtLen += len(step.input)

			if len(msgs) > 0 && msgs[len(msgs)-1].ChatAssistantMessage != nil {
				asst := msgs[len(msgs)-1].ChatAssistantMessage
				asst.ToolCalls = append(asst.ToolCalls, tc)
			} else {
				asst := components.ChatAssistantMessage{
					Role:      components.ChatAssistantMessageRoleAssistant,
					ToolCalls: []components.ChatToolCall{tc},
				}
				if len(reasoning) > 0 {
					asst.ReasoningDetails = reasoning
				}
				msgs = append(msgs, components.CreateChatMessagesAssistant(asst))
			}
			reasoning = nil

		case ToolOutputStep:
			msgs = append(msgs, components.CreateChatMessagesTool(components.ChatToolMessage{
				Role:       components.ChatToolMessageRoleTool,
				ToolCallID: step.id,
				Content:    components.CreateChatToolMessageContentStr(step.content),
			}))
			txtLen += len(step.content)
			reasoning = nil

		default:
			panic(fmt.Sprintf("unexpected step type: %s", step.typ))
		}
	}

	return msgs, txtLen
}
