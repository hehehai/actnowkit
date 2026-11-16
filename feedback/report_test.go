package feedback

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/actnowkit/redact"
)

func TestStepsAndCapture(t *testing.T) {
	site := newFakeSite()
	defer site.Close()
	logDir := t.TempDir()
	var lines strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&lines, "2026-11-16 10:00:%02d.000 INFO app line %d for ann@example.com token=s3cr3t\n", i, i)
	}
	os.WriteFile(filepath.Join(logDir, "dicta.log"), []byte(lines.String()), 0o600)
	now := time.Date(2026, 11, 16, 10, 42, 7, 0, time.UTC)
	m, err := New(Config{App: "dicta", Version: "0.1.0-beta.7", BaseURL: site.srv.URL, Dir: t.TempDir(),
		Collect: CollectConfig{LogDir: logDir, Logs: []LogSpec{{Name: "dicta.log"}}, Personal: &redact.Personal{Home: "/Users/ann"},
			Scrub: func(s string) string { return strings.ReplaceAll(s, "Weekly sync", "…") }},
		Converter: noConv{}, Now: func() time.Time { return now }, Sleep: func(context.Context, time.Duration) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 35; i++ {
		m.Step(fmt.Sprintf("step %d", i))
	}
	m.Step("  import audio from \"/Users/ann/Weekly sync.m4a\"  ")
	steps := m.Steps()
	if len(steps) != MaxSteps || steps[0].Name != "step_6" {
		t.Fatalf("steps: %d, first %q", len(steps), steps[0].Name)
	}
	if last := steps[len(steps)-1].Name; strings.ContainsAny(last, `"/ `) || len(last) > 40 {
		t.Errorf("step name kept the user's words: %q", last)
	}

	rep, err := m.Capture(ErrorContext{Title: "Transcription failed", Code: "E-ASR-0042", Kind: "engine_out_of_memory",
		Where: "Meetings › Weekly sync › Transcribe", Err: errors.New("open /Users/ann/Weekly sync.m4a: out of memory; token=abc123")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rep.ID, "ER-") || len(rep.Logs) != ReportLogLines || rep.Version != "0.1.0-beta.7" {
		t.Fatalf("report: %+v", rep)
	}
	all, _ := json.Marshal(rep)
	for _, leak := range []string{"Weekly sync", "ann@example.com", "s3cr3t", "abc123", "/Users/ann"} {
		if strings.Contains(string(all), leak) {
			t.Errorf("report keeps %q", leak)
		}
	}
	if rep.Where != "Meetings › … › Transcribe" {
		t.Errorf("where %q", rep.Where)
	}
	back, err := m.LoadReport(rep.ID)
	if err != nil || back.Code != "E-ASR-0042" {
		t.Fatalf("load: %+v %v", back, err)
	}
	if _, err := m.LoadReport("../x"); err == nil {
		t.Error("loaded a path")
	}

	// Sent with the report: the context in the request, the report in
	// diagnostics.json, even with diagnostics off.
	d := Prefill{Report: rep}.Draft()
	if d.Kind != KindBug || !d.IncludeReport || !d.Diagnostics {
		t.Fatalf("draft %+v", d)
	}
	d.Body, d.Email, d.Diagnostics = "It stops at 63%", "a@example.com", false
	sub, err := m.Submit(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	it := site.items[sub.ID]
	c := it.Req.Context
	if c == nil || c.Code != "E-ASR-0042" || c.ReportID != rep.ID || c.Where != "Meetings › … › Transcribe" || len(c.Steps) != MaxSteps {
		t.Fatalf("context %+v", c)
	}
	var diag Diagnostics
	for _, f := range it.Files {
		if f.Meta.Role == RoleDiagnostics {
			json.Unmarshal(f.Data, &diag)
		}
	}
	if diag.Report == nil || diag.Report.ID != rep.ID || len(diag.Logs) != 0 {
		t.Errorf("diagnostics: report %v, %d logs", diag.Report, len(diag.Logs))
	}

	// Unticked: neither.
	d2 := Prefill{Report: rep}.Draft()
	d2.Body, d2.Email, d2.IncludeReport, d2.Diagnostics = "Another one", "a@example.com", false, false
	sub2, err := m.Submit(context.Background(), d2)
	if err != nil {
		t.Fatal(err)
	}
	if it2 := site.items[sub2.ID]; it2.Req.Context != nil || len(it2.Files) != 0 {
		t.Errorf("unticked report sent: %+v, %d files", it2.Req.Context, len(it2.Files))
	}

	// Old reports go.
	m.cfg.Now = func() time.Time { return now.Add(ReportMaxAge + time.Hour) }
	os.Chtimes(filepath.Join(m.reportsDir(), rep.ID+".json"), now, now)
	m.pruneReports()
	if _, err := m.LoadReport(rep.ID); err == nil {
		t.Error("old report kept")
	}
}

// movieWithDuration is a minimal QuickTime file: ftyp, moov/mvhd.
func movieWithDuration(version byte, scale uint32, dur uint64) []byte {
	ftyp := append([]byte{0, 0, 0, 16}, []byte("ftypqt  \x00\x00\x00\x00")...)
	var mvhd []byte
	if version == 1 {
		body := make([]byte, 4+8+8+4+8)
		body[0] = 1
		binary.BigEndian.PutUint32(body[20:], scale)
		binary.BigEndian.PutUint64(body[24:], dur)
		mvhd = append(binary.BigEndian.AppendUint32(nil, uint32(8+len(body))), "mvhd"...)
		mvhd = append(mvhd, body...)
	} else {
		body := make([]byte, 4+4+4+4+4)
		binary.BigEndian.PutUint32(body[12:], scale)
		binary.BigEndian.PutUint32(body[16:], uint32(dur))
		mvhd = append(binary.BigEndian.AppendUint32(nil, uint32(8+len(body))), "mvhd"...)
		mvhd = append(mvhd, body...)
	}
	free := append([]byte{0, 0, 0, 12}, []byte("free1234")...)
	moovBody := append(free, mvhd...)
	moov := append(binary.BigEndian.AppendUint32(nil, uint32(8+len(moovBody))), "moov"...)
	moov = append(moov, moovBody...)
	mdat := append([]byte{0, 0, 0, 16}, []byte("mdat01234567")...)
	return append(append(ftyp, mdat...), moov...)
}

type thumbConv struct{ noConv }

func (thumbConv) VideoThumbnail(_ context.Context, src, dst string, px int) error {
	return copyFile(filepath.Join(filepath.Dir(src), "frame.png"), dst)
}

func TestVideoDurationAndThumbnail(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		v     byte
		scale uint32
		dur   uint64
		want  time.Duration
	}{{0, 600, 25200, 42 * time.Second}, {1, 1000, 90500, 90500 * time.Millisecond}} {
		p := filepath.Join(dir, fmt.Sprintf("m%d.mov", c.v))
		os.WriteFile(p, movieWithDuration(c.v, c.scale, c.dur), 0o600)
		if got, err := VideoDuration(p); err != nil || got != c.want {
			t.Errorf("v%d: %v %v", c.v, got, err)
		}
	}
	bad := filepath.Join(dir, "bad.mp4")
	os.WriteFile(bad, []byte("\x00\x00\x00\x10ftypisom\x00\x00\x00\x00"), 0o600)
	if _, err := VideoDuration(bad); err == nil {
		t.Error("no moov, no error")
	}

	png := writePNG(t, dir, "big.png", 1200, 600, false)
	img, err := Thumbnail(context.Background(), noConv{}, png, 200)
	if err != nil || img.Bounds().Dx() != 200 || img.Bounds().Dy() != 100 {
		t.Fatalf("image thumb: %v %v", img, err)
	}
	writePNG(t, dir, "frame.png", 320, 180, false)
	mov := filepath.Join(dir, "m0.mov")
	if _, err := Thumbnail(context.Background(), noConv{}, mov, 128); !errors.Is(err, ErrUnsupported) {
		t.Errorf("video without thumbnailer: %v", err)
	}
	img, err = Thumbnail(context.Background(), thumbConv{}, mov, 128)
	if err != nil || img.Bounds().Dx() != 128 {
		t.Fatalf("video thumb: %v %v", img, err)
	}
}
