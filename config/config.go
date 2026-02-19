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
	Name   string `hcl:"name,label"`
	Model  string `hcl:"model,optional"`
	APIKey string `hcl:"api_key,optional"`
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
	Provider    string      `hcl:"provider,optional"`
	Providers   []Provider  `hcl:"provider,block"`
	MCPServers  []MCPServer `hcl:"mcpserver,block"`
	Skills      []string    `hcl:"skills,optional"`
	BraveAPIKey string      `hcl:"brave_api_key,optional"`
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

func Options(fs *flag.FlagSet) (string, string, string, *Config, error) {
	var configFilename string
	var noConfig bool
	var useOpenAI bool
	var useAnthropic bool
	var useGemini bool
	var modelName string
	var apiKey string
	var braveAPIKey string

	fs.StringVar(&configFilename, "config", "", "config filename")
	fs.BoolVar(&noConfig, "no-config", false, "do not load config")
	fs.BoolVar(&useOpenAI, "openai", false, "use openai")
	fs.BoolVar(&useAnthropic, "anthropic", false, "use anthropic")
	fs.BoolVar(&useGemini, "gemini", false, "use gemini")
	fs.StringVar(&modelName, "model", "", "generate using this model `model`")
	fs.StringVar(&apiKey, "apikey", "", "`api key` to use")
	fs.StringVar(&braveAPIKey, "brave-apikey", "", "`api key` for Brave Search")
	fs.Parse(os.Args[1:])

	var provider string
	if useOpenAI {
		provider = "openai"
	}
	if useAnthropic {
		if provider != "" {
			return "", "", "", nil, errors.New("multiple providers specified")
		}
		provider = "anthropic"
	}
	if useGemini {
		if provider != "" {
			return "", "", "", nil, errors.New("multiple providers specified")
		}
		provider = "gemini"
	}

	var cfg *Config
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
			return "", "", "", nil, err
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
		if braveAPIKey != "" {
			cfg.BraveAPIKey = braveAPIKey
		} else if cfg.BraveAPIKey == "" {
			cfg.BraveAPIKey = os.Getenv("BRAVE_API_KEY")
		}
	}

	return provider, modelName, apiKey, cfg, nil
}
