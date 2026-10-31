package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/tool"
)

type echoArgs struct {
	Text string `json:"text"`
}

func newTestServer(t *testing.T, withTools bool) string {
	t.Helper()

	svr := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	if withTools {
		sdk.AddTool(svr, &sdk.Tool{Name: "echo", Description: "echo text"},
			func(ctx context.Context, req *sdk.CallToolRequest, args echoArgs) (*sdk.CallToolResult,
				any, error) {

				return &sdk.CallToolResult{
					Content: []sdk.Content{&sdk.TextContent{Text: args.Text}},
				}, nil, nil
			})
		sdk.AddTool(svr, &sdk.Tool{Name: "fail", Description: "always fails"},
			func(ctx context.Context, req *sdk.CallToolRequest, args struct{}) (*sdk.CallToolResult,
				any, error) {

				return nil, nil, errors.New("boom")
			})
	}

	hs := httptest.NewServer(sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return svr }, nil))
	t.Cleanup(hs.Close)
	return hs.URL
}

func newTestClient(t *testing.T, withTools bool) *Client {
	t.Helper()

	clnt, err := NewClient(t.Context(),
		config.MCPServer{Name: "test", Type: "http", URL: newTestServer(t, withTools)}, false)
	if err != nil {
		t.Fatalf("NewClient() failed with %s", err)
	}
	t.Cleanup(clnt.Close)
	return clnt
}

func callTool(t *testing.T, tools map[string]tool.Tool, name, args string) (string, error) {
	t.Helper()

	tl, ok := tools[name]
	if !ok {
		t.Fatalf("tool %s not found", name)
	}
	return tl.Func(t.Context(), nil, []byte(args))
}

func TestNewClientConfig(t *testing.T) {
	cases := []struct {
		cfg config.MCPServer
		err string
	}{
		{cfg: config.MCPServer{Name: "a"}, err: "missing command"},
		{cfg: config.MCPServer{Name: "b", Type: "stdio"}, err: "missing command"},
		{cfg: config.MCPServer{Name: "c", Type: "sse"}, err: "missing URL"},
		{cfg: config.MCPServer{Name: "d", Type: "http"}, err: "missing URL"},
		{cfg: config.MCPServer{Name: "e", Type: "bogus"}, err: "unknown MCP server type"},
	}

	for _, c := range cases {
		_, err := NewClient(t.Context(), c.cfg, false)
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("NewClient(%+v) got %v, want %q", c.cfg, err, c.err)
		}
	}
}

func TestAddTools(t *testing.T) {
	clnt := newTestClient(t, true)
	if clnt.Name() != "test" {
		t.Errorf("Name() got %q, want %q", clnt.Name(), "test")
	}

	tools := map[string]tool.Tool{}
	clnt.AddTools(tools)
	if len(tools) != 2 {
		t.Fatalf("AddTools() got %d tools, want 2", len(tools))
	}

	echo := tools["echo"]
	if echo.Name != "echo" || echo.Description != "echo text" {
		t.Errorf("echo tool got %q %q", echo.Name, echo.Description)
	}
	props, ok := echo.Schema["properties"].(map[string]any)
	if !ok || props["text"] == nil {
		t.Errorf("echo schema missing text property: %v", echo.Schema)
	}
}

func TestCallTool(t *testing.T) {
	clnt := newTestClient(t, true)
	tools := map[string]tool.Tool{}
	clnt.AddTools(tools)

	s, err := callTool(t, tools, "echo", `{"text":"hi"}`)
	if err != nil {
		t.Errorf("echo failed with %s", err)
	} else if s != "hi" {
		t.Errorf("echo got %q, want %q", s, "hi")
	}

	_, err = callTool(t, tools, "fail", `{}`)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("fail got %v, want boom", err)
	}

	_, err = callTool(t, tools, "echo", `not json`)
	if err == nil {
		t.Error("echo with invalid json did not fail")
	}
}

func TestNoTools(t *testing.T) {
	clnt := newTestClient(t, false)
	tools := map[string]tool.Tool{}
	clnt.AddTools(tools)
	if len(tools) != 0 {
		t.Errorf("AddTools() got %d tools, want 0", len(tools))
	}
}

func TestReconnect(t *testing.T) {
	clnt := newTestClient(t, true)
	tools := map[string]tool.Tool{}
	clnt.AddTools(tools)

	clnt.Close()

	s, err := callTool(t, tools, "echo", `{"text":"again"}`)
	if err != nil {
		t.Errorf("echo after Close failed with %s", err)
	} else if s != "again" {
		t.Errorf("echo got %q, want %q", s, "again")
	}
}
