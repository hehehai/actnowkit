package activity

import (
	"testing"
	"time"
)

func kindTitle(kind string) string {
	if kind == "export" {
		return "Exporting"
	}
	return ""
}

func TestCardsByKind(t *testing.T) {
	var tr Tracker
	now := time.Unix(1000, 0)
	cs := tr.Cards(now, []Job{
		{ID: "a", Kind: "export", Title: "Export A", Running: true, Fraction: 0.5},
		{ID: "b", Kind: "ai", Title: "Summarize B"},
		{ID: "c", Kind: "export", Title: "Export C"},
	}, kindTitle)
	if len(cs) != 2 {
		t.Fatalf("cards %+v", cs)
	}
	if c := cs[0]; c.Key != "job-export" || c.Title != "Exporting" || c.Done != 0 || c.Total != 2 || c.Frac != 0 || len(c.Jobs) != 2 {
		t.Errorf("exports %+v", c)
	}
	if c := cs[1]; c.Title != "Summarize B" || c.Total != 1 || !c.Waiting || c.Frac != -1 {
		t.Errorf("ai %+v", c)
	}
	// One running job: its own title and fraction.
	cs = tr.Cards(now, []Job{{ID: "a", Kind: "export", Title: "Export A", Running: true, Fraction: 0.25}}, kindTitle)
	if c := cs[0]; c.Title != "Export A" || c.Frac != 0.25 || c.Waiting || c.Done != 0 || c.Total != 1 {
		t.Errorf("one %+v", c)
	}
	// Running without a fraction.
	cs = tr.Cards(now, []Job{{ID: "a", Kind: "ai", Title: "Summarize", Running: true, Fraction: -1}}, kindTitle)
	if c := cs[0]; c.Frac != -1 || c.Waiting {
		t.Errorf("going on %+v", c)
	}
}

func TestDoneCountsAndFinished(t *testing.T) {
	tr := Tracker{DoneFor: 3 * time.Second}
	now := time.Unix(1000, 0)
	tr.Ended("export", "Exporting", true)
	tr.Ended("export", "Exporting", false) // failed: not done
	cs := tr.Cards(now, []Job{{ID: "c", Kind: "export", Title: "Export C", Running: true, Fraction: 0.9}}, kindTitle)
	if c := cs[0]; c.Title != "Exporting" || c.Done != 1 || c.Total != 2 || c.Frac != 0.5 {
		t.Errorf("1/2 %+v", c)
	}
	tr.Ended("export", "Exporting", true)
	cs = tr.Cards(now, nil, kindTitle)
	if len(cs) != 1 || !cs[0].Finished || cs[0].Done != 2 || cs[0].Frac != 2 || cs[0].Title != "Exporting" || !Showing(cs) {
		t.Fatalf("finished %+v", cs)
	}
	if cs = tr.Cards(now.Add(2*time.Second), nil, kindTitle); len(cs) != 1 {
		t.Errorf("gone too soon %+v", cs)
	}
	if cs = tr.Cards(now.Add(3*time.Second), nil, kindTitle); len(cs) != 0 || Showing(cs) {
		t.Errorf("stays %+v", cs)
	}
	// Forgotten: a new job of the kind starts from nothing.
	cs = tr.Cards(now.Add(4*time.Second), []Job{{ID: "d", Kind: "export", Title: "Export D", Running: true, Fraction: 0}}, kindTitle)
	if c := cs[0]; c.Title != "Export D" || c.Total != 1 {
		t.Errorf("after %+v", c)
	}
	// Only failures: no finished card.
	var tr2 Tracker
	tr2.Ended("ai", "Summarize", false)
	if cs := tr2.Cards(now, nil, kindTitle); len(cs) != 0 {
		t.Errorf("failed only %+v", cs)
	}
}

func TestStack(t *testing.T) {
	if Height(0, false) != 0 || Height(1, true) != CardHeight || Height(3, false) != CardHeight+2*CardPeek || Height(9, false) != CardHeight+2*CardPeek {
		t.Errorf("closed heights")
	}
	if Height(9, true) != 4*CardHeight+3*CardGap || Height(2, true) != 2*CardHeight+CardGap {
		t.Errorf("open heights")
	}
	ps := Poses(6, false)
	if len(ps) != Show || ps[0] != (Pose{Scale: 1}) {
		t.Fatalf("poses %+v", ps)
	}
	if p := ps[1]; p.Top != CardHeight+CardGap || p.Y != CardPeek-p.Top || p.Scale != 1-ShrinkStep || p.Fade {
		t.Errorf("second %+v", p)
	}
	if p := ps[3]; !p.Fade || p.Y != 2*CardPeek-p.Top {
		t.Errorf("fourth %+v", p)
	}
	for _, p := range Poses(6, true) {
		if p.Y != 0 || p.Scale != 1 || p.Fade {
			t.Errorf("open %+v", p)
		}
	}
}
