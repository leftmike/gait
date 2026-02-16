/*
To Do:
- MaxOutputTokens
- Slash commands
-- /clear: clear conversation history and free up context
-- /cost: show token usage statistics
-- /export: export the current conversation to a file or clipboard
-- /mcp: manage mcp servers / list configured mcp tools
-- /model: set the AI model to use / choose what model and reasoning effort to use
-- /status: show current session configuration and token usage

-- /mcp__<server>__<prompt>: expose the <prompt> at <server>
-- /tools -- list tools

- leverage filesys for the agent reading skill files

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
	"log"
	"strings"
	"time"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/filesys"
	"github.com/leftmike/gait/mcpclient"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"

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

func slashHelp(args []string) {
	if len(args) > 0 {
		fmt.Println("/help: no arguments allowed")
		return
	}

	fmt.Print(`/exit (quit): exit the REPL
/help: show help and available commands
/models: list available models
/skills: list available skills or show skill details
`)
}

func slashModels(args []string, provider, apiKey string) {
	if len(args) > 0 {
		fmt.Println("/models: no arguments allowed")
		return
	}

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

func slashSkills(args []string, skills []*skill.Skill) {
	if len(args) == 0 {
		for _, sk := range skills {
			fmt.Println(sk.Name)
		}
	} else {
		for _, arg := range args {
			sk := skill.FindSkill(skills, arg)
			if sk == nil {
				fmt.Printf("skill not found: %s\n\n", arg)
				continue
			}

			fmt.Printf("name: %s\n", sk.Name)
			fmt.Printf("description: %s\n", sk.Description)
			fmt.Printf("directory: %s\n", sk.Dir)
			if sk.License != "" {
				fmt.Printf("license: %s\n", sk.License)
			}
			if sk.Compatibility != "" {
				fmt.Printf("compatibility: %s\n", sk.Compatibility)
			}
			if sk.AllowedTools != "" {
				fmt.Printf("allowed tools: %s\n", sk.AllowedTools)
			}
			if len(sk.Metadata) > 0 {
				fmt.Println("metadata:")
				for k, v := range sk.Metadata {
					fmt.Printf("    %s: %s\n", k, v)
				}
			}
			fmt.Println()
		}
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

type readFileArgs struct {
	Path string `json:"path" gait:"the path of the file to read"`
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

	var skills []*skill.Skill
	for _, dir := range cfg.Skills {
		dirSkills, err := skill.ReadDir(dir)
		if err != nil {
			if verbose {
				fmt.Printf("%s: %s\n", dir, err)
			}
		} else {
			skills = append(skills, dirSkills...)
		}
	}

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

	ffs := filesys.NewForestFS()
	defer ffs.Close()

	readFile := func(ctx context.Context, buf []byte) (string, error) {
		var args readFileArgs
		err := json.Unmarshal(buf, &args)
		if err != nil {
			return "", err
		}

		buf, err = ffs.ReadFile(args.Path)
		if err != nil {
			return "", err
		}
		return string(buf), nil
	}

	tools := model.Tools{
		{
			Name:        "get_weather",
			Description: "gets the current weather for the given city",
			Func:        getWeather,
			Schema:      model.MustToolSchema[getWeatherArgs](),
		},
		{
			Name:        "current_temperature",
			Description: "gets the current temperature for the given location",
			Func:        currentTemperature,
			Schema:      model.MustToolSchema[currentTemperatureArgs](),
		},
		{
			Name:        "read_file",
			Description: "reads the contents of a file",
			Func:        readFile,
			Schema:      model.MustToolSchema[readFileArgs](),
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

	if len(skills) > 0 {
		st.SystemPrompt(skill.SystemPrompt(skills))
	}

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
			case "/help":
				slashHelp(args)
			case "/models":
				slashModels(args, provider, apiKey)
			case "/skills":
				slashSkills(args, skills)
			default:
				fmt.Println("slash command must be /exit, /help, /models, or /skills")
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
}
