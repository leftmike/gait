package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/leftmike/sandbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/tool"
)

type Client struct {
	svrCfg    config.MCPServer
	verbose   bool
	clnt      *mcp.Client
	sess      *mcp.ClientSession
	retry     bool
	prompts   []*mcp.Prompt
	resources []*mcp.Resource
	tools     []*mcp.Tool
}

func NewClient(ctx context.Context, svrCfg config.MCPServer, verbose bool) (*Client, error) {
	switch svrCfg.Type {
	case "", "stdio":
		if svrCfg.Command == "" {
			return nil, fmt.Errorf("missing command for local MCP server: %s", svrCfg.Name)
		}
	case "sse", "http":
		if svrCfg.URL == "" {
			return nil, fmt.Errorf("missing URL for remote MCP server: %s", svrCfg.Name)
		}
	default:
		return nil, fmt.Errorf("unknown MCP server type: %s: %s", svrCfg.Name, svrCfg.Type)
	}

	clnt := Client{
		svrCfg:  svrCfg,
		verbose: verbose,
		clnt:    mcp.NewClient(&mcp.Implementation{Name: "gait", Version: "v0.1.0"}, nil),
	}

	err := clnt.WithSession(ctx, clnt.newClient)
	if err != nil {
		return nil, err
	}

	return &clnt, nil
}

func (clnt *Client) newClient(ctx context.Context, sess *mcp.ClientSession) error {
	ir := sess.InitializeResult()
	if ir.Capabilities.Prompts != nil {
		ret, err := sess.ListPrompts(ctx, nil)
		if err != nil {
			return err
		}
		clnt.prompts = ret.Prompts
	}
	if ir.Capabilities.Resources != nil {
		ret, err := sess.ListResources(ctx, nil)
		if err != nil {
			return err
		}
		clnt.resources = ret.Resources
	}
	if ir.Capabilities.Tools != nil {
		ret, err := sess.ListTools(ctx, nil)
		if err != nil {
			return err
		}
		clnt.tools = ret.Tools
	}

	return nil
}

func (clnt *Client) Close() {
	if clnt.sess != nil {
		err := clnt.sess.Close()
		if clnt.verbose && err != nil {
			fmt.Printf("session close: %s\n", err)
		}

		clnt.sess = nil
	}
}

func (clnt *Client) transport() mcp.Transport {
	switch clnt.svrCfg.Type {
	case "", "stdio":
		return &mcp.CommandTransport{
			Command: exec.Command(clnt.svrCfg.Command, clnt.svrCfg.Args...),
		}
	case "sse":
		return &mcp.SSEClientTransport{
			Endpoint: clnt.svrCfg.URL,
		}
	case "http":
		return &mcp.StreamableClientTransport{
			Endpoint: clnt.svrCfg.URL,
		}
	default:
		panic(fmt.Sprintf("unexpected MCP server type: %s", clnt.svrCfg.Type))
	}
}

func (clnt *Client) WithSession(ctx context.Context,
	with func(ctx context.Context, sess *mcp.ClientSession) error) error {

	if clnt.sess != nil && clnt.sess.Ping(ctx, nil) != nil {
		clnt.sess.Close()
		clnt.sess = nil
	}

	if clnt.sess == nil {
		backoff := 250 * time.Millisecond
		for {
			var err error
			clnt.sess, err = clnt.clnt.Connect(ctx, clnt.transport(), nil)
			if err == nil {
				break
			} else if !clnt.retry {
				return err
			}

			fmt.Printf("with session: backoff: %dms: %s", backoff, err)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff = min(backoff*2, 30*time.Second)
			}
		}
	}

	clnt.retry = true
	return with(ctx, clnt.sess)
}

func (clnt *Client) Name() string {
	return clnt.svrCfg.Name
}

func (clnt *Client) AddTools(tools map[string]tool.Tool) {
	for _, tl := range clnt.tools {
		schema, ok := tl.InputSchema.(map[string]any)
		if !ok {
			schema = map[string]any{}
		}

		name := tl.Name
		tools[name] = tool.Tool{
			Name:        name,
			Description: tl.Description,
			Schema:      schema,
			Func: func(ctx context.Context, _ *sandbox.Sandbox, buf []byte) (string, error) {
				var s string
				err := clnt.WithSession(ctx,
					func(ctx context.Context, sess *mcp.ClientSession) error {
						var args map[string]any
						err := json.Unmarshal(buf, &args)
						if err != nil {
							return err
						}

						ret, err := sess.CallTool(ctx,
							&mcp.CallToolParams{Name: name, Arguments: args})
						if err != nil {
							return err
						}

						var lines []string
						for _, cnt := range ret.Content {
							if tc, ok := cnt.(*mcp.TextContent); ok {
								lines = append(lines, tc.Text)
							}
						}

						if ret.IsError {
							return errors.New(strings.Join(lines, "\n"))
						}
						s = strings.Join(lines, "\n")
						return nil
					})

				if err != nil {
					return "", err
				}
				return s, nil
			},
		}
	}
}
