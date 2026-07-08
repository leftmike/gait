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
	"github.com/leftmike/gait/llmreg"
	"github.com/leftmike/gait/util"
)

type anthropicClient struct {
	client anthropic.Client
	apiKey string
	name   string
	models map[string]ModelMetadata
}

type anthropicModel struct {
	model           anthropic.Model
	includeThoughts bool
	effort          anthropic.OutputConfigEffort
	maxTokens       int64
	tools           map[string]Tool
	toolParams      []anthropic.ToolUnionParam
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

func newAnthropicClient(apiKey string) (Client, error) {
	pvdr, err := llmreg.FindProvider("anthropic")
	if err != nil {
		return nil, err
	}

	return &anthropicClient{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		apiKey: apiKey,
		name:   pvdr.Name,
		models: listModels(pvdr),
	}, nil
}

func (clnt *anthropicClient) EffortLevels() []string {
	return []string{"low", "medium", "high", "xhigh", "max"}
}

func (clnt *anthropicClient) Provider() string {
	return "anthropic"
}

func (clnt *anthropicClient) ProviderName() string {
	return clnt.name
}

func (clnt *anthropicClient) ListModels() map[string]ModelMetadata {
	return clnt.models
}

func (clnt *anthropicClient) NewModel(mdlCfg config.ModelConfig, tools map[string]Tool) (Model,
	error) {

	var mdl anthropicModel
	err := mdl.SetModelConfig(mdlCfg)
	if err != nil {
		return nil, err
	}

	err = mdl.SetTools(tools)
	if err != nil {
		return nil, err
	}

	return &mdl, nil
}

func (clnt *anthropicClient) NewState() State {
	return &anthropicState{}
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

func (clnt *anthropicClient) Generate(ctx context.Context, amdl Model, ast State,
	opts *Options) error {

	mdl := amdl.(*anthropicModel)
	st := ast.(*anthropicState)

	for {
		msgParams, txtLen := st.toMessageParams()
		req := anthropic.MessageNewParams{
			MaxTokens: mdl.maxTokens,
			Messages:  msgParams,
			Model:     mdl.model,
			Tools:     mdl.toolParams,
		}
		if st.systemPrompt != "" {
			req.System = []anthropic.TextBlockParam{{Text: st.systemPrompt}}
			txtLen += len(st.systemPrompt)
		}

		// XXX: move to NewModel
		if strings.Contains(mdl.model, "-4-5") || strings.Contains(mdl.model, "-4-1") {
			req.Thinking.OfEnabled = &anthropic.ThinkingConfigEnabledParam{
				BudgetTokens: 4096,
			}
		} else {
			req.OutputConfig = anthropic.OutputConfigParam{
				Effort: mdl.effort,
			}
			if mdl.includeThoughts {
				req.Thinking.OfAdaptive = &anthropic.ThinkingConfigAdaptiveParam{
					Display: "summarized",
				}
			} else if mdl.effort != "" {
				req.Thinking.OfAdaptive = &anthropic.ThinkingConfigAdaptiveParam{
					Display: "omitted",
				}
			}
		}

		if opts.Trace {
			fmt.Print("Trace: Anthropic Messages.NewStreaming(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", mdl.model, len(mdl.tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		strm := clnt.client.Messages.NewStreaming(ctx, req)
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

			out, err := callTool(ctx, mdl.tools, blk.Name, []byte(blk.Input))
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

func (mdl *anthropicModel) SetModelConfig(mdlCfg config.ModelConfig) error {
	maxTokens := int64(64000)
	if mdlCfg.MaxTokens > 0 {
		maxTokens = int64(mdlCfg.MaxTokens)
	}

	var effort anthropic.OutputConfigEffort
	switch mdlCfg.Effort {
	case "", "default":
		effort = ""
	case "low":
		effort = anthropic.OutputConfigEffortLow
	case "medium":
		effort = anthropic.OutputConfigEffortMedium
	case "high":
		effort = anthropic.OutputConfigEffortHigh
	case "xhigh":
		effort = anthropic.OutputConfigEffortXhigh
	case "max":
		effort = anthropic.OutputConfigEffortMax
	default:
		return fmt.Errorf("invalid effort: %s", mdlCfg.Effort)
	}

	mdl.model = anthropic.Model(mdlCfg.Model)
	mdl.includeThoughts = mdlCfg.IncludeThoughts
	mdl.effort = effort
	mdl.maxTokens = maxTokens

	return nil
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

func (mdl *anthropicModel) SetTools(tools map[string]Tool) error {
	mdl.tools = tools
	mdl.toolParams = toAnthropicTools(tools)
	return nil
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
