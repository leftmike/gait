/*
To Do:
- MaxOutputTokens
- Gemini
-- Seed in GenerateContentConfig
-- Turn on thinking?
-- Streaming
-- gemini-3-flash-preview
- Anthropic
-- Turn on thinking?
-- Reasoning summaries
-- StopReason max_tokens
- Test all of the providers
- Pass ctx into functions
*/

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

type getWeatherArgs struct {
	City    string `json:"city" jsonschema:"the city to get the weather for"`
	State   string `json:"state,omitzero" jsonschema:"the state of the city"`
	Country string `json:"country" jsonschema:"the country of the city"`
}

func getWeather(buf []byte) (string, error) {
	var args getWeatherArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	if verbose {
		fmt.Printf("$$ city: %s state: %s country: %s $$\n", args.City, args.State, args.Country)
	}
	return fmt.Sprintf("It is 75 degrees and sunny in %s.", args.City), nil
}

func newModel(provider, modelName, apiKey string, opts *model.Options) (model.Model, error) {
	if strings.EqualFold(provider, "openai") {
		return model.NewOpenAIModel(modelName, apiKey, opts)
	} else if strings.EqualFold(provider, "anthropic") {
		return model.NewAnthropicModel(modelName, apiKey, opts)
	} else if strings.EqualFold(provider, "gemini") {
		return model.NewGeminiModel(modelName, apiKey, opts)
	}

	return nil, fmt.Errorf("unknown provider: %s", provider)
}

func main() {
	provider, modelName, apiKey, opts, err := options()
	if err != nil {
		log.Fatalln(err)
	}
	/*
		infos, err := model.ListGeminiModels(context.Background(), apiKey)
		if err != nil {
			log.Fatalln(err)
		}
		for _, info := range infos {
			fmt.Printf("%s", info.Name)
			if info.DisplayName != "" {
				fmt.Printf(" [%s]", info.DisplayName)
			}
			if !info.Created.Equal(time.Time{}) {
				fmt.Printf(" (%s)", info.Created.Format("02 Jan 2006"))
			}
			fmt.Println()
		}
	*/
	mdl, err := newModel(provider, modelName, apiKey, opts)
	if err != nil {
		log.Fatalln(err)
	}

	if verbose {
		fmt.Println(provider, modelName)
	}

	tools := model.Tools{
		{
			Name:        "get_weather",
			Description: "Gets the current weather for the given city",
			Func:        getWeather,
			Schema:      model.MustToolSchema[getWeatherArgs](),
		},
	}

	if err != nil {
		log.Fatalln(err)
	}

	ctx := context.Background()
	line := liner.NewLiner()
	defer line.Close()

	var st model.State
	for {
		s, err := line.Prompt("> ")
		if err == io.EOF {
			fmt.Println()
			break
		} else if err != nil {
			log.Fatalln(err)
		}

		mdl.Prompt(&st, s)
		n := len(st.Steps)

		err = mdl.Generate(ctx, &st, tools, opts)
		if err != nil {
			log.Fatalln(err)
		}
		for n < len(st.Steps) {
			step := st.Steps[n]

			switch step.Type() {
			case model.PromptStep:
				panic("did not expect prompt step in model output")
			case model.ModelResponseStep:
				fmt.Println(step.Content())
			case model.ReasoningStep:
				if opts.Summary {
					fmt.Printf("[%s]\n", step.Content())
				}
			case model.ToolCallStep:
				if opts.Verbose {
					fmt.Printf("Tool Call: %s(%s)\n", step.Name(), string(step.Input()))
				}
			case model.ToolOutputStep:
				if opts.Verbose {
					fmt.Println("Tool Output: ", step.Content())
				}
			}

			n += 1
		}
	}
}
