package feedback

import (
	"net/mail"
	"os"
	"strings"
	"unicode/utf8"
)

// Problem is a reason the form cannot be sent yet; the app shows the
// matching text (ProblemText) beside the field.
type Problem string

const (
	ProblemKind          Problem = "kind"
	ProblemBodyShort     Problem = "body_short"
	ProblemBodyLong      Problem = "body_long"
	ProblemEmailRequired Problem = "email_required"
	ProblemEmailInvalid  Problem = "email_invalid"
	ProblemTooManyImages Problem = "too_many_images"
	ProblemImageMissing  Problem = "image_missing"
	ProblemImageTooLarge Problem = "image_too_large"
	ProblemImageType     Problem = "image_type"
	ProblemVideoMissing  Problem = "video_missing"
	ProblemVideoTooLarge Problem = "video_too_large"
	ProblemVideoType     Problem = "video_type"
)

// ValidEmail reports whether s looks like one plain address.
func ValidEmail(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > MaxEmailLen || strings.ContainsAny(s, " <>\"") {
		return false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	return at > 0 && strings.Contains(s[at+1:], ".")
}

// Validate checks a draft against the limits; who says whether the email
// may be empty (Identity.Linked from the site). The files are checked by
// their size and first bytes only; Submit prepares them.
func Validate(d Draft, who Identity) []Problem {
	var ps []Problem
	if !d.Kind.Valid() {
		ps = append(ps, ProblemKind)
	}
	n := utf8.RuneCountInString(strings.TrimSpace(d.Body))
	switch {
	case n < MinBodyRunes:
		ps = append(ps, ProblemBodyShort)
	case n > MaxBodyRunes:
		ps = append(ps, ProblemBodyLong)
	}
	email := strings.TrimSpace(d.Email)
	switch {
	case email == "" && !who.Linked:
		ps = append(ps, ProblemEmailRequired)
	case email != "" && !ValidEmail(email):
		ps = append(ps, ProblemEmailInvalid)
	}
	if len(d.Images) > MaxImages {
		ps = append(ps, ProblemTooManyImages)
	}
	for _, p := range d.Images {
		if pr := CheckImage(p); pr != "" {
			ps = append(ps, pr)
			break
		}
	}
	if d.Video != "" {
		if pr := CheckVideo(d.Video); pr != "" {
			ps = append(ps, pr)
		}
	}
	return ps
}

// CheckImage is the check of a picked image (when it is picked): it
// exists, is not over MaxImageInputBytes and looks like an image.
func CheckImage(path string) Problem {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return ProblemImageMissing
	}
	if st.Size() > MaxImageInputBytes {
		return ProblemImageTooLarge
	}
	if imageFormat(path) == "" {
		return ProblemImageType
	}
	return ""
}

// CheckVideo is the check of a picked video: it exists, is not over
// MaxVideoInputBytes and is an MP4 or QuickTime movie.
func CheckVideo(path string) Problem {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return ProblemVideoMissing
	}
	if st.Size() > MaxVideoInputBytes {
		return ProblemVideoTooLarge
	}
	if videoMIME(path) == "" {
		return ProblemVideoType
	}
	return ""
}
