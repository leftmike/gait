package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/sandbox"
	"github.com/leftmike/gait/tool"
)

type fakeServer struct {
	*httptest.Server
	mutex     sync.Mutex
	responses []string
	status    int
	requests  []map[string]any
}

func newFakeServer(t *testing.T, status int, responses ...string) *fakeServer {
	t.Helper()

	fs := &fakeServer{responses: responses, status: status}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tags", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"models":[
{"name":"m","model":"m","capabilities":["completion","tools","thinking"],
	"details":{"context_length":4096}},
{"name":"nt","model":"nt","capabilities":["completion"]}]}`)
	})
	mux.HandleFunc("/", fs.handle)
	fs.Server = httptest.NewServer(mux)
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fakeServer) handle(w http.ResponseWriter, r *http.Request) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fs.requests = append(fs.requests, req)

	if len(fs.responses) == 0 {
		http.Error(w, `{"error":"no more responses"}`, http.StatusBadRequest)
		return
	}
	rsp := fs.responses[0]
	fs.responses = fs.responses[1:]

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(fs.status)
	fmt.Fprintln(w, rsp)
}

func (fs *fakeServer) request(t *testing.T, n int) map[string]any {
	t.Helper()

	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	if n >= len(fs.requests) {
		t.Fatalf("request %d: only %d requests", n, len(fs.requests))
	}
	return fs.requests[n]
}

func messageRoles(t *testing.T, req map[string]any) []string {
	t.Helper()

	msgs, ok := req["messages"].([]any)
	if !ok {
		t.Fatalf("request missing messages: %v", req)
	}
	var roles []string
	for _, m := range msgs {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	return roles
}

type fakeTool struct {
	mutex    sync.Mutex
	location string
	err      error
}

func (ft *fakeTool) tools() map[string]tool.Tool {
	return map[string]tool.Tool{
		"current_temperature": {
			Name:        "current_temperature",
			Description: "Gets the current temperature for the given location",
			Func: func(ctx context.Context, sb sandbox.Sandbox, buf []byte) (string, error) {
				var args struct {
					Location string `json:"location"`
				}
				if err := json.Unmarshal(buf, &args); err != nil {
					return "", err
				}

				ft.mutex.Lock()
				defer ft.mutex.Unlock()
				ft.location = args.Location
				if ft.err != nil {
					return "", ft.err
				}
				return "40", nil
			},
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{"type": "string"},
				},
				"required": []any{"location"},
			},
		},
	}
}

func newFakeModel(t *testing.T, clnt Client, mdlCfg config.ModelConfig, ft *fakeTool) Model {
	t.Helper()

	mdl, err := clnt.NewModel(mdlCfg)
	if err != nil {
		t.Fatalf("NewModel() failed with %s", err)
	}

	sb, err := sandbox.Open(context.Background(), "none", sandbox.Policy{}, nil)
	if err != nil {
		t.Fatalf("NewSandbox() failed with %s", err)
	}
	err = mdl.SetTools(ft.tools(), sb)
	if err != nil {
		t.Fatalf("SetTools() failed with %s", err)
	}
	return mdl
}

func stepTypes(st State) []StepType {
	var types []StepType
	for n := range st.Len() {
		types = append(types, st.Step(n).Type)
	}
	return types
}

func checkSteps(t *testing.T, st State, want ...StepType) {
	t.Helper()

	if got := stepTypes(st); !slices.Equal(got, want) {
		t.Errorf("steps: got %v want %v", got, want)
	}
}

const (
	chatToolCallResponse = `{"id":"1","object":"chat.completion","created":0,"model":"m",
"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,
"tool_calls":[{"id":"c1","type":"function","function":{"name":"current_temperature",
"arguments":"{\"location\":\"Seattle\"}"}}]}}],
"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	chatTextResponse = `{"id":"2","object":"chat.completion","created":0,"model":"m",
"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"It is 40"}}],
"usage":{"prompt_tokens":20,"completion_tokens":7,"total_tokens":27}}`
	chatLengthResponse = `{"id":"3","object":"chat.completion","created":0,"model":"m",
"choices":[{"index":0,"finish_reason":"length","message":{"role":"assistant","content":"It"}}],
"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	chatNoChoicesResponse = `{"id":"4","object":"chat.completion","created":0,"model":"m",
"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`
)

func newFakeLlamaCpp(t *testing.T, fs *fakeServer) Client {
	t.Helper()

	clnt, err := NewClient(&config.ClientConfig{Provider: "llamacpp", BaseURL: fs.URL})
	if err != nil {
		t.Fatalf("NewClient() failed with %s", err)
	}
	return clnt
}

