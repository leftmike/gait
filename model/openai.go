package model

import (
	"context"
	"encoding/json"
	"fmt"

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
					Parameters:  tool.Parameters,
					Name:        name,
					Description: param.NewOpt(tool.Description),
				},
			})
	}

	return toolParams
}

func (m *openAIModel) Generate(ctx context.Context, s string, tools Tools, opts *Options) (string,
	error) {

	var reasoningParam responses.ReasoningParam
	if opts != nil && opts.Summary {
		reasoningParam.Summary = "detailed"
	}

	rsp, err := m.client.Responses.New(ctx,
		responses.ResponseNewParams{
			Model: m.name,
			Tools: openAIToolParams(tools),
			Input: responses.ResponseNewParamsInputUnion{
				OfString: param.NewOpt(s),
			},
			Reasoning: reasoningParam,
		})
	if err != nil {
		return "", err
	}

	for _, rspItem := range rsp.Output {
		switch rspItem.Type {
		case "message":
			for _, cnt := range rspItem.Content {
				fmt.Println(cnt.Text)
			}

		case "reasoning":
			for _, smmry := range rspItem.Summary {
				fmt.Printf("[%s]\n", smmry.Text)
			}

		case "function_call":
			fmt.Println("call:", rspItem.Name, rspItem.Arguments)
			// XXX: check for the name
			out := tools[rspItem.Name].Function(json.RawMessage(rspItem.Arguments))
			fmt.Println("result:", out)

		default:
			fmt.Println(rspItem.Type)
		}
	}

	return rsp.OutputText(), nil
}
