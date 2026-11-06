package llm

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// The app's sentinels, as an app sets them.
var (
	errCancelled     = errors.New("cancelled")
	errNotDownloaded = errors.New("model not downloaded")
	errInvalid       = errors.New("invalid request")
	errUnsupported   = errors.New("not supported")
	errLicence       = errors.New("licence not accepted")
	errBusy          = errors.New("busy")
	errEngine        = errors.New("engine unavailable")
)

func TestMain(m *testing.M) {
	SetSentinels(Sentinels{Cancelled: errCancelled, NotDownloaded: errNotDownloaded, Invalid: errInvalid,
		Unsupported: errUnsupported, LicenceRequired: errLicence, Busy: errBusy, Engine: errEngine})
	os.Exit(m.Run())
}

// protoError is an app's engine protocol error.
type protoError struct {
	Code, Message string
	Data          json.RawMessage
}

func (e *protoError) Error() string { return e.Code + ": " + e.Message }
func (e *protoError) EngineError() (string, string, json.RawMessage) {
	return e.Code, e.Message, e.Data
}

// The skills of a test app.
const (
	skillTranslate Skill = "translate"
	skillTitle     Skill = "title"
)

func testDefaults(s Skill) SkillDefaults {
	switch s {
	case skillTranslate:
		return SkillDefaults{Temperature: 0.2, MaxTokens: 1024}
	case skillTitle:
		return SkillDefaults{Temperature: 0.3, MaxTokens: 200}
	}
	return SkillDefaults{Temperature: 0.3, MaxTokens: 1024}
}
