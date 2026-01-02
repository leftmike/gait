package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/leftmike/gait/model"
	"github.com/peterh/liner"
)

var (
	getWeatherTool = model.Tool{
		Name:        "get_weather",
		Description: "Gets the current weather for the given city",
		Args: []model.ToolArg{
			{Arg: "city", Description: "The city to get the weather for"},
			{Arg: "state", Description: "The state of the city", Optional: true},
			{Arg: "country", Description: "The country of the city"},
		},
		Func: func(city, state, country string) (string, error) {
			if verbose {
				fmt.Printf("$$ city: %s state: %s country: %s $$\n", city, state, country)
			}
			return fmt.Sprintf("It is 75 degrees and sunny in %s.", city), nil
		},
	}
)

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

	tools := model.Tools{
		&getWeatherTool,
	}

	err = tools.Build()
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
