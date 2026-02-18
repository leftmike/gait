package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leftmike/gait/agent"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
)

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

func slash(ag *agent.Agent, s string) {
	cmd, args := parseSlash(s)
	switch cmd {
	case "/exit", "/quit":
		return
	case "/help":
		slashHelp(args)
	case "/models":
		slashModels(args, ag.Provider(), ag.APIKey())
	case "/skills":
		slashSkills(args, ag)
	default:
		fmt.Println("slash command must be /exit, /help, /models, or /skills")
	}
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

func slashSkills(args []string, ag *agent.Agent) {
	if len(args) == 0 {
		for _, sk := range ag.Skills() {
			fmt.Println(sk.Name)
		}
	} else {
		for _, arg := range args {
			sk := skill.FindSkill(ag.Skills(), arg)
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
