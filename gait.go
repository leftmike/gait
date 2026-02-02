/*
To Do:
- MaxOutputTokens
- Slash commands
-- /clear -- clear the context window
-- /tools -- list tools
-- /mcp -- list mcp server including type and status

- mcpclient/Client.WithSession: only Ping if session not used in longer than 250ms
- Read Claude desktop config file
- Read Claude code config file
- Read OpenAI config file (if possible)
- Read Gemini config file (if possible)

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
	"strings"
	"time"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/mcpclient"
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

func slashList(provider, apiKey string) {
	var infos []model.ModelInfo
	var err error

	ctx := context.Background()
	switch provider {
	case "openai":
		infos, err = model.ListOpenAIModels(ctx, apiKey)
	case "anthropic":
		infos, err = model.ListAnthropicModels(ctx, apiKey)
	case "gemini":
		infos, err = model.ListGeminiModels(ctx, apiKey)
	default:
		panic(fmt.Sprintf("unknown provider: %s", provider))
	}

	if err != nil {
		fmt.Printf("list models: %s: %s\n", provider, err)
		return
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
}

func parseSlash(s string) (string, []string) {
	args := strings.Split(s, " ")
	i := 0
	for _, arg := range args {
		if arg != "" {
			args[i] = arg
			i += 1
		}
	}

	return args[0], args[1:i]
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

	provider, modelName, apiKey, cfg, err := config.Options(fs)
	if err != nil {
		log.Fatalln(err)
	}

	ctx := context.Background()
	var clnts []*mcpclient.Client
	for _, svrCfg := range cfg.MCPServers {
		clnt, err := mcpclient.NewClient(ctx, svrCfg, verbose)
		if err != nil {
			if verbose {
				fmt.Printf("mcp server %v failed: %s", svrCfg, err)
			}
			continue
		}

		if verbose {
			fmt.Printf("mcp server: %s\n", svrCfg.Name)
		}

		clnts = append(clnts, clnt)
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

	for _, clnt := range clnts {
		tools, err = clnt.AddTools(tools)
		if err != nil {
			log.Fatalf("%s: %s\n", clnt.Name(), err)
		}
	}

	opts := &model.Options{
		Verbose:  verbose,
		Trace:    trace,
		Thinking: thinking,
	}

	mdl, err := newModel(provider, modelName, apiKey, opts)
	if err != nil {
		log.Fatalln(err)
	}

	if verbose {
		fmt.Println(provider, modelName)
	}

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

		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "/") {
			cmd, args := parseSlash(s)
			switch cmd {
			case "/exit", "/quit":
				return

			case "/list":
				if len(args) == 0 {
					slashList(provider, apiKey)
				} else {
					fmt.Println("/list has no arguments")
				}

			default:
				fmt.Println("slash command must be /exit or /list")
			}

			continue
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
