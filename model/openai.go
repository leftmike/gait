package model

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/responses"
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

/*
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city": map[string]any{
				"type":        "string",
				"description": "The city to get the weather for",
			},
			"state": map[string]any{
				"type":        "string",
				"description": "The state of the city",
			},
			"country": map[string]any{
				"type":        "string",
				"description": "The country of the city",
			},
		},
		"requried": []string{"city", "country"},
	},
*/

var (
	openAIKind = map[reflect.Kind]string{
		reflect.Bool:    "boolean",
		reflect.Int:     "integer",
		reflect.Int8:    "integer",
		reflect.Int16:   "integer",
		reflect.Int32:   "integer",
		reflect.Int64:   "integer",
		reflect.Uint:    "integer",
		reflect.Uint8:   "integer",
		reflect.Uint16:  "integer",
		reflect.Uint32:  "integer",
		reflect.Uint64:  "integer",
		reflect.Float32: "number",
		reflect.Float64: "number",
		reflect.String:  "string",
	}
)

func toOpenAIToolParams(tl Tool) map[string]any {
	/*
		props := map[string]any{}
		var req []string
		for _, arg := range tl.Args {
			s, ok := openAIKind[arg.typ.Kind()]
			if !ok {
				panic(fmt.Sprintf("openai tool argument type not supported: %s %s %s", tl.Name,
					arg.Name, arg.typ))
			}

			props[arg.Name] = map[string]any{
				// null, boolean, object, array, number, string, integer
				"type":        s,
				"description": arg.Description,
			}
			if !arg.Optional {
				req = append(req, arg.Name)
			}
		}

		return map[string]any{
			"type":       "object",
			"properties": props,
			"required":   req,
		}
	*/
	return nil
}

func toOpenAITools(tools Tools) []responses.ToolUnionParam {
	var toolParams []responses.ToolUnionParam
	for _, tl := range tools {
		toolParams = append(toolParams,
			responses.ToolUnionParam{
				OfFunction: &responses.FunctionToolParam{
					Parameters:  toOpenAIToolParams(tl),
					Name:        tl.Name,
					Description: param.NewOpt(tl.Description),
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

func (m *openAIModel) Generate(ctx context.Context, st *State, tools Tools, opts *Options) error {
	var reasoningParam responses.ReasoningParam
	if opts != nil && opts.Summary {
		if opts.Verbose {
			reasoningParam.Summary = "detailed"
		} else {
			reasoningParam.Summary = "concise"
		}
	}

	for {
		var buf strings.Builder
		if st.SystemPrompt != "" {
			fmt.Fprintf(&buf, "System: %s\n", st.SystemPrompt)
		}

		for _, step := range st.Steps {
			if step.Type == ReasoningStep {
				continue
			}

			fmt.Fprintf(&buf, "%s: %s\n", stepName[step.Type], step.Content)
		}

		if opts.Trace {
			fmt.Print("Trace: OpenAI Responses.New(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", m.name, len(tools), buf.Len())
			}
			fmt.Print(") -> ")
		}

		rsp, err := m.client.Responses.New(ctx,
			responses.ResponseNewParams{
				Model: m.name,
				Tools: toOpenAITools(tools),
				Input: responses.ResponseNewParamsInputUnion{
					OfString: param.NewOpt(buf.String()),
				},
				Reasoning: reasoningParam,
			})
		if opts.Trace {
			fmt.Print(err)
			if opts.Verbose {
				fmt.Printf(" input: %d output: %d total: %d", rsp.Usage.InputTokens,
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
					st.appendStep(ModelResponseStep, cnt.Text)
				}

			case "reasoning":
				for _, smmry := range rspItem.Summary {
					st.appendStep(ReasoningStep, smmry.Text)
				}

			case "function_call":
				if opts.Trace {
					fmt.Printf("Trace: calling %s(%s)\n", rspItem.Name, rspItem.Arguments)
				}

				toolCalls = true
				st.appendStep(ToolCallStep, fmt.Sprintf("%s(%s)", rspItem.Name, rspItem.Arguments))
				out, err := tools.Call(rspItem.Name, []byte(rspItem.Arguments), opts)
				if opts.Trace {
					fmt.Printf("Trace: results from %s() -> (%q, ", rspItem.Name, out)
					fmt.Print(err)
					fmt.Println(")")
				}
				if err != nil {
					out = fmt.Sprintf("error: %s", err)
				}
				st.appendStep(ToolOutputStep, out)

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
