package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/leftmike/gait/model"
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

func (cfg *Config) findProvider(name string) *Provider {
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

func readConfig(filenames []string) (Config, error) {
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

var (
	verbose bool
)

func options() (string, string, string, *model.Options, error) {
	var configFilename string
	var noConfig bool
	var provider string
	var modelName string
	var apiKey string
	var summary bool

	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.StringVar(&configFilename, "config", "", "config filename")
	flag.BoolVar(&noConfig, "no-config", false, "do not load config")
	flag.StringVar(&provider, "provider", "", "generate using this llm `provider`")
	flag.StringVar(&modelName, "model", "", "generate using this llm `model`")
	flag.StringVar(&apiKey, "apikey", "", "`api key` to use")
	flag.BoolVar(&summary, "summary", false, "summarize reasoning")
	flag.Parse()

	if !noConfig {
		var filenames []string
		if configFilename != "" {
			filenames = []string{configFilename}
		} else {
			filenames = configFilenames()
		}
		cfg, err := readConfig(filenames)
		if err != nil {
			return "", "", "", nil, err
		}

		if provider == "" {
			provider = cfg.Provider
		}
		if modelName == "" {
			p := cfg.findProvider(provider)
			if p != nil {
				modelName = p.Model
			}
		}
		if apiKey == "" {
			p := cfg.findProvider(provider)
			if p != nil {
				apiKey = p.APIKey
			}
		}
	}

	return provider, modelName, apiKey, &model.Options{
		Verbose: verbose,
		Summary: summary,
	}, nil
}
