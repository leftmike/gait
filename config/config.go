package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Provider struct {
	Name    string `hcl:"name,label"`
	Model   string `hcl:"model,optional"`
	APIKey  string `hcl:"api_key,optional"`
	Summary *bool  `hcl:"summary,optional"`
}

type Config struct {
	Provider  string     `hcl:"provider,optional"`
	Providers []Provider `hcl:"provider,block"`
}

func (cfg *Config) FindProvider(name string) *Provider {
	for _, provider := range cfg.Providers {
		if strings.EqualFold(name, provider.Name) {
			return &provider
		}
	}

	return nil
}

func configFilenames() []string {
	if runtime.GOOS == "windows" {
		return []string{"~/gait/gait.hcl", "~/gait.hcl", "./gait.hcl"}
	}

	return []string{"~/.gait/gait.hcl", "~/.gait.hcl", "./gait.hcl"}
}

func ReadConfig(filenames []string) (Config, error) {
	for _, name := range filenames {
		buf, err := os.ReadFile(name)
		if err == nil {
			var cfg Config
			err := hclsimple.Decode(name, buf, nil, &cfg)
			return cfg, err
		}
	}

	return Config{}, fmt.Errorf("config file not found: %v", filenames)
}

func Options(fs *flag.FlagSet) (string, string, string, error) {
	var configFilename string
	var noConfig bool
	var useOpenAI bool
	var useAnthropic bool
	var useGemini bool
	var modelName string
	var apiKey string

	fs.StringVar(&configFilename, "config", "", "config filename")
	fs.BoolVar(&noConfig, "no-config", false, "do not load config")
	fs.BoolVar(&useOpenAI, "openai", false, "use openai")
	fs.BoolVar(&useAnthropic, "anthropic", false, "use anthropic")
	fs.BoolVar(&useGemini, "gemini", false, "use gemini")
	fs.StringVar(&modelName, "model", "", "generate using this model `model`")
	fs.StringVar(&apiKey, "apikey", "", "`api key` to use")
	fs.Parse(os.Args[1:])

	var provider string
	if useOpenAI {
		provider = "openai"
	}
	if useAnthropic {
		if provider != "" {
			return "", "", "", errors.New("multiple providers specified")
		}
		provider = "anthropic"
	}
	if useGemini {
		if provider != "" {
			return "", "", "", errors.New("multiple providers specified")
		}
		provider = "gemini"
	}

	if !noConfig {
		var filenames []string
		if configFilename != "" {
			filenames = []string{configFilename}
		} else {
			filenames = configFilenames()
		}
		cfg, err := ReadConfig(filenames)
		if err != nil {
			return "", "", "", err
		}

		if provider == "" {
			provider = cfg.Provider
		}
		if modelName == "" {
			p := cfg.FindProvider(provider)
			if p != nil {
				modelName = p.Model
			}
		}
		if apiKey == "" {
			p := cfg.FindProvider(provider)
			if p != nil {
				apiKey = p.APIKey
			}
		}
	}

	return provider, modelName, apiKey, nil
}
