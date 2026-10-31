package model_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/system"
	"github.com/leftmike/gait/tool"
)

var (
	provider = flag.String("provider", "", "limit tests to this provider")
)

func testSandbox() *system.Sandbox {
	sb, err := system.NewSandbox(nil, nil)
	if err != nil {
		panic(fmt.Sprintf("new sandbox failed: %s", err))
	}
	return sb
}

func TestMain(m *testing.M) {
	flag.Parse()

	os.Exit(m.Run())
}

type testModelFunc func(t *testing.T, clnt model.Client, provider, name string,
	mdlCfg config.ModelConfig)

func testModels(t *testing.T, test testModelFunc, mdlCfg config.ModelConfig) {
	t.Helper()

	cfg, err := config.ReadConfig([]string{"./gait.hcl", "../gait.hcl"})
	if err != nil {
		t.Fatalf("ReadConfig() failed with %s", err)
	}

	cases := []struct {
		provider, model string
		short           bool
		local           bool
		thoughts        bool
	}{
		{provider: "anthropic", model: "claude-sonnet-5-5", thoughts: true},
		{provider: "anthropic", model: "claude-haiku-5-5", thoughts: true},
		{provider: "google", model: "gemini-3.1-flash-lite", short: true, thoughts: true},
		{provider: "huggingface", model: "Qwen/Qwen3.6-35B-A3B", short: true},
		{provider: "llamacpp", local: true},
		{provider: "ollama", model: "llama3.2:3b", local: true},
		{provider: "openai", model: "gpt-5.4-nano", thoughts: true},
		{provider: "openrouter", model: "nvidia/nemotron-3-nano-30b-a3b", short: true,
			thoughts: true},
	}

	for _, c := range cases {
		if *provider != "" && *provider != c.provider {
			continue
		} else if mdlCfg.IncludeThoughts && !c.thoughts {
			continue
		}

		t.Run(c.provider+"/"+c.model, func(t *testing.T) {
			if testing.Short() && !c.short {
				t.Skipf("skipping %s %s", c.provider, c.model)
			} else if *provider == "" && c.local {
				t.Skipf("skipping %s (local) %s", c.provider, c.model)
			}
			t.Parallel()

			clntCfg, ok := cfg.FindClientConfig(c.provider)
			if !c.local && (!ok || clntCfg.APIKey == "") {
				t.Fatalf("missing api key for provider: %s", c.provider)
			} else if c.local {
				clntCfg.Provider = c.provider
			}

			clnt, err := model.NewClient(&clntCfg)
			if err != nil {
				t.Fatalf("NewClient(%s) failed with %s", c.provider, err)
			}

			cfg := mdlCfg
			cfg.Model = c.model
			test(t, clnt, c.provider, c.model, cfg)
		})
	}
}

func testSimple(t *testing.T, clnt model.Client, provider, name string,
	mdlCfg config.ModelConfig) {

	mdl, err := clnt.NewModel(mdlCfg)
	if err != nil {
		t.Fatalf("NewModel(%s, %s) failed with %s", provider, name, err)
	}

	st := clnt.NewState()
	st.Prompt("Hello")
	n := st.Len()

	ctx := context.Background()
	err = mdl.Generate(ctx, st, &model.Options{})
	if err != nil {
		t.Errorf("Generate(%s, %s) failed with %s", provider, name, err)
	}

	if st.Len() == n {
		t.Errorf("Generate(%s, %s) missing response: st.Len(): %d n: %d", provider, name, st.Len(),
			n)
	} else {
		step := st.Step(st.Len() - 1)
		if step.Type != model.ModelResponseStep {
			t.Errorf("Generate(%s, %s) missing model response; got %s", provider, name, step.Type)
		}
	}
}

func TestSimple(t *testing.T) {
	t.Parallel()
	testModels(t, testSimple, config.ModelConfig{})
}

type locations struct {
	mutex       sync.Mutex
	temperature string
	weather     string
}

type currentTemperatureArgs struct {
	Location string `json:"location" jsonschema:"location to get the current temperature for"`
}

func (l *locations) currentTemperature(ctx context.Context, sb *system.Sandbox,
	buf []byte) (string, error) {

	var args currentTemperatureArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	l.mutex.Lock()
	l.temperature = args.Location
	l.mutex.Unlock()

	return "40", nil
}

type currentWeatherArgs struct {
	Location string `json:"location" jsonschema:"location to get the current weather for"`
}

func (l *locations) currentWeather(ctx context.Context, sb *system.Sandbox,
	buf []byte) (string, error) {

	var args currentWeatherArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	l.mutex.Lock()
	l.weather = args.Location
	l.mutex.Unlock()

	return "sunny and 70", nil
}

