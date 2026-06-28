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

type testModelFunc func(t *testing.T, mdl model.Model, provider, name string, opts *config.Options)

func testModels(t *testing.T, test testModelFunc, opts *config.Options) {
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
		} else if opts.IncludeThoughts && !c.thoughts {
			continue
		} else if testing.Short() && !c.short {
			fmt.Printf("skipping %s %s\n", c.provider, c.model)
			continue
		} else if *provider == "" && c.local {
			fmt.Printf("skipping %s (local) %s\n", c.provider, c.model)
			continue
		}

		p := cfg.FindProvider(c.provider)
		if c.local {
			if p == nil {
				p = &config.Provider{}
			}
		} else if p == nil || p.APIKey == "" {
			t.Fatalf("missing api key for provider: %s", c.provider)
		}

		var mdl model.Model
		switch c.provider {
		case "openai":
			mdl, err = model.NewOpenAIModel(p.APIKey)
			if err != nil {
				t.Fatalf("NewOpenAIModel() failed with %s", err)
			}
		case "anthropic":
			mdl, err = model.NewAnthropicModel(p.APIKey)
			if err != nil {
				t.Fatalf("NewAnthropicModel() failed with %s", err)
			}
		case "gemini":
			mdl, err = model.NewGeminiModel(p.APIKey)
			if err != nil {
				t.Fatalf("NewGeminiModel() failed with %s", err)
			}
		case "ollama":
			mdl, err = model.NewOllamaModel(p)
			if err != nil {
				t.Fatalf("NewOllamaModel() failed with %s", err)
			}
		case "llamacpp":
			mdl, err = model.NewLlamaCppModel(p)
			if err != nil {
				t.Fatalf("NewLlamaCppModel() failed with %s", err)
			}
		default:
			t.Fatalf("unknown provider: %s", c.provider)
		}

		testOpts := *opts
		testOpts.Model = c.model
		test(t, mdl, c.provider, c.model, &testOpts)
	}
}

func testSimple(t *testing.T, mdl model.Model, provider, name string, opts *config.Options) {
	fmt.Println(provider, name)

	ctx := context.Background()
	st := mdl.NewState()
	st.Prompt("Hello")
	n := st.Len()

	err := mdl.Generate(ctx, st, nil, opts)
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
	testModels(t, testSimple, &config.Options{})
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

func testSimpleTool(t *testing.T, mdl model.Model, provider, name string, opts *config.Options) {
	fmt.Println(provider, name)

	tools := map[string]model.Tool{
		"current_temperature": model.Tool{
			Name:        "current_temperature",
			Description: "Gets the current temperature for the given location",
			Func:        currentTemperature,
			Schema:      model.MustToolSchema[currentTemperatureArgs](),
		},
	}

	ctx := context.Background()
	st := mdl.NewState()
	st.Prompt(`What is the current temperature for seattle? You must call the
current_temperature tool.`)
	n := st.Len()

	temperatureLocation = ""
	err := mdl.Generate(ctx, st, tools, opts)
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
	testModels(t, testSimpleTool, &config.Options{})
}

func testMultiTool(t *testing.T, mdl model.Model, provider, name string, opts *config.Options) {
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

	ctx := context.Background()
	st := mdl.NewState()
	st.Prompt(`What is the current temperature and weather for seattle? You must call both the
current_temperature and current_weather tools.`)
	n := st.Len()

	temperatureLocation = ""
	weatherLocation = ""
	err := mdl.Generate(ctx, st, tools, opts)
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
	if (provider != "openai" && toolCalls != 2) || (provider == "openai" && toolCalls < 2) {
		t.Errorf("Generate(%s %s) tool call steps: got %d want 2", provider, name, toolCalls)
	}
	if (provider != "openai" && toolCalls != 2) || (provider == "openai" && toolCalls < 2) {
		t.Errorf("Generate(%s %s) tool output steps: got %d want 2", provider, name, toolOutputs)
	}
}

func TestMultiTool(t *testing.T) {
	testModels(t, testMultiTool, &config.Options{})
}

func TestMultiToolThinking(t *testing.T) {
	testModels(t, testMultiTool, &config.Options{IncludeThoughts: true})
}
