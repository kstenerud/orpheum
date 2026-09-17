# Roadmap

The recommended implementation plan, derived from the research in
[`contributors/design/research/`](contributors/design/research/) and the decisions in
[`contributors/decisions/working-notes.md`](contributors/decisions/working-notes.md).

Written 2026-09-17, revised the same day after the direct-play-first direction
([D17](contributors/decisions/working-notes.md)). Pre-implementation. Phase boundaries are
ordering constraints, not dates.

---

## Phase 0 — Measure the number the plan rests on

**[`playback-survey.md`](contributors/design/plans/playback-survey.md)**

A throwaway tool that `ffprobe`s a real library and asks each target client's **own runtime
probe** — `mediaCapabilities.decodingInfo()`, `MediaCodecList`, libmpv — what it can actually
decode. Output: the direct-play rate per client, and a ranked list of the specific failure
reasons.

**Why this and not the transcoder.** D17 bets that direct play covers the common case and
transcoding can be deferred. That rests on a number **nobody has published** — Tautulli exposes
it per-server, but no aggregate survey exists. Measuring it costs days; building against a
guess costs months.

**The three numbers that matter:** the direct-play rate with the platform decoder alone; the
rate **with libmpv in the client** (the delta is D10's two-engine recommendation quantified, and
it may be most of the problem); and how much of the remaining tail is **audio-only** — video
fine, TrueHD/DTS unsupported — which is cheap to fix and needs no video encoder at all.

**Exit:** numbers written to `research/playback-survey-results.md`, and D17 confirmed or
superseded. If the rate is low, the segmented transcoder returns to Phase 1 and this roadmap is
rewritten.

---

## Phase 1 — A server that is correct before it is featureful

`library-model.md` → `scanner.md` → `stream-service.md` *(direct play only)*

The schema (`Title → Edition → MediaFile`, provider IDs and field provenance in side tables,
audit/change-journal tables from commit one), the phased scanner with its own legible log and
report, and the playback service.

**What ships in the playback service:** ranged direct-play `GET`; the **rendition-list API
shape**, initially one entry; subtitle extraction at scan time with font attachments; trickplay
sprites; a **transcode-reason field** that can say "this server cannot transcode"; and a
capability handshake. Those last two are the seams that keep transcoding additive — they must
exist now even though nothing uses them yet.

**Deliberately absent:** any encoder. Users beyond one admin. Remote access. Sharing. Plugins.
Music.

---

## Phase 2 — Make it visible

`web-client.md`

The shared TypeScript SDK is the real deliverable — D2 puts rendition choice on the client, and
that is only safe if the probe-and-pick logic is written once and shared. The web UI is its
first consumer and doubles as the admin surface.

---

## Phase 3 — The client the project is judged on

`tv-client.md` — Android TV, React Native shell over the shared SDK, Kotlin native player module
with **Media3 and libmpv side by side**. Under D17 that second engine is not a nicety: it is the
main thing standing between "direct play only" and "plays your library". The Android phone
client then comes nearly free.

**Prototype remote-control focus navigation before writing any real screens.** It is what a TV
app lives or dies on, research cannot settle it, and it is where React Native on TV is most
likely to disappoint.

---

## Phase 4 — Transcoding, as a separate optional component

`transcoding.md`, prioritised by what Phase 0 measured.

**(a) Compatibility transcoding first** — a *library* problem. Old or odd files, converted
offline by a background job into a stored alternate version that then direct-plays forever. No
segment alignment, no keyframe grid, no ABR. If Phase 0 says the tail is a few hundred old
files, a batch job is the entire feature.

**(b) Bandwidth transcoding second** — a *network* problem, unrelated to the file, and the only
case that genuinely needs live keyframe-aligned segmented transcoding with adaptive bitrate. It
is coupled to remote access, not to the library, and on a LAN it is almost never needed.

Ships as a **separate component** with the GPL encoder build: smaller default install, smaller
licence and patent surface for that install, and a packaging boundary that mechanically stops
encoder logic spreading through the core — which is how `EncodingHelper.cs` reached 8,042 lines.

---

## Phase 5 — Borrow an ecosystem

`jellyfin-api-shim.md` — an out-of-process adapter speaking Jellyfin's API against our public
REST API, so Findroid, Swiftfin, Infuse, Streamyfin and Wolphin work against our server.

It is also where a device-profile evaluator has to live — accepting a profile and answering it
from our rendition list. **That keeps the design we rejected permanently outside the core, in a
component we are free to abandon.** Bridge, not foundation: our own clients speak our own API,
and if the adapter starts constraining that API it gets frozen rather than extended.

Schedulable any time after Phase 1 stabilises the API. Placed here because a stable API matters
more than an early one — but if the Phase 3 TV client slips, moving this earlier is the obvious
hedge.

---

## Phase 6 — The hard product problems

`remote-access.md` and `sharing-model.md`, both `UNSPECIFIED`, both needing a research round
before design.

Late because they are hard, not unimportant — they are what decides whether this replaces Plex
for a *household* or only for one person. Remote access has a genuine tension with D13's
no-required-account promise, and the research must resolve it honestly rather than quietly
weakening one side. Note that Phase 4(b) and remote access are the same project seen from two
directions.

---

## Phase 7 — Audio, cheaply

An OpenSubsonic API surface (a second out-of-process adapter, per D15) over the existing model,
plus a music library type.

**Server-side work only.** Speaking OpenSubsonic inherits Symfonium, Amperfy, Tempo and Feishin
for free. There is nothing equivalent for video, which is exactly why video is expensive and
music is not.

Not in scope: audiobooks and podcasts — a different product wearing the same clothes, and
Audiobookshelf's 1,191 open issues are the evidence.

---

## Explicitly not on this roadmap

Photos (D1), DRM, a device-profile database in the core (D2/D18), an encoding ladder,
microservices, a Roku client, in-process protocol plugins, live TV and DVR.

---

## The risks, ranked

1. **The direct-play rate is lower than assumed.** Phase 0 exists to find out in days rather
   than months. This is the only risk that rewrites the roadmap rather than delaying it.
2. **Client cost swamps everything.** Clients are where every project in this field spends its
   effort, and where Plex's fifteen-year lead is real. Phase 5 is the main lever, and bringing
   it forward is the standing hedge.
3. **TV focus navigation on React Native disappoints.** Fallback is a native Kotlin
   Compose-for-TV client reusing only the API contract — more work, not a dead end.
4. **Postgres-only costs installs.** Mitigated by shipping it in the default compose file. If it
   bites, D5 is the decision most worth revisiting.
5. **Codec patents.** Separate from copyright licence and not resolved by being open source.
   Direct-play-first reduces a default install's exposure to nearly nothing because it never
   encodes; get real advice before anything commercial.
6. **Scope creep into audiobooks, comics, live TV, photos.** The competitive trackers show
   exactly what this costs. The roadmap's job is to keep saying no.
