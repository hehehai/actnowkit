package llm

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// script is a Backend whose attempts answer in turn (the last repeats).
type script struct {
	mu    sync.Mutex
	steps []func(Request) (Response, error)
	reqs  []Request
}

func (s *script) Info() BackendInfo { return BackendInfo{ID: "script", Transport: "test"} }

func (s *script) Chat(ctx context.Context, req Request) (Response, error) {
	s.mu.Lock()
	i := min(len(s.reqs), len(s.steps)-1)
	s.reqs = append(s.reqs, req)
	f := s.steps[i]
	s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Response{}, ctxErr(ctx, "script", err, 0)
	}
	return f(req)
}

func text(t string) func(Request) (Response, error) {
	return func(Request) (Response, error) {
		return Response{Text: t, FinishReason: FinishStop, Usage: Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}}, nil
	}
}

func fail(e *Error) func(Request) (Response, error) {
	return func(Request) (Response, error) { return Response{}, e }
}

func noSleep(context.Context, time.Duration) error { return nil }

func TestRetries(t *testing.T) {
	var slept []time.Duration
	sleep := func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	busy := &Error{Code: CodeBusy, Retryable: true}
	down := &Error{Code: CodeUnavailable, Retryable: true}
	b := &script{steps: []func(Request) (Response, error){fail(busy), fail(down), text("done")}}
	m := &Meter{}
	c := &Client{Backend: b, Sleep: sleep, Meter: m, Defaults: testDefaults}
	resp, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: User, Content: "x"}}, Skill: skillTranslate})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Attempts != 3 || len(slept) != 2 {
		t.Fatalf("attempts %d slept %v", resp.Attempts, slept)
	}
	// Full jitter over the doubling backoff, floored at half.
	if slept[0] < 250*time.Millisecond || slept[0] > 500*time.Millisecond || slept[1] < 500*time.Millisecond || slept[1] > time.Second {
		t.Fatalf("backoff %v", slept)
	}
	snap := m.Snapshot()
	if len(snap) != 1 || snap[0].Calls != 1 || snap[0].Attempts != 3 || snap[0].Skill != skillTranslate || snap[0].Usage.TotalTokens != 18 {
		t.Fatalf("meter %+v", snap)
	}
	// The skill's defaults fill what the request leaves unset.
	if r := b.reqs[2]; *r.Temperature != 0.2 || r.MaxTokens != 1024 {
		t.Fatalf("defaults %+v", r)
	}
	// What the request sets stays; no Defaults, nothing filled.
	b = &script{steps: []func(Request) (Response, error){text("ok")}}
	c.Backend = b
	if _, err := c.Chat(context.Background(), Request{Skill: skillTitle, MaxTokens: 50, Temperature: F64(0.9)}); err != nil {
		t.Fatal(err)
	}
	c.Defaults = nil
	if _, err := c.Chat(context.Background(), Request{Skill: skillTitle}); err != nil {
		t.Fatal(err)
	}
	if r := b.reqs[0]; *r.Temperature != 0.9 || r.MaxTokens != 50 {
		t.Fatalf("set %+v", r)
	}
	if r := b.reqs[1]; r.Temperature != nil || r.MaxTokens != 0 {
		t.Fatalf("no defaults %+v", r)
	}
}

func TestBackoff(t *testing.T) {
	p := RetryPolicy{Base: time.Second, Max: 3 * time.Second}
	for n, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 3 * time.Second, 70: 3 * time.Second} {
		if lo, hi := p.delay(n, func() float64 { return 0 }), p.delay(n, func() float64 { return 1 }); lo != want/2 || hi != want {
			t.Errorf("retry %d: %v..%v, want %v..%v", n, lo, hi, want/2, want)
		}
	}
}

func TestRetryGivesUp(t *testing.T) {
	b := &script{steps: []func(Request) (Response, error){fail(&Error{Code: CodeUnavailable, Retryable: true})}}
	c := &Client{Backend: b, Sleep: noSleep, Retry: RetryPolicy{MaxAttempts: 4}}
	_, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: User, Content: "x"}}})
	if Code(err) != CodeUnavailable || len(b.reqs) != 4 || !errors.Is(err, errEngine) {
		t.Fatalf("err %v reqs %d", err, len(b.reqs))
	}
	// Not retryable: once.
	b = &script{steps: []func(Request) (Response, error){fail(&Error{Code: CodeContextLength})}}
	c.Backend = b
	if _, err := c.Chat(context.Background(), Request{}); Code(err) != CodeContextLength || len(b.reqs) != 1 || !errors.Is(err, errInvalid) {
		t.Fatalf("err %v reqs %d", err, len(b.reqs))
	}
	// No backend.
	if _, err := (&Client{}).Chat(context.Background(), Request{}); Code(err) != CodeUnavailable {
		t.Fatal(err)
	}
}

