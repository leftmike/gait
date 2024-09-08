package main

import (
	"context"
	"fmt"
	"log"
	"runtime"

	"github.com/teilomillet/gollm"
)

func configFilenames() []string {
	if runtime.GOOS == "windows" {
		return []string{"~/gait/gait.hcl", "~/gait.hcl", "./gait.hcl"}
	}

	return []string{"~/.gait/gait.hcl", "~/.gait.hcl", "./gait.hcl"}
}

func main() {
	cfg, err := loadConfig(configFilenames())
	if err != nil {
		log.Fatalf("load config: %s", err)
	}
	opts, provider, model, err := options(cfg)
	if err != nil {
		log.Fatalf("options: %s", err)
	}
	fmt.Printf("%s: %s\n", provider, model)

	llm, err := gollm.NewLLM(opts...)
	if err != nil {
		log.Fatalf("Failed to create LLM: %v", err)
	}

	ctx := context.Background()

	prompt := gollm.NewPrompt("Tell me a short joke about programming.")
	response, err := llm.Generate(ctx, prompt)
	if err != nil {
		log.Fatalf("Failed to generate text: %v", err)
	}
	fmt.Printf("Response: %s\n", response)
}
