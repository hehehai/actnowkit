package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// Defaults for Client.
const (
	DefaultTimeout     = 10 * time.Minute // one attempt
	DefaultMaxAttempts = 3
	DefaultRepairs     = 2
	// RepairTemperature is the least temperature of a sampled repair turn.
	RepairTemperature = 0.7
)

// RetryPolicy is exponential backoff with full jitter (the engine
// restarting, or busy).
type RetryPolicy struct {
	MaxAttempts int           // total attempts, ≥1 (0: DefaultMaxAttempts)
	Base        time.Duration // first backoff (0: 500 ms)
	Max         time.Duration // backoff cap (0: 20 s)
}

func (p RetryPolicy) attempts() int {
	if p.MaxAttempts <= 0 {
		return DefaultMaxAttempts
	}
	return p.MaxAttempts
}

// delay is the wait before retry n (1-based).
func (p RetryPolicy) delay(n int, jitter func() float64) time.Duration {
	base, maxD := p.Base, p.Max
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	if maxD <= 0 {
		maxD = 20 * time.Second
	}
	d := base << (n - 1)
	if d > maxD || d <= 0 {
		d = maxD
	}
	// Full jitter, floored at half so retries don't fire instantly.
	return d/2 + time.Duration(jitter()*float64(d/2))
}

// Client adds the skills' defaults, timeouts, retries, usage accounting
// and JSON output to a Backend. The zero value of every field is usable;
// Backend is required.
type Client struct {
	Backend Backend
	// Timeout bounds an attempt (0: DefaultTimeout; < 0: none);
	// Request.Timeout overrides. The local engine's calls may wait for the
	// gpu worker first: set none and let ctx decide.
	Timeout time.Duration
	Retry   RetryPolicy
	Meter   *Meter // optional
	// Defaults are the app's skills' defaults: a request's unset
	// Temperature and MaxTokens come from Defaults(req.Skill). nil: none.
	Defaults func(Skill) SkillDefaults

	// Sleep waits between retries; tests replace it. nil: a timer honouring ctx.
	Sleep func(ctx context.Context, d time.Duration) error
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Chat runs req with retries. A streaming request is retried only while
// nothing has been streamed yet. The returned Usage sums every attempt;
// every call that was tried is metered, failed or not.
func (c *Client) Chat(ctx context.Context, req Request) (Response, error) {
	if c.Backend == nil {
		return Response{}, newErr(CodeUnavailable, "provider", "none")
	}
	provider := c.Backend.Info().ID
	req = withDefaults(req, c.Defaults)
	timeout := req.Timeout
	if timeout == 0 {
		timeout = c.Timeout
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	req.Timeout = timeout
	streamed := false
	var filter *thinkFilter
	if user := req.OnDelta; user != nil {
		filter = &thinkFilter{emit: func(s string) { streamed = true; user(s) }}
		req.OnDelta = filter.write
	}
	var total Usage
	start := time.Now()
	// giveUp records the failure of a call after attempts and returns it.
	giveUp := func(e *Error, attempts int) (Response, error) {
		resp := Response{Usage: total, Attempts: attempts}
		c.Meter.record(provider, req, resp, e, time.Since(start))
		return resp, e
	}
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			e := ctxErr(ctx, provider, err, timeout)
			if attempt == 1 {
				return Response{}, e // not tried
			}
			return giveUp(e, attempt-1)
		}
		if filter != nil {
			filter.reset()
		}
		actx, cancel := ctx, context.CancelFunc(func() {})
		if timeout > 0 {
			actx, cancel = context.WithTimeout(ctx, timeout)
		}
		resp, err := c.Backend.Chat(actx, req)
		cancel()
		if err == nil {
			if filter != nil {
				filter.flush()
			}
			resp.Text = StripThinking(resp.Text)
			if resp.Usage.TotalTokens == 0 {
				resp.Usage = estimateUsage(req.Messages, resp.Text)
			}
			total = total.Add(resp.Usage)
			resp.Usage, resp.Attempts = total, attempt
			if resp.Text == "" && resp.FinishReason != FinishLength {
				err = newErr(CodeEmpty, "provider", provider)
				c.Meter.record(provider, req, resp, err, time.Since(start))
				return resp, err
			}
			c.Meter.record(provider, req, resp, nil, time.Since(start))
			return resp, nil
		}
		var e *Error
		if !errors.As(err, &e) {
			e = ctxErr(ctx, provider, err, timeout)
		}
		// A timeout of the attempt (not the caller's deadline) is retryable.
		if ctx.Err() != nil {
			e = ctxErr(ctx, provider, ctx.Err(), timeout)
		}
		if !e.Retryable || streamed || attempt >= c.Retry.attempts() || ctx.Err() != nil {
			return giveUp(e, attempt)
		}
		d := c.Retry.delay(attempt, rand.Float64)
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < d {
			return giveUp(e, attempt) // no time left to wait
		}
		if err := c.sleep(ctx, d); err != nil {
			return giveUp(ctxErr(ctx, provider, err, timeout), attempt)
		}
	}
}

