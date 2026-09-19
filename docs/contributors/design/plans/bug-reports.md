> **ABOUTME:** The diagnostic report a user generates and pastes into a GitHub issue — its two
> mechanisms, its two privacy modes, and the pseudonymisation that lets a media server's logs be
> shared at all.

# Bug reports

- **Status:** PLANNED — designed 2026-09-19 ([D74](../../decisions/working-notes.md)); structure adapted from the [yoloai](https://github.com/kstenerud/yoloai) design, which solved this already.
- **Depends on:** [remote-access.md](remote-access.md) (the session id from [D73](../../decisions/working-notes.md))

## Why this is not boilerplate

A bug report from a media server is a **privacy problem wearing a diagnostics costume**. The
logs are full of film and television titles, file paths containing real names, household
usernames, and — once there is a relay — IP addresses of other people's homes. A library is
intimate in a way an API key is not, and unlike a key it cannot be rotated after disclosure.

And the sensitive thing is usually the *subject*: "this file will not play" is the modal bug.
So a naive report is either useless or a leak.

## Two mechanisms, two types

| | Purpose |
| --- | --- |
| `--bugreport <type>` on any command | **Flight recorder.** Opens the file before the command runs, writes static sections at once, always finalises — success, error or panic |
| `bugreport <type>` subcommand | **Forensic.** For a problem that already happened and left state behind |

| Type | Contents |
| --- | --- |
| `safe` | Pseudonymised and redacted. Carries a *review before sharing* notice |
| `unsafe` | Complete, plus the pseudonym mapping table. Carries a prominent **do not share publicly** banner |

The type is a **required argument with no default**. Sanitisation is best-effort and the report
says so, including the instruction not to file credential-related bugs as `safe`.

Output is one markdown file, mode `0600`, written to `.tmp` and atomically renamed.

## Pseudonymise, do not redact

`[REDACTED]` destroys correlation, which is most of a log's value. So:

- **Per report, a random salt.** Each title, path component and username becomes a short stable
  tag — `item:a7f3`, `user:2b8d`.
- **Within a report, correlation works**: the maintainer sees `item:a7f3` fail in three places.
- **Across reports the tags differ**, so nobody can be tracked between issues.
- **The mapping table exists only in the local `unsafe` copy**, so the user can look up `a7f3`
  and say what it was — privately, and only if asked.

### Keep the shape, drop the identity

This is where nearly all the diagnostic value lives, at nearly no privacy cost:

| Kept | Dropped |
| --- | --- |
| container, codecs, profiles, levels, bit depth | title |
| channel layouts, track counts, dispositions, language tags | path components (depth kept) |
| attachment count and types, duration, size, file extension | usernames, IP addresses |

`hevc / Main10 / L5.1 / 3 audio / 12 subtitle / 1 attachment` diagnoses a playback bug
completely and says nothing about which film it is.

The residual case is a **filename-parsing** bug, where the name *is* the evidence. That is
served by the user sharing one filename deliberately — minimal disclosure, not a library dump.

## It fans in across components

A session touches the core, one or more nodes, possibly a relay, and a client. Each reports
according to its own visibility and all of it lands at the core
([D73](../../decisions/working-notes.md), [principles §9](../../principles/README.md)), so the
report is a fan-in keyed by the **session id**.

- **A component that does not answer is listed as absent with a reason.** *"node `nas`: no
  response, last seen 4 h ago"* is diagnostic; a silently missing section is a trap. Same rule
  as [D40](../../decisions/working-notes.md) and [D59](../../decisions/working-notes.md):
  unreadable is not empty.
- **Summary pastes, bundle attaches.** Four components will not fit GitHub's 65,535-byte issue
  body, so the threshold is **64,000 bytes** for the pasted summary and the rest is an attached
  file — GitHub accepts `.md`, `.txt`, `.log`, `.json` and `.zip` up to 25 MB, which is cleaner
  than a gist. The attachment obeys the same `safe`/`unsafe` rules.
- **One outer `<details>` wrapper**, so a pasted report is a single collapsed line whose
  `<summary>` carries version, timestamp and type — triage without expanding.

## The version inventory, which is the most useful section

We ship our own FFmpeg, libass and FreeType, and several decisions turn on knowing which:

- **FreeType version** — CVE-2025-27363 covers ≤ 2.13.0, and
  [D56](../../decisions/working-notes.md)'s embedded-font risk depends on where the client sits.
- **Which `ASS_Feature`s the build supports** — [D50](../../decisions/working-notes.md) already
  probes `ass_track_set_feature` for a −1 return, so the answer is already computed.
- **Which scripts the client has fonts for** ([D53](../../decisions/working-notes.md)).
- **FFmpeg version *and configure flags***, since the licence ladder and the available encoders
  both hang off them.

[D61](../../decisions/working-notes.md) wants a staleness check over the same inventory. Build
it once; the bug report and the staleness warning are two views of it.

## Legibility is a privacy feature

Plain markdown the user can actually read before posting — no base64, no binary, nothing opaque.
"Review before sharing" is meaningless advice if the thing cannot be reviewed.

## Done when

A user with a playback problem runs one command, gets one file, can read every line of it, and
pastes it into an issue that a maintainer can triage without asking for anything else — and
nothing in it names a film, a person or a house. The `unsafe` variant of the same report answers
the follow-up question.
