package llm

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Error codes. The app maps them to messages; Params carry the details
// named in each comment.
const (
	CodeUnavailable   = "llm.unavailable"     // provider: the engine is not running (starting, restarting, failed)
	CodeBusy          = "llm.busy"            // provider: the engine is busy or out of memory (retryable)
	CodeServer        = "llm.engine"          // provider, status: the engine failed the call
	CodeBadRequest    = "llm.bad_request"     // provider, status, detail
	CodeModelNotFound = "llm.model_not_found" // provider, model: not downloaded
	CodeContextLength = "llm.context_length"  // provider, model, limit, actual (tokens): prompt + reply too long
	CodeTimeout       = "llm.timeout"         // provider, seconds
	CodeCancelled     = "llm.cancelled"       //
	CodeBadResponse   = "llm.bad_response"    // provider, detail: unparseable result
	CodeEmpty         = "llm.empty"           // provider: no text
	CodeInvalidJSON   = "llm.invalid_json"    // detail: no JSON value in the reply after repairs
	CodeSchema        = "llm.schema"          // path, detail: JSON does not match the schema after repairs
	CodeUnsupported   = "llm.unsupported"     // provider, feature
	CodeConfig        = "llm.config"          // provider, field: no model chosen
)

// Error is every error this package returns.
type Error struct {
	Code      string
	Params    map[string]string
	Retryable bool  // a retry (same backend) may succeed
	Err       error // the cause, if any
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Code)
	keys := make([]string, 0, len(e.Params))
	for k := range e.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%q", k, e.Params[k])
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Unwrap exposes the cause and the app's sentinel of the code
// (SetSentinels), so callers can test errors.Is(err, theirErrCancelled)
// or context.Canceled.
func (e *Error) Unwrap() []error {
	var out []error
	if e.Err != nil {
		out = append(out, e.Err)
	}
	s := sentinels.Load()
	if s == nil {
		return out
	}
	var x error
	switch e.Code {
	case CodeCancelled:
		x = s.Cancelled
	case CodeModelNotFound:
		x = s.NotDownloaded
	case CodeBadRequest, CodeContextLength, CodeConfig, CodeInvalidJSON, CodeSchema:
		x = s.Invalid
	case CodeUnsupported:
		x = s.Unsupported
	case CodeBusy:
		x = s.Busy
	case CodeUnavailable, CodeServer:
		x = s.Engine
	}
	if x != nil {
		out = append(out, x)
	}
	return out
}

// Is matches another *Error by Code, so errors.Is(err, &llm.Error{Code: llm.CodeEmpty}) works.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code && t.Params == nil
}

// Code returns err's code if it is (or wraps) an *Error, else "".
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// Param returns a param of err's *Error ("" when none).
func Param(err error, key string) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Params[key]
	}
	return ""
}

// Sentinels are the app's error values, linked to the codes: an *Error
// unwraps to the one of its code, and EngineBackend reads its caller's
// errors by them (a call failing with an error that wraps Busy is
// CodeBusy, retried). A nil field links nothing.
type Sentinels struct {
	Cancelled       error // CodeCancelled
	NotDownloaded   error // CodeModelNotFound
	Invalid         error // CodeBadRequest, CodeContextLength, CodeConfig, CodeInvalidJSON, CodeSchema
	Unsupported     error // CodeUnsupported
	LicenceRequired error // the caller's licence gate: read as CodeUnsupported (unwraps to Unsupported)
	Busy            error // CodeBusy
	Engine          error // CodeUnavailable, CodeServer
}

var sentinels atomic.Pointer[Sentinels]

// SetSentinels links the app's error values to the codes. Call it once,
// before any model call (an init function of the package defining them).
func SetSentinels(s Sentinels) { sentinels.Store(&s) }

// is is errors.Is for a sentinel that may be unset.
func is(err, target error) bool { return target != nil && errors.Is(err, target) }

func newErr(code string, params ...string) *Error {
	e := &Error{Code: code}
	if len(params) > 0 {
		e.Params = map[string]string{}
		for i := 0; i+1 < len(params); i += 2 {
			e.Params[params[i]] = params[i+1]
		}
	}
	return e
}

func (e *Error) with(k, v string) *Error {
	c := *e
	c.Params = maps.Clone(e.Params)
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	c.Params[k] = v
	return &c
}

// ctxErr converts a transport error into an *Error, distinguishing
// cancellation and deadlines.
func ctxErr(ctx context.Context, provider string, err error, timeout time.Duration) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	if ctx.Err() != nil {
		if errors.Is(context.Cause(ctx), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			x := newErr(CodeTimeout, "provider", provider, "seconds", fmt.Sprintf("%.0f", timeout.Seconds()))
			x.Retryable, x.Err = true, err
			return x
		}
		x := newErr(CodeCancelled)
		x.Err = ctx.Err()
		return x
	}
	if errors.Is(err, context.DeadlineExceeded) {
		x := newErr(CodeTimeout, "provider", provider, "seconds", fmt.Sprintf("%.0f", timeout.Seconds()))
		x.Retryable, x.Err = true, err
		return x
	}
	x := newErr(CodeServer, "provider", provider, "status", "engine")
	x.Err = err
	return x
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && cut < len(s) && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}
