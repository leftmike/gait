package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/leftmike/gait/model"
	"github.com/peterh/liner"
)

func getWeather(args json.RawMessage) string {
	return "It is 75 degrees and sunny."
}

/*
	func (w weatherTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
		var p struct {
			City    string `json:"city"`
			State   string `json:"state"`
			Country string `json:"country"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return "", err
		}

return fmt.Sprintf("It's sunny and 75 degrees in %s %s, %s", p.City, p.State, p.Country), nil
*/

func newModel(provider, modelName, apiKey string, opts *model.Options) (model.Model, error) {
	if strings.EqualFold(provider, "openai") {
		return model.NewOpenAIModel(modelName, apiKey, opts)
	}

	return nil, fmt.Errorf("unknown provider: %s", provider)
}

func main() {
	provider, modelName, apiKey, opts, err := options()
	if err != nil {
		log.Fatalln(err)
	}

	mdl, err := newModel(provider, modelName, apiKey, opts)
	if err != nil {
		log.Fatalln(err)
	}

	if verbose {
		fmt.Println(provider, modelName)
	}

	ctx := context.Background()
	line := liner.NewLiner()
	defer line.Close()

	tools := model.Tools{
		"get_weather": {
			Description: "Gets the current weather for the given city",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{
						"type":        "string",
						"description": "The city to get the weather for",
					},
					"state": map[string]any{
						"type":        "string",
						"description": "The state of the city",
					},
					"country": map[string]any{
						"type":        "string",
						"description": "The country of the city",
					},
				},
				"requried": []string{"city", "country"},
			},
			Function: getWeather,
		},
	}

	var st model.State
	for {
		s, err := line.Prompt("> ")
		if err == io.EOF {
			fmt.Println()
			break
		} else if err != nil {
			log.Fatalln(err)
		}

		st.Prompt(s)
		n := len(st.Steps)
		err = mdl.Generate(ctx, &st, tools, opts)
		if err != nil {
			log.Fatalln(err)
		}
		for n < len(st.Steps) {
			step := st.Steps[n]

			switch step.Type {
			case model.PromptStep:
				panic("did not expect prompt step in model output")
			case model.ModelResponseStep:
				fmt.Println(step.Content)
			case model.ReasoningStep:
				if opts.Summary {
					fmt.Printf("[%s]\n", step.Content)
				}
			case model.ToolCallStep:
				if opts.Verbose {
					fmt.Println("Tool Call: ", step.Content)
				}
			case model.ToolOutputStep:
				if opts.Verbose {
					fmt.Println("Tool Output: ", step.Content)
				}
			}

			n += 1
		}
	}
}
