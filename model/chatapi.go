package model

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	openai_param "github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/util"
)

// Model for OpenAI-compatible Chat Completions API (/v1/chat/completions)
type chatAPIClient struct {
	client   openai.Client
	provider string
}

type chatAPIModel struct {
	clnt       *chatAPIClient
	model      string
	tools      map[string]Tool
	toolParams []openai.ChatCompletionToolUnionParam
}

type chatAPIStep struct {
	typ     StepType
	content string
	name    string
	id      string
	input   string
}

type chatAPIState struct {
	systemPrompt  string
	steps         []chatAPIStep
	inputTokens   int64
	outputTokens  int64
	contextTokens int64
}

func newLlamaCppClient(clntCfg config.ClientConfig) (Client, error) {
	baseURL := clntCfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8080/v1"
	}
	apiKey := clntCfg.APIKey
	if apiKey == "" {
		apiKey = "no-api-key"
	}

	return &chatAPIClient{
		client:   openai.NewClient(option.WithBaseURL(baseURL), option.WithAPIKey(apiKey)),
		provider: clntCfg.Provider,
	}, nil
}

func (clnt *chatAPIClient) Provider() string {
	return clnt.provider
}

func (clnt *chatAPIClient) ProviderName() string {
	return clnt.provider // XXX
}

func (clnt *chatAPIClient) ListModels() map[string]ModelMetadata {
	return map[string]ModelMetadata{} // XXX
}

func (clnt *chatAPIClient) NewModel(mdlCfg config.ModelConfig, tools map[string]Tool) (Model,
	error) {

	// XXX: mdlCfg.IncludeThoughts
	// XXX: mdlCfg.MaxTokens

	if mdlCfg.Effort != "" && mdlCfg.Effort != "default" {
		return nil, fmt.Errorf("%s: effort is not supported: %s", clnt.provider, mdlCfg.Effort)
	}

	mdl := chatAPIModel{
		clnt:  clnt,
		model: mdlCfg.Model,
	}

	err := mdl.SetTools(tools)
	if err != nil {
		return nil, err
	}

	return &mdl, nil
}

func (clnt *chatAPIClient) NewState() State {
	return &chatAPIState{}
}

func (mdl *chatAPIModel) Generate(ctx context.Context, ast State, opts *Options) error {
	st := ast.(*chatAPIState)

	for {
		msgs, txtLen := st.toMessages()

		if opts.Trace {
			fmt.Printf("Trace: %s Chat.Completions.New(", mdl.clnt.provider)
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", mdl.model, len(mdl.tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		params := openai.ChatCompletionNewParams{
			Model:    mdl.model,
			Messages: msgs,
		}
		if len(mdl.toolParams) > 0 {
			params.Tools = mdl.toolParams
		}

		rsp, err := mdl.clnt.client.Chat.Completions.New(ctx, params)
		if opts.Trace {
			fmt.Print(err)
			if opts.Verbose && rsp != nil {
				fmt.Printf(" tokens: input: %d output: %d total: %d",
					rsp.Usage.PromptTokens, rsp.Usage.CompletionTokens, rsp.Usage.TotalTokens)
			}
			fmt.Println()
		}
		if err != nil {
			return err
		}

		st.inputTokens += rsp.Usage.PromptTokens
		st.outputTokens += rsp.Usage.CompletionTokens
		st.contextTokens = rsp.Usage.PromptTokens + rsp.Usage.CompletionTokens

		if len(rsp.Choices) == 0 {
			return fmt.Errorf("%s: no choices returned", mdl.clnt.provider)
		}
		if rsp.Choices[0].FinishReason == "length" {
			return fmt.Errorf("max tokens reached: output was truncated")
		}
		msg := rsp.Choices[0].Message

		if msg.Content != "" {
			st.steps = append(st.steps, chatAPIStep{
				typ:     ModelResponseStep,
				content: msg.Content,
			})
		}

		var toolCalls []openai.ChatCompletionMessageToolCallUnion
		for _, tc := range msg.ToolCalls {
			st.steps = append(st.steps, chatAPIStep{
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

			out, err := callTool(ctx, mdl.tools, tc.Function.Name, []byte(tc.Function.Arguments))
			if opts.Trace {
				fmt.Printf("Trace: results from %s() -> (%s, ", tc.Function.Name,
					util.Lines(out, 1, 160))
				fmt.Print(err)
				fmt.Println(")")
			}
			if err != nil {
				out = fmt.Sprintf("error: %s", err)
			}
			st.steps = append(st.steps, chatAPIStep{
				typ:     ToolOutputStep,
				name:    tc.Function.Name,
				id:      tc.ID,
				content: out,
			})
		}
	}

	return nil
}

func toOpenAICompatTools(tools map[string]Tool) []openai.ChatCompletionToolUnionParam {
	var toolParams []openai.ChatCompletionToolUnionParam
	for _, tl := range tools {
		toolParams = append(toolParams,
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        tl.Name,
				Description: openai_param.NewOpt(tl.Description),
				Parameters:  shared.FunctionParameters(tl.Schema),
			}))
	}

	return toolParams
}

