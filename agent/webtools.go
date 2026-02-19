package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	htmlmarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/leftmike/gait/model"
)

const (
	maxWebResponseBody = 1024 * 128
)

type webFetchArgs struct {
	URL string `json:"url" gait:"the URL to fetch"`
}

func webFetch(ctx context.Context, buf []byte) (string, error) {
	var args webFetchArgs
	if err := json.Unmarshal(buf, &args); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
	if err != nil {
		return "", err
	}
	// XXX: update to user agent to be more generic
	req.Header.Set("User-Agent", "gait/0.1 (+https://github.com/leftmike/gait)")
	// XXX: should accept json at least
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

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

func (ag *Agent) AddWebFetchTool() {
	ag.AddTool("web_fetch", "fetches the content of a URL and returns it as text",
		webFetch, model.MustToolSchema[webFetchArgs]())
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

func makeWebSearch(apiKey string) model.ToolFunc {
	return func(ctx context.Context, buf []byte) (string, error) {
		var args webSearchArgs
		if err := json.Unmarshal(buf, &args); err != nil {
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

func (ag *Agent) AddWebSearchTool(apiKey string) {
	ag.AddTool("web_search", "searches the web using Brave Search and returns relevant results",
		makeWebSearch(apiKey), model.MustToolSchema[webSearchArgs]())
}
