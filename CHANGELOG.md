# Changelog

## v0.16.0 — 2026-11-06

The apps' copies of the language-model layer, merged: a new package only,
so code written for v0.15 builds and behaves as before.

- New package `llm` (CaptionCraft's, Dicta's and Paralign's
  `internal/llm`, best of the three): `Request`/`Response`/`Usage`,
  `Backend`; `Client` (`Chat`: retries with full-jitter backoff,
  per-attempt timeout, `<think>` stripping and filtered streaming,
  estimated usage; `JSON`: extraction, `Schema` validation, decoding and
  repair turns); `Schema` (`Object`, `Array`, `String`, `Number`,
  `Integer`, `Boolean`, `Enum`, `Range`, `Count`, `Describe`, `Nullable`,
  `Closed`, `Validate`); `ExtractJSON`, and CaptionCraft's `closeJSON` and
  `replyJSON` as `CloseJSON` and `ReplyJSON`; `Meter` and
  `EstimateTokens`; `StripThinking`; `*Error` with its codes, `Code`,
  `Param`; `EngineBackend` with the engine's `llm.generate` wire types
  (`GenerateParams`, `GenerateResult`, `GenerateMessage`,
  `TokenNotification`, the same in every app's protocol package).
  What was each app's is injected: the skills and their defaults
  (`Skill`, `SkillDefaults`, `Client.Defaults`; the apps keep their own
  skill tables), the error values an `*Error` unwraps to and the engine
  call's errors are read by (`SetSentinels`), the protocol's error type
  (`EngineError`, an interface the app's `proto.Error` implements).
- Fixes merged from the copies:
  - Dicta: a call given up (no time left before the deadline, the wait
    before a retry cancelled, ctx ended between attempts) is metered
    too; `ExtractJSON` takes the longest value that parses outside any
    other (not a "[1]" citing before the answer) and stops at a value
    left open (what parses inside a reply cut off is a fragment);
    `Client.JSON` treats a reply cut off inside a value as cut off (a
    repair turn, or `CodeInvalidJSON`), not its parsed fragment.
    (Refined here: a reply cut off after its whole value is used.)
  - Paralign: a repair turn samples from the request as asked (at least
    `RepairTemperature`, its own topP, the turn as seed) and a request
    that samples already keeps its settings.
  - CaptionCraft: `CloseJSON` drops a list item cut off rather than
    closing it half written (a suggestion without its "right" deleted
    words), and `ReplyJSON` closes a reply cut off before searching it.
    One string scanner now serves `ExtractJSON`, `CloseJSON` and the
    cut-off test (it was written four times).
- Dropped as unused in every app: `ModelInfo`, `Schema.Name`,
  `EngineBackend.MapError` (replaced by `EngineError` and the
  sentinels), `Error.RetryAfter` and `RetryPolicy.MaxRetryAfter` (no
  backend sets it), `Meter.OnRecord`, `Total` and `Reset`; the skill
  constants and `Defaults` (each app's).
- Drift between the copies: Dicta lacks the sampled repair turns; Dicta
  and Paralign lack `SkillHighlights` (Dicta also the proofread, terms and
  chapters skills), which stay the apps'. AudioSmith and Auralink carry
  an older, larger variant with cloud providers (OpenAI-compatible,
  Anthropic, a router, presets, Retry-After, `WithBudget` queueing for
  the engine's gpu lane), other error codes (`llm.server` for the
  engine's `llm.engine`, `llm.network`…) and no `Seconds`/
  `TokensPerSecond`; they are not merged here. Inboxly's `llm` is a
  different design (Apple Foundation Models and MLX behind a preempting
  runner) and stays Inboxly's.

## v0.15.0 — 2026-11-06

The apps' copies of the language table and of the translation checks,
merged: new packages only, so code written for v0.14 builds and behaves as
before. actnowkit now requires `golang.org/x/text` (v0.43.0; `lang`'s
search folding and tag parsing).

- New package `lang`: the language table (CaptionCraft's, the superset of
  the copies: 652 languages, `languages.tsv` + `aliases.tsv` embedded,
  `gen/main.go` to regenerate them) — `Language` (codes, names, script,
  RTL, region, `Flag`, `NameIn`, macrolanguage, OmniVoice/Whisper),
  `Canonical`, `Lookup`, `All`, `Len`, `Fold`, `Search`, `OmniVoiceCodes`,
  `WhisperCodes`, `Match`, `Annotate`, `ForModel`, `Option`; and `Tag`
  (the apps' `translate.Lang`): `Parse`, `Variant`, `Describe`,
  `Unspaced`.
  Drift between the copies: Inboxly, Paralign and Dicta carry subsets
  (no `Flag`, `All`, `Search`, `Match`…; Dicta's table is unsorted, which
  nothing of its showed); AudioSmith and Auralink build the search index
  at load rather than on the first `Search`; Act ships a 4-column table
  (code, ISO 639-3, English and Chinese names). The data is the same in
  all (Act's columns included). Inboxly's `Detect` and code-block rules
  stay in Inboxly.
- New package `script`: `IsUnspaced`, `IsWordRune` (CaptionCraft's
  `text`), `Dominant`, `Has`, `Of` and the script names (the checks'
  `dominantScript`, `hasScript`, `targetScript`).
- New package `hanzi`: the Hant-Hans and Hans-Hant tables of the apps'
  `translate/hanzi*.go` — `Simplified`, `Traditional` (new),
  `TraditionalOnly`, `SimplifiedOnly`, `HasTraditionalOnly`,
  `HasSimplifiedOnly`, `ToSimplified`, `ToTraditional`.
- New package `transcheck`: the checks of the apps' `translate/checks.go`
  and `hanzi.go`, exported — `Untranslated`, `WrongScript`, `Wordless`,
  `CutShort`, `TooLong`, `ShortenedTooMuch`, `Repeats` (`OverlapShare`),
  `MeaningUnits`, `Sentences`, `SplitSentences`, `ChineseScript`,
  `OtherHan`, `ToScript`, `AsItIs`, and Paralign's `Checked` (a system
  engine's translation). Drift: Dicta lacks `wordless` and
  `shortenedTooMuch`; Paralign and Dicta tokenise `Repeats`' trigrams with
  a narrower unspaced-letter rule (Han, kana, Thai) than CaptionCraft's
  `text.IsUnspaced && text.IsWordRune` (also Lao, Khmer, Myanmar, Tibetan,
  Bopomofo, full-width letters and digits, and combining marks), which
  `transcheck` keeps.

## v0.14.0 — 2026-11-06

The apps' copies of their UI translation core and third-party licence
generator, merged: nothing else changed, so code written for v0.13 builds
and behaves as before.

- New package `i18n`: `Catalog` (`New`, `MustNew` over the app's `fs.FS`
  of `<tag>.json` catalogues), `T`, `N`, `X` (gettext's `context\x04msg`
  keys), `SetLocale`, `Locale`, `Supported`, `DateTime`, `Date`; `Match`
  (Chinese by script: zh-TW → zh-Hant, never offered zh-Hans; other
  languages by their language subtag), `PluralOne` (French and Portuguese
  0 and 1; none in Chinese, Japanese, Korean…), `Name` (autonyms). From
  the apps' copies: an empty translation falls back to English in `X` as
  in `T` (CaptionCraft), and a singular without a number is not formatted
  (`%!(EXTRA int=1)`; Refto #302).
- New package `i18n/i18ntest`: `Parity` scans a module's T/N/X calls
  (nested modules skipped) and `Check`s (`Problems`) the catalogues — every
  key translated, none unused, the same formatting verbs, the source
  language's catalogue empty —, `Sync`s them; `Verbs`; `Style` and
  `ZhHansRules` (Inboxly's Chinese conventions).
- New package `thirdparty`: the `ThirdPartyLicenses.txt` format (`Parse`,
  `Load`, `Component`, `Ships: program`), `Classify`, `Copyleft`,
  `CopyleftIn`, `Check`.
- New command `cmd/actnow-licenses`: the apps' generator, configured by the
  app's `actnow-licenses.json` (app name, header file, patched Go modules,
  `Package.resolved`, script-pinned Swift packages, npm list, extras,
  programs; each optional part may be left out). `-check` also fails when
  the header changed (Inboxly); source files named like a licence
  (`licence_test.go`, `License.swift`) are not copied (Inboxly); Swift
  clones are cached in `actnow-licenses` under the user cache directory,
  shared by the apps.
## v0.13.0 — 2026-10-31

Act is renamed Refto, as a new product (no migration): its row, pins and
default shortcut move to the new id. Code that names the app `"act"`
(`AppByID`, `PinsOfApp`, `shortcuts.DefaultsOf`) must say `"refto"`;
nothing else changed.

- `family`: the table's row `act` (Act, `dev.hehehai.act`, URL scheme
  `act`) becomes `refto` (Refto, `dev.hehehai.refto`, URL scheme `refto`,
  team `TeamActNow`). Its product page stays https://actnow.dev/act until
  the site has one for Refto. `dev.hehehai.act` and its variants are no
  longer family bundle ids (`ErrNotFamily`); `dev.hehehai.refto.engine` is
  a component.
- `shortcuts`: Quick Collect (`Control+Option+Command+S`) is Refto's
  default; `DefaultsOf("act")` is empty.
- `models`: the pins table lists `refto` where it listed `act` (the same
  five pins: EmbeddingGemma 2, Qwen3.5 2B and the three shared with
  Dicta, AudioSmith and VoxtNew).
- Claims an old Act build wrote (`models/claims/dev.hehehai.act.json`)
  still protect its models like any app's: while Act is installed (its
  manifest names it "Act") or unknown to the registry, they count as a
  user; once it is uninstalled they are orphans, cleaned up only when the
  user asks, and never taking a model an installed app (Refto) claims.
  Apps without claims that relied on `PinsOfApp("act")` to keep an
  installed Act's models no longer do; Act always wrote claims.

## v0.12.1 — 2026-10-26

What a model download takes from an endpoint is checked against the
catalogue (Act's review #108 HUB-M1): a racing mirror can no longer change
a pinned model's files.

- hfhub: an entry pinned to a commit takes only an endpoint that answers
  that commit; one that answers another (out of date, or serving its own)
  fails and the others are asked — no snapshot of the other commit, no
  refs/main moved to it.
- hfhub: a file of `Entry.Files` with a `SHA256` is checked against it
  whether it is in LFS or in git (configs, chat templates, tokenizers,
  vocabularies), not only against the blob id the endpoint lists; a blob
  of such a file already in the cache that does not match is fetched
  again. Catalogues can now pin every file (before, a pin on a git file
  failed as a mismatch with the endpoint's empty LFS hash).

## v0.12.0 — 2026-10-21

The activity cards of the family's window layout (as Act's): nothing else
changed, so code written for v0.11 builds and behaves as before.

- New package `activity`: `Tracker` (`Ended`, `Cards`) groups an app's
  unfinished jobs by kind into cards — a single job with its title and
  fraction (or waiting), several with the kind's title and done/total,
  counting the jobs of the kind that ended while others ran —, and keeps a
  kind's finished card for `DoneFor` (3 s) when some of its jobs succeeded;
  `Showing`; the stack's geometry (`CardHeight` 44, `CardGap` 6,
  `CardPeek` 5, two cards behind, `Show` 4, `Height`, `Poses`). No UI:
  each app draws the cards, in its words and look. Taken from Act's and
  CaptionCraft's, Dicta's cards.

## v0.11.0 — 2026-10-20

Inboxly joins the family, with the capability and pins it uses; nothing
else changed, so code written for v0.10 builds and behaves as before (new
rows and constants only).

- `family`: Inboxly joins the table (`inboxly`, `dev.hehehai.inboxly`, URL
  scheme `inboxly`, https://actnow.dev/inboxly, team `TeamActNow`). Its
  engine (`dev.hehehai.inboxly.engine`) is a component, not an app.
- `shortcuts`: Inboxly's default, Quick Compose on
  `Control+Option+Command+N` (Carbon, enabled), free in the family table
  (Pik's picker is ⌃⌥⌘P, HereNow's panel ⌃⌥⌘H, Act's Quick Collect ⌃⌥⌘S,
  Dicta's reserved meeting key ⌃⌥⌘R).
- `registry`: new capability `CapMail` (`"mail"`: a local mail client that
  opens `mailto:` links, `<scheme>://compose[?to=…&subject=…&body=…]` and
  `<scheme>://thread?id=<message id>`).
- `models`: the pins table lists Inboxly on the three pins its catalogue
  shares (EmbeddingGemma 2 and Qwen3.5 2B with Act, Qwen3 4B Instruct 2507
  with AudioSmith and Dicta), so the other apps do not clean them up while
  Inboxly may use them.

## v0.10.0 — 2026-10-20

Licences and in-app updates for the paid apps (CaptionCraft first). The
protocol is the site's docs/licensing.md.

- New package `licence`: `New(Config)` → `Client` with `Status` (offline,
  from the stored token), `Activate` (a code typed loosely; `DeviceLimit`
  with the bound Macs), `Refresh`, `Check`, `Run` (checks every 6 h,
  refreshes at 7 days, retries after 1/5/30 min and 2 h, `wake`),
  `ServerStatus`, `Forget`, `ManageURL`, `EarlyAccessURL`,
  `ActivationHelpURL`. Tokens `anl1.<kid>.<payload>.<sig>` are verified with
  Ed25519 (`ParseToken`, `ProductionKeys`: kid `lk-2026-10`) and must name
  this app, this Mac (`DeviceID`: IOPlatformUUID + the app's salt, SHA-256,
  through IOKit with purego) and the request's nonce. States `Inactive`,
  `Valid`, `NeedsRefresh` (due / updated / clock), `Grace` (30 days
  offline), `Expired` (code ended / offline too long),
  `WhitelistNotApplicable` (early access at or above `paid_since`),
  `DeviceLimit`, `Revoked` (code revoked / Mac removed). The clock check
  keeps the latest time seen and the server offset (`Record`). Stores:
  `NewKeychainStore(bundleID)` (the app's own `<bundle id>.licence` item,
  through credentials' Keychain), `MemoryStore`. `VerifySelf` /
  `VerifyBundle` check a code signature against `TeamRequirement`
  (`TeamActNow`, 779M533DL5) with the Security framework. `ThisDevice` (the
  computer name, model, macOS version). `CompareVersions`,
  `WhitelistBlocked`. Texts (`StateText`, `ErrorText`, `Text…`) in English
  and Chinese.
- New package `updates`: a `Manager` over MyGo's updater API (passed as
  `Funcs`; actnowkit still does not import MyGo): `Check` → `Available`
  with the notes in the app's language from actnow.dev's changelog API
  (`HistoryClient`, `NotesLocale`; English where a release has none, the
  manifest's notes when the API is unreachable), `Install` with progress
  (`Downloading` → `Ready`; `Failed` leaves the app unchanged), `Restart`,
  `Dismiss`, `History` (Settings → About), `AutoCheck`, `Subscribe`.
  Early-access users get `NeedsPaidLicence` for updates at or above
  `paid_since` (`Install` needs `acceptPaid`; `TextNeedsPaid`).
- `internal/darwin`: `PlatformUUID`, `ComputerName`, `CheckSelfRequirement`,
  `CheckBundleRequirement`.

## v0.9.1 — 2026-10-20

- `sounds`: for the pickers' second look: `Player.Previewing` (the choice
  the last preview plays and how long it has left; `StopPreview` ends it),
  `UnknownLength`, `Player.SetClock`; `VolumeSteps` (0, 25, 50, 75, 100),
  `NearestVolumeStep`, `VolumeLevel` (muted, low, high: the volume
  button's icon); `Catalog.GroupName`.

## v0.9.0 — 2026-10-20

- New package `sounds`: the family's cue sounds. `Load` reads an app's
  `catalog.json` (schema `actnow.sounds.catalog/1`: groups, tags, uses,
  sounds with names in English and Chinese, durations, pairs, the system
  sounds each replaces, SHA-256) from the app's `fs.FS`; actnowkit ships no
  audio. `Event`, `Prefs` (on, volume, a choice per event: a sound, `none`
  or `system:<name>`), `Target`, `Resolve` with a system-sound fallback for
  builds without the catalogue, `MigrateSystem` / `MigrateClosest` for old
  settings; `Filter`, `GroupCounts`, `TagCounts`, `Suggested` for pickers;
  `Player` (`Preload`, `PlayEvent`, `Preview` cutting off the last one,
  `StopPreview`). macOS output: WAV decoded into an AVAudioEngine with 8
  player nodes (starts in microseconds, repeats overlap, the engine stops
  after 5 s idle), AVAudioPlayer for other files, NSSound for system
  sounds; `Silent` elsewhere; `Recorder` for tests. `DecodeWAV`, `ToneWAV`.
- New command `cmd/actnow-sounds`: `list`, `check`, `subset` (the sounds an
  app ships, with their catalogue and a size limit), `play` (macOS).

## v0.8.0 — 2026-10-19

The family audit's first batch (spec v1.0.9). **The model library moves**:
apps that update must call `Store.Import` before showing model states.

- `models`: **the default `$HUB` is `$FAMILY/models/hub`**
  (`~/Library/Application Support/ActNow/models/hub`, `DefaultHub`,
  `DefaultHubIn`), still in the Hugging Face layout. `ResolveHub` no longer
  reads `HF_HUB_CACHE` / `HF_HOME`: an app started from Finder and one
  started from a shell chose different folders. The Hugging Face caches are
  now import sources (`LegacyHubs`; `EnvHub` keeps its meaning,
  `HFDefaultHub` is `~/.cache/huggingface/hub`, `FromHFDefault` its source).
- `models`: `Store.Import(ctx, from, backup, want...)` brings an app's
  catalogue models (and the claims of its previous library) over from those
  caches: hard links on the same volume, a copy across volumes when it
  leaves `ImportReserve` free; holding the destination repo's lock
  exclusively and the source's shared; refs naming the commit copied;
  the app's previous claims claimed again (`OriginImported`; the
  catalogue models are left to the app's own migration or reuse card).
  Nothing in the source is changed, models no
  family app wants are not touched. Each app records what it handled in
  `$FAMILY/models/imported/<bundle id>.json`, so a model deleted after the
  import does not come back. Deleting a hard-linked blob reports nothing
  freed (`Decision.FreeBytes`, `Result.Freed`): the cache still holds it.
- `models`: Time Machine. `SetBackupExcluded` / `IsBackupExcluded` set the
  sticky attribute `com.apple.metadata:com_apple_backup_excludeItem` (the
  bytes `tmutil addexclusion` writes; pure Go through purego).
  `ApplyBackup(fam, hub, backup)` marks the whole default library, and in a
  folder the user chose only the repos family apps claim; `MarkRepoBackup`
  for one repo. hfhub marks the repos it finishes. `EnsureHub` creates the
  library with `$FAMILY`'s 0700 parents.
- `settings`: `models.backup` (`KeyModelsBackup`, `"on"`/`"off"`, default
  off = excluded; `ModelsBackup(s)`), family-wide without a follow flag.
- `models`: the "installed" check compares sizes. Downloaders record each
  blob's verified size in `models--<owner>--<name>/.actnow/blobs.json`
  (`RecordBlobSizes`, `ReadBlobSizes`; hfhub and `Import` do), and
  `Claim.Sizes` carries catalogue sizes (hfhub fills it from `Entry.Files`):
  a file of another size is listed in `Status.Damaged` / `Local.Damaged`
  and the model is `Partial`. A download repairs it.
- `models`: lost claims are "unsure". `AppState.ModelsRoot` (from the
  manifest, `registry.States`) names the `$HUB` an app keeps models in; an
  installed app with this `$HUB` and no claims file makes `Decide` unsure
  (`Decision.Unclaimed`), so nothing it may use is deleted.
  `Store.Heal(ctx, installed...)` recreates the app's claims file when it is
  missing (claiming the installed models, or writing it empty): call it at
  launch and on activation.
- `redact` (new): `URL`, `Text`, `Error` take API keys and tokens out of URL
  queries (`key=`, `token=`, `X-Amz-Signature=`…), `user:pass@` out of proxy
  URLs, `Authorization` headers, bearer tokens and well-known key shapes
  (`AIza…`, `sk-…`, `hf_…`, `ghp_…`) out of logs, error details and
  diagnostics.
- `runtime`: files carry the writer's executable (`File.Exe`); `Live` (and
  so `ReadAll`, `Others`, `LoadedBy`) ignores a file whose pid now runs
  another executable, or that was written before the system started, so a
  crashed app's models stop counting when its pid is reused.
- `models` texts: `TextBackupModels`, `TextBackupModelsHint`, `TextImported`,
  `TextFolderDefault`, `TextFolderChosen`, `TextDamaged`, `TextUnclaimed`.
- `conformance`: the fixture's `$HUB` is `$FAMILY/models/hub`
  (`Fixture.Legacy` is a Hugging Face cache beside it, `$HF_HUB_CACHE`);
  registered peers write their claims file; new scenario
  `DeleteWithLostClaims`. `Check` skips the library inside `$FAMILY`.
- `hfhub`: `CacheDir` is `models.DefaultHub`; a waiter whose blob holder
  left no owner file tries the lock again before reporting "unknown" (a
  flaky test on macOS).

## v0.7.2 — 2026-10-19

A pins table update; nothing else changed.

- `models`: the Qwen3-ForcedAligner 0.6B 4-bit pin lists Act, which now
  uses it (optional subtitle timing, M5) at the same commit, so the other
  apps do not clean it up while Act may use it.

## v0.7.1 — 2026-10-19

A fix to the default shortcut table; nothing else changed.

- `shortcuts`: Act's Quick Collect moves to `Control+Option+Command+S`
  ("save"). v0.7.0 gave it ⌃⌥⌘A, which Pik already uses for Pick Area
  (off by default, and not in the table). Pik's Pick Area now has its
  row (`pik`/`pickArea`, ⌃⌥⌘A, off), so the disjointness test guards it.

## v0.7.0 — 2026-10-19

Act joins the family, with the capabilities, model roles and pins it
uses; nothing else changed, so code written for v0.6 builds and behaves as
before (new rows and constants only).

- `family`: Act joins the table (`act`, `dev.hehehai.act`, URL scheme
  `act`, https://actnow.dev/act, team `TeamActNow`). Its engine
  (`dev.hehehai.act.engine`) is a component, not an app.
- `shortcuts`: Act's default, Quick Collect on `Control+Option+Command+A`
  (Carbon, enabled), free in the family table (Pik's picker is ⌃⌥⌘P,
  HereNow's panel ⌃⌥⌘H, Dicta's reserved meeting key ⌃⌥⌘R). Its
  screenshot shortcuts have no default keys.
- `registry`: new capabilities `CapLibrary` (`"library"`: a library that
  takes files other apps hand over, `<scheme>://import?path=<file>…` and
  `<scheme>://new?url=<link>`), `CapEmbedLocal` (`"embed.local"`) and
  `CapVLMLocal` (`"vlm.local"`), the strings Act already declares.
- `models`: roles `RoleEmbed` (`"embed"`) and `RoleVLM` (`"vlm"`); the
  pins table gains EmbeddingGemma 2 (`mlx-community/embeddinggemma-2-8bit`
  at `7505ef2`, role `embed`) and lists Act on the three pins its
  catalogue shares (Qwen3-ASR 0.6B, Qwen3.5 2B, Gemma 4 E4B), so the
  other apps do not clean them up while Act may use them.

## v0.6.0 — 2026-10-18

One app and one capability join the family; nothing else changed, so code
written for v0.5 builds and behaves as before (`family.Apps` gains a row,
`registry` a constant).

- `family`: Paralign joins the table (`paralign`, `dev.hehehai.paralign`,
  URL scheme `paralign`, https://actnow.dev/paralign, team `TeamActNow`).
  Its engine (`dev.hehehai.paralign.engine`) is a component, not an app.
  It has no global shortcut, so `shortcuts.Defaults` is unchanged.
- `registry`: new capability `CapLearning` (`"learning"`): the app does
  local language learning from media and opens
  `<scheme>://import?path=<file>[&subtitles=<file>]` for a video or audio
  file another app hands over, optionally with the subtitles to study
  (CaptionCraft's and Dicta's "Study in Paralign").

## v0.5.1 — 2026-10-18

- `mirror`: the ActNow mirror's files are now served by R2 itself
  (Worker responses reach mainland China at a few KB/s, R2's at MB/s), and
  R2 cannot redirect: `Client.Latest` also reads a `200` answer with JSON
  `{"tag": "<tag>"}` (GitHub still redirects). `Client.File` gives a
  source it left for stalling or crawling one more try on a new
  connection (some of a CDN's addresses crawl from China, others do not);
  the last attempt is never left for being slow.

## v0.5.0 — 2026-10-18

Downloads from GitHub for users in China; nothing else changed.

- New package `mirror`: files of GitHub releases from github.com or the
  family's mirror, `https://mirror.actnow.dev/github` (spec §9.6), which
  answers GitHub's paths. `Sources` maps the family's download source
  (`network.hfEndpoints`: automatic → both, `official` → github.com,
  `mirror` → the mirror; `$ACTNOW_GITHUB_SOURCES` replaces them).
  `Client.File` races the sources for each file (the first to deliver
  `Lead` bytes wins), leaves a source that stalls, crawls, fails or serves
  a file with another SHA-256 for the next, and returns `ErrChecksum`
  (`*ChecksumError`) when only wrong files came. `Client.Latest` reads
  the latest release's tag from the `/releases/latest` redirect of the
  first source to answer; `Try` runs a check that needs several files of
  one source (a checksum file and its signature) at every source and keeps
  the first that passes; `Bytes` and `Small` fetch small files. Reference
  wordings `TextSourceAuto`, `TextSourceOfficial`, `TextSourceMirror`,
  `TextSourceNote`.

## v0.4.0 — 2026-10-18

Two apps join the family; nothing else changed, so code written for v0.3
builds and behaves as before (`family.Apps` and `shortcuts.Defaults` gain
rows).

- `family`: HereNow joins the table (`herenow`, `dev.hehehai.herenow`,
  URL scheme `herenow`, https://actnow.dev/herenow, team `TeamActNow`).
- `shortcuts`: HereNow's default, `openPanel` "Open HereNow" on
  ⌃⌥⌘H (`Control+Option+Command+H`, press, Carbon, enabled, not
  exclusive); it collides with no other default.
- `family`: CaptionCraft joins the table (`captioncraft`,
  `dev.hehehai.captioncraft`, URL scheme `captioncraft`,
  https://actnow.dev/captioncraft, team `TeamActNow`). It has no global
  shortcut, so `shortcuts.Defaults` is unchanged.
- `registry`: new capability `CapSubtitles` (`"subtitles"`): the app makes
  video subtitles and opens `<scheme>://import?path=<file>` for a file
  another app hands over (Dicta's "Make subtitles in CaptionCraft").

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
