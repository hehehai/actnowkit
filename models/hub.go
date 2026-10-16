// Package models is the family model library (spec §7): the Hugging Face
// cache ($HUB) shared by every family app, with claims ("I use
// repo@commit, keep it"), flock locks compatible with huggingface_hub's
// .locks/ directory, the deletion rules that keep one app from deleting
// another's models, the family pins table and model licence acceptances.
//
// It does not download: hfhub (or an app's own downloader) does, holding
// the locks of this package and claiming what it downloaded.
//
// Typical use:
//
//	s := &models.Store{Family: family.Default(), Hub: hub, BundleID: "dev.hehehai.dicta", AppID: "dicta", Apps: presence}
//	s.Claim(ctx, models.Claim{Repo: r, Revision: c, Model: "qwen3-asr-0.6b-4bit", Role: "asr", Origin: models.OriginMigrated})
//	d, _ := s.Decide(models.Ref{Repo: r, Revision: c}) // who else uses it
//	res, err := s.Delete(ctx, ref, models.ThisApp)    // only removes files nobody else claims
package models

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Ref is a model's identity: repo@commit (spec §7.2).
type Ref struct {
	Repo     string `json:"repo"`
	Revision string `json:"revision"`
}

func (r Ref) String() string { return r.Repo + "@" + r.Revision }

// ParseRef parses "owner/name@commit".
func ParseRef(s string) (Ref, error) {
	repo, rev, ok := strings.Cut(s, "@")
	if !ok || !ValidRepo(repo) || rev == "" {
		return Ref{}, fmt.Errorf("model ref %q: want owner/name@commit", s)
	}
	return Ref{Repo: repo, Revision: rev}, nil
}

var (
	repoPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	commitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// ValidRepo reports whether repo is "owner/name" with the characters the
// Hub allows, so that paths built from it stay inside $HUB (spec §13).
func ValidRepo(repo string) bool {
	owner, name, ok := strings.Cut(repo, "/")
	return ok && len(repo) <= 200 && repoPart.MatchString(owner) && repoPart.MatchString(name) &&
		!strings.Contains(repo, "..") && !strings.Contains(name, "/")
}

// IsCommit reports a 40-hex commit id.
func IsCommit(rev string) bool { return commitRE.MatchString(rev) }

// Sources of the hub directory (ResolveHub).
type HubSource string

const (
	FromFamily HubSource = "family"       // settings.json models.root
	FromApp    HubSource = "app"          // the app's own folder, when it does not follow the family
	FromEnvHub HubSource = "HF_HUB_CACHE" // environment
	FromEnvHome HubSource = "HF_HOME"     // environment: $HF_HOME/hub
	FromDefault HubSource = "default"     // ~/.cache/huggingface/hub
)

// ResolveHub finds $HUB (spec §7.1), the first that is set of:
//
//  1. familyRoot, settings.json models.root, when the app follows the family;
//  2. own, the app's private folder, when it does not;
//  3. $HF_HUB_CACHE;
//  4. $HF_HOME/hub;
//  5. ~/.cache/huggingface/hub.
//
// Steps 3–5 are huggingface_hub's own rules, so existing caches stay put.
func ResolveHub(follow bool, familyRoot, own string) (string, HubSource) {
	if follow && familyRoot != "" {
		return filepath.Clean(familyRoot), FromFamily
	}
	if !follow && own != "" {
		return filepath.Clean(own), FromApp
	}
	return EnvHub()
}

// EnvHub is $HUB from the environment alone (steps 3–5 of ResolveHub).
func EnvHub() (string, HubSource) {
	if d := os.Getenv("HF_HUB_CACHE"); d != "" {
		return d, FromEnvHub
	}
	if d := os.Getenv("HF_HOME"); d != "" {
		return filepath.Join(d, "hub"), FromEnvHome
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "huggingface", "hub"), FromDefault
}

// RepoFolder is a repo's directory name: "models--owner--name".
func RepoFolder(repo string) string { return "models--" + strings.ReplaceAll(repo, "/", "--") }

// RepoDir is $HUB/models--owner--name.
func RepoDir(hub, repo string) string { return filepath.Join(hub, RepoFolder(repo)) }

// SnapshotDir is $HUB/models--owner--name/snapshots/<commit>, the path an
// engine is given (spec §7.1: never let it pick a snapshot itself).
func SnapshotDir(hub string, r Ref) string {
	return filepath.Join(RepoDir(hub, r.Repo), "snapshots", r.Revision)
}

// LocksDir is $HUB/.locks/models--owner--name, huggingface_hub's lock
// directory for the repo (spec §7.4).
func LocksDir(hub, repo string) string { return filepath.Join(hub, ".locks", RepoFolder(repo)) }

// RepoLockName is the repo lock's file name.
const RepoLockName = "actnow-repo.lock"

// RepoLockFile is $HUB/.locks/models--owner--name/actnow-repo.lock.
func RepoLockFile(hub, repo string) string { return filepath.Join(LocksDir(hub, repo), RepoLockName) }

// BlobLockFile is $HUB/.locks/models--owner--name/<blob>.lock, the same
// file huggingface_hub locks while it downloads that blob.
func BlobLockFile(hub, repo, blob string) string {
	return filepath.Join(LocksDir(hub, repo), blob+".lock")
}

// EngineCopiesDir is $HUB/mlx-audio, mlx-audio-swift's flat copies.
func EngineCopiesDir(hub string) string { return filepath.Join(hub, "mlx-audio") }

// EngineCopy is $HUB/mlx-audio/<owner>_<name> (spec §7.8).
func EngineCopy(hub, repo string) string {
	return filepath.Join(EngineCopiesDir(hub), strings.ReplaceAll(repo, "/", "_"))
}

// SameHub reports whether two hub paths are the same directory (as
// written in claims: cleaned, symlinks resolved when they exist).
func SameHub(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ca, cb := canonical(a), canonical(b)
	return ca == cb
}

func canonical(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
