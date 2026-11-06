package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                          `{"a":1}`,
		"```json\n[1,2]\n```":              `[1,2]`,
		`Here: {"a":"}"} trailing`:         `{"a":"}"}`,
		"<think>{x}</think>\n{\"b\":true}": `{"b":true}`,
		`text [not json] {"ok":1}`:         `{"ok":1}`,
		"nothing":                          ``,
		// Not a citation before the answer, not a fragment of a reply
		// cut off.
		`Answer [1]: {"a":1}`: `{"a":1}`,
		`{"lines":[{"id":"1","text":"hello"},{"id":"2","text":"wor`: ``,
		`{"a":1} then {"b":`: `{"a":1}`,
	}
	for in, want := range cases {
		if got := string(ExtractJSON(in)); got != want {
			t.Errorf("ExtractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestReplyJSONCutOff: a reply that ran out of tokens keeps its whole
// items, not its first one alone, nor a last one half written.
func TestReplyJSONCutOff(t *testing.T) {
	type item struct{ ID, Wrong, Right string }
	text := `{"items":[{"id":"1","wrong":"a","right":"b"},{"id":"2","wrong":"c","right":"d"},{"id":"3","wrong":"e"`
	var out struct{ Items []item }
	raw := ReplyJSON(Response{Text: text, FinishReason: FinishLength})
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Items) != 2 || out.Items[1].Right != "d" {
		t.Fatalf("%s: %+v %v", raw, out.Items, err)
	}
	// Said it stopped, left open all the same: closed too.
	if raw := ReplyJSON(Response{Text: text, FinishReason: FinishStop}); json.Unmarshal(raw, &out) != nil || len(out.Items) != 2 {
		t.Fatalf("open, stopped: %s", raw)
	}
	// A string cut off in an object (not a list's item) is closed.
	if raw := ReplyJSON(Response{Text: `{"title":"Talk","brief":"About the`, FinishReason: FinishLength}); string(raw) != `{"title":"Talk","brief":"About the"}` {
		t.Errorf("cut string: %s", raw)
	}
	// Whole: as it is.
	if raw := ReplyJSON(Response{Text: "Here: " + `{"items":[]}`, FinishReason: FinishStop}); string(raw) != `{"items":[]}` {
		t.Errorf("whole reply: %s", raw)
	}
	if raw := ReplyJSON(Response{Text: "no JSON", FinishReason: FinishLength}); raw != nil {
		t.Errorf("none: %s", raw)
	}
}

func TestCloseJSON(t *testing.T) {
	for in, want := range map[string]string{
		`{"a":"x\`:                         `{"a":"x"}`,
		`{"a":1,`:                          `{"a":1}`,
		`{"a":1,"b":`:                      `{"a":1}`,
		`{"a":1,"b"`:                       `{"a":1}`,
		`{"a":[1,2`:                        `{"a":[1,2]}`,
		`x {"a":{"b":[{"c":1},{"c":2},{"c`: `{"a":{"b":[{"c":1},{"c":2}]}}`,
		`{"a":1}`:                          ``,
		`[1,2`:                             ``,
		`{"a":tru`:                         ``, // no comma to drop back to
	} {
		if got := string(CloseJSON(in)); got != want {
			t.Errorf("CloseJSON(%q) = %q, want %q", in, got, want)
		}
	}
	if !leftOpen(`x {"a":[1`) || leftOpen(`{"a":[1]} "b`) || leftOpen("none") {
		t.Error("leftOpen")
	}
}

func TestSchema(t *testing.T) {
	s := Object(map[string]*Schema{
		"e":    Enum("a", "b"),
		"n":    Number().Range(0, 1),
		"i":    Integer(),
		"opt":  {Type: "string", Nullable: true},
		"list": Array(Boolean()).Count(1, -1),
		"s":    String().Describe("text"),
	}, "e", "n", "list")
	errs := s.Validate(json.RawMessage(`{"e":"c","n":2,"i":1.5,"opt":null,"list":[],"x":1}`))
	var paths []string
	for _, e := range errs {
		paths = append(paths, e.Path)
	}
	if strings.Join(paths, ",") != "/e,/i,/list,/n,/" {
		t.Fatalf("%v", errs)
	}
	if errs := s.Validate(json.RawMessage(`{"e":"a","n":0.5,"list":[true],"opt":"x"}`)); len(errs) != 0 {
		t.Fatal(errs)
	}
	if errs := s.Validate(json.RawMessage(`{"e":`)); len(errs) != 1 || errs[0].Path != "/" {
		t.Fatal(errs)
	}
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"additionalProperties":false`) || !strings.Contains(string(b), `"type":["string","null"]`) || !strings.Contains(string(b), `"description":"text"`) {
		t.Fatal(string(b))
	}
	if b, _ := json.Marshal(Object(nil)); string(b) != `{"additionalProperties":false,"properties":{},"type":"object"}` {
		t.Fatal(string(b))
	}
}
