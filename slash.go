package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/leftmike/gait/agent"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
	"github.com/leftmike/gait/tool"
)

var (
	slashCommands = map[string]struct {
		cmd  string
		desc string
		fn   func(ag *agent.Agent, st model.State, args []string) error
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
		"/effort": {
			cmd:  "/effort",
			desc: "show or set the reasoning effort level",
			fn:   slashEffort,
		},
		"/exit": {cmd: "/exit", desc: "exit the REPL", fn: slashExit},
		"/help": {cmd: "/help", desc: "show help and available commands"},
		"/model": {
			cmd:  "/model",
			desc: "show or select the model to use",
			fn:   slashModel,
		},
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

func slash(ag *agent.Agent, st model.State, s string) error {
	cmd, args := parseSlash(s)
	if sc, ok := slashCommands[cmd]; ok {
		return sc.fn(ag, st, args)
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

func slashClear(ag *agent.Agent, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/clear: no arguments allowed: %s", args)
	}

	st.Clear()
	return nil
}

func slashExit(ag *agent.Agent, st model.State, args []string) error {
	return io.EOF
}

func slashHelp(ag *agent.Agent, st model.State, args []string) error {
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

func slashModels(ag *agent.Agent, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/models: no arguments allowed: %s", args)
	}

	models := ag.Client.ListModels()
	var ids []string
	for id := range models {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Printf("%-24s %-20s %-5s %-10s %-10s %s\n",
		"ID", "NAME", "REAS", "CONTEXT", "OUTPUT", "COST (IN/OUT per M)")
	for _, id := range ids {
		mmd := models[id]
		reas := "no"
		if mmd.Reasoning {
			reas = "yes"
		}
		fmt.Printf("%-24s %-20s %-5s %-10d %-10d $%.2f / $%.2f\n",
			id, mmd.Name, reas, mmd.ContextLimit, mmd.OutputLimit,
			mmd.InputCost, mmd.OutputCost)
	}

	return nil
}

func slashModel(ag *agent.Agent, st model.State, args []string) error {
	if len(args) == 0 {
		fmt.Printf("model: %s\n", ag.ModelConfig.Model)
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("/model: expected a single model id: %s", args)
	}

	mdlCfg := ag.ModelConfig
	mdlCfg.Model = args[0]
	mdl, err := ag.Client.NewModel(mdlCfg)
	if err != nil {
		return err
	}
	err = mdl.SetTools(tool.Tools{
		Tools:   ag.Tools,
		Sandbox: nil,
	})
	if err != nil {
		return err
	}

	ag.Model = mdl
	ag.ModelConfig = mdlCfg
	fmt.Printf("model: %s\n", args[0])
	return nil
}

func slashEffort(ag *agent.Agent, st model.State, args []string) error {
	if len(args) == 0 {
		if ag.ModelConfig.Effort == "" {
			fmt.Println("effort: (default)")
		} else {
			fmt.Printf("effort: %s\n", ag.ModelConfig.Effort)
		}
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("/effort: expected a single effort level: %s", args)
	}

	mdlCfg := ag.ModelConfig
	mdlCfg.Effort = args[0]
	mdl, err := ag.Client.NewModel(mdlCfg)
	if err != nil {
		return err
	}
	err = mdl.SetTools(tool.Tools{
		Tools:   ag.Tools,
		Sandbox: nil,
	})
	if err != nil {
		return err
	}

	ag.Model = mdl
	ag.ModelConfig = mdlCfg
	fmt.Printf("effort: %s\n", args[0])
	return nil
}

func slashSkills(ag *agent.Agent, st model.State, args []string) error {
	if len(args) == 0 {
		for _, sk := range ag.Skills {
			fmt.Println(sk.Name)
		}
	} else {
		for i, arg := range args {
			if i > 0 {
				fmt.Println()
			}

			sk := skill.FindSkill(ag.Skills, arg)
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

func slashCost(ag *agent.Agent, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/cost: no arguments allowed: %s", args)
	}

	inputTokens, outputTokens, _ := st.Usage()
	inputCost, outputCost := st.Cost()
	fmt.Printf("input tokens:  %d ($%.4f)\n", inputTokens, inputCost)
	fmt.Printf("output tokens: %d ($%.4f)\n", outputTokens, outputCost)
	fmt.Printf("total cost:    $%.4f\n", inputCost+outputCost)

	return nil
}

func slashContext(ag *agent.Agent, st model.State, args []string) error {

	if len(args) > 0 {
		return fmt.Errorf("/context: no arguments allowed: %s", args)
	}

	printContext(ag.Model, st)
	return nil
}

func printContext(mdl model.Model, st model.State) {
	_, _, contextTokens := st.Usage()

	limit := mdl.ContextLimit()
	if limit > 0 {
		pct := float64(contextTokens) / float64(limit) * 100
		fmt.Printf("context: %d / %d tokens (%.1f%%)\n", contextTokens, limit, pct)
	} else {
		fmt.Printf("context: %d tokens\n", contextTokens)
	}
}

func slashStatus(ag *agent.Agent, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/status: no arguments allowed: %s", args)
	}

	fmt.Printf("provider: %s\n", ag.Client.Provider())
	fmt.Printf("model:    %s\n", ag.ModelConfig.Model)
	if ag.ModelConfig.Effort != "" {
		fmt.Printf("effort:   %s\n", ag.ModelConfig.Effort)
	}
	fmt.Printf("thoughts: %t\n", ag.ModelConfig.IncludeThoughts)

	inputTokens, outputTokens, _ := st.Usage()
	inputCost, outputCost := st.Cost()
	fmt.Printf("input tokens:  %d ($%.4f)\n", inputTokens, inputCost)
	fmt.Printf("output tokens: %d ($%.4f)\n", outputTokens, outputCost)
	printContext(ag.Model, st)

	return nil
}