func TestLlamaCppToolLoop(t *testing.T) {
	t.Parallel()

	fs := newFakeServer(t, http.StatusOK, chatToolCallResponse, chatTextResponse)
	clnt := newFakeLlamaCpp(t, fs)
	if clnt.Provider() != "llamacpp" {
		t.Errorf("Provider(): got %s want llamacpp", clnt.Provider())
	}

	var ft fakeTool
	mdl := newFakeModel(t, clnt, config.ModelConfig{Model: "m"}, &ft)

	st := clnt.NewState()
	st.SystemPrompt("be brief")
	st.Prompt("What is the temperature in Seattle?")
	err := mdl.Generate(context.Background(), st, &Options{})
	if err != nil {
		t.Fatalf("Generate() failed with %s", err)
	}

	checkSteps(t, st, PromptStep, ToolCallStep, ToolOutputStep, ModelResponseStep)
	if ft.location != "Seattle" {
		t.Errorf("tool location: got %q want Seattle", ft.location)
	}
	if step := st.Step(2); step.Content != "40" {
		t.Errorf("tool output: got %q want 40", step.Content)
	}
	if step := st.Step(3); step.Content != "It is 40" {
		t.Errorf("response: got %q want It is 40", step.Content)
	}

	req := fs.request(t, 0)
	if req["model"] != "m" {
		t.Errorf("request model: got %v want m", req["model"])
	}
	if tools, _ := req["tools"].([]any); len(tools) != 1 {
		t.Errorf("request tools: got %v", req["tools"])
	}
	if roles := messageRoles(t, req); !slices.Equal(roles, []string{"system", "user"}) {
		t.Errorf("first request roles: got %v", roles)
	}

	req = fs.request(t, 1)
	roles := messageRoles(t, req)
	if !slices.Equal(roles, []string{"system", "user", "assistant", "tool"}) {
		t.Errorf("second request roles: got %v", roles)
	}
	toolMsg := req["messages"].([]any)[3].(map[string]any)
	if toolMsg["tool_call_id"] != "c1" || toolMsg["content"] != "40" {
		t.Errorf("tool message: got %v", toolMsg)
	}

	in, out, ctx := st.Usage()
	if in != 30 || out != 12 || ctx != 27 {
		t.Errorf("Usage(): got %d, %d, %d want 30, 12, 27", in, out, ctx)
	}

	st.Clear()
	if st.Len() != 0 {
		t.Errorf("Clear(): Len() got %d want 0", st.Len())
	}
}

func TestLlamaCppToolError(t *testing.T) {
	t.Parallel()

	fs := newFakeServer(t, http.StatusOK, chatToolCallResponse, chatTextResponse)
	clnt := newFakeLlamaCpp(t, fs)
	ft := fakeTool{err: errors.New("sensor offline")}
	mdl := newFakeModel(t, clnt, config.ModelConfig{Model: "m"}, &ft)

	st := clnt.NewState()
	st.Prompt("What is the temperature in Seattle?")
	err := mdl.Generate(context.Background(), st, &Options{})
	if err != nil {
		t.Fatalf("Generate() failed with %s", err)
	}

	checkSteps(t, st, PromptStep, ToolCallStep, ToolOutputStep, ModelResponseStep)
	if step := st.Step(2); step.Content != "error: sensor offline" {
		t.Errorf("tool output: got %q want error: sensor offline", step.Content)
	}
}

func TestLlamaCppErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		rsp    string
		want   string
	}{
		{name: "length", status: http.StatusOK, rsp: chatLengthResponse, want: "max tokens"},
		{name: "no choices", status: http.StatusOK, rsp: chatNoChoicesResponse,
			want: "no choices"},
		{name: "bad request", status: http.StatusBadRequest,
			rsp: `{"error":{"message":"bad model"}}`, want: "400"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			fs := newFakeServer(t, c.status, c.rsp)
			clnt := newFakeLlamaCpp(t, fs)
			mdl := newFakeModel(t, clnt, config.ModelConfig{Model: "m"}, &fakeTool{})

			st := clnt.NewState()
			st.Prompt("hello")
			err := mdl.Generate(context.Background(), st, &Options{})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Generate(): got %v want error containing %q", err, c.want)
			}
		})
	}
}

func TestLlamaCppNewModel(t *testing.T) {
	t.Parallel()

	clnt := newFakeLlamaCpp(t, newFakeServer(t, http.StatusOK))
	_, err := clnt.NewModel(config.ModelConfig{Model: "m", Effort: "high"})
	if err == nil {
		t.Error("NewModel(effort: high) did not fail")
	}
	_, err = clnt.NewModel(config.ModelConfig{Model: "m", Effort: "default"})
	if err != nil {
		t.Errorf("NewModel(effort: default) failed with %s", err)
	}
}

const (
	ollamaToolCallResponse = `{"model":"m","message":{"role":"assistant","thinking":"hmm",` +
		`"tool_calls":[{"function":{"name":"current_temperature",` +
		`"arguments":{"location":"Seattle"}}}]},"done":true,"done_reason":"stop",` +
		`"prompt_eval_count":10,"eval_count":5}`
	ollamaTextResponse = `{"model":"m","message":{"role":"assistant","content":"It is 40"},` +
		`"done":true,"done_reason":"stop","prompt_eval_count":20,"eval_count":7}`
	ollamaLengthResponse = `{"model":"m","message":{"role":"assistant","content":"It"},` +
		`"done":true,"done_reason":"length","prompt_eval_count":1,"eval_count":1}`
)

