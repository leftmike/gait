package main

import (
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/bedrock"
	"github.com/tmc/langchaingo/llms/openai"
)

func NewModel(provider, model, apikey string, opts []llms.CallOption) (llms.Model, error) {
	switch provider {
	case "openai":
		return openai.New(openai.WithModel(model), openai.WithToken(apikey))
	case "anthropic":
		return anthropic.New(anthropic.WithModel(model), anthropic.WithToken(apikey))
	case "bedrock":
		// For Bedrock, AWS credentials are typically loaded from environment variables
		// or AWS config files, so we don't need to pass an API key
		return bedrock.New(bedrock.WithModel(model))
	}

	return nil, fmt.Errorf("unknown provider: %s", provider)
}
