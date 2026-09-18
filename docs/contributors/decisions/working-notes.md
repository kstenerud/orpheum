> **ABOUTME:** The append-only, D-numbered decision log — what was decided, what was rejected,
> why, and what follows. The authority other docs cite by number.

# Working Notes

All entries dated **2026-09-17** unless stated. Every decision below is **provisional** — they
are made from research, not from a running system. Each carries a **Status** line saying what
would validate or overturn it.

---

## D1 — Scope: video first, audio second, photos probably never

**Decided.** Build a video media server. Design the data model so music can be added without a
schema rewrite, and add it once video is good. Treat photos as out of scope for the
foreseeable future.

**Why.**

- Video is where the pain is. Plex's paywall and product drift have produced a large,
  motivated, actively-churning audience ([sentiment.md](../design/research/sentiment.md)), and
  Jellyfin is not catching all of it — its own users describe the client experience as "hit or
  miss".
- **Music is cheap because of OpenSubsonic.** A server that speaks the OpenSubsonic API
  inherits a mature third-party client ecosystem at zero client cost
  ([competitors-audio.md](../design/research/competitors-audio.md)). Nothing equivalent exists
  for video. This makes audio a *server-side-only* project, which is a fraction of the work.
- **Photos is a different product.** Its core job is reliable background acquisition from
  phones, which means a first-class mobile app with background upload, delta sync and
  deletion semantics — roughly the cost of the entire video project again, almost all of it in
  the client ([competitors-photos.md](../design/research/competitors-photos.md)). Immich
  already exists, is AGPL, has 114k stars, and is moving faster than we could.

**Rejected:** "one-stop server for all three from the start". It is the stated ambition, and it
is how Audiobookshelf ended up with 1,191 open issues and how PhotoPrism ended up with an
eight-year-old Postgres request. Scope discipline is visible in a tracker, and we have the
trackers to look at.

**Consequences.** The data model must not *exclude* audio or photos — generic `Library`,
`Item`, `MediaFile` shapes rather than `Movie`/`Episode` everywhere. Photos, if ever, is a
separate service sharing auth and storage, not a third library type.

**Status:** Provisional. Revisit the audio decision once video direct-play and the TV client
are solid. Revisit photos only if Immich becomes unusable or hostile.

---

## D2 — Playback negotiation: the client decides, from a manifest

**Decided.** No server-side device-capability database. The server publishes an HLS manifest
listing every rendition it can produce — including passthrough of the original — and the client
picks, using its own runtime capability probe.

**Why.** This is the highest-leverage architectural decision available and it is thoroughly
evidenced ([tech-playback.md](../design/research/tech-playback.md) §1):

- Jellyfin/Emby/Plex use static `DeviceProfile` declarations evaluated server-side. Jellyfin's
  `StreamBuilder` is 2,493 lines in a namespace literally called `Dlna`, and the resulting
  wrong decisions are the top user complaint in both directions — unnecessary transcodes and
  failed direct plays. Users are asking for manual override of an automatic decision
  (`Force "direct play" in client` +49, `Feature to force transcode` +21).
- A profile is a static claim about a dynamic fact. Only the client can ask
  `navigator.mediaCapabilities.decodingInfo()` or `MediaCodecInfo.CodecCapabilities` and learn
  not just *supported* but **smooth** and **power-efficient**.
- Kyoo has already validated the inverse design in exactly our problem space and reports a
  simpler API for it.

**Rejected:** device profiles (the incumbent design; an open-ended maintenance tax and the
largest bug class in the field).

**Consequences.** Transmuxing gets harder — the passthrough rendition must be segmented on the
same keyframe grid as the transcoded ones (D4). A bad client picks badly, so the probe-and-pick
logic lives in a shared client SDK, and the server may include a *hint*, never a mandate.

**Status:** Provisional; the strongest of these decisions. Overturned only if a major target
platform turns out to have no usable runtime capability probe — a risk since
**substantially reduced** by [D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list),
which lets a client with a poor capability API ship a static list instead of probing.

---

## D3 — Streaming format: CMAF/fMP4 segments, HLS manifest, no DRM

**Decided.** fMP4 (CMAF) segments with an HLS manifest. No DRM, ever.

**Why.** Apple's HLS authoring specification requires fMP4 for HEVC, HDR10 and Dolby Vision —
MPEG-TS cannot carry them, and TS paths are implicated in "plays for four minutes then errors"
reports. fMP4 is also DASH's container, so a DASH manifest over the same segments is nearly
free later. HLS is the most universally consumed manifest format across browsers, Apple,
Android and TV platforms.

No DRM because **the user owns the files**. Skipping FairPlay/Widevine/PlayReady removes key
servers, licence proxies and platform certification — and is the main reason a small team can
compete here at all.

**Status:** Provisional; low risk.

---

## D4 — Three playback paths: direct, on-demand segmented, background-prepared

**Decided.** (a) Direct play — a ranged GET of the original — is the default and is always
offered as a rendition. (b) On-demand, segment-addressed transcoding is the fallback, built on
the Kyoo model. (c) Background pre-preparation of a single compatibility rendition is a later
*job type*, designed for but not built first.

**Why.** Every commercial service encodes ahead of time and serves static segments; self-hosted
servers transcode live, which is why home media servers need a GPU and why "will it direct
play?" is a question users learned to care about
([commercial-streaming.md](../design/research/commercial-streaming.md)). We cannot pre-encode
everything at home, but we can stop treating live transcoding as the only answer.

**Mechanics for (b)**, verified in Kyoo's source: extract all source keyframe timestamps up
front from packet-level `ffprobe` data (not frame decoding), cache them versioned, use them as
the segment grid for *every* rendition including passthrough, force IDR frames there, start
transcodes at the requested segment rather than from zero, and share one transcode between
viewers. Known trap: **NVENC ignores `-force_key_frames` and needs `-force_idr 1`.**

**Consequences.** Audio-only renditions are first-class: many "video transcodes" are really
just a TrueHD/DTS problem, and transcoding audio while passing video through is a large cheap
win. The job system must exist early enough that (c) is an added job type, not a rewrite.