func (mdl *chatAPIModel) SetTools(tools map[string]Tool) error {
	mdl.tools = tools
	mdl.toolParams = toOpenAICompatTools(tools)
	return nil
}

func (st *chatAPIState) SystemPrompt(s string) {
	st.systemPrompt = s
}

func (st *chatAPIState) Prompt(s string) {
	st.steps = append(st.steps, chatAPIStep{
		typ:     PromptStep,
		content: s,
	})
}

func (st *chatAPIState) Len() int {
	return len(st.steps)
}

func (st *chatAPIState) Step(n int) Step {
	step := st.steps[n]
	return Step{
		Type:    step.typ,
		Content: step.content,
		Name:    step.name,
		Input:   []byte(step.input),
	}
}

func (st *chatAPIState) Clear() {
	st.steps = st.steps[:0]
}

func (st *chatAPIState) Usage() (int64, int64, int64) {
	return st.inputTokens, st.outputTokens, st.contextTokens
}

func (step chatAPIStep) toolCall() openai.ChatCompletionMessageToolCallUnionParam {
	if step.typ != ToolCallStep {
		panic("must be a tool call step")
	}

	return openai.ChatCompletionMessageToolCallUnionParam{
		OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
			ID: step.id,
			Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
				Name:      step.name,
				Arguments: step.input,
			},
		},
	}
}

func (st *chatAPIState) toMessages() ([]openai.ChatCompletionMessageParamUnion, int) {
	var msgs []openai.ChatCompletionMessageParamUnion
	var txtLen int

	if st.systemPrompt != "" {
		msgs = append(msgs, openai.SystemMessage(st.systemPrompt))
		txtLen += len(st.systemPrompt)
	}

	for _, step := range st.steps {
		switch step.typ {
		case PromptStep:
			msgs = append(msgs, openai.UserMessage(step.content))
			txtLen += len(step.content)

		case ModelResponseStep:
			msgs = append(msgs, openai.AssistantMessage(step.content))
			txtLen += len(step.content)

		case ToolCallStep:
			if len(msgs) == 0 || msgs[len(msgs)-1].OfAssistant == nil {
				msgs = append(msgs, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{
							step.toolCall(),
						},
					},
				})
			} else {
				asst := msgs[len(msgs)-1].OfAssistant
				asst.ToolCalls = append(asst.ToolCalls, step.toolCall())
			}

		case ToolOutputStep:
			msgs = append(msgs, openai.ToolMessage(step.content, step.id))
			txtLen += len(step.content)

		case ThinkingStep:
			// No portable Chat Completions field for reasoning; skip on resend.

		default:
			panic(fmt.Sprintf("unexpected step type: %s", step.typ))
		}
	}

	return msgs, txtLen
}
