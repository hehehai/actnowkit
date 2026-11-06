package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The family engine's local LLM: llm.generate (protocol 2, §5.4) runs a
// chat completion on an MLX model in the gpu worker as a job, streaming
// its text as llm.token notifications. These are its wire types; the
// apps' engine protocol packages alias them.
const (
	MethodGenerate = "llm.generate" // params GenerateParams → result GenerateResult
	NotifyToken    = "llm.token"    // params TokenNotification, in order, before the result
)

// GenerateMessage is a chat message of llm.generate.
type GenerateMessage struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// GenerateParams are llm.generate's params.
type GenerateParams struct {
	Model     string            `json:"model"` // repo id
	ModelPath string            `json:"modelPath,omitempty"`
	Messages  []GenerateMessage `json:"messages"`
	// MaxTokens caps the reply (0 = the engine's default, 2048, within the
	// context the engine allows).
	MaxTokens   int      `json:"maxTokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"` // nil = the model's default (0.7); 0 = greedy
	TopP        *float64 `json:"topP,omitempty"`        // nil = 0.8
	Seed        *int64   `json:"seed,omitempty"`
	Stop        []string `json:"stop,omitempty"` // strings that end the reply (not included)
	// Stream asks for llm.token notifications.
	Stream bool `json:"stream,omitempty"`
	// JSON asks for one JSON value; Schema (JSON Schema) constrains it.
	// The engine constrains decoding to the grammar when it can (greedy:
	// Temperature, TopP and Seed are ignored unless SampleConstrained),
	// and the app validates either way.
	JSON   bool            `json:"json,omitempty"`
	Schema json.RawMessage `json:"schema,omitempty"`
	// SampleConstrained samples a constrained reply with Temperature, TopP
	// and Seed as a free one.
	SampleConstrained bool `json:"sampleConstrained,omitempty"`
}