func TestTimeoutAndCancel(t *testing.T) {
	slow := &blocking{}
	c := &Client{Backend: slow, Timeout: 50 * time.Millisecond, Sleep: noSleep, Retry: RetryPolicy{MaxAttempts: 2}}
	_, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: User, Content: "x"}}})
	if Code(err) != CodeTimeout || slow.calls != 2 {
		t.Fatalf("err %v calls %d", err, slow.calls)
	}
	// No timeout: only ctx ends it.
	ctx, cancel := context.WithCancel(context.Background())
	c.Timeout = -1
	slow.started = make(chan struct{}, 1)
	go func() { <-slow.started; cancel() }()
	_, err = c.Chat(ctx, Request{Messages: []Message{{Role: User, Content: "x"}}})
	if Code(err) != CodeCancelled || !errors.Is(err, errCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	// Cancelled before: not tried.
	if resp, err := c.Chat(ctx, Request{}); Code(err) != CodeCancelled || resp.Attempts != 0 {
		t.Fatalf("err %v resp %+v", err, resp)
	}
}

// blocking answers when its ctx ends.
type blocking struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
}

func (b *blocking) Info() BackendInfo { return BackendInfo{ID: "blocking"} }
func (b *blocking) Chat(ctx context.Context, req Request) (Response, error) {
	b.mu.Lock()
	b.calls++
	if b.started != nil {
		b.started <- struct{}{}
	}
	b.mu.Unlock()
	<-ctx.Done()
	return Response{}, ctxErr(ctx, "blocking", ctx.Err(), 0)
}

func TestNoRetryAfterStreaming(t *testing.T) {
	b := &script{steps: []func(Request) (Response, error){func(r Request) (Response, error) {
		r.OnDelta("partial")
		return Response{}, &Error{Code: CodeUnavailable, Retryable: true}
	}}}
	var got string
	c := &Client{Backend: b, Sleep: noSleep}
	_, err := c.Chat(context.Background(), Request{OnDelta: func(s string) { got += s }})
	if Code(err) != CodeUnavailable || len(b.reqs) != 1 || got != "partial" {
		t.Fatalf("err %v reqs %d got %q", err, len(b.reqs), got)
	}
}

func TestEmptyAndThinking(t *testing.T) {
	b := &script{steps: []func(Request) (Response, error){text("<think>hmm</think>\n")}}
	c := &Client{Backend: b}
	resp, err := c.Chat(context.Background(), Request{})
	if Code(err) != CodeEmpty || resp.Text != "" {
		t.Fatalf("err %v resp %+v", err, resp)
	}
	// A reply with no usage gets an estimate.
	b.steps = []func(Request) (Response, error){func(Request) (Response, error) { return Response{Text: "abcdefgh"}, nil }}
	resp, err = c.Chat(context.Background(), Request{Messages: []Message{{Role: User, Content: "你好"}}})
	if err != nil || !resp.Usage.Estimated || resp.Usage.PromptTokens != 6 || resp.Usage.CompletionTokens != 2 {
		t.Fatalf("err %v usage %+v", err, resp.Usage)
	}
}

