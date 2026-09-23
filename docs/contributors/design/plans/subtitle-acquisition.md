> **ABOUTME:** Finding and fetching subtitles a file does not have, and — the part that decides
> whether the feature is worth having — proving the one it found is the right one before
> anybody sees it.

# Subtitle acquisition

- **Status:** PLANNED — gap identified 2026-09-21 ([D140](../../decisions/working-notes.md)), specified the same day by [D147](../../decisions/working-notes.md) and [D148](../../decisions/working-notes.md) on the measurements in [subtitle-sync.md](../research/subtitle-sync.md). Provider constraints researched in [tech-licensing.md](../research/tech-licensing.md) and [tech-metadata.md](../research/tech-metadata.md). **Cut A has no verification** ([D178](../../decisions/working-notes.md) §4), so Cut A's form is acquisition on request with an unverified candidate list and the manual offset control (below); OpenSubtitles is a Go package behind the provider interface, not a WASM plugin ([D193](../../decisions/working-notes.md) §3).
- **Depends on:** [schema.md](schema.md) (per-user provider credentials, in the `setting` table), [identification.md](identification.md) (what the file *is*), [library-model.md](library-model.md) (where a fetched track lives, and the speech map), [scanner.md](scanner.md) (the pass that produces it)

## Why this exists

[D11](../../decisions/working-notes.md) makes subtitles the project's headline claim — *"the
single largest defect area in the field"*, *"the cheapest place to be visibly better"* — and
everything built on it handles **subtitles that exist**: extraction at scan, font subsetting,
libass correctness, the overlay rendition, the statutory control set.

**A file with no subtitle track is an ordinary situation**, and it is the reason **Bazarr**
exists as a separate program. The requests are on the trackers: *On-Demand Subtitle Search*
(+13) and *Downloadable Subtitles Like Plex in Jellyfin TV App* (+12) on `jellyfin-androidtv`.

But the interesting half is not the fetching. It is that **the fetch as the field implements it
produces a coin flip**, and shipping that would make D11's claim worse, not better. The project
owner's description of it is the specification for what not to build:

> *"You have no idea which ones work, try them one-at-a-time until one of them actually
> successfully downloads (about half of them fail to download), then it's either out of sync
> right off the bat, or drifts over time, or maybe — just maybe — it actually works."*

## In Cut A: on request, labelled unverified, corrected by hand

[D178](../../decisions/working-notes.md) puts **acquisition on request** in Cut A — per title, by
a person, which is also all OpenSubtitles' 20 downloads a day permits — and puts **subtitle
verification out**, because the speech map is a 31-hour pass over the library. Everything below
that depends on a score therefore waits, and Cut A uses the plan's own fallback:

- **A person asks**, from the title, for subtitles in a language. Nothing is fetched otherwise.
- **The candidates are listed, each labelled *unverified*.** The list is ordered by what the
  provider can tell us without measuring: a `moviehash` match (§3a) first, then the
  identification match. The label is on every row, because without a score nothing may look
  verified ([D147](../../decisions/working-notes.md)'s rule still holds in its negative form).
- **The person picks one**, it is written where §7 says, and it plays.
- **[D90](../../decisions/working-notes.md)'s offset control is the correction**, and a nudge the
  person keeps watching through is persisted onto the track (§3).
- **The budget is shown before the fetch** and exhaustion is reported as a quota, exactly as §4.
- **The provider is a Go package** behind the provider interface
  ([D193](../../decisions/working-notes.md) §3), and each user's OpenSubtitles credentials live
  in the database's `setting` table ([schema.md](schema.md)).

The automatic path of §2–§3 and the library-wide check of §5 arrive with the speech map, and
then this list becomes the *"nothing cleared the threshold"* case rather than the ordinary one.

## The shape

**One rule** ([D147](../../decisions/working-notes.md)): *a candidate is scored by measurement
against the audio, after correction, and a candidate that cannot be verified is never presented
as though it were.*

Everything else follows from it.

### 1. The reference comes first, and it is usually free

[D148](../../decisions/working-notes.md)'s hierarchy, in order of preference (separation
measured over 65 real subtitle files on two films):

