package feedbacktest

import (
	"context"
	"testing"

	"github.com/hehehai/actnowkit/feedback"
)

func TestSiteTakesFeedback(t *testing.T) {
	s := New()
	defer s.Close()
	s.Tokens["tok"] = "ann@example.com"
	m, err := feedback.New(feedback.Config{App: "dicta", Version: "1.0", BaseURL: s.URL, Dir: t.TempDir(), HTTPClient: s.Client(),
		LicenceToken: func() string { return "tok" }})
	if err != nil {
		t.Fatal(err)
	}
	who, err := m.Identity(context.Background())
	if err != nil || !who.Linked || who.EmailHint != "a***@example.com" {
		t.Fatalf("identity %+v %v", who, err)
	}
	sub, err := m.Submit(context.Background(), feedback.Draft{Kind: feedback.KindIdea, Body: "Dark icons, please.", Diagnostics: true})
	if err != nil {
		t.Fatal(err)
	}
	it := s.Items()[sub.ID]
	if it == nil || !it.Completed || it.Email != "ann@example.com" || sub.Number != "FB-1042" {
		t.Fatalf("item %+v, sub %+v", it, sub)
	}
	s.SupportReplies(sub.ID, "Noted!")
	th, err := m.Thread(context.Background(), sub.ID)
	if err != nil || len(th.Messages) != 1 {
		t.Fatalf("thread %+v %v", th, err)
	}
}
