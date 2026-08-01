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

type RWActions struct {
	Allow []string `hcl:"allow,optional"`
	Ask   []string `hcl:"ask,optional"`
	Deny  []string `hcl:"deny,optional"`
}

type ExecuteActions struct {
	Allow [][]string `hcl:"allow,optional"`
	Ask   [][]string `hcl:"ask,optional"`
	Deny  [][]string `hcl:"deny,optional"`
}

type SandboxConfig struct {
	Name    string          `hcl:"name,label"`
	Read    *RWActions      `hcl:"read,block"`
	Execute *ExecuteActions `hcl:"execute,block"`
	Write   *RWActions      `hcl:"write,block"`
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

/*
system name {
    read {
        allow = []
        ask = ["."]
        deny = []
    }
}
*/

type Config struct {
	Provider       string          `hcl:"provider,optional"`
	ClientConfigs  []ClientConfig  `hcl:"provider,block"`
	Sandbox        string          `hcl:"sandbox,optional"`
	SandboxConfigs []SandboxConfig `hcl:"sandbox,block"`
	MCPServers     []MCPServer     `hcl:"mcpserver,block"`
	Skills         []string        `hcl:"skills,optional"`
	BraveAPIKey    string          `hcl:"brave_api_key,optional"`
}

type AgentConfig struct {
	ModelConfig   ModelConfig
	ClientConfig  *ClientConfig
	SandboxConfig *SandboxConfig
}

func (cfg *Config) FindClientConfig(provider string) (ClientConfig, bool) {
	for _, clntCfg := range cfg.ClientConfigs {
		if strings.EqualFold(provider, clntCfg.Provider) {
			return clntCfg, true
		}
	}

	return ClientConfig{}, false
}

func (cfg *Config) FindSandboxConfig(sandbox string) *SandboxConfig {
	for _, sbCfg := range cfg.SandboxConfigs {
		if strings.EqualFold(sandbox, sbCfg.Name) {
			return &sbCfg
		}
	}

	return nil
}

func configFilenames() []string {
	if runtime.GOOS == "windows" {
		return []string{"./gait.hcl", "~/gait/gait.hcl", "~/gait.hcl"}
	}

	return []string{"./gait.hcl", "~/.gait/gait.hcl", "~/.gait.hcl"}
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

func stringsJoin(vals []string, conj string) string {
	switch len(vals) {
	case 1:
		return vals[0]
	case 2:
		return fmt.Sprintf("%s %s %s", vals[0], conj, vals[1])
	default:
		return fmt.Sprintf("%s, %s %s", strings.Join(vals[:len(vals)-1], ", "), conj,
			vals[len(vals)-1])
	}
}

func ParseFlags(fs *flag.FlagSet) (AgentConfig, *Config, error) {
	var configFilename string
	var noConfig bool
	var useAnthropic bool
	var useGoogle bool
	var useHuggingFace bool
	var useLlamaCpp bool
	var useOllama bool
	var useOpenAI bool
	var useOpenRouter bool
	var model string
	var apiKey string
	var baseURL string
	var thoughts, hasThoughts bool
	var effort string
	var maxTokens int
	var sandbox string
	var noSandbox bool
	var braveAPIKey string

	fs.StringVar(&configFilename, "config", "", "config filename")
	fs.BoolVar(&noConfig, "no-config", false, "do not load config")
	fs.BoolVar(&useAnthropic, "anthropic", false, "use anthropic")
	fs.BoolVar(&useGoogle, "google", false, "use google")
	fs.BoolVar(&useHuggingFace, "huggingface", false, "use huggingface")
	fs.BoolVar(&useLlamaCpp, "llamacpp", false, "use llama.cpp")
	fs.BoolVar(&useOllama, "ollama", false, "use ollama")
	fs.BoolVar(&useOpenAI, "openai", false, "use openai")
	fs.BoolVar(&useOpenRouter, "openrouter", false, "use openrouter")
	fs.StringVar(&model, "model", "", "generate using this model `model`")
	fs.StringVar(&apiKey, "apikey", "", "`api_key` to use")
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
	fs.StringVar(&sandbox, "sandbox", "", "tool `sandbox`")
	fs.BoolVar(&noSandbox, "no-sandbox", false, "tools don't use a sandbox")
	fs.StringVar(&braveAPIKey, "brave-apikey", "", "`api_key` for Brave Search")
	fs.Parse(os.Args[1:])

	if maxTokens < 0 {
		return AgentConfig{}, nil, fmt.Errorf("max-tokens must be positive")
	}

	providers := map[string]bool{
		"anthropic":   useAnthropic,
		"google":      useGoogle,
		"huggingface": useHuggingFace,
		"llamacpp":    useLlamaCpp,
		"ollama":      useOllama,
		"openai":      useOpenAI,
		"openrouter":  useOpenRouter,
	}

	var provider string
	for name, use := range providers {
		if use {
			if provider != "" {
				return AgentConfig{}, nil, errors.New("multiple providers specified")
			}
			provider = name
		}
	}

	cfg := &Config{}
	var sbCfg *SandboxConfig
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
			return AgentConfig{}, nil, err
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

		if noSandbox {
			if sandbox != "" && sandbox != "none" {
				return AgentConfig{}, nil, errors.New("no sandbox and sandbox flags not allowed")
			}
		} else {
			if sandbox == "" {
				sandbox = cfg.Sandbox
			}
			if sandbox != "" && sandbox != "none" {
				sbCfg = cfg.FindSandboxConfig(sandbox)
				if sbCfg == nil {
					return AgentConfig{}, nil, fmt.Errorf("unknown sandbox: %s", sandbox)
				}
			}
		}
	}

	if braveAPIKey != "" {
		cfg.BraveAPIKey = braveAPIKey
	} else if cfg.BraveAPIKey == "" {
		cfg.BraveAPIKey = os.Getenv("BRAVE_API_KEY")
	}

	return AgentConfig{
		ModelConfig: ModelConfig{
			Model:           model,
			IncludeThoughts: thoughts,
			Effort:          effort,
			MaxTokens:       maxTokens,
		},
		ClientConfig: &ClientConfig{
			Provider: provider,
			APIKey:   apiKey,
			BaseURL:  baseURL,
		},
		SandboxConfig: sbCfg,
	}, cfg, nil
}
