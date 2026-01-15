package model

import (
	"context"
	"encoding/json"
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
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		name:   anthropic.Model(name),
	}, nil
}

type anthropicStep struct {
	typ     StepType
	content string
	name    string
	id      string
	input   json.RawMessage
	isError bool
}

type anthropicState struct {
	systemPrompt string
	steps        []anthropicStep
}

func (st *anthropicState) SystemPrompt(s string) {
	st.systemPrompt = s
}

func (st *anthropicState) Prompt(s string) {
	st.steps = append(st.steps, anthropicStep{
		typ:     PromptStep,
		content: s,
	})
}

func (st *anthropicState) Len() int {
	return len(st.steps)
}

func (st *anthropicState) Step(n int) Step {
	step := st.steps[n]
	return Step{
		Type:    step.typ,
		Content: step.content,
		Name:    step.name,
		Input:   step.input,
	}
}

var (
	stepRole = [5]anthropic.MessageParamRole{
		PromptStep:        anthropic.MessageParamRoleUser,
		ModelResponseStep: anthropic.MessageParamRoleAssistant,
		ToolCallStep:      anthropic.MessageParamRoleAssistant,
		ToolOutputStep:    anthropic.MessageParamRoleUser,
	}
)

func (st *anthropicState) toMessageParams() ([]anthropic.MessageParam, int) {
	var msgParams []anthropic.MessageParam
	var txtLen int
	for _, step := range st.steps {
		switch step.typ {
		case PromptStep, ModelResponseStep:
			msgParams = append(msgParams, anthropic.MessageParam{
				Role: stepRole[step.typ],
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock(step.content),
				},
			})

		case ReasoningStep:
			continue // XXX: is this right?

		case ToolCallStep:
			msgParams = append(msgParams, anthropic.MessageParam{
				Role: stepRole[step.typ],
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewToolUseBlock(step.id, step.input, step.name),
				},
			})

		case ToolOutputStep:
			msgParams = append(msgParams, anthropic.MessageParam{
				Role: stepRole[step.typ],
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewToolResultBlock(step.id, step.content, step.isError),
				},
			})

		default:
			panic(fmt.Sprintf("unexpected step type: %d", step.typ))
		}

		txtLen += len(step.content) + len(step.input)
	}

	return msgParams, txtLen
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
		toolParams = append(toolParams, anthropic.ToolUnionParam{
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

func (mdl *anthropicModel) NewState() State {
	return &anthropicState{}
}

func (mdl *anthropicModel) Generate(ctx context.Context, ast State, tools Tools,
	opts *Options) error {

	st := ast.(*anthropicState)
	toolParams := toAnthropicTools(tools)

	for {
		msgParams, txtLen := st.toMessageParams()
		req := anthropic.MessageNewParams{
			MaxTokens: 1024 * 32,
			Messages:  msgParams,
			Model:     mdl.name,
			Tools:     toolParams,
		}
		if st.systemPrompt != "" {
			req.System = []anthropic.TextBlockParam{{Text: st.systemPrompt}}
			txtLen += len(st.systemPrompt)
		}

		if opts.Trace {
			fmt.Print("Trace: Anthropic Messages.NewStreaming(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", mdl.name, len(tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		strm := mdl.client.Messages.NewStreaming(ctx, req)
		defer strm.Close()

		var rsp anthropic.Message
		for strm.Next() {
			err := rsp.Accumulate(strm.Current())
			if err != nil {
				if opts.Trace {
					fmt.Println("accumulate:", err)
				}
				return err
			}
		}

		err := strm.Err()
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
				st.steps = append(st.steps, anthropicStep{
					typ:     ModelResponseStep,
					content: blk.Text,
				})

			case "thinking":
				st.steps = append(st.steps, anthropicStep{
					typ:     ReasoningStep,
					content: blk.Text,
				}) // XXX: is this right?

			case "tool_use":
				if opts.Trace {
					fmt.Printf("Trace: calling %s(%s)", blk.Name, blk.Input)
					if opts.Verbose {
						fmt.Printf(" id: %s", blk.ID)
					}
					fmt.Println()
				}

				toolCalls = true
				st.steps = append(st.steps, anthropicStep{
					typ:   ToolCallStep,
					name:  blk.Name,
					id:    blk.ID,
					input: blk.Input,
				})
				out, err := tools.Call(ctx, blk.Name, []byte(blk.Input), opts)
				if opts.Trace {
					fmt.Printf("Trace: results from %s() -> (%q, ", blk.Name, out)
					fmt.Print(err)
					fmt.Println(")")
				}
				if err != nil {
					out = fmt.Sprintf("error: %s", err)
				}
				st.steps = append(st.steps, anthropicStep{
					typ:     ToolOutputStep,
					name:    blk.Name,
					id:      blk.ID,
					content: out,
					isError: err != nil,
				})

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

func ListAnthropicModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	lst, err := client.Models.List(ctx, anthropic.ModelListParams{Limit: anthropic.Int(999)})
	if err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, mi := range lst.Data {
		models = append(models, ModelInfo{
			Name:        mi.ID,
			DisplayName: mi.DisplayName,
			Created:     mi.CreatedAt,
		})
	}

	return models, nil
}
