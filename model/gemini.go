package model

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/genai"
)

type geminiModel struct {
	client *genai.Client
	name   string
}

func NewGeminiModel(name, apiKey string, opts *Options) (Model, error) {
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
		name:   name,
	}, nil
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

func (st *geminiState) toContents() ([]*genai.Content, int) {
	var cnts []*genai.Content
	var txtLen int
	for _, step := range st.steps {
		switch step.typ {
		case PromptStep:
			cnts = append(cnts, &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{genai.NewPartFromText(step.content)},
			})

		case ModelResponseStep:
			cnts = append(cnts, &genai.Content{
				Role: "model",
				Parts: []*genai.Part{
					{
						Text:             step.content,
						ThoughtSignature: step.thoughts,
					},
				},
			})

		case ReasoningStep:
			// Gemini doesn't have reasoning blocks, skip
			continue // XXX: is this right?

		case ToolCallStep:
			cnts = append(cnts, &genai.Content{
				Parts: []*genai.Part{
					{
						FunctionCall: &genai.FunctionCall{
							ID:   step.id,
							Args: step.args,
							Name: step.name,
						},
						ThoughtSignature: step.thoughts,
					},
				},
				Role: "model",
			})

		case ToolOutputStep:
			rsp := map[string]any{}
			if step.isError {
				rsp["error"] = step.content
			} else {
				rsp["output"] = step.content
			}

			cnts = append(cnts, &genai.Content{
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							ID:       step.id,
							Name:     step.name,
							Response: rsp,
						},
					},
				},
				Role: "user",
			})

		default:
			panic(fmt.Sprintf("unexpected step type: %d", step.typ))
		}

		txtLen += len(step.content) + len(step.input)
	}

	return cnts, txtLen
}

func toGeminiTools(tools Tools) []*genai.FunctionDeclaration {
	var decls []*genai.FunctionDeclaration
	for _, tl := range tools {
		decls = append(decls, &genai.FunctionDeclaration{
			Description:          tl.Description,
			Name:                 tl.Name,
			ParametersJsonSchema: tl.Schema.schema,
		})
	}

	return decls
}

func partType(prt *genai.Part) string {
	var s string
	if prt.MediaResolution != nil {
		s = "media resolution"
	} else if prt.CodeExecutionResult != nil {
		s = "code execution result"
	} else if prt.ExecutableCode != nil {
		s = "executable code"
	} else if prt.FileData != nil {
		s = "file data"
	} else if prt.FunctionCall != nil {
		s = "function call"
	} else if prt.FunctionResponse != nil {
		s = "function response"
	} else if prt.InlineData != nil {
		s = "inline data"
	} else if prt.Text != "" {
		s = "text"
	} else if prt.ThoughtSignature != nil {
		s = "thought signature"
	} else if prt.VideoMetadata != nil {
		s = "video metadata"
	} else {
		s = "--empty--"
	}

	if prt.Thought {
		s += " thought"
	}

	return s
}

func (mdl *geminiModel) NewState() State {
	return &geminiState{}
}

func (mdl *geminiModel) Generate(ctx context.Context, ast State, tools Tools,
	opts *Options) error {

	st := ast.(*geminiState)

	var gccfg genai.GenerateContentConfig
	if st.systemPrompt != "" {
		gccfg.SystemInstruction = genai.NewContentFromText(st.systemPrompt, genai.RoleUser)
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
				fmt.Printf("%s, %d tools, %d bytes", mdl.name, len(tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		rsp, err := mdl.client.Models.GenerateContent(ctx, mdl.name, cnts, &gccfg)

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

		var toolCalls bool
		for _, cnd := range rsp.Candidates {
			for _, prt := range cnd.Content.Parts {
				if prt.Text != "" {
					st.steps = append(st.steps, geminiStep{
						typ:      ModelResponseStep,
						content:  prt.Text,
						thoughts: prt.ThoughtSignature,
					})
				} else if prt.FunctionCall != nil {
					toolCalls = true

					buf, err := json.Marshal(prt.FunctionCall.Args)

					st.steps = append(st.steps, geminiStep{
						typ:      ToolCallStep,
						name:     prt.FunctionCall.Name,
						id:       prt.FunctionCall.ID,
						input:    buf,
						args:     prt.FunctionCall.Args,
						thoughts: prt.ThoughtSignature,
					})

					var out string
					if err == nil {
						if opts.Trace {
							fmt.Printf("Trace: calling %s(%s)", prt.FunctionCall.Name, buf)
							if opts.Verbose {
								fmt.Printf(" id: %s", prt.FunctionCall.ID)
							}
							fmt.Println()
						}
						out, err = tools.Call(ctx, prt.FunctionCall.Name, buf, opts)
						if opts.Trace {
							fmt.Printf("Trace: results from %s() -> (%q, ",
								prt.FunctionCall.Name, out)
							fmt.Print(err)
							fmt.Println(")")
						}
					}

					if err != nil {
						out = fmt.Sprintf("error: %s", err)
					}
					st.steps = append(st.steps, geminiStep{
						typ:     ToolOutputStep,
						name:    prt.FunctionCall.Name,
						id:      prt.FunctionCall.ID,
						content: out,
						isError: err != nil,
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

		if !toolCalls {
			break
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
			if name[0] != "model" || len(name) != 2 {
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
