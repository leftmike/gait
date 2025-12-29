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
	fmt.Println("tool call: ", args)
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

	for {
		s, err := line.Prompt("> ")
		if err == io.EOF {
			fmt.Println()
			break
		} else if err != nil {
			log.Fatalln(err)
		}

		s, err = mdl.Generate(ctx, s, tools, opts)
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Println(s)
	}
}
