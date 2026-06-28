/*
To Do:
- Config: Thoughts and Effort
- MaxOutputTokens

- Slash commands
-- /context: show usage of the current context
-- /cost: show token usage statistics
-- /export: export the current conversation to a file or clipboard
-- /mcp: manage mcp servers / list configured mcp tools
-- /model: set the AI model to use / choose what model and reasoning effort to use
-- /status: show current session configuration and token usage
-- /mcp__<server>__<prompt>: expose the <prompt> at <server>
-- /tools -- list tools

- web_fetch: get user confirmation / config of domains / urls to fetch
- glob: support ** syntax etc
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
- Read Gemini config file (if possible)

- mcp servers: at startup, load them in separate go routines and don't wait on them

- Gemini
-- Seed in GenerateContentConfig

- Anthropic
-- StopReason max_tokens

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
	"slices"
	"strings"

	"github.com/peterh/liner"

	"github.com/leftmike/gait/agent"
	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/util"
)

var (
	verbose bool
	trace   bool
)

func newModel(provider *config.Provider) (model.Model, error) {
	switch provider.Name {
	case "openai":
		return model.NewOpenAIModel(provider.APIKey)
	case "anthropic":
		return model.NewAnthropicModel(provider.APIKey)
	case "gemini":
		return model.NewGeminiModel(provider.APIKey)
	case "ollama":
		return model.NewOllamaModel(provider)
	case "llamacpp":
		return model.NewLlamaCppModel(provider)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider.Name)
	}
}

func interact(ag *agent.Agent, opts *config.Options) error {
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
				if opts.IncludeThoughts {
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

	opts, provider, cfg, err := config.ParseFlags(fs)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
		os.Exit(1)
	}

	opts.Verbose = verbose
	opts.Trace = trace

	mdl, err := newModel(provider)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
		os.Exit(1)
	}

	effort := opts.Effort
	if effort != "" && effort != "default" && !slices.Contains(mdl.EffortLevels(), effort) {
		fmt.Printf("%s: effort must be default, %s: %s\n", os.Args[0],
			strings.Join(mdl.EffortLevels(), ", "), effort)
		os.Exit(1)
	}

	if verbose {
		fmt.Println(provider.Name, opts.Model)
	}

	ag := agent.NewAgent(provider, mdl)

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

	ag.AddWebFetchTool()
	if cfg.BraveAPIKey != "" {
		ag.AddWebSearchTool(cfg.BraveAPIKey)
	}

	err = interact(ag, opts)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
	}
}
