# Roadmap

The recommended implementation plan, derived from the research in
[`contributors/design/research/`](contributors/design/research/) and the decisions in
[`contributors/decisions/working-notes.md`](contributors/decisions/working-notes.md).

Written 2026-09-17, pre-implementation. Phase boundaries are ordering constraints, not dates.

---

## Phase 0 — Settle the one assumption that could invalidate everything

**[`transcode-core-spike.md`](contributors/design/plans/transcode-core-spike.md)**

A standalone Go binary — no database, no API, no UI — that extracts a source's keyframe grid,
serves an HLS manifest advertising passthrough plus transcoded renditions, and produces
segments on demand aligned to that grid. Run it against a deliberately pathological real
library on the actual target hardware.

**Why first.** The entire architecture rests on segment-aligned on-demand transcoding working.
Kyoo proves it can be done; it does not prove *we* can do it, at our library sizes, on our
hardware, across our three target players.

**The number that decides the rest of the project:** what fraction of a real library
direct-plays on each target client with no server work at all. If that number is high, the
transcoder matters less than assumed and the whole plan simplifies.

**Exit:** measured answers written to `research/transcode-spike-results.md`, and either D4
confirmed or a superseding decision recorded.

---

## Phase 1 — A server that is correct before it is featureful

`library-model.md` → `scanner.md` → `stream-service.md`

The schema (`Title → Edition → MediaFile`, provider IDs and field provenance in side tables,
audit/change-journal tables from commit one), the phased scanner with its own legible log and
report, and the streaming service promoted out of the spike.

**What "correct" means here:** a library rebuilt from empty loses nothing; an interrupted scan
resumes; the scan report answers "why is this file not in my library?" without a debug log;
every decision the server makes on the user's behalf is inspectable.

**Deliberately absent in Phase 1:** user management beyond one admin, remote access, sharing,
plugins, music, transcode pre-preparation.

---

## Phase 2 — Make it visible

`web-client.md`, and then the decision point on `jellyfin-api-shim.md`.

The shared TypeScript SDK is the real deliverable — D2 puts rendition choice on the client, and
that is only safe if the probe-and-pick logic is written once and shared. The web UI is the
first consumer and doubles as the admin surface.

**The decision point.** Before writing a second client, settle whether to serve a
Jellyfin-compatible API surface. Navidrome does the mirror image of this deliberately, and if a
shim makes Findroid, Swiftfin, Infuse and Streamyfin work on day one it is worth more than any
client we could write in the same time — and could move the first-party TV client from Phase 3
to Phase 4. **Do not start the TV client with this unresolved.**

---

## Phase 3 — The client the project is judged on

`tv-client.md` — Android TV, React Native shell over the shared SDK, Kotlin native player module
with Media3 and libmpv side by side.

**Prototype remote-control focus navigation before writing any real screens.** It is what a TV
app lives or dies on, research cannot settle it, and it is where React Native on TV is most
likely to disappoint. The Android phone client then comes nearly free.

---

## Phase 4 — The hard product problems

`remote-access.md` and `sharing-model.md`, both currently `UNSPECIFIED` and both needing a
research round before they can be designed.

These are placed late because they are hard, not because they are unimportant — they are the
two features that decide whether this replaces Plex for a household or only for one person.
Remote access has a genuine tension with D13's no-required-account promise, and the research
must resolve it honestly rather than quietly weakening one side.

---

## Phase 5 — Audio, cheaply

Add an OpenSubsonic API surface over the existing model, and a music library type.

This is a **server-side-only project**. Speaking OpenSubsonic inherits a mature third-party
client ecosystem — Symfonium, Amperfy, Tempo, Feishin — at zero client cost. There is nothing
equivalent for video, which is exactly why video is expensive and music is not.

Not in scope: audiobooks and podcasts. They are a different product wearing the same clothes,
and Audiobookshelf's 1,191 open issues are the evidence for what happens when that is ignored.

---

## Explicitly not on this roadmap

Photos (D1 — Immich exists, is AGPL, has 114k stars and moves faster than we could), DRM, a
device-profile database, an encoding ladder, microservices, a Roku client, in-process plugins,
live TV and DVR.

---

## The risks, ranked

1. **The transcode core is harder than the research suggests.** Mitigated by Phase 0 existing
   at all. This is the only risk that invalidates the plan rather than delaying it.
2. **Client cost swamps everything.** Clients are where every project in this field spends its
   effort and where Plex's fifteen-year lead is real. The `jellyfin-api-shim` decision is the
   main lever.
3. **TV focus navigation on React Native disappoints.** Fallback is a native Kotlin
   Compose-for-TV client reusing only the API contract — more work, not a dead end.
4. **Postgres-only costs installs.** Mitigated by shipping it in the default compose file. If
   it bites, D5 is the decision most worth revisiting.
5. **Metadata provider licensing.** TMDB conditions commercial use on a written agreement;
   TheTVDB tiers by the licensee's revenue. Needs a real answer before any public release, and
   the answer is probably "the user supplies their own key".
6. **Scope creep into audiobooks, comics, live TV, photos.** The competitive trackers show
   exactly what this costs. The roadmap's job is to keep saying no.
