package feedback

import (
	"time"
)

// Kind of feedback.
type Kind string

const (
	KindBug      Kind = "bug"
	KindIdea     Kind = "idea"
	KindQuestion Kind = "question"
	KindOther    Kind = "other"
)

// Kinds in the order a form lists them.
var Kinds = []Kind{KindBug, KindIdea, KindQuestion, KindOther}

// Valid reports whether k is one of Kinds.
func (k Kind) Valid() bool {
	for _, x := range Kinds {
		if k == x {
			return true
		}
	}
	return false
}

// Status of a feedback item on the site. Spam is shown to its sender as
// closed.
type Status string

const (
	StatusNew         Status = "new"
	StatusTriaged     Status = "triaged"
	StatusInProgress  Status = "in_progress"
	StatusWaitingUser Status = "waiting_user"
	StatusResolved    Status = "resolved"
	StatusClosed      Status = "closed"
)

// Open reports whether the item still waits for something.
func (s Status) Open() bool { return s != StatusResolved && s != StatusClosed }

// Role of an attachment.
type Role string

const (
	RoleImage       Role = "image"
	RoleVideo       Role = "video"
	RoleDiagnostics Role = "diagnostics"
)

// Limits of the protocol (the sites enforce the same).
const (
	MaxImages           = 3
	MaxImageBytes       = 5 << 20
	MaxVideos           = 1
	MaxVideoBytes       = 20 << 20
	MaxDiagnosticsBytes = 2 << 20
	// MaxBodyRunes and MinBodyRunes bound what the user writes.
	MaxBodyRunes = 10000
	MinBodyRunes = 5
	MaxEmailLen  = 254
	// MaxImageSide is the longest side images are scaled to.
	MaxImageSide = 3840
	// Inputs larger than these are refused before any work.
	MaxImageInputBytes = 64 << 20
	MaxVideoInputBytes = 1 << 30
)

// Draft is what the form holds.
type Draft struct {
	Kind  Kind
	Body  string
	Email string
	// Images are the files the user picked (originals: Submit re-encodes
	// them), at most MaxImages.
	Images []string
	// Video is a file the user picked ("" for none).
	Video string
	// Diagnostics: send diagnostics.json (the form's switch, on by default).
	Diagnostics bool
	// Report: the error report the window was opened with (a "Report this
	// problem" link); IncludeReport sends it (on by default; the user may
	// untick it). It goes in diagnostics.json and the request's context.
	Report        *Report
	IncludeReport bool
}

// File is a prepared attachment in the outbox.
type File struct {
	// Ref names the file in the create request ("image-1", "video",
	// "diagnostics").
	Ref    string `json:"ref"`
	Role   Role   `json:"role"`
	Name   string `json:"name"` // the file in the outbox directory
	MIME   string `json:"mime"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"` // hex
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	// Original is the picked file's base name (for the site's list).
	Original string `json:"original,omitempty"`
	// AttachmentID once the site created it, Stored once uploaded.
	AttachmentID string `json:"attachmentId,omitempty"`
	Stored       bool   `json:"stored,omitempty"`
}

// SendState of a submission.
type SendState string

const (
	SendQueued    SendState = "queued"
	SendUploading SendState = "uploading"
	SendSent      SendState = "sent"
	// SendFailed: stopped; Retry or Discard. Rejected (the site said no:
	// a 4xx answer) cannot succeed by retrying the same content.
	SendFailed SendState = "failed"
)

// Submission is one feedback on its way: the outbox entry
// (Dir/outbox/<LocalID>/submission.json).
type Submission struct {
	Schema    string    `json:"schema"`
	LocalID   string    `json:"localId"`
	CreatedAt time.Time `json:"createdAt"`

	App     string   `json:"app"`
	Version string   `json:"version"`
	Build   string   `json:"build,omitempty"`
	Channel string   `json:"channel,omitempty"`
	Locale  string   `json:"locale,omitempty"`
	Kind    Kind     `json:"kind"`
	Body    string   `json:"body"`
	Email   string   `json:"email,omitempty"`
	Files   []File   `json:"files,omitempty"`
	System  System   `json:"system"`
	Context *Context `json:"context,omitempty"`

	// The site's answer to create.
	ID        string `json:"id,omitempty"`
	Number    string `json:"number,omitempty"`
	ViewToken string `json:"viewToken,omitempty"`
	WebURL    string `json:"webUrl,omitempty"`

	State     SendState `json:"state"`
	Attempts  int       `json:"attempts,omitempty"`
	LastError string    `json:"lastError,omitempty"`
	// Rejected: the site refused it (ErrorCode says why).
	Rejected  bool   `json:"rejected,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// Bytes is the size of the files to upload.
func (s *Submission) Bytes() (total, stored int64) {
	for _, f := range s.Files {
		total += f.Bytes
		if f.Stored {
			stored += f.Bytes
		}
	}
	return total, stored
}

// Summary is the body's first line, cut to n runes.
func Summary(body string, n int) string {
	line := body
	for i, r := range body {
		if r == '\n' {
			line = body[:i]
			break
		}
	}
	rs := []rune(line)
	if len(rs) > n {
		return string(rs[:n-1]) + "…"
	}
	return line
}

// Item is a feedback of the history, as the site lists it.
type Item struct {
	ID        string    `json:"id"`
	Number    string    `json:"number"`
	App       string    `json:"app"`
	Kind      Kind      `json:"kind"`
	Status    Status    `json:"status"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Messages counts the thread's messages (the replies and the user's).
	Messages    int       `json:"messages"`
	LastReplyAt time.Time `json:"lastReplyAt,omitzero"`
	WebURL      string    `json:"webUrl,omitempty"`
}

// Message of a thread.
type Message struct {
	ID string `json:"id"`
	// Author: "user" or "support".
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// Attachment as the site lists it in a thread.
type Attachment struct {
	ID    string `json:"id"`
	Role  Role   `json:"role"`
	Name  string `json:"name"`
	MIME  string `json:"mime"`
	Bytes int64  `json:"bytes"`
}

// Thread is one feedback with its messages.
type Thread struct {
	Item        Item         `json:"item"`
	Body        string       `json:"body"`
	Messages    []Message    `json:"messages"`
	Attachments []Attachment `json:"attachments"`
}

// Identity is what the site knows of the sender.
type Identity struct {
	// Linked: the site has an address to answer (the licence code's
	// owner); the email is optional.
	Linked bool `json:"linked"`
	// EmailHint is that address masked ("r***@gmail.com").
	EmailHint string `json:"emailHint,omitempty"`
	// Known: the answer came from the site (false when it could not be
	// asked: the email is then required).
	Known bool `json:"-"`
}
