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

func testModels(t *testing.T, test func(t *testing.T, mdl model.Model, provider, name string)) {
	t.Helper()

	cfg, err := config.ReadConfig([]string{"../gait.hcl"})
	if err != nil {
		t.Fatalf("ReadConfig() failed with %s", err)
	}

	models := []struct{ provider, name string }{
		{"openai", "gpt-5"},
		{"anthropic", "claude-3-7-sonnet-20250219"},
		{"gemini", "gemini-2.5-flash"},
	}

	for _, m := range models {
		if *provider != "" && *provider != m.provider {
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

		test(t, mdl, m.provider, m.name)
	}
}

func testSimple(t *testing.T, mdl model.Model, provider, name string) {
	fmt.Println(provider, name)

	ctx := context.Background()
	st := mdl.NewState()
	st.Prompt("Hello")
	n := st.Len()

	err := mdl.Generate(ctx, st, nil, &model.Options{})
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
	testModels(t, testSimple)
}

var (
	location string
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

	location = args.Location

	return "40", nil
}

func testSimpleTool(t *testing.T, mdl model.Model, provider, name string) {
	fmt.Println(provider, name)

	tools := model.Tools{
		{
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

	location = ""
	err := mdl.Generate(ctx, st, tools, &model.Options{})
	if err != nil {
		t.Errorf("Generate(%s, %s) failed with %s", provider, name, err)
	}

	if location == "" {
		t.Errorf("Generate(%s, %s) current weather not called", provider, name)
	} else if !strings.Contains(strings.ToLower(location), "seattle") {
		t.Errorf("Generate(%s %s) argument did not include seattle: %s", provider, name, location)
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
	testModels(t, testSimpleTool)
}
