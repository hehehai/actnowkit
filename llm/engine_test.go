package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestEngineBackend(t *testing.T) {
	var gotParams GenerateParams
	call := func(ctx context.Context, method string, params any, notify func(string, json.RawMessage)) (json.RawMessage, error) {
		if method != MethodGenerate {
			t.Fatalf("method %s", method)
		}
		b, _ := json.Marshal(params)
		_ = json.Unmarshal(b, &gotParams)
		if notify != nil {
			notify("job.progress", json.RawMessage(`{"fraction":0.5}`))
			notify(NotifyToken, json.RawMessage(`{"text":"你"}`))
			notify(NotifyToken, json.RawMessage(`{"text":"好"}`))
		}
		return json.RawMessage(`{"text":"你好","finishReason":"stop","promptTokens":4,"completionTokens":2,"seconds":0.5,"tokensPerSecond":40}`), nil
	}
	eb := &EngineBackend{Call: call, DefaultModel: "mlx-community/Qwen3-4B-4bit"}
	c := &Client{Backend: eb, Sleep: noSleep, Defaults: testDefaults}
	var deltas string
	resp, err := c.Chat(context.Background(), Request{Skill: skillTranslate, Messages: []Message{{Role: User, Content: "x"}}, OnDelta: func(s string) { deltas += s }})
	if err != nil || resp.Text != "你好" || deltas != "你好" || resp.Usage.TotalTokens != 6 || resp.Provider != "local" || resp.TokensPerSecond != 40 || resp.Seconds != 0.5 || resp.Model != "mlx-community/Qwen3-4B-4bit" {
		t.Fatalf("%v %+v %q", err, resp, deltas)
	}
	if gotParams.Model != "mlx-community/Qwen3-4B-4bit" || !gotParams.Stream || *gotParams.Temperature != 0.2 || gotParams.MaxTokens != 1024 {
		t.Fatalf("params %+v", gotParams)
	}
	// The caller's errors map to codes by the app's sentinels.
	eb.Call = func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error) {
		return nil, fmt.Errorf("weights: %w", errNotDownloaded)
	}
	if _, err := c.Chat(context.Background(), Request{}); Code(err) != CodeModelNotFound || !errors.Is(err, errNotDownloaded) {
		t.Fatal(err)
	}
	for cause, want := range map[error]string{errLicence: CodeUnsupported, errUnsupported: CodeUnsupported, errBusy: CodeBusy,
		errEngine: CodeUnavailable, errCancelled: CodeCancelled, errors.New("odd"): CodeServer} {
		eb.Call = func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error) {
			return nil, cause
		}
		if _, err := eb.Chat(context.Background(), Request{}); Code(err) != want || !errors.Is(err, cause) {
			t.Errorf("%v → %v, want %s", cause, err, want)
		}
	}
	n := 0
	eb.Call = func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error) {
		n++
		if n == 1 {
			return nil, fmt.Errorf("worker restarting: %w", errEngine)
		}
		return json.RawMessage(`{"text":"ok"}`), nil
	}
	if resp, err := c.Chat(context.Background(), Request{}); err != nil || resp.Attempts != 2 {
		t.Fatal(err, resp.Attempts)
	}
	// An unreadable result, no model, no caller.
	eb.Call = func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error) {
		return json.RawMessage(`"x"`), nil
	}
	if _, err := eb.Chat(context.Background(), Request{}); Code(err) != CodeBadResponse {
		t.Error(err)
	}
	if _, err := (&EngineBackend{Call: eb.Call}).Chat(context.Background(), Request{}); Code(err) != CodeConfig || !errors.Is(err, errInvalid) {
		t.Error(err)
	}
	if _, err := (&EngineBackend{ID: "e"}).Chat(context.Background(), Request{}); Code(err) != CodeUnavailable || Param(err, "provider") != "e" {
		t.Error(err)
	}
}