| Reference | Separation | Cost |
| --- | --- | --- |
| an existing correct subtitle track in the file | **0.65** | none |
| the **centre channel** of the audio | 0.31–0.39 | one audio-only decode, ~32 s for a 2-hour film |
| the stereo downmix | 0.22–0.23 | same |

The audio path produces a **speech map** — a 10 ms bitmap, run-length encoded to **1–3 kB per
film**, stored beside the other scan artefacts as a tenth pass in
[D102](../../decisions/working-notes.md)'s pipeline. It is computed once and answers every
later question in **3–8 ms**.

The detector must be a real VAD. An energy threshold measures the orchestra, not the dialogue
— proven against a dialogue-free master, where it reported the same speech fraction as the
full mix — and `webrtcvad`, the field's default, is barely better on scored content.

### 2. Candidates are scored, not listed

- Ask every configured provider, subject to §4's budget.
- For each candidate, search **offset in ±60 s × the discrete framerate ratio set**
  (`24/23.976`, `25/23.976`, `25/24`, and inverses) and take the **normalised correlation**
  at the best correction. 84,007 evaluations, 3–8 ms.
- **Correction is lossless**: measured, a candidate desynced by −10.9 s *and* a 0.959 framerate
  ratio corrects to the same worst-cue accuracy as one that was never corrupted. So desync
  tells you nothing about candidate quality and must not enter the ranking; the score after
  correction is the whole judgement.
- **Accept above the threshold, correct, write, done.** Over 128 candidate evaluations with
  half the correct candidates deliberately desynced, a fixed threshold of `r ≥ 0.25` accepted
  64/64 right-film candidates and 0/64 wrong-film candidates.

### 3. What the user sees

- **In the ordinary case, nothing.** It is a background job; the subtitle is simply there. The
  list of guesses does not exist, because picking was never a human judgement. *(Not in Cut A,
  which has no speech map; see above.)*
- **When nothing clears the threshold**, say so in those words — *"no subtitle for this could be
  verified against the audio"* — and offer the best unverified candidate as an explicit,
  labelled choice. Unverified must never look like verified.
- **The measured offset is shown.** [D90](../../decisions/working-notes.md)'s manual offset
  control stays and is the residual correction.
- **A user nudge is ground truth.** An offset a user set and then kept watching through is
  persisted onto the track and outranks our score.

### 3a. What identifies the file to the provider — and it is free

The best possible candidate is one whose uploader had **the identical file**, which is what
`hash` being worth more than every other term combined in subliminal's score means. Getting
that match needs the provider's own hash, and this was recorded as an open question — *"almost
certainly not the hash we have, and whether it costs a second read of the file is a real
question for the node pipeline"*.

**It costs nothing.** Read from `subliminal/utils.py`:

| Provider hash | Definition |
| --- | --- |
| **OpenSubtitles** `moviehash` | file size **plus the sum of little-endian `int64`s over the first 64 KiB and the last 64 KiB**, truncated to 64 bits |
| **TheSubDB** | MD5 of the first 64 KiB concatenated with the last 64 KiB |

[D116](../../decisions/working-notes.md)'s sampling content hash already reads **1 MB of head,
1 MB of tail, and the size**. Both provider hashes are computed over a **strict subset of that
same window**, so they are extra arithmetic on bytes the node has already read — **no second
read, no extra pass, no extra I/O**, on a pass measured at 3.2 ms per file and 7 GB across the
whole reference library.

So the node emits them alongside the content hash, and they cost nothing. *([D179](../../decisions/working-notes.md)
has since anchored the identity hash on the media payload rather than the file's ends, so the
file's last 64 KiB is no longer certain to be in the window already read — at most one small
extra read, still no extra pass.)* Note they are
**provider identifiers, not our identity** — [D116](../../decisions/working-notes.md)'s hash
answers *is this the same file I saw before*, and these answer *has anyone else seen this exact
file*; they are different questions that happen to share a read.

