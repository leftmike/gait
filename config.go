package main

import (
	"io/ioutil"
	"os"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/mitchellh/go-homedir"
)

type Model struct {
	Name        string         `hcl:"name,label"`
	MaxRetries  *int           `hcl:"max_retries,optional"`
	MaxTokens   *int           `hcl:"max_tokens,optional"`
	Memory      *int           `hcl:"memory,optional"`
	RetryDelay  *time.Duration `hcl:"retry_delay,optional"`
	Temperature *float64       `hcl:"temperature,optional"`
}

type Provider struct {
	Name        string         `hcl:"name,label"`
	APIKey      string         `hcl:"api_key"`
	MaxRetries  *int           `hcl:"max_retries,optional"`
	MaxTokens   *int           `hcl:"max_tokens,optional"`
	Memory      *int           `hcl:"memory,optional"`
	RetryDelay  *time.Duration `hcl:"retry_delay,optional"`
	Temperature *float64       `hcl:"temperature,optional"`
	Model       string         `hcl:"model,optional"`
	Models      []Model        `hcl:"model,block"`
}

type Config struct {
	Provider    string         `hcl:"provider,optional"`
	MaxRetries  *int           `hcl:"max_retries,optional"`
	MaxTokens   *int           `hcl:"max_tokens,optional"`
	Memory      *int           `hcl:"memory,optional"`
	RetryDelay  *time.Duration `hcl:"retry_delay,optional"`
	Temperature *float64       `hcl:"temperature,optional"`
	Providers   []Provider     `hcl:"provider,block"`
}

func (cfg *Config) FindProvider(name string) Provider {
	for _, p := range cfg.Providers {
		if p.Name == name {
			return p
		}
	}

	return Provider{}
}

func (p *Provider) FindModel(name string) Model {
	for _, m := range p.Models {
		if m.Name == name {
			return m
		}
	}

	return Model{}
}

func loadConfig(filenames []string) (Config, error) {
	for _, filename := range filenames {
		filename, err := homedir.Expand(filename)
		if err != nil {
			return Config{}, err
		}

		buf, err := ioutil.ReadFile(filename)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return Config{}, err
		}

		tree, ret := hclsyntax.ParseConfig(buf, filename, hcl.Pos{Line: 1, Column: 1})
		if ret.HasErrors() {
			return Config{}, ret
		}

		var cfg Config
		ret = gohcl.DecodeBody(tree.Body, nil, &cfg)
		if ret.HasErrors() {
			return Config{}, ret
		}
		return cfg, nil
	}

	return Config{}, nil
}
