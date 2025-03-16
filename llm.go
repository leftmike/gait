package main

import (
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/openai"
)

func NewModel(provider, model, apikey string, opts []llms.CallOption) (llms.Model, error) {
	switch provider {
	case "openai":
		return openai.New(openai.WithModel(model), openai.WithToken(apikey))
	case "anthropic":
		return anthropic.New(anthropic.WithModel(model), anthropic.WithToken(apikey))
	}

	return nil, fmt.Errorf("unknown provider: %s", provider)
}
