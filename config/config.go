package config

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsimple"

	"github.com/leftmike/gait/sandbox"
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

type NetworkConfig struct {
	Mode  string   `hcl:"mode"` // none, all, allow
	Hosts []string `hcl:"hosts,optional"`
}

type SandboxConfig struct {
	Name    string         `hcl:"name,label"`
	Type    string         `hcl:"type"`
	Read    []string       `hcl:"read,optional"`
	Write   []string       `hcl:"write,optional"`
	Deny    []string       `hcl:"deny,optional"`
	Env     []string       `hcl:"env,optional"`
	Network *NetworkConfig `hcl:"network,block"`
	Backend hcl.Body       `hcl:",remain"`
}

func (sbCfg *SandboxConfig) Policy() (sandbox.Policy, error) {
	p := sandbox.Policy{
		ReadOnly:  sbCfg.Read,
		ReadWrite: sbCfg.Write,
		Deny:      sbCfg.Deny,
		Env:       sbCfg.Env,
	}

	if sbCfg.Network != nil {
		switch sbCfg.Network.Mode {
		case "none":
			p.Network.Mode = sandbox.NetNone
		case "all":
			p.Network.Mode = sandbox.NetAll
		case "allow":
			p.Network.Mode = sandbox.NetAllowList
		default:
			return sandbox.Policy{}, fmt.Errorf("sandbox %s: unknown network mode: %s",
				sbCfg.Name, sbCfg.Network.Mode)
		}
		p.Network.Hosts = sbCfg.Network.Hosts
	}

	return p, nil
}

type MCPServer struct {
	Name    string   `hcl:"name,label"`
	Type    string   `hcl:"type,optional"`    // stdio, http, sse
	Command string   `hcl:"command,optional"` // stdio
	Args    []string `hcl:"args,optional"`    // stdio
	URL     string   `hcl:"url,optional"`     // http, sse
	// Headers
}

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
	ModelConfig  ModelConfig
	ClientConfig *ClientConfig
	// SandboxConfig is nil when there is no sandbox.
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

func expandHome(name string) string {
	if name != "~" && !strings.HasPrefix(name, "~/") {
		return name
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return name
	}
	return filepath.Join(home, name[1:])
}

func ReadConfig(filenames []string) (*Config, error) {
	for _, name := range filenames {
		name = expandHome(name)
		buf, err := os.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}

		var cfg Config
		err = hclsimple.Decode(name, buf, nil, &cfg)
		if err != nil {
			return nil, err
		}
		err = cfg.checkDuplicates()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return &cfg, nil
	}

	return nil, fmt.Errorf("config file not found: %v", filenames)
}

func checkDuplicate(kind string, names []string) error {
	seen := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate %s block: %s", kind, name)
		}
		seen[key] = true
	}
	return nil
}

func (cfg *Config) checkDuplicates() error {
	var providers, sandboxes []string
	for _, clntCfg := range cfg.ClientConfigs {
		providers = append(providers, clntCfg.Provider)
	}
	for _, sbCfg := range cfg.SandboxConfigs {
		sandboxes = append(sandboxes, sbCfg.Name)
	}

	err := checkDuplicate("provider", providers)
	if err != nil {
		return err
	}
	return checkDuplicate("sandbox", sandboxes)
}

func ParseFlags(fs *flag.FlagSet, args []string) (AgentConfig, *Config, error) {
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
	fs.StringVar(&braveAPIKey, "brave-apikey", "", "`api_key` for Brave Search")
	err := fs.Parse(args)
	if err != nil {
		return AgentConfig{}, nil, err
	}

	if maxTokens < 0 {
		return AgentConfig{}, nil, fmt.Errorf("max-tokens must not be negative")
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

		cfg, err = ReadConfig(filenames)
		if err != nil {
			return AgentConfig{}, nil, err
		}

		if provider == "" {
			provider = strings.ToLower(cfg.Provider)
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

		if sandbox == "" {
			sandbox = cfg.Sandbox
		}
		if sandbox != "" && sandbox != "none" {
			sbCfg = cfg.FindSandboxConfig(sandbox)
			if sbCfg == nil {
				return AgentConfig{}, nil, fmt.Errorf("unknown sandbox: %s", sandbox)
			}
		}
	} else if sandbox != "" && sandbox != "none" {
		return AgentConfig{}, nil, errors.New("sandbox flag requires a config")
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
