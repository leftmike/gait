package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, src string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "gait.hcl")
	if err := os.WriteFile(name, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	cases := map[string]string{
		"~":               home,
		"~/.gait.hcl":     filepath.Join(home, ".gait.hcl"),
		"./gait.hcl":      "./gait.hcl",
		"~other/gait.hcl": "~other/gait.hcl",
	}
	for in, want := range cases {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadConfig(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.hcl")
	good := writeConfig(t, `
provider = "anthropic"
provider "anthropic" {
    model = "m"
}
sandbox = "s"
sandbox "s" {
    read {
        ask = ["."]
    }
}
`)
	cfg, err := ReadConfig([]string{missing, good})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "anthropic" || len(cfg.ClientConfigs) != 1 || cfg.Sandbox != "s" {
		t.Errorf("ReadConfig = %+v", cfg)
	}

	if _, err := ReadConfig([]string{missing}); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Errorf("ReadConfig(missing) = %v, want not found", err)
	}

	if _, err := ReadConfig([]string{t.TempDir()}); err == nil ||
		strings.Contains(err.Error(), "not found") {
		t.Errorf("ReadConfig(dir) = %v, want read error", err)
	}

	for _, src := range []string{
		"provider \"a\" {\n}\nprovider \"A\" {\n}\n",
		"sandbox \"s\" {\n}\nsandbox \"s\" {\n}\n",
	} {
		if _, err := ReadConfig([]string{writeConfig(t, src)}); err == nil ||
			!strings.Contains(err.Error(), "duplicate") {
			t.Errorf("ReadConfig(%q) = %v, want duplicate", src, err)
		}
	}
}

func TestParseFlags(t *testing.T) {
	name := writeConfig(t, `
provider = "Anthropic"
provider "anthropic" {
    model = "m"
}
`)

	parse := func(args ...string) (AgentConfig, error) {
		agntCfg, _, err := ParseFlags(flag.NewFlagSet("test", flag.ContinueOnError), args)
		return agntCfg, err
	}

	agntCfg, err := parse("-config", name)
	if err != nil {
		t.Fatal(err)
	}
	if agntCfg.ClientConfig.Provider != "anthropic" || agntCfg.ModelConfig.Model != "m" {
		t.Errorf("ParseFlags = %+v %+v", agntCfg.ClientConfig, agntCfg.ModelConfig)
	}

	if _, err := parse("-no-config", "-ollama", "-sandbox", "s"); err == nil {
		t.Error("ParseFlags(-no-config -sandbox s) did not fail")
	}
	if _, err := parse("-no-config", "-ollama", "-sandbox", "none"); err != nil {
		t.Errorf("ParseFlags(-no-config -sandbox none) = %v", err)
	}
	if _, err := parse("-no-config", "-max-tokens", "-1"); err == nil {
		t.Error("ParseFlags(-max-tokens -1) did not fail")
	}
	if _, err := parse("-no-config", "-ollama", "-openai"); err == nil {
		t.Error("ParseFlags(multiple providers) did not fail")
	}
}
