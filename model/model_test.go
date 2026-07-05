package model_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/model"
)

var (
	provider = flag.String("provider", "", "limit tests to this provider")
)

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
		{provider: "openai", model: "gpt-5.4-nano", thoughts: true},
		{provider: "anthropic", model: "claude-sonnet-4-6", thoughts: true},
		{provider: "anthropic", model: "claude-haiku-4-5-20251001", thoughts: true},
		{provider: "gemini", model: "gemini-3.1-flash-lite", short: true, thoughts: true},
		{provider: "ollama", model: "llama3.2:3b", local: true},
		{provider: "llamacpp", local: true},
	}

	for _, c := range cases {
		if *provider != "" && *provider != c.provider {
			continue
		} else if mdlCfg.IncludeThoughts && !c.thoughts {
			continue
		} else if testing.Short() && !c.short {
			fmt.Printf("skipping %s %s\n", c.provider, c.model)
			continue
		} else if *provider == "" && c.local {
			fmt.Printf("skipping %s (local) %s\n", c.provider, c.model)
			continue
		}

		clntCfg, ok := cfg.FindClientConfig(c.provider)
		if !c.local && (!ok || clntCfg.APIKey == "") {
			t.Fatalf("missing api key for provider: %s", c.provider)
		}

		var clnt model.Client
		switch c.provider {
		case "openai":
			clnt, err = model.NewOpenAIClient(clntCfg.APIKey)
			if err != nil {
				t.Fatalf("NewOpenAIClient() failed with %s", err)
			}
		case "anthropic":
			clnt, err = model.NewAnthropicClient(clntCfg.APIKey)
			if err != nil {
				t.Fatalf("NewAnthropicClient() failed with %s", err)
			}
		case "gemini":
			clnt, err = model.NewGeminiClient(clntCfg.APIKey)
			if err != nil {
				t.Fatalf("NewGeminiClient() failed with %s", err)
			}
		case "ollama":
			clnt, err = model.NewOllamaClient(clntCfg)
			if err != nil {
				t.Fatalf("NewOllamaClient() failed with %s", err)
			}
		case "llamacpp":
			clnt, err = model.NewLlamaCppClient(clntCfg)
			if err != nil {
				t.Fatalf("NewLlamaCppClient() failed with %s", err)
			}
		default:
			t.Fatalf("unknown provider: %s", c.provider)
		}

		mdlCfg.Model = c.model
		test(t, clnt, c.provider, c.model, mdlCfg)
	}
}

func testSimple(t *testing.T, clnt model.Client, provider, name string,
	mdlCfg config.ModelConfig) {

	fmt.Println(provider, name)

	mdl, err := clnt.NewModel(mdlCfg, nil)
	if err != nil {
		t.Errorf("NewModel(%s, %s) failed with %s", provider, name, err)
	}

	st := clnt.NewState()
	st.Prompt("Hello")
	n := st.Len()

	ctx := context.Background()
	err = clnt.Generate(ctx, mdl, st, &model.Options{})
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
	testModels(t, testSimple, config.ModelConfig{})
}

var (
	temperatureLocation string
	weatherLocation     string
)

type currentTemperatureArgs struct {
	Location string `json:"location" jsonschema:"location to get the current temperature for"`
}

func currentTemperature(ctx context.Context, buf []byte) (string, error) {
	var args currentTemperatureArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	temperatureLocation = args.Location

	return "40", nil
}

type currentWeatherArgs struct {
	Location string `json:"location" jsonschema:"location to get the current weather for"`
}

func currentWeather(ctx context.Context, buf []byte) (string, error) {
	var args currentWeatherArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	weatherLocation = args.Location

	return "sunny and 70", nil
}

func testSimpleTool(t *testing.T, clnt model.Client, provider, name string,
	mdlCfg config.ModelConfig) {

	fmt.Println(provider, name)

	tools := map[string]model.Tool{
		"current_temperature": model.Tool{
			Name:        "current_temperature",
			Description: "Gets the current temperature for the given location",
			Func:        currentTemperature,
			Schema:      model.MustToolSchema[currentTemperatureArgs](),
		},
	}

	mdl, err := clnt.NewModel(mdlCfg, tools)
	if err != nil {
		t.Errorf("NewModel(%s, %s) failed with %s", provider, name, err)
	}

	st := clnt.NewState()
	st.Prompt(`What is the current temperature for seattle? You must call the
current_temperature tool.`)
	n := st.Len()

	temperatureLocation = ""

	ctx := context.Background()
	err = clnt.Generate(ctx, mdl, st, &model.Options{})
	if err != nil {
		t.Errorf("Generate(%s, %s) failed with %s", provider, name, err)
	}

	if temperatureLocation == "" {
		t.Errorf("Generate(%s, %s) current temperature not called", provider, name)
	} else if !strings.Contains(strings.ToLower(temperatureLocation), "seattle") {
		t.Errorf("Generate(%s %s) argument did not include seattle: %s", provider, name,
			temperatureLocation)
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
	testModels(t, testSimpleTool, config.ModelConfig{})
}

func testMultiTool(t *testing.T, clnt model.Client, provider, name string,
	mdlCfg config.ModelConfig) {

	fmt.Println(provider, name)

	tools := map[string]model.Tool{
		"current_temperature": model.Tool{
			Name:        "current_temperature",
			Description: "Gets the current temperature for the given location",
			Func:        currentTemperature,
			Schema:      model.MustToolSchema[currentTemperatureArgs](),
		},
		"current_weather": model.Tool{
			Name:        "current_weather",
			Description: "Gets the current weather for the given location",
			Func:        currentWeather,
			Schema:      model.MustToolSchema[currentWeatherArgs](),
		},
	}

	mdl, err := clnt.NewModel(mdlCfg, tools)
	if err != nil {
		t.Errorf("NewModel(%s, %s) failed with %s", provider, name, err)
	}

	st := clnt.NewState()
	st.Prompt(`What is the current temperature and weather for seattle? You must call both the
current_temperature and current_weather tools.`)
	n := st.Len()

	temperatureLocation = ""
	weatherLocation = ""

	ctx := context.Background()
	err = clnt.Generate(ctx, mdl, st, &model.Options{})
	if err != nil {
		t.Errorf("Generate(%s, %s) failed with %s", provider, name, err)
	}

	if temperatureLocation == "" {
		t.Errorf("Generate(%s, %s) current temperature not called", provider, name)
	} else if !strings.Contains(strings.ToLower(temperatureLocation), "seattle") {
		t.Errorf("Generate(%s %s) argument did not include seattle: %s", provider, name,
			temperatureLocation)
	}

	if weatherLocation == "" {
		t.Errorf("Generate(%s, %s) current weather not called", provider, name)
	} else if !strings.Contains(strings.ToLower(weatherLocation), "seattle") {
		t.Errorf("Generate(%s %s) argument did not include seattle: %s", provider, name,
			weatherLocation)
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

	// XXX
	if (provider != "openai" && toolCalls != 2) || (provider == "openai" && toolCalls < 2) {
		t.Errorf("Generate(%s %s) tool call steps: got %d want 2", provider, name, toolCalls)
	}
	if (provider != "openai" && toolCalls != 2) || (provider == "openai" && toolCalls < 2) {
		t.Errorf("Generate(%s %s) tool output steps: got %d want 2", provider, name, toolOutputs)
	}
}

func TestMultiTool(t *testing.T) {
	testModels(t, testMultiTool, config.ModelConfig{})
}

func TestMultiToolThinking(t *testing.T) {
	testModels(t, testMultiTool, config.ModelConfig{IncludeThoughts: true})
}
