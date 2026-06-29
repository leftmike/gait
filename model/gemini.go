package model

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/genai"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/util"
)

type geminiModel struct {
	client *genai.Client
	apiKey string
}

func NewGeminiModel(apiKey string) (Model, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	return &geminiModel{
		client: client,
		apiKey: apiKey,
	}, nil
}

func (mdl *geminiModel) EffortLevels() []string {
	return []string{"minimal", "low", "medium", "high"}
}

type geminiStep struct {
	typ      StepType
	content  string
	name     string
	id       string
	input    json.RawMessage
	args     map[string]any
	thoughts []byte
	isError  bool
}

type geminiState struct {
	systemPrompt string
	steps        []geminiStep
}

func (st *geminiState) SystemPrompt(s string) {
	st.systemPrompt = s
}

func (st *geminiState) Prompt(s string) {
	st.steps = append(st.steps, geminiStep{
		typ:     PromptStep,
		content: s,
	})
}

func (st *geminiState) Len() int {
	return len(st.steps)
}

func (st *geminiState) Step(n int) Step {
	step := st.steps[n]
	return Step{
		Type:    step.typ,
		Content: step.content,
		Name:    step.name,
		Input:   step.input,
	}
}

func (st *geminiState) Clear() {
	st.steps = st.steps[:0]
}

func (st *geminiState) toContents() ([]*genai.Content, int) {
	var cnts []*genai.Content
	var txtLen int
	for _, step := range st.steps {
		var role string
		var prt *genai.Part

		switch step.typ {
		case PromptStep:
			role = "user"
			prt = &genai.Part{
				Text: step.content,
			}

		case ModelResponseStep:
			role = "model"
			prt = &genai.Part{
				Text:             step.content,
				ThoughtSignature: step.thoughts,
			}

		case ThinkingStep:
			role = "model"
			prt = &genai.Part{
				Text:    step.content,
				Thought: true,
			}

		case ToolCallStep:
			role = "model"
			prt = &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   step.id,
					Args: step.args,
					Name: step.name,
				},
				ThoughtSignature: step.thoughts,
			}

		case ToolOutputStep:
			rsp := map[string]any{}
			if step.isError {
				rsp["error"] = step.content
			} else {
				rsp["output"] = step.content
			}

			role = "user"
			prt = &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       step.id,
					Name:     step.name,
					Response: rsp,
				},
			}

		default:
			panic(fmt.Sprintf("unexpected step type: %d", step.typ))
		}

		if len(cnts) == 0 || cnts[len(cnts)-1].Role != role {
			cnts = append(cnts, &genai.Content{
				Role:  role,
				Parts: []*genai.Part{prt},
			})
		} else {
			cnts[len(cnts)-1].Parts = append(cnts[len(cnts)-1].Parts, prt)
		}
		txtLen += len(step.content) + len(step.input)
	}

	return cnts, txtLen
}

func toGeminiTools(tools map[string]Tool) []*genai.FunctionDeclaration {
	var decls []*genai.FunctionDeclaration
	for _, tl := range tools {
		decls = append(decls, &genai.FunctionDeclaration{
			Description:          tl.Description,
			Name:                 tl.Name,
			ParametersJsonSchema: tl.Schema,
		})
	}

	return decls
}

func partType(prt *genai.Part) string {
	if prt.MediaResolution != nil {
		return "media resolution"
	} else if prt.CodeExecutionResult != nil {
		return "code execution result"
	} else if prt.ExecutableCode != nil {
		return "executable code"
	} else if prt.FileData != nil {
		return "file data"
	} else if prt.FunctionCall != nil {
		return "function call"
	} else if prt.FunctionResponse != nil {
		return "function response"
	} else if prt.InlineData != nil {
		return "inline data"
	} else if prt.Text != "" {
		if prt.Thought {
			return "thought"
		}
		return "text"
	} else if prt.ThoughtSignature != nil {
		return "thought signature"
	} else if prt.VideoMetadata != nil {
		return "video metadata"
	}

	return "--empty--"
}

func toGeminiThinkingLevel(opts *config.Options) genai.ThinkingLevel {
	switch opts.Effort {
	case "", "default":
		return genai.ThinkingLevelUnspecified
	case "minimal":
		return genai.ThinkingLevelMinimal
	case "low":
		return genai.ThinkingLevelLow
	case "medium":
		return genai.ThinkingLevelMedium
	case "high":
		return genai.ThinkingLevelHigh
	}

	panic(fmt.Sprintf("invalid effort %s", opts.Effort))
}

func (mdl *geminiModel) NewState() State {
	return &geminiState{}
}

type geminiToolCall struct {
	buf []byte
	err error
	prt *genai.Part
}

