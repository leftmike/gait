package model

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/OpenRouterTeam/go-sdk/models/components"
	"google.golang.org/genai"
)

var testInput = json.RawMessage(`{"a":1}`)

func checkShape(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("shape:\n got  %v\n want %v", got, want)
	}
}

func checkTxtLen(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("txtLen: got %d want %d", got, want)
	}
}

func TestAnthropicToMessageParams(t *testing.T) {
	st := anthropicState{
		systemPrompt: "sys",
		steps: []anthropicStep{
			{typ: PromptStep, content: "p1"},
			{typ: ThinkingStep, content: "t1", signature: "s1"},
			{typ: ModelResponseStep, content: "r1"},
			{typ: PromptStep, content: "p2"},
			{typ: ThinkingStep, content: "t2", signature: "s2"},
			{typ: ToolCallStep, name: "f", id: "c1", input: testInput},
			{typ: ToolCallStep, name: "g", id: "c2", input: testInput},
			{typ: ToolOutputStep, name: "f", id: "c1", content: "o1"},
			{typ: ToolOutputStep, name: "g", id: "c2", content: "e1", isError: true},
			{typ: ModelResponseStep, content: "r2"},
		},
	}

	params, txtLen := st.toMessageParams()

	var shape []string
	for _, p := range params {
		var blks []string
		for _, b := range p.Content {
			switch {
			case b.OfText != nil:
				blks = append(blks, "text")
			case b.OfThinking != nil:
				blks = append(blks, "thinking")
			case b.OfToolUse != nil:
				blks = append(blks, "tool_use")
			case b.OfToolResult != nil:
				blks = append(blks, "tool_result")
			default:
				blks = append(blks, "unknown")
			}
		}
		shape = append(shape, string(p.Role)+":"+strings.Join(blks, ","))
	}
	checkShape(t, shape, []string{
		"user:text",
		"assistant:thinking,text",
		"user:text",
		"assistant:thinking,tool_use,tool_use",
		"user:tool_result,tool_result",
		"assistant:text",
	})
	checkTxtLen(t, txtLen, 30)

	if th := params[1].Content[0].OfThinking; th.Signature != "s1" || th.Thinking != "t1" {
		t.Errorf("thinking block: got %+v", th)
	}
	if tu := params[3].Content[2].OfToolUse; tu.ID != "c2" || tu.Name != "g" {
		t.Errorf("tool use block: got %+v", tu)
	}
	ok := params[4].Content[0].OfToolResult
	if ok.ToolUseID != "c1" || ok.IsError.Value {
		t.Errorf("tool result block: got %+v", ok)
	}
	bad := params[4].Content[1].OfToolResult
	if bad.ToolUseID != "c2" || !bad.IsError.Value {
		t.Errorf("error tool result block: got %+v", bad)
	}
}

func TestGoogleToContents(t *testing.T) {
	args := map[string]any{"a": 1}
	st := googleState{
		systemPrompt: "sys",
		steps: []googleStep{
			{typ: PromptStep, content: "p1"},
			{typ: ThinkingStep, content: "t1"},
			{typ: ModelResponseStep, content: "r1", thoughts: []byte("s1")},
			{typ: PromptStep, content: "p2"},
			{typ: ThinkingStep, content: "t2"},
			{typ: ToolCallStep, name: "f", id: "c1", input: testInput, args: args,
				thoughts: []byte("s2")},
			{typ: ToolCallStep, name: "g", id: "c2", input: testInput, args: args},
			{typ: ToolOutputStep, name: "f", id: "c1", content: "o1"},
			{typ: ToolOutputStep, name: "g", id: "c2", content: "e1", isError: true},
			{typ: ModelResponseStep, content: "r2"},
		},
	}

	cnts, txtLen := st.toContents()

	var shape []string
	for _, c := range cnts {
		var prts []string
		for _, p := range c.Parts {
			prts = append(prts, partType(p))
		}
		shape = append(shape, c.Role+":"+strings.Join(prts, ","))
	}
	checkShape(t, shape, []string{
		"user:text",
		"model:thought,text",
		"user:text",
		"model:thought,function call,function call",
		"user:function response,function response",
		"model:text",
	})
	checkTxtLen(t, txtLen, 30)

	if sig := string(cnts[1].Parts[1].ThoughtSignature); sig != "s1" {
		t.Errorf("response thought signature: got %q want s1", sig)
	}
	fc := cnts[3].Parts[1]
	if fc.FunctionCall.ID != "c1" || fc.FunctionCall.Name != "f" ||
		string(fc.ThoughtSignature) != "s2" {

		t.Errorf("function call: got %+v", fc)
	}
	if rsp := cnts[4].Parts[0].FunctionResponse.Response; rsp["output"] != "o1" {
		t.Errorf("function response: got %v", rsp)
	}
	if rsp := cnts[4].Parts[1].FunctionResponse.Response; rsp["error"] != "e1" {
		t.Errorf("error function response: got %v", rsp)
	}
}

