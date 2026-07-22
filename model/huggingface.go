package model

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/llmreg"
	"github.com/leftmike/gait/system"
	"github.com/leftmike/gait/tool"
	"github.com/leftmike/gait/util"
)

type huggingFaceClient struct {
	client openai.Client
	name   string
	models map[string]ModelMetadata
}

type huggingFaceModel struct {
	clnt         *huggingFaceClient
	model        string
	effort       shared.ReasoningEffort
	maxTokens    int64
	contextLimit int
	inputCost    float64
	outputCost   float64
	tools        map[string]tool.Tool
	sandbox      *system.Sandbox
	toolParams   []openai.ChatCompletionToolUnionParam
}

func newHuggingFaceClient(apiKey string) (Client, error) {
	pvdr, err := llmreg.FindProvider("huggingface")
	if err != nil {
		return nil, err
	}

	if apiKey == "" {
		apiKey = os.Getenv("HF_TOKEN")
	}

	return &huggingFaceClient{
		client: openai.NewClient(option.WithAPIKey(apiKey),
			option.WithBaseURL("https://router.huggingface.co/v1")),
		name:   pvdr.Name,
		models: listModels(pvdr),
	}, nil
}

func (clnt *huggingFaceClient) Provider() string {
	return "huggingface"
}

func (clnt *huggingFaceClient) ProviderName() string {
	return clnt.name
}

func (clnt *huggingFaceClient) ListModels() map[string]ModelMetadata {
	return clnt.models
}

func (clnt *huggingFaceClient) NewModel(mdlCfg config.ModelConfig) (Model, error) {
	mmd, ok := clnt.models[strings.SplitN(mdlCfg.Model, ":", 2)[0]]
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

	var effort shared.ReasoningEffort
	switch mdlCfg.Effort {
	case "", "default":
		effort = ""
	case "none":
		effort = shared.ReasoningEffortNone
	case "minimal":
		effort = shared.ReasoningEffortMinimal
	case "low":
		effort = shared.ReasoningEffortLow
	case "medium":
		effort = shared.ReasoningEffortMedium
	case "high":
		effort = shared.ReasoningEffortHigh
	case "xhigh":
		effort = shared.ReasoningEffortXhigh
	default:
		return nil, fmt.Errorf("effort must be none, minimal, low, medium, high, or xhigh: %s",
			mdlCfg.Effort)
	}

	return &huggingFaceModel{
		clnt:         clnt,
		model:        mdlCfg.Model,
		effort:       effort,
		maxTokens:    maxTokens,
		contextLimit: mmd.ContextLimit,
		inputCost:    mmd.InputCost,
		outputCost:   mmd.OutputCost,
	}, nil
}

func (clnt *huggingFaceClient) NewState() State {
	return &chatAPIState{}
}

func (mdl *huggingFaceModel) Generate(ctx context.Context, ast State, opts *Options) error {
	st := ast.(*chatAPIState)

	for {
		msgs, txtLen := st.toMessages()

		if opts.Trace {
			fmt.Print("Trace: HuggingFace Chat.Completions.New(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", mdl.model, len(mdl.toolParams), txtLen)
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
		if mdl.maxTokens > 0 {
			params.MaxCompletionTokens = openai.Int(mdl.maxTokens)
		}
		if mdl.effort != "" {
			params.ReasoningEffort = mdl.effort
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
		st.inputCost += float64(rsp.Usage.PromptTokens) / 10_000 * mdl.inputCost
		st.outputCost += float64(rsp.Usage.CompletionTokens) / 10_000 * mdl.outputCost

		if len(rsp.Choices) == 0 {
			return fmt.Errorf("huggingface: no choices returned")
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

func (mdl *huggingFaceModel) SetTools(tools map[string]tool.Tool, sb *system.Sandbox) error {
	mdl.tools = tools
	mdl.sandbox = sb
	mdl.toolParams = toOpenAICompatTools(tools)
	return nil
}

func (mdl *huggingFaceModel) ContextLimit() int {
	return mdl.contextLimit
}
