// Package llm is the ActNow apps' language-model layer: chat requests to a
// model, retries, JSON replies checked against a schema, and usage
// accounting. The apps run their own local model — the MLX LLM in the
// family engine's gpu worker (llm.generate, EngineBackend); nothing here
// talks to the network.
//
// Layers, bottom up:
//
//   - Backend: one model endpoint, one attempt per call. EngineBackend is
//     the engine's llm.generate (engine.go); tests script their own.
//   - Client: a Backend plus retries with backoff (the engine restarting,
//     or busy), each skill's defaults (Client.Defaults: the app's table),
//     usage accounting (Meter), <think> filtering and JSON output
//     (extraction, schema validation and repair turns).
//   - JSON helpers: ExtractJSON finds the value in a reply, CloseJSON
//     closes one cut off, ReplyJSON does both for a Response.
//
// The app supplies what is its own: the engine call (EngineBackend.Call),
// its skills and their defaults (Skill, Client.Defaults), and its error
// sentinels (SetSentinels), which *Error values unwrap to.
//
// Everything is goroutine-safe and blocks: call it off the UI thread.
// Errors are *Error values with a stable Code and Params, never
// user-visible text.
package llm

import (
	"context"
	"encoding/json"
	"time"
)

// Role is a chat message role.
type Role string

const (
	System    Role = "system"
	User      Role = "user"
	Assistant Role = "assistant"
)

// Message is one chat turn.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Format selects the output format.
type Format uint8

const (
	FormatText Format = iota // free text
	FormatJSON               // a JSON value; Schema (if set) constrains it
)

// Request is one chat completion.
type Request struct {
	// Model is the catalogue repo of the model. Empty: the backend's default.
	Model    string
	Messages []Message

	Temperature *float64 // nil: the skill's default (Client), else the engine's
	TopP        *float64
	MaxTokens   int // 0: the skill's default (Client), else the engine's
	Stop        []string
	Seed        *int64

	// Format and Schema ask for JSON. The engine constrains decoding to
	// the schema (xgrammar) when it can — greedily, Temperature, TopP and
	// Seed ignored, unless SampleConstrained; Client.JSON validates and
	// repairs either way.
	Format Format
	Schema *Schema
	// SampleConstrained samples a constrained reply as a free one (a
	// repair turn that greedy decoding would answer the same way again).
	SampleConstrained bool

	// OnDelta, when set, streams: it is called with each text delta, in
	// order, before Chat returns (on the engine's reader goroutine: it must
	// not block for long). The Response still carries the whole text.
	OnDelta func(delta string)

	// Timeout bounds one attempt (0: the Client's default; < 0: none).
	Timeout time.Duration

	// Skill tags the call: the Client fills its defaults and accounts
	// usage under it.
	Skill Skill
}

// F64 returns a pointer to v, for Request.Temperature and TopP.
func F64(v float64) *float64 { return &v }

// Usage is the token count of a call. Estimated is true when the
// backend reported none and the counts come from EstimateTokens.
type Usage struct {
	PromptTokens     int  `json:"promptTokens"`
	CompletionTokens int  `json:"completionTokens"`
	TotalTokens      int  `json:"totalTokens"`
	Estimated        bool `json:"estimated,omitempty"`
}

// Add returns u + v.
func (u Usage) Add(v Usage) Usage {
	return Usage{
		PromptTokens:     u.PromptTokens + v.PromptTokens,
		CompletionTokens: u.CompletionTokens + v.CompletionTokens,
		TotalTokens:      u.TotalTokens + v.TotalTokens,
		Estimated:        u.Estimated || v.Estimated,
	}
}

// Finish reasons.
const (
	FinishStop   = "stop"
	FinishLength = "length" // hit MaxTokens
	FinishOther  = "other"
)

// Response is a finished completion.
type Response struct {
	Text         string
	FinishReason string
	Model        string // as reported by the backend
	Provider     string // backend id
	Usage        Usage
	Latency      time.Duration // of the successful attempt
	// Seconds is the engine's generation time (model load excluded) and
	// TokensPerSecond its decoding speed; 0 when not reported.
	Seconds         float64
	TokensPerSecond float64
	Attempts        int // 1 + retries (Client)
	Repairs         int // JSON repair turns (Client.JSON)
	// JSON is the validated value for FormatJSON requests made through
	// Client.JSON.
	JSON json.RawMessage
}

// BackendInfo describes a backend.
type BackendInfo struct {
	ID        string // "local" (the engine), or a test's
	Transport string // "engine"
}

// Backend is one model endpoint. Chat makes a single attempt (Client adds
// retries); it honours ctx cancellation and returns *Error.
// Implementations are goroutine-safe.
type Backend interface {
	Info() BackendInfo
	Chat(ctx context.Context, req Request) (Response, error)
}
