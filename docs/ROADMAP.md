# Roadmap

The recommended implementation plan, derived from the research in
[`contributors/design/research/`](contributors/design/research/) and the decisions in
[`contributors/decisions/working-notes.md`](contributors/decisions/working-notes.md).

Written 2026-09-17, revised the same day after the direct-play-first direction
([D17](contributors/decisions/working-notes.md)). Revised **2026-09-19** after an independent
audit ([audit-2026-09-19.md](contributors/design/research/audit-2026-09-19.md),
[D80](contributors/decisions/working-notes.md)–[D108](contributors/decisions/working-notes.md)),
which added six schema items to Phase 1 and moved one thing out of Phase 4. Revised
**2026-09-20** when the last two unspecified plans were researched
([D109](contributors/decisions/working-notes.md)–[D118](contributors/decisions/working-notes.md)),
which put a measured *"17 minutes to a browsable library"* under Phase 1 and made the
first-scan ordering a requirement. Revised **2026-09-21** when Watch Together was settled by
the project owner ([D117](contributors/decisions/working-notes.md)–[D124](contributors/decisions/working-notes.md))
and finally acquired a phase. Revised **2026-09-23** after a second audit
([audit-2026-09-23.md](contributors/design/research/audit-2026-09-23.md),
[D191](contributors/decisions/working-notes.md)–[D193](contributors/decisions/working-notes.md)),
which moved every media read and FFmpeg run into the node, made **prepare** Cut A's one path
besides direct play, and froze Cut A's design. Pre-implementation. Phase boundaries are ordering
constraints, not dates.

---

## Now: Cut A — two people, one of them remote

**Decided 2026-09-23** ([D178](contributors/decisions/working-notes.md)). The first release is for the owner at home and one person
remote, watching together with subtitles. **A feature is in it only if one of them needs it;
the schema that would preclude a later feature lands regardless; additive tables arrive with their features** ([D180](contributors/decisions/working-notes.md)). The phases below are the order *after* the cut
— Phase 0 and the parts of Phases 1–2b that the cut includes are its own work — and everything
the cut leaves out keeps its place there. **Where a phase's text below promotes
something this section leaves out, this section wins.**

**Cut A's design is frozen** ([D193](contributors/decisions/working-notes.md) §8).
**[plans/cut-a.md](contributors/design/plans/cut-a.md) is the build contract** — topology, byte
path, repository layout, endpoints, message sets, errors, the job queue and milestones M0–M5 —
and a new decision is made only when code or a measurement forces one.

- **Library:** scan, identify, ambiguous-match chooser and manual fixes, versions, extras, NFO
  read, *what the scanner saw*.
- **Playback in a browser:** direct play, negotiation, **remux to fMP4 and audio transcoding**
  (mandatory — browsers and Chromecast refuse MKV and most of TrueHD, DTS and AC-3), all through
  **prepare** ([D193](contributors/decisions/working-notes.md)): one node FFmpeg job per File and
  profile (`remux`, `h264-8m`) writing a growing HLS `EVENT` playlist, so playback starts once the
  first segments exist. The node does every media read and FFmpeg run; the core authenticates
  and relays.
- **Finding and resuming:** browse and search, play state and Continue Watching, artwork.
- **Subtitles:** embedded and sidecar with libass, a sync offset control, **acquisition on
  request** with the server's OpenSubtitles login ([D224](contributors/decisions/working-notes.md)). Subtitles start off; a person's
  choice is kept per file and carried to the work's other files ([D223](contributors/decisions/working-notes.md) §3,
  [D219](contributors/decisions/working-notes.md)).
- **Quiet dialogue:** [D83](contributors/decisions/working-notes.md)'s dialogue boost in the downmix, with the device declaring
  stereo speakers, and loudness levelling: the node measures each sound in a job, and the player
  applies a gain towards -23 LUFS ([D226](contributors/decisions/working-notes.md)).