// JSON runs req as a JSON request and decodes the reply into out (a
// pointer, or nil). When the reply holds no JSON, was cut off (out of
// tokens) inside a value, fails to decode, or breaks req.Schema, it asks
// the model to fix it, up to repairs times (<0: none; 0:
// DefaultRepairs), each repair turn sampled (sampleRepair).
// Response.JSON holds the accepted value.
func (c *Client) JSON(ctx context.Context, req Request, out any, repairs int) (Response, error) {
	if repairs == 0 {
		repairs = DefaultRepairs
	}
	if repairs < 0 {
		repairs = 0
	}
	req.Format = FormatJSON
	base := req.Messages
	first := req
	var total Usage
	attempts := 0
	for turn := 0; ; turn++ {
		resp, err := c.Chat(ctx, req)
		total = total.Add(resp.Usage)
		attempts += resp.Attempts
		if err != nil {
			return Response{Usage: total, Attempts: attempts, Repairs: turn}, err
		}
		resp.Usage, resp.Attempts, resp.Repairs = total, attempts, turn
		raw := ExtractJSON(resp.Text)
		var problems []string
		code := CodeInvalidJSON
		switch {
		case resp.FinishReason == FinishLength && leftOpen(resp.Text):
			// What parses of a reply cut off is a part of it.
			raw, problems = nil, []string{"the reply was cut off; send a shorter, complete JSON value"}
		case raw == nil:
			problems = []string{"the reply contained no JSON value"}
		case req.Schema != nil:
			for _, se := range req.Schema.Validate(raw) {
				problems = append(problems, se.String())
			}
			if len(problems) > 0 {
				code = CodeSchema
			}
		}
		if len(problems) == 0 && out != nil {
			if err := json.Unmarshal(raw, out); err != nil {
				problems = []string{"decode: " + err.Error()}
				code = CodeSchema
			}
		}
		if len(problems) == 0 {
			resp.JSON = raw
			return resp, nil
		}
		if turn >= repairs {
			e := newErr(code, "detail", clip(strings.Join(problems, "; "), 300))
			if code == CodeSchema {
				e = e.with("path", firstPath(problems))
			}
			return resp, e
		}
		req.Messages = append(append([]Message{}, base...),
			Message{Role: Assistant, Content: clip(resp.Text, 8000)},
			Message{Role: User, Content: repairPrompt(problems)})
		req = sampleRepair(req, first, turn+1)
	}
}

// sampleRepair has repair turn n sample its constrained reply: greedy
// decoding answers a repair mostly as before (a reply caught in a loop
// loops again). At least RepairTemperature, topP 0.95 unless the request
// set one, seeded with n so a rerun gives the same reply. A request that
// samples already (first, the request as asked) keeps its own settings.
func sampleRepair(req, first Request, n int) Request {
	if first.SampleConstrained {
		return req
	}
	req.SampleConstrained = true
	t := RepairTemperature
	if first.Temperature != nil && *first.Temperature > t {
		t = *first.Temperature
	}
	req.Temperature = F64(t)
	req.TopP = first.TopP
	if req.TopP == nil {
		req.TopP = F64(0.95)
	}
	seed := int64(n)
	req.Seed = &seed
	return req
}

func firstPath(problems []string) string {
	if len(problems) == 0 {
		return ""
	}
	p, _, _ := strings.Cut(problems[0], ": ")
	return p
}

func repairPrompt(problems []string) string {
	var b strings.Builder
	b.WriteString("Your previous reply could not be used:\n")
	for i, p := range problems {
		if i == 10 {
			fmt.Fprintf(&b, "- … and %d more\n", len(problems)-10)
			break
		}
		b.WriteString("- " + p + "\n")
	}
	b.WriteString("Send the corrected answer again as one JSON value only, with nothing before or after it.")
	return b.String()
}

// StripThinking removes a leading <think>…</think> block (a hybrid
// thinking model that thought despite enable_thinking: false).
func StripThinking(s string) string {
	t := strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(t, "<think>") {
		return s
	}
	if i := strings.Index(t, "</think>"); i >= 0 {
		return strings.TrimLeft(t[i+len("</think>"):], " \t\r\n")
	}
	return "" // still thinking when cut off
}

// thinkFilter hides a leading <think>…</think> block from stream deltas.
type thinkFilter struct {
	emit  func(string)
	buf   strings.Builder
	state int // 0 deciding, 1 inside think, 2 passing through, 3 after think (trim leading space)
}

func (f *thinkFilter) reset() { f.buf.Reset(); f.state = 0 }

func (f *thinkFilter) write(s string) {
	switch f.state {
	case 2:
		f.emit(s)
		return
	case 3:
		s = strings.TrimLeft(s, " \t\r\n")
		if s != "" {
			f.state = 2
			f.emit(s)
		}
		return
	}
	f.buf.WriteString(s)
	b := f.buf.String()
	if f.state == 0 {
		t := strings.TrimLeft(b, " \t\r\n")
		switch {
		case strings.HasPrefix(t, "<think>"):
			f.state = 1
		case len(t) < len("<think>") && strings.HasPrefix("<think>", t):
			return // undecided
		default:
			f.state = 2
			f.buf.Reset()
			f.emit(b)
			return
		}
	}
	if i := strings.Index(b, "</think>"); i >= 0 {
		rest := b[i+len("</think>"):]
		f.buf.Reset()
		f.state = 3
		f.write(rest)
	}
}

func (f *thinkFilter) flush() {
	if f.state == 0 && f.buf.Len() > 0 {
		f.emit(f.buf.String())
	}
	f.buf.Reset()
}
