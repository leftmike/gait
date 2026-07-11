package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	ollama "github.com/ollama/ollama/api"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/tool"
	"github.com/leftmike/gait/util"
)

type ollamaClient struct {
	client *ollama.Client
	models map[string]ModelMetadata
}

type ollamaModel struct {
	clnt         *ollamaClient
	model        string
	think        *ollama.ThinkValue
	numPredict   int
	contextLimit int
	tools        map[string]tool.Tool
	toolDefs     ollama.Tools
}

func listOllamaModels(client *ollama.Client) map[string]ModelMetadata {
	lst, err := client.List(context.Background())
	if err != nil {
		return map[string]ModelMetadata{}
	}

	mmdm := map[string]ModelMetadata{}
	for _, m := range lst.Models {
		if !slices.Contains(m.Capabilities, "tools") {
			continue
		}

		mmdm[m.Model] = ModelMetadata{
			Model:        m.Model,
			Name:         m.Name,
			Reasoning:    slices.Contains(m.Capabilities, "thinking"),
			ContextLimit: m.Details.ContextLength,
			// XXX: OutputLimit
		}
	}
	return mmdm
}

func newOllamaClient(clntCfg config.ClientConfig) (Client, error) {
	baseURL := clntCfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	client := ollama.NewClient(base, http.DefaultClient)

	return &ollamaClient{
		client: client,
		models: listOllamaModels(client),
	}, nil
}

func (clnt *ollamaClient) Provider() string {
	return "ollama"
}

func (clnt *ollamaClient) ProviderName() string {
	return "Ollama"
}

func (clnt *ollamaClient) ListModels() map[string]ModelMetadata {
	return clnt.models
}

func (clnt *ollamaClient) NewModel(mdlCfg config.ModelConfig) (Model, error) {
	mmd, ok := clnt.models[mdlCfg.Model]
	if !ok {
		return nil, fmt.Errorf("unknown model: %s", mdlCfg.Model)
	}

	var think *ollama.ThinkValue
	switch mdlCfg.Effort {
	case "", "default":
		if mdlCfg.IncludeThoughts {
			think = &ollama.ThinkValue{Value: true}
		}
	case "low", "medium", "high", "max":
		think = &ollama.ThinkValue{Value: mdlCfg.Effort}
	default:
		return nil, fmt.Errorf("effort must be low, medium, high, or max: %s", mdlCfg.Effort)
	}

	numPredict := 8192
	if mdlCfg.MaxTokens > 0 {
		numPredict = mdlCfg.MaxTokens
	}

	return &ollamaModel{
		clnt:         clnt,
		model:        mdlCfg.Model,
		think:        think,
		numPredict:   numPredict,
		contextLimit: mmd.ContextLimit,
	}, nil
}

func (clnt *ollamaClient) NewState() State {
	return &chatAPIState{}
}

func (st *chatAPIState) toOllamaMessages() ([]ollama.Message, int) {
	var msgs []ollama.Message
	var txtLen int

	if st.systemPrompt != "" {
		msgs = append(msgs, ollama.Message{Role: "system", Content: st.systemPrompt})
		txtLen += len(st.systemPrompt)
	}

	var thinking string
	for _, step := range st.steps {
		switch step.typ {
		case PromptStep:
			msgs = append(msgs, ollama.Message{Role: "user", Content: step.content})
			txtLen += len(step.content)

		case ModelResponseStep:
			msgs = append(msgs, ollama.Message{
				Role:     "assistant",
				Thinking: thinking,
				Content:  step.content,
			})
			txtLen += len(thinking) + len(step.content)
			thinking = ""

		case ToolCallStep:
			var args ollama.ToolCallFunctionArguments
			json.Unmarshal([]byte(step.input), &args) //nolint:errcheck
			tc := ollama.ToolCall{
				Function: ollama.ToolCallFunction{
					Name:      step.name,
					Arguments: args,
				},
			}

			if len(msgs) > 0 && msgs[len(msgs)-1].Role == "assistant" &&
				msgs[len(msgs)-1].Content == "" {

				msgs[len(msgs)-1].ToolCalls = append(msgs[len(msgs)-1].ToolCalls, tc)
			} else {
				msgs = append(msgs, ollama.Message{
					Role:      "assistant",
					Thinking:  thinking,
					ToolCalls: []ollama.ToolCall{tc},
				})
				txtLen += len(thinking)
				thinking = ""
			}

		case ToolOutputStep:
			msgs = append(msgs, ollama.Message{Role: "tool", Content: step.content})
			txtLen += len(step.content)

		case ThinkingStep:
			thinking = step.content

		default:
			panic(fmt.Sprintf("unexpected step type: %s", step.typ))
		}
	}

	return msgs, txtLen
}

