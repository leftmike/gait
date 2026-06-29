package model

import (
	"context"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	openai_param "github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/util"
)

// Model for OpenAI-compatible Chat Completions API (/v1/chat/completions)
type chatAPIModel struct {
	client   openai.Client
	provider string
}

func newChatAPIModel(provider *config.Provider, client openai.Client) (Model, error) {
	return &chatAPIModel{
		client:   client,
		provider: provider.Name,
	}, nil
}

func newLlamaCppClient(baseURL, apiKey string) openai.Client {
	if baseURL == "" {
		baseURL = "http://localhost:8080/v1"
	}
	if apiKey == "" {
		apiKey = "no-api-key"
	}
	return openai.NewClient(option.WithBaseURL(baseURL), option.WithAPIKey(apiKey))
}

func NewLlamaCppModel(provider *config.Provider) (Model, error) {
	return newChatAPIModel(provider, newLlamaCppClient(provider.BaseURL, provider.APIKey))
}

func (mdl *chatAPIModel) EffortLevels() []string {
	return nil
}

type chatAPIStep struct {
	typ     StepType
	content string
	name    string
	id      string
	input   string
}

type chatAPIState struct {
	systemPrompt string
	steps        []chatAPIStep
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

func (mdl *chatAPIModel) NewState() State {
	return &chatAPIState{}
}

func (mdl *chatAPIModel) Generate(ctx context.Context, ast State,
	tools map[string]Tool, opts *config.Options) error {

	// XXX: opts.IncludeThoughts and opts.Effort

	st := ast.(*chatAPIState)
	toolParams := toOpenAICompatTools(tools)

	for {
		msgs, txtLen := st.toMessages()

		if opts.Trace {
			fmt.Printf("Trace: %s Chat.Completions.New(", mdl.provider)
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", opts.Model, len(tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		params := openai.ChatCompletionNewParams{
			Model:    opts.Model,
			Messages: msgs,
		}
		if len(toolParams) > 0 {
			params.Tools = toolParams
		}

		rsp, err := mdl.client.Chat.Completions.New(ctx, params)
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

		if len(rsp.Choices) == 0 {
			return fmt.Errorf("%s: no choices returned", mdl.provider)
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

			out, err := callTool(ctx, tools, tc.Function.Name, []byte(tc.Function.Arguments), opts)
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

func listOpenAICompatModels(ctx context.Context, client openai.Client) ([]ModelInfo, error) {
	lst, err := client.Models.List(ctx)
	if err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, md := range lst.Data {
		models = append(models, ModelInfo{
			Name:    md.ID,
			Created: time.Unix(md.Created, 0),
		})
	}

	return models, nil
}

func ListLlamaCppModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	return listOpenAICompatModels(ctx, newLlamaCppClient(baseURL, apiKey))
}