func (mdl *geminiModel) Generate(ctx context.Context, ast State,
	tools map[string]Tool, opts *config.Options) error {

	st := ast.(*geminiState)
	thinkingLevel := toGeminiThinkingLevel(opts)

	var gccfg genai.GenerateContentConfig
	if opts.MaxTokens > 0 {
		gccfg.MaxOutputTokens = int32(opts.MaxTokens)
	} else {
		gccfg.MaxOutputTokens = 65536
	}
	if st.systemPrompt != "" {
		gccfg.SystemInstruction = genai.NewContentFromText(st.systemPrompt, genai.RoleUser)
	}
	if opts.IncludeThoughts || thinkingLevel != genai.ThinkingLevelUnspecified {
		gccfg.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: opts.IncludeThoughts,
			ThinkingLevel:   thinkingLevel,
		}
	}
	if len(tools) > 0 {
		gccfg.Tools = []*genai.Tool{
			{
				FunctionDeclarations: toGeminiTools(tools),
			},
		}
	}

	for {
		cnts, txtLen := st.toContents()
		txtLen += len(st.systemPrompt)

		if opts.Trace {
			fmt.Print("Trace: Gemini GenerateContent(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", opts.Model, len(tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		rsp, err := mdl.client.Models.GenerateContent(ctx, opts.Model, cnts, &gccfg)

		if opts.Trace {
			fmt.Print(err)
			if opts.Verbose {
				md := rsp.UsageMetadata
				fmt.Printf(" tokens: input: %d output: %d total %d", md.PromptTokenCount,
					md.CandidatesTokenCount, md.TotalTokenCount)
			}
			fmt.Println()
		}
		if err != nil {
			return err
		}

		if opts.Trace {
			if opts.Verbose {
				fmt.Print("Trace: Parts: [")
			}

			var cnt int
			var needComma bool
			for _, cnd := range rsp.Candidates {
				for _, prt := range cnd.Content.Parts {
					if opts.Verbose {
						if needComma {
							fmt.Print(", ")
						} else {
							needComma = true
						}

						fmt.Print(partType(prt))
					}

					cnt += 1
				}
			}

			if opts.Verbose {
				fmt.Println("]")
			} else {
				fmt.Printf("Trace: %d Parts\n", cnt)
			}
		}

		var toolCalls []geminiToolCall
		for _, cnd := range rsp.Candidates {
			for _, prt := range cnd.Content.Parts {
				if prt.Thought {
					st.steps = append(st.steps, geminiStep{
						typ:     ThinkingStep,
						content: prt.Text,
					})
				} else if prt.Text != "" {
					st.steps = append(st.steps, geminiStep{
						typ:      ModelResponseStep,
						content:  prt.Text,
						thoughts: prt.ThoughtSignature,
					})
				} else if prt.FunctionCall != nil {
					buf, err := json.Marshal(prt.FunctionCall.Args)

					st.steps = append(st.steps, geminiStep{
						typ:      ToolCallStep,
						name:     prt.FunctionCall.Name,
						id:       prt.FunctionCall.ID,
						input:    buf,
						args:     prt.FunctionCall.Args,
						thoughts: prt.ThoughtSignature,
					})
					toolCalls = append(toolCalls, geminiToolCall{
						buf: buf,
						err: err,
						prt: prt,
					})
				} else {
					if opts.Trace {
						fmt.Printf("Trace: unexpected Part: %s\n", partType(prt))
					} else if opts.Verbose {
						fmt.Printf("[%s]\n", partType(prt))
					}
				}
			}
		}

		if len(toolCalls) == 0 {
			break
		}

		for _, tc := range toolCalls {
			var out string
			err := tc.err
			if err == nil {
				if opts.Trace {
					fmt.Printf("Trace: calling %s(%s)", tc.prt.FunctionCall.Name, tc.buf)
					if opts.Verbose {
						fmt.Printf(" id: %s", tc.prt.FunctionCall.ID)
					}
					fmt.Println()
				}
				out, err = callTool(ctx, tools, tc.prt.FunctionCall.Name, tc.buf, opts)
				if opts.Trace {
					fmt.Printf("Trace: results from %s() -> (%s, ", tc.prt.FunctionCall.Name,
						util.Lines(out, 1, 160))
					fmt.Print(err)
					fmt.Println(")")
				}
			}

			if err != nil {
				out = fmt.Sprintf("error: %s", err)
			}
			st.steps = append(st.steps, geminiStep{
				typ:     ToolOutputStep,
				name:    tc.prt.FunctionCall.Name,
				id:      tc.prt.FunctionCall.ID,
				content: out,
				isError: err != nil,
			})
		}
	}

	return nil
}

func ListGeminiModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	pg, err := client.Models.List(ctx, &genai.ListModelsConfig{PageSize: 999})
	if err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range pg.Items {
		if slices.Contains(m.SupportedActions, "generateContent") {
			name := strings.SplitN(m.Name, "/", 2)
			if name[0] != "models" || len(name) != 2 {
				continue
			}

			models = append(models, ModelInfo{
				Name:        name[1],
				DisplayName: m.DisplayName,
			})
		}
	}

	return models, nil
}