func (mdl *ollamaModel) Generate(ctx context.Context, ast State, opts *Options) error {
	st := ast.(*chatAPIState)

	for {
		msgs, txtLen := st.toOllamaMessages()

		numCtx := (txtLen/4 + mdl.numPredict) * 6 / 5
		if numCtx < 4096 {
			numCtx = 4096
		}

		var stream bool
		req := &ollama.ChatRequest{
			Model:    mdl.model,
			Messages: msgs,
			Tools:    mdl.toolDefs,
			Stream:   &stream,
			Think:    mdl.think,
			Options: map[string]any{
				"num_predict": mdl.numPredict,
				"num_ctx":     numCtx,
			},
		}

		if opts.Trace {
			fmt.Print("Trace: ollama Chat(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", mdl.model, len(mdl.tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		var toolCalls []ollama.ToolCall
		err := mdl.clnt.client.Chat(ctx, req, func(rsp ollama.ChatResponse) error {
			st.inputTokens += int64(rsp.PromptEvalCount)
			st.outputTokens += int64(rsp.EvalCount)
			st.contextTokens = int64(rsp.PromptEvalCount) + int64(rsp.EvalCount)

			if rsp.DoneReason == "length" {
				return fmt.Errorf("max tokens reached: output was truncated")
			}

			if rsp.Message.Thinking != "" {
				st.steps = append(st.steps, chatAPIStep{
					typ:     ThinkingStep,
					content: rsp.Message.Thinking,
				})
			}

			if rsp.Message.Content != "" {
				st.steps = append(st.steps, chatAPIStep{
					typ:     ModelResponseStep,
					content: rsp.Message.Content,
				})
			}

			for _, tc := range rsp.Message.ToolCalls {
				args, err := json.Marshal(tc.Function.Arguments)
				if err != nil {
					return err
				}
				st.steps = append(st.steps, chatAPIStep{
					typ:   ToolCallStep,
					name:  tc.Function.Name,
					input: string(args),
				})
				toolCalls = append(toolCalls, tc)
			}

			return nil
		})
		if opts.Trace {
			fmt.Print(err)
			fmt.Println()
		}
		if err != nil {
			return err
		}

		if len(toolCalls) == 0 {
			break
		}

		for _, tc := range toolCalls {
			args, _ := json.Marshal(tc.Function.Arguments)
			if opts.Trace {
				fmt.Printf("Trace: calling %s(%s)\n", tc.Function.Name, args)
			}

			out, err := tool.CallTool(ctx, mdl.tools, tc.Function.Name, args)
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
				content: out,
			})
		}
	}

	return nil
}

func toOllamaTools(tools map[string]tool.Tool) (ollama.Tools, error) {
	var toolDefs ollama.Tools
	for _, tl := range tool.Sorted(tools) {
		schemaJSON, err := json.Marshal(tl.Schema)
		if err != nil {
			return nil, err
		}
		var params ollama.ToolFunctionParameters
		if err := json.Unmarshal(schemaJSON, &params); err != nil {
			return nil, err
		}
		toolDefs = append(toolDefs, ollama.Tool{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        tl.Name,
				Description: tl.Description,
				Parameters:  params,
			},
		})
	}
	return toolDefs, nil
}

func (mdl *ollamaModel) SetTools(tools map[string]tool.Tool) error {
	toolDefs, err := toOllamaTools(tools)
	if err != nil {
		return err
	}

	mdl.tools = tools
	mdl.toolDefs = toolDefs

	return nil
}

func (mdl *ollamaModel) ContextLimit() int {
	return mdl.contextLimit
}
