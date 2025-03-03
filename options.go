package main

import (
	"errors"
	"flag"
	"fmt"
	"reflect"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

var (
	intModelOptions = map[string]*struct {
		usage string
		with  func(int) llms.CallOption
		val   int
	}{
		"candidate_count": {
			usage: " number of response candidates to generate",
			with:  llms.WithCandidateCount,
		},
		"max_tokens": {usage: "maximum tokens to generate", with: llms.WithMaxTokens},
		"top_k":      {usage: "use top-k sampling", with: llms.WithTopK},
		"seed":       {usage: "seed to use for deterministic sampling", with: llms.WithSeed},
		"min_length": {usage: "minimum length of generated text", with: llms.WithMinLength},
		"max_length": {usage: "maximum length of generated text", with: llms.WithMaxLength},
		"n": {
			usage: "how many chat completion choices to generate for each input message",
			with:  llms.WithN,
		},
	}

	float64ModelOptions = map[string]*struct {
		usage string
		with  func(float64) llms.CallOption
		val   float64
	}{
		"temperature": {usage: "temperature", with: llms.WithTemperature},
		"top_p":       {usage: "use top-p sampling", with: llms.WithTopP},
		"repetition_penalty": {
			usage: "repetition penalty for sampling",
			with:  llms.WithRepetitionPenalty,
		},
		"frequency_penalty": {
			usage: "frequency penalty for sampling",
			with:  llms.WithFrequencyPenalty,
		},
		"presence_penalty": {
			usage: "presence penalty for sampling",
			with:  llms.WithPresencePenalty,
		},
	}

	verbose bool
)

func optionalFields(typ reflect.Type, field func(fdx int, name string)) {
	num := typ.NumField()
	for fdx := 0; fdx < num; fdx += 1 {
		fld := typ.Field(fdx)
		tags := strings.Split(fld.Tag.Get("hcl"), ",")
		if len(tags) != 2 || tags[1] != "optional" {
			continue
		}

		field(fdx, tags[0])
	}
}

func options(cfg *Config) (string, string, string, []llms.CallOption, error) {
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.BoolVar(&verbose, "v", false, "verbose output")

	providerFlag := flag.String("provider", "", "generate using this llm `provider`")
	modelFlag := flag.String("model", "", "generate using this llm `model`")
	apikeyFlag := flag.String("apikey", "", "`api key` to use")

	optionalFields(reflect.ValueOf(Model{}).Type(),
		func(fdx int, name string) {
			if opt, ok := intModelOptions[name]; ok {
				flag.IntVar(&opt.val, name, 0, opt.usage)
			} else if opt, ok := float64ModelOptions[name]; ok {
				flag.Float64Var(&opt.val, name, 0.0, opt.usage)
			} else {
				panic(fmt.Sprintf("missing model option: %s", name))
			}
		})

	flag.Parse()

	provider := *providerFlag
	if provider == "" {
		if cfg.Provider == "" {
			return "", "", "", nil, errors.New("no provider specified")
		}

		provider = cfg.Provider
	}
	p := cfg.FindProvider(provider)

	apikey := *apikeyFlag
	if apikey == "" {
		if p.APIKey == "" {
			return "", "", "", nil, fmt.Errorf("provider missing API key: %s", provider)
		}

		apikey = p.APIKey
	}

	model := *modelFlag
	if model == "" {
		if p.Model == "" {
			return "", "", "", nil, fmt.Errorf("no model specified: %s", provider)
		}

		model = p.Model
	}
	m := cfg.FindModel(provider, model)

	var opts []llms.CallOption

	val := reflect.ValueOf(m)
	optionalFields(val.Type(),
		func(fdx int, name string) {
			if opt, ok := intModelOptions[name]; ok {
				if opt.val != 0 {
					opts = append(opts, opt.with(opt.val))
					if verbose {
						fmt.Printf("flag: %s: %v\n", name, opt.val)
					}
				} else if ip := val.Field(fdx).Interface().(*int); ip != nil && *ip != 0 {
					opts = append(opts, opt.with(*ip))
					if verbose {
						fmt.Printf("config: %s: %v\n", name, *ip)
					}
				}
			} else if opt, ok := float64ModelOptions[name]; ok {
				if opt.val != 0.0 {
					opts = append(opts, opt.with(opt.val))
					if verbose {
						fmt.Printf("flag: %s: %v\n", name, opt.val)
					}
				} else if fp := val.Field(fdx).Interface().(*float64); fp != nil && *fp != 0.0 {
					opts = append(opts, opt.with(*fp))
					if verbose {
						fmt.Printf("config: %s: %v\n", name, *fp)
					}
				}
			} else {
				panic(fmt.Sprintf("missing model option: %s", name))
			}
		})

	return provider, model, apikey, opts, nil
}
