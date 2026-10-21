// Package activity is the logic of the ActNow apps' activity cards: the
// stack at the foot of the sidebar that says what is going on (the
// family's window spec §2). It has no UI: the app hands it its unfinished
// jobs and the ones that end, and draws the cards it returns, in its own
// words and look.
//
// The rules, the same in every app:
//
//   - Jobs of one kind make one card: a single job shows its own title and
//     how far it is (Frac, or Waiting while queued); several show the
//     kind's title and "done/total", counting the jobs of the kind that
//     ended while others of it still ran.
//   - When the last job of a kind ends and some of them succeeded, the kind
//     shows a finished card (Finished, Done items) for DoneFor (3 s), then
//     goes.
//   - Kinds keep the order in which their first job was listed (the app
//     lists running jobs first, then by age), finished cards after them.
//   - The app puts its own cards around these: one that needs the user
//     (an engine that stopped) first, then what the user started (a
//     recording, a project being made), then these, then downloads.
//
// Stack (stack.go) is where each card sits, closed (the first in full, two
// peeking out under it) or open (a list of Show cards).
//
// A Tracker is not safe for concurrent use: keep it with the window's
// state, on the UI thread. Its zero value is ready.
package activity

import (
	"slices"
	"time"
)

// DoneFor is how long a finished card stays, unless a Tracker says.
const DoneFor = 3 * time.Second

// Job is an unfinished job as the cards see it.
type Job struct {
	ID, Kind, Title string
	Running         bool
	// Fraction is how far a running job is, 0–1; below 0 not known.
	Fraction float64
}

// Card is one card of jobs.
type Card struct {
	// Key is stable while the card shows: "job-" and its kind.
	Key  string
	Kind string
	// Title is the job's title (one job) or the kind's (several, or
	// finished; the job's when the kind has no title of its own).
	Title string
	// Done of Total jobs of the kind are done (several: Total > 1; one job:
	// Total is 1 and Done 0).
	Done, Total int
	// Frac is how far it is, 0–1; -1 going on without a known fraction; 2
	// none (a finished card).
	Frac float64
	// Waiting: a single job, queued.
	Waiting bool
	// Finished: the kind's last job ended; Done jobs succeeded. The card
	// goes after DoneFor.
	Finished bool
	// Jobs are the ids of the card's unfinished jobs (to cancel them).
	Jobs []string
}

// Tracker remembers, by kind, the jobs that ended while others of the kind
// ran (for "done/total") and when a kind's last job ended (its finished
// card).
type Tracker struct {
	// DoneFor is how long a finished card stays; 0: DoneFor.
	DoneFor time.Duration

	done     map[string]int
	finished map[string]time.Time
	titles   map[string]string
}

func (t *Tracker) init() {
	if t.done == nil {
		t.done, t.finished, t.titles = map[string]int{}, map[string]time.Time{}, map[string]string{}
	}
}

func (t *Tracker) doneFor() time.Duration {
	if t.DoneFor > 0 {
		return t.DoneFor
	}
	return DoneFor
}

// Ended records that a job of kind ended; title names the kind on its
// finished card (the kind's title, or the job's when the kind has none);
// succeeded counts it among the done.
func (t *Tracker) Ended(kind, title string, succeeded bool) {
	t.init()
	if succeeded {
		t.done[kind]++
	}
	if title != "" {
		t.titles[kind] = title
	}
}

// Cards are the cards of the jobs: one a kind of active (in the order of
// their first job), then the kinds that finished less than DoneFor before
// now. kindTitle names a kind's jobs when there are several ("" when it
// has no name of its own: the first job's title). It forgets the kinds
// whose finished card has gone.
func (t *Tracker) Cards(now time.Time, active []Job, kindTitle func(kind string) string) []Card {
	t.init()
	byKind := map[string][]Job{}
	var order []string
	for _, j := range active {
		if _, ok := byKind[j.Kind]; !ok {
			order = append(order, j.Kind)
		}
		byKind[j.Kind] = append(byKind[j.Kind], j)
	}
	var out []Card
	for _, kind := range order {
		js := byKind[kind]
		delete(t.finished, kind)
		title := js[0].Title
		name := ""
		if kindTitle != nil {
			name = kindTitle(kind)
		}
		if name != "" && (len(js) > 1 || t.done[kind] > 0) {
			title = name
		}
		c := Card{Key: "job-" + kind, Kind: kind, Title: title, Done: t.done[kind], Total: t.done[kind] + len(js), Frac: -1}
		for _, j := range js {
			c.Jobs = append(c.Jobs, j.ID)
		}
		switch {
		case c.Total > 1:
			c.Frac = float64(c.Done) / float64(c.Total)
		case js[0].Running && js[0].Fraction >= 0:
			c.Frac = js[0].Fraction
		case !js[0].Running:
			c.Waiting = true
		}
		if c.Total == 1 {
			c.Done = 0
		}
		out = append(out, c)
	}
	// Kinds that finished: a moment, then gone.
	kinds := make([]string, 0, len(t.done))
	for k := range t.done {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for _, kind := range kinds {
		if _, running := byKind[kind]; running {
			continue
		}
		at, ok := t.finished[kind]
		if !ok {
			at = now
			t.finished[kind] = at
		}
		n := t.done[kind]
		if now.Sub(at) >= t.doneFor() || n == 0 {
			delete(t.done, kind)
			delete(t.finished, kind)
			continue
		}
		title := t.titles[kind]
		if title == "" {
			title = kind
		}
		out = append(out, Card{Key: "job-" + kind, Kind: kind, Title: title, Done: n, Total: n, Frac: 2, Finished: true})
	}
	return out
}

// Showing reports whether a finished card shows (the window should build
// again within a second, for it to go).
func Showing(cards []Card) bool {
	return slices.ContainsFunc(cards, func(c Card) bool { return c.Finished })
}
