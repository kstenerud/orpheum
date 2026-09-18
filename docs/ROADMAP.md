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

**What ships in the playback service:** ranged direct-play `GET`; the **three-axis rendition
negotiation** (video ‖ audio ‖ subtitles, [D22](contributors/decisions/working-notes.md));
**audio transcoding and remuxing**, which are LGPL-only and cover the most common real-world
transcode; subtitle extraction at scan time with font attachments; trickplay sprites; a
**transcode-reason field** that can say "this server cannot transcode video"; and a capability
handshake. Those last two are the seams that keep video transcoding additive — they must exist
now even though nothing uses them yet.

**Two things promoted into Phase 1 by the plugin survey** ([D20](contributors/decisions/working-notes.md)),
because both are schema-shaped and expensive to retrofit: **auth with OIDC/LDAP** (a third-party
Jellyfin SSO plugin has 1,458 stars — that is a bug report about a core), and **a rules engine**
for smart collections, smart playlists and retention (~7,000 stars across plugins and companion
apps exist because no server here lets a user express a rule about their library).

**Deliberately absent:** any *video* encoder. Remote access. Sharing. Plugins. Music.

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

## Phase 4 — Video transcoding, as a plugin we happen to maintain

`transcoding.md`, prioritised by what Phase 0 measured.

Built on the **public** plugin API, with no privileged access to the core
([D19](contributors/decisions/working-notes.md)). That is not tidiness: it is the only way to
find out whether the plugin API is any good while we still own both sides of it, and it is what
makes the next paragraph possible.

Note what Phase 1 already covered: **audio-only transcoding**, which is the most common real
case. This phase is the *video* remainder.

**(a) Compatibility transcoding first** — a *library* problem. Old or odd files, converted
offline by a background job into a stored alternate version that then direct-plays forever. No
segment alignment, no keyframe grid, no ABR. If Phase 0 says the tail is a few hundred old
files, a batch job is the entire feature.

**(b) Bandwidth transcoding second** — a *network* problem, unrelated to the file, and the only
case that genuinely needs live keyframe-aligned segmented transcoding with adaptive bitrate. It
is coupled to remote access, not to the library, and on a LAN it is almost never needed.

**The capability that falls out for free: the machine that encodes need not be the machine that
serves.** A transcoder that registers rather than links can live on the desktop with the GPU
while the NAS serves files, and several can register with the core picking by advertised
capability. Nobody in this field offers this — `rffmpeg` (1,073 stars, **written by a Jellyfin
founder**) is an out-of-tree wrapper that impersonates the `ffmpeg` binary and SSHes to another
host, because Jellyfin has nowhere to put it.

Ships with its own GPL encoder build: smaller default install, smaller licence and patent
surface for that install, and a boundary that mechanically stops encoder logic spreading through
the core — which is how `EncodingHelper.cs` reached 8,042 lines.

**Build the interface last.** Write the transcoder against a private contract, get it working on
real hardware, then promote the contract to public. An API designed against an imaginary consumer
is how plugin systems go bad.

---

## Phase 5 — Borrow an ecosystem

`jellyfin-api-shim.md` — a tier-2 protocol adapter speaking Jellyfin's API against our public
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

An OpenSubsonic API surface — a second tier-2 protocol adapter, the same shape as Phase 5 — over
the existing model, plus a music library type.

**Server-side work only.** Speaking OpenSubsonic inherits Symfonium, Amperfy, Tempo and Feishin
for free. There is nothing equivalent for video, which is exactly why video is expensive and
music is not.

Not in scope: audiobooks and podcasts — a different product wearing the same clothes, and
Audiobookshelf's 1,191 open issues are the evidence.

---

## Explicitly not on this roadmap

Photos (D1), DRM, a device-profile database in the core (D2/D18), an encoding ladder,
microservices, a Roku client, in-process protocol plugins, an encoder in the core binary (D19),
live TV and DVR.

---

## The risks, ranked

1. **The direct-play rate is lower than assumed.** Phase 0 exists to find out in days rather
   than months. This is the only risk that rewrites the roadmap rather than delaying it.
2. **Client cost swamps everything.** Clients are where every project in this field spends its
   effort, and where Plex's fifteen-year lead is real. Phase 5 is the main lever, and bringing
   it forward is the standing hedge.
3. **TV focus navigation on React Native disappoints.** Fallback is a native Kotlin
   Compose-for-TV client reusing only the API contract — more work, not a dead end.
4. ~~**Postgres-only costs installs.**~~ **Resolved** — re-examined and reversed to SQLite
   ([D31](contributors/decisions/working-notes.md)). The residual risk is the opposite one: if a
   genuinely multi-tenant deployment ever appears, adding Postgres means a second set of
   `sqlc`-generated queries.
5. **Codec patents.** Separate from copyright licence and not resolved by being open source.
   Direct-play-first reduces a default install's exposure to nearly nothing because it never
   encodes; get real advice before anything commercial.
6. **Scope creep into audiobooks, comics, live TV, photos.** The competitive trackers show
   exactly what this costs. The roadmap's job is to keep saying no.
7. **Plugin-API speculative generality.** Three tiers is already a taxonomy, and taxonomies
   invite designing for members that do not exist. Mitigation: every tier must have a real
   first-party member before it is built, and the transcoder's interface is *extracted* from a
   working implementation rather than designed ahead of one. Tier 3's second consumer is already
   identified — intro/credit detection, whose requirements match the transcoder's almost exactly
   with a completely different workload ([D20](contributors/decisions/working-notes.md)).
8. **The plugin ABI becomes a compatibility millstone.** Jellyfin plugins break every release;
   Plex killed its plugin system after seven years as unsupportable. Mitigation: a declared
   versioned interface rather than a link against internals, a host-service list short enough to
   read, and our own transcoder depending on it so it can never be the <2%-usage feature that
   gets cut.
