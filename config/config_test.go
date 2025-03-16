package config

import (
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern  string
		str      string
		expected bool
	}{
		// Empty pattern matches anything
		{"", "anything", true},

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
		result := match(test.pattern, test.str)
		if result != test.expected {
			t.Errorf("match(%q, %q) = %v, expected %v",
				test.pattern, test.str, result, test.expected)
		}
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		config      *Config
		expectError bool
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
			expectError: false,
		},
		{
			config: &Config{
				Models: []Model{
					{Provider: "[invalid", Name: "model"},
				},
			},
			expectError: true,
		},
		{
			config: &Config{
				Models: []Model{
					{Provider: "provider", Name: "model[abc"},
				},
			},
			expectError: true,
		},
		{
			config: &Config{
				Providers: []Provider{
					{Name: "provider["},
				},
			},
			expectError: true,
		},
		{
			config:      &Config{},
			expectError: false,
		},
	}

	for i, test := range tests {
		err := test.config.validateConfig()
		if test.expectError && err == nil {
			t.Errorf("test %d: expected error but got nil", i)
		}
		if !test.expectError && err != nil {
			t.Errorf("test %d: expected no error but got: %v", i, err)
		}
	}
}
