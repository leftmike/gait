package main

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/tmc/langchaingo/llms"
)

func flagOption[T int | float64 | time.Duration](opts []llms.CallOption, f T,
	c, p, m *T, withFn func(T) llms.CallOption) []llms.CallOption {

	if f != 0 {
		return append(opts, withFn(f))
	} else if m != nil {
		return append(opts, withFn(*m))
	} else if p != nil {
		return append(opts, withFn(*p))
	} else if c != nil {
		return append(opts, withFn(*c))
	}
	return opts
}

func options(cfg Config) (string, string, string, []llms.CallOption, error) {
	providerFlag := flag.String("provider", "", "generate using this llm `provider`")
	modelFlag := flag.String("model", "", "generate using this llm `model`")
	apikeyFlag := flag.String("apikey", "", "`api key` to use")
	maxTokens := flag.Int("tokens", 0, "maximum `tokens` to generate")
	temperature := flag.Float64("temperature", 0.0, "generate using this `temperature`")

	flag.Parse()

	provider := *providerFlag
	if provider == "" {
		if cfg.Provider == "" {
			return "", "", "", nil, errors.New("no provider specified")
		}

		provider = cfg.Provider
	}
	p := cfg.FindProvider(provider)

	apikey := *apikeyFlag
	if apikey == "" {
		if p.APIKey == "" {
			return "", "", "", nil, fmt.Errorf("provider missing API key: %s", provider)
		}

		apikey = p.APIKey
	}

	model := *modelFlag
	if model == "" {
		if p.Model == "" {
			return "", "", "", nil, fmt.Errorf("no model specified: %s", provider)
		}

		model = p.Model
	}
	m := p.FindModel(model)

	var opts []llms.CallOption
	opts = flagOption(opts, *maxTokens, cfg.MaxTokens, p.MaxTokens, m.MaxTokens,
		llms.WithMaxTokens)
	opts = flagOption(opts, *temperature, cfg.Temperature, p.Temperature, m.Temperature,
		llms.WithTemperature)

	return provider, model, apikey, opts, nil
}
