package main

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/teilomillet/gollm"
)

func flagOption[T int | float64 | time.Duration](opts []gollm.ConfigOption, f T,
	c, p, m *T, setFn func(T) gollm.ConfigOption) []gollm.ConfigOption {

	if f != 0 {
		return append(opts, setFn(f))
	} else if m != nil {
		return append(opts, setFn(*m))
	} else if p != nil {
		return append(opts, setFn(*p))
	} else if c != nil {
		return append(opts, setFn(*c))
	}
	return opts
}

func options(cfg Config) ([]gollm.ConfigOption, string, string, error) {
	logFlag := flag.String("log", "off", "log level: off, error, warn, info, debug")
	providerFlag := flag.String("provider", "", "generate using this llm `provider`")
	apikeyFlag := flag.String("apikey", "", "`api key` to use")
	modelFlag := flag.String("model", "", "generate using this llm `model`")
	maxRetries := flag.Int("retries", 0, "maximum `number` of times to retry")
	maxTokens := flag.Int("tokens", 0, "maximum `tokens` to generate")
	memory := flag.Int("memory", 0, "maximum memory `tokens`")
	retryDelay := flag.Duration("delay", 0, "`time` between retries")
	temperature := flag.Float64("temperature", 0.0, "generate using this `temperature`")

	flag.Parse()

	var logLevel gollm.LogLevel
	switch *logFlag {
	case "off":
		logLevel = gollm.LogLevelOff
	case "error":
		logLevel = gollm.LogLevelError
	case "warn":
		logLevel = gollm.LogLevelWarn
	case "info":
		logLevel = gollm.LogLevelInfo
	case "debug":
		logLevel = gollm.LogLevelDebug
	default:
		return nil, "", "", errors.New("bad log level")
	}
	opts := append([]gollm.ConfigOption{}, gollm.SetDebugLevel(logLevel))

	provider := *providerFlag
	if provider == "" {
		if cfg.Provider == "" {
			return nil, "", "", errors.New("no provider specified")
		}

		provider = cfg.Provider
	}
	opts = append(opts, gollm.SetProvider(provider))
	p := cfg.FindProvider(provider)

	apikey := *apikeyFlag
	if apikey == "" {
		if p.APIKey == "" {
			return nil, "", "", fmt.Errorf("provider missing API key: %s", provider)
		}

		apikey = p.APIKey
	}
	opts = append(opts, gollm.SetAPIKey(apikey))

	model := *modelFlag
	if model == "" {
		if p.Model == "" {
			return nil, "", "", fmt.Errorf("no model specified: %s", provider)
		}

		model = p.Model
	}
	opts = append(opts, gollm.SetModel(model))
	m := p.FindModel(model)

	opts = flagOption(opts, *maxRetries, cfg.MaxRetries, p.MaxRetries, m.MaxRetries,
		gollm.SetMaxRetries)
	opts = flagOption(opts, *maxTokens, cfg.MaxTokens, p.MaxTokens, m.MaxTokens,
		gollm.SetMaxTokens)
	opts = flagOption(opts, *memory, cfg.Memory, p.Memory, m.Memory, gollm.SetMemory)
	opts = flagOption(opts, *retryDelay, cfg.RetryDelay, p.RetryDelay, m.RetryDelay,
		gollm.SetRetryDelay)
	opts = flagOption(opts, *temperature, cfg.Temperature, p.Temperature, m.Temperature,
		gollm.SetTemperature)

	return opts, provider, model, nil
}
