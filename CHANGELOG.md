# Changelog

## v0.1.0 — 2026-10-17

First release: the first phase of the ActNow family protocol (spec v1.0.2).

- `family`: the family directory, JSON files with schema versions and kept
  unknown members, atomic writes, corrupt files set aside, `flock` locks,
  Darwin notifications (purego; in-process elsewhere), the family app table
  (AudioSmith, Dicta, Auralink, VoxtNew, Pik) and bundle-id variants.
- `registry`: manifests, discovery through manifests, LaunchServices and
  `NSRunningApplication`, Info.plist reading (XML and binary), the signing
  Team ID.
- `models`: `$HUB` resolution, claims, repo and blob locks with owner files,
  the installed check, delete decisions and deletion, "delete for all apps"
  tombstones, orphan clean-up, usage, reuse survey, `mlx-audio` clean-up,
  licence acceptances, the family pins table.
- `hfhub` (experimental): the Hugging Face downloader from Dicta's
  `internal/hub`, on the family locks and claims.
- `settings`: `settings.json`, follow flags, first-run migration.
- `credentials`: the vendor table, the `security` Keychain wrapper, the
  consent flow, legacy item migration.
- `shortcuts`: canonical keys, conflicts, prompts, family defaults.
- `runtime`: runtime files, the family memory budget, the pasting flag.
- `conformance`: sample files and cases, fixture, fake Hugging Face
  endpoint, scenario suite, directory check; `cmd/actnow-conformance`.
