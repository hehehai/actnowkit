package feedback

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/hehehai/actnowkit/redact"
)

// System is this Mac, as diagnostics and the create request tell it: no
// names, serial numbers or addresses.
type System struct {
	OS       string `json:"os,omitempty"`      // "macOS 15.4.1"
	OSBuild  string `json:"osBuild,omitempty"` // "24E263"
	Model    string `json:"model,omitempty"`   // "Mac15,12"
	Chip     string `json:"chip,omitempty"`    // "Apple M3"
	Arch     string `json:"arch,omitempty"`    // "arm64"
	CPUs     int    `json:"cpus,omitempty"`
	MemoryGB int    `json:"memoryGB,omitempty"`
	// FreeDiskGB on the volume of the home folder, rounded.
	FreeDiskGB int    `json:"freeDiskGB,omitempty"`
	Locale     string `json:"locale,omitempty"` // the app's language
	// TZOffset: minutes east of UTC (to read the log times).
	TZOffset int `json:"tzOffset"`
}

// ThisSystem is System of the running Mac.
func ThisSystem() System {
	s := thisSystem()
	s.OS = strings.TrimSpace(s.OS)
	s.OSBuild = strings.TrimSpace(s.OSBuild)
	s.Arch = runtime.GOARCH
	if h, err := os.UserHomeDir(); err == nil {
		s.FreeDiskGB = int((freeBytes(h) + 1<<29) >> 30)
	}
	_, off := time.Now().Zone()
	s.TZOffset = off / 60
	return s
}

// LogSpec names a log of the app's logs folder to include.
type LogSpec struct {
	// Name of the current file ("dicta.log"); rotated copies are Name.1,
	// Name.2… (the apps' logs package).
	Name string
	// Lines: the newest lines to include (0: 1000 for the first spec, 200
	// for the others), within MaxBytes (0: 256 KB).
	Lines    int
	MaxBytes int
}

// CollectConfig says what Collect gathers.
type CollectConfig struct {
	// LogDir is the app's logs folder (mygo.PathLogs).
	LogDir string
	// Logs to include, the app's main log first. Every rotated copy of
	// them is also searched for warnings and errors (ProblemLines).
	Logs []LogSpec
	// CrashFile in LogDir ("crash.log"; "" for none): its last report.
	CrashFile string
	// ProblemDays: how far back the warnings and errors go (0: 7).
	ProblemDays int
	// ProblemLines: at most this many of them (0: 300).
	ProblemLines int
	// Settings returns the app's settings as JSON, already without
	// secrets and the user's own words (the apps' diagnostic bundle has
	// it). Optional. Settings and Extra are called from the goroutine
	// running Submit or Diagnostics, not the UI thread: an app whose state
	// lives on the UI thread hands them a snapshot.
	Settings func() json.RawMessage
	// Extra returns lines the app adds ("Engine": "1.2 (ready)",
	// "Loaded models": …). Optional.
	Extra func() map[string]string
	// Scrub is the app's own redaction of a log line (the tails, the
	// problems, the crash), after redact.Text and redact.Personal
	// (Inboxly: subjects; Dicta: transcript text). Settings and Extra do
	// not go through it.
	Scrub func(string) string
	// Personal: who to mask (redact.ThisUser() when zero).
	Personal *redact.Personal
}

// LogTail is the newest lines of one log.
type LogTail struct {
	File      string   `json:"file"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated,omitempty"`
}

// Diagnostics is diagnostics.json.
type Diagnostics struct {
	Schema      string            `json:"schema"`
	CollectedAt time.Time         `json:"collectedAt"`
	App         AppInfo           `json:"app"`
	System      System            `json:"system"`
	Extra       map[string]string `json:"extra,omitempty"`
	Settings    json.RawMessage   `json:"settings,omitempty"`
	// Problems: the warnings and errors of the last ProblemDays, oldest
	// first, from every log and its rotated copies.
	Problems []string  `json:"problems,omitempty"`
	Logs     []LogTail `json:"logs,omitempty"`
	Crash    string    `json:"crash,omitempty"`
}

