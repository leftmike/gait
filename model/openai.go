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

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/util"
)

type openAIModel struct {
	client openai.Client
	apiKey string
}

func NewOpenAIModel(apiKey string) (Model, error) {
	return &openAIModel{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
		apiKey: apiKey,
	}, nil
}

func (mdl *openAIModel) EffortLevels() []string {
	return []string{"none", "minimal", "low", "medium", "high", "xhigh"}
}

type openAIStep struct {
	typ       StepType
	content   string
	name      string
	id        string
	input     string
	encrypted string
}

type openAIState struct {
	systemPrompt  string
	steps         []openAIStep
	inputTokens   int64
	outputTokens  int64
	contextTokens int64
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
		Input:   json.RawMessage(step.input),
	}
}

func (st *openAIState) Clear() {
	st.steps = st.steps[:0]
}

func (st *openAIState) Usage() (int64, int64, int64) {
	return st.inputTokens, st.outputTokens, st.contextTokens
}

func openAIInputText(role, text string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemUnionParam{
		OfInputMessage: &responses.ResponseInputItemMessageParam{
			Role: role,
			Content: []responses.ResponseInputContentUnionParam{
				{
					OfInputText: &responses.ResponseInputTextParam{
						Text: text,
					},
				},
			},
		},
	}
}

func (st *openAIState) toInputItemList() ([]responses.ResponseInputItemUnionParam, int) {
	var lst []responses.ResponseInputItemUnionParam
	var txtLen int

	if st.systemPrompt != "" {
		lst = append(lst, openAIInputText("system", st.systemPrompt))
		txtLen += len(st.systemPrompt)
	}

	for idx, step := range st.steps {
		switch step.typ {
		case PromptStep:
			lst = append(lst, openAIInputText("user", step.content))
			txtLen += len(step.content)

		case ModelResponseStep:
			lst = append(lst, responses.ResponseInputItemUnionParam{
				OfOutputMessage: &responses.ResponseOutputMessageParam{
					Content: []responses.ResponseOutputMessageContentUnionParam{
						{
							OfOutputText: &responses.ResponseOutputTextParam{
								Text: step.content,
							},
						},
					},
				},
			})
			txtLen += len(step.content)

		case ThinkingStep:
			if idx+1 < len(st.steps) && st.steps[idx+1].typ == ModelResponseStep {
				lst = append(lst, responses.ResponseInputItemUnionParam{
					OfReasoning: &responses.ResponseReasoningItemParam{
						ID:               step.id,
						EncryptedContent: openai_param.NewOpt(step.encrypted),
						Summary: []responses.ResponseReasoningItemSummaryParam{
							{
								Text: step.content,
							},
						},
					},
				})
				txtLen += len(step.content)
			}

		case ToolCallStep:
			lst = append(lst, responses.ResponseInputItemUnionParam{
				OfFunctionCall: &responses.ResponseFunctionToolCallParam{
					Arguments: step.input,
					CallID:    step.id,
					Name:      step.name,
				},
			})
			txtLen += len(step.input)

		case ToolOutputStep:
			lst = append(lst, responses.ResponseInputItemUnionParam{
				OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
					CallID: step.id,
					Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
						OfString: openai_param.NewOpt(step.content),
					},
				},
			})
			txtLen += len(step.content)
		}
	}

	return lst, txtLen
}

func toOpenAITools(tools map[string]Tool) []responses.ToolUnionParam {
	var toolParams []responses.ToolUnionParam
	for _, tl := range tools {
		toolParams = append(toolParams, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Parameters:  tl.Schema,
				Name:        tl.Name,
				Description: openai_param.NewOpt(tl.Description),
			},
		})
	}

	return toolParams
}

func toOpenAIEffort(opts *config.Options) responses.ReasoningEffort {
	switch opts.Effort {
	case "", "default":
		return ""
	case "none":
		return responses.ReasoningEffortNone
	case "minimal":
		return responses.ReasoningEffortMinimal
	case "low":
		return responses.ReasoningEffortLow
	case "medium":
		return responses.ReasoningEffortMedium
	case "high":
		return responses.ReasoningEffortHigh
	case "xhigh":
		return responses.ReasoningEffortXhigh
	}

	panic(fmt.Sprintf("invalid effort %s", opts.Effort))
}

func (mdl *openAIModel) NewState() State {
	return &openAIState{}
}

