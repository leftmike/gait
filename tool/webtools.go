package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	htmlmarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/leftmike/gait/system"
)

const (
	maxWebResponseBody = 1024 * 128
)

type webFetchArgs struct {
	URL string `json:"url" gait:"the URL to fetch"`
}

func webFetch(ctx context.Context, sys *system.System, buf []byte) (string, error) {
	var args webFetchArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AI-Agent/1.0)")
	req.Header.Set("Accept", "application/json,text/plain,text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", rsp.StatusCode, rsp.Status)
	}

	ct := rsp.Header.Get("Content-Type")
	r := io.LimitReader(rsp.Body, maxWebResponseBody)

	if strings.Contains(ct, "text/html") {
		md, err := htmlmarkdown.ConvertReader(r)
		if err != nil {
			return "", err
		}
		return string(md), nil
	}

	body, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

var WebFetch = Tool{
	Name:        "web_fetch",
	Description: "fetches the content of a URL and returns it as text",
	Func:        webFetch,
	Schema:      MustToolSchema[webFetchArgs](),
}

type webSearchArgs struct {
	Query string `json:"query" gait:"the search query"`
}

type braveWebResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type braveResponse struct {
	Web struct {
		Results []braveWebResult `json:"results"`
	} `json:"web"`
}

func makeWebSearch(apiKey string) ToolFunc {
	return func(ctx context.Context, sys *system.System, buf []byte) (string, error) {
		var args webSearchArgs
		err := json.Unmarshal(buf, &args)
		if err != nil {
			return "", err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			"https://api.search.brave.com/res/v1/web/search?q="+url.QueryEscape(args.Query), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Subscription-Token", apiKey)

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer rsp.Body.Close()

		if rsp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP %d: %s", rsp.StatusCode, rsp.Status)
		}

		body, err := io.ReadAll(io.LimitReader(rsp.Body, maxWebResponseBody))
		if err != nil {
			return "", err
		}

		var brave braveResponse
		err = json.Unmarshal(body, &brave)
		if err != nil {
			return "", err
		} else if len(brave.Web.Results) == 0 {
			return "No results found.", nil
		}

		var sb strings.Builder
		for _, ret := range brave.Web.Results {
			fmt.Fprintf(&sb, "- [%s](%s)\n  %s\n", ret.Title, ret.URL, ret.Description)
		}
		return strings.TrimSpace(sb.String()), nil
	}
}

func WebSearch(apiKey string) Tool {
	return Tool{
		Name:        "web_search",
		Description: "searches the web using Brave Search and returns relevant results",
		Func:        makeWebSearch(apiKey),
		Schema:      MustToolSchema[webSearchArgs](),
	}
}