func TestPartType(t *testing.T) {
	cases := []struct {
		prt  genai.Part
		want string
	}{
		{prt: genai.Part{MediaResolution: &genai.PartMediaResolution{}}, want: "media resolution"},
		{prt: genai.Part{CodeExecutionResult: &genai.CodeExecutionResult{}},
			want: "code execution result"},
		{prt: genai.Part{ExecutableCode: &genai.ExecutableCode{}}, want: "executable code"},
		{prt: genai.Part{FileData: &genai.FileData{}}, want: "file data"},
		{prt: genai.Part{FunctionCall: &genai.FunctionCall{}}, want: "function call"},
		{prt: genai.Part{FunctionResponse: &genai.FunctionResponse{}},
			want: "function response"},
		{prt: genai.Part{InlineData: &genai.Blob{}}, want: "inline data"},
		{prt: genai.Part{Text: "x", Thought: true}, want: "thought"},
		{prt: genai.Part{Text: "x"}, want: "text"},
		{prt: genai.Part{ThoughtSignature: []byte("s")}, want: "thought signature"},
		{prt: genai.Part{VideoMetadata: &genai.VideoMetadata{}}, want: "video metadata"},
		{prt: genai.Part{}, want: "--empty--"},
	}

	for _, c := range cases {
		if got := partType(&c.prt); got != c.want {
			t.Errorf("partType(%+v): got %q want %q", c.prt, got, c.want)
		}
	}
}

func TestOpenAIToInputItemList(t *testing.T) {
	st := openAIState{
		systemPrompt: "sys",
		steps: []openAIStep{
			{typ: PromptStep, content: "p1"},
			{typ: ThinkingStep, content: "t1", id: "rs1", encrypted: "enc1"},
			{typ: ModelResponseStep, content: "r1"},
			{typ: PromptStep, content: "p2"},
			{typ: ThinkingStep, content: "t2", id: "rs2", encrypted: "enc2"},
			{typ: ToolCallStep, name: "f", id: "c1", input: string(testInput)},
			{typ: ToolCallStep, name: "g", id: "c2", input: string(testInput)},
			{typ: ToolOutputStep, name: "f", id: "c1", content: "o1"},
			{typ: ToolOutputStep, name: "g", id: "c2", content: "e1"},
			{typ: ModelResponseStep, content: "r2"},
		},
	}

	lst, txtLen := st.toInputItemList()

	var shape []string
	for _, item := range lst {
		switch {
		case item.OfInputMessage != nil:
			shape = append(shape, "input_message:"+item.OfInputMessage.Role)
		case item.OfOutputMessage != nil:
			shape = append(shape, "output_message")
		case item.OfReasoning != nil:
			shape = append(shape, "reasoning")
		case item.OfFunctionCall != nil:
			shape = append(shape, "function_call")
		case item.OfFunctionCallOutput != nil:
			shape = append(shape, "function_call_output")
		default:
			shape = append(shape, "unknown")
		}
	}
	checkShape(t, shape, []string{
		"input_message:system",
		"input_message:user",
		"reasoning",
		"output_message",
		"input_message:user",
		"reasoning",
		"function_call",
		"function_call",
		"function_call_output",
		"function_call_output",
		"output_message",
	})
	checkTxtLen(t, txtLen, 33)

	rs := lst[2].OfReasoning
	if rs.ID != "rs1" || rs.EncryptedContent.Value != "enc1" || rs.Summary[0].Text != "t1" {
		t.Errorf("reasoning: got %+v", rs)
	}
	rs = lst[5].OfReasoning
	if rs.ID != "rs2" || rs.EncryptedContent.Value != "enc2" || rs.Summary[0].Text != "t2" {
		t.Errorf("reasoning before tool call: got %+v", rs)
	}
	if fc := lst[7].OfFunctionCall; fc.CallID != "c2" || fc.Name != "g" {
		t.Errorf("function call: got %+v", fc)
	}
	if out := lst[8].OfFunctionCallOutput; out.CallID != "c1" || out.Output.OfString.Value != "o1" {
		t.Errorf("function call output: got %+v", out)
	}
}

