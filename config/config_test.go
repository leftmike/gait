package config

import (
	"reflect"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		pat string
		str string
		ret bool
	}{
		// Exact match
		{"exact", "exact", true},
		{"exact", "notexact", false},

		// Glob patterns
		{"*", "anything", true},
		{"any*", "anything", true},
		{"*thing", "anything", true},
		{"any*ing", "anything", true},
		{"a?y*ing", "anything", true},
		{"a?z*ing", "anything", false},

		// More complex patterns
		{"*[abc]", "nope", false},
		{"*[abc]", "nota", true},
		{"test?", "test1", true},
		{"test?", "test", false},
		{"test?", "test12", false},
	}

	for _, test := range tests {
		ret := match(test.pat, test.str)
		if ret != test.ret {
			t.Errorf("match(%q, %q) got %v, want %v", test.pat, test.str, ret, test.ret)
		}
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		config *Config
		fail   bool
	}{
		{
			config: &Config{
				Providers: []Provider{
					{Name: "openai"},
					{Name: "*-api"},
				},
				Models: []Model{
					{Provider: "openai", Name: "gpt-*"},
					{Provider: "*", Name: "*"},
				},
			},
			fail: false,
		},
		{
			config: &Config{
				Models: []Model{
					{Provider: "[invalid", Name: "model"},
				},
			},
			fail: true,
		},
		{
			config: &Config{
				Models: []Model{
					{Provider: "provider", Name: "model[abc"},
				},
			},
			fail: true,
		},
		{
			config: &Config{
				Providers: []Provider{
					{Name: "provider["},
				},
			},
			fail: true,
		},
		{
			config: &Config{},
			fail:   false,
		},
	}

	for _, test := range tests {
		err := test.config.validateConfig()
		if test.fail {
			if err == nil {
				t.Errorf("validateConfig(%+v) did not fail", test.config)
			}
		} else {
			if err != nil {
				t.Errorf("validateConfig(%+v) failed with %s", test.config, err)
			}
		}
	}
}

func TestParseConfig(t *testing.T) {
	maxTokens8192 := 8192
	maxTokens4096 := 4096
	maxTokens2048 := 2048
	temperature07 := 0.7

	tests := []struct {
		buf    string
		config *Config
		fail   bool
	}{
		{
			buf: `
provider="openai"
provider openai {
	model="gpt-4"
	api_key="test-key"
}
model openai gpt-4 {
	max_tokens=8192
	temperature=0.7
}
`,
			config: &Config{
				Provider: "openai",
				Providers: []Provider{
					{Name: "openai", Model: "gpt-4", APIKey: "test-key"},
				},
				Models: []Model{
					{
						Provider:    "openai",
						Name:        "gpt-4",
						MaxTokens:   &maxTokens8192,
						Temperature: &temperature07,
					},
				},
			},
			fail: false,
		},
		{
			buf: `
provider="openai"
provider openai {
	api_key="test-key"
}
model openai "*" {
	max_tokens=8192
}
`,
			config: &Config{
				Provider: "openai",
				Providers: []Provider{
					{Name: "openai", APIKey: "test-key"},
				},
				Models: []Model{
					{Provider: "openai", Name: "*", MaxTokens: &maxTokens8192},
				},
			},
			fail: false,
		},
		{
			buf: `
provider="openai"
provider openai {
	api_key="test-key"
}
model openai "gpt-4o-mini" {
	max_tokens=4096
}
model openai "*" {
	max_tokens=8192
}
provider "anthropic" {
	api_key="api-key"
}
model anthropic "claude-3-7-sonnet" {
	max_tokens=2048
	temperature=0.7
}
`,
			config: &Config{
				Provider: "openai",
				Providers: []Provider{
					{Name: "openai", APIKey: "test-key"},
					{Name: "anthropic", APIKey: "api-key"},
				},
				Models: []Model{
					{Provider: "openai", Name: "gpt-4o-mini", MaxTokens: &maxTokens4096},
					{Provider: "openai", Name: "*", MaxTokens: &maxTokens8192},
					{
						Provider:    "anthropic",
						Name:        "claude-3-7-sonnet",
						MaxTokens:   &maxTokens2048,
						Temperature: &temperature07,
					},
				},
			},
			fail: false,
		},
		{
			buf: `
provider="openai"
provider openai {
	api_key="test-key"

// Missing closing brace
`,
			config: nil,
			fail:   true,
		},
		{
			buf: `
provider="openai"
provider openai {
	api_key="test-key"
}
model openai "[invalid" {
	max_tokens=8192
}
`,
			config: nil,
			fail:   true,
		},
		{
			buf:    ``,
			config: &Config{},
			fail:   false,
		},
	}

	for _, test := range tests {
		cfg, err := parseConfig("test.hcl", []byte(test.buf))

		if test.fail {
			if err == nil {
				t.Errorf("parseConfig(%s) did not fail", test.buf)
			}
		} else {
			if err != nil {
				t.Errorf("parseConfig(%s) failed with %s", test.buf, err)
				continue
			}

			if !reflect.DeepEqual(test.config, cfg) {
				t.Errorf("parseConfig(%s) got %+v want %+v", test.buf, cfg, test.config)
			}
		}
	}
}
