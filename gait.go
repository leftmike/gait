/*
To Do:
- MaxOutputTokens
- Slash commands
-- /cost: show token usage statistics
-- /export: export the current conversation to a file or clipboard
-- /mcp: manage mcp servers / list configured mcp tools
-- /model: set the AI model to use / choose what model and reasoning effort to use
-- /status: show current session configuration and token usage

-- /mcp__<server>__<prompt>: expose the <prompt> at <server>
-- /tools -- list tools

- codex skills prompt: https://github.com/openai/codex/blob/99f47d6e9a3546c14c43af99c7a58fa6bd130548/codex-rs/core/src/skills/render.rs#L19

- mcpclient/Client.WithSession: only Ping if session not used in longer than 250ms
- Read Claude desktop config file
- Read Claude code config file
- Read OpenAI config file (if possible)
- Read Gemini config file (if possible)

- mcp servers: at startup, load them in separate go routines and don't wait on them

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
	"os"
	"strings"

	"github.com/leftmike/gait/agent"
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

func newModel(provider, apiKey string, opts *model.Options) (model.Model, error) {
	switch provider {
	case "openai":
		return model.NewOpenAIModel(apiKey, opts)
	case "anthropic":
		return model.NewAnthropicModel(apiKey, opts)
	case "gemini":
		return model.NewGeminiModel(apiKey, opts)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

func interact(ag *agent.Agent, opts *model.Options) error {
	line := liner.NewLiner()
	defer line.Close()

	st := ag.Model().NewState()
	ag.SystemPrompt(st)

	ctx := context.Background()
	for {
		s, err := line.Prompt("> ")
		if err == io.EOF {
			fmt.Println()
			break
		} else if err != nil {
			return err
		}

		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "/") {
			err := slash(ag, st, s)
			if err == io.EOF {
				fmt.Println()
				break
			} else if err != nil {
				fmt.Printf("%s: %s\n", os.Args[0], err)
				os.Exit(1)
			}

			continue
		}

		st.Prompt(s)
		n := st.Len()

		err = ag.Generate(ctx, st, opts)
		if err != nil {
			return err
		}

		for n < st.Len() {
			step := st.Step(n)
			n += 1

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
		}
	}

	return nil
}

func main() {
	fs := flag.NewFlagSet("gait", flag.ExitOnError)

	var thinking bool
	fs.BoolVar(&verbose, "verbose", false, "verbose output")
	fs.BoolVar(&verbose, "v", false, "verbose output")
	fs.BoolVar(&trace, "trace", false, "trace model interaction")
	fs.BoolVar(&trace, "t", false, "trace model interaction")
	fs.BoolVar(&thinking, "thinking", true, "turn on and show `thinking`")

	provider, modelName, apiKey, cfg, err := config.Options(fs)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
		os.Exit(1)
	}

	opts := &model.Options{
		Verbose:  verbose,
		Trace:    trace,
		Thinking: thinking,
	}

	mdl, err := newModel(provider, apiKey, opts)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
		os.Exit(1)
	}

	if verbose {
		fmt.Println(provider, modelName)
	}

	ag := agent.NewAgent(provider, modelName, apiKey, mdl)
	defer ag.Close()

	for _, dir := range cfg.Skills {
		err := ag.AddSkill(dir)
		if err != nil && verbose {
			fmt.Printf("%s: %s\n", dir, err)
		}
	}

	ctx := context.Background()
	for _, svrCfg := range cfg.MCPServers {
		err := ag.AddServer(ctx, svrCfg, verbose)
		if verbose {
			if err != nil {
				fmt.Printf("mcp server %v failed: %s", svrCfg, err)
			} else {
				fmt.Printf("mcp server: %s\n", svrCfg.Name)
			}
		}
	}

	ag.AddTool("get_weather", "gets the current weather for the given city", getWeather,
		model.MustToolSchema[getWeatherArgs]())
	ag.AddTool("current_temperature", "gets the current temperature for the given location",
		currentTemperature, model.MustToolSchema[currentTemperatureArgs]())

	err = interact(ag, opts)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
	}
}
