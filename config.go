package main

import (
	"io/ioutil"
	"os"
	"regexp"

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

func match(s1, s2 string) bool {
	if s1 == "" {
		return true
	}

	if s1 == s2 {
		return true
	}

	re, err := regexp.Compile(s1)
	return err == nil && re.MatchString(s2)
}

func (cfg *Config) FindProvider(name string) Provider {
	for _, p := range cfg.Providers {
		if match(p.Name, name) {
			return p
		}
	}

	return Provider{}
}

func (cfg *Config) FindModel(provider, name string) Model {
	for _, m := range cfg.Models {
		if match(m.Provider, provider) && match(m.Name, name) {
			return m
		}
	}

	return Model{}
}

func loadConfig(filenames []string) (*Config, error) {
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

		tree, ret := hclsyntax.ParseConfig(buf, filename, hcl.Pos{Line: 1, Column: 1})
		if ret.HasErrors() {
			return nil, ret
		}

		var cfg Config
		ret = gohcl.DecodeBody(tree.Body, nil, &cfg)
		if ret.HasErrors() {
			return nil, ret
		}
		return &cfg, nil
	}

	return nil, nil
}
