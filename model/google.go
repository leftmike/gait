package model

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/llmreg"
	"github.com/leftmike/gait/util"
)

type googleClient struct {
	client *genai.Client
	apiKey string
	name   string
	models map[string]ModelMetadata
}

type googleModel struct {
	model           string
	includeThoughts bool
	thinkingLevel   genai.ThinkingLevel
	maxOutputTokens int32
	tools           map[string]Tool
	funcDecls       []*genai.FunctionDeclaration
}

type googleStep struct {
	typ      StepType
	content  string
	name     string
	id       string
	input    json.RawMessage
	args     map[string]any
	thoughts []byte
	isError  bool
}

type googleState struct {
	systemPrompt  string
	steps         []googleStep
	inputTokens   int32
	outputTokens  int32
	contextTokens int32
}

type googleToolCall struct {
	buf []byte
	err error
	prt *genai.Part
}

func newGoogleClient(apiKey string) (Client, error) {
	pvdr, err := llmreg.FindProvider("google")
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	return &googleClient{
		client: client,
		apiKey: apiKey,
		name:   pvdr.Name,
		models: listModels(pvdr),
	}, nil
}

func (clnt *googleClient) EffortLevels() []string {
	return []string{"minimal", "low", "medium", "high"}
}

func (clnt *googleClient) Provider() string {
	return "google"
}

func (clnt *googleClient) ProviderName() string {
	return clnt.name
}

func (clnt *googleClient) ListModels() map[string]ModelMetadata {
	return clnt.models
}

func toGoogleFuncDecls(tools map[string]Tool) []*genai.FunctionDeclaration {
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

func (clnt *googleClient) NewModel(mdlCfg config.ModelConfig, tools map[string]Tool) (Model,
	error) {

	var thinkingLevel genai.ThinkingLevel
	switch mdlCfg.Effort {
	case "", "default":
		thinkingLevel = genai.ThinkingLevelUnspecified
	case "minimal":
		thinkingLevel = genai.ThinkingLevelMinimal
	case "low":
		thinkingLevel = genai.ThinkingLevelLow
	case "medium":
		thinkingLevel = genai.ThinkingLevelMedium
	case "high":
		thinkingLevel = genai.ThinkingLevelHigh
	default:
		return nil, fmt.Errorf("invalid effort: %s", mdlCfg.Effort)
	}

	var maxOutputTokens int32
	if mdlCfg.MaxTokens > 0 {
		maxOutputTokens = int32(mdlCfg.MaxTokens)
	} else {
		maxOutputTokens = 65536
	}

	return &googleModel{
		model:           mdlCfg.Model,
		includeThoughts: mdlCfg.IncludeThoughts,
		thinkingLevel:   thinkingLevel,
		maxOutputTokens: maxOutputTokens,
		tools:           tools,
		funcDecls:       toGoogleFuncDecls(tools),
	}, nil
}

func (clnt *googleClient) NewState() State {
	return &googleState{}
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

func (st *googleState) toContents() ([]*genai.Content, int) {
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

func (clnt *googleClient) Generate(ctx context.Context, amdl Model, ast State,
	opts *Options) error {

	mdl := amdl.(*googleModel)
	st := ast.(*googleState)

	gccfg := genai.GenerateContentConfig{
		MaxOutputTokens: mdl.maxOutputTokens,
	}

	if st.systemPrompt != "" {
		gccfg.SystemInstruction = genai.NewContentFromText(st.systemPrompt, genai.RoleUser)
	}
	if mdl.includeThoughts || mdl.thinkingLevel != genai.ThinkingLevelUnspecified {
		gccfg.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: mdl.includeThoughts,
			ThinkingLevel:   mdl.thinkingLevel,
		}
	}
	if len(mdl.tools) > 0 {
		gccfg.Tools = []*genai.Tool{
			{
				FunctionDeclarations: mdl.funcDecls,
			},
		}
	}

	for {
		cnts, txtLen := st.toContents()
		txtLen += len(st.systemPrompt)

		if opts.Trace {
			fmt.Print("Trace: Google GenerateContent(")
			if opts.Verbose {
				fmt.Printf("%s, %d tools, %d bytes", mdl.model, len(mdl.tools), txtLen)
			}
			fmt.Print(") -> ")
		}

		rsp, err := clnt.client.Models.GenerateContent(ctx, mdl.model, cnts, &gccfg)

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

		st.inputTokens += rsp.UsageMetadata.PromptTokenCount
		st.outputTokens += rsp.UsageMetadata.CandidatesTokenCount
		st.contextTokens = rsp.UsageMetadata.PromptTokenCount +
			rsp.UsageMetadata.CandidatesTokenCount

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

		var toolCalls []googleToolCall
		for _, cnd := range rsp.Candidates {
			if cnd.FinishReason == genai.FinishReasonMaxTokens {
				return fmt.Errorf("max tokens reached: output was truncated")
			}
			for _, prt := range cnd.Content.Parts {
				if prt.Thought {
					st.steps = append(st.steps, googleStep{
						typ:     ThinkingStep,
						content: prt.Text,
					})
				} else if prt.Text != "" {
					st.steps = append(st.steps, googleStep{
						typ:      ModelResponseStep,
						content:  prt.Text,
						thoughts: prt.ThoughtSignature,
					})
				} else if prt.FunctionCall != nil {
					buf, err := json.Marshal(prt.FunctionCall.Args)

					st.steps = append(st.steps, googleStep{
						typ:      ToolCallStep,
						name:     prt.FunctionCall.Name,
						id:       prt.FunctionCall.ID,
						input:    buf,
						args:     prt.FunctionCall.Args,
						thoughts: prt.ThoughtSignature,
					})
					toolCalls = append(toolCalls, googleToolCall{
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
				out, err = callTool(ctx, mdl.tools, tc.prt.FunctionCall.Name, tc.buf)
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
			st.steps = append(st.steps, googleStep{
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

func (st *googleState) SystemPrompt(s string) {
	st.systemPrompt = s
}

func (st *googleState) Prompt(s string) {
	st.steps = append(st.steps, googleStep{
		typ:     PromptStep,
		content: s,
	})
}

func (st *googleState) Len() int {
	return len(st.steps)
}

func (st *googleState) Step(n int) Step {
	step := st.steps[n]
	return Step{
		Type:    step.typ,
		Content: step.content,
		Name:    step.name,
		Input:   step.input,
	}
}

func (st *googleState) Clear() {
	st.steps = st.steps[:0]
}

func (st *googleState) Usage() (int64, int64, int64) {
	return int64(st.inputTokens), int64(st.outputTokens), int64(st.contextTokens)
}
