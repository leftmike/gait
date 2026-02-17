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

type testModelFunc func(t *testing.T, mdl model.Model, provider, name string, opts *model.Options)

func testModels(t *testing.T, test testModelFunc, opts *model.Options) {
	t.Helper()

	cfg, err := config.ReadConfig([]string{"./gait.hcl", "../gait.hcl"})
	if err != nil {
		t.Fatalf("ReadConfig() failed with %s", err)
	}

	models := []struct {
		provider, name string
		short          bool
	}{
		{"openai", "gpt-5-mini", false},
		{"anthropic", "claude-haiku-4-5-20251001", false},
		{"gemini", "gemini-2.5-flash-lite", true},
	}

	for _, m := range models {
		if *provider != "" && *provider != m.provider {
			continue
		} else if testing.Short() && !m.short {
			fmt.Printf("skipping %s %s\n", m.provider, m.name)
			continue
		}

		p := cfg.FindProvider(m.provider)
		if p == nil || p.APIKey == "" {
			t.Fatalf("missing api key for provider: %s", m.provider)
		}

		var mdl model.Model
		switch m.provider {
		case "openai":
			mdl, err = model.NewOpenAIModel(m.name, p.APIKey, &model.Options{})
			if err != nil {
				t.Fatalf("NewOpenAIModel(%s) failed with %s", m.name, err)
			}
		case "anthropic":
			mdl, err = model.NewAnthropicModel(m.name, p.APIKey, &model.Options{})
			if err != nil {
				t.Fatalf("NewAnthropicModel(%s) failed with %s", m.name, err)
			}
		case "gemini":
			mdl, err = model.NewGeminiModel(m.name, p.APIKey, &model.Options{})
			if err != nil {
				t.Fatalf("NewGeminiModel(%s) failed with %s", m.name, err)
			}
		default:
			t.Fatalf("unknown provider: %s", m.provider)
		}

		test(t, mdl, m.provider, m.name, opts)
	}
}

func testSimple(t *testing.T, mdl model.Model, provider, name string, opts *model.Options) {
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
	testModels(t, testSimple, &model.Options{})
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

func testSimpleTool(t *testing.T, mdl model.Model, provider, name string, opts *model.Options) {
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
	st.Prompt("what is the current temperature for seattle?")
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
	testModels(t, testSimpleTool, &model.Options{})
}

func testMultiTool(t *testing.T, mdl model.Model, provider, name string, opts *model.Options) {
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
	st.Prompt("what is the current temperature and weather for seattle?")
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
	testModels(t, testMultiTool, &model.Options{})
}

func TestMultiToolThinking(t *testing.T) {
	testModels(t, testMultiTool, &model.Options{Thinking: true})
}
