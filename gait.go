/*
To Do:
- MaxOutputTokens

- OpenAI
-- Assistant messages can contain multiple content blocks (thinking, text, tool_use); batch those
  together into single assistant messages on requests???

- Gemini
-- Seed in GenerateContentConfig
-- gemini-3-flash-preview

- Anthropic
-- StopReason max_tokens
*/

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/model"

	"github.com/peterh/liner"
)

var (
	verbose bool
	trace   bool
)

type getWeatherArgs struct {
	City    string `json:"city" jsonschema:"the city to get the weather for"`
	State   string `json:"state,omitzero" jsonschema:"the state of the city"`
	Country string `json:"country" jsonschema:"the country of the city"`
}

func getWeather(ctx context.Context, buf []byte) (string, error) {
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

type currentTemperatureArgs struct {
	Location string `json:"location" jsonschema:"location to get the current temperature for"`
}

func currentTemperature(ctx context.Context, buf []byte) (string, error) {
	var args currentTemperatureArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	return "40", nil
}

func newModel(provider, modelName, apiKey string, opts *model.Options) (model.Model, error) {
	switch provider {
	case "openai":
		return model.NewOpenAIModel(modelName, apiKey, opts)
	case "anthropic":
		return model.NewAnthropicModel(modelName, apiKey, opts)
	case "gemini":
		return model.NewGeminiModel(modelName, apiKey, opts)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

func main() {
	fs := flag.NewFlagSet("gait", flag.ExitOnError)

	var thinking bool
	fs.BoolVar(&verbose, "verbose", false, "verbose output")
	fs.BoolVar(&verbose, "v", false, "verbose output")
	fs.BoolVar(&trace, "trace", false, "trace model interaction")
	fs.BoolVar(&trace, "t", false, "trace model interaction")
	fs.BoolVar(&thinking, "thinking", false, "turn on and show thinking")

	provider, modelName, apiKey, err := config.Options(fs)
	if err != nil {
		log.Fatalln(err)
	}
	opts := &model.Options{
		Verbose:  verbose,
		Trace:    trace,
		Thinking: thinking,
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
		{
			Name:        "current_temperature",
			Description: "Gets the current temperature for the given location",
			Func:        currentTemperature,
			Schema:      model.MustToolSchema[currentTemperatureArgs](),
		},
	}

	ctx := context.Background()
	line := liner.NewLiner()
	defer line.Close()

	st := mdl.NewState()
	for {
		s, err := line.Prompt("> ")
		if err == io.EOF {
			fmt.Println()
			break
		} else if err != nil {
			log.Fatalln(err)
		}

		st.Prompt(s)
		n := st.Len()

		err = mdl.Generate(ctx, st, tools, opts)
		if err != nil {
			log.Fatalln(err)
		}

		for n < st.Len() {
			step := st.Step(n)

			switch step.Type {
			case model.PromptStep:
				panic("did not expect prompt step in model output")
			case model.ModelResponseStep:
				fmt.Println(step.Content)
			case model.ThinkingStep:
				if opts.Thinking {
					fmt.Printf("[%s]\n", step.Content)
				}
			case model.ToolCallStep:
				if opts.Verbose {
					fmt.Printf("Tool Call: %s(%s)\n", step.Name, string(step.Input))
				}
			case model.ToolOutputStep:
				if opts.Verbose {
					fmt.Printf("Tool Output: %s: %s\n", step.Name, step.Content)
				}
			}

			n += 1
		}
	}
}
