package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/leftmike/gait/agent"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
)

var (
	slashCommands = map[string]struct {
		cmd  string
		desc string
		fn   func(agnt *agent.Agent, st model.State, args []string) error
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
			cmd:  "/effort [level]",
			desc: "show or set the reasoning effort level",
			fn:   slashEffort,
		},
		"/exit": {cmd: "/exit", desc: "exit the REPL", fn: slashExit},
		"/help": {cmd: "/help", desc: "show help and available commands"},
		"/model": {
			cmd:  "/model [model]",
			desc: "show or select the model to use",
			fn:   slashModel,
		},
		"/models": {
			cmd:  "/models [order]",
			desc: "list available models, optionally order by id, name, or cost",
			fn:   slashModels,
		},
		"/quit": {fn: slashExit}, // alias for /exit
		"/skills": {
			cmd:  "/skills [skill ...]",
			desc: "list available skills or show skill details",
			fn:   slashSkills,
		},
		"/status": {
			cmd:  "/status",
			desc: "show current session configuration and token usage",
			fn:   slashStatus,
		},
	}

	sortedCommands []string
)

func init() {
	// Eliminate circular dependency
	sc := slashCommands["/help"]
	sc.fn = slashHelp
	slashCommands["/help"] = sc

	for cmd, sc := range slashCommands {
		if sc.cmd != "" {
			sortedCommands = append(sortedCommands, cmd)
		}
	}
	slices.Sort(sortedCommands)
}

func slash(agnt *agent.Agent, st model.State, s string) error {
	cmd, args := parseSlash(s)
	if sc, ok := slashCommands[cmd]; ok {
		return sc.fn(agnt, st, args)
	}

	fmt.Printf("commands: %s\n", strings.Join(sortedCommands, " "))
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

func slashClear(agnt *agent.Agent, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/clear: no arguments allowed: %s", args)
	}

	st.Clear()
	return nil
}

func slashExit(agnt *agent.Agent, st model.State, args []string) error {
	return io.EOF
}

func slashHelp(agnt *agent.Agent, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/help: no arguments allowed: %s", args)
	}

	for _, cmd := range sortedCommands {
		sc := slashCommands[cmd]
		fmt.Printf("%s\n        %s\n", sc.cmd, sc.desc)
	}

	return nil
}

func sortModels(models map[string]model.ModelMetadata, orderBy string) []string {
	var mmds []*model.ModelMetadata
	for id := range models {
		mmd := models[id]
		mmds = append(mmds, &mmd)
	}
	slices.SortFunc(mmds, func(mmd1, mmd2 *model.ModelMetadata) int {
		switch orderBy {
		case "id":
			return strings.Compare(strings.ToLower(mmd1.Model), strings.ToLower(mmd2.Model))
		case "name":
			return strings.Compare(strings.ToLower(mmd1.Name), strings.ToLower(mmd2.Name))
		case "cost":
			if mmd1.InputCost < mmd2.InputCost {
				return -1
			} else if mmd1.InputCost > mmd2.InputCost {
				return 1
			} else if mmd1.OutputCost < mmd2.OutputCost {
				return -1
			} else if mmd1.OutputCost > mmd2.OutputCost {
				return 1
			}

			return strings.Compare(strings.ToLower(mmd1.Model), strings.ToLower(mmd2.Model))
		default:
			panic(fmt.Sprintf("unexpected order by: %s", orderBy))
		}
	})
	var ids []string
	for _, mmd := range mmds {
		ids = append(ids, mmd.Model)
	}

	return ids
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}

	return s[:max]
}

