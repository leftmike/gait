package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/responses"
)

type openAIModel struct {
	client openai.Client
	name   string
}

func NewOpenAIModel(name, apiKey string, opts *Options) (Model, error) {
	return &openAIModel{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
		name:   name,
	}, nil
}

func openAIToolParams(tools Tools) []responses.ToolUnionParam {
	var toolParams []responses.ToolUnionParam
	for name, tool := range tools {
		toolParams = append(toolParams,
			responses.ToolUnionParam{
				OfFunction: &responses.FunctionToolParam{
					Parameters:  tool.Parameters, // XXX: generate based on the Function
					Name:        name,
					Description: param.NewOpt(tool.Description),
				},
			})
	}

	return toolParams
}

var (
	stepName = [5]string{
		PromptStep:        "User",
		ModelResponseStep: "Assistant",
		ToolCallStep:      "Tool Call",
		ToolOutputStep:    "Tool Output",
	}
)

func (m *openAIModel) generate(ctx context.Context, st *State, tools Tools, opts *Options) error {
	var reasoningParam responses.ReasoningParam
	if opts != nil && opts.Summary {
		if opts.Verbose {
			reasoningParam.Summary = "detailed"
		} else {
			reasoningParam.Summary = "concise"
		}
	}

	for {
		var buf strings.Builder
		if st.SystemPrompt != "" {
			fmt.Fprintf(&buf, "System: %s\n", st.SystemPrompt)
		}

		for _, step := range st.Steps {
			if step.Type == ReasoningStep {
				continue
			}

			fmt.Fprintf(&buf, "%s: %s\n", stepName[step.Type], step.Content)
		}

		rsp, err := m.client.Responses.New(ctx,
			responses.ResponseNewParams{
				Model: m.name,
				Tools: openAIToolParams(tools),
				Input: responses.ResponseNewParamsInputUnion{
					OfString: param.NewOpt(buf.String()),
				},
				Reasoning: reasoningParam,
			})
		if err != nil {
			return err
		}

		var toolCalls bool
		for _, rspItem := range rsp.Output {
			switch rspItem.Type {
			case "message":
				for _, cnt := range rspItem.Content {
					st.appendStep(ModelResponseStep, cnt.Text)
				}

			case "reasoning":
				for _, smmry := range rspItem.Summary {
					st.appendStep(ReasoningStep, smmry.Text)
				}

			case "function_call":
				toolCalls = true
				st.appendStep(ToolCallStep, fmt.Sprintf("%s(%s)", rspItem.Name, rspItem.Arguments))
				// XXX: check for the name
				out := tools[rspItem.Name].Function(json.RawMessage(rspItem.Arguments))
				st.appendStep(ToolOutputStep, out)

			default:
				fmt.Println(rspItem.Type)
			}
		}

		if !toolCalls {
			break
		}
	}

	return nil
}

func (m *openAIModel) Generate(ctx context.Context, st *State, tools Tools, opts *Options) (int,
	error) {

	cnt := len(st.Steps)
	err := m.generate(ctx, st, tools, opts)
	if err != nil {
		return 0, err
	}
	return len(st.Steps) - cnt, nil
}