func (mdl *openAIModel) Generate(ctx context.Context, ast State,
	tools map[string]Tool, opts *config.Options) error {

	st := ast.(*openAIState)

	reasoningParam := responses.ReasoningParam{
		Effort: toOpenAIEffort(opts),
	}

	var include []responses.ResponseIncludable
	if opts.IncludeThoughts {
		if opts.Verbose {
			reasoningParam.Summary = "detailed"
		} else {
			reasoningParam.Summary = "concise"
		}
		include = []responses.ResponseIncludable{"reasoning.encrypted_content"}
	}

	toolParams := toOpenAITools(tools)

	for {
		lst, txtLen := st.toInputItemList()

		if opts.Trace {
			fmt.Print("Trace: OpenAI Responses.New(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", opts.Model, len(tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		rspParams := responses.ResponseNewParams{
			Model: opts.Model,
			Tools: toolParams,
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: lst,
			},
			Reasoning: reasoningParam,
			Include:   include,
		}
		if opts.MaxTokens > 0 {
			rspParams.MaxOutputTokens = openai.Int(int64(opts.MaxTokens))
		}

		rsp, err := mdl.client.Responses.New(ctx, rspParams)
		if opts.Trace {
			fmt.Print(err)
			if opts.Verbose && rsp != nil {
				fmt.Printf(" tokens: input: %d output: %d total: %d", rsp.Usage.InputTokens,
					rsp.Usage.OutputTokens, rsp.Usage.TotalTokens)
			}
			fmt.Println()
		}
		if err != nil {
			return err
		}

		st.inputTokens += rsp.Usage.InputTokens
		st.outputTokens += rsp.Usage.OutputTokens
		st.contextTokens = rsp.Usage.InputTokens + rsp.Usage.OutputTokens

		if rsp.Status == responses.ResponseStatusIncomplete {
			return fmt.Errorf("max tokens reached: output was truncated")
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
						fmt.Printf("message[%d]", len(rspItem.Content))
					case "reasoning":
						fmt.Printf("reasoning[%d, %d]", len(rspItem.Summary), len(rspItem.Content))
					default:
						fmt.Print(rspItem.Type)
					}
				}
				fmt.Println("]")
			} else {
				fmt.Printf("Trace: %d ResponseItems\n", len(rsp.Output))
			}
		}

		var toolCalls []responses.ResponseOutputItemUnion
		for _, item := range rsp.Output {
			switch item.Type {
			case "message":
				var cnt string
				if len(item.Content) == 1 {
					cnt = item.Content[0].Text
				} else {
					var buf strings.Builder
					for _, cnt := range item.Content {
						buf.WriteString(cnt.Text)
					}
					cnt = buf.String()
				}

				st.steps = append(st.steps, openAIStep{
					typ:     ModelResponseStep,
					content: cnt,
				})

			case "reasoning":
				var cnt string
				if len(item.Summary) == 1 {
					cnt = item.Summary[0].Text
				} else {
					var buf strings.Builder
					for _, smmry := range item.Summary {
						buf.WriteString(smmry.Text)
					}
					cnt = buf.String()
				}

				st.steps = append(st.steps, openAIStep{
					typ:       ThinkingStep,
					id:        item.ID,
					content:   cnt,
					encrypted: item.EncryptedContent,
				})

			case "function_call":
				st.steps = append(st.steps, openAIStep{
					typ:   ToolCallStep,
					name:  item.Name,
					id:    item.CallID,
					input: item.Arguments,
				})
				toolCalls = append(toolCalls, item)

			default:
				if opts.Trace {
					fmt.Printf("Trace: unexpected ResponseItem.Type: %s\n", item.Type)
				} else if opts.Verbose {
					fmt.Printf("[%s]\n", item.Type)
				}
			}
		}

		if len(toolCalls) == 0 {
			break
		}

		for _, item := range toolCalls {
			if opts.Trace {
				fmt.Printf("Trace: calling %s(%s)\n", item.Name, item.Arguments)
			}

			out, err := callTool(ctx, tools, item.Name, []byte(item.Arguments), opts)
			if opts.Trace {
				fmt.Printf("Trace: results from %s() -> (%s, ", item.Name, util.Lines(out, 1, 160))
				fmt.Print(err)
				fmt.Println(")")
			}
			if err != nil {
				out = fmt.Sprintf("error: %s", err)
			}
			st.steps = append(st.steps, openAIStep{
				typ:     ToolOutputStep,
				name:    item.Name,
				id:      item.CallID,
				content: out,
			})
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
