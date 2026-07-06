package llmreg

import (
	"encoding/json"
	"fmt"
	"sync"
)

type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Doc    string           `json:"doc"`
	NPM    string           `json:"npm"`
	Env    []string         `json:"env"`
	Models map[string]Model `json:"models"`
}

type Model struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Family      string     `json:"family"`
	Attachment  bool       `json:"attachment"`
	Reasoning   bool       `json:"reasoning"`
	ToolCall    bool       `json:"tool_call"`
	Knowledge   string     `json:"knowledge"`
	ReleaseDate string     `json:"release_date"`
	LastUpdated string     `json:"last_updated"`
	OpenWeights bool       `json:"open_weights"`
	Modalities  Modalities `json:"modalities"`
	Limit       Limits     `json:"limit"`
	Cost        Costs      `json:"cost"`
}

type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type Limits struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type Costs struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

var (
	providers    map[string]Provider
	providersErr error
	once         sync.Once
)

func Providers(refresh bool) (map[string]Provider, error) {
	once.Do(func() {
		var buf []byte
		buf, providersErr = fetchJSON("https://models.dev/api.json", "modelsdev.json", refresh)
		if providersErr == nil {
			providersErr = json.Unmarshal(buf, &providers)
		}
	})

	return providers, providersErr
}

func FindProvider(provider string) (Provider, error) {
	providers, err := Providers(false)
	if err != nil {
		return Provider{}, err
	}

	pvdr, ok := providers[provider]
	if !ok {
		return Provider{}, fmt.Errorf("provider not found: %s", provider)
	}

	return pvdr, nil
}