func TestJSONRepair(t *testing.T) {
	schema := Object(map[string]*Schema{
		"lines": Array(Object(map[string]*Schema{"id": String(), "text": String()}, "id", "text")).Count(2, 2),
	}, "lines")
	b := &script{steps: []func(Request) (Response, error){
		text("Sure! Here you go: no json"),
		text("```json\n{\"lines\":[{\"id\":\"1\",\"text\":\"a\"}]}\n```"),
		text(`{"lines":[{"id":"1","text":"a"},{"id":"2","text":"b"}]}`),
	}}
	c := &Client{Backend: b}
	var out struct {
		Lines []struct{ ID, Text string }
	}
	resp, err := c.JSON(context.Background(), Request{Messages: []Message{{Role: System, Content: "s"}, {Role: User, Content: "u"}}, Schema: schema}, &out, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Lines) != 2 || resp.Repairs != 2 || resp.Usage.TotalTokens != 54 || resp.Attempts != 3 || string(resp.JSON) != `{"lines":[{"id":"1","text":"a"},{"id":"2","text":"b"}]}` {
		t.Fatalf("out %+v resp %+v", out, resp)
	}
	last := b.reqs[2].Messages
	if len(last) != 4 || last[2].Role != Assistant {
		t.Fatalf("repair turn has %d messages", len(last))
	}
	if !strings.Contains(last[3].Content, "/lines: must have at least 2 items") {
		t.Fatalf("repair prompt %q", last[3].Content)
	}
	// The first turn is greedy; each repair samples, seeded by its number.
	if r := b.reqs[0]; r.SampleConstrained || r.Seed != nil || r.Temperature != nil || r.Format != FormatJSON {
		t.Fatalf("first turn %+v", r)
	}
	for n, r := range b.reqs[1:] {
		if !r.SampleConstrained || r.Seed == nil || *r.Seed != int64(n+1) || *r.Temperature != RepairTemperature || *r.TopP != 0.95 {
			t.Fatalf("repair %d: %+v", n+1, r)
		}
	}
	// A warmer request keeps its temperature on a repair, and its topP.
	b = &script{steps: []func(Request) (Response, error){text("no json"), text(`{"lines":[{"id":"1","text":"a"},{"id":"2","text":"b"}]}`)}}
	c.Backend = b
	if _, err := c.JSON(context.Background(), Request{Messages: []Message{{Role: User, Content: "u"}}, Schema: schema, Temperature: F64(0.9), TopP: F64(0.5)}, nil, 1); err != nil {
		t.Fatal(err)
	}
	if r := b.reqs[1]; !r.SampleConstrained || *r.Temperature != 0.9 || *r.TopP != 0.5 {
		t.Fatalf("warm repair %+v", r)
	}
	// A request that samples already keeps its settings.
	b = &script{steps: []func(Request) (Response, error){text("no json"), text(`{"lines":[{"id":"1","text":"a"},{"id":"2","text":"b"}]}`)}}
	c.Backend = b
	seed := int64(9)
	if _, err := c.JSON(context.Background(), Request{Schema: schema, SampleConstrained: true, Temperature: F64(0.1), Seed: &seed}, nil, 1); err != nil {
		t.Fatal(err)
	}
	if r := b.reqs[1]; *r.Temperature != 0.1 || *r.Seed != 9 || r.TopP != nil {
		t.Fatalf("sampling repair %+v", r)
	}
	// Out of repairs → schema error with the path.
	c.Backend = &script{steps: []func(Request) (Response, error){text(`{"lines":[]}`)}}
	_, err = c.JSON(context.Background(), Request{Messages: []Message{{Role: User, Content: "u"}}, Schema: schema}, nil, -1)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeSchema || e.Params["path"] != "/lines" || !errors.Is(err, errInvalid) {
		t.Fatalf("err %v", err)
	}
	// Not decodable into out → schema error.
	c.Backend = &script{steps: []func(Request) (Response, error){text(`{"lines":"x"}`)}}
	var wrong struct{ Lines []int }
	if _, err := c.JSON(context.Background(), Request{}, &wrong, -1); Code(err) != CodeSchema {
		t.Fatalf("err %v", err)
	}
}

// A reply cut off is repaired as cut off, though a part of it parses; one
// cut off after its whole value is used.
func TestJSONCutOff(t *testing.T) {
	schema := Object(map[string]*Schema{"lines": Array(String())}, "lines")
	cut := func(Request) (Response, error) {
		return Response{Text: `{"lines":["a","b",{"x":1}, "c`, FinishReason: FinishLength}, nil
	}
	b := &script{steps: []func(Request) (Response, error){cut, text(`{"lines":["a"]}`)}}
	c := &Client{Backend: b}
	if _, err := c.JSON(context.Background(), Request{Schema: schema}, nil, 1); err != nil {
		t.Fatal(err)
	}
	if p := b.reqs[1].Messages; len(p) != 2 || !strings.Contains(p[1].Content, "cut off") {
		t.Errorf("repair %+v", p)
	}
	c.Backend = &script{steps: []func(Request) (Response, error){cut}}
	if _, err := c.JSON(context.Background(), Request{Schema: schema}, nil, -1); Code(err) != CodeInvalidJSON || !strings.Contains(Param(err, "detail"), "cut off") {
		t.Errorf("err %v", err)
	}
	whole := func(Request) (Response, error) {
		return Response{Text: `{"lines":["a"]} That is all I`, FinishReason: FinishLength}, nil
	}
	c.Backend = &script{steps: []func(Request) (Response, error){whole}}
	if resp, err := c.JSON(context.Background(), Request{Schema: schema}, nil, -1); err != nil || string(resp.JSON) != `{"lines":["a"]}` {
		t.Errorf("whole: %v %s", err, resp.JSON)
	}
}

