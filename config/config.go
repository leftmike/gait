package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/mitchellh/go-homedir"
)

type Model struct {
	Provider       string   `hcl:"provider,label"`
	Name           string   `hcl:"name,label"`
	CandidateCount *int     `hcl:"candidate_count,optional"`
	MaxTokens      *int     `hcl:"max_tokens,optional"`
	Temperature    *float64 `hcl:"temperature,optional"`
	// StopWords []string
	// StreamingFunc func(ctx context.Context, chunk []byte) error
	TopK              *int     `hcl:"top_k,optional"`
	TopP              *float64 `hcl:"top_p,optional"`
	Seed              *int     `hcl:"seed,optional"`
	MinLength         *int     `hcl:"min_length,optional"`
	MaxLength         *int     `hcl:"max_length,optional"`
	N                 *int     `hcl:"n,optional"`
	RepetitionPenalty *float64 `hcl:"repetition_penalty,optional"`
	FrequencyPenalty  *float64 `hcl:"frequency_penalty,optional"`
	PresencePenalty   *float64 `hcl:"presence_penalty,optional"`
	//JSONMode *bool `hcl:"json_mode,optional"`
	// Tools []Tool
	// ToolChoice any
	// Metadata map[string]interface{}
}

type Provider struct {
	Name   string `hcl:"name,label"`
	Model  string `hcl:"model,optional"`
	APIKey string `hcl:"api_key,optional"`
}

type Config struct {
	Provider  string     `hcl:"provider,optional"`
	Providers []Provider `hcl:"provider,block"`
	Models    []Model    `hcl:"model,block"`
}

func match(pattern, str string) bool {
	if pattern == str {
		return true
	}

	matched, err := filepath.Match(pattern, str)
	if err != nil {
		panic(fmt.Sprintf("unexpected bad glob pattern %q: %v", pattern, err))
	}
	return matched
}

func (cfg *Config) findProvider(name string) Provider {
	for _, p := range cfg.Providers {
		if match(p.Name, name) {
			return p
		}
	}

	return Provider{}
}

func (cfg *Config) findModel(provider, name string) Model {
	for _, m := range cfg.Models {
		if match(m.Provider, provider) && match(m.Name, name) {
			return m
		}
	}

	return Model{}
}

func (cfg *Config) validateConfig() error {
	for _, m := range cfg.Models {
		if m.Provider != "" {
			if _, err := filepath.Match(m.Provider, ""); err != nil {
				return fmt.Errorf("model: bad pattern %q: %v", m.Provider, err)
			}
		}

		if m.Name != "" {
			if _, err := filepath.Match(m.Name, ""); err != nil {
				return fmt.Errorf("model: bad pattern %q: %v", m.Name, err)
			}
		}
	}

	for _, p := range cfg.Providers {
		if p.Name != "" {
			_, err := filepath.Match(p.Name, "")
			if err != nil {
				return fmt.Errorf("provider: bad pattern %q: %v", p.Name, err)
			}
		}
	}

	return nil
}

func parseConfig(filename string, buf []byte) (*Config, error) {
	tree, ret := hclsyntax.ParseConfig(buf, filename, hcl.Pos{Line: 1, Column: 1})
	if ret.HasErrors() {
		return nil, ret
	}

	var cfg Config
	ret = gohcl.DecodeBody(tree.Body, nil, &cfg)
	if ret.HasErrors() {
		return nil, ret
	}

	if err := cfg.validateConfig(); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}

	return &cfg, nil
}

func Load(filenames []string) (*Config, error) {
	for _, filename := range filenames {
		filename, err := homedir.Expand(filename)
		if err != nil {
			return nil, err
		}

		buf, err := ioutil.ReadFile(filename)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return nil, err
		}

		cfg, err := parseConfig(filename, buf)
		if err != nil {
			return nil, err
		}

		return cfg, nil
	}

	return nil, nil
}
