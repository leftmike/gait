package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/leftmike/gait/agent"
	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
)

var (
	slashCommands = map[string]struct {
		cmd  string
		desc string
		fn   func(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State, args []string) error
	}{
		"/clear": {
			cmd:  "/clear",
			desc: "clear conversation history and free up context",
			fn:   slashClear,
		},
		"/context": {
			cmd:  "/context",
			desc: "show usage of the current context window",
			fn:   slashContext,
		},
		"/cost": {
			cmd:  "/cost",
			desc: "show token usage statistics for the session",
			fn:   slashCost,
		},
		"/exit":   {cmd: "/exit", desc: "exit the REPL", fn: slashExit},
		"/help":   {cmd: "/help", desc: "show help and available commands"},
		"/models": {cmd: "/models", desc: "list available models", fn: slashModels},
		"/quit":   {fn: slashExit}, // alias for /exit
		"/skills": {
			cmd:  "/skills",
			desc: "list available skills or show skill details",
			fn:   slashSkills,
		},
		"/status": {
			cmd:  "/status",
			desc: "show current session configuration and token usage",
			fn:   slashStatus,
		},
	}
)

func init() {
	// Eliminate circular dependency
	sc := slashCommands["/help"]
	sc.fn = slashHelp
	slashCommands["/help"] = sc
}

func slash(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State, s string) error {
	cmd, args := parseSlash(s)
	if sc, ok := slashCommands[cmd]; ok {
		return sc.fn(ag, mdlCfg, st, args)
	}

	fmt.Print("commands:")
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

func slashClear(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/clear: no arguments allowed: %s", args)
	}

	st.Clear()
	return nil
}

func slashExit(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State, args []string) error {
	return io.EOF
}

func slashHelp(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State, args []string) error {
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

func slashModels(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/models: no arguments allowed: %s", args)
	}

	models := ag.Client().ListModels()
	var ids []string
	for id := range models {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Printf("%-24s %-20s %-5s %-10s %-10s %s\n",
		"ID", "NAME", "REAS", "CONTEXT", "OUTPUT", "COST (IN/OUT per M)")
	for _, id := range ids {
		mdl := models[id]
		reas := "no"
		if mdl.Reasoning {
			reas = "yes"
		}
		fmt.Printf("%-24s %-20s %-5s %-10d %-10d $%.2f / $%.2f\n",
			id, mdl.Name, reas, mdl.ContextLimit, mdl.OutputLimit,
			mdl.InputCost, mdl.OutputCost)
	}

	return nil
}

func slashSkills(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State, args []string) error {
	if len(args) == 0 {
		for _, sk := range ag.Skills() {
			fmt.Println(sk.Name)
		}
	} else {
		for i, arg := range args {
			if i > 0 {
				fmt.Println()
			}

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
		}
	}

	return nil
}

func slashCost(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/cost: no arguments allowed: %s", args)
	}

	inputTokens, outputTokens, _ := st.Usage()
	fmt.Printf("input tokens:  %d\n", inputTokens)
	fmt.Printf("output tokens: %d\n", outputTokens)

	return nil
}

func slashContext(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State,
	args []string) error {

	if len(args) > 0 {
		return fmt.Errorf("/context: no arguments allowed: %s", args)
	}

	printContext(ag, mdlCfg, st)
	return nil
}

func printContext(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State) {
	/*
		u := st.Usage()
		window := model.ContextWindow(ag.Provider(), mdlCfg.Model)
		pct := 0.0
		if window > 0 {
			pct = float64(u.ContextTokens) / float64(window) * 100
		}
		fmt.Printf("context: %d / %d tokens (%.1f%%)\n", u.ContextTokens, window, pct)
	*/
}

func slashStatus(ag *agent.Agent, mdlCfg config.ModelConfig, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/status: no arguments allowed: %s", args)
	}

	fmt.Printf("provider: %s\n", ag.Provider())
	fmt.Printf("model:    %s\n", mdlCfg.Model)
	if mdlCfg.Effort != "" {
		fmt.Printf("effort:   %s\n", mdlCfg.Effort)
	}
	fmt.Printf("thoughts: %t\n", mdlCfg.IncludeThoughts)

	inputTokens, outputTokens, _ := st.Usage()
	fmt.Printf("input tokens:  %d\n", inputTokens)
	fmt.Printf("output tokens: %d\n", outputTokens)
	printContext(ag, mdlCfg, st)

	return nil
}