func newFakeOllama(t *testing.T, fs *fakeServer) Client {
	t.Helper()

	clnt, err := NewClient(&config.ClientConfig{Provider: "ollama", BaseURL: fs.URL})
	if err != nil {
		t.Fatalf("NewClient() failed with %s", err)
	}
	return clnt
}

func TestOllamaListModels(t *testing.T) {
	t.Parallel()

	clnt := newFakeOllama(t, newFakeServer(t, http.StatusOK))
	mdls := clnt.ListModels()
	if len(mdls) != 1 {
		t.Fatalf("ListModels(): got %v want only m", mdls)
	}
	mmd := mdls["m"]
	if !mmd.Reasoning || mmd.ContextLimit != 4096 {
		t.Errorf("ListModels()[m]: got %+v", mmd)
	}
}

func TestOllamaToolLoop(t *testing.T) {
	t.Parallel()

	fs := newFakeServer(t, http.StatusOK, ollamaToolCallResponse, ollamaTextResponse)
	clnt := newFakeOllama(t, fs)

	var ft fakeTool
	mdl := newFakeModel(t, clnt,
		config.ModelConfig{Model: "m", IncludeThoughts: true, MaxTokens: 100}, &ft)
	if mdl.ContextLimit() != 4096 {
		t.Errorf("ContextLimit(): got %d want 4096", mdl.ContextLimit())
	}

	st := clnt.NewState()
	st.SystemPrompt("be brief")
	st.Prompt("What is the temperature in Seattle?")
	err := mdl.Generate(context.Background(), st, &Options{})
	if err != nil {
		t.Fatalf("Generate() failed with %s", err)
	}

	checkSteps(t, st, PromptStep, ThinkingStep, ToolCallStep, ToolOutputStep,
		ModelResponseStep)
	if ft.location != "Seattle" {
		t.Errorf("tool location: got %q want Seattle", ft.location)
	}
	if step := st.Step(2); step.Name != "current_temperature" ||
		string(step.Input) != `{"location":"Seattle"}` {

		t.Errorf("tool call step: got %+v", step)
	}

	req := fs.request(t, 0)
	if req["think"] != true {
		t.Errorf("request think: got %v want true", req["think"])
	}
	if opts, _ := req["options"].(map[string]any); opts["num_predict"] != float64(100) {
		t.Errorf("request options: got %v", req["options"])
	}
	if tools, _ := req["tools"].([]any); len(tools) != 1 {
		t.Errorf("request tools: got %v", req["tools"])
	}

	req = fs.request(t, 1)
	roles := messageRoles(t, req)
	if !slices.Equal(roles, []string{"system", "user", "assistant", "tool"}) {
		t.Errorf("second request roles: got %v", roles)
	}
	asst := req["messages"].([]any)[2].(map[string]any)
	if asst["thinking"] != "hmm" {
		t.Errorf("assistant thinking: got %v want hmm", asst["thinking"])
	}
	if tcs, _ := asst["tool_calls"].([]any); len(tcs) != 1 {
		t.Errorf("assistant tool calls: got %v", asst["tool_calls"])
	}

	in, out, ctx := st.Usage()
	if in != 30 || out != 12 || ctx != 27 {
		t.Errorf("Usage(): got %d, %d, %d want 30, 12, 27", in, out, ctx)
	}
}

func TestOllamaErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		rsp    string
		want   string
	}{
		{name: "length", status: http.StatusOK, rsp: ollamaLengthResponse, want: "max tokens"},
		{name: "server error", status: http.StatusInternalServerError,
			rsp: `{"error":"model crashed"}`, want: "model crashed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			fs := newFakeServer(t, c.status, c.rsp)
			clnt := newFakeOllama(t, fs)
			mdl := newFakeModel(t, clnt, config.ModelConfig{Model: "m"}, &fakeTool{})

			st := clnt.NewState()
			st.Prompt("hello")
			err := mdl.Generate(context.Background(), st, &Options{})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Generate(): got %v want error containing %q", err, c.want)
			}
		})
	}
}

func TestOllamaNewModel(t *testing.T) {
	t.Parallel()

	clnt := newFakeOllama(t, newFakeServer(t, http.StatusOK))

	cases := []struct {
		mdlCfg config.ModelConfig
		fail   bool
	}{
		{mdlCfg: config.ModelConfig{Model: "unknown"}, fail: true},
		{mdlCfg: config.ModelConfig{Model: "m", Effort: "extreme"}, fail: true},
		{mdlCfg: config.ModelConfig{Model: "m", Effort: "low"}},
		{mdlCfg: config.ModelConfig{Model: "m", Effort: "default"}},
	}

	for _, c := range cases {
		_, err := clnt.NewModel(c.mdlCfg)
		if c.fail && err == nil {
			t.Errorf("NewModel(%+v) did not fail", c.mdlCfg)
		} else if !c.fail && err != nil {
			t.Errorf("NewModel(%+v) failed with %s", c.mdlCfg, err)
		}
	}
}
