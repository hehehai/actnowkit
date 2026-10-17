# Changelog

## v0.3.0 — 2026-10-17

- New package `listsel`: the row selection of the apps' log views, with no
  UI. `Model[K]` holds the rows selected by stable key, the anchor and the
  pointer gesture: a plain click copies its row (`CopyRow`) and sets the
  anchor, Shift-click selects a range, ⌘-click toggles a row, pressing and
  moving more than `DragSlop` selects by dragging (⌘ adds), `SelectAll`,
  `Clear`, `Cancel`, and `Prune` as filters change and lines rotate out.
  `Items` and `Text` format the lines to copy; `TextSelected` … `TextCopied`
  are reference wordings in English and Chinese. Nothing else changed.

## v0.2.2 — 2026-10-17

- `models`: pins: Dicta and AudioSmith share one MOSS Transcribe Diarize
  repo, `vanch007/mlx-MOSS-Transcribe-Diarize-8bit` (Dicta moved to it in
  its 736f427); the `majentik/MOSS-Transcribe-Diarize-MLX-8bit` pin, which
  no app's catalogue has any more, is gone. `PinsOfApp("dicta")` now
  includes the shared repo, so AudioSmith's phase-0 check keeps it for
  Dicta.

## v0.2.1 — 2026-10-17

- macOS: the AppKit lookups (`registry.DefaultSystem.AppPaths`, `Running`,
  `TeamID`, and so `registry.Discover`) lock the OS thread for the life of
  their autorelease pool. Called from a goroutine, the scheduler could move
  the goroutine to another thread between the pool's push and pop, which
  crashed the process ("semasleep on Darwin signal stack"). Callers no
  longer need a workaround of their own.

## v0.2.0 — 2026-10-17

- `models`: a claim names every model of the app that uses the repo
  (`Claim.Models`, a new member: claims files stay `actnow.models.claims/1`,
  and files without it read as before, `model` alone). Claiming merges the
  names (and the file lists); `Store.Release(ref, model)` removes one name,
  and the claim goes with the last; `Store.DeleteModel(ctx, ref, model,
  scope)` removes one model: when other models of the app still use the
  repo, ThisApp keeps the claim and the files (`Result.OwnModels`).
  `Decision.MyModels`. So deleting one of two models that share an
  auxiliary repo no longer drops the other's claim on it.
- `models`: `HeldLock.Downgrade` (exclusive → shared); `family`:
  `Lock.Convert`.
- `hfhub`: `Client.Delete` removes the entry by its id (`DeleteModel`) from
  its main and auxiliary repos.
- `conformance`: the `CatalogApp` interface and the `SharedRepoWithinApp`
  scenario; sample `claims-models.json`.

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
