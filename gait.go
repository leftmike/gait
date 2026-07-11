/*
To Do:
- Slash commands
-- /export: export the current conversation to a file or clipboard
-- /mcp: manage mcp servers / list configured mcp tools
-- /effort: set the level of effort
-- /mcp__<server>__<prompt>: expose the <prompt> at <server>
-- /tools -- list tools

- add support for open router

- web_fetch: get user confirmation / config of domains / urls to fetch
- claude code builtin tools: Bash, Edit, Write, Read, Glob, Grep, Agent, WebFetch, WebSearch,
  AskUserQuestion, ExitPlanMode
- remove brave search?

- tool search tool: https://www.anthropic.com/engineering/advanced-tool-use

- channels: https://code.claude.com/docs/en/channels-reference

- restricted sandbox for running cli programs

- codex skills prompt: https://github.com/openai/codex/blob/99f47d6e9a3546c14c43af99c7a58fa6bd130548/codex-rs/core/src/skills/render.rs#L19

- mcpclient/Client.WithSession: only Ping if session not used in longer than 250ms
- Read Claude desktop config file
- Read Claude code config file
- Read OpenAI config file (if possible)
- Read Google config file (if possible)

- mcp servers: at startup, load them in separate go routines and don't wait on them

- OpenAI Codex
-- API access via codex: https://simonwillison.net/2026/Apr/23/gpt-5-5/
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/peterh/liner"

	"github.com/leftmike/gait/agent"
	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/tool"
	"github.com/leftmike/gait/util"
)

var (
	verbose bool
	trace   bool
)

func interact(ag *agent.Agent, opts *model.Options) error {
	line := liner.NewLiner()
	defer line.Close()

	err := ag.Model.SetTools(ag.Tools)
	if err != nil {
		return err
	}

	st := ag.Client.NewState()
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
			}

			continue
		}

		st.Prompt(s)
		n := st.Len()

		err = ag.Model.Generate(ctx, st, opts)
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
				if ag.ModelConfig.IncludeThoughts {
					fmt.Printf("Thinking: [%s]\n", step.Content)
				}
			case model.ToolCallStep:
				if opts.Verbose {
					fmt.Printf("Tool Call: %s(%s)\n", step.Name, string(step.Input))
				}
			case model.ToolOutputStep:
				if opts.Verbose {
					fmt.Printf("Tool Output: %s: [%s]\n", step.Name,
						util.Lines(step.Content, 16, 160))
				}
			}
		}
	}

	return nil
}

func main() {
	fs := flag.NewFlagSet("gait", flag.ExitOnError)

	fs.BoolVar(&verbose, "verbose", false, "verbose output")
	fs.BoolVar(&verbose, "v", false, "verbose output")
	fs.BoolVar(&trace, "trace", false, "trace model interaction")
	fs.BoolVar(&trace, "t", false, "trace model interaction")

	mdlCfg, clntCfg, cfg, err := config.ParseFlags(fs)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
		os.Exit(1)
	}

	opts := &model.Options{
		Verbose: verbose,
		Trace:   trace,
	}

	clnt, err := model.NewClient(clntCfg)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
		os.Exit(1)
	}

	mdl, err := clnt.NewModel(mdlCfg)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
		os.Exit(1)
	}

	if verbose {
		fmt.Println(clntCfg.Provider, mdlCfg.Model)
	}

	var tools map[string]tool.Tool
	switch clntCfg.Provider {
	case "anthropic":
		tools = tool.Anthropic(cfg.BraveAPIKey)
	case "openai":
		tools = tool.OpenAI()
	default:
		tools = tool.All(cfg.BraveAPIKey)
	}

	ag := agent.Agent{
		Client:      clnt,
		Model:       mdl,
		ModelConfig: mdlCfg,
		Tools:       tools,
	}

	/*
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
	*/

	err = interact(&ag, opts)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
	}
}
