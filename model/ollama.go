package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	ollama "github.com/ollama/ollama/api"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/util"
)

type ollamaClient struct {
	client *ollama.Client
}

type ollamaModel struct {
	model      string
	think      *ollama.ThinkValue
	numPredict int
	tools      map[string]Tool
	toolDefs   ollama.Tools
}

func newOllama(baseURL string) (*ollama.Client, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	return ollama.NewClient(base, http.DefaultClient), nil
}

func newOllamaClient(clntCfg config.ClientConfig) (Client, error) {
	client, err := newOllama(clntCfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return &ollamaClient{client: client}, nil
}

func (clnt *ollamaClient) EffortLevels() []string {
	return []string{"low", "medium", "high", "max"}
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

func toOllamaTools(tools map[string]Tool) (ollama.Tools, error) {
	var toolDefs ollama.Tools
	for _, tl := range tools {
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

func (clnt *ollamaClient) NewModel(mdlCfg config.ModelConfig, tools map[string]Tool) (Model,
	error) {

	var think *ollama.ThinkValue
	if mdlCfg.Effort != "" && mdlCfg.Effort != "default" {
		think = &ollama.ThinkValue{Value: mdlCfg.Effort}
	} else if mdlCfg.IncludeThoughts {
		think = &ollama.ThinkValue{Value: true}
	}

	numPredict := 8192
	if mdlCfg.MaxTokens > 0 {
		numPredict = mdlCfg.MaxTokens
	}

	toolDefs, err := toOllamaTools(tools)
	if err != nil {
		return nil, err
	}

	return &ollamaModel{
		model:      mdlCfg.Model,
		think:      think,
		numPredict: numPredict,
		tools:      tools,
		toolDefs:   toolDefs,
	}, nil
}

func (clnt *ollamaClient) NewState() State {
	return &chatAPIState{}
}

func (clnt *ollamaClient) Generate(ctx context.Context, amdl Model, ast State,
	opts *Options) error {

	mdl := amdl.(*ollamaModel)
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
		err := clnt.client.Chat(ctx, req, func(rsp ollama.ChatResponse) error {
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

			out, err := callTool(ctx, mdl.tools, tc.Function.Name, args)
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

func ListOllamaModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	client, err := newOllama(baseURL)
	if err != nil {
		return nil, err
	}

	lst, err := client.List(ctx)
	if err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range lst.Models {
		models = append(models, ModelInfo{
			Name:    m.Name,
			Created: m.ModifiedAt,
		})
	}
	return models, nil
}
