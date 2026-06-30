package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	anthropic_param "github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/util"
)

type anthropicModel struct {
	client anthropic.Client
	apiKey string
}

func NewAnthropicModel(apiKey string) (Model, error) {
	return &anthropicModel{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		apiKey: apiKey,
	}, nil
}

func (mdl *anthropicModel) EffortLevels() []string {
	return []string{"low", "medium", "high", "xhigh", "max"}
}

type anthropicStep struct {
	typ       StepType
	content   string
	name      string
	id        string
	signature string
	input     json.RawMessage
	isError   bool
}

type anthropicState struct {
	systemPrompt  string
	steps         []anthropicStep
	inputTokens   int64
	outputTokens  int64
	contextTokens int64
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

func (st *anthropicState) Clear() {
	st.steps = st.steps[:0]
}

func (st *anthropicState) Usage() (int64, int64, int64) {
	return st.inputTokens, st.outputTokens, st.contextTokens
}

func (st *anthropicState) toMessageParams() ([]anthropic.MessageParam, int) {
	var params []anthropic.MessageParam
	var txtLen int
	for _, step := range st.steps {
		var role anthropic.MessageParamRole
		var blk anthropic.ContentBlockParamUnion

		switch step.typ {
		case PromptStep:
			role = anthropic.MessageParamRoleUser
			blk = anthropic.NewTextBlock(step.content)

		case ModelResponseStep:
			role = anthropic.MessageParamRoleAssistant
			blk = anthropic.NewTextBlock(step.content)

		case ThinkingStep:
			role = anthropic.MessageParamRoleAssistant
			blk = anthropic.NewThinkingBlock(step.signature, step.content)

		case ToolCallStep:
			role = anthropic.MessageParamRoleAssistant
			blk = anthropic.NewToolUseBlock(step.id, step.input, step.name)

		case ToolOutputStep:
			role = anthropic.MessageParamRoleUser
			blk = anthropic.NewToolResultBlock(step.id, step.content, step.isError)

		default:
			panic(fmt.Sprintf("unexpected step type: %d", step.typ))
		}

		if len(params) == 0 || params[len(params)-1].Role != role {
			params = append(params, anthropic.MessageParam{
				Role:    role,
				Content: []anthropic.ContentBlockParamUnion{blk},
			})
		} else {
			params[len(params)-1].Content = append(params[len(params)-1].Content, blk)
		}
		txtLen += len(step.content) + len(step.input)
	}

	return params, txtLen
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

func toAnthropicTools(tools map[string]Tool) []anthropic.ToolUnionParam {
	var toolParams []anthropic.ToolUnionParam
	for _, tl := range tools {
		toolParams = append(toolParams, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				InputSchema: toAnthropicInputSchema(tl.Schema),
				Name:        tl.Name,
				Description: anthropic_param.NewOpt(tl.Description),
				Type:        anthropic.ToolTypeCustom,
			},
		})
	}
	return toolParams
}

func toAnthropicEffort(opts *config.Options) anthropic.OutputConfigEffort {
	switch opts.Effort {
	case "", "default":
		return ""
	case "low":
		return anthropic.OutputConfigEffortLow
	case "medium":
		return anthropic.OutputConfigEffortMedium
	case "high":
		return anthropic.OutputConfigEffortHigh
	case "xhigh":
		return anthropic.OutputConfigEffortXhigh
	case "max":
		return anthropic.OutputConfigEffortMax
	}

	panic(fmt.Sprintf("invalid effort %s", opts.Effort))
}

func (mdl *anthropicModel) NewState() State {
	return &anthropicState{}
}

func anthropicMaxTokens(opts *config.Options) int64 {
	if opts.MaxTokens > 0 {
		return int64(opts.MaxTokens)
	}
	return 64000
}

func (mdl *anthropicModel) Generate(ctx context.Context, ast State,
	tools map[string]Tool, opts *config.Options) error {

	st := ast.(*anthropicState)
	toolParams := toAnthropicTools(tools)
	effort := toAnthropicEffort(opts)

	for {
		msgParams, txtLen := st.toMessageParams()
		req := anthropic.MessageNewParams{
			MaxTokens: anthropicMaxTokens(opts),
			Messages:  msgParams,
			Model:     anthropic.Model(opts.Model),
			Tools:     toolParams,
		}
		if st.systemPrompt != "" {
			req.System = []anthropic.TextBlockParam{{Text: st.systemPrompt}}
			txtLen += len(st.systemPrompt)
		}

		if strings.Contains(opts.Model, "-4-5-") || strings.Contains(opts.Model, "-4-1-") {
			req.Thinking.OfEnabled = &anthropic.ThinkingConfigEnabledParam{
				BudgetTokens: 4096,
			}
		} else {
			req.OutputConfig = anthropic.OutputConfigParam{
				Effort: effort,
			}
			if opts.IncludeThoughts {
				req.Thinking.OfAdaptive = &anthropic.ThinkingConfigAdaptiveParam{
					Display: "summarized",
				}
			} else if effort != "" {
				req.Thinking.OfAdaptive = &anthropic.ThinkingConfigAdaptiveParam{
					Display: "omitted",
				}
			}
		}

		if opts.Trace {
			fmt.Print("Trace: Anthropic Messages.NewStreaming(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", anthropic.Model(opts.Model), len(tools),
					txtLen)
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

		st.inputTokens += rsp.Usage.InputTokens
		st.outputTokens += rsp.Usage.OutputTokens
		st.contextTokens = rsp.Usage.InputTokens + rsp.Usage.OutputTokens

		if rsp.StopReason == anthropic.StopReasonMaxTokens {
			return fmt.Errorf("max tokens reached: output was truncated")
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

		var toolCalls []anthropic.ContentBlockUnion
		for _, blk := range rsp.Content {
			switch blk.Type {
			case "text":
				st.steps = append(st.steps, anthropicStep{
					typ:     ModelResponseStep,
					content: blk.Text,
				})

			case "thinking":
				st.steps = append(st.steps, anthropicStep{
					typ:       ThinkingStep,
					content:   blk.Thinking,
					signature: blk.Signature,
				})

			case "tool_use":
				toolCalls = append(toolCalls, blk)
				st.steps = append(st.steps, anthropicStep{
					typ:   ToolCallStep,
					name:  blk.Name,
					id:    blk.ID,
					input: blk.Input,
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

		if len(toolCalls) == 0 {
			break
		}

		for _, blk := range toolCalls {
			if blk.Type != "tool_use" {
				panic(fmt.Sprintf("unexpected block type in tool calls: %s", blk.Type))
			}

			if opts.Trace {
				fmt.Printf("Trace: calling %s(%s)", blk.Name, blk.Input)
				if opts.Verbose {
					fmt.Printf(" id: %s", blk.ID)
				}
				fmt.Println()
			}

			out, err := callTool(ctx, tools, blk.Name, []byte(blk.Input), opts)
			if opts.Trace {
				fmt.Printf("Trace: results from %s() -> (%s, ", blk.Name, util.Lines(out, 1, 160))
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