// Calls given up while waiting to retry are metered too.
func TestMeterCountsGivingUp(t *testing.T) {
	busy := &Error{Code: CodeBusy, Retryable: true}
	m := &Meter{}
	b := &script{steps: []func(Request) (Response, error){fail(busy)}}
	cancelled := func(context.Context, time.Duration) error { return context.Canceled }
	c := &Client{Backend: b, Sleep: cancelled, Meter: m, Retry: RetryPolicy{Base: time.Hour, Max: time.Hour}}
	if _, err := c.Chat(context.Background(), Request{Skill: skillTranslate}); err == nil {
		t.Fatal("no error")
	}
	// No time left before the deadline to wait.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c.Sleep = noSleep
	if _, err := c.Chat(ctx, Request{Skill: skillTranslate}); Code(err) != CodeBusy {
		t.Fatalf("err %v", err)
	}
	snap := m.Snapshot()
	if len(snap) != 1 || snap[0].Calls != 2 || snap[0].Failures != 2 || snap[0].Attempts != 2 {
		t.Errorf("meter %+v", snap)
	}
	var nilMeter *Meter
	if nilMeter.Snapshot() != nil {
		t.Error("nil meter")
	}
}

func TestThinkingStripped(t *testing.T) {
	if got := StripThinking("<think>\nhmm\n</think>\n\nAnswer"); got != "Answer" {
		t.Fatal(got)
	}
	if got := StripThinking("Answer <think>"); got != "Answer <think>" {
		t.Fatal(got)
	}
	if got := StripThinking("<think>still"); got != "" {
		t.Fatal(got)
	}
	var out strings.Builder
	f := &thinkFilter{emit: func(s string) { out.WriteString(s) }}
	for _, d := range []string{"<th", "ink>reason", "ing</thi", "nk>", "\n  ", "Hi", " you"} {
		f.write(d)
	}
	f.flush()
	if out.String() != "Hi you" {
		t.Fatalf("%q", out.String())
	}
	out.Reset()
	f.reset()
	for _, d := range []string{"<", "b>bold"} {
		f.write(d)
	}
	f.flush()
	if out.String() != "<b>bold" {
		t.Fatalf("%q", out.String())
	}
}

func TestEstimateTokens(t *testing.T) {
	if n := EstimateTokens("你好世界"); n != 4 {
		t.Fatal(n)
	}
	if n := EstimateTokens("abcdefgh"); n != 2 {
		t.Fatal(n)
	}
}

func TestErrors(t *testing.T) {
	e := &Error{Code: CodeContextLength, Params: map[string]string{"model": "m", "limit": "10"}, Err: errors.New("cause")}
	if e.Error() != `llm.context_length limit="10" model="m": cause` {
		t.Fatal(e.Error())
	}
	if !errors.Is(e, &Error{Code: CodeContextLength}) || errors.Is(e, &Error{Code: CodeBusy}) {
		t.Fatal("Is by code")
	}
	if Code(errors.New("x")) != "" || Param(e, "limit") != "10" || Param(errors.New("x"), "limit") != "" {
		t.Fatal("Code/Param")
	}
	f := e.with("actual", "12")
	if f.Params["actual"] != "12" || e.Params["actual"] != "" {
		t.Fatal("with copies")
	}
	for code, want := range map[string]error{CodeModelNotFound: errNotDownloaded, CodeUnsupported: errUnsupported, CodeBusy: errBusy,
		CodeServer: errEngine, CodeCancelled: errCancelled, CodeSchema: errInvalid} {
		if !errors.Is(&Error{Code: code}, want) {
			t.Errorf("%s does not unwrap to %v", code, want)
		}
	}
	if errors.Is(&Error{Code: CodeEmpty}, errInvalid) || errors.Is(&Error{Code: CodeTimeout}, errEngine) {
		t.Error("unwraps to a sentinel not its own")
	}
	if got := clip("你好世界", 4); got != "你…" {
		t.Errorf("clip %q", got)
	}
}