func slashModels(agnt *agent.Agent, st model.State, args []string) error {
	orderBy := "id"
	if len(args) == 1 {
		if !slices.Contains([]string{"id", "name", "cost"}, args[0]) {
			return fmt.Errorf("/models: order by must be id, name, or cost: %s", args[0])
		}
		orderBy = args[0]
	} else if len(args) > 0 {
		return fmt.Errorf("/models: only one optional argument allowed: %s", args)
	}

	fmt.Printf("%-24s %-20s %-5s %-10s %-10s %s\n",
		"ID", "NAME", "REAS", "CONTEXT", "OUTPUT", "COST (IN/OUT per M)")

	models := agnt.Client.ListModels()
	for _, id := range sortModels(models, orderBy) {
		mmd := models[id]
		if len(id) > 24 {
			fmt.Printf("%-44s  ", truncate(id, 44))
		} else {
			fmt.Printf("%-24s %-20s ", id, truncate(mmd.Name, 20))
		}

		reas := "no"
		if mmd.Reasoning {
			reas = "yes"
		}
		fmt.Printf("%-5s %-10d %-10d $%.2f / $%.2f\n", reas, mmd.ContextLimit, mmd.OutputLimit,
			mmd.InputCost, mmd.OutputCost)
	}

	return nil
}

func slashModel(agnt *agent.Agent, st model.State, args []string) error {
	if len(args) == 0 {
		fmt.Printf("model: %s\n", agnt.AgentConfig.ModelConfig.Model)
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("/model: expected a single model id: %s", args)
	}

	mdlCfg := agnt.AgentConfig.ModelConfig
	mdlCfg.Model = args[0]
	mdl, err := agnt.Client.NewModel(mdlCfg)
	if err != nil {
		return err
	}
	err = mdl.SetTools(agnt.Tools, agnt.Sandbox)
	if err != nil {
		return err
	}

	agnt.Model = mdl
	agnt.AgentConfig.ModelConfig = mdlCfg
	fmt.Printf("model: %s\n", args[0])
	return nil
}

func slashEffort(agnt *agent.Agent, st model.State, args []string) error {
	if len(args) == 0 {
		if agnt.AgentConfig.ModelConfig.Effort == "" {
			fmt.Println("effort: (default)")
		} else {
			fmt.Printf("effort: %s\n", agnt.AgentConfig.ModelConfig.Effort)
		}
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("/effort: expected a single effort level: %s", args)
	}

	mdlCfg := agnt.AgentConfig.ModelConfig
	mdlCfg.Effort = args[0]
	mdl, err := agnt.Client.NewModel(mdlCfg)
	if err != nil {
		return err
	}
	err = mdl.SetTools(agnt.Tools, agnt.Sandbox)
	if err != nil {
		return err
	}

	agnt.Model = mdl
	agnt.AgentConfig.ModelConfig = mdlCfg
	fmt.Printf("effort: %s\n", args[0])
	return nil
}

func slashSkills(agnt *agent.Agent, st model.State, args []string) error {
	if len(args) == 0 {
		for _, sk := range agnt.Skills {
			fmt.Println(sk.Name)
		}
	} else {
		for i, arg := range args {
			if i > 0 {
				fmt.Println()
			}

			sk := skill.FindSkill(agnt.Skills, arg)
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

func slashCost(agnt *agent.Agent, st model.State, args []string) error {
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

func slashContext(agnt *agent.Agent, st model.State, args []string) error {

	if len(args) > 0 {
		return fmt.Errorf("/context: no arguments allowed: %s", args)
	}

	printContext(agnt.Model, st)
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

func slashStatus(agnt *agent.Agent, st model.State, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("/status: no arguments allowed: %s", args)
	}

	fmt.Printf("provider: %s\n", agnt.Client.Provider())
	fmt.Printf("model:    %s\n", agnt.AgentConfig.ModelConfig.Model)
	if agnt.AgentConfig.ModelConfig.Effort != "" {
		fmt.Printf("effort:   %s\n", agnt.AgentConfig.ModelConfig.Effort)
	}
	fmt.Printf("thoughts: %t\n", agnt.AgentConfig.ModelConfig.IncludeThoughts)

	inputTokens, outputTokens, _ := st.Usage()
	inputCost, outputCost := st.Cost()
	fmt.Printf("input tokens:  %d ($%.4f)\n", inputTokens, inputCost)
	fmt.Printf("output tokens: %d ($%.4f)\n", outputTokens, outputCost)
	printContext(agnt.Model, st)

	return nil
}
