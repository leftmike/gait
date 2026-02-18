package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/leftmike/gait/agent"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
)

var (
	slashCommands = map[string]struct {
		cmd  string
		desc string
		fn   func(ag *agent.Agent, args []string) error
	}{
		"/exit":   {cmd: "/exit", desc: "exit the REPL", fn: slashExit},
		"/help":   {cmd: "/help", desc: "show help and available commands"},
		"/models": {cmd: "/models", desc: "list available models", fn: slashModels},
		"/quit":   {fn: slashExit}, // alias for /exit
		"/skills": {
			cmd:  "/skills",
			desc: "list available skills or show skill details",
			fn:   slashSkills,
		},
	}
)

func init() {
	// Eliminate circular dependency
	sc := slashCommands["/help"]
	sc.fn = slashHelp
	slashCommands["/help"] = sc
}

func slash(ag *agent.Agent, s string) error {
	cmd, args := parseSlash(s)
	if sc, ok := slashCommands[cmd]; ok {
		return sc.fn(ag, args)
	}

	fmt.Print("commands: ")
	for _, sc := range slashCommands {
		if sc.cmd != "" {
			fmt.Printf(" %s", sc.cmd)
		}
	}
	fmt.Println()

	return nil
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

func slashExit(ag *agent.Agent, args []string) error {
	return io.EOF
}

func slashHelp(ag *agent.Agent, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/help: no arguments allowed: %s", args)
	}

	for _, sc := range slashCommands {
		if sc.cmd == "" {
			continue
		}
		fmt.Printf("%s: %s\n", sc.cmd, sc.desc)
	}

	return nil
}

func slashModels(ag *agent.Agent, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/models: no arguments allowed: %s", args)
	}

	var infos []model.ModelInfo
	var err error

	ctx := context.Background()
	switch ag.Provider() {
	case "openai":
		infos, err = model.ListOpenAIModels(ctx, ag.APIKey())
	case "anthropic":
		infos, err = model.ListAnthropicModels(ctx, ag.APIKey())
	case "gemini":
		infos, err = model.ListGeminiModels(ctx, ag.APIKey())
	default:
		panic(fmt.Sprintf("unknown provider: %s", ag.Provider()))
	}

	if err != nil {
		return fmt.Errorf("list models for %s: %s\n", ag.Provider(), err)
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

	return nil
}

func slashSkills(ag *agent.Agent, args []string) error {
	if len(args) == 0 {
		for _, sk := range ag.Skills() {
			fmt.Println(sk.Name)
		}
	} else {
		for _, arg := range args {
			sk := skill.FindSkill(ag.Skills(), arg)
			if sk == nil {
				return fmt.Errorf("skill not found: %s", arg)
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

	return nil
}
