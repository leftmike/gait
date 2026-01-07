package model

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	anthropic_param "github.com/anthropics/anthropic-sdk-go/packages/param"
)

type anthropicModel struct {
	client anthropic.Client
	name   anthropic.Model
}

func NewAnthropicModel(name, apiKey string, opts *Options) (Model, error) {
	return &anthropicModel{
		client: anthropic.NewClient(
			option.WithAPIKey(apiKey),
		),
		name: anthropic.Model(name),
	}, nil
}

func toAnthropicInputSchema(scm map[string]any) anthropic.ToolInputSchemaParam {
	var req []string
	if val, ok := scm["required"]; ok {
		for _, v := range val.([]any) {
			req = append(req, v.(string))
		}
	}

	return anthropic.ToolInputSchemaParam{
		Properties: scm["properties"],
		Required:   req,
	}
}

func toAnthropicTools(tools Tools) []anthropic.ToolUnionParam {
	var toolParams []anthropic.ToolUnionParam
	for _, tl := range tools {
		toolParams = append(toolParams,
			anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					InputSchema: toAnthropicInputSchema(tl.Schema.schema),
					Name:        tl.Name,
					Description: anthropic_param.NewOpt(tl.Description),
					Type:        anthropic.ToolTypeCustom,
				},
			})
	}
	return toolParams
}

var (
	stepRole = [5]anthropic.MessageParamRole{
		PromptStep:        anthropic.MessageParamRoleUser,
		ModelResponseStep: anthropic.MessageParamRoleAssistant,
		ToolCallStep:      anthropic.MessageParamRoleAssistant,
		ToolOutputStep:    anthropic.MessageParamRoleUser,
	}
)

func (m *anthropicModel) Generate(ctx context.Context, st *State, tools Tools,
	opts *Options) error {

	toolParams := toAnthropicTools(tools)

	for {
		var txtLen int
		var msgParams []anthropic.MessageParam
		for _, step := range st.Steps {
			switch step.Type {
			case PromptStep, ModelResponseStep:
				msgParams = append(msgParams, anthropic.MessageParam{
					Role: stepRole[step.Type],
					Content: []anthropic.ContentBlockParamUnion{
						anthropic.NewTextBlock(step.Content),
					},
				})

			case ReasoningStep:
				continue // XXX: is this right?

			case ToolCallStep:
				msgParams = append(msgParams, anthropic.MessageParam{
					Role: stepRole[step.Type],
					Content: []anthropic.ContentBlockParamUnion{
						anthropic.NewToolUseBlock(step.ID, step.Input, step.Name),
					},
				})

			case ToolOutputStep:
				msgParams = append(msgParams, anthropic.MessageParam{
					Role: stepRole[step.Type],
					Content: []anthropic.ContentBlockParamUnion{
						anthropic.NewToolResultBlock(step.ID, step.Content, step.IsError),
					},
				})

			default:
				panic(fmt.Sprintf("unexpected step type: %d", step.Type))
			}

			txtLen += len(step.Content) + len(step.Input)
		}

		req := anthropic.MessageNewParams{
			MaxTokens: 1024 * 8,
			Messages:  msgParams,
			Model:     m.name,
			Tools:     toolParams,
		}
		if st.SystemPrompt != "" {
			req.System = []anthropic.TextBlockParam{{Text: st.SystemPrompt}}
			txtLen += len(st.SystemPrompt)
		}

		if opts.Trace {
			fmt.Print("Trace: Anthropic Messages.New(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", m.name, len(tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		rsp, err := m.client.Messages.New(ctx, req)
		if opts.Trace {
			fmt.Print(err)
			if opts.Verbose {
				fmt.Printf(" tokens: input: %d output: %d", rsp.Usage.InputTokens,
					rsp.Usage.OutputTokens)
			}
			fmt.Println()
		}
		if err != nil {
			return err
		}

		if opts.Trace {
			if opts.Verbose {
				fmt.Print("Trace: ContentBlocks: [")
				for i, blk := range rsp.Content {
					if i > 0 {
						fmt.Print(", ")
					}

					fmt.Print(blk.Type)
				}
				fmt.Println("]")
			} else {
				fmt.Printf("Trace: %d ContentBlocks\n", len(rsp.Content))
			}
		}

		var toolCalls bool
		for _, blk := range rsp.Content {
			switch blk.Type {
			case "text":
				st.appendStep(ModelResponseStep, blk.Text)
			case "thinking":
				st.appendStep(ReasoningStep, blk.Text) // XXX: is this right?
			case "tool_use":
				if opts.Trace {
					fmt.Printf("Trace: calling %s(%s)", blk.Name, blk.Input)
					if opts.Verbose {
						fmt.Printf(" id: %s", blk.ID)
					}
					fmt.Println()
				}

				toolCalls = true
				st.appendToolCall(blk.Name, blk.ID, blk.Input)
				out, err := tools.Call(blk.Name, []byte(blk.Input), opts)
				if opts.Trace {
					fmt.Printf("Trace: results from %s() -> (%q, ", blk.Name, out)
					fmt.Print(err)
					fmt.Println(")")
				}
				if err != nil {
					out = fmt.Sprintf("error: %s", err)
				}
				st.appendToolOutput(err != nil, blk.ID, out)

			default:
				// "redacted_thinking", "server_tool_use", "web_search_tool_result"
				if opts.Trace {
					fmt.Printf("Trace: unexpected ContentBlock.Type: %s\n", blk.Type)
				} else if opts.Verbose {
					fmt.Printf("[%s]\n", blk.Type)
				}
			}
		}

		if !toolCalls {
			break
		}
	}

	return nil
}
