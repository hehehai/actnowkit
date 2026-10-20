package licence

import (
	"errors"
	"fmt"
)

// TeamActNow is the Developer ID team every licensed ActNow app is signed
// with (wei guan, 779M533DL5).
const TeamActNow = "779M533DL5"

// Errors of VerifySelf.
var (
	// ErrUnsigned: no signature, or an ad hoc one (development builds).
	ErrUnsigned = errors.New("licence: the app is not signed with a Developer ID")
	// ErrSelfCheckUnsupported: not macOS.
	ErrSelfCheckUnsupported = errors.New("licence: code signatures are checked on macOS only")
)

// TeamRequirement is the code-signing requirement of a Developer ID team.
func TeamRequirement(team string) string {
	return fmt.Sprintf(`anchor apple generic and certificate leaf[subject.OU] = "%s"`, team)
}

// VerifySelf checks the running app's own code signature: valid (nothing
// changed since signing) and from team (TeamActNow). A copy that was
// patched and signed again by someone else fails (docs/licensing.md §14).
// Development builds are unsigned (ErrUnsigned): call it in release builds
// only, and treat any error as "not licensed".
func VerifySelf(team string) error { return checkSelf(TeamRequirement(team)) }

// VerifyBundle is VerifySelf for an app bundle on disk (all its files):
// for checking an update before it replaces the running app.
func VerifyBundle(path, team string) error { return checkBundle(path, TeamRequirement(team)) }