func testSimpleTool(t *testing.T, clnt model.Client, provider, name string,
	mdlCfg config.ModelConfig) {

	var locs locations
	tools := map[string]tool.Tool{
		"current_temperature": tool.Tool{
			Name:        "current_temperature",
			Description: "Gets the current temperature for the given location",
			Func:        locs.currentTemperature,
			Schema:      tool.MustToolSchema[currentTemperatureArgs](),
		},
	}

	mdl, err := clnt.NewModel(mdlCfg)
	if err != nil {
		t.Fatalf("NewModel(%s, %s) failed with %s", provider, name, err)
	}
	err = mdl.SetTools(tools, testSandbox())
	if err != nil {
		t.Fatalf("SetTools(%s, %s) failed with %s", provider, name, err)
	}

	st := clnt.NewState()
	st.Prompt(`What is the current temperature for seattle? You must call the
current_temperature tool.`)
	n := st.Len()

	ctx := context.Background()
	err = mdl.Generate(ctx, st, &model.Options{})
	if err != nil {
		t.Errorf("Generate(%s, %s) failed with %s", provider, name, err)
	}

	if locs.temperature == "" {
		t.Errorf("Generate(%s, %s) current temperature not called", provider, name)
	} else if !strings.Contains(strings.ToLower(locs.temperature), "seattle") {
		t.Errorf("Generate(%s %s) argument did not include seattle: %s", provider, name,
			locs.temperature)
	}

	var toolCall, toolOutput bool
	for n < st.Len() {
		step := st.Step(n)
		n += 1

		if step.Type == model.ToolCallStep {
			toolCall = true
		} else if step.Type == model.ToolOutputStep {
			toolOutput = true
		}
	}
	if !toolCall {
		t.Errorf("Generate(%s %s) missing tool call step", provider, name)
	}
	if !toolOutput {
		t.Errorf("Generate(%s %s) missing tool output step", provider, name)
	}
}

func TestSimpleTool(t *testing.T) {
	t.Parallel()
	testModels(t, testSimpleTool, config.ModelConfig{})
}

func testMultiTool(t *testing.T, clnt model.Client, provider, name string,
	mdlCfg config.ModelConfig) {

	var locs locations
	tools := map[string]tool.Tool{
		"current_temperature": tool.Tool{
			Name:        "current_temperature",
			Description: "Gets the current temperature for the given location",
			Func:        locs.currentTemperature,
			Schema:      tool.MustToolSchema[currentTemperatureArgs](),
		},
		"current_weather": tool.Tool{
			Name:        "current_weather",
			Description: "Gets the current weather for the given location",
			Func:        locs.currentWeather,
			Schema:      tool.MustToolSchema[currentWeatherArgs](),
		},
	}

	mdl, err := clnt.NewModel(mdlCfg)
	if err != nil {
		t.Fatalf("NewModel(%s, %s) failed with %s", provider, name, err)
	}
	err = mdl.SetTools(tools, testSandbox())
	if err != nil {
		t.Fatalf("SetTools(%s, %s) failed with %s", provider, name, err)
	}

	st := clnt.NewState()
	st.Prompt(`What is the current temperature and weather for seattle? You must call both the
current_temperature and current_weather tools.`)
	n := st.Len()

	ctx := context.Background()
	err = mdl.Generate(ctx, st, &model.Options{})
	if err != nil {
		t.Errorf("Generate(%s, %s) failed with %s", provider, name, err)
	}

	if locs.temperature == "" {
		t.Errorf("Generate(%s, %s) current temperature not called", provider, name)
	} else if !strings.Contains(strings.ToLower(locs.temperature), "seattle") {
		t.Errorf("Generate(%s %s) argument did not include seattle: %s", provider, name,
			locs.temperature)
	}

	if locs.weather == "" {
		t.Errorf("Generate(%s, %s) current weather not called", provider, name)
	} else if !strings.Contains(strings.ToLower(locs.weather), "seattle") {
		t.Errorf("Generate(%s %s) argument did not include seattle: %s", provider, name,
			locs.weather)
	}
	var toolCalls, toolOutputs int
	for n < st.Len() {
		step := st.Step(n)
		n += 1

		if step.Type == model.ToolCallStep {
			toolCalls += 1
		} else if step.Type == model.ToolOutputStep {
			toolOutputs += 1
		}
	}

	if toolCalls != 2 {
		t.Errorf("Generate(%s %s) tool call steps: got %d want 2", provider, name, toolCalls)
	}
	if toolOutputs != 2 {
		t.Errorf("Generate(%s %s) tool output steps: got %d want 2", provider, name, toolOutputs)
	}
}

func TestMultiTool(t *testing.T) {
	t.Parallel()
	testModels(t, testMultiTool, config.ModelConfig{})
}

func TestMultiToolThinking(t *testing.T) {
	t.Parallel()
	testModels(t, testMultiTool, config.ModelConfig{IncludeThoughts: true})
}
