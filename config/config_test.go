package config

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leftmike/gait/sandbox"
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
    type = "none"
    read = ["."]
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
		"sandbox \"s\" {\ntype = \"none\"\n}\nsandbox \"s\" {\ntype = \"none\"\n}\n",
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

func TestSandboxPolicy(t *testing.T) {
	cfg, err := ReadConfig([]string{writeConfig(t, `
sandbox "s" {
    type  = "none"
    read  = ["~/go"]
    write = ["."]
    deny  = ["./.env"]
    env   = ["PATH"]
    network {
        mode  = "allow"
        hosts = ["proxy.golang.org"]
    }
    none {
    }
}
sandbox "t" {
    type = "none"
}
sandbox "u" {
    type = "none"
    network {
        mode = "some"
    }
}
`)})
	if err != nil {
		t.Fatal(err)
	}

	p, err := cfg.FindSandboxConfig("s").Policy()
	if err != nil {
		t.Fatal(err)
	}
	want := sandbox.Policy{
		ReadOnly:  []string{"~/go"},
		ReadWrite: []string{"."},
		Deny:      []string{"./.env"},
		Env:       []string{"PATH"},
		Network: sandbox.Network{
			Mode:  sandbox.NetAllowList,
			Hosts: []string{"proxy.golang.org"},
		},
	}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("Policy = %+v, want %+v", p, want)
	}

	p, err = cfg.FindSandboxConfig("t").Policy()
	if err != nil {
		t.Fatal(err)
	}
	if p.Network.Mode != sandbox.NetNone {
		t.Errorf("default network mode = %d, want none", p.Network.Mode)
	}

	if _, err := cfg.FindSandboxConfig("u").Policy(); err == nil {
		t.Error("Policy with an unknown network mode did not fail")
	}

	if _, err := ReadConfig([]string{writeConfig(t, "sandbox \"s\" {\n}\n")}); err == nil {
		t.Error("ReadConfig of a sandbox without a type did not fail")
	}
}

func TestParseFlagsSandbox(t *testing.T) {
	name := writeConfig(t, `
provider = "ollama"
sandbox = "s"
sandbox "s" {
    type = "none"
}
`)

	parse := func(args ...string) (AgentConfig, error) {
		agntCfg, _, err := ParseFlags(flag.NewFlagSet("test", flag.ContinueOnError), args)
		return agntCfg, err
	}

	agntCfg, err := parse("-config", name)
	if err != nil {
		t.Fatal(err)
	} else if agntCfg.SandboxConfig == nil || agntCfg.SandboxConfig.Name != "s" {
		t.Errorf("SandboxConfig = %+v, want s", agntCfg.SandboxConfig)
	}

	for _, args := range [][]string{
		{"-config", name, "-no-sandbox"},
		{"-config", name, "-sandbox", "none"},
	} {
		agntCfg, err := parse(args...)
		if err != nil {
			t.Fatal(err)
		} else if agntCfg.SandboxConfig != nil {
			t.Errorf("ParseFlags(%v) SandboxConfig = %+v, want nil", args,
				agntCfg.SandboxConfig)
		}
	}

	if _, err := parse("-config", name, "-sandbox", "x"); err == nil {
		t.Error("ParseFlags(-sandbox x) did not fail")
	}
}