### 4. The budget is schema, and the failure is reported honestly

[tech-licensing.md](../research/tech-licensing.md): OpenSubtitles is **5 downloads/day
anonymous, 20/day with a free account, 1000/day VIP**. Consequences:

- **Credentials are per user, not per server** — the same rule as
  [D13](../../decisions/working-notes.md)'s provider keys. They live in the database's `setting`
  table ([schema.md](schema.md)), which holds what changes at runtime
  ([D181](../../decisions/working-notes.md)), and [auth.md](auth.md) says how they are protected.
- **Counting downloads per user per day is a table.** The remaining budget is visible *before*
  a fetch, and exhausting it refuses cleanly.
- **Report the reason, not the symptom.** "Your daily quota is spent, 0 of 20 left, resets at
  midnight UTC" — not "failed". Bazarr maintains a typed back-off table with **3–6 hour**
  penalties for `DownloadLimitExceeded`; that is what "about half of them fail to download"
  actually is, and it is a budget message wearing an error's clothes.
- **Several providers**, as [tech-metadata.md](../research/tech-metadata.md) already concludes
  for metadata.

### 5. Verification is not only for fetched subtitles

The same 8 ms scores an embedded track or a sidecar the user already had. Those are out of sync
too, and nothing in the field can tell you which. A library-wide *"these 14 subtitle tracks do
not match their audio"* report is a query.

### 6. The parser is an adversarial-input parser

**7 of 65 subtitle files in the two *official* Blender releases needed tolerant parsing** — one
is an HTML page saved as `.srt`, several write `- >` instead of `-->`, several put the cue text
on the timestamp line. A strict parser returns zero cues and the user is told the download
failed. Encodings are worse: `ffsubsync` runs three separate charset detectors in order.
Belongs with [threat-model.md](threat-model.md)'s hostile-data-to-the-node rule, with an
extremely ordinary subject.

### 7. Where a fetched track goes

A sidecar beside the media, which is what every other tool does and what
[D98](../../decisions/working-notes.md) covers: a fetched subtitle is *about the work*, so it
is the clearest sidecar candidate there is. It needs a writable path, which is
[principles §2](../../principles/README.md)'s read-only default, so on a read-only library the
track lives in the cache and the user is told it will not survive a rebuild.

### 8. Who may do it

A capability on the role ([D130](../../decisions/working-notes.md)) — a download spends a
shared budget and touches an external service.

## Prior art we should read rather than reinvent

- **`ffsubsync`** (MIT) — FFT alignment, the discrete ratio set, golden-section search over
  continuous ratios, three VADs, three charset detectors, and a serialisable speech map. Bazarr
  vendors the whole package.
- **`alass`** — piecewise alignment with a split penalty, which is the one failure class we do
  not cover.

The parts that are ours are the **normalised, comparable score** — `ffsubsync` states in its
own source that its magnitude is not comparable, which is precisely why nobody ranks with it —
and the **policy** of scoring every candidate before offering any.

## What is not covered

- **Structural breaks** — an ad-break-segmented rip or a different cut — are neither an offset
  nor a ratio. A single linear correction cannot fix them; the honest behaviour is a mediocre
  score, an unverified label and the manual offset control. Piecewise alignment is a later
  version.
- **Stereo-only sources have no centre channel**, so they get the weaker reference. Unmeasured
  whether that costs enough to matter.
- **The pass on NAS-class hardware**, and silero's cost on ARM.

## Done when

**Cut A:** a person asks for subtitles for a film that has none, sees candidates each labelled
unverified, picks one, and corrects it with the offset control, which is kept; the quota is
visible before the fetch.

**Later, with verification:** a film with no subtitles gains a **verified** one in the user's language without the user
choosing from a list; an unverifiable one is refused in those words rather than served
silently; a user's provider quota is visible before it is spent and its exhaustion is reported
as a quota rather than a failure; a subtitle that was already in the library can be checked by
the same measurement; and nothing is fetched for a user who did not ask and holds no
credential.
