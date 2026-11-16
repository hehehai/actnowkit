package feedbackui

import (
	"strings"

	"github.com/hehehai/actnowkit/family"
	"github.com/hehehai/actnowkit/feedback"
)

// Texts are the window's wordings, in English and Simplified Chinese:
// actnowkit's feedback.Text* by default (DefaultTexts). An app changes
// one by setting it on a copy (Dicta's buttons carry no ellipsis:
// Texts.Preview = family.Text{EN: "See what is sent", ...}); Options.T,
// when set, translates every one of them from the app's catalogue.
// Placeholders ({app}, {site}, {count}…) are filled after T.
type Texts struct {
	WindowTitle, WindowSub, ReportTitle, ReportLink                                       family.Text
	Kind, KindBug, KindIdea, KindQuestion, KindOther                                      family.Text
	Body, BodyPlaceholder                                                                 family.Text
	Email, EmailPlaceholder, EmailLinked, EmailNeeded, Optional                           family.Text
	Attachments, Add, AddSub, AddFiles, AttachHint, AttachCount, Remove, OpenWith         family.Text
	ImageOf, VideoOf, Previous, Next                                                      family.Text
	Diagnostics, DiagnosticsHint, Preview, WhatIsSent, WhatIsSentDiag, WhatIsSentReport   family.Text
	Copy, Copied, Done                                                                    family.Text
	IncludeReport, ContextError, ContextWhere, ContextWhen, ContextSteps, ContextReport   family.Text
	ReportLines, Today                                                                    family.Text
	Privacy, PrivacyLink, HistoryCount, History, Cancel, Close, Send                      family.Text
	Preparing, Sending, Failed, SendFailed, RateLimited, Retry, Discard                   family.Text
	SentTitle, SentBody, ViewIt                                                           family.Text
	NoHistory, Pending, NewReply, Refresh, Back, Loading, Unreachable, ViewOnWeb          family.Text
	Reply, YourReply, You, Support, OpenWindow                                            family.Text
	StatusNew, StatusTriaged, StatusInProgress, StatusWaitingUser, StatusResolved, Closed family.Text
}

// DefaultTexts are actnowkit's.
func DefaultTexts() *Texts {
	return &Texts{
		WindowTitle: feedback.TextWindowTitle, WindowSub: feedback.TextWindowSub,
		ReportTitle: feedback.TextReportTitle, ReportLink: feedback.TextReportLink,
		Kind: feedback.TextKind, KindBug: feedback.TextKindBug, KindIdea: feedback.TextKindIdea,
		KindQuestion: feedback.TextKindQuestion, KindOther: feedback.TextKindOther,
		Body: feedback.TextBody, BodyPlaceholder: feedback.TextBodyPlaceholder,
		Email: feedback.TextEmail, EmailPlaceholder: feedback.TextEmailPlaceholder, EmailLinked: feedback.TextEmailLinked,
		EmailNeeded: feedback.TextEmailNeeded, Optional: feedback.TextOptional,
		Attachments: feedback.TextAttachments, Add: feedback.TextAdd, AddSub: feedback.TextAddSub, AddFiles: feedback.TextAddFiles,
		AttachHint: feedback.TextAttachHint, AttachCount: feedback.TextAttachCount, Remove: feedback.TextRemove,
		OpenWith: feedback.TextOpenWith, ImageOf: feedback.TextImageOf, VideoOf: feedback.TextVideoOf,
		Previous: feedback.TextPrevious, Next: feedback.TextNext,
		Diagnostics: feedback.TextDiagnostics, DiagnosticsHint: feedback.TextDiagnosticsHint, Preview: feedback.TextSeeWhatIsSent,
		WhatIsSent: feedback.TextWhatIsSent, WhatIsSentDiag: feedback.TextWhatIsSentDiagnostics, WhatIsSentReport: feedback.TextWhatIsSentReport,
		Copy: feedback.TextCopy, Copied: feedback.TextCopied, Done: feedback.TextDone,
		IncludeReport: feedback.TextIncludeReport, ContextError: feedback.TextContextError, ContextWhere: feedback.TextContextWhere,
		ContextWhen: feedback.TextContextWhen, ContextSteps: feedback.TextContextSteps, ContextReport: feedback.TextContextReport,
		ReportLines: feedback.TextReportLines, Today: feedback.TextToday,
		Privacy: feedback.TextPrivacy, PrivacyLink: feedback.TextPrivacyLink, HistoryCount: feedback.TextHistoryCount,
		History: feedback.TextHistory, Cancel: feedback.TextCancel, Close: feedback.TextClose, Send: feedback.TextSend,
		Preparing: feedback.TextPreparing, Sending: feedback.TextSending, Failed: feedback.TextFailed, SendFailed: feedback.TextSendFailed,
		RateLimited: feedback.TextRateLimited, Retry: feedback.TextRetry, Discard: feedback.TextDiscard,
		SentTitle: feedback.TextSentTitle, SentBody: feedback.TextSentBody, ViewIt: feedback.TextViewIt,
		NoHistory: feedback.TextNoHistory, Pending: feedback.TextPending, NewReply: feedback.TextNewReply,
		Refresh: feedback.TextRefresh, Back: feedback.TextBack, Loading: feedback.TextLoading, Unreachable: feedback.TextUnreachable,
		ViewOnWeb: feedback.TextViewOnWeb, Reply: feedback.TextReply, YourReply: feedback.TextYourReply,
		You: feedback.TextYou, Support: feedback.TextSupport, OpenWindow: feedback.TextOpenWindow,
		StatusNew: feedback.TextStatusNew, StatusTriaged: feedback.TextStatusTriaged, StatusInProgress: feedback.TextStatusInProgress,
		StatusWaitingUser: feedback.TextStatusWaitingUser, StatusResolved: feedback.TextStatusResolved, Closed: feedback.TextStatusClosed,
	}
}

// zh reports whether the app's language is Chinese.
func (w *Window) zh() bool {
	if w.opt.Lang == nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(w.opt.Lang()), "zh")
}

// tx is t in the app's language with its placeholders filled: {app} and
// {site} always, the others from kv.
func (w *Window) tx(t family.Text, kv ...string) string {
	var s string
	switch {
	case w.opt.T != nil:
		s = w.opt.T(t)
	case w.zh() && t.ZH != "":
		s = t.ZH
	default:
		s = t.EN
	}
	if !strings.Contains(s, "{") {
		return s
	}
	pairs := []string{"{app}", w.opt.AppName, "{site}", w.opt.Site}
	for i := 0; i+1 < len(kv); i += 2 {
		pairs = append(pairs, "{"+kv[i]+"}", kv[i+1])
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

func (w *Window) kindText(k feedback.Kind) family.Text {
	x := w.texts
	switch k {
	case feedback.KindBug:
		return x.KindBug
	case feedback.KindIdea:
		return x.KindIdea
	case feedback.KindQuestion:
		return x.KindQuestion
	}
	return x.KindOther
}

func (w *Window) statusText(s feedback.Status) family.Text {
	x := w.texts
	switch s {
	case feedback.StatusTriaged:
		return x.StatusTriaged
	case feedback.StatusInProgress:
		return x.StatusInProgress
	case feedback.StatusWaitingUser:
		return x.StatusWaitingUser
	case feedback.StatusResolved:
		return x.StatusResolved
	case feedback.StatusClosed:
		return x.Closed
	}
	return x.StatusNew
}

func (w *Window) problemText(p feedback.Problem) string {
	return w.tx(feedback.ProblemText(p))
}