**Status:** **Refined by [D17](#d17--direct-play-first-transcoding-additive-and-optional)** —
the three paths stand, but v1 ships only (a), and (b)/(c) are re-ordered and re-scoped. Path (b)
still needs a spike against a real library before it is trusted.

---

## D5 — Database: PostgreSQL only, shipped in the default compose

**Decided.** PostgreSQL. Not SQLite. Ship it in the default Docker Compose so installation is
still one command.

**Why.** ([tech-storage.md](../design/research/tech-storage.md) §1) SQLite's single writer lock
collides directly with this workload — a library scan writing thousands of rows while
transcodes and progress updates also write. This is not theoretical: Audiobookshelf #5349 (an
unhandled `SQLITE_BUSY` during a scan took the whole server down), Jellyfin #11589 (`database
is locked` after 10.9), and Jellyfin's `"Database Locked" on LXC containers using ZFS` group
(228 comments — note that SQLite's locking is sensitive to the filesystem, and self-hosters run
ZFS, btrfs and NFS).

The asymmetry decides it: adding Postgres later is a project nobody finishes — PhotoPrism's
Postgres request has **189 reactions and is eight years old**; Audiobookshelf's has 66. Nobody
is asking for the reverse. Postgres also supplies FTS, `pg_trgm` fuzzy matching for the
scanner, JSONB, `LISTEN`/`NOTIFY` for job wakeups without a broker, and `pgvector` if ever
needed — each otherwise a separate dependency.

**Rejected:** SQLite (chosen by Navidrome, Stash and Kavita and working fine for them — the
tiebreaker is the scan-while-streaming write pattern and the ZFS/NFS deployment reality, not
throughput).

**Consequences.** All SQL behind a repository layer, so an embedded engine remains addable.
The install story must stay one command or this decision costs us users.

**Status:** **Superseded by [D31](#d31--sqlite-with-a-pure-go-driver-supersedes-d5)**
(2026-09-18). Re-examined as this status line invited: the concurrency argument does not survive
the actual write rate, most of the capability arguments evaporate, and the three cited failures
were respectively an application bug, unattributed, and a driver issue.

---

## D6 — Data access: `sqlc`, no ORM

**Decided.** Write SQL; generate typed Go with `sqlc`. A small hand-written dynamic-filter
builder (or `squirrel`) for search and smart playlists. No GORM, no Ent, no EF Core.

**Why.** The two ORM users in the survey are both cautionary. Jellyfin adopted EF Core late in
10.11 over an un-normalised schema and produced hour-long migrations plus severe slowdowns from
client-side enumeration; 12.0 is still finishing the cleanup. PhotoPrism runs the *deprecated
v1* of GORM. The two cleanest data layers in the survey (Immich's `kysely`, Kyoo's `sqlc`) both
use typed SQL rather than an ORM. `sqlc` also best matches this project's stated criterion —
strong typing with fast mechanical feedback — because a wrong column name is a build failure.

**Status:** Provisional; low risk.

---

## D7 — The files are the source of truth; the database is a rebuildable index

**Decided.** Tags and sidecars on disk are authoritative for anything derived from the media.
The database is a derived index that can be dropped and rebuilt. The exceptions — user-authored
metadata corrections and playback state — are ours, and are mirrored to sidecar files so a user
can rebuild from scratch and lose nothing.

**Why.** This is Navidrome's model and it is why Navidrome has no migration horror stories,
while Jellyfin's users now open upgrade threads with "Upgrade experiences, anyone?". It also
removes the "a migration ate my library" fear class entirely, which is a reputational event
that lasts years.

**Consequences.** Media directories are **mounted read-only by default**; writing to the media
tree is an explicit per-library opt-in. (A Jellyfin complaint on HN is that the UI says a
deletion "will be removed from Jellyfin" when it in fact deletes the file.)

**Status:** Provisional; low risk.

---

## D8 — A change journal and delta-sync cursor exist in the first schema

**Decided.** Every syncable table gets a paired audit table recording deletions (UUIDv7 primary
key, indexed timestamp), and the API serves a cursor-based "what changed since X" endpoint
including deletes, from day one.

**Why.** Immich's `*_audit` tables plus `sync.service` are exactly this, and they exist because
offline clients need deletes — an `updatedAt` column cannot express one. We will have offline
clients (download-for-travel is a non-negotiable client feature). Retrofitting a change journal
is a migration over the whole schema.

**Status:** Provisional; low risk, and cheap now versus expensive later. Note that this design
has **no consumer until a mobile client exists** — so it is unvalidated until then, and the
first offline-capable handset build is where it gets tested for real.

---

## D9 — Server language: Go, one language on the server

**Decided.** Go, with `golangci-lint` in strict mode. **One language on the server**, with a
single sanctioned future exception: an out-of-process, optional ML service in Python if photos
or content-based search ever happen.

**Why.** ([tech-backend.md](../design/research/tech-backend.md)) It satisfies both stated
constraints. Boring: three of the surveyed competitors plus Kyoo's transcoder are Go.
Strongly typed with excellent linting: `golangci-lint` is the de-facto standard and
`gofmt` ends formatting debate permanently — both directly serve "easy for agents to write
quality software", as does the 1–3s incremental build that keeps the feedback loop tight.
Fit: FFmpeg is a subprocess and Go is very good at subprocesses; goroutines plus `context`
match many simultaneous long-lived streams; a single static binary cross-compiles to the
arm64 NAS and Pi hardware self-hosters actually use.

**Rejected:** Rust (better type system, but 10–30s incremental builds tax an agent loop
thousands of times, async adds complexity to exactly our hottest code, and no prior art in this
domain — the closest call); C#/.NET (best prior art, but building in the incumbent's stack
invites building the incumbent's architecture, and we are specifically trying not to);
TypeScript/Node (best structural prior art in Immich, but weaker feedback than a real compiler,
a churning dependency graph, and a poor host for supervising many long-lived FFmpeg processes).

**Consequences.** The one-language rule is the load-bearing half. Kyoo's five-language,
two-package-manager monorepo is the thing not to copy: every extra language is another
toolchain, lint config, CI lane and set of conventions to get right.

**Status:** Provisional. Revisit only if the transcode core turns out to *be* the project, in
which case Rust's correctness starts to outweigh its build times.

---

## D10 — Clients: React Native shell, native player modules, two engines

**Decided.** A shared React Native/TypeScript UI shell with a **native player module per
platform** (Swift for Apple, Kotlin for Android, `hls.js` + `jassub` for web). Each client
supports **two engines**: the platform-native one by default, libmpv as the "play anything"
fallback, user-switchable.

**Why.** ([tech-clients.md](../design/research/tech-clients.md);
**re-examined and corrected in [client-framework.md](../design/research/client-framework.md)**)
Streamyfin ships `modules/exoplayer-player` and `modules/mpv-player` side by side, with
`MPVLayerRenderer`, `PiPController` and `MPVNowPlayingManager` on Apple.

**Two corrections to this entry as originally written**, recorded rather than quietly fixed:
Kyoo was cited as independent validation, but **Kyoo has not shipped a TV client** (its README
says Android TV is "planned") — so it validates the pattern for phone and web only, and the
TV evidence is one project, not two. And the Streamyfin star count quoted elsewhere in this
tree (302) was its companion *plugin* repo; the application has **5,197 stars**, more than
either native competitor. The engines with the best format coverage (mpv, VLC) are not
the ones with the best platform integration (AVPlayer, Media3), so a client that wants both
must ship both. Playback cannot be shared JavaScript — it needs the hardware decoder, surface
compositing, PiP, media session, HDR passthrough and audio routing.

**Order:** v1 web + Android/Android TV. v2 Apple (iOS/tvOS), desktop, Chromecast receiver.
v3 Tizen/webOS/Kodi. Roku probably never — BrightScript shares no code and its decoder set
cannot render bitmap subtitles at all.

**Risk recorded:** `react-native-tvos` is a fork of React Native, not first-party. Re-checked
2026-09-18: releases track upstream within weeks (0.86/0.87), 15 open issues, and the commit log
is largely upstream React Native core contributors because it is a thin tracking fork. Better
than "community fork" implied — though a low issue count is also consistent with few users.

**The strongest counter-signal, recorded in full because it is recent and serious:** Shopify
announced on 2026-09-10 that it is **moving from React Native back to Swift and Kotlin** (HN:
1,272 points, 958 comments), explicitly because *"LLMs changed one of the core assumptions
behind our 2020 decision"*. That reasoning is uncomfortably close to this project's own premise.

It does not overturn D10, for three reasons argued in
[client-framework.md](../design/research/client-framework.md): it is a **resourcing** argument
and Shopify has hundreds of engineers where we have one person plus agents; the thread's
best objection — that parity is a *divergence* cost accruing across edge cases "which current
agents aren't great at tracking" — **cuts against a small team maintaining parallel native
codebases**, not for it; and it is an unproven plan eight days old.

Against it stands domain evidence pointing the other way: in the Jellyfin client ecosystem the
React Native client (Streamyfin, 2024, 5,197 stars) has **more adoption than either native
client** (Findroid 4,255, Swiftfin 4,171, both 2021), while Findroid is "not optimized for TV
use yet" and Jellyfin's own native Android TV app's top issue is a playback rewrite open since
2021. Measured share ratio in Streamyfin's tree: **31% native, 69% TypeScript** across 79
screens — high for the native fraction, but the "you drop into native anyway" objection is
still wrong as a blanket claim here.

**Kotlin/Compose Multiplatform considered and rejected** (Compose went stable on iOS in 2026, so
this is newly a real option): no tvOS target and no coherent cross-platform TV story, and it
does not help the web client we need regardless — it would solve the platforms we worry about
least and skip the one we worry about most, while adding a third language.

**Status:** Provisional, **re-examined 2026-09-18 and upheld**. The risk was never performance,
the fork, or code sharing — it is **TV focus navigation**, and the mitigation is unchanged:
prototype it before writing any screens, now testable on real hardware including the
first-generation touchpad Siri Remote. What is added is a falsifiable bail-out: reverse this
decision if the focus prototype feels wrong on the oldest Fire Stick, if `react-native-tvos`
falls two minor versions behind upstream for a quarter, if our own native fraction passes
roughly half, or if Shopify's migration is reported a success with numbers in a year.

---

## D11 — Subtitles are a subsystem, not a feature

**Decided.** Extract embedded subtitle tracks and font attachments **at scan time**, not at
playback. Serve subtitles as HLS renditions and fonts as addressable attachments. Render ASS/SSA
with libass on every client (`jassub` on web, native libass elsewhere). Never burn in unless
there is no alternative. Expose subtitle offset adjustment in every player UI.

**Why.** This is the single largest defect area in the field. The most-reacted Jellyfin server
issue is subtitle desync on resume (+103, **open since 2020-03**); the top `jellyfin-web` bug is
subtitle desync; subtitles are the most frequently named pain point across the HN threads. The
workaround an HN user arrived at independently — pre-extract everything to SRT and disable
serving embedded subs — should simply be the default behaviour, because on-demand extraction
from a large MKV is exactly the "progress indicator loops forever" complaint. Burning in is
worse than it looks: it forces a video transcode of content that would otherwise direct-play.

**Status:** **Extended by [D23](#d23--axis-switching-is-mid-session-by-default-burn-in-is-the-one-thing-that-breaks-it)**
— scan-time extraction turns out to be the *enabling requirement* for mid-playback track
switching, not merely a latency optimisation, and closed captions need separate handling
entirely. Provisional; well-evidenced.

---

## D12 — Providers are sandboxed WASM plugins; no in-process plugin code

**Decided.** Metadata providers, scrapers and scrobblers run as **Extism/WebAssembly plugins**
with an explicit host-capability list, out of the server's address space. Field-level provider
ranking with recorded provenance. Provider API keys belong to the user.

**Why.** ([tech-metadata.md](../design/research/tech-metadata.md)) Navidrome demonstrates the
model — plugins in Go, Rust, Python or TypeScript, sandboxed, with host services for HTTP,
cache, KV, scheduler and artwork. The alternatives in the field are Jellyfin's in-process .NET
assemblies (a plugin can do anything the server can) and Stash's in-process JS interpreter for
user-supplied scrapers — and third-party scrapers are precisely the code you least want running
unsandboxed. Provenance matters because Jellyfin has live bugs where a refresh silently
overwrites user corrections or pulls from providers the user disabled.

Keys belong to the user because TMDB's terms make commercial use conditional on a written
agreement, TheTVDB tiers by the licensee's revenue, and OpenSubtitles budgets downloads per
account (20/day free).

**Status:** **Extended by [D15](#d15--two-plugin-tiers-in-process-wasm-and-out-of-process-adapters)**
(WASM is one of two plugin tiers). The licensing question is answered for an open-source release
by [D16](#d16--gplv3-no-nonfree-components-and-provider-keys-belong-to-the-user).

---

## D13 — No telemetry, no account, no upsell — as a stated promise

**Decided.** Zero telemetry by default and opt-in only when added. No third-party account ever
required. No storefront, no recommendations for content the user doesn't own, no upsell. Stated
in the README as a promise, not buried in a privacy policy.

**Why.** This is the emotional core of the Plex churn, and it is nearly free to deliver.
Verbatim from the threads: "Plex lost their best users by missing the point of self hosting
entirely"; "I don't want my adblocker to have to block services running on my local network";
"they were recently hacked exposing customer accounts details. their push for online accounts
is what ruled it out for me"; "i am prepared to pay them to NEVER EVER change the ui, never
shove another streaming shit in my face".

**Status:** **Amended by [D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)**
— the no-telemetry, no-ads, no-storefront, no-upsell half stands unchanged and is not
provisional. The absolute "no third-party account is ever required" does not survive contact
with NAT topology and TLS certificates, and is replaced by a falsifiable promise about *what* we
would ever charge for.

---

## D14 — Remote access for non-technical family is a headline feature

**Decided.** Treat "your parents can watch your library from their house without you
configuring their router" as a first-class product requirement with its own design, not a
deployment note.

**Why.** It is simultaneously the thing Plex users most resent (the cloud account) and the
single thing keeping them on Plex. From the threads: "the sore point right now is outside
access, which Jellyfin really isn't made for... I can't give friends and family access, either,
which was dead easy with Plex" and "Tailscale is great for me, but not feasible for family".
The self-hosted field's answer to this is uniformly "set up a VPN", which is not an answer for
the audience that matters.

Related and unsolved everywhere: **granular per-user sharing of a subset of a library.**
Jellyfin users symlink subsets into separate libraries to fake it; Immich's sharing issue has
**1,020 reactions** under a feature freeze; PhotoPrism's has 844 and is seven years old.
Whoever solves sharing wins something real.

**Status:** **Researched; see [D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)**
and [remote-access.md](../design/research/remote-access.md). The topology is settled (there is no
discoverability without a signpost) and a tiered shape is proposed. What remains open is a
product-strategy question rather than a technical one: whether this project wants to operate a
hosted service at all.

---

## D15 — Two plugin tiers: in-process WASM, and out-of-process adapters

**Decided.** Extend [D12](#d12--providers-are-sandboxed-wasm-plugins-no-in-process-plugin-code)
from one plugin mechanism to two, chosen by what the extension needs to do:

| Tier | Mechanism | For |
| --- | --- | --- |
| **In-process, sandboxed** | Extism / WebAssembly with an explicit host-capability list | Small, request-shaped work: metadata providers, scrapers, scrobblers, lyrics, filename-parser rules |
| **Out-of-process adapter** | A separate process speaking our REST API, owning its own listener | Whole protocol surfaces: the Jellyfin-compatible API, OpenSubsonic, DLNA |

**Why two.** WASM is right for "given this title, return metadata" and wrong for "serve an HTTP
API and stream gigabytes of video". Streaming large bodies across the WASM boundary is
pathological, and a protocol surface needs its own auth, its own websockets and its own route
namespace. Forcing both into one mechanism would either cripple the sandbox or bloat the host
interface until the sandbox is meaningless.

**Why the adapter tier is out-of-process rather than a compiled-in module.** Zero coupling to
the core; it can be developed, versioned, shipped and **abandoned** independently; a bug in it
cannot take the server down; and it is written against exactly the same public API a third party
would use, which keeps that API honest. Byte-streaming does not need proxying — the adapter
answers with a redirect to the core's own media URL.

**Consequences.** The core server's public REST API must be complete enough to build a full
competing protocol surface on top of it. That is a constraint, and a healthy one.

**Status:** **Refined by [D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)**
— two tiers become three, with the transcoder as the first member of the third.

---

## D16 — GPLv3, no nonfree components, and provider keys belong to the user

**Decided.** Licence the project **GPLv3**. Build and pin our own FFmpeg with `--enable-gpl`;
**never** `--enable-nonfree`, and never a patch that reclassifies a nonfree component. Use the
native AAC encoder plus Opus and `libmp3lame` rather than `libfdk-aac`. Prefer **AV1/Opus**
wherever we choose an output codec. Provider API keys are always supplied by the user.

**Why.** ([tech-licensing.md](../design/research/tech-licensing.md)) FFmpeg is a licence ladder
and the build configuration picks the rung: x264 and x265 both require `--enable-gpl`, which
makes the whole binary GPL. Jellyfin — a GPL-2.0 project — ships a **GPLv3** FFmpeg build with
`--enable-gpl --enable-version3`, so GPLv3 is where this dependency chain lands regardless;
choosing GPLv2 would create a one-way incompatibility with our own encoder.

The `libfdk-aac` exclusion is deliberate and specific. FFmpeg's own configure requires
`--enable-nonfree` for it, which produces a binary FFmpeg says may not be redistributed at all.
Jellyfin carries a patch literally named `0026-remove-fdk-aac-from-nonfree.patch` to get around
that. Whether that is legally sound is not ours to rule on — but the existence of the patch is
evidence that the boundary is real, and the audio-quality difference does not justify standing
on a contested point in a project meant for other people to redistribute.

**Codec patents are a separate question from copyright licence** and are not resolved here.
Every distribution and every project in the survey ships H.264/HEVC; the pools have
historically pursued commercial vendors. Two things follow that are ours to control: **decoding
carries less exposure than encoding** — which the direct-play-first direction (D17) reduces
almost to nothing for a default install — and **AV1/Opus are royalty-free by design**, so they
are the right default wherever we pick an output codec.

Provider keys belong to the user because TMDB conditions *commercial* use on a written
agreement and shipping our own key would make us the API consumer for every install; because
TheTVDB tiers by the licensee's revenue; and because OpenSubtitles budgets downloads per
account. It is also what [D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise)
implies anyway.

**Rejected:** AGPL (this is software people run at home, not a network service; AGPL's benefit
does not apply and its friction does), and a permissive licence (nothing stops the exact
re-close that created Jellyfin, which is this project's whole cautionary tale).

**Status:** Provisional on the licence choice; firm on nonfree components. Get real legal
advice before anything commercial ever happens, and do not answer the patent question in a
README.

---

## D17 — Direct play first; transcoding additive and optional

**Decided.** v1 ships **direct play only**. The playback API returns a list of available
renditions from day one, initially containing just the original. Transcoding is added later as
extra entries in that list, and ships as a **separable optional component**, not as part of the
core server.

**Why.**

1. **The manifest shape makes it additive.** Because the client picks from a list
   ([D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)), growing the list
   requires no client change, no protocol version and no migration. Under a device-profile
   design the transcode decision is welded into the protocol from the first release; under ours
   it is a schedule decision. This is the payoff from D2 that was not obvious when D2 was made.
2. **The common case is a ranged GET.** A modern library watched on a LAN needs no encoder at
   all. Both drivers of transcoding are exceptions:
   **compatibility** (old or odd files — a *library* problem, solvable offline, once per file)
   and **bandwidth** (remote or weak network — a *network* problem, unrelated to the file, and
   the only one that genuinely needs live adaptive segmenting).
3. **The client's engine choice is a bigger lever than the server.** A client shipping libmpv
   alongside the platform decoder direct-plays close to everything, at zero server CPU. Swiftfin
   states this as its rationale. This is already [D10](#d10--clients-react-native-shell-native-player-modules-two-engines);
   D17 makes it load-bearing.
4. **It shrinks the licence surface of the default install.** A core that only demuxes, probes,
   extracts subtitles and makes thumbnails needs no encoder — which also means no encoding, and
   encoding is where the codec-patent exposure concentrates (D16).
5. **It enforces the boundary for free.** Jellyfin's `EncodingHelper.cs` is 8,042 lines because
   nothing stopped transcoding logic from spreading through the core. A packaging boundary
   stops it mechanically.

**The unknown this rests on.** The real direct-play rate across a real library and real clients
is **not published anywhere** — Tautulli exposes it per-server but no aggregate survey exists.
This is now the first thing to measure (`plans/playback-survey.md`), because it decides how
urgent transcoding is and nothing else should be guessed on it.

**Order once measured:** compatibility transcoding first (pre-prepared background job, the
simpler problem), then bandwidth transcoding (live segmented, coupled to remote access
[D14](#d14--remote-access-for-non-technical-family-is-a-headline-feature), not to the library).

**What must exist in v1 so this stays additive:** the rendition-list API shape; a job system
that can host an encode job; a transcode *reason* field the client can display even when the
answer is "we can't"; and a capability handshake so a server without the transcoding component
can say so — which has to exist anyway for hardware too weak to transcode.

**Refined by [D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three):**
"separable optional component" is weaker than what we actually want. The transcoder is a
**plugin on the public API**, which makes the separation structural rather than a packaging
convention, and unlocks remote transcoding as a side effect.

**Rejected:** building the segmented transcoder first (the original Phase 0). It was ordered
first because it was the riskiest assumption; it is better handled by *measuring* the
assumption cheaply and deferring the build.

**Status:** Provisional, pending the playback survey. If the measured direct-play rate is low,
this decision is wrong and the transcoder returns to Phase 1. **Note how to read that
measurement:** the rate is per-client and the plan must survive the *worst* client in a
household, not the mean — an H.264-only Apple TV against a mostly-HEVC library is the
counter-example, and it sets the video transcoder's priority regardless of what the average
says.

---

## D18 — The Jellyfin-compatible surface is an adapter, and it quarantines device profiles

**Decided.** Build the Jellyfin-compatible API as an **out-of-process adapter**
([D15](#d15--two-plugin-tiers-in-process-wasm-and-out-of-process-adapters)), in this repository
but shipped separately and optional. It is explicitly a **bridge, not a foundation**: our own
clients speak our own API, the adapter is allowed to be lossy, and if it ever starts constraining
the native API it gets frozen rather than extended.

**Why an adapter and not core.** The concern that it would constrain our model to what
Jellyfin's API can express is real and is the reason it must not be core. As a separate process
it constrains nothing: it translates what it can, returns sensible failures for what it cannot,
and its limitations are visibly *its* limitations.

**The part that makes it better than a bridge.** Jellyfin clients authenticate with Jellyfin's
auth and negotiate playback with a `DeviceProfile` — the exact design D2 rejects. The adapter is
therefore where a device-profile evaluator has to live: it accepts a profile and answers it from
our rendition list, rather than from a capability database. **That puts the 8,000-line problem
permanently outside the core, in a component we are free to abandon**, while still getting
Findroid, Swiftfin, Infuse, Streamyfin and Wolphin working against our server. Navidrome does
the mirror image of this deliberately, serving a Jellyfin-compatible API to borrow Jellyfin's
music clients.

**Consequences.** Byte-streaming is handled by redirecting to the core's media URLs, not by
proxying. The adapter needs the core's public API to be complete — a healthy constraint. Under
D17 the adapter can only offer what the core can produce, so a device profile demanding a
transcode the core cannot perform gets an honest failure rather than a broken stream; this is a
correctness improvement over guessing, and it is another reason the transcode-reason field must
exist in v1.

**Status:** Provisional. Schedule it after the native API stabilises; it may reduce the urgency
of the first-party TV client, and that trade should be re-examined once it exists rather than
assumed now.

---

## D19 — The transcoder is a plugin we happen to maintain; plugin tiers become three

**Decided.** The transcoder is not "an optional component" in a packaging sense
([D17](#d17--direct-play-first-transcoding-additive-and-optional)'s weaker formulation). It is a
**first-party plugin built on the public plugin API**, with no privileged access to the core that
a third party could not also have.

This refines [D15](#d15--two-plugin-tiers-in-process-wasm-and-out-of-process-adapters) from two
tiers to three, because a transcoder fits neither existing one:

| Tier | Mechanism | Direction | Members |
| --- | --- | --- | --- |
| **1. Sandboxed extensions** | Extism / WebAssembly, explicit host-capability list | Core calls in, request-shaped | Metadata providers, scrapers, scrobblers, lyrics, filename-parser rules |
| **2. Protocol adapters** | Separate process, own listener, consumes our public REST API | **Inbound** — clients talk to it, it talks to us | Jellyfin-compatible API ([D18](#d18--the-jellyfin-compatible-surface-is-an-adapter-and-it-quarantines-device-profiles)), OpenSubsonic, DLNA |
| **3. Capability services** | Separate process, registers with the core, may serve bytes to clients directly | **Outbound** — we call it; clients may be redirected to it | **The transcoder.** Later: an ML service, if photos ever happen |

A transcoder is long-lived, stateful and streaming, so tier 1 is wrong — pushing video through
a WASM boundary is pathological. It is not a protocol surface consuming our API, so tier 2 is
wrong by direction. Tier 3 is the shape it actually has, and it has confirming prior art:
Immich's machine-learning service is exactly this, and Kyoo's `gocoder` is a standalone
transcoding service already reused by two unrelated projects (Meelo, Blee).

### Why this is better than an "optional component"

1. **It keeps the plugin API honest.** The transcoder is the most demanding extension this
   project will ever have. If it can be built on the public API with no special privileges,
   the API is proven adequate; if it cannot, we find out while we still own both sides. This is
   the library-first / first-party-consumer-as-honesty-keeper argument. The counter-example is
   in the survey: Jellyfin's plugin API is weak partly because nothing load-bearing is built on
   it, so nothing ever forced it to be good.
2. **It unlocks remote and distributed transcoding, which nobody in this field has.** If the
   transcoder registers with the core rather than being linked into it, it does not have to be
   on the same machine. The NAS serves files; the desktop with a GPU transcodes. Several
   transcoders can register and the core picks by advertised capability (this one has NVENC,
   that one has QSV).

   **The demand is proven and the absence of a seam is proven with it:** `rffmpeg` — 1,073
   stars, created 2019, still maintained in 2025, **written by a Jellyfin founder** — is an
   out-of-tree Python wrapper that impersonates the `ffmpeg` binary and SSHes to a remote host,
   because Jellyfin's architecture offers nowhere to put this. A founder resorting to an
   impersonation hack outside his own project is about as clear as architectural evidence gets.
3. **It makes the degraded path the default path.** D17 already requires a "this server cannot
   transcode" answer. With the transcoder as a plugin, that answer is what an install returns
   until one is registered — so it is exercised constantly rather than being a rarely-taken
   branch. Untested fallbacks are where bugs live.
4. **It gives the licence and patent boundary a mechanism rather than a convention.** The GPL
   encoder build and the x264/x265 linkage live in a component the user installs deliberately
   (D16). A third party can ship an alternative — a commercially-licensed encoder, a
   vendor-SDK build, a hardware appliance — without touching us, and a user in a jurisdiction
   where this matters can simply not install one.
5. **It does not conflict with [D9](#d9--server-language-go-one-language-on-the-server).** The
   one-language rule governs *our server code*, and our transcoder will be Go. A defined network
   protocol is a boundary, not a language choice; a third-party transcoder can be anything.

### The hard part, stated honestly

**Who owns the rendition list?** The core knows the file, its probe results and its keyframe
grid. The transcoder knows what it can encode on this hardware. The list the client picks from
is a *join* of those two, computed per item. That join is the whole interface, and getting it
wrong yields either a leaky abstraction or a transcoder that cannot do its job.

**Largely defused by [D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list).**
With a closed, named output menu the transcoder's capability declaration is just *the set of menu
names it can produce on this hardware*, and the join becomes a three-way set intersection —
`client ∩ transcoder ∩ {original}` — rather than an open negotiation. The remaining interface
questions below still stand.

Secondary questions the interface must answer: how the transcoder reads media (shared mount, or
the core streaming bytes to it — the former is what everyone already does, the latter is what
makes a truly remote transcoder work); how capabilities are advertised and re-probed when
hardware changes; how a transcoder failing mid-stream is reported to the client; and how
segment URLs are signed so a redirect to a transcoder does not become an open proxy.

### Sequencing — the part that keeps this from becoming speculative generality

**Do not design the plugin API up front.** Build the transcoder as a separate process against a
*private* interface, get it working, and then **promote that interface to public once it has
survived contact with real hardware.** An API designed against one imaginary consumer is how
plugin systems become bad; an API extracted from one working consumer is how they become good.

Until promotion, the transcoder is a first-party service with an unstable contract, and that is
fine — it is not shipped in the first releases anyway (D17).

**Consequences.** The default Docker Compose ships the transcoder alongside Postgres, so the
install is still one command and the boundary is still real. The core must expose probe results
and keyframe data through its public API, because the transcoder needs them and must not get
them by reaching inside. Segment URLs need signing.

**Status:** Provisional. The interface is deliberately not specified here; it is an output of
building the thing, not an input.

---

## D20 — Plugin-architecture constraints, and four things the catalogue says belong in the core

**Decided.** Constraints on the plugin design, derived from surveying what people actually
extend and work around
([plugins-and-workarounds.md](../design/research/plugins-and-workarounds.md)). These are
**seams to leave room for**, not work to start: the plugin API stays private and unstable until
a first-party consumer has proven it ([D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)).

### The constraints

1. **The plugin boundary is a declared, versioned interface — never a link against server
   internals.** Jellyfin plugins are .NET assemblies compiled against the server's types and are
   "strictly 1 version compatible"; the standing user-facing answer to "I upgraded and my plugin
   vanished" is *wait for the author*. Intro Skipper serves a **manifest that varies by
   requesting server version** and branches its git repo per Jellyfin release. That is
   infrastructure built solely to survive a self-inflicted problem, and it is the ecosystem's
   largest recurring tax. WASM plus a semver'd host interface makes it impossible by
   construction; out-of-process tiers make it impossible twice.
2. **Capability names are part of the contract, and plugins are steps triggered by events.**
   Both, from the two positive models: Navidrome declares named capabilities (`MetadataAgent`,
   `Scrobbler`, `Lyrics`, `SonicSimilarity`, `TaskWorker`, …) plus an enumerated host-service
   list; Immich registers plugin **methods** against **workflow triggers** with a compatibility
   check, so the UI offers only the plugins that fit an event. "A plugin is a step in a pipeline
   fired by an event" is a far smaller contract than "a plugin is code with access to the
   server".
3. **Host services are a short list a human can read.** Immich's is six functions
   (`searchAlbums`, `createAlbum`, `addAssetsToAlbum`, `addAssetsToAlbums`, `httpRequest`,
   `bulkTagAssets`). The security surface must be reviewable at a glance.
4. **Metadata providers must be the easiest plugin to write, and the interface must not assume
   TMDB/TVDB shapes.** Providers are the largest category by count and by stars, and the most
   popular ones are **regional** — MetaShark (2,141), Bangumi (1,134), Douban (664), Danmu (659)
   are all Chinese-market and together outweigh every official provider combined. Different
   identity models, different title semantics, different episode ordering. An interface fitted to
   TMDB will fail the people most motivated to extend it.
5. **Plugin-to-plugin composition needs a designed answer**, or reflection becomes it — which is
   literally what happened: three popular Jellyfin plugins depend on `file-transformation`, and
   its README instructs authors to "use reflection to invoke the plugin" because assemblies land
   in different .NET load contexts.
6. **A client extension seam must be designed in.** Jellyfin's plugin API is server-side only,
   and the result is `jellyfin-plugin-file-transformation` (538 stars): an HTTP middleware that
   **rewrites the web client's served JavaScript and HTML on the fly** so other plugins can
   inject UI. It originated as Jellyfin PR #9095, which was not merged. Home sections (514),
   skins (659) and danmu (659) sit on top of it. A string-rewriter over a minified bundle is a
   fragile, security-relevant thing to be load-bearing — and it exists because the alternative
   was nothing. **If we do not design this seam, someone will build that.**
7. **One first-party catalogue, with signing.** Fragmented per-author manifest URLs produced an
   aggregator plugin (257 stars). Distribution is part of the design.

### Tier 3's specification arrived on its own

The most-starred plugin in the entire field is **Intro Skipper (2,773)** — Chromaprint audio
fingerprinting to detect intros and credits. Its requirements, read from its source: raw media
file access; **a pinned FFmpeg** (its README requires jellyfin-ffmpeg ≥ 7.1.3-1 — the same
dependency our transcoder has); long-running batch compute over the whole library; **its own
persistent state with its own schema migrations**; results published into the core's model; and
a way to put a button in the client UI.

That list is nothing like a metadata provider's, which confirms D19's three tiers were not
over-taxonomy. It is also almost exactly the transcoder's list, with a completely different
workload — so **intro/credit detection should be the second tier-3 plugin we write**. Two
unlike consumers is what validates a tier rather than fitting it to one.

### What the catalogue says belongs in the core, not in a plugin

A popular plugin for a thing every user wants is a bug report against the core. Four of these
are loud enough to change the roadmap:

- **Auth: OIDC/OAuth and LDAP.** A third-party SSO plugin at **1,458 stars** plus official LDAP
  at 224, and Navidrome's two most-requested issues are the same two things. **v1 requirement**,
  not a plugin.
- **A rules engine** — smart playlists, smart collections, retention policy. Roughly **7,000
  stars** across plugins (smartlists 363, auto-collections 292, mergeversions 439, media-cleaner
  260, tmdbboxsets 176) and companion apps (Kometa 3,416, Maintainerr 2,271, Janitorr 752). This
  is a core data-model feature and it must inform `library-model.md` before the schema is
  written, not after.
- **Good search.** A Meilisearch plugin at 329 stars against a core whose `Search results are
  limited to 24.` issue has been open since 2020. Already [D5](#d5--database-postgresql-only-shipped-in-the-default-compose);
  this says it is not optional.
- **Webhooks and an event bus.** A popular official plugin (248) *and* the foundation of
  Immich's entire plugin model. It is core infrastructure that the plugin system then consumes.

### The finding that outweighs all of the above

**The community's real extension mechanism is a separate program talking to the server's HTTP
API, not a plugin.** Roughly **80,000 stars** of companion applications — Sonarr 16,293,
Radarr 14,380, Seerr 12,615, Prowlarr 7,227, Tautulli 6,585, Lidarr 5,660, Bazarr 4,279,
Kometa 3,416, Maintainerr 2,271, Janitorr 752 — against roughly 20,000 across the whole Jellyfin
plugin catalogue.

So: **the public HTTP API is the most important extension point in the product by an order of
magnitude**, and it matters more than the plugin API. This raises the stakes on generating
OpenAPI from the code and gating the build on it being current
([D9](#d9--server-language-go-one-language-on-the-server)), and it is independent confirmation of
tier 2 ([D15](#d15--two-plugin-tiers-in-process-wasm-and-out-of-process-adapters)) and of the
adapter approach ([D18](#d18--the-jellyfin-compatible-surface-is-an-adapter-and-it-quarantines-device-profiles)).

`rffmpeg` is the exception that proves it: every other companion app could be built because the
HTTP API had a seam, and remote transcoding could not, so it was built by impersonating the
`ffmpeg` binary and SSHing elsewhere. **When the API has no seam for something people want, they
do not give up — they do something worse.**

### The cautionary tale, and why it argues *for* D19

Plex sunset its plugin system in 2018, stating: fewer than **2% of users** used it; the protocol
was outdated and hard to support; and it no longer fitted the business.

- A plugin system nobody important uses is pure maintenance cost and will eventually be killed.
  **This is the strongest argument for D19**: with our own transcoder built on the public plugin
  API, that API can never be in the "<2% usage" position, because the product depends on it.
- The plugin boundary is a long-term compatibility commitment. Plex's was unsupportable seven
  years in. Keep ours small and versioned from the start.
- When a vendor's interests diverge from its users', the extension point is cut first. That is a
  governance argument for building on our own extension points in the open.

**Status:** Provisional, and deliberately not a specification. The four core-not-plugin items
are the actionable part now — auth and the rules engine both need to land in
`library-model.md` and the v1 scope before the schema is written.

---

## D21 — A fixed, named output menu, negotiated by a prioritised client list

**Decided.** The server's **output** formats are a short, fixed, project-owned **enumeration of
named profiles** — not a capability space. A client requests playback by sending a **prioritised
subset of that menu**; the server returns a manifest containing the intersection of what the
client asked for and what it can produce, always including the untouched original when it
matches. Every client must include the baseline and every transcoder must produce it, so the
intersection can never be empty.

### Why the asymmetry is real

**Inputs are unbounded** — a home library contains everything anyone has ever encoded, and
handling that is the hard, irreducible part of this project. **Outputs are not**, and it is
worth being precise about why: the output set is **already chosen for us by silicon**. Every
hardware encoder that matters supports the same three video codecs and nothing else worth
having — NVENC, QSV, AMF and VAAPI all do H.264/HEVC/AV1; VideoToolbox does H.264/HEVC.

So a broad output matrix was never available in hardware. Fixing a small menu **costs nothing we
could otherwise have had**; it only stops us writing software-encoder paths for formats no client
wants and no GPU accelerates.

### The menu

Named profiles bundling codec + profile + level + bit depth + transfer characteristics, because
those are not independently selectable in practice and naming the combination is what stops this
becoming a grammar:

- **Video:** `h264-sdr` *(mandatory baseline)*, `hevc-sdr`, `hevc-hdr10`, `av1-sdr`, `av1-hdr10`
- **Audio:** `aac-stereo` *(mandatory baseline)*, `aac-multi`, `ac3`, `eac3`, `opus`, `flac`
- **Container:** always fMP4/CMAF ([D3](#d3--streaming-format-cmaffmp4-segments-hls-manifest-no-drm)), so not a dimension.

**Resolution and bitrate are not menu entries.** They are what ABR varies *within* a chosen
profile. Conflating the two is part of what makes device profiles unmanageable.

**Dolby Vision and Atmos are deliberately absent** — passthrough-only. Direct-play them where the
client handles them; fall back to a menu profile otherwise. We never re-encode into them.

**AV1 is an entry, not a baseline.** Hardware decode reaches roughly 80% of consumer devices and
~88% of large-screen devices certified 2021–2025, but the remaining fifth is pre-2020 TVs and
older streaming sticks — precisely the long tail a home media server exists to serve.

### Why this is not a device profile in disguise

The most important thing to get right, since
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) exists to reject exactly that
design. The difference is categorical:

| | `DeviceProfile` | Output menu |
| --- | --- | --- |
| Client sends | An open-ended **description** of capabilities: containers, codecs, profiles, levels, bitrates, conditional rules | A **subset of the server's published enumeration**, in preference order |
| Server does | Evaluates **the media** against **the client's rules** to pick a play method | Intersects **two sets of names** |
| Grows by | Devices and users inventing new conditions | The project adding an entry, deliberately and under review |

The server never reasons about whether this client can play this file. It computes a set
intersection over a closed vocabulary. The precedent is boring and established: **HTTP `Accept`
with q-values**, applied to a media menu.

### The mandatory baseline makes negotiation total

Every client must list `h264-sdr` + `aac-stereo`; every transcoder must produce them. The
intersection therefore cannot be empty and negotiation cannot fail. This upgrades "a reasonable
assumption that we'll handle at least one of them" into a guarantee written into the contract,
which is worth considerably more than the assumption.

### What it fixes elsewhere

1. **Dumb clients stop being a problem.** D2's honest caveat was that the client picks, and a bad
   client picks badly. A Tizen, webOS or Roku client with a poor capability API no longer needs
   runtime probe-and-pick — it ships a static compiled-in list and is correct by construction.
2. **It largely defuses D19's hardest open question.** "Who owns the rendition list" becomes a
   three-way **set intersection** — `client ∩ transcoder ∩ {original}` — over a closed
   vocabulary, because the transcoder plugin's capability declaration is now just a set of menu
   names it can produce on this hardware. That is a far smaller interface than the open
   negotiation D19 anticipated.
3. **It makes the Jellyfin adapter easier** ([D18](#d18--the-jellyfin-compatible-surface-is-an-adapter-and-it-quarantines-device-profiles)):
   mapping an inbound `DeviceProfile` onto our menu is *classification into ~11 buckets*, not an
   evaluation engine. The quarantined complexity shrinks.
4. **The manifest never advertises something that will fail**, because it is filtered against the
   client's own list before it is built.

### What it does not fix — stated so nobody is surprised later

**Most of `EncodingHelper.cs`.** That file is 8,042 lines mainly because of the
**hardware-acceleration matrix** — per-backend filter chains, pixel-format upload/download,
tone-mapping paths — not because of output-format breadth. A fixed menu removes a real slice of
the complexity and removes it permanently, but hwaccel remains the dominant cost and is handled
separately (one interface per accelerator family, principles §5).

### The failure mode to watch

**The menu must stay an enumeration and must never become a grammar.** The pressure will be
constant and each instance will sound reasonable: "this TV needs H.264 but capped at Level 4.0";
"this one does 8-bit HEVC but only at 30fps". Each concession is a *condition*, and conditions
are what `DeviceProfile` is made of.

The discipline: the menu is owned by the project and versioned. A device needing something not on
it takes the baseline, or someone adds an entry as a deliberate reviewed change. **If we ever find
ourselves adding a condition rather than an entry, we have rebuilt the thing D2 rejected** — and
that is the review question for any change to this list.

**Status:** Provisional. The initial menu contents are a starting point to be checked against the
playback survey (`plans/playback-survey.md`), which will say what real clients actually accept.
The *shape* — closed enumeration, prioritised client subset, mandatory baseline — is the part
that matters.

---

## D22 — Video, audio and subtitles negotiate as independent axes

**Decided.** The output menu from
[D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) is **three
orthogonal axes**, not one joint vocabulary. A client sends a prioritised list per axis; the
server intersects each axis separately. Subtitles were already separate
([D11](#d11--subtitles-are-a-subsystem-not-a-feature)); this makes the split explicit and total.

### HLS already works this way

We are not inventing a mechanism. HLS declares alternate audio and subtitle renditions with
`EXT-X-MEDIA:TYPE=AUDIO|SUBTITLES` and a `GROUP-ID`, and video variants reference an audio group
by name. Every player we target — `hls.js`, ExoPlayer/Media3, AVPlayer, libmpv — handles this
natively. Separating the axes is the *conventional* shape; muxing them is the special case.

### Why it matters

1. **Additive, not multiplicative — and a joint menu would violate D21 on its own terms.**
   Five video profiles and six audio profiles are eleven names as separate axes and **thirty** as
   pairs. More importantly, a menu of pairs *is* a grammar: `hevc-hdr10+eac3` is a rule about a
   combination, and D21's stated failure mode is exactly the menu acquiring combinatorial
   structure. Orthogonal axes are what keep the enumeration an enumeration.
2. **The most common real transcode is audio-only.** Video fine, audio TrueHD or DTS-HD and the
   client cannot decode it. Joint negotiation makes that one indivisible choice and risks
   re-encoding video that was already perfect — the single loudest complaint about the incumbents
   ([sentiment.md](../design/research/sentiment.md)). Separate axes let us copy the video and
   transcode only the audio, which costs roughly one percent as much CPU.
3. **Multi-track is intrinsic to this domain.** A film commonly carries three to eight audio
   tracks — languages, commentary, descriptive audio — and users switch between them
   mid-playback. Under a joint vocabulary, changing language is a different variant and can
   restart or re-transcode the video. Jellyfin's trackers carry this as `Configurable default
   language for audio/subtitle` (+18) and `Selectable Audio and Subtitle Tracks Prior to Playback
   Start` (+18).
4. **Audio capability is dynamic in a way video capability is not.** The same device answers
   differently depending on the *audio path* — HDMI/ARC into a receiver that does E-AC-3, the
   TV's own speakers, or Bluetooth headphones — and that path changes mid-session when someone
   plugs something in. Video capability is a property of the decoder and does not move. Separate
   axes make a mid-session audio switch a cheap rendition change rather than a full
   re-negotiation.
5. **Music needs only one axis.** When the audio library arrives (Phase 7) it uses the audio axis
   and nothing else, with no null-video special case to carve out.

### The consequence that changes where code lives

**The licence boundary falls exactly between the axes.** Verified against FFmpeg's own
`configure` on 2026-09-17:

- `EXTERNAL_LIBRARY_GPL_LIST` contains **no audio encoder whatsoever** — it is `x264`, `x265`,
  `xvid`, `xavs`, `xavs2`, `davs2` plus non-codec tools (`avisynth`, `frei0r`, `libcdio`,
  `libdvdnav`, `libdvdread`, `librubberband`, `libvidstab`). Every GPL trigger is a **video**
  encoder.
- AAC, AC-3, E-AC-3, FLAC, ALAC and DCA all have **native LGPL encoders inside libavcodec**
  (`aacenc.c`, `ac3enc.c`, `eac3enc.c`, `flacenc.c`, `alacenc.c`, `dcaenc.c`) — no external
  library, no GPL rung.
- `libopus` appears on none of the GPL, version3 or nonfree lists. (`EXTERNAL_LIBRARY_NONFREE_LIST`
  is `decklink`, `libfdk_aac`, `libmpeghdec` — confirming
  [D16](#d16--gplv3-no-nonfree-components-and-provider-keys-belong-to-the-user)'s exclusion of
  `libfdk_aac`.)

**Therefore the entire audio menu is producible from a plain LGPL FFmpeg build**, and:

- **Audio transcoding lives in the core**, not in the transcoder plugin. No `--enable-gpl`, no
  x264/x265, and none of the video-codec patent exposure that §2 of
  [tech-licensing.md](../design/research/tech-licensing.md) is careful about.
- **The transcoder plugin is a *video* transcoder.** That is a smaller, sharper component than
  "the transcoder", with a clearer boundary.
- **The core therefore handles the common case and the plugin handles the rare one** — which is a
  much better split than the one D17 assumed. A default install with no plugin can already fix
  the TrueHD-on-a-cheap-TV problem, which is most of what real users hit.

This refines [D16](#d16--gplv3-no-nonfree-components-and-provider-keys-belong-to-the-user),
[D17](#d17--direct-play-first-transcoding-additive-and-optional) and
[D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three) without
overturning any of them.

**Qualified by [D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp):**
audio encoding stays in the core *distribution* — packaging follows the licence — but it does
not run in the core *process*. The trust boundary does not follow the licence boundary, and all
FFmpeg work runs in a confined worker.

### Mechanics

- **Audio segmentation is easier than video, not harder.** Audio frames are independently
  decodable, so audio segments can always be cut on the video keyframe grid that
  [D4](#d4--three-playback-paths-direct-on-demand-segmented-background-prepared) already
  computes. Known bounded wrinkle: audio frame granularity does not divide arbitrary timestamps
  evenly (AAC is 1024 samples, ~21.3 ms at 48 kHz), so boundaries drift slightly; this is a
  standard, solved problem, not a design risk.
- **Whole-file direct play is unaffected and remains the v1 happy path** (D17). A ranged `GET` of
  the untouched container satisfies all three axes at once and bypasses HLS entirely.
- **Per-axis passthrough is a remux, not an encode** — copying a video track into fMP4 segments
  needs a muxer and no encoder, so it also belongs in the core.
- **Subtitles are the third axis** and were already modelled that way (D11): separate renditions,
  fonts as addressable attachments, never burned in by default.

### The risk to measure, not assert

Some older TV platforms are reported to handle **demuxed HLS** (separate audio group) less
reliably than muxed audio+video. That would be an argument for offering a muxed variant as a
compatibility fallback — a fourth thing the menu would have to express, which is unwelcome.

**This is not asserted here because it is not verified.** It is added to
`plans/playback-survey.md` as a question for real devices. If it proves true on a platform we
care about, the fallback is a muxed variant flagged on the video axis, and that concession must
be reviewed against D21's no-grammar rule.

**Status:** Provisional. The axis split and the LGPL/GPL boundary are solid; the demuxed-HLS
compatibility question is open and **now directly measurable** — several generations of Fire TV
Stick and an old Apple TV are exactly the older-firmware population the concern is about. The
dynamic-audio-path claim is likewise testable, and a **handset is the right device for it**,
where route changes mid-playback are routine rather than occasional
([test-hardware.md](../design/research/test-hardware.md)).

---

## D23 — Axis switching is mid-session by default; burn-in is the one thing that breaks it

**Decided.** Changing the subtitle track — turning captions on, switching language — and changing
the audio track are **normal mid-playback operations**, not session restarts. This is promoted to
an invariant over [D22](#d22--video-audio-and-subtitles-negotiate-as-independent-axes):

> **No operation on one axis may require restarting or re-negotiating another.**

There is exactly one thing that breaks it, and it is named and labelled rather than forbidden:
subtitle burn-in.

### What makes mid-session switching work

HLS declares every alternate rendition in the master playlist up front, so a client switches by
fetching different segments — no new session, no server round-trip beyond the fetch. That works
**only if every track is enumerated and already materialised when playback starts.**

Which means **[D11](#d11--subtitles-are-a-subsystem-not-a-feature)'s scan-time extraction is not
an optimisation — it is the enabling requirement for this.** If a user turns subtitles on forty
minutes into a film and the server only then starts demuxing a 60 GB remux to produce them, that
is precisely the complaint recorded in
[sentiment.md](../design/research/sentiment.md): "I will just add manually - can't, progress
indicator loops forever." The workaround an HN user arrived at independently — pre-extract
everything, disable serving embedded subs — is the correct default, and this decision is why.

Corollary: the subtitle axis is negotiated **once, at session start**. The client declares what it
can *render*; the manifest carries every matching track; the client then switches freely.

### Burn-in collapses the subtitle axis into the video axis

This is the sharpest available statement of why burn-in is a last resort, and it is stronger than
D11's original "it forces a video transcode":

If subtitles are burned into the picture, then turning them off, or changing language, requires a
**different video stream**. The axes are no longer independent, and the invariant above is
violated.

So burn-in is not a subtitle rendition at all — **it is a video rendition with the subtitle
selection baked in**, and it must be labelled as such in the manifest so a client can tell the
user that choosing it locks their subtitle choice for the session. It is offered only when a
client can render neither text nor bitmap subtitles and the content has no text alternative.

### The axis is logical; the transport differs by format

Worth stating explicitly, because "subtitles are an HLS rendition" is only true for one of the
four cases. Verified against FFmpeg's `hlsenc.c` and `libavcodec` on 2026-09-17:

| Format | Transport | Notes |
| --- | --- | --- |
| **WebVTT** | In-manifest HLS subtitle rendition | FFmpeg's HLS muxer uses the WebVTT muxer for subtitle variant streams. This is the mandatory baseline. |
| **ASS/SSA** | **Out-of-band sidecar + font attachments**, rendered by the client's libass | HLS has no ASS rendition type. Never silently convert ASS to WebVTT — it discards styling, positioning, karaoke and typesetting, which for anime *is* the subtitle. Offer a converted WebVTT as a separate, clearly degraded entry for clients without libass. |
| **PGS / VobSub** | Out-of-band bitmap, composited by the client | Burn-in only if the client can do neither. Note Roku cannot render bitmap subtitles at all — the one platform where burn-in is unavoidable, and a reason it ranks poorly in [tech-clients.md](../design/research/tech-clients.md). |
| **CEA-608/708** | **Not a track at all** — see below | HLS signals these with `EXT-X-MEDIA:TYPE=CLOSED-CAPTIONS`, distinct from `TYPE=SUBTITLES`. |

### Closed captions are a trap, and "turn on CC" is exactly where it springs

CEA-608/708 captions are **carried inside the video elementary stream** as SEI side data, not as a
separate track. Two consequences that no amount of subtitle-track handling will cover:

1. **Transcoding the video destroys them** unless side data is explicitly carried through
   (FFmpeg provides `vf_ccrepack` and A53 side-data handling for exactly this). Remuxing preserves
   them. So a broadcast recording or OTA rip that direct-plays with working captions can lose
   them the moment a bandwidth transcode kicks in — a bug that appears only under load, on only
   some content, which is the worst kind.
2. **They cannot be pre-extracted by demuxing**, because there is nothing to demux. They must be
   *decoded* out of the video stream (FFmpeg's `ccaption_dec`).

**Decision:** at scan time, detect embedded captions and **extract them into a real subtitle
track**, so they become an ordinary switchable rendition like everything else. Separately,
**preserve the in-stream captions on remux, and require the video transcoder plugin to carry
them through as a conformance condition** — a transcoder that silently drops captions fails its
tests, not the user's evening.

This decouples "turn on CC" from whether the video happens to be transcoding, which is the whole
point.

### Track semantics that have to be modelled, not bolted on

Each of these is a live complaint on an incumbent's tracker, and each is a property of the data
model rather than of the player:

- **Forced subtitles.** The subset shown only for foreign dialogue, flagged in the container
  (`AV_DISPOSITION_FORCED`). A different user intention from "subtitles on": forced tracks should
  auto-enable when the audio language matches the user's, and must be a flag on the track rather
  than a separate concept.
- **Default and preferred track selection, persisted.** Per user, and ideally per series: setting
  Japanese audio with English subtitles on episode 1 should carry to episode 2. Jellyfin carries
  this as `Configurable default language for audio/subtitle` (+18) and `Subtitle default option
  per user` (+12).
- **Per-track offset/sync adjustment**, which must survive a track switch. Requested on every
  client tracker in the survey.
- **Sidecars added after the scan.** A `.srt` dropped next to the film later must be picked up by
  the watcher and appear on next play at the latest.
- **Overlapping and multi-part tracks**, which Jellyfin carries as `Stack subtitles with
  overlapping timestamps` (+15).

**Status:** Provisional. The closed-caption extraction-plus-passthrough requirement is the part
most likely to be got wrong quietly, so it belongs in the playback survey's corpus and in the
transcoder's conformance tests rather than in a checklist somewhere.

---

## D24 — Language is a preference axis, and track selections stick to the series

**Decided.** Clients send a prioritised **language preference** alongside their capability lists.
An **explicit** track choice by the user is remembered as a re-resolvable descriptor and applied
to the rest of the series until they choose again.

### Language is a preference, not a capability — keep them apart

[D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) and
[D22](#d22--video-audio-and-subtitles-negotiate-as-independent-axes) negotiate **capability**.
Language is categorically different and must not be merged into the same list:

| | Capability (codec profiles) | Language |
| --- | --- | --- |
| Nature | **Hard constraint** — the client physically cannot decode it | **Soft preference** — the client just wants it |
| Effect | *Filters* what the server may produce | *Orders* what already exists |
| On no match | Must not happen; a mandatory baseline guarantees it | Falls back down a chain; never fails |
| Scoped to | The **device** (and its current audio path) | The **user** |

The last row decides where state lives: capability is per-device and travels in the request;
language preference is per-user, device-independent, and therefore **server-side state that
syncs** ([D8](#d8--a-change-journal-and-delta-sync-cursor-exist-in-the-first-schema)). Setting
Japanese audio on the TV must be honoured by the tablet.

Pleasingly, the whole negotiation remains ordinary HTTP content negotiation: `Accept` with
q-values for the menu, **`Accept-Language` with q-values for language**. The boring, established
mechanism already has a header for this axis.

### Language matching is fiddlier than it looks — verified, not assumed

- **ISO 639-2 has two competing 3-letter codes for the same language**, bibliographic and
  terminological: `ger`/`deu`, `fre`/`fra`, `chi`/`zho`, `dut`/`nld`, `gre`/`ell`, `ice`/`isl`.
  Media containers use 639-2 and are inconsistent about which; BCP 47 uses 639-1. This is real
  enough that FFmpeg ships a **695-entry conversion table** (`libavformat/avlanguage.c`) and an
  explicit `AVLangCodespace` enum covering `ISO639_2_BIBL`, `ISO639_2_TERM` and `ISO639_1`.
  **Normalise every track's tag to BCP 47 at scan time, and keep the original** for provenance.
- **Script subtags are load-bearing**, above all for Chinese: `zh-Hans` and `zh-Hant` are not
  interchangeable for a reader.
- **Region subtags matter less but still matter**: `pt-BR` vs `pt-PT`, `es-419` vs `es-ES`. Use
  RFC 4647 basic filtering — prefix match on subtag boundaries — so a preference for `pt` matches
  `pt-BR`, but `pt-PT` does not match `pt-BR`.
- **Untagged and `und` tracks are common.** Resolution order: exact tag → language-only match →
  `und` → the file's own `DEFAULT` disposition → first track of that type.

### Language alone is not enough: remember (language, kind)

A track tagged `eng` may be the full dialogue, SDH, forced signs-and-songs, a director's
commentary, or audio description. Remembering "English" and re-applying it next episode is
therefore wrong often enough to be a bug.

FFmpeg's dispositions give the vocabulary, verified present in `avformat.h`:
`FORCED`, `HEARING_IMPAIRED`, `VISUAL_IMPAIRED`, `COMMENT`, `DESCRIPTIONS`, `CAPTIONS`, `DUB`,
`ORIGINAL`, `KARAOKE`, `DEFAULT`.

Real files set these inconsistently or not at all, so the **scanner** must also derive kind from
the track title — "English (SDH)", "Signs & Songs", "Commentary by the director" — and record
which source it used. That is scanner work
([scanner.md](../design/plans/scanner.md)), not player work.

### The rule that makes stickiness correct: remember the criteria, not the selection

**Never remember a track index.** Track 3 in episode 1 is a different language in episode 2. This
is a real and common bug class in existing systems.

Store a **descriptor** — `{language, kind}`, or the explicit value `off` — and **re-resolve it
against each new episode's actual tracks**. Three rules follow, each of which is easy to get
wrong:

1. **Fallback never overwrites the preference.** If episode 5 has no Japanese track and we fall
   back to English, episode 6 must still try Japanese first. A fallback is not a choice.
2. **Only an explicit user action updates the sticky preference.** Automatic selection never
   does, or the preference drifts silently toward whatever the files happen to contain.
3. **`off` is a choice, distinct from unset.** Turning subtitles off on episode 1 must turn them
   off on episode 2. The stored state is tri-valued: `off` / unset / `{language, kind}`.

### The resolution chain, most specific first

```
explicit choice in this session
  → remembered choice for this series          ← the user's ask
    → per-library default                      ← "anime: ja audio + en subs"
      → user's global language preference
        → the forced-subtitle rule (below)
          → the file's own DEFAULT disposition
            → first track of that type