// GenerateResult is llm.generate's result.
type GenerateResult struct {
	Text             string  `json:"text"`
	FinishReason     string  `json:"finishReason"` // "stop" | "length"
	Model            string  `json:"model,omitempty"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	Seconds          float64 `json:"seconds,omitempty"`         // generation time, excluding model load
	TokensPerSecond  float64 `json:"tokensPerSecond,omitempty"` // completion tokens / decode time
	Constrained      bool    `json:"constrained,omitempty"`     // JSON grammar applied
}

// TokenNotification is llm.token's params: the next piece of the reply.
type TokenNotification struct {
	Job  string `json:"job"`
	Text string `json:"text"`
}

// The engine protocol's error codes EngineBackend reads.
const (
	EngineCodeBadRequest         = "bad_request"
	EngineCodeUnsupported        = "unsupported"
	EngineCodeModelNotDownloaded = "model_not_downloaded"
	EngineCodeLicenceRequired    = "licence_required"
	EngineCodeOOM                = "oom"
	EngineCodeBusy               = "busy"
	EngineCodeCancelled          = "cancelled"
	EngineCodeInputTooLong       = "input_too_long"   // data: {"limit":…,"actual":…}
	EngineCodeContextTooLong     = "context_too_long" // data: {"limit":…,"actual":…}
)

// EngineError is an error of the engine protocol: the app's protocol
// package implements it on its error type, and EngineBackend finds it in
// what the caller returns (errors.As) to read the code.
type EngineError interface {
	error
	EngineError() (code, message string, data json.RawMessage)
}

// EngineCaller sends one engine request and waits for its result. The app
// wires it to the gpu worker (through its priority gate, filling in the
// model's snapshot directory); notify receives the request's
// notifications in order, before the call returns. Its errors are the
// protocol's (EngineError) or wrap the app's sentinels (SetSentinels).
type EngineCaller func(ctx context.Context, method string, params any, notify func(method string, params json.RawMessage)) (json.RawMessage, error)

// EngineBackend is the local MLX model behind the engine protocol.
type EngineBackend struct {
	ID           string // default "local"
	Call         EngineCaller
	DefaultModel string // catalogue repo, used when a request names none
}

func (e *EngineBackend) id() string {
	if e.ID == "" {
		return "local"
	}
	return e.ID
}

func (e *EngineBackend) Info() BackendInfo {
	return BackendInfo{ID: e.id(), Transport: "engine"}
}

func (e *EngineBackend) Chat(ctx context.Context, req Request) (Response, error) {
	if e.Call == nil {
		return Response{}, newErr(CodeUnavailable, "provider", e.id())
	}
	model := req.Model
	if model == "" {
		model = e.DefaultModel
	}
	if model == "" {
		return Response{}, newErr(CodeConfig, "provider", e.id(), "field", "model")
	}
	msgs := req.Messages
	if req.Format == FormatJSON {
		msgs = withJSONInstruction(msgs, req.Schema)
	}
	p := GenerateParams{
		Model: model, MaxTokens: req.MaxTokens, Temperature: req.Temperature,
		TopP: req.TopP, Seed: req.Seed, Stop: req.Stop, Stream: req.OnDelta != nil,
		JSON: req.Format == FormatJSON, SampleConstrained: req.SampleConstrained && req.Format == FormatJSON,
	}
	for _, m := range msgs {
		p.Messages = append(p.Messages, GenerateMessage{Role: string(m.Role), Content: m.Content})
	}
	if req.Format == FormatJSON && req.Schema != nil {
		if b, err := json.Marshal(req.Schema); err == nil {
			p.Schema = b
		}
	}
	start := time.Now()
	var notify func(string, json.RawMessage)
	if req.OnDelta != nil {
		notify = func(method string, params json.RawMessage) {
			if method != NotifyToken {
				return
			}
			var t TokenNotification
			if json.Unmarshal(params, &t) == nil && t.Text != "" {
				req.OnDelta(t.Text)
			}
		}
	}
	raw, err := e.Call(ctx, MethodGenerate, p, notify)
	if err != nil {
		return Response{}, e.mapErr(ctx, err, model, req.Timeout)
	}
	var r GenerateResult
	if err := json.Unmarshal(raw, &r); err != nil {
		x := newErr(CodeBadResponse, "provider", e.id(), "detail", clip(string(raw), 200))
		x.Err = err
		return Response{}, x
	}
	if r.Model == "" {
		r.Model = model
	}
	return Response{
		Text: r.Text, FinishReason: normFinish(r.FinishReason), Model: r.Model, Provider: e.id(),
		Usage:   Usage{PromptTokens: r.PromptTokens, CompletionTokens: r.CompletionTokens, TotalTokens: r.PromptTokens + r.CompletionTokens},
		Latency: time.Since(start), Seconds: r.Seconds, TokensPerSecond: r.TokensPerSecond,
	}, nil
}

// mapErr reads a failed call: cancellation and deadlines, the protocol's
// codes, then the app's sentinels; anything else is the engine failing.
func (e *EngineBackend) mapErr(ctx context.Context, err error, model string, timeout time.Duration) error {
	s := sentinels.Load()
	if s == nil {
		s = &Sentinels{}
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || is(err, s.Cancelled) {
		if ctx.Err() == nil && is(err, s.Cancelled) {
			x := newErr(CodeCancelled)
			x.Err = err
			return x
		}
		return ctxErr(ctx, e.id(), err, timeout)
	}
	// tagged is err wrapping the protocol code's sentinel, as the
	// caller's own errors do (err as it is when the app set none).
	notDownloaded, unsupported, busy := false, false, false
	tagged := func(sentinel error) error {
		if sentinel == nil {
			return err
		}
		return fmt.Errorf("%w: %w", sentinel, err)
	}
	var ee EngineError
	if errors.As(err, &ee) {
		code, msg, data := ee.EngineError()
		switch code {
		case EngineCodeContextTooLong, EngineCodeInputTooLong:
			x := newErr(CodeContextLength, "provider", e.id(), "model", model)
			var d struct {
				Limit, Actual int
			}
			if json.Unmarshal(data, &d) == nil && d.Limit > 0 {
				x.Params["limit"], x.Params["actual"] = fmt.Sprint(d.Limit), fmt.Sprint(d.Actual)
			}
			x.Err = err
			return x
		case EngineCodeModelNotDownloaded:
			err, notDownloaded = tagged(s.NotDownloaded), true
		case EngineCodeLicenceRequired, EngineCodeUnsupported:
			err, unsupported = tagged(s.Unsupported), true
		case EngineCodeBusy, EngineCodeOOM:
			err, busy = tagged(s.Busy), true
		case EngineCodeCancelled:
			x := newErr(CodeCancelled)
			x.Err = err
			return x
		case EngineCodeBadRequest:
			x := newErr(CodeBadRequest, "provider", e.id(), "status", "engine", "detail", clip(msg, 200))
			x.Err = err
			return x
		}
	}
	switch {
	case notDownloaded || is(err, s.NotDownloaded):
		x := newErr(CodeModelNotFound, "provider", e.id(), "model", model)
		x.Err = err
		return x
	case unsupported || is(err, s.LicenceRequired) || is(err, s.Unsupported):
		x := newErr(CodeUnsupported, "provider", e.id(), "feature", "model")
		x.Err = err
		return x
	case busy || is(err, s.Busy):
		x := newErr(CodeBusy, "provider", e.id())
		x.Retryable, x.Err = true, err
		return x
	case is(err, s.Engine):
		x := newErr(CodeUnavailable, "provider", e.id())
		x.Retryable, x.Err = true, err
		return x
	}
	x := newErr(CodeServer, "provider", e.id(), "status", "engine")
	x.Err = err
	return x
}

// withJSONInstruction returns msgs with the JSON instruction appended to
// the system message (one is added when there is none).
func withJSONInstruction(msgs []Message, s *Schema) []Message {
	instr := jsonInstruction(s)
	out := make([]Message, 0, len(msgs)+1)
	added := false
	for _, m := range msgs {
		if m.Role == System && !added {
			m.Content += instr
			added = true
		}
		out = append(out, m)
	}
	if !added {
		out = append([]Message{{Role: System, Content: strings.TrimSpace(instr)}}, out...)
	}
	return out
}

// jsonInstruction is appended to the system prompt for FormatJSON
// requests: the engine's grammar enforces the shape, but a model that
// knows it writes better JSON (and the grammar may be off).
func jsonInstruction(s *Schema) string {
	var b strings.Builder
	b.WriteString("\n\nReply with a single JSON value only: no prose, no Markdown fences.")
	if s != nil {
		js, _ := json.Marshal(s)
		b.WriteString(" It must match this JSON Schema:\n")
		b.Write(js)
	}
	return b.String()
}

func normFinish(r string) string {
	switch r {
	case "stop", "eos":
		return FinishStop
	case "length":
		return FinishLength
	case "":
		return ""
	}
	return FinishOther
}
