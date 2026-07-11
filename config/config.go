package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

type ModelConfig struct {
	Model           string
	IncludeThoughts bool
	Effort          string
	MaxTokens       int
}

type ClientConfig struct {
	Provider  string `hcl:"provider,label"`
	Model     string `hcl:"model,optional"`
	APIKey    string `hcl:"api_key,optional"`
	BaseURL   string `hcl:"base_url,optional"`
	Thoughts  bool   `hcl:"thoughts,optional"`
	Effort    string `hcl:"effort,optional"`
	MaxTokens int    `hcl:"max_tokens,optional"`
}

/*
"mcpServers": {
        "remote-http-files": {
          "type": "http",
          "url": "https://localhost:8443/mcp"
        },
        "remote-sse-files": {
          "type": "sse",
          "url": "https://localhost:8443/sse"
        }
      },
*/

type MCPServer struct {
	Name    string   `hcl:"name,label"`
	Type    string   `hcl:"type,optional"`    // stdio, http, sse
	Command string   `hcl:"command,optional"` // stdio
	Args    []string `hcl:"args,optional"`    // stdio
	URL     string   `hcl:"url,optional"`     // http, sse
	// Headers
}

type Config struct {
	Provider      string         `hcl:"provider,optional"`
	ClientConfigs []ClientConfig `hcl:"provider,block"`
	MCPServers    []MCPServer    `hcl:"mcpserver,block"`
	Skills        []string       `hcl:"skills,optional"`
	BraveAPIKey   string         `hcl:"brave_api_key,optional"`
}

func (cfg *Config) FindClientConfig(provider string) (ClientConfig, bool) {
	for _, clntCfg := range cfg.ClientConfigs {
		if strings.EqualFold(provider, clntCfg.Provider) {
			return clntCfg, true
		}
	}

	return ClientConfig{}, false
}

func configFilenames() []string {
	if runtime.GOOS == "windows" {
		return []string{"~/gait/gait.hcl", "~/gait.hcl", "./gait.hcl"}
	}

	return []string{"~/.gait/gait.hcl", "~/.gait.hcl", "./gait.hcl"}
}

func ReadConfig(filenames []string) (*Config, error) {
	for _, name := range filenames {
		buf, err := os.ReadFile(name)
		if err == nil {
			var cfg Config
			err := hclsimple.Decode(name, buf, nil, &cfg)
			return &cfg, err
		}
	}

	return nil, fmt.Errorf("config file not found: %v", filenames)
}

func ParseFlags(fs *flag.FlagSet) (ModelConfig, ClientConfig, *Config, error) {
	var configFilename string
	var noConfig bool
	var useOpenAI bool
	var useAnthropic bool
	var useGoogle bool
	var useOllama bool
	var useLlamaCpp bool
	var model string
	var apiKey string
	var baseURL string
	var thoughts, hasThoughts bool
	var effort string
	var maxTokens int
	var braveAPIKey string

	fs.StringVar(&configFilename, "config", "", "config filename")
	fs.BoolVar(&noConfig, "no-config", false, "do not load config")
	fs.BoolVar(&useOpenAI, "openai", false, "use openai")
	fs.BoolVar(&useAnthropic, "anthropic", false, "use anthropic")
	fs.BoolVar(&useGoogle, "google", false, "use google")
	fs.BoolVar(&useOllama, "ollama", false, "use ollama")
	fs.BoolVar(&useLlamaCpp, "llamacpp", false, "use llama.cpp")
	fs.StringVar(&model, "model", "", "generate using this model `model`")
	fs.StringVar(&apiKey, "apikey", "", "`api key` to use")
	fs.StringVar(&baseURL, "baseurl", "", "`base url` of the model server")
	fs.BoolFunc("thoughts", "include `thoughts`", func(s string) error {
		var err error
		thoughts, err = strconv.ParseBool(s)
		if err != nil {
			return err
		}
		hasThoughts = true
		return nil
	})
	fs.StringVar(&effort, "effort", "", "reasoning `effort`; depends on provider and model")
	fs.IntVar(&maxTokens, "max-tokens", 0, "maximum output `tokens`")
	fs.StringVar(&braveAPIKey, "brave-apikey", "", "`api key` for Brave Search")
	fs.Parse(os.Args[1:])

	if maxTokens < 0 {
		return ModelConfig{}, ClientConfig{}, nil, fmt.Errorf("max-tokens must be positive")
	}

	providers := map[string]bool{
		"openai":    useOpenAI,
		"anthropic": useAnthropic,
		"google":    useGoogle,
		"ollama":    useOllama,
		"llamacpp":  useLlamaCpp,
	}

	var provider string
	for name, use := range providers {
		if use {
			if provider != "" {
				return ModelConfig{}, ClientConfig{}, nil,
					errors.New("multiple providers specified")
			}
			provider = name
		}
	}

	cfg := &Config{}
	if !noConfig {
		var filenames []string
		if configFilename != "" {
			filenames = []string{configFilename}
		} else {
			filenames = configFilenames()
		}

		var err error
		cfg, err = ReadConfig(filenames)
		if err != nil {
			return ModelConfig{}, ClientConfig{}, nil, err
		}

		if provider == "" {
			provider = cfg.Provider
		}
		clntCfg, ok := cfg.FindClientConfig(provider)

		if model == "" && ok {
			model = clntCfg.Model
		}
		if apiKey == "" && ok {
			apiKey = clntCfg.APIKey
		}
		if baseURL == "" && ok {
			baseURL = clntCfg.BaseURL
		}
		if !hasThoughts && ok {
			thoughts = clntCfg.Thoughts
		}
		if effort == "" && ok {
			effort = clntCfg.Effort
		}
		if maxTokens == 0 && ok {
			maxTokens = clntCfg.MaxTokens
		}
	}

	if braveAPIKey != "" {
		cfg.BraveAPIKey = braveAPIKey
	} else if cfg.BraveAPIKey == "" {
		cfg.BraveAPIKey = os.Getenv("BRAVE_API_KEY")
	}

	return ModelConfig{
			Model:           model,
			IncludeThoughts: thoughts,
			Effort:          effort,
			MaxTokens:       maxTokens,
		}, ClientConfig{
			Provider: provider,
			APIKey:   apiKey,
			BaseURL:  baseURL,
		}, cfg, nil
}
