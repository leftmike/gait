package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/teilomillet/gollm"
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
	cfg, err := loadConfig(configFilenames())
	if err != nil {
		fatal("load config", err)
	}
	opts, provider, model, err := options(cfg)
	if err != nil {
		fatal("options", err)
	}

	llm, err := gollm.NewLLM(opts...)
	if err != nil {
		fatal("unable to create LLM", err)
	}

	if IsTerminal() {
		fmt.Printf("%s: %s\n", provider, model)
		err := Interact(llm, historyFilename())
		if err != nil {
			fatal("interact", err)
		}
	} else {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal("reading stdin", err)
		}

		s, err := llm.Generate(context.Background(), gollm.NewPrompt(string(buf)),
			gollm.WithFullResponse())
		if err != nil {
			fatal("llm generate", err)
		}

		fmt.Println(s)
	}
}
