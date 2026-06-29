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

type ollamaModel struct {
	client *ollama.Client
}

func newOllamaClient(baseURL string) (*ollama.Client, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	return ollama.NewClient(base, http.DefaultClient), nil
}

func NewOllamaModel(provider *config.Provider) (Model, error) {
	client, err := newOllamaClient(provider.BaseURL)
	if err != nil {
		return nil, err
	}
	return &ollamaModel{client: client}, nil
}

func (mdl *ollamaModel) EffortLevels() []string {
	return []string{"low", "medium", "high", "max"}
}

func toOllamaThink(opts *config.Options) *ollama.ThinkValue {
	if opts.Effort != "" && opts.Effort != "default" {
		return &ollama.ThinkValue{Value: opts.Effort}
	}
	if opts.IncludeThoughts {
		return &ollama.ThinkValue{Value: true}
	}
	return nil
}

func (mdl *ollamaModel) NewState() State {
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

func (mdl *ollamaModel) Generate(ctx context.Context, ast State,
	tools map[string]Tool, opts *config.Options) error {

	st := ast.(*chatAPIState)
	toolDefs, err := toOllamaTools(tools)
	if err != nil {
		return err
	}

	stream := false
	think := toOllamaThink(opts)

	numPredict := 8192
	if opts.MaxTokens > 0 {
		numPredict = opts.MaxTokens
	}

	for {
		msgs, txtLen := st.toOllamaMessages()

		numCtx := (txtLen/4 + numPredict) * 6 / 5
		if numCtx < 4096 {
			numCtx = 4096
		}

		req := &ollama.ChatRequest{
			Model:    opts.Model,
			Messages: msgs,
			Tools:    toolDefs,
			Stream:   &stream,
			Think:    think,
			Options: map[string]any{
				"num_predict": numPredict,
				"num_ctx":     numCtx,
			},
		}

		if opts.Trace {
			fmt.Print("Trace: ollama Chat(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", opts.Model, len(tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		var rspMsg ollama.Message
		var doneReason string
		err := mdl.client.Chat(ctx, req, func(rsp ollama.ChatResponse) error {
			rspMsg = rsp.Message
			doneReason = rsp.DoneReason
			return nil
		})
		if opts.Trace {
			fmt.Print(err)
			fmt.Println()
		}
		if err != nil {
			return err
		}

		if doneReason == "length" {
			return fmt.Errorf("max tokens reached: output was truncated")
		}

		if rspMsg.Thinking != "" {
			st.steps = append(st.steps, chatAPIStep{
				typ:     ThinkingStep,
				content: rspMsg.Thinking,
			})
		}

		if rspMsg.Content != "" {
			st.steps = append(st.steps, chatAPIStep{
				typ:     ModelResponseStep,
				content: rspMsg.Content,
			})
		}

		var toolCalls []ollama.ToolCall
		for _, tc := range rspMsg.ToolCalls {
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

		if len(toolCalls) == 0 {
			break
		}

		for _, tc := range toolCalls {
			args, _ := json.Marshal(tc.Function.Arguments)
			if opts.Trace {
				fmt.Printf("Trace: calling %s(%s)\n", tc.Function.Name, args)
			}

			out, err := callTool(ctx, tools, tc.Function.Name, args, opts)
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
	client, err := newOllamaClient(baseURL)
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