- **Together:** local accounts, invite from outside, Watch Together for two with voice (browser
  WebRTC through a TURN server embedded in our binary), reachability (a port-forward, then IPv6,
  then a standard tunnel to a VPS, under one public HTTPS name from DNS-01), prepared versions
  (prepare's `h264-8m`), the cast receiver. The session behaves as if nobody has an account; a
  signed-in participant's viewing is also recorded as their own
  ([D191](contributors/decisions/working-notes.md)). **Her television plays and her phone
  talks** ([D188](contributors/decisions/working-notes.md)), by the first path her setup allows: its browser, our cast receiver,
  a sideloaded prototype, or else the laptop — joining the session as a device of her
  membership, not paired to the server (D191).
- **Running it:** backup and restore, now playing.
- **The architecture:** media nodes, and the web client good on a phone — plain React +
  TypeScript + Vite, with a pinned jellyfin-ffmpeg on the node (D193 §6).

**Built privately, then public betas** ([D192](contributors/decisions/working-notes.md)). Cut A is the internal stage: a
private repository, the owner and their partner testing. The public packaging channels, the
release feed and the published cast receiver are beta-stage work.

**Out for now:** roles and parental controls, collections-as-sequences, offline, segment
detection, trickplay, subtitle verification, NFO write, TV pairing and
the native TV client, ceilings and adaptation, the Jellyfin adapter, OIDC/LDAP, rules authoring
and the predicate compiler, play statistics, live bandwidth transcoding and live
segment-addressed remux, our own relay role, the WASM plugin tier (D193).

**Phase 0 narrows:** what fraction of the owner's library direct-plays in Chrome, Safari and a
Chromecast, and what the rest needs — remux, audio, or video. The lab owns no Chromecast yet
([D190](contributors/decisions/working-notes.md) §6), so one has to be bought first.

---

## Phase 0 — Measure the number the plan rests on

**[`playback-survey.md`](contributors/design/plans/playback-survey.md)**

*Narrowed for Cut A ([D178](contributors/decisions/working-notes.md) §4): Chrome, Safari and a
Chromecast only. The `MediaCodecList` and libmpv numbers below wait for the TV client.*

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

*Read through Cut A. Of what this phase lists, the speech map and verification, trickplay,
OIDC/LDAP, the rules engine and its predicate compiler, roles, certifications, typed segments
and device pairing come **after** the cut ([D178](contributors/decisions/working-notes.md) §4,
[D193](contributors/decisions/working-notes.md)); their additive tables arrive with their
features ([D180](contributors/decisions/working-notes.md)). Loudness columns are in the cut
([D226](contributors/decisions/working-notes.md)).*

`library-model.md` → `scanner.md` → `identification.md` → `stream-service.md`
~~*(direct play only)*~~ *(direct play and prepare, D193)*, with `internationalisation.md`
~~and `rules-engine.md`~~ landing **before** the schema rather than after it.

Two of those are easy to miss because they are not obviously Phase 1.
**[identification.md](contributors/design/plans/identification.md)** was split out of the
scanner by [D59](contributors/decisions/working-notes.md) — the node **observes**, the core
**concludes** — and the split is what makes a parser improvement a re-interpretation pass over
the database with every node switched off. And
**[internationalisation.md](contributors/design/plans/internationalisation.md)** is
schema-shaping in three places ([D45](contributors/decisions/working-notes.md),
[D47](contributors/decisions/working-notes.md),
[D48](contributors/decisions/working-notes.md)): the translation split, the materialised CLDR
sort-key column, and the FTS5 tokeniser — **which is immutable once the virtual table exists**,
so it cannot be deferred. Getting either wrong means the first migration is an i18n migration.

The schema (**`Title → Edition → Version → File`** with Collections orthogonal —
[D79](contributors/decisions/working-notes.md) corrected this from three levels on 2026-09-19 —
provider IDs in side tables, overrides carrying their origin ([D183](contributors/decisions/working-notes.md)),
~~audit/change-journal tables from commit one~~ the change journal arriving with its feature, D180),
the node's snapshot-plus-journal scanner, and the playback service.

**And the first scan has an ordering requirement now, not just a shape**
([D118](contributors/decisions/working-notes.md),
[D114](contributors/decisions/working-notes.md)): passes are ordered so an item becomes
**playable as early as possible** — measured at **17 minutes to a browsable 3,500-item
library** against days for the full treatment — the user can prioritise one title mid-scan,
and the core tails the node's in-progress report so items appear while the scan is still
running rather than when it ends.

**What ships in the playback service:** ranged direct-play `GET`, relayed by the core from the
node; the **three-axis rendition negotiation** (video ‖ audio ‖ subtitles,
[D22](contributors/decisions/working-notes.md)); **audio transcoding and remuxing** — and in
Cut A a `h264-8m` video encode — all in the node's prepare job
([D193](contributors/decisions/working-notes.md)); text subtitle extraction with font
attachments; a **transcode-reason field** that can say "this server cannot transcode video"; and
a capability handshake, which in Cut A is how a node advertises `remux`, `aac` and its encoder
(D193 §3). Those last two are the seams that keep further video transcoding additive.
*After Cut A:* **the speech map, and with it subtitle verification**
([D147](contributors/decisions/working-notes.md),
[D148](contributors/decisions/working-notes.md)) — a tenth scan pass costing ~32 s a film and
1–3 kB of storage, which is what lets us say a subtitle *matches the audio* rather than
*matches the filename*, and which applies to tracks the user already had as much as to
fetched ones; and trickplay sprites.

**Two things promoted into Phase 1 by the plugin survey** ([D20](contributors/decisions/working-notes.md)) — both now after Cut A,
because both are schema-shaped and expensive to retrofit: **auth with OIDC/LDAP** (a third-party
Jellyfin SSO plugin has 1,458 stars — that is a bug report about a core), and **a rules engine**
for smart collections, smart playlists and retention (~7,000 stars across plugins and companion
apps exist because no server here lets a user express a rule about their library).

**Six more things the 2026-09-19 audit put in Phase 1**, all for the same reason — they are
schema, and `library-model.md` is here:

- **Play state as a guarded position plus an event log**
  ([play-state.md](contributors/design/plans/play-state.md),
  [D81](contributors/decisions/working-notes.md)). Jellyfin's single mutable row is why two
  devices rewind each other, and why Tautulli (6,590★) exists as a separate program.
- **Typed media segments**, at the Version
  ([media-segments.md](contributors/design/plans/media-segments.md),
  [D82](contributors/decisions/working-notes.md)). The *detector* was already scheduled as a
  ~~tier-3 plugin~~ node capability (D193 §3); the table it publishes into was never planned, which is exactly the gap
  Jellyfin closed in 10.10.
- **Loudness columns per audio track**
  ([audio-presentation.md](contributors/design/plans/audio-presentation.md),
  [D83](contributors/decisions/working-notes.md)) — measured by the node, because we do
  the downmix and therefore own the quiet dialogue. In Cut A, by a job per file rather than at
  scan, and kept by the payload's hash ([D226](contributors/decisions/working-notes.md) §2).
- **Certifications as `(system, code)`, plus a content-descriptor vector**
  ([parental-controls.md](contributors/design/plans/parental-controls.md),
  [D84](contributors/decisions/working-notes.md),
  [D131](contributors/decisions/working-notes.md)). A household product with children in it
  needs this, and flattening national rating systems to an integer fails *open*. The vector is
  the same argument one level up: a tween blocked from *Sex and the City* and a teenager
  blocked from *The Blair Witch Project* do not sort onto one scale. Seven axes, tri-valued so
  unknown is distinct from none, each with provenance. **Schema only** — the template set that
  makes this usable is deliberately later, because a template whose blocked-item count was
  never checked against a real library is a guess with a progress bar.
- **People as entities with provider identity**
  ([D95](contributors/decisions/working-notes.md)) — never keyed on the display name.
- **Read the `.nfo` sidecars that are already in the library**
  ([D140](contributors/decisions/working-notes.md),
  [identification.md](contributors/design/plans/identification.md)). Phase 1 because **first
  run is when it matters**: the user this project is for arrives from Kodi, Emby or Jellyfin
  with years of corrections beside their media, and
  [D7](contributors/decisions/working-notes.md) promises corrections survive. A first scan
  that ignores them destroys exactly what we promise to protect, at the moment of highest
  goodwill, and a later fix cannot recover someone who already gave up. *Plex NFO Agent Forum
  Preview* is 291 likes and **510 posts — the most-replied thread on their forum this year**.
- **A `role` table, and one `role_id` per user**
  ([D130](contributors/decisions/working-notes.md)). A role is a named policy object — a closed
  capability set plus one rules-engine predicate — so "Kids" is defined once and children are
  assigned to it. Schema, and therefore here. Jellyfin's `UserPolicy.cs` is the warning: **44
  settable fields per user and no grouping**, which is what happens when there is no way to say
  "these people are the same".
- **Episode orderings and extras** ([D96](contributors/decisions/working-notes.md),
  [D97](contributors/decisions/working-notes.md)) — D79's correction repeating twice more. An
  episode needs a position *per ordering* (aired, DVD, absolute), and an extra is a typed
  relation rather than a level, because a trailer is neither a cut nor an encoding.
- **[auth.md](contributors/design/plans/auth.md)**, which had no plan at all. D20 already put
  OIDC/LDAP here; identity, sessions, tokens and **RFC 8628 device pairing**
  ([D93](contributors/decisions/working-notes.md)) needed an owner. Cut A takes local accounts,
  sessions and invites; pairing waits until her television is used alone
  ([D191](contributors/decisions/working-notes.md)).

And **[api-compatibility.md](contributors/design/plans/api-compatibility.md)**
([D85](contributors/decisions/working-notes.md)) — not schema, but its CI gate is cheap now and
its deadline is the first client ship, because our clients go through app stores and
televisions never update.

**Deliberately absent:** ~~any *video* encoder. Remote access. Sharing.~~ Cut A brings a video
encoder (prepare's `h264-8m`), reachability and invites (D178, D193). Still absent: sharing
beyond an invite, plugins, music.

---

## Phase 2 — Make it visible

`web-client.md`

The shared TypeScript SDK is the real deliverable — D2 puts rendition choice on the client, and
that is only safe if the probe-and-pick logic is written once and shared. The web UI is its
first consumer and doubles as the admin surface.

**Two things become buildable here rather than later**, and both are cheap because they are
this client in a different shell. A **cast receiver** is a web app
([D124](contributors/decisions/working-notes.md)) and is the answer for a Watch Together
guest's television, so the build order is **web client → cast receiver**, well before our own
TV client is finished. And **Watch Together's LAN half** needs nothing from Phase 6 — see
below.

---

## Phase 2b — Watch Together for two, remote, with voice

**Moved here 2026-09-23 by the project owner** ([D175](contributors/decisions/working-notes.md)):
two people in different houses is the case that matters first, and it is the design's best case
rather than a stretch. In order, after the web client:

1. **Watch Together's LAN half** — the sync machinery, six methods, voice.
2. **Reachability.** A port-forward plus TLS if the owner's ISP allows it; if it is behind
   carrier-grade NAT, check IPv6, and failing that ~~**the relay ([D71](contributors/decisions/working-notes.md))
   moves here from Phase 6**~~ a standard tunnel (rathole, frp or WireGuard) to a VPS; our own
   relay role stays deferred ([D193](contributors/decisions/working-notes.md) §4). One public
   HTTPS name, certified by DNS-01, serves the web client, the cast receiver and TURN (§5). For
   the remote person it is always just an invite link.
3. **Invite someone outside the house** ([D174](contributors/decisions/working-notes.md)) — an
   account, a role, a link.
4. **Prepared versions** — ~~Phase 4(a)'s batch transcoder, moved forward: a film chosen in
   advance gets a ~8 Mbps Version and then direct-plays~~ prepare's `h264-8m` profile started
   in advance: "get this ready", or automatically when a session is created (D193 §2). The
   result is a cache, not a Version ([D169](contributors/decisions/working-notes.md)). Live ABR
   (4b) stays late.
5. **Remote Watch Together for two**, with voice from the start; headphones on at least one side
   make every configuration a solved case ([D63](contributors/decisions/working-notes.md),
   [D65](contributors/decisions/working-notes.md)).

**The web client is the phone client until native ones exist**
([D176](contributors/decisions/working-notes.md)), so it has to be good on a phone from the start.

---

## Phase 3 — The client the project is judged on

`tv-client.md` — Android TV over the shared SDK, Kotlin native player module with **Media3 and
libmpv side by side**. The UI framework is decided by building the same four screens in Compose
for TV and in react-native-tvos when TV work starts ([D193](contributors/decisions/working-notes.md) §6). Under D17 that second engine is not a nicety: it is the
main thing standing between "direct play only" and "plays your library". The Android phone
client then comes nearly free.

**Prototype remote-control focus navigation before writing any real screens.** It is what a TV
app lives or dies on, research cannot settle it, and it is what the paired prototype decides.

---

## Phase 4 — Video transcoding, as a node capability

`transcoding.md`, prioritised by what Phase 0 measured.

~~Built on the **public** plugin API, with no privileged access to the core
([D19](contributors/decisions/working-notes.md)).~~ **Plugin tier 3 is retired**
([D193](contributors/decisions/working-notes.md) §3): the encoder is a capability a media node
advertises (`h264_qsv`, `h264_nvenc`, `x264`), and the core sends a job to a node that can do
it. That keeps D19's point — no privileged encoder logic in the core — without a plugin API.

Note what Cut A already covers: **prepare**, with `remux` and `h264-8m` — audio transcoding, the
common case, and a single video profile. This phase is the *video* remainder.

**(a) Compatibility transcoding first** — a *library* problem. Old or odd files, converted
offline by a background job into a stored alternate version that then direct-plays forever. No
segment alignment, no keyframe grid, no ABR. If Phase 0 says the tail is a few hundred old
files, a batch job is the entire feature.

**(b) Bandwidth transcoding second** — a *network* problem, unrelated to the file, and the only
case that genuinely needs live keyframe-aligned segmented transcoding with adaptive bitrate. It
is coupled to remote access, not to the library, and on a LAN it is almost never needed.

**But it is not really Phase 4.** [D86](contributors/decisions/working-notes.md): remote access
is the headline feature and the paid tier, and over a degraded remote link there is nothing
lower to fall back to unless this exists. Direct play has no adaptation at all — the rendition
is chosen once, at the start. So **(b)'s real deadline is Phase 6's, whichever comes first**,
and the segment-grid alignment specified above stops being a detail of transcoding and becomes a
requirement of remote access. The client-side half is free: under
[D2](contributors/decisions/working-notes.md) the client already picks, and Netflix's own
SIGCOMM'14 study says the right input is its buffer level rather than a bandwidth estimate.

**The capability that falls out for free: the machine that encodes need not be the machine that
serves.** A second node can live on the desktop with the GPU while the NAS serves files, and
the core picks by advertised capability. Nobody in this field offers this — `rffmpeg` (1,073 stars, **written by a Jellyfin
founder**) is an out-of-tree wrapper that impersonates the `ffmpeg` binary and SSHes to another
host, because Jellyfin has nowhere to put it.

The node process is the boundary that mechanically stops encoder logic spreading through the
core — which is how `EncodingHelper.cs` reached 8,042 lines. The FFmpeg is a pinned
jellyfin-ffmpeg portable build; ~~its own GPL encoder build~~ our own hardened build is deferred
(D193 §4, §6).

~~**Build the interface last.** Write the transcoder against a private contract, get it working
on real hardware, then promote the contract to public.~~ There is no public transcoder interface
to build: the node's job protocol is internal (D193 §3).

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

`remote-access.md` and `sharing-model.md`. **No longer `UNSPECIFIED`** — both were researched
2026-09-18/19 and are now `PARTLY SPECIFIED` and `PLANNED`; what remains in the first is a
product decision rather than a design one.

Late because they are hard, not unimportant — they are what decides whether this replaces Plex
for a *household* or only for one person. Remote access has a genuine tension with D13's
no-required-account promise, and the research must resolve it honestly rather than quietly
weakening one side. Note that Phase 4(b) and remote access are the same project seen from two
directions — and [D86](contributors/decisions/working-notes.md) makes that a dependency rather
than an observation: **remote access ships broken on a bad link without 4(b)**.

The mechanism is settled — the relay is a mode of our own binary that the core dials out to
([D71](contributors/decisions/working-notes.md); deferred past Cut A, which uses a standard
tunnel, [D193](contributors/decisions/working-notes.md)), and a share turned out to be a grantee plus
a predicate, a fourth binding of the rules engine
([D76](contributors/decisions/working-notes.md)). What is left here is the product decision,
not the design.

---

## Phase 6b — Watch Together, beyond two

*Two people remote moved to Phase 2b ([D175](contributors/decisions/working-notes.md)); what is
left here is the party of four or five, which is gated on live adaptive bitrate.*

`watch-together.md` — which had no place on this roadmap at all until 2026-09-21, despite
being one of the six things [overview.md](contributors/design/overview.md) names as what we do
differently, and carrying eight decisions. It is here rather than in a phase of its own
because **it splits along the same seam Phase 6 does**:

- **The LAN half is buildable as soon as Phase 2 lands.** It needs `stream-service.md` and the
  SDK, and nothing else. It exercises the whole sync machinery — position reports over bounded
  round-trips (no clock synchronisation, [D66](contributors/decisions/working-notes.md)), suspensions,
  command serialisation, the six-method client surface — with none of the NAT traversal
  ([D62](contributors/decisions/working-notes.md)). Build it there and the hard part is done.
- **The remote half is ~~Phase 6~~ Phase 2b** ([D175](contributors/decisions/working-notes.md)),
  because the motivating case is someone in another house. Voice needs no TURN service of anyone
  else's ([D68](contributors/decisions/working-notes.md)) — both parties are already connected to
  the media server by construction, so the server relays it, through a TURN server embedded in
  our binary ([D193](contributors/decisions/working-notes.md)).

The design constraint is the schedule risk, and it is hard: **the client-specific surface is
six methods** ([D62](contributors/decisions/working-notes.md)). Five commercial
implementations were withdrawn and Plex's own note gives the cause — theirs lived in every
client. If a seventh method starts to look necessary, stop.

What the owner settled in 2026-09-20/21, all of it schedule-relevant because it is session
state rather than client work: the **session link is the grant** and revocation is the control
([D117](contributors/decisions/working-notes.md)); a **link admits, a membership persists**, so
rotation is seamless and ejection is owner-only, with a **cap on participants** as the control
that works before anyone notices ([D119](contributors/decisions/working-notes.md)); a
participant is a **person** with devices attached by role, which is what makes a phone a voice
companion rather than a second guest ([D120](contributors/decisions/working-notes.md)); the
session is the object and what plays is **mutable state on it**
([D122](contributors/decisions/working-notes.md)); and the `playback` role may be a **cast
receiver**, which is the best path for a guest and is Phase 2 work
([D124](contributors/decisions/working-notes.md)).

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

## What this roadmap is not

It is the **when**. [`FEATURES.md`](FEATURES.md) is the **whether and how much** — every
feature this project knows about, ranked, with the declined ones kept visible and a column
saying what would make each possible again ([D153](contributors/decisions/working-notes.md)).

The two are read differently. A phase here is a schedule. That file's **retrofit-cost** column
is a constraint: it names the handful of things — *an item need not have a File*, *Collection
membership is three kinds*, *a rating separates critic from audience* — that cost nothing to
keep true now and cannot be recovered once assumed away
([principles §12](contributors/principles/README.md)). **Several of them are owed by
`library-model.md`, which is Phase 1.**

## Unscheduled, and deliberately so

Some plans have no phase because they are independent of the critical path and can land
whenever there is room. Listed rather than forgotten — `ls contributors/design/plans/` is the
authority, not this list:

- **[accessibility.md](contributors/design/plans/accessibility.md)** — mostly already built by
  [D50](contributors/decisions/working-notes.md) and
  [D55](contributors/decisions/working-notes.md) without anyone noticing; what remains is making
  the CEA-708 mapping explicit and declaring what the overlay rendition costs. Belongs with
  whichever client is being written.
- **[observability.md](contributors/design/plans/observability.md)** — a `/metrics` endpoint,
  off by default. It is a prerequisite for
  [performance-targets.md](contributors/design/plans/performance-targets.md) meaning anything
  outside CI, and the two must agree on metric names.
- **[testing-strategy.md](contributors/design/plans/testing-strategy.md)** and
  **[performance-targets.md](contributors/design/plans/performance-targets.md)** are no longer
  unspecified (2026-09-20). Neither is a phase: the corpus tiers and the regression gates are
  built alongside whatever they are gating, starting with Phase 1. The one thing with a
  deadline is the **OpenAPI breaking-change gate**, which must exist before the first client
  ships ([D85](contributors/decisions/working-notes.md)).
- **[artwork.md](contributors/design/plans/artwork.md)** — the home page is fifty posters and
  is an image-serving benchmark. In Cut A ([D178](contributors/decisions/working-notes.md)).

- **[play-statistics.md](contributors/design/plans/play-statistics.md)** *(new, 2026-09-21)* —
  the best demand-to-cost ratio the source sweep found
  ([D149](contributors/decisions/working-notes.md)). **9,196★** of companion applications exist
  to show people a history their own server already stores;
  [D81](contributors/decisions/working-notes.md)'s event log holds the data and nothing reads
  it back. A query and a page, and it belongs with Phase 2 or the first client that has room.
- **[external-lists.md](contributors/design/plans/external-lists.md)** *(new, 2026-09-21)* — a
  collection whose membership is authored elsewhere, which is a third kind alongside
  [D79](contributors/decisions/working-notes.md)'s explicit list and
  [D75](contributors/decisions/working-notes.md)'s predicate. Kometa is 3,414★ and builds from
  22 external services.

**Two things from the same sweep are not new plans and are due earlier than they look.**
**Per-role bandwidth and concurrent-stream ceilings** ([auth.md](contributors/design/plans/auth.md))
are schema and they gate the **paid relay** — a relay without them is an unbounded bill. ~~so they
are due with Phase 1's role object, not with Phase 6~~ Their columns are dropped from the first
schema and arrive with ceilings, before any relay we run ([D180](contributors/decisions/working-notes.md),
[D193](contributors/decisions/working-notes.md) §4). And **external critic/audience scores**
([library-model.md](contributors/design/plans/library-model.md),
[rules-engine.md](contributors/design/plans/rules-engine.md)) are Title-level schema with a
critic/audience split and a vote count, so they land before the model freezes.

And **[offline-download.md](contributors/design/plans/offline-download.md)** is Phase 3+ as
client work, with one part due earlier: the offline **manifest** must be able to enumerate its
own dependencies, because the font pipeline
([D53](contributors/decisions/working-notes.md)–[D58](contributors/decisions/working-notes.md))
currently assumes the server is reachable, and a downloaded film with Japanese subtitles renders
as tofu on a plane.

## Explicitly not on this roadmap

Photos (D1), DRM, a device-profile database in the core (D2/D18), an encoding ladder,
microservices, a Roku client, in-process protocol plugins, an encoder in the core binary (D19),
live TV and DVR.

Added 2026-09-21 by the source sweep ([D149](contributors/decisions/working-notes.md),
[D150](contributors/decisions/working-notes.md)): **DLNA/UPnP** — Jellyfin moved it out of its
core and the plugin now carries 84 open issues against 87 stars; **3D/stereoscopic**;
**Sonos and multi-room**; **games, sports and music videos as library types**; **poster
overlays baked into artwork**; and **a content-source plugin tier** — the one Plex killed in
2018 at *"less than 2 percent of people were using Plugins"*. And **outbound notifications**,
proposed by that sweep and withdrawn a day later
([D151](contributors/decisions/working-notes.md)): a media server has no asynchronous outcome
you are waiting on, so every state change it could announce is already a row on a screen —
measured demand was **8 reactions** against subtitles' 479.

---

## The risks, ranked

1. **The direct-play rate is lower than assumed.** Phase 0 exists to find out in days rather
   than months. Prepare ([D193](contributors/decisions/working-notes.md)) now absorbs much of the
   tail — remux, audio, and one video profile — so a low rate costs node time and first-play
   latency more than it rewrites the roadmap; live segment-addressed transcoding returns only if
   that latency is measured to hurt.
2. **Client cost swamps everything.** Clients are where every project in this field spends its
   effort, and where Plex's fifteen-year lead is real. Phase 5 is the main lever, and bringing
   it forward is the standing hedge.
3. **TV focus navigation disappoints.** No longer a bet on React Native: the framework is
   chosen by building the same four screens in Compose for TV and react-native-tvos
   ([D193](contributors/decisions/working-notes.md) §6).
4. ~~**Postgres-only costs installs.**~~ **Resolved** — re-examined and reversed to SQLite
   ([D31](contributors/decisions/working-notes.md)). The residual risk is the opposite one: if a
   genuinely multi-tenant deployment ever appears, adding Postgres means a second set of
   `sqlc`-generated queries.
5. **Codec patents.** Separate from copyright licence and not resolved by being open source.
   Direct-play-first reduces a default install's exposure, though Cut A's `h264-8m` profile
   does encode H.264 — through the hardware encoder where present; get real advice before
   anything commercial.
6. **Scope creep into audiobooks, comics, live TV, photos.** The competitive trackers show
   exactly what this costs. The roadmap's job is to keep saying no.
7. **Plugin-API speculative generality.** *Largely retired by
   [D193](contributors/decisions/working-notes.md) §3: tier 3 dissolved into the node and tier 1
   is deferred until a third-party provider is asked for.* Three tiers is already a taxonomy, and taxonomies
   invite designing for members that do not exist. Mitigation: every tier must have a real
   first-party member before it is built, and the transcoder's interface is *extracted* from a
   working implementation rather than designed ahead of one. Tier 3's second consumer is already
   identified — intro/credit detection, whose requirements match the transcoder's almost exactly
   with a completely different workload ([D20](contributors/decisions/working-notes.md)).
8. **Client/server version skew** ([D85](contributors/decisions/working-notes.md)). Jellyfin
   12.0 did **two different things** to two legacy surfaces, and the difference is the whole
   lesson: the `/emby/` and `/mediabrowser/` route prefixes are simply **gone** — a 404
   mid-operation — while `X-Emby-Authorization` was put **behind `EnableLegacyAuthorization`**,
   a flag that defaults off and is recoverable with one setting. Read on their master at
   13.0.0, 2026-09-19; the write-ups say it was removed and the source says otherwise. One is
   a break and one is a deprecation, and nothing in their process distinguishes them. Our
   exposure is larger: our clients are first-party, ship through app stores,
   run on televisions that never update, and — under
   [D2](contributors/decisions/working-notes.md) — carry decision logic that cannot be fixed
   from the server. Mitigation is a written contract and a CI diff of the generated OpenAPI
   document, both cheap and both worthless if added after the first client ships.
9. **The plugin ABI becomes a compatibility millstone.** Jellyfin plugins break every release;
   Plex killed its plugin system after seven years as unsupportable. Mitigation: a declared
   versioned interface rather than a link against internals, a host-service list short enough to
   read, and ~~our own transcoder depending on it~~ (the transcoder is now a node capability,
   D193) a real first-party member before any tier is built.
