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

/*
func toGeminiSchema(scm map[string]any) *genai.Schema {
	schema := &genai.Schema{
		Type: genai.TypeObject,
	}

	if props, ok := scm["properties"].(map[string]any); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for key, val := range props {
			if propMap, ok := val.(map[string]any); ok {
				propSchema := &genai.Schema{}
				if propType, ok := propMap["type"].(string); ok {
					switch propType {
					case "string":
						propSchema.Type = genai.TypeString
					case "integer":
						propSchema.Type = genai.TypeInteger
					case "number":
						propSchema.Type = genai.TypeNumber
					case "boolean":
						propSchema.Type = genai.TypeBoolean
					case "array":
						propSchema.Type = genai.TypeArray
					case "object":
						propSchema.Type = genai.TypeObject
					}
				}
				if desc, ok := propMap["description"].(string); ok {
					propSchema.Description = desc
				}
				if items, ok := propMap["items"].(map[string]any); ok {
					propSchema.Items = toGeminiSchema(map[string]any{"properties": items})
				}
				schema.Properties[key] = propSchema
			}
		}
	}

	if req, ok := scm["required"].([]any); ok {
		for _, v := range req {
			if str, ok := v.(string); ok {
				schema.Required = append(schema.Required, str)
			}
		}
	}

	return schema
}
*/

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

func (m *geminiModel) Generate(ctx context.Context, st *State, tools Tools,
	opts *Options) error {

	var gccfg genai.GenerateContentConfig
	if st.SystemPrompt != "" {
		gccfg.SystemInstruction = genai.NewContentFromText(st.SystemPrompt, genai.RoleUser)
	}
	if len(tools) > 0 {
		gccfg.Tools = []*genai.Tool{
			{
				FunctionDeclarations: toGeminiTools(tools),
			},
		}
	}

	for {
		txtLen := len(st.SystemPrompt)

		var cnts []*genai.Content
		for _, step := range st.Steps {
			switch step.Type {
			case PromptStep:
				/*
					// Flush any pending tool responses before user message
					if len(pendingToolResponses) > 0 {
						contents = append(contents, &genai.Content{
							Role:  "user",
							Parts: pendingToolResponses,
						})
						pendingToolResponses = nil
					}
				*/
				cnts = append(cnts, &genai.Content{
					Role:  "user",
					Parts: []*genai.Part{genai.NewPartFromText(step.Content)},
				})

			case ModelResponseStep:
				cnts = append(cnts, &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{genai.NewPartFromText(step.Content)},
				})

			case ReasoningStep:
				// Gemini doesn't have reasoning blocks, skip
				continue // XXX: is this right?

			case ToolCallStep:
				cnts = append(cnts, &genai.Content{
					Parts: []*genai.Part{
						{
							FunctionCall: &genai.FunctionCall{
								ID:   step.ID,
								Args: step.Args,
								Name: step.Name,
							},
						},
					},
					Role: "model",
				})

			case ToolOutputStep:
				rsp := map[string]any{}
				if step.IsError {
					rsp["error"] = step.Content
				} else {
					rsp["output"] = step.Content
				}

				cnts = append(cnts, &genai.Content{
					Parts: []*genai.Part{
						{
							FunctionResponse: &genai.FunctionResponse{
								ID:       step.ID,
								Name:     step.Name,
								Response: rsp,
							},
						},
					},
					Role: "user",
				})

			default:
				panic(fmt.Sprintf("unexpected step type: %d", step.Type))
			}

			txtLen += len(step.Content) + len(step.Input)
		}

		/*
			// Flush any remaining tool responses
			if len(pendingToolResponses) > 0 {
				contents = append(contents, &genai.Content{
					Role:  "user",
					Parts: pendingToolResponses,
				})
			}
		*/

		if opts.Trace {
			fmt.Print("Trace: Gemini GenerateContent(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", m.name, len(tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		rsp, err := m.client.Models.GenerateContent(ctx, m.name, cnts, &gccfg)

		if opts.Trace {
			fmt.Print(err)
			if opts.Verbose {
				if rsp != nil { // XXX: are nil checks necessary
					md := rsp.UsageMetadata
					if md != nil {
						fmt.Printf(" tokens: input: %d output: %d total %d", md.PromptTokenCount,
							md.CandidatesTokenCount, md.TotalTokenCount)
					}
					fmt.Print(rsp.ModelVersion)
				}
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
					st.appendStep(ModelResponseStep, prt.Text)
				} else if prt.FunctionCall != nil {
					fc := prt.FunctionCall
					buf, err := json.Marshal(fc.Args)
					if err != nil {
						// XXX: handle by returning the error to the model
						panic(err)
					}

					if opts.Trace {
						fmt.Printf("Trace: calling %s(%s)", fc.Name, buf)
						if opts.Verbose {
							fmt.Printf(" id: %s", fc.ID)
						}
						fmt.Println()
					}

					toolCalls = true
					st.appendToolCall(fc.Name, fc.ID, buf, fc.Args)
					out, err := tools.Call(fc.Name, buf, opts)
					if opts.Trace {
						fmt.Printf("Trace: results from %s() -> (%q, ", fc.Name, out)
						fmt.Print(err)
						fmt.Println(")")
					}
					if err != nil {
						out = fmt.Sprintf("error: %s", err)
					}
					st.appendToolOutput(err != nil, fc.Name, fc.ID, out)
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