// AppInfo is the app part of Diagnostics.
type AppInfo struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Build    string `json:"build,omitempty"`
	Channel  string `json:"channel,omitempty"`
	Go       string `json:"go"`
	Licensed string `json:"licence,omitempty"` // edition and state, never the code
}

// DiagnosticsSchema of diagnostics.json.
const DiagnosticsSchema = "actnow.feedback-diagnostics/1"

// levelRE finds a warning or error line of the apps' log format
// ("2026-11-16 10:00:00.000 ERROR   source  message") or a Go panic.
var (
	levelRE = regexp.MustCompile(`(?i)\b(ERROR|WARN|WARNING|FATAL|PANIC|CRITICAL)\b|^panic:|^fatal error:`)
	timeRE  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2})`)
)

// Collect gathers the diagnostics. Every line goes through redact.Text,
// then redact.Personal, then cfg.Scrub; what it cannot read it leaves
// out. The result fits MaxDiagnosticsBytes (the logs are cut first).
func Collect(cfg CollectConfig, app AppInfo, now time.Time) Diagnostics {
	if app.Go == "" {
		app.Go = runtime.Version()
	}
	d := Diagnostics{Schema: DiagnosticsSchema, CollectedAt: now.UTC(), App: app, System: ThisSystem()}
	pers := redact.ThisUser()
	if cfg.Personal != nil {
		pers = *cfg.Personal
	}
	// Settings and Extra come from the app already without its users'
	// words: they get the secret and personal rules only; Scrub is for
	// the log lines.
	base := func(s string) string { return pers.Text(redact.Text(s)) }
	clean := func(s string) string {
		s = base(s)
		if cfg.Scrub != nil {
			s = cfg.Scrub(s)
		}
		return s
	}
	if cfg.Extra != nil {
		d.Extra = map[string]string{}
		for k, v := range cfg.Extra() {
			d.Extra[k] = base(v)
		}
	}
	if cfg.Settings != nil {
		if raw := cfg.Settings(); len(raw) > 0 && json.Valid(raw) {
			s := base(string(raw))
			if json.Valid([]byte(s)) {
				d.Settings = json.RawMessage(s)
			}
		}
	}
	if cfg.LogDir != "" {
		days := cfg.ProblemDays
		if days <= 0 {
			days = 7
		}
		maxProblems := cfg.ProblemLines
		if maxProblems <= 0 {
			maxProblems = 300
		}
		since := now.Add(-time.Duration(days) * 24 * time.Hour)
		for i, spec := range cfg.Logs {
			lines, maxBytes := spec.Lines, spec.MaxBytes
			if lines <= 0 {
				lines = 200
				if i == 0 {
					lines = 1000
				}
			}
			if maxBytes <= 0 {
				maxBytes = 256 << 10
			}
			path := filepath.Join(cfg.LogDir, spec.Name)
			if tail, cut, err := tailLines(path, lines, maxBytes); err == nil {
				for j := range tail {
					tail[j] = clean(tail[j])
				}
				d.Logs = append(d.Logs, LogTail{File: spec.Name, Lines: tail, Truncated: cut})
			}
			for _, f := range rotated(path) {
				d.Problems = append(d.Problems, problemLines(f, since)...)
			}
		}
		sort.SliceStable(d.Problems, func(i, j int) bool { return lineTime(d.Problems[i]) < lineTime(d.Problems[j]) })
		if len(d.Problems) > maxProblems {
			d.Problems = d.Problems[len(d.Problems)-maxProblems:]
		}
		for i := range d.Problems { // only the lines kept: redaction costs
			d.Problems[i] = clean(d.Problems[i])
		}
		if cfg.CrashFile != "" {
			d.Crash = clean(lastCrash(filepath.Join(cfg.LogDir, cfg.CrashFile)))
		}
	}
	d.fit(MaxDiagnosticsBytes)
	return d
}

// rotated lists path and its copies path.1 … path.9 that exist, oldest
// first.
func rotated(path string) []string {
	var out []string
	for i := 9; i >= 1; i-- {
		p := fmt.Sprintf("%s.%d", path, i)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	if _, err := os.Stat(path); err == nil {
		out = append(out, path)
	}
	return out
}

// lineTime is the line's leading time stamp ("" when none: sorted first).
func lineTime(s string) string {
	if m := timeRE.FindStringSubmatch(s); m != nil {
		return strings.Replace(m[1], "T", " ", 1)
	}
	return ""
}

// problemLines are the warning and error lines of a file at or after
// since (lines without a time are kept), not yet redacted.
func problemLines(path string, since time.Time) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	cut := since.Format("2006-01-02 15:04:05")
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !levelRE.MatchString(line) {
			continue
		}
		if t := lineTime(line); t != "" && t < cut {
			continue
		}
		if len(line) > 2000 {
			line = line[:2000] + "…"
		}
		out = append(out, line)
	}
	return out
}

// tailLines reads the last n lines of a file within maxBytes; cut is true
// when there was more.
func tailLines(path string, n, maxBytes int) (lines []string, cut bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	start := st.Size() - int64(maxBytes)
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)))
	if err != nil {
		return nil, false, err
	}
	if start > 0 {
		cut = true
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:] // a partial first line
		}
	}
	all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(all) == 1 && all[0] == "" {
		return nil, cut, nil
	}
	if len(all) > n {
		all, cut = all[len(all)-n:], true
	}
	return all, cut, nil
}

// lastCrash is the last report of a crash.log: from the last line naming
// a session (the apps' crashlog writes "=== <time> <app> <version> …")
// or the last "panic:"/"fatal error:", at most 64 KB.
func lastCrash(path string) string {
	lines, _, err := tailLines(path, 4000, 256<<10)
	if err != nil || len(lines) == 0 {
		return ""
	}
	start := -1
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if strings.HasPrefix(l, "===") || strings.HasPrefix(l, "panic:") || strings.HasPrefix(l, "fatal error:") {
			start = i
			if strings.HasPrefix(l, "===") {
				break
			}
		}
	}
	if start < 0 {
		start = 0
	}
	s := strings.Join(lines[start:], "\n")
	if len(s) > 64<<10 {
		s = s[:64<<10] + "\n…"
	}
	return s
}

// fit cuts the logs (oldest lines first), then the problems, until the
// JSON fits limit bytes.
func (d *Diagnostics) fit(limit int) {
	for i := 0; i < 64; i++ {
		b, err := json.Marshal(d)
		if err != nil || len(b) <= limit {
			return
		}
		over := len(b) - limit
		cutSome := func(lines []string) ([]string, bool) {
			n := 0
			for n < len(lines) && over > 0 {
				over -= len(lines[n]) + 4
				n++
			}
			if n == 0 {
				return lines, false
			}
			if n < len(lines)/4 && len(lines) > 8 {
				n = len(lines) / 4 // in bigger steps
			}
			return lines[n:], true
		}
		done := false
		for j := range d.Logs {
			if len(d.Logs[j].Lines) > 0 {
				d.Logs[j].Lines, done = cutSome(d.Logs[j].Lines)
				d.Logs[j].Truncated = true
				break
			}
		}
		if !done && len(d.Problems) > 0 {
			d.Problems, done = cutSome(d.Problems)
		}
		if !done && len(d.Crash) > 0 {
			d.Crash = d.Crash[:len(d.Crash)/2] + "\n…"
			done = true
		}
		if !done {
			d.Settings = nil
			d.Extra = nil
			return
		}
	}
}

// Preview is diagnostics.json as the user reads it before sending: the
// JSON indented.
func (d Diagnostics) Preview() string {
	b, _ := json.MarshalIndent(d, "", "  ")
	return string(b)
}
