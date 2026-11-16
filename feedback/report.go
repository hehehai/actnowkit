package feedback

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/hehehai/actnowkit/family"
	"github.com/hehehai/actnowkit/redact"
)

// ErrorContext is what an app knows where something failed, for the
// "Report this problem" link: Capture turns it into a Report.
type ErrorContext struct {
	// Title for people ("Transcription failed").
	Title string
	// Code: the app's stable error code ("E-ASR-0042").
	Code string
	// Kind: machine-readable ("engine_out_of_memory").
	Kind string
	// Where in the app, without the user's content: "Meetings ›
	// Transcribe", not the meeting's name (it is scrubbed anyway).
	Where string
	// Err: the error; its text goes through redact and the app's Scrub.
	Err error
}

// Step is one entry of the recent actions (Manager.Step).
type Step struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
}

// Report is an error report: the context, the recent actions and the log
// lines at the time of the error. Capture keeps it on this Mac
// (Dir/reports/<ID>.json); it is sent only when the user sends feedback
// with it (diagnostics.json's "report", and the create request's context).
type Report struct {
	Schema  string    `json:"schema"`
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	Version string    `json:"version,omitempty"`
	Title   string    `json:"title"`
	Code    string    `json:"code,omitempty"`
	Kind    string    `json:"kind,omitempty"`
	Where   string    `json:"where,omitempty"`
	Error   string    `json:"error,omitempty"`
	Steps   []Step    `json:"steps,omitempty"`
	// Logs: the newest lines of the main log at the time (ReportLogLines).
	Logs []string `json:"logs,omitempty"`
}

// ReportKind is the schema kind of a report file.
const ReportKind = "feedback-report"

// Limits of reports and steps.
const (
	MaxSteps       = 30
	ReportLogLines = 40
	ReportMaxAge   = 7 * 24 * time.Hour
	MaxReports     = 50
)

// Context is the part of a Report the site stores in its columns (the
// panel's filters and "common errors"); the rest goes in diagnostics.json.
type Context struct {
	Title    string    `json:"title,omitempty"`
	Code     string    `json:"code,omitempty"`
	Kind     string    `json:"kind,omitempty"`
	Where    string    `json:"where,omitempty"`
	At       time.Time `json:"at,omitzero"`
	ReportID string    `json:"reportId,omitempty"`
	Steps    []string  `json:"steps,omitempty"`
}

// Context of the report, for the create request.
func (r *Report) Context() *Context {
	if r == nil {
		return nil
	}
	c := &Context{Title: r.Title, Code: r.Code, Kind: r.Kind, Where: r.Where, At: r.At, ReportID: r.ID}
	for _, s := range r.Steps {
		c.Steps = append(c.Steps, s.Name)
	}
	return c
}

// Prefill is what opens the feedback window filled in: from Settings or
// the Help menu (zero), from a "Report this problem" link (Kind bug and
// the Report), or with files the app already has (a screenshot).
type Prefill struct {
	Kind   Kind
	Body   string
	Report *Report
	Images []string
	Video  string
}

// stepName keeps an action name short and free of the user's words:
// letters, digits, '_', '-', '.', at most 40 characters.
func stepName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
		if b.Len() >= 40 {
			break
		}
	}
	return b.String()
}

// Step records a user action (an action's name, never what it acted on):
// the newest MaxSteps go with error reports. Cheap; any goroutine.
func (m *Manager) Step(name string) {
	name = stepName(name)
	if name == "" {
		return
	}
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	m.steps = append(m.steps, Step{Name: name, At: m.cfg.Now().UTC()})
	if len(m.steps) > MaxSteps {
		m.steps = append([]Step(nil), m.steps[len(m.steps)-MaxSteps:]...)
	}
}

// Steps are the recent actions, oldest first.
func (m *Manager) Steps() []Step {
	m.stepMu.Lock()
	defer m.stepMu.Unlock()
	return append([]Step(nil), m.steps...)
}

func (m *Manager) reportsDir() string { return filepath.Join(m.cfg.Dir, "reports") }

func newReportID() string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return "ER-" + strings.ToUpper(hex.EncodeToString(b[:]))
}

// Capture makes an error report now: the context (its title, where and
// error through redact.Text, redact.Personal and the app's Scrub), the
// recent steps and the newest ReportLogLines of the main log, saved on
// this Mac. Call it where the app shows the error, then show
// "Report this problem" with the report.
func (m *Manager) Capture(ec ErrorContext) (*Report, error) {
	clean := m.cleaner()
	r := &Report{
		Schema: family.SchemaOf(ReportKind, 1), ID: newReportID(), At: m.cfg.Now().UTC(), Version: m.cfg.Version,
		Title: clean(ec.Title), Code: stepName(ec.Code), Kind: stepName(ec.Kind), Where: clean(ec.Where), Steps: m.Steps(),
	}
	if ec.Err != nil {
		r.Error = clean(ec.Err.Error())
		if len(r.Error) > 2000 {
			r.Error = r.Error[:2000] + "…"
		}
	}
	cc := m.cfg.Collect
	if cc.LogDir != "" && len(cc.Logs) > 0 {
		if lines, _, err := tailLines(filepath.Join(cc.LogDir, cc.Logs[0].Name), ReportLogLines, 64<<10); err == nil {
			for _, l := range lines {
				r.Logs = append(r.Logs, clean(l))
			}
		}
	}
	m.pruneReports()
	data, err := family.MarshalIndent(r)
	if err != nil {
		return r, err
	}
	return r, family.WriteFileAtomic(filepath.Join(m.reportsDir(), r.ID+".json"), data, 0o600)
}

// LoadReport reads a report Capture saved.
func (m *Manager) LoadReport(id string) (*Report, error) {
	if !strings.HasPrefix(id, "ER-") || strings.ContainsAny(id, `/\.`) {
		return nil, os.ErrNotExist
	}
	var r Report
	if err := family.Load(filepath.Join(m.reportsDir(), id+".json"), ReportKind, 1, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// pruneReports drops reports older than ReportMaxAge, and the oldest past
// MaxReports.
func (m *Manager) pruneReports() {
	ents, err := os.ReadDir(m.reportsDir())
	if err != nil {
		return
	}
	type rep struct {
		path string
		at   time.Time
	}
	var reps []rep
	now := m.cfg.Now()
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(m.reportsDir(), e.Name())
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(fi.ModTime()) > ReportMaxAge {
			os.Remove(p)
			continue
		}
		reps = append(reps, rep{p, fi.ModTime()})
	}
	sort.Slice(reps, func(i, j int) bool { return reps[i].at.After(reps[j].at) })
	for i := MaxReports - 1; i < len(reps); i++ {
		os.Remove(reps[i].path)
	}
}

// cleaner is the redaction of a line for the diagnostics and reports:
// redact.Text, redact.Personal, the app's Scrub.
func (m *Manager) cleaner() func(string) string {
	cc := m.cfg.Collect
	pers := redact.ThisUser()
	if cc.Personal != nil {
		pers = *cc.Personal
	}
	return func(s string) string {
		s = pers.Text(redact.Text(s))
		if cc.Scrub != nil {
			s = cc.Scrub(s)
		}
		return s
	}
}

// ErrNoReport: Prefill names a report this Mac no longer has.
var ErrNoReport = errors.New("feedback: no such error report")