```

**Per-library defaults earn their place**: an anime library wants Japanese audio with English
subtitles while a Western TV library wants English audio and no subtitles, and expressing that
once per library avoids doing it once per series.

**Sticky scope** is the nearest grouping ancestor: the series for an episode; the item itself for
a film. Collections are the obvious generalisation and are deliberately not built now.

### The forced-subtitle default rule, stated concretely

This is the sensible starting point of the chain and connects to
[D23](#d23--axis-switching-is-mid-session-by-default-burn-in-is-the-one-thing-that-breaks-it):

- Chosen audio language **≠** the user's primary language → enable **full** subtitles in the
  primary language.
- Chosen audio language **=** the user's primary language → enable **forced-only** subtitles
  (signs and songs) if such a track exists, otherwise none.

It is what people expect, and everything above it in the chain can override it.

### Two consequences for the data model

- **The subtitle axis selection is a set, not a single value.** Usually empty or one, sometimes
  two — dual subtitles for language learners is a real want (Jellyfin's `Dual Subtitles` carries
  +33). Allowing a set now is free; retrofitting it is a migration.
- **These preferences are user data and go in the change journal** (D8), so they reach every
  device and survive a rebuild ([D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index)
  — they are ours, not derived from the files, so they mirror to sidecars too).

### Say why, not just what

Per principles §3, the playback response reports what was pre-selected **and the reason**:
"English subtitles — you chose them for this series", versus "— your account default", versus
"— the file marks this track default". It costs almost nothing and it removes the entire "why
did it do that?" class of confusion. No incumbent does this.

### Evidence

Jellyfin's trackers: `Configurable default language for audio/subtitle` (+18), `Subtitle default
option per user` (+12), `Selectable Audio and Subtitles Tracks Prior to Playback Start, **and
Context Awareness**` (+18) — that last title is asking for precisely this decision — and `Dual
Subtitles` (+33).

**Status:** Provisional. The language-normalisation table and kind-inference heuristics need a
real-world corpus in the scanner's test suite; both are the kind of thing that is only ever
right against real files.

---

## D25 — Remote access is tiered, and the promise changes from "no accounts" to "we only charge for another machine"

**Decided.** Supersedes the absolute form of
[D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise) and gives
[D14](#d14--remote-access-for-non-technical-family-is-a-headline-feature) a shape.

**The topology fact is not negotiable.** Two machines behind different NATs cannot find each
other without a third party — neither knows its own public address as seen from outside and
neither can be contacted first. Tailscale's own data path makes this explicit: **every**
connection begins via a DERP relay to exchange endpoints, and only then attempts a direct path
in parallel. Direct connection is an optimisation over a rendezvous, never a replacement for
one. ([remote-access.md](../design/research/remote-access.md))

**And the collision starts earlier than remote access — it starts with HTTPS.** A home server
cannot get a publicly-trusted certificate for itself: no domain, and ACME will not issue for a
bare IP in any way a normal user can drive. Plex solved this by operating `plex.direct`,
resolving `1-2-3-4.<hash>.plex.direct` to `1.2.3.4`, and issuing each server a wildcard cert for
`*.<hash>.plex.direct`. **So a user who never leaves their LAN and never shares with anyone
still needs a third party to get warning-free HTTPS.** D13 was written thinking about remote
access; the certificate problem reaches it first.

### Three tiers

| Tier | What it is | Who runs it | Cost |
| --- | --- | --- | --- |
| **Local** | LAN access, self-signed or user-supplied certificates, server-local accounts, capability links | The user | Free, forever, no account |
| **Self-hosted assist** | The user runs their own rendezvous, relay, certificate story and IdP — **our software, their machine, documented** | The user | Free |
| **Hosted assist** | We run rendezvous, relay, `*.<hash>.<ourdomain>` certificates and a cross-server identity directory | Us | Subscription |

**The middle tier is load-bearing and is not optional.** If a user can self-host the signpost,
the paid tier is a convenience rather than a gate and the promise below is true. If it is
theatre, the promise is a lie. It also costs little, because the hosted tier runs the same
software.

### The restated promise

> **We charge only for things that genuinely require another machine.** Everything that can run
> on your own hardware does, for free, forever. No feature that works locally will ever be moved
> behind a payment. No telemetry, no ads, no storefront, no upsell in the UI.

This is better than "no accounts" because it is *falsifiable* and it names the exact category —
rendezvous, relay, certificates, cross-server identity — where a third party is a law of physics
rather than a business decision. It is also the precise inversion of the grievance against Plex,
which took a feature that already worked and paywalled it.

### The economics work, and the reason is the 90%

Signalling is nearly free. Relay carries video and is the expensive part — a two-hour film at
20 Mbps is about 18 GB — but at commodity European prices (Hetzner: roughly €1/TB beyond a large
included allowance) that is around **two euro-cents per relayed film**. Hyperscaler egress is
30–75× that and indie-PaaS egress is disqualifying, so **the provider choice matters more than
the architecture**. Above all, Tailscale's >90% direct-connection rate means relay is an
exception path rather than the service. A modest subscription covers this comfortably; the
binding constraint is abuse, not cost.

### Identity: sharing is the same question

Sharing with someone requires a stable identifier for that person that both ends agree on, and
that identifier has to live somewhere. **That is why Plex has an account system** — not because
Plex wanted a login. Every open competitor that skipped the account also skipped sharing:
Immich's sharing issue has 1,020 reactions under a feature freeze, PhotoPrism's 844 over seven
years, and Jellyfin users symlink subsets into fake libraries.

Four identity options, not exclusive, and all four are the design: **server-local accounts**
(always available, no identity across servers), **capability URLs** (unguessable scoped links;
excellent for one item, poor for ongoing access because they cannot be attributed or revoked per
person), **bring-your-own OIDC** (identity exists, we operate nothing, needs configuration the
target user cannot do), and **our optional directory** (maximum convenience, the premium tier,
and exactly what D13 promised not to require).

**Rejected:** keeping "no third-party account is ever required" as an absolute. It is keepable
only by abandoning the non-technical-family case, which is the single largest reason people stay
on Plex. Quietly weakening it later would be worse than restating it now.

**Consequences.** The software must **degrade to tier 1 rather than break** if a hosted service
disappears, with a documented and *tested* exit path — the audience for this project was burned
by exactly this kind of dependency. Signed, scoped, expiring media URLs become load-bearing
(also required by D19 and D26). Abuse handling must be designed before anything is operated.

**Status:** Provisional and the most consequential open question left. The *technical* half is
unusually cheap to settle: a handset on mobile data is a real symmetric-CGNAT client, and the
Proxmox hosts cover the server side, so every row of the mechanism table can be measured rather
than argued. **Whether this project wants to operate a hosted service at all is not a technical
decision** — it implies a company,
uptime obligations, liability and an entity that can be pressured. Declining it is coherent;
the cost is that the headline use case stays unsolved. Tiers 1 and 2 are worth building either
way, and building them first keeps the option open.

---

## D26 — No FFmpeg in the core process; confine it with Landlock + seccomp

**Decided.** **No FFmpeg invocation ever happens in the core server process.** Probe, demux,
subtitle extraction, thumbnailing and audio encoding all run in a confined **media worker**
child — the same shape as the video transcoder from
[D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three), using
the same interface. The default confinement is **Landlock + seccomp-bpf**.

**Why: assume compromise, not prevention.** Every file in the library is attacker-supplied data
parsed by a large C codebase. FFmpeg has roughly **494 published CVE records since 2005**, with
recent years in the mid-20s to high-30s annually, dominated by memory-safety failures across
demuxers, decoders, protocol handlers and parsers. Two things make 2026 different: **discovery
is accelerating because LLM-driven analysis is now pointed at it** — published 2026 work
describes an AI agent finding 21 FFmpeg zero-days, and separate AI-assisted analysis finding
seven memory-safety flaws traditional tools missed — and **our typical user's library contains
files from places that are not a shop.** This is the same assumption every browser vendor makes
about their own media stack, which is why Chrome and Firefox sandbox codecs.

### The finding that changes the architecture

**The trust boundary does not follow the licence boundary.**
[D22](#d22--video-audio-and-subtitles-negotiate-as-independent-axes) put audio encoding in the
core because the GPL rung falls between video and audio. But probe, demux, subtitle extraction,
thumbnailing *and* audio encoding all parse hostile input, and all of them were going to run in
the core process.

Separate the two concerns:

- **Packaging follows the licence** — audio encoding ships in the core distribution, video
  encoding as the separately-installed plugin. D16 and D22 unchanged.
- **Process follows trust** — all FFmpeg work runs in the confined worker.

The core then **never parses attacker-controlled bytes at all**: it handles HTTP, SQL and JSON
and delegates every encounter with a media file. That is a far stronger claim than "we sandboxed
the transcoder", it unifies D19's worker interface with the trust boundary, and it costs a
process boundary we were already building.

### Why Landlock + seccomp rather than containers

The constraint was to pick a stance that does not consume the development budget. Modern Linux
provides **Landlock** (filesystem and network access control, kernel 5.13+) and **seccomp-bpf**
(syscall filtering) as *unprivileged, per-process* mechanisms requiring **no root, no namespaces
and no cgroups** — a process can be confined about as tightly as a container with no container
runtime installed. Maintained Go bindings exist for both (`landlock-lsm/go-landlock`,
`elastic/go-seccomp-bpf`).

The policy is small enough to audit: read one input file, write one cache directory, and — for
probe, subtitle and thumbnail work — **no network at all**. A decoder compromised under that
confinement can corrupt one cache directory.

Stronger rungs stay **opt-in**: a dedicated user plus systemd hardening (older kernels), a
container per invocation, a microVM. The strongest rung is already free — D19 lets the
transcoder run on another machine entirely, which is the "locked-down box that gets wiped"
option.

**Consequences.** Confinement capability must be **reported, not assumed**: the user can see
whether their install is confined, because Landlock needs kernel 5.13+ and macOS and Windows
need different mechanisms. The confinement needs an actual escape test in
[`testing-strategy.md`](../design/plans/testing-strategy.md) — a sandbox nobody verified is a
comment. And a **hardened FFmpeg build** that disables decoders nobody needs is free at build
time; the playback survey's format census tells us which those are.

**Status:** Provisional. The per-platform confinement matrix is the open part.

---

## D27 — Change detection is a strategy interface that reports which strategy it is using

**Decided.** Library change detection is a per-root strategy with an explicit capability report:
native filesystem watching where it works, a NAS-side index agent where offered, mtime-delta or
full walking otherwise, and an explicit rescan trigger always.

**Why.** `inotify` does not work over SMB or NFS — how a large share of self-hosters mount media
— has watch limits a big library exhausts, and **fails silently**, which is the worst property a
detection mechanism can have. "The scanner did not notice my new file" and "the scanner
rescanned everything at 3 a.m. and pegged the CPU" are the same bug seen from two sides.

**The reporting half is the important half.** The server says which strategy is active per
library root and warns when the chosen one is degraded or stale. The field's current failure is
that watching silently is not happening and nobody knows.

**The NAS index agent** — an agent on the NAS watching locally, maintaining an index file on the
share, with our server watching one file — is the only approach that genuinely solves the
network-share case, and it is architecturally free: it is a **tier-3 capability service** under
D19, the second independent case for that tier after the transcoder, which is good evidence the
tier is real rather than fitted to one consumer.

Its transport is the elegant part: **no network protocol, no port, no authentication, no
firewall rule** — both sides already have the share mounted. Its costs are real and recorded:
consumer NAS platforms are closed and architecturally fragmented so adoption will be a minority;
a stale or dead agent reproduces the exact silent-failure we are removing, so it must heartbeat
and the server must surface staleness; and the index needs write-temp-then-atomic-rename plus a
checksum so a reader never trusts a torn file.

**The load-bearing corollary: periodic walking must be good enough on its own**, because most
installs will use it. That makes its efficiency a requirement with a number in
[`performance-targets.md`](../design/plans/performance-targets.md), not a fallback afterthought.
If the product is unusable without the agent, the agent is a trap.

**Status:** Provisional. The agent is worth building later and with real demand — note that on
the author's own hardware both NAS units run Debian/Proxmox, so the server runs *on* the NAS and
native watching works; this solves other people's problem.

---

## D28 — Keep the exit from React Native cheap, and designate the fallbacks

**Decided.** [D10](#d10--clients-react-native-shell-native-player-modules-two-engines) stands,
and three rules preserve the ability to leave it. The fallback is **not** "go fully native" —
it depends on how React Native would fail, and the options are named in advance.

### The fallbacks, by failure mode

| How RN fails | Fallback | Why |
| --- | --- | --- |
| **TV only** — the likely case: focus navigation or performance on weak sticks | **Hybrid** — keep RN for web and phone, go native for TV (Compose for TV on Android, SwiftUI on tvOS) | RN's weak spot is exactly and only TV. Isolating the fallback to the platform that failed is far cheaper than a wholesale migration, and a TV UI is a small surface: rows, a grid, a detail page, a player. |
| **Generally** — the fork stalls or the ecosystem decays | **KMP for shared logic + native UI on every platform** | A proven path with real adoption. Web still needs its own client, unless Kotlin/JS or Kotlin/Wasm supplies its logic layer — possible, less-trodden. |
| We wanted maximum sharing | Compose Multiplatform | **Rejected: no tvOS target.** It solves the platforms we worry about least. |

**KMP is the designated general-purpose Plan B**, and the earlier dismissal of it in D10
conflated two different things. **KMP** (share logic, native UI everywhere) is conservative and
widely adopted — professional-developer usage roughly doubled in a year, 7% → 18%; production
users include Cash App, Forbes, McDonald's, Quizlet and Block's Bitkey; Google and JetBrains are
both invested, and Google being the vendor of one of the two mobile platforms is the part that
matters. **CMP** (share the UI too, via Skia) is a different and newer bet, and is the one that
fails our requirements.

**One claim qualified, because it is the one that looks most relevant and is not:** Netflix's
KMP usage is in its **Studio** applications — internal production tooling — not the consumer
streaming client. It is not evidence for KMP in consumer media playback.

**Flutter is rejected as a fallback**, which is worth stating because it is the more popular
framework overall: **no official tvOS support** (the request has been open since 2019), and
Android TV support that Flutter's own umbrella issue describes as having gaps in "tooling, I/O,
navigation, focus, core design language, documentation, samples, and tests". It is worse than
React Native for our target set. Note in passing that **webOS is now officially supported** by
an LG-shipped Flutter SDK, which is the one place Flutter leads — relevant only if the v3
webOS target is ever built, where **Lightning.js** is the stronger option anyway.

### Why the exit is cheap, and the three rules that keep it that way

**The expensive asset is already framework-independent.** The native player modules — player
engine, renderer, PiP, media session, libmpv and ExoPlayer integration — are plain Swift and
Kotlin behind a thin bridge. Streamyfin's 33,097 lines of them are not React Native code.
Migrating means dropping the wrapper and keeping the implementation. Combined with a generated
OpenAPI contract ([D9](#d9--server-language-go-one-language-on-the-server)), a closed rendition
menu ([D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list)) and a
small probe-and-pick interface
([D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)), **a framework switch is a
UI rewrite, not a system rewrite.**

That property is worth preserving deliberately rather than by luck:

1. **Native player modules carry no React Native types at their core.** Engine, renderer, PiP
   and media-session code are plain Swift/Kotlin; only a thin adapter knows about RN.
2. **The shared SDK stays framework-agnostic TypeScript** — no React or React Native imports in
   the API client, negotiation or offline logic. It must be usable from a plain web page, and
   the device-lab web harness ([device-lab.md](../design/plans/device-lab.md)) is the forcing
   function that proves it continuously.
3. **Track the native fraction.** Streamyfin sits at 31%; passing roughly half is one of D10's
   bail-out triggers and is only observable if measured.

None of these costs anything today. All three stop being available if ignored for a year.

**Status:** Provisional. **The open question is now closed by
[D29](#d29--a-native-client-reimplements-the-cheapest-cross-platform-logic-is-server-side-logic):**
a native TV client reimplements, and it is cheap because most of "the shared SDK" is generated,
server-side, or inherently platform-specific — and a TV needs no offline download, which is the
only genuinely hard shareable piece.

---

## D29 — A native client reimplements; the cheapest cross-platform logic is server-side logic

**Decided.** Closes the question
[D28](#d28--keep-the-exit-from-react-native-cheap-and-designate-the-fallbacks) left open. If the
hybrid fallback is taken and the TV client goes native, it **reimplements** — no bridge, no
shared core, no fourth language. And the reason it is cheap becomes a standing design rule.

### Why: "the shared SDK" is much less than it sounds

Decomposing it ([shared-core.md](../design/research/shared-core.md)) deflates the problem:

| Part | Status |
| --- | --- |
| API client | **Generated**, not shared — `openapi-generator` emits Kotlin, KMP and Swift from the same document ([D9](#d9--server-language-go-one-language-on-the-server)). Regenerating is not porting. |
| Capability probe | **Cannot be shared by nature** — `MediaCodecList`, `decodingInfo()`, VideoToolbox. Nothing is lost. |
| Rendition negotiation | **Mostly already server-side** — [D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) made it a set intersection over ~11 names, performed by the server. The client's share is tens of lines. |
| Language preference | **Server-side** ([D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)). |
| Queue / resume | Shareable, moderate. |
| **Offline download** | **The one genuinely hard shareable piece.** |

**And the clincher: a TV does not go anywhere, so a TV client needs no offline download.** The
single piece of logic that would justify an exotic sharing mechanism is a *phone* concern, and
the hybrid fallback keeps phones on React Native.

This is not luck. D9's generate-the-client rule and D21's closed-enumeration negotiation made
the reimplementation small, and both were adopted for unrelated reasons.

### The mechanisms, including one verified dead

- **Embedding a JS engine** (Hermes standalone, QuickJS, JavaScriptCore) — **rejected.** It
  reintroduces the bridge the migration was escaping, ships another runtime, and on iOS
  JavaScriptCore outside a `WKWebView` cannot JIT. Only justified for a large shared core, which
  we do not have.
- **A Go core via `gomobile`** — **verified dead for us**, and it was the conceptually most
  attractive option because D9 already makes the server Go. From `cmd/gomobile/env.go`:
  `applePlatforms = []string{"ios", "iossimulator", "macos", "maccatalyst"}`. **No tvOS.** It
  fails on precisely the platform the fallback exists for. (Otherwise healthier than its
  reputation: maintained through September 2026, 37 open issues.)
- **A Rust core via UniFFI** — the strongest option on capability, and the only one that is not
  a bet on how the client question resolves: Swift (so iOS *and* tvOS), Kotlin and Kotlin
  Multiplatform, **Wasm for the web**, and **React Native Turbo Modules via Hermes bindings** —
  so it would serve the current architecture *and* every fallback. Used extensively by Mozilla
  in Firefox. **Not adopted**, because it is a fourth language against D9's one-language
  discipline, bought to hedge a failure that may not happen, for an offline state machine not
  yet written. Speculative generality.
- **A KMP core** — already D28's general-purpose Plan B. Larger adoption and a first-party
  vendor; weaker web story. Versus Rust/UniFFI, that trade should be re-run **with the offline
  module actually written**, not now.

### The durable rule this produces

> **The cheapest cross-platform logic is logic that runs on the server.**

Every piece of logic moved server-side needs porting zero times instead of three. This is
available to us in a way it is not available to most mobile applications, **because we ship the
server** — and it is the general form of what D21 and D24 already did by instinct.

Four things keep the §1 decomposition true, and it is a property of decisions rather than a law:

1. **The OpenAPI document stays the source of the API client** — generated, CI-gated, never
   hand-maintained. A hand-written client accumulating logic is how porting cost appears.
2. **Negotiation stays a closed enumeration intersected server-side.** D21's no-grammar rule now
   has a second reason behind it: conditions creeping into the menu grow the client's share and
   therefore the reimplementation cost.
3. **Offline logic is quarantined in its own module.** It is the one genuinely portable asset and
   is only portable if separable.
4. **Anything that can live on the server does.**

**Status:** Provisional. Revisit only if React Native fails generally rather than on TV — at
which point the KMP-versus-Rust/UniFFI comparison is a real decision with real inputs.

---

## D30 — tvOS is cheap from inside the Apple stack, and never has a web fallback

**Decided.** Two standing rules and a re-pricing, from noticing that tvOS was the consistent
failure point across every client-technology evaluation in this tree
([tvos.md](../design/research/tvos.md)).

### Why every cross-platform option failed on the same platform

Three structural reasons, and none is "the vendors haven't got around to it":

1. **There is no web runtime.** tvOS has **no WebKit, no `WKWebView`, no browser** — the classes
   to fetch a page exist, nothing to render one does. This kills Capacitor, Ionic, Lightning.js,
   Tizen/webOS-style HTML apps and the Jellyfin-style wrap-the-web-UI shell, in one stroke.
2. **The Focus Engine needs real UIKit views.** tvOS navigation is a platform-owned traversal
   over the view hierarchy, not an input-event stream the app interprets. **A canvas renderer
   presents no focusable views** — Flutter and Compose Multiplatform both paint through Skia
   into one surface, so the platform's navigation model has nothing to attach to. React Native
   works precisely because it maps to real UIKit views; `react-native-tvos` carries UIFocus
   handling in its Fabric component views plus `TVFocusEventHandler`/`TVEventHandler`.
3. **It is a whole engine port for ~8% of the market.** Flutter's maintainers list "an entirely
   new embedder in the engine, as well as test harnesses" plus buildroot/Dart/Skia changes, with
   wireless debugging as a prerequisite **because tvOS has no USB tether** — and label it
   **P3**. tvOS is about 8% of US primary streaming viewing, tied with Tizen, against Roku ~37%,
   Fire TV ~17%, Google TV ~14%.

Apple also **deprecated TVMLKit at WWDC 2024** — its own JavaScript/XML framework for TV apps —
directing developers to SwiftUI. Apple tried the scripted-TV-app path and retired it; no
blessed non-native route is coming.

### The measurement that re-prices the fallback

From the Swiftfin tree — the Jellyfin Apple client, 4,171 stars:

| Target | Lines | Files | Views |
| --- | ---: | ---: | ---: |
| `Shared/` | **59,018** | 604 | 89 |
| `Swiftfin` (iOS) | 12,361 | 116 | 93 |
| `Swiftfin tvOS` | **2,382** | **22** | **5** |

**~80% shared between iOS and tvOS; the tvOS-specific portion is ~3.2%.** So:

> **tvOS is expensive to reach from outside the Apple stack, and nearly free from inside it.**

The difficulty is not a property of the platform; it is a property of reaching it from a
non-Apple toolchain.

### The consequences

1. **[D28](#d28--keep-the-exit-from-react-native-cheap-and-designate-the-fallbacks)'s hybrid
   fallback is cheaper than it was priced.** Going native on Apple buys **iOS *and* tvOS for
   roughly the cost of iOS plus 3%**. That makes
   [D10](#d10--clients-react-native-shell-native-player-modules-two-engines) a safer bet, not a
   riskier one.
2. **The question to ask is "do we ever write Swift?", not "which framework reaches tvOS".** If
   yes, tvOS is nearly free. If no, it is a wall.
3. **Standing rule: never plan a tvOS story whose contingency is the web client.** The
   ship-the-web-UI panic button exists on every other platform we target — Tizen and webOS apps
   *are* web apps, Android TV has WebView — and does not exist here. Any plan that quietly
   assumes it is already broken.
4. **Standing test for any future client framework: canvas or platform views?** Canvas
   renderers will be absent or bad on tvOS; view-mapping frameworks will work. This predicted
   all six evaluations in this tree and costs nothing to apply.
5. **Skipping tvOS stays legitimate.** Spotify said in 2016 not to expect an Apple TV app "any
   time soon" and shipped one in **October 2025** — nine years — with no visible consequence;
   Immich and Kyoo have no TV client at all. But for *our* audience there is a counter-signal:
   the most-praised client in [sentiment.md](../design/research/sentiment.md) is **Infuse, which
   is Apple-only**, and HN threads repeatedly pair it with an Apple TV. The 8% figure probably
   understates the enthusiast segment this project is aimed at.

**Status:** Provisional. Rules 3 and 4 are the durable part and cost nothing. The re-pricing in
rule 1 should be re-checked against our own code if the hybrid is ever taken — Swiftfin's 3.2%
is one project's ratio, not a law.

---

## D31 — SQLite with a pure-Go driver; supersedes D5

**Decided.** **Supersedes [D5](#d5--database-postgresql-only-shipped-in-the-default-compose).**
The database is **SQLite**, in WAL mode, via a **pure-Go driver**. Postgres is not shipped, not
required, and not supported for now.

D5's own status line named it "the one most worth re-examining", so this follows that note
rather than reversing on a whim. Full working in
[sqlite-reconsidered.md](../design/research/sqlite-reconsidered.md).

### D5's evidence was partly misread

- **Audiobookshelf #5349** — a scan hit `SQLITE_BUSY` "and that rejection was **unhandled** —
  which took down the entire server." An **application bug**, not a SQLite limit. `SQLITE_BUSY`
  is the documented signal to wait and retry.
- **Jellyfin's lock complaints** — real pain, but **unattributed**. They arrived with the EF
  Core migration whose other symptom was transactions held open far too long. I presented
  contention as SQLite's semantics when it may have been Jellyfin's usage.
- **"core dump without SSE4.1"** — a **native-library** failure, which is a driver argument and
  disappears with pure Go (below).

### The numbers do not support the concurrency worry

The metric is writers, not users. Twenty concurrent streams at one progress update per ten
seconds is about **two writes per second**; SQLite in WAL mode does thousands of batched writes
per second. Three orders of magnitude of headroom. The worry was contention, and contention is
a design question — one **Navidrome has already solved in production** with Go, SQLite, a media
library and a scanner:

- `_busy_timeout=15000&_journal_mode=WAL&_foreign_keys=on&synchronous=normal`, pool sized
  `max(4, runtime.NumCPU())` because more connections buy no write parallelism;
- and the sharp edge worth stealing: **`SQLITE_BUSY` and `SQLITE_BUSY_SNAPSHOT` share the
  message "database is locked", and `busy_timeout` can retry the first and never the second** —
  a stale snapshot can only be recovered by rolling back and retrying the whole transaction.
  Inspect the extended result code to tell them apart.

### Most of D5's capability arguments evaporate

SQLite has excellent **FTS5** and has **JSONB**. `LISTEN`/`NOTIFY` is only needed *because*
Postgres is a separate process — in one binary it is a channel, which argues for SQLite.
`pgvector` is moot because photos are out of scope ([D1](#d1--scope-video-first-audio-second-photos-probably-never)).
Only **`pg_trgm`** is a genuine loss, and filename→title fuzzy matching belongs in the
identification component in Go anyway.

### The driver is not a detail — cgo would break a D9 promise

`mattn/go-sqlite3` is the most used, but **cgo breaks trivial cross-compilation**, which is one
of [D9](#d9--server-language-go-one-language-on-the-server)'s stated reasons for choosing Go,
and it reintroduces the native-crash class. So: **pure Go**, choosing between
`modernc.org/sqlite` (v1.59.0, actively released) and `ncruces/go-sqlite3` **by benchmark on the
low-end target**, not by reputation.

### What this buys, which D5 under-weighted

One binary and no second service — a large part of the product for self-hosted software.
**Backup becomes copying a file**, which closes a gap our own
[backup-restore.md](../design/plans/backup-restore.md) had flagged as an unpaid cost of D5. No
Postgres major-version upgrade dance, against "upgrades must be boring" being one of the
strongest findings in [sentiment.md](../design/research/sentiment.md). And RAM back on the 8 GB
Celeron that [test-hardware.md](../design/research/test-hardware.md) calls the honest low end.

### The multi-process objection dissolves, and pleasingly

[D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three) and
[D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp) put workers in
separate processes, possibly on separate machines — two processes writing one SQLite file over a
share would be a serious mistake. But those workers **must not touch the database**: they take a
job and return results over the core's API, and the core stays the sole writer. That is exactly
what D26's threat model wants anyway, since a confined process parsing hostile media should not
hold database credentials. **The architecture that makes the security story good is what makes
SQLite viable.**

### Conditions and remaining risks

1. **Refuse to run with the database on a network filesystem.** SQLite locking over NFS/SMB is
   unsafe; detect at startup and fail with a clear message rather than corrupt quietly.
2. **Retry-on-`BUSY_SNAPSHOT` lives in the repository layer**, once.
3. **Single-writer discipline**: serialise writes through one path; batch scanner writes into
   transactions.
4. **Adding Postgres later is not free** — `sqlc` emits dialect-specific code — but it is
   bounded because every query lives in the repository layer regardless.

**Rejected:** supporting both engines. That doubles the generated query surface and the test
matrix to serve a deployment shape that does not exist yet.

**Status:** Provisional. Postgres remains right for a genuinely multi-tenant or multi-node
deployment; if that ever materialises it arrives with a repository layer in place and a real
requirement to design against, which beats paying the operational cost now for a workload that
does not exist.

---

## D32 — Being told beats watching; the no-agent path is a tiered, measured, self-reporting walk

**Decided.** Refines [D27](#d27--change-detection-is-a-strategy-interface-that-reports-which-strategy-it-is-using)
by correcting its ordering and specifying the path for users who cannot or will not install
anything on their NAS — which is most of them.

### The correction: build the inbound endpoint first

D27 treated an explicit rescan trigger as a last-resort safety valve. That is backwards.
**Nothing that polls or watches beats the component that wrote the file telling us it did**, and
for this audience that component usually exists: the companion-app survey found roughly 80,000
stars of `*arr` ecosystem, and **Sonarr and Radarr already emit webhooks on "On File Import",
"On File Upgrade" and "On Import Complete", with paths in the payload.**

So the first thing built is **an inbound endpoint** — `POST` "this path changed", plus a CLI
command and a UI button on the same path. A few dozen lines, instant, zero running cost, and for
a large share of real libraries it is the entire feature. Polling becomes the fallback for
manually-managed content rather than the primary mechanism.

### The ladder

**A** be told (webhook/API/CLI) → **B** native watching (`inotify`/`FSEvents`/`kqueue`, local
only) → **C** SMB `CHANGE_NOTIFY` (deferred, see below) → **D** directory-mtime pruned walk →
**E** full `stat` walk as the safety net → **F** content hashing, on demand only.

The NAS index agent (D27) sits between B and D for those who will install it.

### Tier D is the workhorse, and it suits this domain specifically

A directory's mtime moves when an entry is added or removed, not when a file is edited in place.
For most workloads that makes mtime pruning unreliable; for a **media library it is close to
ideal, because libraries are append-mostly** — files are added, removed or replaced wholesale
and essentially never edited. The walk collapses from *O(all files)* to *O(changed
directories)*.

Caveats, each requiring tier E behind it: `rsync -t` and `mv` can **preserve** mtimes; in-place
modification is invisible at directory level; and some SMB/NFS configurations do not update
directory mtimes dependably.

### Optimise round trips, not bytes

Network-walk cost is **one round trip per operation**. At 1 ms RTT, stat-ing every file in a
100,000-file library is minutes; over Wi-Fi far worse. Therefore:

- **Never `stat` per file.** SMB2 and NFS carry attributes in the directory listing, so a
  correct walk costs one round trip *per directory*. In Go that means `filepath.WalkDir` and not
  calling `Info()` where the entry already has what is needed — an easy and expensive mistake.
- **Slice the walk** so it never blocks for minutes or presents as a stall.
- **Adaptive per-directory cadence.** Libraries are extremely skewed — a downloads folder
  changes daily, a 2004 films folder never will. Hot directories get checked often, cold ones
  fall to a slow cycle. A global interval is either too slow for the hot corner or too expensive
  for the cold mass.
- **Let the user declare a library archival or hot.** An explicit statement beats any heuristic
  and costs one config field.

### Probe at setup, and report the question the user actually has

Make D27's capability report measured rather than descriptive, when a library is added:

1. Write a temp file and check whether the parent directory's mtime moved — **a direct test of
   whether tier D can be trusted on this share**.
2. Time a listing of a few hundred entries to derive a per-directory cost and an **estimated
   deep-scan duration**.
3. Report protocol, chosen tier, estimated cost and planned cadence.

And extend the reporting requirement: under polling the user's real question is not "which
strategy" but **"when did you last actually look?"** Surface **last successful deep scan and
current staleness**, per library.

### SMB `CHANGE_NOTIFY` — deferred, with a trigger

SMB2 has a real change-notification primitive that would give instant, agent-free detection on
SMB shares. Two blockers, checked 2026-09-18:

- **No Go client implements it.** `CloudSoda/go-smb2` has the command constant and its
  `STATUS_NOTIFY_ENUM_DIR` error handling but **no request struct, no completion-filter
  constants and no public API** — a placeholder.
- **An OS `cifs` mount cannot expose it**, since the kernel does not surface SMB change
  notifications to userspace.

NFS offers no equivalent worth relying on (none in v3; v4.1 directory delegations are rarely
enabled).

**The contained design if it is ever warranted:** keep the OS mount for reading media — faster,
page cache, `sendfile` — and open a **separate minimal SMB session used only for
`CHANGE_NOTIFY`** on the library roots. That needs connect, authenticate, tree-connect,
open-directory and change-notify, not a full filesystem client. A real differentiator, since no
incumbent does it. **Trigger:** tiered polling proving inadequate for SMB users in practice,
*and* agent adoption being as low as expected.

**Status:** Provisional. The inbound endpoint and tier D are v1; everything else is ordering.
The `*arr` webhook integration should be documented as a first-class setup step, not buried —
it is the difference between instant and a poll interval for most of this audience.
