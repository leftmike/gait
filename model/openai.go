package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	openai_param "github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

type openAIModel struct {
	client openai.Client
	name   string
}

func NewOpenAIModel(name, apiKey string, opts *Options) (Model, error) {
	return &openAIModel{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
		name:   name,
	}, nil
}

type openAIStep struct {
	typ     StepType
	content string
	name    string
	input   json.RawMessage
}

type openAIState struct {
	systemPrompt string
	steps        []openAIStep
}

func (st *openAIState) SystemPrompt(s string) {
	st.systemPrompt = s
}

func (st *openAIState) Prompt(s string) {
	st.steps = append(st.steps, openAIStep{
		typ:     PromptStep,
		content: s,
	})
}

func (st *openAIState) Len() int {
	return len(st.steps)
}

func (st *openAIState) Step(n int) Step {
	step := st.steps[n]
	return Step{
		Type:    step.typ,
		Content: step.content,
		Name:    step.name,
		Input:   step.input,
	}
}

func toOpenAITools(tools Tools) []responses.ToolUnionParam {
	var toolParams []responses.ToolUnionParam
	for _, tl := range tools {
		toolParams = append(toolParams, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Parameters:  tl.Schema.schema,
				Name:        tl.Name,
				Description: openai_param.NewOpt(tl.Description),
			},
		})
	}
	return toolParams
}

var (
	stepName = [5]string{
		PromptStep:        "User",
		ModelResponseStep: "Assistant",
		ToolCallStep:      "Tool Call",
		ToolOutputStep:    "Tool Output",
	}
)

func (mdl *openAIModel) NewState() State {
	return &openAIState{}
}

func (mdl *openAIModel) Generate(ctx context.Context, ast State, tools Tools,
	opts *Options) error {

	st := ast.(*openAIState)

	var reasoningParam responses.ReasoningParam
	if opts != nil && opts.Summary {
		if opts.Verbose {
			reasoningParam.Summary = "detailed"
		} else {
			reasoningParam.Summary = "concise"
		}
	}

	toolParams := toOpenAITools(tools)

	for {
		var buf strings.Builder
		if st.systemPrompt != "" {
			fmt.Fprintf(&buf, "System: %s\n", st.systemPrompt)
		}

		for _, step := range st.steps {
			switch step.typ {
			case PromptStep, ModelResponseStep, ToolOutputStep:
				fmt.Fprintf(&buf, "%s: %s\n", stepName[step.typ], step.content)

			case ReasoningStep:
				continue

			case ToolCallStep:
				fmt.Fprintf(&buf, "%s: %s(%s)", stepName[step.typ], step.name, step.input)
			}
		}

		if opts.Trace {
			fmt.Print("Trace: OpenAI Responses.New(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", mdl.name, len(tools), buf.Len())
			}
			fmt.Print(") -> ")
		}

		rsp, err := mdl.client.Responses.New(ctx,
			responses.ResponseNewParams{
				Model: mdl.name,
				Tools: toolParams,
				Input: responses.ResponseNewParamsInputUnion{
					OfString: openai_param.NewOpt(buf.String()),
				},
				Reasoning: reasoningParam,
			})
		if opts.Trace {
			fmt.Print(err)
			if opts.Verbose {
				fmt.Printf(" tokens: input: %d output: %d total: %d", rsp.Usage.InputTokens,
					rsp.Usage.OutputTokens, rsp.Usage.TotalTokens)
			}
			fmt.Println()
		}
		if err != nil {
			return err
		}

		if opts.Trace {
			if opts.Verbose {
				fmt.Print("Trace: ResponseItems: [")
				for i, rspItem := range rsp.Output {
					if i > 0 {
						fmt.Print(", ")
					}

					switch rspItem.Type {
					case "message":
						if len(rspItem.Content) > 0 {
							fmt.Print("message")
						}
					case "reasoning":
						if len(rspItem.Summary) > 0 {
							fmt.Print("reasoning")
						}
					default:
						fmt.Print(rspItem.Type)
					}
				}
				fmt.Println("]")
			} else {
				fmt.Printf("Trace: %d ResponseItems\n", len(rsp.Output))
			}
		}

		var toolCalls bool
		for _, rspItem := range rsp.Output {
			switch rspItem.Type {
			case "message":
				for _, cnt := range rspItem.Content {
					st.steps = append(st.steps, openAIStep{
						typ:     ModelResponseStep,
						content: cnt.Text,
					})
				}

			case "reasoning":
				for _, smmry := range rspItem.Summary {
					st.steps = append(st.steps, openAIStep{
						typ:     ReasoningStep,
						content: smmry.Text,
					})
				}

			case "function_call":
				if opts.Trace {
					fmt.Printf("Trace: calling %s(%s)\n", rspItem.Name, rspItem.Arguments)
				}

				toolCalls = true
				st.steps = append(st.steps, openAIStep{
					typ:   ToolCallStep,
					name:  rspItem.Name,
					input: json.RawMessage(rspItem.Arguments),
				})
				out, err := tools.Call(ctx, rspItem.Name, []byte(rspItem.Arguments), opts)
				if opts.Trace {
					fmt.Printf("Trace: results from %s() -> (%q, ", rspItem.Name, out)
					fmt.Print(err)
					fmt.Println(")")
				}
				if err != nil {
					out = fmt.Sprintf("error: %s", err)
				}
				st.steps = append(st.steps, openAIStep{
					typ:     ToolOutputStep,
					name:    rspItem.Name,
					content: out,
				})

			default:
				if opts.Trace {
					fmt.Printf("Trace: unexpected ResponseItem.Type: %s\n", rspItem.Type)
				} else if opts.Verbose {
					fmt.Printf("[%s]\n", rspItem.Type)
				}
			}
		}

		if !toolCalls {
			break
		}
	}

	return nil
}

func ListOpenAIModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	client := openai.NewClient(option.WithAPIKey(apiKey))
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
