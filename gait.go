package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/leftmike/gait/config"
	"github.com/tmc/langchaingo/llms"
)

func configFilenames() []string {
	if runtime.GOOS == "windows" {
		return []string{"~/gait/gait.hcl", "~/gait.hcl", "./gait.hcl"}
	}

	return []string{"~/.gait/gait.hcl", "~/.gait.hcl", "./gait.hcl"}
}

func historyFilename() string {
	if runtime.GOOS == "windows" {
		return "gait.history"
	}
	return ".gait_history"
}

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %s: %s\n", os.Args[0], msg, err)
	os.Exit(1)
}

func main() {
	cfg, err := config.Load(configFilenames())
	if err != nil {
		fatal("load config", err)
	}
	provider, model, apikey, opts, err := config.Options(cfg)
	if err != nil {
		fatal("options", err)
	}

	llm, err := NewModel(provider, model, apikey, opts)
	if err != nil {
		fatal("unable to create model", err)
	}

	if IsTerminal() {
		if config.Verbose {
			fmt.Printf("%s: %s\n", provider, model)
		}

		err := Interact(llm, opts, historyFilename())
		if err != nil {
			fatal("interact", err)
		}
	} else {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal("reading stdin", err)
		}

		s, err := llms.GenerateFromSinglePrompt(context.Background(), llm, string(buf), opts...)
		if err != nil {
			fatal("llm generate", err)
		}

		fmt.Println(s)
	}
}