func TestOpenRouterToMessages(t *testing.T) {
	reasoning := []components.ReasoningDetailUnion{
		components.CreateReasoningDetailUnionReasoningSummary(
			components.ReasoningDetailSummary{Summary: "t"}),
	}
	st := openRouterState{
		systemPrompt: "sys",
		steps: []openRouterStep{
			{typ: PromptStep, content: "p1"},
			{typ: ThinkingStep, content: "t1", reasoning: reasoning},
			{typ: ModelResponseStep, content: "r1"},
			{typ: PromptStep, content: "p2"},
			{typ: ThinkingStep, content: "t2", reasoning: reasoning},
			{typ: ToolCallStep, name: "f", id: "c1", input: string(testInput)},
			{typ: ToolCallStep, name: "g", id: "c2", input: string(testInput)},
			{typ: ToolOutputStep, name: "f", id: "c1", content: "o1"},
			{typ: ToolOutputStep, name: "g", id: "c2", content: "e1"},
			{typ: ModelResponseStep, content: "r2"},
		},
	}

	msgs, txtLen := st.toMessages()

	var shape []string
	for _, m := range msgs {
		switch {
		case m.ChatSystemMessage != nil:
			shape = append(shape, "system")
		case m.ChatUserMessage != nil:
			shape = append(shape, "user")
		case m.ChatAssistantMessage != nil:
			s := "assistant"
			if len(m.ChatAssistantMessage.ReasoningDetails) > 0 {
				s += ":reasoning"
			}
			for range m.ChatAssistantMessage.ToolCalls {
				s += ":tool_call"
			}
			shape = append(shape, s)
		case m.ChatToolMessage != nil:
			shape = append(shape, "tool:"+m.ChatToolMessage.ToolCallID)
		default:
			shape = append(shape, "unknown")
		}
	}
	checkShape(t, shape, []string{
		"system",
		"user",
		"assistant:reasoning",
		"user",
		"assistant:reasoning:tool_call:tool_call",
		"tool:c1",
		"tool:c2",
		"assistant",
	})
	checkTxtLen(t, txtLen, 29)

	if tc := msgs[4].ChatAssistantMessage.ToolCalls[1]; tc.ID != "c2" || tc.Function.Name != "g" {
		t.Errorf("tool call: got %+v", tc)
	}
}

func chatAPITestState() chatAPIState {
	return chatAPIState{
		systemPrompt: "sys",
		steps: []chatAPIStep{
			{typ: PromptStep, content: "p1"},
			{typ: ThinkingStep, content: "t1"},
			{typ: ModelResponseStep, content: "r1"},
			{typ: PromptStep, content: "p2"},
			{typ: ThinkingStep, content: "t2"},
			{typ: ToolCallStep, name: "f", id: "c1", input: string(testInput)},
			{typ: ToolCallStep, name: "g", id: "c2", input: string(testInput)},
			{typ: ToolOutputStep, name: "f", id: "c1", content: "o1"},
			{typ: ToolOutputStep, name: "g", id: "c2", content: "e1"},
			{typ: ModelResponseStep, content: "r2"},
			{typ: PromptStep, content: "p3"},
			{typ: ModelResponseStep, content: "r3"},
			{typ: ToolCallStep, name: "f", id: "c3", input: string(testInput)},
			{typ: ToolOutputStep, name: "f", id: "c3", content: "o3"},
		},
	}
}

func TestChatAPIToMessages(t *testing.T) {
	st := chatAPITestState()
	msgs, txtLen := st.toMessages()

	var shape []string
	for _, m := range msgs {
		switch {
		case m.OfSystem != nil:
			shape = append(shape, "system")
		case m.OfUser != nil:
			shape = append(shape, "user")
		case m.OfAssistant != nil:
			s := "assistant"
			if m.OfAssistant.Content.OfString.Valid() {
				s += ":" + m.OfAssistant.Content.OfString.Value
			}
			for _, tc := range m.OfAssistant.ToolCalls {
				s += ":" + tc.OfFunction.ID
			}
			shape = append(shape, s)
		case m.OfTool != nil:
			shape = append(shape, "tool:"+m.OfTool.ToolCallID)
		default:
			shape = append(shape, "unknown")
		}
	}
	checkShape(t, shape, []string{
		"system",
		"user",
		"assistant:r1",
		"user",
		"assistant:c1:c2",
		"tool:c1",
		"tool:c2",
		"assistant:r2",
		"user",
		"assistant:r3:c3",
		"tool:c3",
	})
	checkTxtLen(t, txtLen, 21)
}

func TestToOllamaMessages(t *testing.T) {
	st := chatAPITestState()
	msgs, txtLen := st.toOllamaMessages()

	var shape []string
	for _, m := range msgs {
		s := m.Role
		if m.Thinking != "" {
			s += ":thinking=" + m.Thinking
		}
		if m.Content != "" {
			s += ":" + m.Content
		}
		for _, tc := range m.ToolCalls {
			s += ":" + tc.Function.Name
		}
		shape = append(shape, s)
	}
	checkShape(t, shape, []string{
		"system:sys",
		"user:p1",
		"assistant:thinking=t1:r1",
		"user:p2",
		"assistant:thinking=t2:f:g",
		"tool:o1",
		"tool:e1",
		"assistant:r2",
		"user:p3",
		"assistant:r3",
		"assistant:f",
		"tool:o3",
	})
	checkTxtLen(t, txtLen, 25)

	if v, ok := msgs[4].ToolCalls[0].Function.Arguments.Get("a"); !ok || v != float64(1) {
		t.Errorf("tool call arguments: got %v, %v", v, ok)
	}
}

func TestStepTypeString(t *testing.T) {
	cases := map[StepType]string{
		PromptStep:        "PromptStep",
		ModelResponseStep: "ModelResponseStep",
		ThinkingStep:      "ThinkingStep",
		ToolCallStep:      "ToolCallStep",
		ToolOutputStep:    "ToolOutputStep",
		StepType(99):      "StepType(99)",
	}
	for st, want := range cases {
		if got := st.String(); got != want {
			t.Errorf("StepType(%d).String(): got %q want %q", int(st), got, want)
		}
	}
}