// The engine's typed errors map to llm codes, and JSON requests carry
// their schema to the engine.
func TestEngineBackendProtocol(t *testing.T) {
	var got GenerateParams
	var failWith error
	eb := &EngineBackend{DefaultModel: "m", Call: func(_ context.Context, _ string, params any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		got = params.(GenerateParams)
		if failWith != nil {
			return nil, failWith
		}
		return json.RawMessage(`{"text":"{\"a\":1}","finishReason":"stop","constrained":true}`), nil
	}}
	resp, err := eb.Chat(context.Background(), Request{Messages: []Message{{Role: User, Content: "x"}}, Format: FormatJSON, Schema: Object(map[string]*Schema{"a": Integer()}, "a"), SampleConstrained: true})
	if err != nil || resp.Text != `{"a":1}` || !got.JSON || !got.SampleConstrained || len(got.Schema) == 0 || got.Messages[0].Role != "system" || got.Messages[1].Content != "x" {
		t.Fatalf("%v %+v %+v", err, resp, got)
	}
	// SampleConstrained is for JSON only.
	if _, err := eb.Chat(context.Background(), Request{SampleConstrained: true}); err != nil || got.SampleConstrained || got.JSON {
		t.Fatalf("%v %+v", err, got)
	}
	for code, want := range map[string]struct {
		code     string
		sentinel error
	}{
		EngineCodeContextTooLong:     {CodeContextLength, errInvalid},
		EngineCodeInputTooLong:       {CodeContextLength, errInvalid},
		EngineCodeModelNotDownloaded: {CodeModelNotFound, errNotDownloaded},
		EngineCodeBadRequest:         {CodeBadRequest, errInvalid},
		EngineCodeUnsupported:        {CodeUnsupported, errUnsupported},
		EngineCodeLicenceRequired:    {CodeUnsupported, errUnsupported},
		EngineCodeBusy:               {CodeBusy, errBusy},
		EngineCodeOOM:                {CodeBusy, errBusy},
		EngineCodeCancelled:          {CodeCancelled, errCancelled},
		"internal":                   {CodeServer, errEngine},
	} {
		pe := &protoError{Code: code, Message: "no"}
		failWith = fmt.Errorf("call: %w", pe)
		_, err := eb.Chat(context.Background(), Request{})
		var back *protoError
		if Code(err) != want.code || !errors.Is(err, want.sentinel) || !errors.As(err, &back) || back != pe {
			t.Errorf("%s → %v, want %s", code, err, want.code)
		}
	}
	failWith = &protoError{Code: EngineCodeContextTooLong, Message: "long", Data: json.RawMessage(`{"limit":16384,"actual":17000,"unit":"tokens"}`)}
	_, err = eb.Chat(context.Background(), Request{})
	if Param(err, "limit") != "16384" || Param(err, "actual") != "17000" || Param(err, "model") != "m" {
		t.Errorf("context data: %v", err)
	}
	failWith = &protoError{Code: EngineCodeBadRequest, Message: "bad schema"}
	if _, err = eb.Chat(context.Background(), Request{}); Param(err, "detail") != "bad schema" {
		t.Errorf("bad request: %v", err)
	}
	// A cancelled ctx is a cancellation, whatever the engine said.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failWith = &protoError{Code: EngineCodeBusy}
	if _, err := eb.Chat(ctx, Request{}); Code(err) != CodeCancelled {
		t.Errorf("cancelled: %v", err)
	}
}

// The JSON instruction goes into the system message, or a new one first.
func TestJSONInstruction(t *testing.T) {
	s := Object(map[string]*Schema{"a": Integer()}, "a")
	msgs := withJSONInstruction([]Message{{Role: User, Content: "u"}}, s)
	if len(msgs) != 2 || msgs[0].Role != System || msgs[0].Content[:5] != "Reply" {
		t.Fatalf("%+v", msgs)
	}
	in := []Message{{Role: System, Content: "rules"}, {Role: User, Content: "u"}}
	msgs = withJSONInstruction(in, nil)
	if len(msgs) != 2 || msgs[0].Content != "rules\n\nReply with a single JSON value only: no prose, no Markdown fences." || in[0].Content != "rules" {
		t.Fatalf("%+v", msgs)
	}
	for in, want := range map[string]string{"stop": FinishStop, "eos": FinishStop, "length": FinishLength, "": "", "abort": FinishOther} {
		if got := normFinish(in); got != want {
			t.Errorf("normFinish(%q) = %q", in, got)
		}
	}
}

// The wire types are the protocol's: their JSON is what the engine reads.
func TestWire(t *testing.T) {
	temp := 0.0
	b, _ := json.Marshal(GenerateParams{Model: "m", ModelPath: "/p", Messages: []GenerateMessage{{Role: "user", Content: "hi"}}, MaxTokens: 5,
		Temperature: &temp, JSON: true, Schema: json.RawMessage(`{}`), SampleConstrained: true, Stream: true})
	if string(b) != `{"model":"m","modelPath":"/p","messages":[{"role":"user","content":"hi"}],"maxTokens":5,"temperature":0,"stream":true,"json":true,"schema":{},"sampleConstrained":true}` {
		t.Fatal(string(b))
	}
}
