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

> **Amended 2026-09-19 by [D98](#d98--a-sidecar-carries-what-is-true-about-the-work-the-database-carries-what-is-true-about-a-person).**
> "Mirrored to sidecars" applies only to what is true about the *work*; per-user state is
> database-only, because the media tree is shared. And the mirror itself needs an **opt-in
> writable path** that [principles §2](../principles/README.md) does not grant by default —
> so the durability promise is kept by the automatic backup
> ([D77](#d77--back-up-with-vacuum-into-back-up-only-the-irreplaceable-and-test-the-restore))
> and the *portability* promise is what the sidecar delivers.

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
  — they are ours, not derived from the files). ~~*so they mirror to sidecars too*~~ —
  **superseded 2026-09-19 by [D98](#d98--a-sidecar-carries-what-is-true-about-the-work-the-database-carries-what-is-true-about-a-person)**:
  these are per-user and the media tree is shared, so they are database-only and
  backup-critical. Nothing else in this decision changes.

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

---

## D33 — Hub-and-spoke: a core that never touches media, and N media nodes that do

**Decided.** The core does not scan, probe, or read media files. That work happens in a **media
node** — the same binary in a different mode — which runs wherever the files are. One core, N
nodes, whatever transport the topology allows. Design in
[media-nodes.md](../design/media-nodes.md).

### The consolidation is the reason to do it

The proposal was a standalone scanner binary. Followed through, **the scanner, the media worker
and the transcoder all want the same three things** — to be next to the files, to have FFmpeg,
and to be confined without database credentials. They are one component:

> A **media node** does everything that requires access to a media file: discovery, probing,
> subtitle and font extraction, thumbnails and trickplay, and optionally transcoding.

That merges [D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)'s
tier-3 capability service, [D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)'s
media worker and [D27](#d27--change-detection-is-a-strategy-interface-that-reports-which-strategy-it-is-using)'s
scanner into one deployable thing.

**It also completes D26 instead of half-satisfying it.** That decision stopped FFmpeg running in
the core but left the core walking user filesystems, where directory entries are also
attacker-influenced. The core's entire input surface is now: HTTP requests, its own SQLite file,
and node reports validated at the boundary.

### One artifact, two roles — non-negotiable

`mediaserver serve` and `mediaserver node --core=<url>`: **the same binary**, and in the default
single-machine install **the core supervises a co-located node as a child process**. The user
installs one thing and runs one thing. Distribution is an opt-in topology change, not the
baseline. Anything else would undo [D31](#d31--sqlite-with-a-pure-go-driver-supersedes-d5)'s
single-file simplicity, which was a large part of why SQLite won.

### It simplifies D27 rather than adding to it

D27 had the *core* selecting between four change-detection strategies per root, **plus** a
separate NAS agent — the same job implemented twice, with the distributed path second-class.

Now there is **one scanner implementation**, normally *local to its files*, so it uses
`inotify`/`FSEvents`/`kqueue` and the network-filesystem watching problem largely evaporates. It
survives only for users who will not install a node next to the files, and for them the node
runs on the core box and walks the mount using [D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk)'s
mtime-pruned strategy. **Same binary, same code, degraded mode.** D27's "NAS agent" stops being
a distinct concept — it is a node that happens to be on a NAS.

### Transport is pluggable, deliberately

HTTP POST node→core; core polling a node endpoint (for nodes that cannot dial out); **a report
file on a shared mount** (no port, no protocol, no auth, no firewall rule — the elegant option
when a share already exists); or a poke via signal file or API, composing with D32's inbound
endpoint that the `*arr` stack can already drive. The report *content* is one contract; how it
travels is deployment detail.

### Do not copy Kyoo's coupling

Kyoo already runs its scanner as a separate service — validating the "core doesn't scan" half —
but that scanner holds a **direct Postgres connection and its own migrations** alongside calling
the API. That violates D31's single-writer discipline and hands database credentials to the
component handling hostile files, which is precisely backwards from D26. **Nodes talk to the
core's API and nothing else; the core is the only writer.**

### The traps, and the one that matters most

1. **A dead node is not deleted media.** The failure mode that destroys someone's library. An
   offline node's items become **unavailable, never gone**: last-seen timestamps, explicit node
   state, and tombstoning **only** on a positive deletion report from a live node — never on
   silence.
2. **A node's report is attacker-influenced data** (paths and probe output derive from hostile
   files). Validate at the boundary; treat a node as a semi-trusted peer, not part of the core.
3. **Overlapping roots** — two nodes seeing the same share must dedupe, not double-count.
4. **Clock skew** across nodes if mtimes are compared.
5. **Idempotency and ordering** — a replayed or late report must not corrupt state.
6. **Version skew.** One artifact, but two running versions in distributed deployments. The
   protocol version and compatibility policy must be real, not aspirational.

### What it unlocks later at no cost

In a true multi-machine topology the core may have no media access at all, so it cannot serve
direct play. D19 already built **signed, scoped, expiring URLs with client redirection** for the
remote transcoder; the same mechanism lets a **node serve its own files** with the core issuing
the redirect. Not needed for v1 — pointless indirection on one machine — but "the core never
touches media, full stop" is reachable without new machinery.

**Status:** Provisional, and the most structurally significant decision since
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest). The report contract is the
real design work and is not specified here. Trap 1 deserves a test before the feature is
believed.

---

## D34 — A node status record that distinguishes dead from wedged from idle

**Decided.** Every media node maintains a small JSON **status record**, separate from its scan
report, that the core reads to reason about the node's health. Detail in
[media-nodes.md](../design/media-nodes.md) §The status record. This specifies the liveness half
of [D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do)'s
contract, and is what makes its trap 1 enforceable rather than aspirational.

### A heartbeat alone cannot detect the failure that matters

Three states share a fresh heartbeat and only one of them is fine:

| | Pulse | Progress | Verdict |
| --- | --- | --- | --- |
| Working | fresh | advancing | fine |
| **Idle** | fresh | static | **fine — nothing to do** |
| Dead | stale | static | process gone |
| **Wedged** | **fresh** | **static** | **alive, achieving nothing** |

So the record carries **three independent things**, not one:

- **`state`** — what the node believes it is doing (`idle`, `scanning`, `probing`,
  `transcoding`, `error`). This is what separates *idle* from *wedged*: static progress is
  correct when idle and a fault when scanning.
- **A liveness pulse**, updated on a timer regardless of work.
- **A progress signal**, updated *only* on completed work, at a granularity fine enough that a
  sensible timeout exists — per directory or per item, never per scan, since a large library
  legitimately takes an hour.

### Compare counters, not timestamps

Timestamps in the record are **for humans**. The core must not compare a node's clock to its
own — clock skew, NTP corrections and VM suspends all break that. The node emits a **monotonic
counter** with each signal, and the core stamps *its own* observation time when it first sees a
new counter value. Staleness is then measured entirely on the core's clock against a
monotonically increasing value.

### Why a file beats an endpoint here

1. **It survives the process.** An API dies with the node; a file still says what the node was
   doing when it stopped — which is precisely what post-mortem needs.
2. **It distinguishes three failure modes where an API gives one.** Stale counter → node died.
   File absent → never ran, or wrong path. Share unreachable → transport problem. An unanswered
   request cannot tell these apart.
3. **It needs no port, protocol or credentials** over the shared-mount transport.
4. **A human can `cat` it** — a cheap answer to "logging around library scanning is difficult to
   interpret", which is a direct complaint about Jellyfin in
   [sentiment.md](../design/research/sentiment.md).

### Requirements that make it work rather than half-work

- **Stable `node_id`, generated once and persisted.** A fresh id per restart makes the core see
  a new node every reboot while the old one goes silent — straight into the dead-node trap. One
  file per node id so nodes sharing a directory never clobber.
- **Atomic writes**: temp, `fsync`, rename in-directory, plus a length or checksum so a reader
  never trusts a torn file — and readers handle torn reads anyway, because SMB caching surprises.
- **Write on state transition *and* on a timer**, always populating `last_error`, so a node that
  dies mid-scan leaves behind what it was doing.
- **Status is not the report.** Two files, two cadences — small and frequent versus large and
  occasional. Conflating them makes the cheap thing expensive.
- `roots[].last_full_scan` lives here, which is where
  [D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk)'s
  "when did you last actually look?" reporting requirement naturally belongs — the node knows,
  so the node reports it.

### What the core does with it

Surface **dead and wedged differently**, because the remedies differ: dead means start it,
wedged means look at `state` and progress detail to see what it is stuck on. Mark an absent
node's items **unavailable, never deleted**. **Never tombstone on silence** — absence of a pulse
is absence of information, not evidence of deletion — and never act on a wedged node's reports,
which are by definition not current.

**Status:** Provisional. The thresholds (how stale is wedged, per state) need real measurements
from a large library before they are fixed, and D33's trap 1 deserves a test that kills a node
mid-scan and asserts nothing is deleted.

---

## D35 — Node exchange directory: progress pair, config generation, and permissions as the trust boundary

**Decided.** Extends [D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)
into the full node↔core contract: a task-defined progress pair, a bidirectional control channel
through the same directory, and a directory layout whose permissions *are* the security model.
Detail in [media-nodes.md](../design/media-nodes.md).

### Progress is a moving pair; either half moving means alive

Progress is **task-defined** — files, directories, bytes, items enriched — expressed as
`current` and `end`, where **`end` is an estimate allowed to move**. A scan's denominator grows
as directories are discovered, and that growth is itself evidence of life. The rule: **if either
number moves, the node is working.** Wedged is both static, for long enough, while `state` is
not `idle`.

Four things it needs to be usable rather than merely present:

- **A `unit` label**, or the UI cannot render it.
- **`end: null` is legal** — some tasks have no knowable end, and then only `current` signals.
- **An `epoch` that increments on task restart.** Without it, a reset to zero is
  indistinguishable from being stuck at zero, and a fresh scan gets called wedged.
- `current` monotonic within an epoch.

### The correction: idle must still tick

The progress pair alone is **not sufficient**. An idle node changes nothing; a dead node also
changes nothing. Watching only progress collapses the two into one state — which is exactly the
distinction D34 exists to preserve.

So the record keeps a **pulse that advances unconditionally, including when idle**. Progress
answers *"is it achieving anything?"*; the pulse answers *"is it there at all?"*. Dropping
either loses a state.

### Clock skew dissolves, and the formulation is simpler than D34's

D34 specified a monotonic counter compared against the core's observation time. The cleaner
statement: **the core times its own probes.** It reads the state file at `T1` and `T2` on its
own clock and compares contents; if any monotonic field changed, the node lived in that window.
Node timestamps become purely **decorative** — a `node_clock` field is worth keeping for
diagnostics ("your NAS clock is four hours out") but must **never** be an input to liveness
logic. This supersedes D34's framing without changing its conclusion.

### Bidirectional control through the same directory

The core writes `config.json`; the node watches and re-reads on change. Scan now, change the
interval, pause, rescan one root.

- **Acknowledgement is the part easily left out.** The config carries a **`generation`** the
  core increments on every change; the node echoes the last applied value as
  **`observed_config`** in its state file. Desired-state / observed-state, which is what makes
  the channel reliable over a shared directory that offers no acknowledgement of its own.
- **The node must poll the config file as well as watch it**, because the exchange directory may
  be on the share — exactly where watching does not work. One small file; the cost is nothing
  and it removes a silent failure.
- **Where HTTP exists, prefer it.** The file channel is the zero-configuration fallback; "scan
  now" in particular already has a better route through
  [D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk)'s
  inbound endpoint.

### The exchange directory, and why its permissions are the security model

```
<exchange-dir>/<node-id>/
  config.json   ← core writes, node reads + watches
  state.json    ← node writes, core reads
  reports/      ← node writes, core reads   (large)
  log/          ← node writes, core reads   (rotated)
```

**Namespaced by node id**, since several nodes may share one location; a duplicate `node_id`
from a copied config must be detected and reported, not silently interleaved. **Reports and logs
need a retention policy from day one** — an exchange directory that fills a share is a memorable
way to break someone's NAS.

The permissions split is the strongest part of this, because it matches the trust model exactly.
The node handles hostile media ([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)),
so it is the less-trusted half, and a careful user can enforce that with filesystem permissions
alone: core writes only `config.json`, node writes only its own outputs, media roots read-only.

**And that same table is the node's Landlock ruleset.** The directory layout and the sandbox
policy are one statement, which is good evidence both are right.

### Degraded modes, stated rather than discovered

- **Core cannot write the exchange directory** → no bidirectional control; the node runs on its
  own defaults and the core reads state only. The core must *say so* rather than silently
  failing to apply settings.
- **Node cannot write anywhere shared** → use HTTP, or run the node on the core's machine
  pointed at a mount ([D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do)'s
  degraded mode).
- **Read-only for both** → the user hand-writes `config.json` and the core gets no liveness.
  Everything works; nothing is observable. Supported, and warned about.

When installed as a daemon, the only thing in `/etc` is the path to this directory.

**Status:** Provisional. Wedged thresholds still need measurement against a real large library,
and the report retention policy is unspecified.

---

## D36 — Idle ticks the progress pair; the node declares its own tick contract

**Decided.** Drops the separate pulse field from
[D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle) /
[D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary).
**An idle node ticks the progress pair itself** — enter idle at `current=0, end=1`, then
`1/2`, `2/3`, and so on. One mechanism, and the core's liveness rule becomes uniform: *did
either number move?*, with no branch on state.

### Why the simplification is right

Idle was the only state where the pair could not move, and it is also the only state where
ticking it **tells no lie** — there is no task whose completion the numbers could
misrepresent. So idle can satisfy the liveness contract using the same field as everything
else, and the separate pulse was carrying exactly one case.

The UI does not suffer: it already branches on `state` to choose its wording, so it simply does
not render a bar when `state == idle`.

### But a uniform *rule* is not a uniform *threshold* — and that is the real trap

Removing the pulse removes the state-dependent *mechanism*. It does not remove the
state-dependent *timing*, and assuming it does would make the core call healthy nodes wedged.

Progress rates differ by orders of magnitude between tasks. An idle node can tick every few
seconds. A node probing a 60 GB remux over a slow link may legitimately produce no movement for
minutes — and mid-task it **cannot** bump the numbers to prove liveness without lying about
completion, which is precisely the honesty that makes idle-ticking acceptable.

So the node **declares its own contract**: a field such as `max_tick_interval_s`, meaning *"I
guarantee at least one of these numbers moves this often, in my current state."* Short when
idle, longer during a slow probe, set by the component that actually knows.

The core's rule is then genuinely uniform **and** correct:

> Wedged when neither `current`, `end` nor `epoch` has changed for more than N × the node's
> declared interval — whatever state it is in.

One rule, one comparison, no per-state tuning in the core, and no lying in the node. That is
better than either the separate pulse or a fixed global threshold.

### Details that still hold

- **`epoch` covers restarts.** A node that restarts goes from `847/848` back to `0/1` —
  numbers moving *downward*. The core must read that as movement, and the incremented epoch
  from D35 is what makes it unambiguous rather than a suspected stall.
- Monotonic `current` within an epoch, unaffected.
- Unbounded growth is a non-issue: ticking every few seconds for a year is a rounding error.
- `state` remains the field that tells the UI and the human *what* the movement means.

### What is lost, honestly

A fixed-interval pulse would have let the core estimate node clock drift or spot process
pauses. Minor, and `node_clock` is diagnostic-only anyway
([D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)),
so nothing depended on it.

**Status:** Provisional. `max_tick_interval_s` per state, and the multiplier N, both need
measurement against a real large library over a real share before they are fixed — the failure
mode of getting them wrong is calling a healthy slow node wedged, which is worse than a late
detection.

---

## D37 — The core judges liveness from observation, not from a node-declared interval

**Decided.** Drops `max_tick_interval_s` from
[D36](#d36--idle-ticks-the-progress-pair-the-node-declares-its-own-tick-contract). The node
reports **facts**; the core does the **judging**, from its own observations, on a graded scale.

### Three reasons, and the third is the one that settles it

1. **The core is better placed to know the cadence.** A declared interval is the node author
   guessing once, at build time, about an environment they cannot see — this NAS, this link,
   this library, this hardware. The core has something better: **history**. It has been watching
   this node, in this state, on this deployment, and can hold an empirical baseline of actual
   inter-movement intervals. An observed distribution adapts; a constant cannot.

2. **A single missed check is not an event.** D36 stated one threshold, which reads as a cliff.
   Most lateness is a big directory or a slow file. See the graded model below.

3. **A wedged node is not a reliable source of its own health thresholds.** This is the one that
   settles it. `max_tick_interval_s` asks the component *being judged* to supply the standard it
   is judged against — and the whole point of this machinery is detecting a node that has
   stopped behaving correctly. Self-reported health criteria are a smell, and the node that most
   needs catching is the one least able to report accurately.

### What the node does instead: be more specific about state

The node's real contribution is not a number, it is precision about **what it is doing**.
Prefer finer-grained states — `enumerating`, `probing`, `hashing`, `extracting`, `transcoding`
— over a single coarse `scanning`.

This is strictly better than a declared interval because it solves the same problem *and* two
others:

- **It gives the core a clean signal to learn against.** A coarse `scanning` that silently
  switches from fast enumeration to slow per-file probing produces a bimodal, useless baseline.
  Split states produce clean ones.
- **It covers the "about to go quiet" case** without a numeric field: `state: "hashing"`,
  `detail: "bigfile.mkv (61 GB)"` tells the core to widen its expectation *and* tells a human
  exactly what is happening.
- **It is legible.** Someone reading the file learns more from `probing` than from a number.

### Graded escalation, with hysteresis, and nothing destructive

| Level | Trigger | Action |
| --- | --- | --- |
| **Healthy** | Moving within the observed baseline | none |
| **Late** | Past expectation, briefly | **Nothing.** Perhaps a subtle UI hint. Most lateness is legitimate. |
| **Stalled** | Persistently late across consecutive observations | Surface in the UI with `state` and `detail`; stop treating its data as fresh |
| **Presumed dead** | No movement for a long multiple | Mark that node's items **unavailable** |

- **Hysteresis in both directions.** Require consecutive observations to escalate, and a clean
  run to recover, so the indicator does not flap.
- **The core chooses its own polling interval**, so "a missed check" is a core-side concept and
  can be made small in absolute time without making escalation twitchy.
- **No level is destructive.** Even *presumed dead* only marks items unavailable — tombstoning
  still requires a positive deletion report from a live, progressing node
  ([D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) trap 1,
  [D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)).

That last point is why the precision D36 was chasing was never needed: **the cost of guessing
wrong is a UI warning, not data loss.** Engineering exactness into a mechanism whose worst
outcome is a misleading status badge was over-design.

### Cold start

No history on first contact, so the core uses a conservative built-in default and refines it by
observation. Being wrong early is cheap, per the above.

**Status:** Provisional; simpler than what it replaces. The baseline model needs to be dull —
something like a rolling percentile of observed intervals per node per state — and explicitly
not a learned model anyone has to reason about.

---

## D38 — Three-tier state reporting, and an ordered command list in the config

**Decided.** Two refinements to the node contract: split what the node reports about itself into
**`state` / `task` / `detail`**, and give the config an **ordered list of commands** so that
"cancel and rescan" is a single atomic intention.

### `state` / `task` / `detail` — and which one the core baselines against

| Field | Nature | Used by |
| --- | --- | --- |
| **`state`** | Small, stable, coarse enum: `idle`, `scanning`, `transcoding`, `error` | API contract; UI headline |
| **`task`** | **Bounded enum** within a state: `enumerating`, `probing`, `hashing`, `extracting` | **The core's cadence baseline**; UI subtitle |
| **`detail`** | Free text: `films/the-unknown.mkv` | Humans only. **Never** keyed on. |

The important constraint, or [D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval)
breaks silently: **`task` must be a bounded enum, and the core baselines on the `(state, task)`
pair, not on `state` alone.** D37's whole argument was that a coarse state which switches
between fast enumeration and slow probing yields a useless bimodal baseline. Moving the
fine-grained information into a free-text field would reintroduce exactly that problem while
looking like it had been solved — free text has unbounded cardinality and cannot be a baseline
key.

So: three tiers, with the middle one carrying the machine-readable precision.

### An ordered command list, because "cancel and rescan" is one intention

Real and frequent frustration: you add files, a scan is already running, and you must wait for
it to finish before you can rescan. So the config carries an ordered list —
`["cancel", "scan"]` — executed in sequence.

**A list rather than a single command is the right call**, because two separate config writes
would race: the core writes `cancel`, the node cancels and goes idle, and the second write may
or may not arrive before something else changes state. One list is one atomic intention.

Mechanically it rides on what already exists: the commands are part of the desired state, the
`generation` counter marks them, and `observed_config` acknowledges that they ran
([D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)).
A closed vocabulary — `cancel`, `scan`, `rescan <root>`, `pause`, `resume` — not arbitrary
strings.

### The trap: a cancelled scan must never drive deletions

**This is the dead-node trap wearing a different costume, and it is the dangerous part of this
feature.**

A cancelled scan has enumerated part of the library. If its partial inventory is treated as
authoritative, every file it had not yet reached looks absent — and the core deletes most of
someone's library on the strength of a scan the user themselves interrupted.

So: **a scan report carries a completeness flag, and an incomplete inventory can only ever add
or update, never remove.** Same rule as
[D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) trap 1 —
removal requires positive evidence from a complete, successful pass, never inference from
absence. This deserves the same test: cancel a scan mid-run and assert nothing is deleted.

### Other things cancellation needs to actually work

- **Poll for commands *during* work, not between phases.** If the node only checks at phase
  boundaries and a phase takes twenty minutes, "cancel" does not deliver "on a dime". Check at
  the same granularity as the progress tick.
- **Stop at a safe point and say so.** Abandon at a directory boundary, leave the report marked
  incomplete, and return to `idle` with a fresh `epoch` — which is exactly what the core needs
  to see that the cancel took effect.
- **`scan` while already scanning is a no-op**, or at most sets a rescan-when-done flag. It
  does **not** implicitly restart — which is precisely why the explicit `["cancel", "scan"]`
  form is needed and unambiguous.
- **Commands must be safe to re-run.** On crash recovery the node sees `generation` ahead of
  `observed_config` and re-executes. Every command in the vocabulary must tolerate that, which
  they do — and any future command that cannot must not be added to this channel.
- **Scope is the node's background work.** Playback-driven transcoding has a viewer attached and
  its own lifecycle; it is not cancelled by this channel.
- **Where HTTP exists, prefer it** — same as [D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk).
  This is the file-transport equivalent, not a competing mechanism. Note also that with the
  `*arr` webhook path in place, the "I just added files" case largely stops arising; cancel is
  for manual additions and for impatience, both of which are legitimate.

**Status:** Provisional. The completeness flag is the load-bearing part and should exist in the
report contract from the first version, because retrofitting it means a period where cancelled
scans can delete libraries.

---

## D39 — Reports are JSONL; completeness is in the filename and is scoped, not global

> **Amended 2026-09-20 by [D118](#d118--the-scan-is-incremental-prioritised-and-steerable-and-playability-comes-first).**
> ~~*the core never parses a file still being written*~~ — the core **may tail** a `.partial`
> report so results appear as they are produced, reading only up to the last newline. The
> safety rule is unchanged because it was never the filename: *unknown means incomplete means
> add-or-update only*. The rename still marks the moment removals may be applied.

**Decided.** The scan report is **JSONL** — one JSON object per line, header first, entries,
trailer last. But the authoritative completeness signal is the **filename**, not the trailer,
and completeness is **per-subtree rather than per-report**.

### JSONL is right, and the best reason is not the obvious one

The obvious reasons hold: streaming write with no need to hold an inventory in memory,
streaming read at the other end, append-only so a crash leaves a valid prefix, and a human can
`grep` it — which matters given how much of this design is about legibility.

The strongest reason is different: **a single JSON document is all-or-nothing to parse, while
JSONL degrades per line.** This file is produced by the component that handles hostile media
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)), from filenames
and probe output that are attacker-influenced. One malformed record should cost one entry and
an error count, not the entire report.

### Completeness belongs in the filename

A trailer says "complete" by its presence — but its **absence** means crashed, cancelled, still
being written, or read raced the writer, and those are indistinguishable.

That ambiguity is at least *safe*: unknown → treat as incomplete → add-or-update only, never
remove ([D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config)). The
fail-safe direction is the correct one. But it is still worse than knowing, and checking
requires reading to the end of a file that may have a hundred thousand lines.

So: the node writes `report-<epoch>.jsonl.partial` and **atomically renames** to
`report-<epoch>.jsonl` when the pass finishes. Completeness becomes an **O(1) check on the
directory listing**, and the core never has to parse a file that is still being written. It may
still read the `.partial` opportunistically for early adds and progress; it acts on removals
only for the final name.

**Keep the trailer anyway** — not as the completeness signal, but as the summary: entry count,
duration, per-root outcome, error count, and a checksum the core can verify. Cheap, and it
catches a truncated-but-renamed file.

And a **header line first**, carrying protocol version, `node_id`, `epoch`, report kind, and the
roots covered — so the metadata is available without reading to the end.

### Completeness is scoped, and that makes cancellation genuinely useful

The refinement that matters most here. A report is not simply complete or incomplete: **each
root, and ideally each subtree, was either fully enumerated or was not.**

- A scan cancelled after `/mnt/films` finished but before `/mnt/tv` started has *complete*
  coverage of films. **Removals within films can be applied safely**; tv is untouched.
- A scan that completed but could not read `/mnt/tv/Series X` because of a permissions error is
  complete everywhere *except* that subtree — and the files beneath it **must not be treated as
  deleted**. That is the dead-node trap at finer granularity, and it is the likeliest way to
  lose data in normal operation, because unreadable directories are common and undramatic.

So the report records **per-root (and per-errored-subtree) enumeration status**, and the core
applies removals only within regions positively marked complete. Without this, a cancelled scan
contributes nothing and cancellation is much less useful; with it, partial work still counts.

### Two report kinds, marked per line

- **Inventory** — a full enumeration of a region. Absence implies removal, *only* where that
  region is marked complete.
- **Delta** — explicit `added` / `changed` / `removed` events, per
  [D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do)'s rule
  that deletions are stated, never inferred.

Each line carries its record type, so a reader never has to guess from context.

### Smaller points

- **Compression is worth it and does not break append.** Paths and probe output are highly
  repetitive; gzip's concatenated-stream property allows append-with-flush, so `.jsonl.gz` stays
  streamable.
- **Retention is easy with epoch-named files** — keep the last N — which discharges the
  obligation [D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)
  flagged.

**Status:** Provisional. Per-subtree completeness is the load-bearing part and, like the
completeness flag itself, belongs in the format from the first version — retrofitting it means
a period in which an unreadable directory silently deletes its contents from the library.

**Extended by [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret)** — the report is the *snapshot* half of a snapshot-plus-journal pair. Every property here survives unchanged.

---

## D40 — `error` and `skip` records, and why the error record carries the completeness scope

**Decided.** The JSONL report gains two record types beyond header / entry / trailer:
**`error`** and **`skip`**. The error record is not merely diagnostic — **it is the mechanism
that expresses [D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)'s
scoped incompleteness**, and therefore the thing standing between a permissions glitch and a
deleted library.

### Errors belong in the stream, in order

Position carries meaning: an error record sits exactly where the failure happened in the
enumeration, so a reader — human or core — knows what was and was not covered around it. That
is strictly better than a side log, and it is why this is a record type rather than a separate
file.

### `scope` is the field the sketch was missing, and it is the important one

Not every error invalidates the same amount, and treating them alike would either lose data or
make cancellation useless:

| `scope` | Cause | Effect on removal safety |
| --- | --- | --- |
| `entry` | One file could not be `stat`ed or probed | That entry is unknown. **Siblings are unaffected** — the directory listing succeeded, so the region stays complete. |
| `subtree` | A directory could not be opened or listed | **Everything beneath it is unknown.** No removals anywhere under that path. |
| `root` | A whole library root is unreachable — share down, mount gone | No removals for that root at all. |
| `run` | The pass itself failed or was cancelled | Falls back to D38's rule: add-or-update only, except within subtrees positively marked complete. |

Without `scope`, a single unreadable file would have to invalidate its whole directory — or,
far worse, an unreadable *directory* would be treated as "no files here", which is precisely
the data-loss path D39 warns about. **This field is what makes per-subtree completeness
expressible at all.**

Note the probe case specifically: a file that enumerated fine but failed to probe **exists**.
It is present-but-unprobed, never absent. That is an `entry`-scoped error and must never
contribute to a removal.

### `code` is a project enum, not an errno

The sketch used `EBADF`. Raw errnos are the wrong contract: they differ across platforms,
Windows has its own set, Go wrapping makes extraction unreliable, and several relevant failures
have no errno at all — a parse failure, a timeout, a cancellation.

So: a **stable, closed project vocabulary** — `permission_denied`, `not_found`, `io_error`,
`timeout`, `unsupported`, `probe_failed`, `cancelled` — which the core keys its logic on, plus
an **optional raw field** (`syscall`, `errno`, driver text) carrying the platform truth for a
human diagnosing it. Same closed-enumeration discipline as
[D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) and the command
vocabulary in [D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config).

And **`path` is a structured field**, not prose embedded in `reason`. A path inside a sentence
cannot be aggregated, matched against a root, or acted on.

### Errors must be aggregated and capped

A share that disappears mid-scan can generate an error per file — a hundred thousand records
that bloat the report and drown everything useful in it. The error channel becoming a
denial-of-service on its own report is a real operational failure, not a hypothetical.

So: **collapse repeated identical `(code, scope, path-prefix)` errors into one record with a
`count`**, and impose a cap after which the node stops enumerating that root, emits a
`root`-scoped error, and moves on. Failing fast on a dead share is better behaviour than
faithfully logging its every file.

### `skip` is a different thing and must not be conflated

A file excluded by an ignore rule or an extension filter is **not** a failure — but it is also
**not in the inventory**, and if that is indistinguishable from an error the core will get the
policy wrong in one direction or the other.

So a separate `skip` record carrying the **rule that caused it**. The distinction matters
because the correct behaviour differs: a file the user *deliberately excluded* probably should
leave the library, while a file we *failed to read* definitely should not. Recording them as
the same thing means choosing one behaviour for both and being wrong half the time.

### Mirror the last error into the status file

The report carries the full ordered set for the core; `last_error` in the status file
([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)) carries the
most recent one plus a count for the human who is glancing at it. Two audiences, two views,
one source.

**Status:** Provisional. `scope` is the load-bearing field and belongs in the format from the
first version, for the same reason as the completeness flag itself — without it, an unreadable
directory is indistinguishable from an empty one.

---

## D41 — Diagnose the failure boundary by walking up; and guard the silent case, which is worse

**Decided.** On a read failure the scanner **walks up the tree to find the smallest broken
subtree** rather than reporting per-file errors, and separately guards against the mount failure
that produces **no error at all**.

### Escalate by probing, not by counting

When `/mnt/tv/Series X/S01E01.mkv` fails, the useful question is not "log this" but **"what is
actually broken?"** So the scanner stats the parent, then its parent, upward, until something
succeeds. The highest failing ancestor is the boundary, and that boundary is exactly
[D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope)'s
`scope`:

- parent readable → `entry` scope; carry on with the siblings.
- `Series X/` unreadable → `subtree` scope; emit one record and skip the rest of it.
- `/mnt/tv` unreadable → `root` scope; abandon the root.

Path depth is small, so this converts ten thousand failures into one diagnosis plus a handful
of `stat` calls. **This supersedes D40's error cap as the primary mechanism** — the cap was a
blunt instrument that gave up after N failures; escalation finds the actual boundary and is
both cheaper and more precise. Keep the cap as a backstop for pathological cases only.

**Memoise the failed prefix for the run**, or the walk-up repeats for every subsequent file
beneath a directory already known to be gone.

### The silent case, which is the real danger

An unmounted mountpoint on Linux usually reverts to **the empty directory underneath it**. So:

- The path exists.
- It is readable.
- Listing it **succeeds** and returns **zero entries**.
- **No error is raised. `scope` never fires. The walk-up finds nothing wrong.**

A naive scanner then reports a *complete* enumeration of a root containing nothing — and every
removal-safety rule built so far permits the core to delete the entire library, because the
report is honestly and correctly marked complete.

**This is the most dangerous failure mode in the design**, precisely because every mechanism
added so far is triggered by errors and this one has none.

Two independent defences, both required:

1. **Verify the mount, not the path.** Record each root's device id (`st_dev`) at setup. A
   mountpoint has a different device id from its parent directory; if they have become equal,
   the mount is gone. This is automatic, needs no user action, and is checked before a root is
   enumerated and again before its removals are applied. A user-placed sentinel file is the
   portable fallback where device ids are not available.

2. **Never believe implausible shrinkage.** Independently of mounts: if a root has lost a large
   fraction of the entries it had at the last complete scan, **the report is marked incomplete
   and removals are withheld pending confirmation.** This catches the whole class of
   no-error failures — a wrong path after reconfiguration, a permissions change that empties a
   listing, a half-finished remount, a typo'd config.

   It must **ask**, not refuse: a user who genuinely deleted a library should be able to say so
   in the UI. Silently refusing is its own bug, just a quieter one.

### A hung mount presents as wedged, not failed

NFS hard mounts block indefinitely rather than returning an error, so filesystem operations need
timeouts — and a timeout is a `subtree`- or `root`-scoped error, not a hang.

Pleasingly, the existing machinery already covers the gap: a scanner blocked on a dead mount
stops moving its progress numbers while its state stays `scanning`, which is exactly the
**wedged** signature from [D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle).
The core will surface it even if the timeout logic is imperfect. `ESTALE` on NFS escalates the
same way as a lost mount.

### Recovery restores; it does not re-add

When a mount returns, items previously marked unavailable must be **restored**, not ingested as
new — which requires the stable identity
[D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) and
[D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)
already mandate. Re-adding would lose watch state, user metadata corrections and playlist
membership, which is a data-loss event wearing a recovery costume.

**Status:** Provisional. The shrinkage threshold needs a real number and should probably scale
— a root of 20 items losing 10 is noise, a root of 40,000 losing 39,000 is not. Both defences
belong in the first version; the silent case cannot be retrofitted safely because by the time it
is noticed the deletions have happened.

---

## D42 — Nodes are sensors; the core is the only thing that decides

**Decided.** A general principle, and a correction to
[D41](#d41--diagnose-the-failure-boundary-by-walking-up-and-guard-the-silent-case-which-is-worse):

> **A node reports what it observed. The core decides what it means.**

D41 put the mount check and the implausible-shrinkage guard *in the node* — "mark the report
incomplete and withhold removals pending confirmation". That is the node making policy, and it
is wrong in four separate ways.

### Why the node cannot and should not judge

1. **It lacks the information.** The shrinkage guard needs *"what did this root contain at the
   last complete scan?"* — which is **core state**. For the node to judge, it would have to keep
   its own history of every root, duplicating the core's inventory and creating a second source
   of truth about the library. The core already knows. This is the decisive argument.
2. **It is the component under suspicion.** A node whose mount just vanished is not who you want
   ruling on what that means — the same reasoning as
   [D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval),
   where a node declaring its own health thresholds was rejected because the node most in need
   of catching is the one least able to report accurately.
3. **Policy lives where the policy is configurable.** The confirmation prompt, the user's
   preferences, the override — all in the core. A node withholding removals puts the decision
   and its override in different processes.
4. **Only the core can reconcile.** With overlapping roots or several nodes seeing one share, a
   node judging in isolation could withhold removals another node has good data for.

### The reframing this produces, which is cleaner than D41's version

The silent-mount problem is **not** "the node must detect a missing mount". It is:

> **The node must report "I listed this directory and it was empty" as distinct from "I did not
> enumerate this directory".**

That distinction is an *observation* only the node can make — and once it is in the report, the
core, which knows the root held forty thousand files yesterday, draws the conclusion itself.
The node needs no threshold, no history and no confirmation mechanism.

That is a genuine simplification of the node, which is usually the sign the boundary is now in
the right place.

### What the node reports — facts only

- **Observed-empty vs not-enumerated**, per directory. The fact above.
- **Filesystem observations** at each root: current `st_dev`, whether the path is a mountpoint,
  filesystem type, read-only flag. The node reports the *current* value; **the core** compares
  it against what it recorded and decides whether a change is alarming.
- **Errors with `scope`** ([D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope)).
- **Whether the pass finished**, was cancelled, or died.

### The line between fact and judgement, which needs care

"This subtree errored" and "this scan was cancelled" are **facts the node owns** — factual
incompleteness, and it stays in the report and in the filename
([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)).

"This shrinkage is implausible" is a **judgement requiring history**, and moves to the core.

The test: *could the node determine this from what it saw during this one pass, with no memory
of previous passes and no knowledge of the library?* If yes it is a fact; if no it is a
judgement and belongs in the core.

### It generalises, and it buys testability

Every input to the core is a sensor — media nodes, `*arr` webhooks
([D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk)),
plugins, user actions. If judgement is scattered across them, nobody can reason about how the
library reaches its state.

Concentrating it also means **the core's decision logic can be tested against synthetic reports
with no filesystem, no nodes and no hardware at all** — a large win for
[testing-strategy.md](../design/plans/testing-strategy.md), where the standing problem is that
most of this product's correctness lives somewhere a unit test cannot reach. Removal safety is
exactly the logic that most deserves exhaustive testing and is now exactly the logic that can
be.

**Status:** Decided, and promoted to a principle
(`principles/README.md` §9) because it is cross-cutting rather than specific to scanning.

---

## D43 — One binary, one process per role, and a thin optional supervisor

**Decided.** The single binary runs **one role per process**. A machine running a core, a node
and a transcoder runs three processes, not one process with three goroutines. The binary can
supervise them itself for convenience, but that supervisor is thin and optional.

### Core and node are separate processes even when co-located

Already implied by [D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do),
and worth stating why:

- **The boundary should be enforced by the operating system, not by discipline.**
  [D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides) says nodes are sensors
  that must not hold database credentials or make policy. In one address space that is a
  convention a tired afternoon can breach; in separate processes it is impossible.
- **The local and distributed cases must be one code path.** If co-located means goroutines and
  remote means processes, there are two implementations and the local one — the common one — is
  the one that never exercises the real protocol.

### Roles within a machine are also separate, and Landlock is the clincher

The tempting design is one process per machine with roles as goroutines. It is wrong, and the
reason is concrete rather than aesthetic:

**Landlock is per-process and monotonic** ([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)) —
a ruleset can only be tightened, never relaxed, and it applies to the whole process. Different
roles need genuinely different rulesets:

| Role | Needs |
| --- | --- |
| Scanner/probe node | read media roots; write its exchange directory |
| Transcoder | read media roots; write a cache directory; **access GPU device nodes** (`/dev/dri`, `/dev/nvidia*`) |

Combining them into one process means applying **the union** of those rulesets — a strictly
weaker sandbox, in which the component doing the riskiest work carries permissions it does not
need. Splitting the processes is what lets each be confined to exactly its job.

Three lesser reasons in the same direction: **resource isolation** (transcoding is long and
CPU/GPU-heavy, scanning is bursty and I/O-heavy, and they want different `nice`/cgroup
treatment); **failure isolation** (a GPU driver hang should not take down the scanner); and
**optionality** — [D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)
makes the transcoder a separately-installed plugin, and "not installed" is cleanly expressed as
"not running".

The memory objection does not bite: a Go process idles in the tens of megabytes, so three of
them is noise even on the 8 GB NAS that
[test-hardware.md](../design/research/test-hardware.md) calls the honest low end.

### Two invocation styles, because two audiences

```
mediaserver run                 # reads config, supervises every enabled role
mediaserver core                # exactly one role, foreground
mediaserver node
mediaserver transcode
```

`run` serves the default install: one thing to start, one thing to stop. The single-role forms
serve anyone who would rather use **systemd units, separate containers, or Kubernetes** — which
is the right way to do it if you already have a supervisor, and it must not be second-class.
One config file listing enabled roles; each role reads the sections it needs.

### The supervisor must stay thin — this is a boundary, not a feature

Fork the enabled roles, pipe their stdio into the log stream, restart on unexpected exit with
backoff, forward signals, exit when told. **Roughly two hundred lines.**

What it explicitly is **not**: a dependency graph, service discovery, socket activation, health
checking beyond process liveness, or anything resembling an init system. Every one of those
exists already in systemd, Docker and Kubernetes, and reimplementing them badly is a known way
to spend a year.

**PID 1 duties, which are a classic bug.** In a container the supervisor *is* PID 1 and must
**reap zombies** and **forward `SIGTERM`/`SIGINT` to children**. Getting this wrong produces
containers that accumulate defunct processes and take ten seconds to stop because nothing
propagated the signal.

### Local IPC uses the remote mechanisms

Co-located roles talk over the **same protocol as the distributed case**, defaulting to a
**Unix socket** locally rather than TCP. Two benefits: the common deployment exercises the same
code the rare one depends on, so the distributed path cannot rot unnoticed; and the socket
file's permissions are the authorisation, which is the same
"[permissions are the security model](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)"
pattern already adopted for the exchange directory.

### The resulting process tree

```
mediaserver run                    (thin supervisor; PID 1 in a container)
├── mediaserver core               SQLite, HTTP, decisions
├── mediaserver node               → forks confined ffprobe/ffmpeg per job
└── mediaserver transcode          → forks confined ffmpeg per job
```

FFmpeg remains a **confined child per invocation** (D26), unchanged: the role processes
supervise it, they do not host it.

**Status:** Provisional. The supervisor's scope is the thing to guard — every feature request
for it should be answered with "use systemd" until there is evidence otherwise.

---

## D44 — Prefer the platform's supervisor; ship the units; and never exit when you could report

**Decided.** Restart is a **deployment** concern, and the right answer is usually not ours.
Order of preference, and we **ship** the first two rather than documenting them:

1. **systemd units** (bare-metal Linux) — restart, watchdog, sandboxing and cgroups for free.
2. **Separate containers** (Compose / Kubernetes) — restart policies and per-container limits.
3. **Our `run` supervisor** ([D43](#d43--one-binary-one-process-per-role-and-a-thin-optional-supervisor)) —
   the fallback for everything else: one container holding all roles, Windows, macOS, or plain
   simplicity.

The supervisor exists because a fallback must exist, not because it is the recommendation.

### What systemd gives us, beyond restarting

- **`Restart=on-failure`** with `RestartSec=`, plus `StartLimitIntervalSec=`/`StartLimitBurst=`
  (5 starts in 60 s by default) so a crash-looping unit **parks in `failed`** rather than
  spinning forever. Deliberate and correct — and it means the failure is only visible if
  something is watching, which the core is
  ([D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval)).
- **`Type=notify` + `sd_notify(READY=1)`** for real readiness rather than "the process started".
- **`WatchdogSec=` + `WATCHDOG=1`** — active liveness, killing and restarting a stuck process
  that exit-based restart cannot catch. See the nuance below.
- **Per-unit sandboxing** — `ProtectSystem=strict`, `ReadWritePaths=`, `ProtectHome=`,
  `NoNewPrivileges=`, `SystemCallFilter=`, `DeviceAllow=`, `PrivateTmp=`,
  `MemoryDenyWriteExecute=`.
- **Per-unit cgroups** — `MemoryMax=`, `CPUWeight=`, `IOWeight=`, `Slice=`.
- **`StateDirectory=` / `CacheDirectory=` / `LogsDirectory=`**, created and owned correctly —
  which is where the exchange directory
  ([D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary))
  should live.

**Two of these validate [D43](#d43--one-binary-one-process-per-role-and-a-thin-optional-supervisor)
from a second direction:** systemd's sandboxing and resource control are **per unit**, exactly
as Landlock is per process. Three roles as three units get three different confinement profiles
and three resource budgets. One unit running all three would take the union — the same argument
that made D43 split the processes, arriving independently.

It also means **the unit file is the outer sandbox on a systemd host**, with D26's in-process
Landlock as defence-in-depth and as the only layer elsewhere. So hardened unit files are part
of the product, not an appendix.

### The watchdog is not our wedged detector — keep the layers separate

Tempting, and wrong to collapse: `WatchdogSec` detects a process that **stops pinging**, which
is liveness of the *process*, not progress of the *work*. A scanner whose work goroutine is
stuck while its runtime is healthy will keep pinging happily.

Gating the ping on progress would turn systemd into a wedged-detector with restart authority —
but that walks straight into D37: a node judging its own health is the least reliable source,
and a legitimately slow task (probing a 60 GB remux) would earn spurious restarts.

So, two layers that do different jobs:

- **systemd's watchdog**: fast, blunt, catches hard hangs. Ping conservatively — fire unless
  definitely stuck.
- **The core's observation** (D37): slower, smarter, catches subtle wedging, graded, and holds
  **no restart authority**.

They compose rather than overlap, and systemd restarting a unit in two seconds is invisible to
the core because D37's escalation has hysteresis and "late" does nothing.

### Restart is already safe, by construction

Nothing special is needed, because earlier decisions bought it:

- A scanner killed mid-scan leaves a `.partial` report that can only add or update, never remove
  ([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)).
- `epoch` increments, so the core reads the reset progress as movement rather than a stall
  ([D36](#d36--idle-ticks-the-progress-pair-the-node-declares-its-own-tick-contract)).
- `node_id` persists, so a restarted node is the *same* node and its items are never orphaned
  ([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)).

### The rule: report failure, do not exit

**A node that cannot do its job should stay alive and say so.** A scanner whose share is
unreachable emits a `root`-scoped error ([D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope),
[D41](#d41--diagnose-the-failure-boundary-by-walking-up-and-guard-the-silent-case-which-is-worse))
and keeps running. It does **not** crash.

Four reasons: exiting **destroys the reporting channel**, which is the thing that would have
explained the problem; repeated exits trip `StartLimitBurst` and park the unit, converting a
recoverable outage into a manual intervention; the core cannot distinguish "exited because the
share is down" from "exited because of a bug"; and a live node reporting `root unreachable` is
strictly more informative than a dead one. This is the natural consequence of
[D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides) — sensors report.

Exit is reserved for conditions that genuinely cannot be reported: a corrupt local config, a
failed sandbox setup, an unrecoverable internal state.

### Kill the process group, or you get orphaned FFmpeg

A common and memorable bug in this domain. When a role process dies, its confined FFmpeg
children must die with it, or they keep a CPU busy forever and nobody knows why.

- systemd's default `KillMode=control-group` kills the whole cgroup — handled.
- Our supervisor must kill the **process group**, not the process.
- Belt and braces on Linux: set `Pdeathsig` in the child's `SysProcAttr` so an FFmpeg child is
  signalled when its parent dies, even if the parent died in a way that skipped cleanup.

### For our own supervisor

Exponential backoff, and — the important part — **never give up silently.** A role that has
stopped restarting must be visible as a state the core can surface, not a line in a log nobody
reads. That is the same failure mode `StartLimitBurst` has, and the same remedy.

**Status:** Provisional. The shipped unit files are a deliverable and want hardening review;
`s6-overlay` is a legitimate alternative to our supervisor inside a single container and is
worth revisiting if the supervisor starts growing features — which is the signal D43 said to
watch for.

---

## D45 — Metadata is multi-language in the schema, chosen per request

**Decided** (2026-09-19). Split every metadata-bearing entity into a **language-invariant row**
and a set of **translation rows** keyed `(item, language)`. Choose among them **at query time**
from the request's `Accept-Language`. Never at ingest.

**Why.** Jellyfin resolves `PreferredMetadataLanguage` through item → parent → library → server
and uses it at *fetch* time, so the database holds exactly one title. Two people in one
household cannot have different languages, and there is no fallback chain — only en-US.
*Language Selection Fallback Heirarchy* (#2521) has been open since **2020-03-05**. This is not
a bug they have failed to fix; it is what the schema permits.

Kyoo has the shape that works, and three of its details are the actual content of this decision:

- **Artwork belongs in the translation row.** Posters have title text baked into the pixels, so
  poster language is a separate axis from title language — which is exactly what Jellyfin #10076
  asks for and cannot have.
- **Genres are an enumeration; tags are free text.** Genres are a closed set stored as symbols
  and rendered from our own UI catalogue, so a Chinese user sees 科幻 from our translation file
  rather than whatever string TMDB returned. Tags cannot be enumerated, so they stay
  per-language. Cheap, sharp, and it removes a whole class of half-translated UI.
- **The original-language record is distinguished**, not just another translation. It always
  exists, which gives a guaranteed fallback anchor and a meaningful "sort by original title".

**`Accept-Language` is the fallback chain.** It is an ordered, weighted preference list with a
wildcard; it is standard; every client already sends something sane. Six years of Jellyfin
feature requests are asking for a thing HTTP has always had.

**Two cautions, both load-bearing.**

Language selection participates in `ORDER BY`, so it must be a **total** order — Kyoo's own
comment says a non-deterministic tie-break makes keyset pagination return the same row on two
pages. And parse `Accept-Language` with `golang.org/x/text/language` rather than by hand:
Kyoo's hand-rolled parser sorts the `q=` weights ascending, which appears to select the
*least*-preferred language, and has no test. `ParseAcceptLanguage` + `NewMatcher` implement full
BCP 47 lookup including likely-subtags and mutual intelligibility (nb/no, zh-Hans/zh-Hant,
pt-BR/pt).

`DISTINCT ON` is Postgres-only; under SQLite this is `ROW_NUMBER() OVER (PARTITION BY item_id
ORDER BY lang_rank, language)`, available since 3.25.

**Status:** Provisional, but schema-shaping and therefore effectively due before
[library-model.md](../design/plans/library-model.md). Overturned by evidence that per-language
artwork is not worth its storage at household scale — which nobody has measured.

---

## D46 — No translated string is ever stored in the database

**Decided** (2026-09-19). Store the fact and its parameters; render at display time. One
catalogue, owned by the client. The server renders text only for delivery that leaves a client
session, and renders it in the **recipient's** language.

**Why.** Jellyfin maintains two catalogues — 1,947 keys in `jellyfin-web`, 123 in the server —
which **overlap by twelve keys**, including `HeaderContinueWatching`. Twelve concepts translated
twice, by two sets of translators, in two Weblate components, displayed in the same UI.

The instructive entry is `ChapterNameValue`: `"Chapter {0}"`, used to generate a name that is
then *written to the database*. It is frozen in whatever language the scanner was running in. A
chapter is `{index: 3}`, not `"Chapter 3"`.

Once the rule holds, the server catalogue collapses to the cases that genuinely leave the
session — notification and activity-log text delivered by email or push — and even those should
render in the recipient's language, which the server knows.

`golang.org/x/text/message` + `catalog` covers that with CLDR plural rules, for a measured
**+594 KB** of binary over hello-world.

**Status:** Provisional. Free if adopted now, expensive later. The failure mode to watch for is
a "convenient" generated display name creeping into a column; a test asserting that no
user-visible text column is written by the scanner would catch it.

---

## D47 — Sort order is a materialised CLDR sort key, per item per locale

**Decided** (2026-09-19). Generate a `collate.KeyFromString` byte key with
`golang.org/x/text/collate` for each (item, locale-in-use), store it in an indexed `BLOB`
column, and let SQLite order on it with the default `BINARY` collation. Not a transliteration,
and not a custom SQLite collation.

**Why not transliteration.** Jellyfin materialises a sort string by lowercasing, stripping
hard-coded English stop-words (`the, a, an` — from the start, the *middle* and the end), removing
diacritics, and then transliterating anything non-ASCII with
`Any-Latin; Latin-Ascii; Lower; NFD; …`, tunable by the environment variable
`JELLYFIN_TRANSLITERATOR_ID`. Four separate mistakes: the stop-words are English and
server-wide; `Any-Latin` renders Japanese kanji as pinyin, so 千と千尋の神隠し does not sort under
*Sen*; transliteration destroys exactly the distinctions the target locale cares about, folding
Swedish Å Ä Ö to the front when Swedish puts them after Z; and a global environment variable to
override the rule is the tell that one server-wide order was never enough.

**Measured, rather than assumed** (Go 1.26.2, x/text v0.42.0). `x/text/collate` gets Swedish,
German phonebook (ü ≡ ue), Chinese pinyin *and* stroke, and Japanese kana equivalence right;
`collate.Numeric` orders `Episode 2` before `Episode 10` natively, deleting Jellyfin's
zero-padding hack. It gets **Korean wrong** — output identical to codepoint order, because
`// TODO: implement script reordering` is still in the source and
[golang/go#19087](https://github.com/golang/go/issues/19087) has been open since 2017 with zero
comments. And `collate/tables.go` declares `UnicodeVersion = "6.2.0"`, `CLDRVersion = "23"` —
2012 and 2013 — while the rest of `x/text` tracks current Unicode.

That is the honest trade: correct for Latin, Cyrillic, Greek, Chinese, Japanese; wrong for
Korean; thirteen years behind on data. The alternative, cgo-linked ICU, costs the
cross-compilation that [D31](#d31--sqlite-with-a-pure-go-driver-supersedes-d5) went out of its way
to buy.

**Why materialise.** Sort keys are byte-comparable, so they need no collation support from the
database at all. Measured at 10,000 titles: sorting in Go at query time takes 16.3 ms but
forfeits `ORDER BY … LIMIT 50` and index paging; key generation is ~1 µs/title and keys average
**~140 B**, so 1.4 MB per 10k items *per locale*, and SQLite pages against an index in O(log n).
A household uses one to three locales.

**Why not a custom collation**, which both pure-Go drivers support
(`modernc.org/sqlite.RegisterCollationUtf8`, `ncruces/go-sqlite3` `CreateCollation`): an index is
bound to the collation it was created with, so you need one index per locale regardless — the
materialised design with extra steps — and it makes the database file unopenable by the `sqlite3`
CLI or any recovery tool, every affected query failing with *no such collation sequence*. A BLOB
column merely degrades to an opaque column, which is what
[D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise)'s "rebuildable index" stance wants.

**Status:** Provisional. Korean is a known, accepted defect — see
[internationalisation.md](../design/research/internationalisation.md) §9 for the three ways out.
Revisit if the locale count per household turns out to be large.

---

## D48 — Two FTS5 tokenisers, routed by script and length, and exactly one normaliser

**Decided** (2026-09-19). Index every searchable entity **twice**: `unicode61
remove_diacritics 2` for Latin word search and bm25 ranking, and `trigram` for substring search.
Route: 1–2 characters → `LIKE`, ≥3 characters → trigram, Latin word queries → unicode61. Feed the
unicode61 index a **normalised-alternates column** generated in Go. That normalisation function
has exactly one implementation.

**Why two.** Measured in SQLite 3.46.1, because the documentation does not say this plainly:

| | Latin + diacritics | ø æ œ ß | CJK | Thai | prefix / bm25 |
| --- | --- | --- | --- | --- | --- |
| `unicode61 remove_diacritics 2` | ✓ | ✗ | ✗ | ✗ | ✓ |
| `trigram` | ✗ | substring only | ✓ (≥3 chars) | ✓ | ✗ |

unicode61 tokenises `千と千尋の神隠し` as **one token**, so `千尋` finds nothing, and shreds Thai
into fragments (`ผ`, `หญ`, `งล`, `ลลา`) that match no word. Trigram handles both but needs three
characters and cannot do prefix search or rank.

Neither fixes atomic letters. `remove_diacritics` removes **combining marks**; Ø Æ Œ ß Ð Ł are
single code points with no decomposition, so `"Bjork"` misses `Bjørk` and `"GOGGS"` misses
`GØGGS` under both tokenisers. No option fixes this — hence the alternates column, which is what
Navidrome arrived at after shipping the bug.

**So run both**, because the cost is a rounding error. Measured on 20,000 mixed-script titles:
content 532 KB, unicode61 index 476 KB, trigram index 1,004 KB — **1.5 MB for both**, each
building in ~30 ms. Query latency: 0.012–0.10 ms for either index; `LIKE` worst case (no match,
full scan) 1.554 ms against trigram's 0.001 ms, which is the case that matters because it is what
happens on every keystroke of a type-ahead.

**One normaliser, and why.** Navidrome shipped FTS5 in February 2026 and a corrective migration
in July: the migration had back-filled `search_normalized` *"with a SQL approximation that cannot
transliterate atomic letters (Ø, æ, ß, …)"*, so `GØGGS` was unfindable. The Unicode lesson is
above; the engineering lesson is that the logic existed twice — once in Go, once as a SQL
`REPLACE` chain — and drifted. A migration that needs normalisation calls the Go function row by
row.

Four further details taken from Navidrome verbatim: contentless tables (`content=''`) with
trigger population and a `RebuildFTS`, because the index is derivable and FTS5 corruption is a
real operational event; **lowercase the user's `AND OR NOT NEAR`**, which are case-sensitive
operators in FTS5 and otherwise throw a syntax error; **no minimum query length for CJK**,
because single CJK characters are meaningful words; and `(token OR token*)` rather than bare
`token*`, which adds no matches but lets bm25 rank literal hits above prefix hits.

**Status:** Provisional but schema-shaping — the tokeniser is baked into the FTS table, so this
is due before the search schema. The routing thresholds are guesses pending a real corpus.

---

## D49 — Unicode discipline at the boundaries, enforced by tests

**Decided** (2026-09-19). Two boundary rules, each with a test that fails the build.

**1. Normalise paths to NFC at the core's ingest boundary, and keep the raw bytes for I/O.**

[D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) puts nodes on arbitrary
machines reporting paths to the core. macOS/APFS returns decomposed filenames; Linux returns the
bytes on disk. Measured:

```
macOS-style NFD : "Amélie"  len=8  bytes= 41 6d 65 cc 81 6c 69 65
Linux-style NFC : "Amélie"  len=7  bytes= 41 6d c3 a9 6c 69 65
equal? false      equal after NFC(both)? true
```

Same file. A macOS node and a Linux node reporting the same SMB share therefore report two
different paths for one file: two rows under `UNIQUE(path)`, or a delete-and-add churn cycle
every time the reporting node changes. **Both halves of the rule matter** — identity, dedup,
display and search use the NFC form; *opening* the file uses the original bytes, because APFS
normalises on lookup and ext4 does not, so an NFC string will not open an NFD file on Linux.
This is a field pair in the report contract
([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)), not an
assumption.

**2. No ASCII-only character classes in the naming-regex corpus.**

Jellyfin's episode expressions use `[\w\s]+?` for the series name. In .NET `\w` is Unicode-aware;
**in Go's RE2 it is not.** Ported verbatim, the expression fails on `進撃の巨人`, `기생충`,
`Матрица` — and on **`Amélie`**. It is not a CJK problem, it is an everything-but-ASCII problem.
`\d` likewise misses Arabic-Indic digits, and `\s` misses U+3000 IDEOGRAPHIC SPACE, which is the
usual separator in Japanese and Chinese filenames, and U+00A0, which Windows renamers emit.

It fails **silently**: the expression does not match, the filename falls through to a worse
expression, and the result is a misparse rather than an error. So ban `\w \d \s \b [[:alpha:]]`
in that corpus, require `\p{L} \p{N} \p{M} \p{Z}`, and run the whole corpus against a fixture of
non-Latin filenames in CI.

**Why a decision and not a note.** Both are free now and near-invisible later — the NFC one is a
wire-format field, and the regex one is a lint rule. Both are the *parse, don't validate* stance
of [media-nodes.md](../design/media-nodes.md) applied to text, and the NFC pair is the same shape
as [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s rule for
track tags: normalise for use, keep the original for provenance.

**Status:** Provisional only in the sense that the fixture corpus does not exist yet. The
findings are measured, not inferred.

---

## D50 — Build libass correctly and turn off VSFilter bug-compatibility for non-authored subtitles

**Decided** (2026-09-19). Build libass with **HarfBuzz**, a current **FriBidi** and
**libunibreak**. Enable `ASS_FEATURE_WHOLE_TEXT_LAYOUT` and `ASS_FEATURE_BIDI_BRACKETS` for
sources that are not authored ASS, and enable `ASS_FEATURE_WRAP_UNICODE` with the **track
language** set. Leave all three off for authored `.ass`/`.ssa`.

**Why.** [D11](#d11--subtitles-are-a-subsystem-not-a-feature) chose libass everywhere partly
because browsers render WebVTT without regard to direction — jellyfin-web #4179, *"Right-to-left
languages subtitles having punctuation in the wrong order"*. libass has the same failure
available and it is **on by default**. From `ass.h`, with `ASS_FEATURE_WHOLE_TEXT_LAYOUT`
disabled, *"text is split into VSFilter-compatible segments and text in each segment is processed
in isolation… The individual runs are then laid out **left-to-right, even if they contain
right-to-left text**."* That is #4179, reproduced inside the renderer we picked to avoid it.

`ASS_FEATURE_BIDI_BRACKETS` gives Unicode 6.3 bracket pairing and needs a new-enough FriBidi.
`ASS_FEATURE_WRAP_UNICODE` gives UAX #14 line breaking, needs **libunibreak** at compile time,
and *"if the track language is set, some additional language-specific tweaks may be applied"* —
without it there is no correct line breaking for Thai, Japanese or Chinese, which have no spaces
to break at. This is the payoff for [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s BCP 47
normalisation: we already have the language to pass.

The defaults are right for libass and wrong for us. Fansubbed ASS is authored against VSFilter's
behaviour, so bug-compatibility is correct there — and only there. SRT, VTT, PGS-converted text
and CC were never authored against it.

**The uncosted item this exposes.** JASSUB is our web path. Its submodules are `libass, fribidi,
freetype, harfbuzz, brotli` — **no libunibreak** — and it never calls `ass_track_set_feature`. So
the browser client as it stands gets segmented bidi, pre-6.3 brackets, and no Unicode line
breaking. Getting this right means patching JASSUB's build and exposing the toggles: a real line
item for D11 that was not in its estimate.

`ass_track_set_feature` returns −1 when a feature is unavailable at runtime, which makes it a
capability probe. Check it once and report the answer rather than silently rendering wrong —
[principles §9](../principles/README.md), sensors report.

**Status:** Provisional. Needs a rendering comparison on real Arabic, Hebrew, Thai and Japanese
subtitle files before the flags are fixed; the split between "authored ASS" and "everything else"
is a guess at where the boundary belongs.

---

## D51 — react-i18next, catalogues served by us, Weblate deferred

**Decided** (2026-09-19). `react-i18next` in the client. Serve catalogues from our own server,
bundling only English. Choose the catalogue format for a translation platform now; connect one
when translators exist.

**Why react-i18next.** The only two projects solving our exact client problem converged on it
independently — Kyoo (`react-i18next` 17 + `i18next-browser-languagedetector` +
`i18next-http-backend`) and Streamyfin (`react-i18next` 17 + `i18next` 26 + `expo-localization`).
Two settings come with it, both from Kyoo: `returnEmptyString: false`, because translation
platforms export untranslated keys as `""` and the default renders them, so a
partially-translated language shows **blank UI** rather than English; and `load: "currentOnly"`.

Plural handling must **not** depend on `Intl.PluralRules`. Hermes does not bundle ICU — it
projects the platform's, whose *"results… may vary between devices running different Android
versions"* — and neither `Intl.PluralRules` nor `Intl.Segmenter` is in its supported list.
Streamyfin pins `compatibilityJSON: "v4"` explicitly; either pin it or ship
`@formatjs/intl-pluralrules`. The same platform variance is an independent argument for
[D47](#d47--sort-order-is-a-materialised-cldr-sort-key-per-item-per-locale): `localeCompare` can
order the same list differently on an Android 10 phone and an Android 14 TV, so **the server owns
sort order** and the client renders the list it is given.

**Why serve rather than bundle.** Streamyfin statically imports all 34 catalogues; a translation
fix then needs an app-store review cycle, which is the mechanism by which translations go stale
on TV. We are unusual in *being* the server the client is already talking to, so a merged
translation reaches every client on next connect, over the LAN, for free.

**Why Weblate, and why not yet.** It is the field's convention — 9 of 11 surveyed projects
(Jellyfin, jellyfin-web, Immich, Kyoo, Swiftfin, Audiobookshelf, PhotoPrism, Stash, Kavita)
against Crowdin for Streamyfin and POEditor for Navidrome. It is AGPL, self-hostable, and free
for libre projects. But Immich's integration is the real cost: two GitHub Actions workflows, a
GitHub App with scoped tokens, and a **required PR check that fails any PR touching
`i18n/!(en)**.json` without the bot's approval** — humans may edit only the English catalogue;
every other file is machine-owned. Their release flow locks the component, forces a commit and
push, finds the single `weblate` PR, approves, merges, polls, and unlocks, with cleanup on
timeout. None of that buys anything before there are translators.

**RTL.** `I18nManager.forceRTL()` sets a native flag read at startup; changing it does not
re-lay-out the running app (react-native #16215, #32509, #47583, and #49451 still reproducing on
0.77). On a TV a restart is worse than on a phone, so follow the OS locale's direction and do not
offer an in-app direction switch. Jellyfin has the matching layout bugs (jellyfin-web #7297,
Arabic series detail page), which is what happens when RTL is absent from the test matrix — so
one RTL locale belongs in the device-lab pass.

**Status:** Provisional. The bundle-vs-serve choice should be revisited if offline use of a
non-English client turns out to matter more than expected.

---

## D52 — Pseudo-localisation is the default development build

**Decided** (2026-09-19). Debug builds render every UI string through a pseudo-localisation
transform by default. Turning it *off* is the opt-in.

**Why.** Taken from Netflix, who published both the mechanism and the retrospective. Their
problem statement: translated text can be **up to 40% longer** than English — worst in German,
Hebrew, Polish, Finnish and Portuguese — and a UI designed in English assumes English string
lengths, line heights and glyphs. At 26 languages one layout mistake becomes "26 defects that
need to be logged, managed, and resolved," when it could have been fixed once before translation
started.

The transform, from their example:

```
Find Help Online   →   [ƒîกี้ð Ĥéļþ Öกี้ļîกี้é one two]
```

Three elements, each catching a different class of defect: the `[` `]` markers reveal
**clipping**, because a developer who cannot see both brackets is looking at a truncated string;
extended characters stress **line height and font coverage**; and the padding simulates the
**40% expansion**. The quiet fourth benefit is the best one — a string that was never
externalised **does not get transformed**, so it stays plain English and stands out on a screen
of accented gibberish. That is a free lint for the failure mode that is otherwise invisible
until a translator asks why a button has no key.

**Three details that make it work, all from their experience rather than the idea:**

- **Opt-out, not opt-in.** Netflix made it "the default development language for all UI
  developers" in debug builds. Opt-in tooling is tooling nobody runs.
- **Exclude fields already length-capped upstream** — synopses, titles — or padding produces
  false positives.
- **Metadata has to be transformed too.** Titles, cast names and ratings come from elsewhere, and
  without them "the experience felt incomplete, half pseudo localized, half English." This is
  exactly the catalogue/metadata split of [D45](#d45--metadata-is-multi-language-in-the-schema-chosen-per-request)
  and [D46](#d46--no-translated-string-is-ever-stored-in-the-database), and it confirms which
  half is harder.

**Why this is unusually cheap for us.** Netflix's own retrospective says the engineering "was not
half of the battle, I would argue it was even less — the real work starts while advocating,
influencing, educating, and convincing development teams." We have no teams to convince. We get
the expensive part of the idea at the cheap part's price, and we get it before there is a single
translated string, which is the only time it is free to adopt.

Their six-week survey found readability suffered, and they proposed replacing `one two three`
padding with vowel multiplication (`[ƒîîîกี้ð Ĥéééļþ …]`). Start there rather than repeating the
mistake.

**Status:** Provisional in form, not in principle. The exact transform should be tuned once
somebody has stared at it for a week; the default-on part is the decision.

---

## D53 — Font coverage is not assumable; the server serves fonts on demand

**Decided** (2026-09-19). Treat script coverage as a client capability that is **reported, not
assumed**. Ship no large fallback fonts in the client. When a client cannot render a script, the
server supplies the font over the same addressable-resource mechanism
[D11](#d11--subtitles-are-a-subsystem-not-a-feature) already uses for embedded subtitle fonts.

**Why.** Measured against the Android 15 Compatibility Definition Document — the document that
decides what a certified device must contain:

```
"CJK" 0 · "Chinese" 0 · "Arabic" 0 · "Hebrew" 0 · "Thai" 0   occurrences
```

It mandates Latin, Greek and Cyrillic (Latin Extended A–D, the Unicode 7.0 currency block), that
`NotoColorEmoji.ttf` not be removed, and that `sans-serif` be Roboto 2.x **"for the languages
that Roboto supports."** Nothing more. A device may ship with no CJK, Arabic, Hebrew, Thai or
Indic font and remain compliant — and on a single-market TV box, which is most of the hardware in
[test-hardware.md](../design/research/test-hardware.md), that is the likely case rather than the
edge case.

The symptom is tofu, and the user's reading of it is **"the subtitles are broken"**, not "my TV
lacks a font". It lands on us either way.

**Why not just bundle one.** Measured:

```
NotoSansCJKjp-Regular.otf   15.70 MB    one weight
NotoSansCJKjp-Bold.otf      16.24 MB
NotoSerifCJK full OTC      ~132 MB
```

~16 MB per weight per CJK variant, shipped to every user including the overwhelming majority who
will never see a CJK glyph, on platforms where app size is a real constraint and an update is a
store review.

**Why serving works.** We are the server the client is already connected to, usually over a LAN.
The client reports which scripts it can render — the same shape as
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)'s "the client is the only party that
can ask its own decoder" — and the server sends what is missing, once, cached. The mechanism
already exists: D11 serves embedded subtitle font attachments as addressable resources, and a
script-fallback font is that with a different key.

This also closes a gap in [D50](#d50--build-libass-correctly-and-turn-off-vsfilter-bug-compatibility-for-non-authored-subtitles).
Correct bidi and line breaking are worthless if the glyphs are boxes, and libass will happily
render tofu with perfect Unicode line breaking between the boxes.

Which font goes where is [D54](#d54--the-plan-b-font-is-unicode-last-resort-he-bundled-noto-is-served-unifont-is-the-middle-option).

**Status:** Provisional. Needs a real measurement of what the Fire TV sticks and the older Android
TV devices actually carry — a direct job for the device lab, and cheap: enumerate the system font
set and the covered scripts on every device in the rig.

---

## D54 — The plan-B font is Unicode Last Resort HE, bundled; Noto is served, Unifont is the middle option

**Decided** (2026-09-19). Three tiers, and the point of the decision is that they are three
rather than one:

1. **The requested font** — an embedded subtitle font, or the client's UI font.
2. **Noto, per script *and per language*, served on demand** by the server
   ([D53](#d53--font-coverage-is-not-assumable-the-server-serves-fonts-on-demand)). Which faces,
   and how they are cut down, is
   [D55](#d55--the-served-font-set-is-the-cta-708-style-axis-in-variable-fonts-subset-per-subtitle).
3. **Unicode Last Resort High-Efficiency, bundled in every client** — 588 KB, never fetched,
   never absent.

**Why there is no single answer.** OpenType caps a font at **65,535 glyphs** and Unicode 18 has
over 150,000 assigned characters, so one file physically cannot do it. The evidence is the only
project that tried: GNU Unifont splits by plane, and its BMP file reports `numGlyphs = 58,911` —
not a design choice, the ceiling arriving. Every platform therefore uses a **cascade**, and the
real question is what sits at the end of it.

**What the platforms put there.** Apple has shipped `.LastResort` since Mac OS 8.5 and selects
the rest of the cascade with `CTFontCopyDefaultCascadeListForLanguages`, which takes a **locale**.
Android does not: its `fonts.xml` is 161 families ending at `und-Yezi`, and grepping AOSP for
`lastresort` or `notdef` returns nothing — fall off the end of Android's chain and you get tofu.
Windows has `IDWriteFontFallback`; Linux has fontconfig, which is the weak link, to the point
that browsers hardcode CJK font names because "at least Debian simply doesn't have a correct
config".

**Why Last Resort HE specifically.** Measured from the binaries:

```
LastResortHE-Regular.ttf   0.56 MB    387 glyphs   1,114,112 code points mapped
LastResort-Regular.ttf     9.15 MB  5,776 glyphs   1,114,112 code points mapped
unifont + unifont_upper   10.75 MB  116,640 glyphs  ~116,600 code points
NotoSansCJKjp-Regular     15.70 MB   (one weight, CJK only)
```

The HE variant covers **every code point in Unicode for 588 KB** by using a cmap **format 13**
(many-to-one range mappings) subtable — the correct table for a font where thousands of code
points share a glyph. It renders a script-identifying character in a framed box (ක for Sinhala,
Ω for Greek), with the block name and code-point range engraved in the frame, and it
distinguishes unassigned code points and the 66 noncharacters from merely-unavailable ones.

Its documented risk is that format 13 "may not be supported in some environments, such as most
Windows and Adobe apps." **That does not reach us, and this is checked rather than assumed:**
our entire text stack is FreeType — libass links it natively and JASSUB compiles it to WASM —
and FreeType implements `tt_cmap13` in `src/sfnt/ttcmap.c` with `TT_CONFIG_CMAP_FORMAT_13`
enabled by default in `ftoption.h`. JASSUB patches FreeType only to disable pthreads.

**Why not just bundle Noto.** 15.7 MB per weight per CJK variant, shipped to every user
including the great majority who will never see a CJK glyph. And the observation that settles
it: **Netflix does not do this either.** Netflix Sans, commissioned from Dalton Maag for a
company in 190 countries, covers per its designer's own portfolio *"Cyrillic, Greek, Latin,
Latin (Vietnamese Extension)"* — no CJK, Arabic, Hebrew, Thai or Indic. Amazon Ember is the same
shape. The two largest streaming services cover exactly the scripts the Android CDD already
mandates and fall through to the platform for the rest, which means on a device without a
Japanese font, **Netflix renders tofu too**. Their remedy is commercial — relationships with
device manufacturers and preinstallation — and is not available to us. Ours is topological:
being a server on the same LAN, we can just send the font, which for our situation is better
than what they do rather than a poor imitation of it.

**Unifont is the middle option and worth keeping.** 10.75 MB for ~116,600 code points — less
than one weight of Noto CJK — dual-licensed GPLv2-with-Font-Embedding-Exception and OFL 1.1,
with the explicitly stated purpose of rendering "something meaningful for each Unicode code
point, rather than a blank box". Legible and ugly. Since we serve fonts anyway it costs nothing
to offer as a server-side option for someone who would rather read a bad glyph than a script
label.

**One thing the tiering must get right: CJK fallback is chosen by *language*, not by script.**
Han unification gives Japanese, Simplified and Traditional Chinese the same code points with
different expected letterforms — "a rough analogy is to use the same code point to encode Latin
capital A and Greek capital alpha". Apple's cascade API takes a locale for precisely this. A
script-keyed fallback renders Japanese with Chinese letterforms: not tofu, just quietly wrong,
and much harder to notice than tofu. The language is available to us —
[D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series) normalises
track language to BCP 47 and [D45](#d45--metadata-is-multi-language-in-the-schema-chosen-per-request)
knows the user's — so this is a plumbing requirement, not a research one.

**The dissent, recorded because it is credible.** Firefox refuses to use Last Resort: Mozilla
bug 1416762, *"prefer to draw hexboxes for unsupported characters"*, fixed November 2017, on the
grounds that hex boxes give "better cross-platform consistency and… actually convey more
information (the exact codepoint involved)". Right for a browser, wrong for us, and the
difference is the reader. A developer wants `U+304B`; a person on a sofa wants to know whether
the app is broken or their TV lacks a Japanese font, and a framed か answers that. So: **Last
Resort in release builds, hex boxes in development**, which pairs with
[D52](#d52--pseudo-localisation-is-the-default-development-build).

**Status:** Provisional. Two things would change it: a device-lab measurement showing FreeType
is *not* in the path on some client we care about (in which case that client takes the 9.15 MB
format-12 variant), or evidence that users find the framed-script glyph more alarming than
informative — which is a question for whoever first sees it on a TV.

---

## D55 — The served font set is the CTA-708 style axis, in variable fonts, subset per subtitle

**Decided** (2026-09-19). Adopting the proposal to host a *set* of fallback faces rather than a
single one, with the set defined by an existing standard and the CJK problem solved by
subsetting rather than by shipping.

Three parts:

1. **Six upright variable faces** covering the seven real CTA-708 styles for Latin, Greek and
   Cyrillic. **5.63 MB raw, ≈2.42 MB as woff2**, served once and cached forever.
2. **Everything else subset per subtitle track at scan time** — ~441 KB for a film's Japanese
   dialogue instead of 15.7 MB.
3. **Last Resort HE underneath**, bundled, per
   [D54](#d54--the-plan-b-font-is-unicode-last-resort-he-bundled-noto-is-served-unifont-is-the-middle-option).

**Why a standard defines the set.** "Enough faces to cover the common subtitle styles" sounds
like a judgement call, and it is not: **CEA/CTA-708 enumerates exactly eight font tags** —
undefined, monospaced serif, proportional serif, monospaced sans-serif, proportional
sans-serif, casual, cursive, small capitals — and the W3C's 608-to-WebVTT mapping gives each a
concrete CSS stack (Courier, Times New Roman, Helvetica, Arial/Swiss, Dom/Impress,
Coronet/Marigold, Engravers Gothic). Every one of those faces is proprietary, which is precisely
why an open-source server has to bring substitutes. Using the standard's axis instead of
inventing one means the set is defensible, bounded, and already matches what captioning
decoders and player UIs expose.

Two collapses take it from eight to six, both from the captioning literature and both confirmed
by measurement. **Small capitals is not a font** — "small caps can be applied to any typeface
category" — it is the OpenType `smcp` feature, and all three Noto faces report `smcp` present.
And **all eight styles are optional in decoders**, so casual and cursive can fold into the sans
face wherever no equivalent exists, which is every non-Latin script.

**Measured** (fonttools, against the real binaries):

| CTA-708 style | OFL substitute | raw | woff2 |
| --- | --- | --- | --- |
| proportional sans | `NotoSans[wdth,wght]` | 2001 K | 848 K |
| proportional serif | `NotoSerif[wdth,wght]` | 1842 K | 765 K |
| monospaced sans | `NotoSansMono[wdth,wght]` | 1668 K | 704 K |
| monospaced serif | `CourierPrime-Regular` | 69 K | 24 K |
| casual | `ComicNeue-Regular` | 55 K | 22 K |
| cursive | `DancingScript[wght]` | 130 K | 55 K |
| small capitals | the `smcp` feature | 0 | 0 |
| **total** | **6 files** | **5.63 MB** | **≈2.42 MB** |

**Variable fonts are the reason this is small.** The Noto faces carry `wght` *and* `wdth` axes,
so one file replaces nine weights × three widths. Without them this is a combinatorial problem;
with them it is six files. Real italics would add two files and take it to 10.18 MB / 4.30 MB —
worth having eventually, not needed first, because **libass synthesises both**:
`ass_glyph_embolden()` and `ass_glyph_italicize()` in `ass_font.c` apply when the face lacks the
style. (There is no `NotoSansMono-Italic` at all, and it does not matter for the same reason.)

**Subsetting is the part only we can do.** Google Fonts serves CJK by cutting fonts into ~100
fixed `unicode-range` slices because a web font server cannot know what text a page will render.
**We have the subtitle file**, so we can subset to exactly the code points it uses:

```
20,000 chars of Japanese (about one film's dialogue)  1,202 code points  →  582 K raw / 441 K woff2
21,000 chars of Chinese                               1,816 code points  →  988 K / 732 K
168,000 chars of Japanese (generous upper bound)      2,405 code points  → 1220 K / 900 K
NotoSansCJKjp-Regular, whole                         65,535 glyphs       →  15.70 M / 11.15 M
```

**36× smaller for a film.** The reason is linguistic, not algorithmic: 168,000 characters of
running Japanese contain only 2,405 distinct code points, because the Jōyō set is 2,136 kanji
and real text saturates below it.

**Where it runs: the node, at scan time.** [D11](#d11--subtitles-are-a-subsystem-not-a-feature)
already extracts subtitles at scan time in the media node, which is where FFmpeg lives. The
subset font is one more scan-time artifact beside the extracted track, and no font parser ever
enters the core's request path. **The tool is already in the build** — `hb-subset` is part of
HarfBuzz (`util/hb-subset.cc`, plus a public C API), and we ship HarfBuzz for libass
([D50](#d50--build-libass-correctly-and-turn-off-vsfilter-bug-compatibility-for-non-authored-subtitles)).
No new dependency.

**A security consequence, which happens to cut our way.** Serving fonts means that if we serve
one **extracted from a media file**, we hand attacker-supplied binary to every client, whose
FreeType then parses it — and font parsers are a classic CVE surface
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)). Subsetting improves this: the served file is
re-serialised by our subsetter rather than passed through. That is **not** a sanitiser and must
not be described as one, but the client does then parse a file our tooling produced. The
subsetter itself parses the hostile input, so it belongs in the same Landlock+seccomp jail as
FFmpeg — which is where it already is, because it runs in the node.

**What still needs a human.** The casual and cursive substitutes above are placeholders chosen
for licence and availability, not for how they look at 40 px on a television from three metres.
Comic Neue and Dancing Script cover Latin only (303 and 559 code points). Someone should look at
them on a TV before they ship, and the monospaced-serif slot deserves a second look too —
Courier Prime is a faithful Courier, and Courier is itself criticised as unsuitable for
captioning.

**Status:** Provisional. The style→face table is a configuration file, not code, so it is cheap
to revise. The subsetting numbers are measured and are unlikely to move; the per-track subset
should be validated against a real anime release with a heavy `.ass` typeset, which is the
adversarial case.

---

## D56 — Embedded fonts are off by default; the client must ask for them

**Decided** (2026-09-19). A font extracted from a media file is **not** used and **not** served
unless the client explicitly requests it. The default rendition uses our own served faces
([D55](#d55--the-served-font-set-is-the-cta-708-style-axis-in-variable-fonts-subset-per-subtitle)).
The manifest *advertises* that a track carries embedded fonts; acting on that is the client's
call.

**Why the default matters more than the capability.** Media is attacker-supplied data
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)), and a font is a
program in the sense that matters: FreeType interprets it. **Two FreeType bugs sit in CISA's
Known Exploited Vulnerabilities catalogue** — confirmed exploited, not theoretical:

| CVE | CVSS | KEV added | Surface |
| --- | --- | --- | --- |
| CVE-2020-15999 | 9.6 | 2021-11-03 | Heap overflow in `Load_SBit_Png` — **PNG embedded inside a font**; CISA notes it as part of an exploit chain |
| CVE-2025-27363 | 8.1 | 2025-05-06 | Out-of-bounds write in **TrueType GX / variable-font subglyph** parsing; arbitrary code execution, FreeType ≤ 2.13.0 |

The second is from **March 2025**. And ≤ 2.13.0 describes exactly the old Fire TV sticks and
Android TV boxes that [D53](#d53--font-coverage-is-not-assumable-the-server-serves-fonts-on-demand)
already identified as the weak devices. The server is an **amplifier**: one bad file reaches
every screen in the household, including the ones nobody updates and cannot update.

**The commercial players do not accept fonts at all.** Netflix's IMSC 1.1 Text Profile:
`tts:fontFamily` *"Should not be used. If specified, the tts:fontFamily should be equal to
`proportionalSansSerif`."* One generic family, resolved by the player — from a company with
**contracts with the studios supplying the content**. If Netflix will not take a named font from
a vetted partner, taking an arbitrary binary from a torrent by default needs an argument nobody
has made.

**The open-source field does the exact inverse**, and it is worth seeing how. `jellyfin-web`'s
`renderSsaAss()` hands every attachment to libass whose **`MimeType` looks like a font** — and
`MimeType` is metadata from inside the container, written by whoever made the file. Their own
comment says the filter exists to skip cover art, not to screen anything. Meanwhile the
*trusted* fallback font set is behind `config.EnableFallbackFont`, **default off**. Untrusted
source on by default, trusted source off by default. We are inverting that.

**What we lose, stated honestly.** Heavily typeset `.ass` — fansubbed anime especially — assumes
its bundled fonts, and ASS positioning is computed against specific font metrics, so substituting
does not merely change the look: signs can overlap, karaoke can drift, and line breaks move. That
audience is precisely the one [D11](#d11--subtitles-are-a-subsystem-not-a-feature) says we must
win. So this is a real cost, and the mitigation is **not** to weaken the default but to make the
opt-in good: the client shows that a track has embedded fonts, the user turns them on, and the
choice sticks per series like every other track preference
([D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)). One
deliberate action, remembered — not a dialog per episode.

Two further reasons the client is the right decider, both matching
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest). **The risk is wildly
uneven across clients** — JASSUB's FreeType runs inside a WASM sandbox in the browser, while
libmpv's runs in-process and unconfined on Android and Apple — and only the client knows which
it is. And only the client knows its own FreeType version.

**When it is opted in, the path is still hardened.** The
[D55](#d55--the-served-font-set-is-the-cta-708-style-axis-in-variable-fonts-subset-per-subtitle)
subsetting pass is where the dangerous parts come off, and `hb-subset` has the controls already:

- `HB_SUBSET_FLAGS_NO_HINTING` drops `fpgm`/`prep`/`cvt ` — the **TrueType hinting bytecode**,
  a stack VM that FreeType interprets and a classic exploitation target.
- `--drop-tables+=CBDT,CBLC,sbix,SVG,EBDT,EBLC,COLR,CPAL` removes the embedded bitmap and colour
  tables — **where CVE-2020-15999 lived** — which a subtitle font has no use for.

Measured, and this is the unusual part:

```
CJK subset, as before                            582 K
CJK subset, --no-hinting                         498 K
CJK subset, --no-hinting --drop-tables+=…        498 K
```

**Hardening makes the output 14% smaller.** No trade-off to weigh, so apply it to every font we
serve, embedded or not.

Also: **never trust the container's declared MIME type.** Sniff the actual format, cap the
attachment size, cap the glyph count, and reject anything that is not a font we can parse — in
the node, before it is ever offered.

**What this does not claim.** Re-serialisation is **not** sanitisation. `hb-subset` parses the
hostile input with HarfBuzz, which has its own bug history, and a font crafted to attack the
*subsetter* is a live possibility. What it buys is that the client parses a file our tooling
emitted, with the hinting VM and bitmap tables gone, rather than the original bytes — and that
the parsing of hostile input happens in the node, inside the same Landlock+seccomp jail as
FFmpeg, rather than on someone's television.

**Status:** Provisional. The scope question it left open — per-user, per-library or per-server —
is **resolved by [D57](#d57--the-embedded-font-opt-in-is-per-user-sticky-per-series-and-the-client-may-still-refuse)**.
The default is not provisional.

---

## D57 — The embedded-font opt-in is per-user, sticky per series, and the client may still refuse

**Decided** (2026-09-19). Resolves the scope question left open by
[D56](#d56--embedded-fonts-are-off-by-default-the-client-must-ask-for-them). The opt-in is
**per user**, remembered at **series** scope, and it is a *preference* — the client still decides
whether to honour it on the device in front of it.

**It needs no new machinery.** This is precisely
[D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s
track-preference model with one more axis, and every rule there transfers unchanged:

- **Sticky scope is the nearest grouping ancestor** — the series for an episode, the item itself
  for a film. Exactly as D24 defines it.
- **Only an explicit user action updates the preference.** Nothing automatic may turn embedded
  fonts on, or the setting drifts toward whatever the files happen to contain — which here is
  not merely untidy, it is the security property quietly eroding.
- **Tri-valued: `on` / unset / `off`.** Unset means "ask"; `off` means the user said no and must
  not be asked again for that series. A user who declines once should not be nagged every
  episode.
- **Store a descriptor, re-resolve per file.** Never "use attachment index 3" — attachment
  indices shift between episodes the same way track indices do, which is the bug class D24
  exists to prevent.

The resolution chain is D24's, shortened, because there is deliberately **no per-library
default**:

```
explicit choice in this session
  → remembered choice for this series     ← the user's ask
    → off                                 ← the root default (D56)
```

**Why no per-library scope, when D24 has one.** A library default is how you would say "anime:
always load embedded fonts", and that is precisely the blanket grant D56 exists to prevent — it
converts a deliberate per-series decision into a standing permission over every file that lands
in a folder, including ones acquired later. Language preferences earn a library default because
getting them wrong is an annoyance; this one does not, because getting it wrong is a code
execution path. The asymmetry is the point.

**Why per-user rather than per-server.** The person who accepts the risk should be the person who
bears it. A server-wide switch means one household member's taste in fansubs decides what runs on
everyone else's television. It also keeps the blast radius legible: when something does go wrong,
"who turned this on" has an answer.

This has a consequence worth stating explicitly: **the served font resource must be authorised
per user**, not merely unguessable. If A opts in and B does not, B's client must not be able to
fetch the attachment — so the URL carries A's authorisation, not just a hash. That folds into the
existing signed-and-scoped-URL work rather than adding to it.

**And the client may still refuse, which is the part that matters most.** The preference says
*"this user wants correct typesetting for this series."* It does not say *"this device should
parse it."* Those are different claims and
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) already separates them:
preference is user state, capability is device state, and the device decides.

The gap is large and real. In a browser, JASSUB's FreeType runs inside a WASM sandbox. On Android
and Apple, libmpv's runs in-process and unconfined, and may be FreeType ≤ 2.13.0 — the range
CVE-2025-27363 covers. So one user preference produces the right behaviour on both: the browser
honours it, and an old TV box declines and says why, without the user having to keep two settings
or understand the difference. The client that declines should say so in the same
"why is this different?" surface that explains transcoding decisions
([principles §3](../principles/README.md)) — *"this subtitle's own fonts weren't loaded on this
device"* — rather than silently rendering substituted metrics and letting the user wonder why the
signs overlap.

**Status:** Provisional in one detail. A per-device override — "never on this box", or conversely
"I know, do it anyway" — is probably wanted eventually, and is a strictly additive change to the
same chain. Not building it now. **The authorisation framing in this entry is corrected by
[D58](#d58--dont-hand-out-a-url-the-client-may-not-use-an-authorisation-check-that-fires-is-a-signal)**
— omission from the manifest is the mechanism; the server-side check is a backstop.

---

## D58 — Don't hand out a URL the client may not use; an authorisation check that fires is a signal

**Decided** (2026-09-19). Correcting the framing in
[D57](#d57--the-embedded-font-opt-in-is-per-user-sticky-per-series-and-the-client-may-still-refuse).

D57 said the served font "must be authorised per user, not merely unguessable" and that a
non-opted-in user's client "must not be able to fetch" it. That describes a server that **says
no a lot**, which is the wrong design. **A client asking for something it is not entitled to is
a bug**, and a design where that happens routinely cannot tell a bug from an attack.

**The mechanism is omission, not rejection.** A user who has not opted in gets a manifest that
**does not contain the embedded-font resource at all**. There is no URL to fetch, so there is
nothing for a correct client to decline to request — the illegal action is unreachable rather
than forbidden. This is just
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)'s model applied honestly: the
manifest is the menu, and you do not put a dish on the menu that the customer may not order.

**The server-side check still exists**, because defence in depth, and because a client is not a
trust boundary. But its role changes completely:

| | Routine control flow | Backstop |
| --- | --- | --- |
| Fires when | normal use | never, in a correct system |
| A rejection means | "working as intended" | **a bug, a stale client, or an attack** |
| Can you alert on it | no — it is noise | **yes** |

That table is the whole value of the correction. An enforcement check that fires during normal
operation is not enforcement, it is control flow wearing a security costume; it generates
exactly the traffic that would hide a real attempt. Move the decision into what the manifest
contains and the check goes quiet — and a quiet check is one whose every firing is worth
looking at.

**The general rule, because this is not about fonts.** It applies to signed and scoped media
URLs, the transcoder plugin's capability grants ([D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)),
the Jellyfin adapter, and every API surface where our own client is the caller:

> **Give a client only what it may use, and treat the refusal path as an alarm rather than a
> branch.**

**One distinction that must not be lost**, or this rule does real damage: it governs
**authorisation of our own cooperating client**, not **validation of untrusted input**. Node
reports are expected to arrive malformed sometimes and the core is expected to reject them
([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global),
[D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides)) — that check *should*
fire, routinely, and alerting on it would be nonsense. Media is attacker-supplied and FFmpeg's
confinement is expected to be load-bearing
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)). Conflating
"validation of hostile data" with "authorisation of a first-party client" is how a project ends
up with no signal in either: the security events drown in the normal ones.

**Practical consequence for D57.** The per-user opt-in state decides **manifest content**, not
just request handling. That is a small implementation difference and a large operational one,
and it is cheaper to build this way than the other way.

**Status:** Provisional only in how the alarm surfaces — a log line, a counter, or something the
user sees. Promoted to [principles §10](../principles/README.md), because it decides arguments
well beyond this one.

---

## D59 — A node publishes a snapshot plus a journal; it observes, it does not interpret

> **Amended 2026-09-20 by [D116](#d116--the-content-hash-is-a-sampling-hash-whole-file-hashing-is-opt-in-and-separate).**
> "Where cheap, a content hash" now has a measurement behind it: the default is a **sampling
> hash** (head, tail and size — 3.2 ms and 2 MB of reads), not a whole-file hash (0.49 s and
> 8 GB of reads per film). Whole-file hashing is a separate, opt-in pass for duplicate
> detection.

**Decided** (2026-09-19). Correcting [scanner.md](../design/plans/scanner.md), whose Shape
section still described Navidrome-style pipeline phases — *enumerate → detect missing/moved →
**identify and enrich** → **resolve playlists/collections*** — written on 2026-09-17, before
[D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) moved the
scanner into the node and before
[D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides) made nodes sensors. The
last two of those phases are interpretation, and the document was never re-read after the
relocation.

### What a node actually has, and therefore what it can say

A node sees the directories and files under its roots, and nothing else. It has no library, no
user, no history beyond what it has itself observed, and no idea that the file it is looking at
is episode 4 of anything. So the only thing it can honestly offer is **a temporal view of a
subtree**: what is there now, and what has changed since.

That is a **snapshot plus a journal**:

- **Snapshot** — a complete enumeration of the node's roots at a point in time. This is
  [D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)'s
  report, renamed for what it actually is, keeping every property it already had: JSONL,
  header/entries/trailer, completeness in the filename via `.partial` → atomic rename, and
  completeness **scoped per root and per errored subtree**.
- **Journal** — append-only observations since that snapshot, one line each, same record
  grammar, same error and skip records with their scopes ([D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope)).

Both carry a **monotonic sequence number**, and the snapshot records the sequence it corresponds
to. The core tracks the last sequence it consumed. If the oldest journal line the node still has
is newer than that, there is a gap, and the core falls back to the snapshot.

**The snapshot is always a valid recovery point, so the node never has to know what the core has
read.** That is the property that keeps this inside D42: no back-channel, no acknowledgement
protocol, no node-side model of core state. Compaction is a node-local storage decision, and
the worst consequence of getting the bound wrong is that the core re-reads a snapshot.
**How that bound is chosen is [D60](#d60--snapshotting-and-discarding-are-different-operations-with-different-triggers)**,
which corrects this entry's conflation of the two.



### The snapshot is not only compaction; it is the only thing that corrects a missed event

Worth stating because it sets the interval. The journal is as good as the watcher that feeds it,
and [D27](#d27--change-detection-is-a-strategy-interface-that-reports-which-strategy-it-is-using)
established that watchers **fail silently** — `inotify` does not work over SMB or NFS and has
watch limits a large library exhausts. A journal cannot show you the event nobody delivered.

A snapshot enumerates, so it catches everything the journal missed. Compaction and correctness
turn out to want the same mechanism, which is convenient, but the correctness requirement is the
one that sets the floor: **periodic full snapshots are mandatory even with a perfect watcher**,
because you cannot know the watcher is perfect.

### The node does not parse filenames

A filename is **reported verbatim** — NFC form plus raw bytes ([D49](#d49--unicode-discipline-at-the-boundaries-enforced-by-tests)) — and never parsed into series, season or episode by the node.

The §9 test says why: the *right* parse depends on the library it lands in, on whether this
series already exists with a known convention, on whether absolute numbering is in play. The
node has none of that.

But the decisive reason is practical and has nothing to do with purity. **The parser will get
better.** If the node parses, then improving the parser means re-scanning every node across
every filesystem, including the NAS over SMB and the one that is currently offline. If the core
parses stored raw paths, improving the parser is a re-interpretation pass over the database that
touches no filesystem at all. That is the same argument as
[D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise)'s rebuildable index, and it
is worth a great deal the first time a regex is wrong.

So the filename parser, its Unicode-safe corpus, and the derivation of track *kind* from title
strings ("English (SDH)", "Signs & Songs") all move to the **core's identification stage**.

### But the node absolutely does look inside the file

This is the other half and it is not a contradiction. The extension is a **hint about whether
probing is worthwhile**, never a conclusion about content, because people misname things
constantly. The node probes and reports what is actually there: container, streams, codecs,
durations, disposition flags, track titles, attachment list.

That is observation and it stays in the node — the core never opens a media file
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)). **When the probe
disagrees with the extension, report both.** The disagreement is itself a fact, and what it
means — a mislabelled file, a corrupt one, a deliberate container swap — is the core's call.

Canonicalisation that is lossless and context-free stays in the node, reported **alongside the
original**: a track's BCP 47 tag beside the raw `ger`/`deu` the container carried
([D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)), the NFC
path beside the raw bytes. Anything that is a *heuristic over text* is interpretation and goes
to the core, by the improve-later argument above.

### Report the event you were given; do not synthesise linkage

A node must not emit "this file moved from A to B" when what it saw was one path disappear and
another appear. Deciding those are the same file requires hash or inode history the core holds.
Report `A gone` and `B appeared`; the core decides it was a move.

The converse matters equally: when `inotify` hands over a `MOVED_FROM`/`MOVED_TO` pair sharing a
cookie, **the kernel linked them and reporting the link is reporting what you saw.** The rule is
not "never emit a move", it is *never invent a relationship the source did not give you*.

### Node-local state may change cost, never content

The node needs a probe cache keyed by path, size and mtime, or every snapshot re-probes fifty
thousand files. §9 warns specifically against giving a sensor a memory, so the boundary needs
stating:

> **Delete the node's local state and it must report exactly the same thing, only slower.**

A cache that can change *what* is reported has stopped being a cache and become judgement. This
is the test to apply whenever node-local storage is proposed.

### What happens to "phases"

The legibility goal behind them was real — *"logging around library scanning being pretty
difficult to interpret"* is a direct Jellyfin complaint — but it is already met by
[D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)'s status record
and [D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config)'s
`state`/`task` pair. Named activities survive as **progress labels**, not as a pipeline whose
later stages draw conclusions.

**Status:** Provisional. The compaction bound — maximum journal length before a fresh snapshot —
is unknown and needs measurement against a real library and a real watcher; it joins the other
threshold guesses in [performance-targets.md](../design/plans/performance-targets.md). This
**extends rather than supersedes D39**: every property of the report survives, applied to the
snapshot.

---

## D60 — Snapshotting and discarding are different operations with different triggers

**Decided** (2026-09-19). [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret)
said "a maximum journal length or size, after which a fresh snapshot is written and older pairs
may be dropped." That sentence conflates two operations that want opposite trigger conditions.
Separating them:

| Operation | Trigger | Why |
| --- | --- | --- |
| **Take a snapshot** | **temporal** — every `snapshot_interval`, plus on request | It is the correction for a silently-failed watcher, so its cadence is a correctness floor, not a housekeeping one |
| **Discard journal entries** | superseded by a snapshot **and** older than `journal_retention` | Retention is what buys the core time to catch up |
| **Emergency discard** | `journal_max_bytes` exceeded | A safety valve against disk exhaustion, and **loud** |

A snapshot no longer implies discarding anything. That alone removes most of the hazard.

### Why size is the wrong primary trigger

The obvious reason is the stated one: a large filesystem change can blow a size bound in
seconds and erase journal the core has not read. A core that is simply **switched off for the
weekend** — an ordinary thing for a home server — plus a torrent client finishing a season pack
is enough.

But the sharper reason is about *when* it fires. **The burst that blows a size bound is, almost
by definition, the anomalous event** — a mass deletion, a re-organisation, a share coming back
with different contents, a permissions change. Size-triggered compaction therefore forces a
fresh full enumeration at precisely the worst moment, and replaces an ordered record of what
happened with a single point-in-time reading taken mid-disruption. It is the backup that runs
during the incident.

Temporal triggering **decorrelates** the measurement from the events being measured, which is
the property you want from any instrument.

And the economics run the right way round. **A quiet library produces almost no journal**,
because a journal line is a changed file. So temporal retention is nearly free in the common
case and only costs during churn — which is exactly when the history is worth having. A size
bound is backwards: it is most restrictive precisely when there is most to say.

### Being honest about what this buys

It is **not** correctness of the library. The snapshot is a complete, scoped enumeration
([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)), so a
core that loses journal still converges — it just re-reads everything. The protections against a
misleading snapshot live in the core's own history and in
[D41](#d41--diagnose-the-failure-boundary-by-walking-up-and-guard-the-silent-case-which-is-worse)'s
observed-empty-versus-not-enumerated distinction, not in the journal.

What it buys is three real things: **diagnosis** — fifty thousand ordered, timestamped events
distinguish "the user re-organised their library" from "the mount flapped" in a way two
snapshots six hours apart never will, and that is the question
[D41](#d41--diagnose-the-failure-boundary-by-walking-up-and-guard-the-silent-case-which-is-worse)
exists to answer; **cost** — re-reading a 48,000-line snapshot over SMB is not free; and **not
taking an expensive measurement during an anomaly**.

Overstating it as a data-loss fix would be wrong, and the design is easier to reason about with
the actual reason written down.

### Pure temporal has its own failure mode, hence the valve

"Retain for seven days" meets a rename loop, a misbehaving sync client, or a filesystem doing
something pathological, and the node fills its own disk. That is **worse** than a discarded
journal: a node that cannot write cannot write its status record
([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)) either, so it
stops looking wedged and starts looking dead.

So `journal_max_bytes` exists as a backstop — and per
[principles §10](../principles/README.md), a backstop that fires is a signal, not a branch.
Hitting it means the node emits an `error`-class record saying it discarded history, how much,
and over what span. The core surfaces that; it does not silently absorb it.

When the valve trips the node **takes a snapshot and keeps as much journal as the budget
allows**, rather than discarding first. Snapshot-then-trim, never trim-then-snapshot.

### Defaults, and who sets them

`snapshot_interval` is a correctness parameter and wants to be short enough that a silently
dead watcher is caught within a day. `journal_retention` wants to be long enough to survive a
plausible core outage — days, not hours — and is nearly free when the library is quiet.

The node must **not** derive these from the core's polling behaviour. That would be the node
modelling core state, which [D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval) already
rejected for the cadence question, and a node under stress is not a reliable source for the
standard it is judged against. They are config fields
([D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)),
with defaults the user can raise.

**`snapshot` joins the command vocabulary.** [D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config)
already defines an ordered command list — `cancel`, `scan`, `rescan <root>`, `pause`, `resume` —
so a core that wants a fresh enumeration now can ask, and one that knows it has fallen behind
can simply not ask. This is a *request for an action*, not the core dictating retention policy,
so it stays within D42.

**Status:** Provisional in its numbers, which are guesses until measured against a real library
and a real watcher — they belong with the other thresholds in
[performance-targets.md](../design/plans/performance-targets.md). The **separation** of the two
triggers is not provisional; it is a correction.

---

## D61 — Containment now, detection later and plugin-shaped; four affordances to keep open

**Decided** (2026-09-19). Proactive threat scanning of media, subtitles, fonts and posters is
**not for the first release**, and the research says its eventual shape is different from the
obvious one. What we do now is keep four seams open, all at near-zero cost, and avoid the two
mistakes that would close them.

### The surface is real and it is not only video

39 entries in CISA's Known Exploited Vulnerabilities catalogue touch media, image or font
parsing — Apple's Image I/O (CVE-2025-43300, August 2025), Samsung's image codec twice in 2025,
FreeType twice. So including subtitles, fonts and posters rather than only video is correct. And
posters are the one binary we fetch **from a third party by default** — TMDB — rather than from
the user's own disk.

### But every platform that has faced this chose containment over detection

**Apple built BlastDoor** after the 0-click iMessage attacks: it *"isolates, parses, transcodes,
and validates untrusted data"* with *"sandbox restrictions and memory safe validation of
output"*, almost no filesystem access and no network. Apple's own description mentions **no
signature scanning or content detection at all**. **Netflix's** published media tooling (Photon)
is *conformance* validation against SMPTE ST 2067-2, not threat detection. **Google's** answer is
fuzzing plus sandboxing — and **FFmpeg, FreeType, HarfBuzz and libass are all in OSS-Fuzz**, so
our entire parsing stack is already continuously fuzzed by a better-resourced programme than we
could run.

That last point reframes the investment. **Shipping current builds promptly beats scanning**,
and it is cheaper. A concrete deliverable falls out of it: the server should know which FFmpeg,
libass and FreeType it is running and tell the user when they are stale — which is the same
information [D53](#d53--font-coverage-is-not-assumable-the-server-serves-fonts-on-demand) wants
about client FreeType versions anyway.

### The scanner is a file parser, which is the thing we are defending against

ClamAV has **158 CVEs**, 29 since 2023, **seven on a single day** (2026-08-07) — XAR, Mach-O,
PDF, GPT, PESpin and two zip parsers, each "an unauthenticated remote attacker could…" from
parsing a file. Adding a scanner to defend FFmpeg means adding a component with a *worse*
parser-CVE record and pointing it at the same bytes.

This does not forbid it. It settles the shape: **any scanner is another hostile-input parser —
tier-3 plugin, inside the same jail, no additional privilege, never a trusted security
component.**

### "Look for shellcode" is the part that does not survive

Saying so plainly because it is the most tempting item. Modern exploits often contain **no
shellcode at all** — ROP and JOP chains are assembled from gadgets already in the process, and
data-only attacks corrupt state without injecting code; a current FFmpeg exploit is typically a
malformed header causing a type confusion, with nothing byte-like to match. And **compressed
video is high-entropy and statistically indistinguishable from packed or encrypted payloads**,
so the classic entropy tell is a 100% false-positive rate on our actual corpus.

The intent — *notice that this file is trying something* — is right. The mechanism below is
better.

### The four affordances to keep open now

Each costs approximately nothing today and is expensive to retrofit.

1. **Unknown record kinds must be skipped, not fatal.** This is the load-bearing one. Reports are
   JSONL and degrade per line ([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)),
   and the protocol is versioned ([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)).
   Make it an explicit rule **now** that the core ignores record kinds it does not recognise,
   counts them, and does not treat the report as malformed. Then an `anomaly` record can be added
   later without a protocol break or a node/core upgrade ordering problem.
2. **The jail must be able to report, not only kill.** seccomp already has the primitives:
   `SECCOMP_RET_LOG` (allow after logging), `SECCOMP_RET_TRAP` (catchable `SIGSYS`, so a worker
   can report before dying), `SECCOMP_RET_USER_NOTIF` (notify a supervisor that can decide), and
   `SECCOMP_FILTER_FLAG_LOG`. Choosing a filter action that can be observed costs nothing now,
   and it means **our sandbox is already a detector and we are simply not reading it**. A file
   that makes FFmpeg attempt an unexpected syscall is stronger evidence than any signature will
   ever be.
3. **Record per-file resource peaks while probing.** The realistic threat to a home server is
   **denial of service, not RCE** — ASS render bombs (Jellyfin ships `libassMemoryLimit` and
   `libassGlyphLimit` for exactly this), decompression bombs in WOFF2/brotli fonts and PNG
   posters, files that pin a CPU forever. We already have cgroups
   ([D43](#d43--one-binary-one-process-per-role-and-a-thin-optional-supervisor),
   [D44](#d44--prefer-the-platforms-supervisor-ship-the-units-and-never-exit-when-you-could-report)), so peak
   CPU, memory and output size per file is free, and an outlier is visible with no new machinery.
4. **Quarantine is a library state, never a file operation.** Never move, never delete, never
   modify — [D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index) and
   [principles §2](../principles/README.md). A suspected file is *marked and not played by
   default*; the bytes are untouched.

### And the two mistakes to avoid

**The node must never conclude "malicious."** It reports observations — seccomp violation,
parser crash, declared type ≠ sniffed type, 1.2 GB decompressed from 40 KB — and the core
decides what they mean, per [D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides)
and [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret).
A node that quarantines is a sensor making policy.

**Do not cry wolf.** These are the user's own files, acquired over fifteen years, many encoded by
broken tools. A server that warns about a 2004 home-video rip has not made anyone safer; it has
taught them to dismiss warnings, which is a security failure and not a cosmetic one.
[Principles §3](../principles/README.md) requires explaining the decision, and a warning the user
cannot act on is not an explanation. Default silent; surface only concrete observations; always
say what to do. *"This file used 40× the memory of anything else in your library while probing"*
is actionable. *"Possible shellcode detected"* is not.

**Status:** Provisional and deliberately unscheduled — see
[content-safety.md](../design/plans/content-safety.md). The four affordances are not
provisional; they are cheap now and awkward later, and item 1 in particular is a wire-format
rule that has to exist before the first node ships.

---

## D62 — Watch Together is a first-class feature, and the per-client surface is six methods

**Decided** (2026-09-19). Build synchronised remote playback with shared controls: anyone in the
group may play, pause and seek; volume stays local. Research:
[watch-together.md](../design/research/watch-together.md).

### Why this is a differentiator rather than a nice-to-have

**Almost every commercial implementation has been shut down.** Disney+ GroupWatch removed
2023-09-18; Amazon Prime Video Watch Party removed 2024-04-02 with no announcement; Sling early
2024; Netflix never shipped one at all; only Hulu survives. And **Plex removed Watch Together on
2025-02-25** for every platform except web.

Plex's own wording names the cause, and it is neither licensing nor lack of demand:

> While this feature won't be available for **most platforms as they get the new experience**,
> you can continue using the feature in our web app…

**It had to be implemented in every client, and it did not survive a client rewrite.**

Demand is not in doubt: **Teleparty**, a browser extension that exists solely because the
platforms will not do this, has **10,000,000 users**. The incumbents abandoned a wanted feature
for a reason we can engineer around, which is the best kind of opening.

### The requirement that follows directly from how theirs died

> **The client-specific surface must be small enough to reimplement in an afternoon.**

Everything else — clock-offset estimation, drift policy, group state, suspensions, command
sequencing, reconnection — lives in the shared TypeScript SDK that
[web-client.md](../design/plans/web-client.md) already calls the real deliverable. The client
implements six methods:

```
currentPosition() → time      seek(time)      play()
isBuffering() → bool          setRate(r)      pause()
```

A client that can do those six gets watch-together, and a client rewrite cannot casually drop
the feature because there is nearly nothing in the client to drop. This is the whole lesson from
five dead implementations, and it is a constraint on our own roadmap as much as a design note.

### The sync mechanism, taken from Jellyfin because it works

Jellyfin's SyncPlay is 7,635 server-side lines and the mechanism is sound:

- **Clock offset by NTP-style four-timestamp exchange**, keeping a rolling window and using the
  **minimum-delay sample**, because the least-delayed sample carries the least queuing noise.
  Greedy polling at 1 s initially, 60 s thereafter.
- **Two correction strategies with a threshold between them**: nudge `playbackRate` for about a
  second to absorb roughly 60–3000 ms of drift; seek for anything larger.
- **A four-state group machine**: `Idle`, `Waiting`, `Paused`, `Playing`, where `Waiting` means
  paused-until-everyone-is-ready.

Their `TODO` comments are the caveat and we should not discover it again the hard way:
*"SpeedToSync is failing on Safari"*, *"a hard time keeping up on Android Chrome"*. Rate nudging
is inconsistent per platform, so `setRate` is **optional** in the six-method contract and
seek-only is the always-available fallback. Whether rate nudging works on our clients is a
device-lab measurement, not an assumption.

### Suspensions with reasons, which is Apple's idea and the best one here

A participant temporarily detaches from group coordination, **stating why**; while suspended it
neither obeys nor issues group commands; several can be active at once; when the last clears it
rejoins. Apple enumerates `stallRecovery`, `audioSessionInterrupted`, `playingInterstitial`,
`userActionRequired`, `coordinatedPlaybackNotPossible` and — the important one —
**`userIsChangingCurrentTime`**.

That last is what makes shared seek control usable at all: **while you are dragging the scrub
bar you are suspended**, so your intermediate positions do not drag everyone else across the
timeline. Without it, "anyone can seek" is unusable, and it is not a thing one would think of
before shipping it badly once.

A reason is also better than a flag for the UI, per [principles §3](../principles/README.md):
"Karl is buffering" is something a person can wait for.

### Concurrent commands are normal, so the server serialises

"Anyone can control it" means two people pausing simultaneously, or one seeking while the other
resumes, is ordinary traffic rather than an error. The server is already in the path: each
command carries the group-state version it was issued against, the server assigns a monotonic
sequence, and clients reconcile **to the server**, never to each other. Commands are therefore
ordered and idempotent, which is what stops the both-pause case oscillating.

**Sync does not go over a WebRTC data channel.** `react-native-webrtc` does not support data
channels on tvOS at all, and it would make synchronisation depend on peer connectivity. It goes
over our own WebSocket, which is also the serialisation point.

**Status:** Provisional. Two questions are unanswered and both are group-size dependent: whether
one participant stalling pauses everyone (Jellyfin makes it a flag; obviously yes for two,
obviously no for eight), and whether the subtitle track is group state or local — one person may
need subtitles, and "seek to the funny bit" and "turn subtitles on" then behave inconsistently.

---

## D63 — Voice chat is WebRTC, and headphones are the supported configuration

**Decided** (2026-09-19). Voice chat over WebRTC, peer-to-peer where possible, with per-source
gain so one loud participant does not drown the film. **Headphones are the supported
configuration and the UI says so.** Speakers are best-effort.

### The hard part is echo cancellation, not the plumbing

The film is playing out loud while the microphone is open. Acoustic echo cancellation works by
subtracting a **reference signal** — what was sent to the speaker — from what the microphone
hears, and the AEC only gets a reference for audio it knows about. **Only the official playback
paths serve as references; custom playback through the Web Audio API does not reliably function
as one.** On native clients the film is decoded by libmpv, ExoPlayer or AVPlayer and reaches the
output by a path the WebRTC stack has no reference for at all.

So without headphones, **your microphone sends the film back to the other person**, delayed and
doubled against their own copy. That is not a tuning problem, and Bluetooth output makes it
worse because AEC needs the reference and the echo time-aligned while wireless adds variable
latency.

Saying "use headphones" is not a cop-out; it is what every voice product assumes in practice,
and with headphones at both ends there is no acoustic path and the entire class of problem
disappears.

### Ducking conflicts with AEC, and the resolution is a rule

Ducking the film when someone speaks means routing the film's audio through a mixer we control —
in a browser, the Web Audio API — which is **exactly what stops it being a usable AEC
reference**. So:

- **Headphones, no acoustic path:** route through Web Audio, duck freely, AEC irrelevant.
- **Speakers:** leave the film on the native path so AEC keeps its reference; accept manual
  levels instead of automatic ducking.

Per-source gain is independent of both and is simply a gain node per remote stream.

### On a television, the phone is the voice endpoint

Measured from the `react-native-webrtc` capability matrix: tvOS has audio/video but **no data
channels**, needs a locally built `WebRTC.framework`, and carries an open tvOS 16 compatibility
issue. Separately, an Apple TV has no app-accessible microphone and neither do most Android TV
boxes — and a microphone across the room from the speakers is the worst possible AEC geometry
even if one existed.

Phone-and-earbuds for voice, television for video. It is the only workable configuration and it
is already a familiar pattern.

**Status:** Provisional. **Extended by [D65](#d65--playback-offset-is-per-participant-and-it-is-an-echo-control)**,
which adds two tiers below headphones and makes speakers genuinely workable rather than
best-effort. Push-to-talk is the guaranteed answer for the speakers case and is unromantic for
the two-people-watching-a-film case this exists for; worth offering, wrong as a default. Text chat is deferred — and the honest note is that on a TV it is nearly useless, no
keyboard and one screen, which is presumably why Disney+ GroupWatch shipped emoji reactions
instead.

---

## D64 — Watch Together makes the remote-access question concrete, and supplies the premium tier's first honest member

**Decided** (2026-09-19). Watch-together is the use case that forces
[D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)'s open product
question, and it resolves *what the paid tier is for* even though it does not resolve whether we
operate one.

Three things are needed for the actual scenario — one participant at home, one travelling — and
they have very different costs:

1. **She must reach the media at all.** Already the project's stated open question
   ([remote-access.md](../design/research/remote-access.md)). Watch-together adds no new
   requirement here; it just makes the absence conspicuous.
2. **A rendezvous for synchronisation.** **Free.** If she is connected to the server, the server
   *is* the rendezvous, and [D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
   sync channel is the connection that already exists. No new infrastructure.
3. **A path for voice.** Peer-to-peer where NAT allows — remote-access.md's research puts direct
   connection above 90% with Tailscale-style hole punching — and a **TURN relay** for the
   remainder.

That third item is the point. A TURN relay is **unambiguously another machine that somebody must
run**, with bandwidth costs that scale with use, and it cannot be self-hosted by the person who
needs it, because the whole reason it exists is that their network cannot be reached.

Which makes it the cleanest possible test of the README's promise: *"we only charge for things
that genuinely require another machine."* Most candidate premium features fail that test on
inspection. This one passes it without argument, and the ~10% figure means the free path is the
common path rather than a crippled teaser.

**What this does not settle:** whether we operate a service at all. That still implies a
company, uptime obligations and an entity that can be pressured, and declining it remains
coherent — the cost is leaving the headline use case to Tailscale, which is a fine answer for
this project's likely early audience and a poor one for the household that motivated it.

**Status:** Provisional, and the least technical entry in this log. What it fixes is the *shape*
of the answer, not the answer. **Corrected by [D68](#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay)**
— voice needs no separate relay, because the media server is already mutually reachable. The
premium tier's justification is remote access itself, not voice.

---

## D65 — Playback offset is per-participant, and it is an echo control

**Decided** (2026-09-19). The group's shared state is a position **plus a per-participant
offset**. Target position for participant *i* is `P + offset_i`, default zero. This started as a
field observation and turns out to have a derivation, an optimum, and an automatic
configuration.

### The observation

Watching with two players and voice over WhatsApp, the echo was fixed by **deliberately delaying
one side's playback** so the leaked film audio arriving over the voice channel lined up with the
local copy. No implementation in the survey does this — Jellyfin, SharePlay and Teleparty all
target identical positions, which is the configuration that **maximises** the echo.

### Why it works, and where the optimum is

Let participant A be ahead of B by `d`, and let one-way voice latency be `L`.

B hears their own film at content position `p_B(t)`, and hears A's leaked room audio captured
`L` ago, which was at `p_A(t−L) = p_B(t) + d − L`. So:

```
misalignment at B  =  L − d      → zero when d = +L   (A ahead by L)
misalignment at A  =  L + d      → zero when d = −L   (A behind by L)
```

Three consequences, and the third is the useful one:

- **Both cannot be zero.** At `d = 0` — what everyone builds — *both* participants suffer a full
  `L` of misalignment. Zero offset is not neutral; it is the worst symmetric choice.
- **Delaying yourself fixes the echo you hear**, at the other person's expense. Which is exactly
  the observed behaviour and confirms the sign.
- **When only one side leaks, it is fully solvable.** If B wears headphones and A does not, only
  B hears an echo, and `d = +L` removes it completely. The rule:

  > **The participant whose room audio leaks runs *ahead* by the one-way voice latency.**

This matters perceptually rather than marginally. Below roughly 30 ms the precedent effect
fuses the two copies into mild coloration; by 150–300 ms — ordinary voice-path latency — it is a
distinct and very disruptive slapback. Moving from `L` to near zero is the difference between
unusable and unnoticeable.

### We can set it automatically, and nobody else can

We already measure round-trip latency on the voice connection, so `L ≈ RTT/2` is in hand. So
"align our audio" can be a button rather than a fiddle, with a manual nudge for the case where
the estimate is off — and the manual nudge is worth keeping regardless, because the user found
this by hand before anyone built it.

### The three-tier answer to echo, which supersedes "just use headphones"

[D63](#d63--voice-chat-is-webrtc-and-headphones-are-the-supported-configuration) was right that
headphones remove the problem and wrong to stop there, since headphones are not always possible.

| Tier | Mechanism | Cost |
| --- | --- | --- |
| 1 | **Headphones** — no acoustic path at all | none |
| 2 | **Offset alignment** — this decision | a field in the group state |
| 3 | **Content-referenced AEC** — use the decoded film audio as the echo reference | real work |

Tier 3 deserves stating because it is **uniquely available to us**. D63 observed that the
platform's AEC cannot cancel the film because it has no reference for audio played by libmpv,
ExoPlayer or AVPlayer. But *we are the player*. We know exactly what the film's audio is, sample
for sample, because we decoded it. Feeding that to a standalone echo canceller — `speexdsp`
(BSD-licensed, actively maintained) or the extracted `webrtc-audio-processing` (BSD-3-Clause) —
gives a **better reference than the operating system can supply**, because the OS only sees the
mixed output while we see the exact source. The difficulty is delay alignment against unknown
output latency, which adaptive cancellers estimate anyway.

Tiers 1 and 2 are nearly free and should ship together. Tier 3 is a later, separable
improvement.

### Why the data model change has to happen now

`P + offset_i` instead of `P` is a trivial generalisation on day one and an awkward retrofit
later, because it touches the group state record, the drift-correction target, and every
"are we in sync?" comparison. It also pays for itself elsewhere: a participant on a
consistently slower link can be given a standing offset instead of being seeked at constantly.

**Status:** Provisional in its automation only. The mechanism is derived and field-confirmed;
whether `RTT/2` is a good enough estimate of one-way voice latency in practice is measurable and
unmeasured, and asymmetric routes will make it wrong sometimes — hence the manual nudge.

---

## D66 — No clock synchronisation: monotonic local time and bounded round-trips

**Decided** (2026-09-19). Clients never synchronise clocks with the server or with each other.
Each keeps a **monotonic** local time, reports its position against it, and the server bounds
the uncertainty with a round-trip measurement. This replaces
[D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
description of the offset exchange as "NTP-style".

### The question was whether device timers are good enough on their own

They are, for the interval that matters, and the arithmetic is worth writing down. **50 ppm
between two consumer devices' clocks is the realistic figure** — a crystal rated 48,000 Hz runs
at 48,000.5 on one device and 47,999.5 on another. So:

```
50 ppm over 10 minutes  →   30 ms      below Jellyfin's 60 ms "do nothing" threshold
50 ppm over 20 minutes  →   60 ms      at the threshold
50 ppm over 3 hours     →  540 ms      well past it
```

So the intuition is quantitatively right: **local timers need reconciling every ten to twenty
minutes, not every second.** Jellyfin polls at 60 s, which for drift alone is roughly 40× more
often than necessary.

### But clock drift is not the dominant term, which changes the design

A stall, a burst of dropped frames, or a seek landing on the nearest keyframe instead of the
requested position moves a client by 100 ms to 2 s **instantly**. No amount of timer accuracy
helps, and no polling interval catches it promptly without being wasteful the rest of the time.

So reconciliation is **two mechanisms, not one**:

- **Periodic**, on a timer measured in minutes, for slow clock and playback-rate drift.
- **Event-driven**, triggered by the client itself — the `isBuffering()` transition, a completed
  seek, a resume — which is when position actually jumps.

The event-driven half is the one that matters, and it is nearly free because those events are
already in the six-method client contract.

### What we never needed was a shared clock

This is the same trick the project already uses for node liveness
([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle),
[D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval)):
*the core knows how much time passed between its own observations, and does not care what time
any component thinks it is.*

Applied here: the client reports "my position was `P` when my monotonic clock read `M`". The
server timestamps its own send and receive, computes the round trip, and knows the client's
position **now** to within that round trip. Keeping the **minimum-RTT sample** over a short
window removes most of the queuing noise — that one trick is worth borrowing from Jellyfin and
is the only NTP-ish idea left.

No clock offset estimation, no absolute time, no dependence on device wall clocks.

**And using wall clocks would be actively worse.** A system clock can be *stepped* — an NTP
daemon correcting a phone mid-film, a timezone change, a user fixing the clock — producing a
spurious jump of arbitrary size in the middle of a sync calculation. Monotonic sources
(`performance.now()` in browsers and React Native, `CLOCK_MONOTONIC` and `mach_absolute_time`
natively, and Go's `time.Now()` which carries a monotonic reading that `Sub` uses) are immune to
all of it. So the simpler design is also the more correct one, which is not the usual trade.

**Status:** Provisional in its intervals. The 10–20 minute figure is derived from a 50 ppm
assumption, not measured on our hardware; the device lab should measure real playback-position
drift per platform, because playback-rate error will likely dominate crystal error and may be
much worse on cheap devices. Joins the threshold list in
[performance-targets.md](../design/plans/performance-targets.md).

---

## D67 — tvOS never gates a feature

**Decided** (2026-09-19). Where tvOS cannot do something, tvOS does without it. No feature is
cut, delayed or redesigned to keep the Apple TV client at parity.

**Why this needs saying.** [D30](#d30--tvos-is-cheap-from-inside-the-apple-stack-and-never-has-a-web-fallback) established
that tvOS is a small share of an Apple client's code, and
[tvos.md](../design/research/tvos.md) catalogued how it breaks nearly every cross-platform
option — no WebKit, a Focus Engine that needs UIKit views, TVMLKit deprecated. Watch Together
made the pattern concrete again: `react-native-webrtc` has no data channels on tvOS, needs a
locally built framework, and carries an open tvOS 16 compatibility issue; and an Apple TV has no
app-accessible microphone at all.

The temptation at each of those is to design around the weakest platform. Refused. The cost of
parity is paid by every other client and by every future feature, and it compounds.

**The escape hatch already exists**, which is what makes this safe rather than reckless.
[D18](#d18--the-jellyfin-compatible-surface-is-an-adapter-and-it-quarantines-device-profiles) ships a
Jellyfin-compatible adapter, and Swiftfin and Infuse are mature Apple TV clients that speak it.
So an Apple TV user is not stranded; they get a good third-party client with a reduced feature
set, which is exactly the outcome this decision accepts.

**The bail-out, stated so it is not a slow drift.** If the list of tvOS exceptions keeps growing,
**dropping first-party tvOS support entirely** is on the table and is not a failure — it is the
adapter doing its job. The trigger is qualitative and should be reviewed rather than computed:
when a tvOS workaround starts changing a design that would otherwise be simpler, that is the
signal.

**Status:** Provisional in scope: this is about *tvOS*, not about Apple. iOS and macOS are
ordinary first-class targets.

---

## D68 — TURN is not an independent requirement; the media server is already the relay

**Decided** (2026-09-19). Correcting
[D64](#d64--watch-together-makes-the-remote-access-question-concrete-and-supplies-the-premium-tiers-first-honest-member),
which claimed a TURN relay was the premium tier's first unarguable member. It is not, because we
are not solving the general peer-to-peer problem.

### The correction

For Watch Together the two endpoints are two people — but there is a third party both are
**already connected to by construction**. If she cannot reach the media server there is no film
to watch together. So by the time voice matters, a mutually reachable rendezvous **already
exists**, and it can carry the voice.

Three reasons this is not a grudging fallback but the better answer:

- **Bandwidth is nothing.** Opus speech is ~24–64 kbps per stream, against a film already
  crossing the same uplink.
- **Latency is nearly nothing**, which is the counter-intuitive part. A conventional TURN sits
  in a datacentre *between* the peers and adds a genuine detour. Ours sits **at one endpoint** —
  the server is on the home participant's LAN — so `her → server → him` is essentially
  `her → him`.
- **No new service, no new machine, no new failure mode.** It is the connection that is already
  carrying the film.

So the premium tier's justification is **remote access itself** — the rendezvous, the
certificates, the relay that makes the server reachable at all — and voice simply rides along.
That is a cleaner claim, and it *strengthens* the README's promise by removing a second
justification that does not survive inspection.

### What the research actually established about relays

Worth recording, because "about 10% need a relay" is too coarse to design around
([remote-access.md](../design/research/remote-access.md) §3).

**Only two situations make a relay unavoidable:**

1. **Both** endpoints behind hard (symmetric, endpoint-dependent-mapping) NAT.
2. UDP blocked outright at either end — corporate networks, hotels, captive portals — where the
   only remedy is a relay on **TCP/443 with TLS**.

**One** hard NAT is a solved problem, and the numbers are striking. Port prediction exploits the
birthday paradox: with 256 open source ports, 174 probes gives 50%, 1,024 gives 98%, 2,048 gives
99.9% — at 100 packets/second, *"half the time we'll get through in under 2 seconds."* **Two**
hard NATs require a collision in both directions simultaneously: roughly **170,000 probes per
side, about 28 minutes**. That is the cliff.

**Published relay rates vary by 8× and the gap is mostly effort, not luck:** Tailscale under
10%, general WebRTC 20–25%, corporate WebRTC 60–85%. The most transferable reason is that
**WebRTC has a deadline and Tailscale does not.** ICE must connect in a second or two, so it
gives up and relays; Tailscale **starts relayed and upgrades to direct in the background**,
which buys unlimited time to find a path nobody is waiting on.

**So: relay first, upgrade later.** Voice starts on the server relay — instant, always works —
and migrates to a direct path if hole punching succeeds. The user never learns which one they
have. This also means the relay path must be correct and pleasant rather than a degraded mode,
since it is what everyone gets for the first second and what a minority keep.

And the ladder underneath is worth having in the stated order, because each rung is cheap:
**IPv6 first** (no NAT at all, and it crossed **50% of Google's traffic on 2026-03-28**), then
UPnP-IGD / NAT-PMP / PCP for an explicit mapping, then STUN and simultaneous send, then port
prediction for a single hard NAT, then the server relay. Local addresses are tried first
regardless, because **many NATs do not hairpin** and two peers behind the same router otherwise
fail in the most confusing possible way.

IPv6's trajectory is itself an argument for restraint: this problem shrinks every year without
us, so the traversal cleverness should be bounded and the relay path should be good.

**Status:** Provisional in one respect. If watch-together ever grows past a handful of
participants, relaying N streams through a household uplink stops being free and a selective
forwarding unit becomes a real design question. At two to four people it is not.

---

## D69 — Group size is a priority order, not a capability: two, then four or five, then a party

**Decided** (2026-09-19). Build Watch Together for **two participants** first, **four or five**
second, and **party viewing** third and later. Leave the openings; do not build the machinery.

### Two is not just the first case, it is the case where the design is best

Worth noticing rather than treating two as a degenerate N:

- **[D65](#d65--playback-offset-is-per-participant-and-it-is-an-echo-control)'s echo alignment is
  *fully* solvable at two and only approximable above it.** With one person on headphones and
  one on speakers, the offset `d = L` removes the echo completely. With three or more, the
  pairwise-optimal offsets are over-determined and there is no assignment that satisfies every
  pair — it degrades to a reference participant and best-effort, which is what everyone else
  already has.
- **"Does one stall pause everyone?" has an obvious answer at two** and no obvious answer at
  five.
- **[D68](#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay)'s
  server relay is free at two** and merely cheap at five.

So the ordering is not a resource compromise; two is where the feature is *best*, and it is the
case that motivated it.

### Where the real ceiling is, and it is not the voice

Working the numbers, because the intuition points at the wrong thing. Opus speech with RTP,
SRTP, UDP and IP overhead at 50 packets/second is roughly **64 kbps per stream**. Per-source
gain is a stated requirement, so the server **forwards rather than mixes** — mixing would
destroy exactly the control that was asked for — which means `N(N−1)` outbound streams:

| N | Voice out of the home uplink |
| --- | --- |
| 2 | 0.13 Mbps |
| 5 | 1.3 Mbps |
| 10 | 5.8 Mbps |
| 20 | 24 Mbps |

Voice only becomes a problem around twenty. **The film is the constraint long before that.** A
1080p web-dl direct-plays at 5–10 Mbps and a remux at 20–30, against a typical household upload
of 10–40 Mbps on fibre and far less elsewhere — the existing note in
[remote-access.md](../design/research/remote-access.md) already puts a two-hour film at 20 Mbps.
**One** remote viewer on a remux can saturate a home uplink. Four or five remote viewers cannot
be served at all without reducing the bitrate.

### Which splits "party mode" into two unrelated features

This is the useful consequence, and it changes what the third priority even is:

- **LAN party** — ten people in one house. No NAT, no relay, no uplink involved, bandwidth
  effectively free. This is almost entirely a **UI** problem: a roster, a way to join, and
  sensible defaults for who may control what. Genuinely cheap, and reachable soon after 4–5.
- **Remote party** — ten people in ten houses. Bounded by the household's upload, so it is gated
  on **adaptive bitrate per viewer**, which is the bandwidth half of
  [transcoding.md](../design/plans/transcoding.md) that [D17](#d17--direct-play-first-transcoding-additive-and-optional)
  deliberately deferred. It is not a watch-together feature at all; it is a transcoding feature
  wearing watch-together's clothes.

So remote party viewing is not third on this list — **it is downstream of a different plan
entirely**, and saying so prevents it being scheduled as though watch-together work would
deliver it.

### The openings to leave, all cheap now and awkward later

Each of these is a shape rather than a feature, and none of them is work today:

1. **Membership is a list with per-member state**, not a pair. The moment it is a pair, a roster
   is a rewrite.
2. **Control is a per-participant capability, defaulting to everyone.** "Anyone can pause" is the
   requirement at two and at five; at twenty it becomes chaos and wants a host. If the capability
   exists from the start and is simply always granted, adding a host is a policy change rather
   than a redesign.
3. **The client handles a *set* of remote audio streams with per-source gain**, even when the set
   has one member. This costs nothing — per-source gain was already required — and it is the
   difference between adding a third participant and rewriting the audio path.
4. **Transport is a server-chosen policy, not a client assumption.** Mesh, server relay or
   eventually a selective forwarding unit should be a decision the client is told about, not one
   it is built around.
5. **The stall policy is a group setting** with a default that varies by size, rather than a
   hardcoded rule.

### What not to build

No selective forwarding unit. No active-speaker detection or top-K forwarding. No moderation
tooling. No roles beyond the capability flag in (2). No text chat. Each of these is a real
feature with a real cost and none of them earns its place at two participants — which is the
case that prompted the whole thing.

**Status:** Provisional in its boundaries. The 4–5 figure is a judgement about household uplinks
rather than a measurement; the honest number depends on what bitrate the library actually
direct-plays at, which is the same unknown [D17](#d17--direct-play-first-transcoding-additive-and-optional)
and the Phase 0 playback survey are already waiting on.

---

## D70 — Hosted fan-out is the right shape for remote party viewing, and the cheap part is keeping it possible

**Recorded, not scheduled** (2026-09-19). If remote party viewing ever happens, the shape is
**fan-out**: the household uploads **one** stream to a distribution node and N participants pull
from it, making the home uplink cost independent of N. Speculative and much later. What matters
now is that the openings cost almost nothing and one of them is easy to close by accident.

### Why the shape is obviously right

[D69](#d69--group-size-is-a-priority-order-not-a-capability-two-then-four-or-five-then-a-party)
established that remote party viewing is uplink-bound: `N × 10 Mbps` out of a house with 10–40
Mbps of upload. Fan-out makes that `1 × 10 Mbps` regardless of N. There is no cleverer answer,
and it is what every content-distribution system does.

### And it is the cheapest thing on the roadmap to build, because HLS was designed for it

The minimum viable fan-out node is **an HTTP caching reverse proxy**. Segments are immutable,
addressable and cacheable; clients already pull them over HTTP
([D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)); one origin fetch serves
every participant. That is `nginx proxy_cache` in spirit, not a media system.

Better still: **synchronised viewing is the ideal caching workload.** Unsynchronised viewers of
one film sit at different positions and get poor locality; watch-together guarantees that every
participant wants the *same segment at the same moment*, so the hit rate approaches 100%. The
feature that creates the bandwidth problem also creates the conditions that make solving it
trivial.

### The one opening that is easy to close by accident

**Segment URLs must be cacheable**, which means the cache key must be derivable from the segment
path alone. Signing every segment URL per user — an obvious-looking application of the
signed-and-scoped-URL work in [threat-model.md](../design/plans/threat-model.md) — would give
each participant a unique URL for identical bytes and **defeat fan-out entirely**, silently, and
long before anyone tried to build it.

So: **authorise the session, not the segment.** The manifest is per-user and authorised; segment
paths are stable and content-addressed; the authorisation travels in a session token that the
cache key ignores. This is ordinary CDN practice and it costs nothing to get right now.

That constraint is the entire deliverable of this entry. Everything below is context.

### It must be able to be blind, and that is not DRM

A fan-out node duplicates the film to N recipients. If it can *read* the film, we have built
something we do not want to operate and users do not want to trust. It can be blind: the
household encrypts segments once with a key it distributes directly to the participants it
authorised, and the node forwards opaque bytes.

**This is not DRM and the distinction is load-bearing.** DRM is key escrow, licence servers,
device attestation and a compliance regime, and
[the overview](../design/overview.md) rules it out. This is transport confidentiality against
*the relay operator*, with the key held by the person who owns the file and handed to people
they chose. No licence server, no attestation, nothing to certify. The user can always decrypt
their own media; that is the point.

### The honest risk, which is not technical

A 1:1 relay for NAT traversal and a 1:N distribution service are the same bytes and a different
legal posture. Functionally, 1:N distribution of one person's film to ten others is the shape of
a pirate streaming service, however legitimate every individual party is. Mere-conduit and
hosting-provider protections are strongest for neutral 1:1 transmission and weaker for something
that looks like a distribution platform, and rightsholders notice fan-out.

Being blind helps the conduit argument and simultaneously means the only possible response to a
complaint is "we cannot see it", which is a mixed position rather than a clean one.

**This, not egress cost, is the reason to be slow here.** For reference the cost is a rounding
error: a two-hour film at 10 Mbps is ~9 GB, so a party of ten is ~90 GB, which at Hetzner's
roughly €1/TB beyond a large allowance is **about nine cents** — and at hyperscaler egress
prices is $7–8, which is the same provider-choice conclusion
[remote-access.md](../design/research/remote-access.md) §5 already reached.

### So build it as software, and host it only as a convenience

The fan-out node should be **something anyone can run on a €5 VPS**, which is
[D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)'s
self-hosted-assist tier doing its job. That is cheaper for us, keeps the exposure with the
person who owns the content, and keeps the hosted version honestly optional — *"we only charge
for things that genuinely require another machine"* reads very differently when the machine is
one you could have rented yourself.

**Status:** Speculative. Not scheduled, not costed beyond the above, and downstream of both
remote party viewing being wanted and the bandwidth half of
[transcoding.md](../design/plans/transcoding.md) existing. The **cacheable-segment-URL
constraint is not speculative** and should be respected from the first HLS endpoint.

---

## D71 — One relay artefact, three jobs, and the core dials out to it

**Decided** (2026-09-19). Generalising the fan-out node of
[D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible)
from a party-mode speculation into **a third mode of the same binary**, useful at two
participants: `relay`, run on any publicly reachable host.

### It is one artefact doing three jobs

Three problems that looked separate turn out to be one deployment:

| Problem | Previously | Now |
| --- | --- | --- |
| The partner in a hotel cannot reach the server at all | [D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)'s hosted-assist tier | the relay |
| Voice cannot hole-punch | [D68](#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay): the media server relays | the relay, when the media server is itself unreachable |
| A party exceeds the household uplink | [D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible): fan-out | the same relay, in cache mode |

D68's reasoning generalises rather than breaks: **the relay is whatever mutually reachable node
already exists.** Usually that is the media server. When the media server is the thing that
cannot be reached, it is a box in a datacentre instead.

### The property that matters most is not the relaying

**The core dials *out* to the relay.** Outbound connections always work, so:

- no port forwarding
- no dynamic DNS
- no UPnP
- **works behind CGNAT**, which is the case nothing else solves

Port forwarding is the single largest support burden in self-hosted anything. A dial-out relay
removes it, and that makes this **more than a fallback for hotels** — it is plausibly the
recommended setup for anyone who wants remote access and does not want to touch their router.

### Two modes, and the near-term one is trivial

- **Tunnel mode** — a byte pipe. TLS passes *through*; the relay terminates nothing, reads
  nothing, cannot tamper. Bandwidth is `N ×` the stream. This covers the stated case (two
  people, one in hotels) and is the one to build.
- **Cache mode** — the [D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible)
  fan-out. The relay terminates HTTP so it can see segment *URLs*, while segment *payloads* stay
  encrypted end to end — so it caches ciphertext and still cannot read the film. Origin
  bandwidth becomes `1 ×`. Later, and only if party viewing happens.

The layering is the point: tunnel mode is a byte pipe because it can be, and cache mode gives up
exactly one thing — URL visibility — to buy fan-out.

### It is a role, so it inherits the machinery for free

`relay` sits beside `core` and `node` under
[D43](#d43--one-binary-one-process-per-role-and-a-thin-optional-supervisor)'s one-binary,
one-process-per-role rule, which means it arrives with a status record
([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)), observation-based
liveness ([D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval)),
platform supervision and hardened units
([D44](#d44--prefer-the-platforms-supervisor-ship-the-units-and-never-exit-when-you-could-report)),
and the report-don't-exit rule. No new operational surface.

It is also **untrusted by construction** — a rented box someone else administers — which is why
tunnel mode terminates nothing and cache mode sees only URLs. Clients authenticate the *core*,
never the relay.

**Placement guidance:** put the relay **near the home server**, because the core↔relay link is
persistent and carries everything, while the client↔relay leg is whatever the traveller's
network happens to be. A relay adjacent to the origin adds almost no latency; one on the far
side of the world adds all of it.

### The economics are the argument, and the comparison is stark

Plex is the direct prior art and it has gone the other way. Plex's own support page now states
that *"Accessing personal video content remotely does have certain requirements and **is not
available for free**"*, and its Relay is reported to be capped at **1 Mbps free / 2 Mbps paid**,
with enforcement recently tightened. 2 Mbps means heavy transcoding: a degraded experience by
construction.

A €5 VPS with a 20 TB allowance carries roughly **2,200 film-hours a month at 10 Mbps**,
uncapped. That is about **five times the bitrate of Plex's paid relay**, unmetered in practice,
for the price of a coffee.

The gap is not cleverness, it is **household economics versus platform economics**: Plex
subsidises a free tier across millions of users on commercial egress; a VPS you rent yourself
subsidises nobody and pays commodity rates. This is the clearest expression yet of the
README's promise — *"we only charge for things that genuinely require another machine"* — because
here the other machine is one **you** can rent, and we are not in the path at all.

### And the obvious shortcut is not available

Cloudflare Tunnel is the first thing everyone reaches for, and its terms forbid this in as many
words:

> "Cloudflare reserves the right to disable or limit your access to or use of the CDN … if you
> use or are suspected of using the CDN without such Paid Services to serve video or a
> disproportionate percentage of pictures, audio files, or other large files."

So the documentation must say this plainly rather than letting people discover it by being
rate-limited or banned. `frp`, `rathole` and a plain VPS are fine; the free CDN tunnel is not.

**Status:** Provisional. Tunnel mode is a small, well-understood component and should be
scheduled with [remote-access.md](../design/plans/remote-access.md); cache mode stays
speculative per D70. The open question this sharpens rather than answers: if self-hosting a
relay is *this* much better than the commercial offering, the hosted tier's remaining
justification is convenience for people who will not rent a VPS — which is a real market and a
much smaller claim than the one we started with.

---

## D72 — The hosted relay has no free tier, holds no data, and has no accounts

**Decided** (2026-09-19). A product decision, the least technical in this log, and it constrains
the architecture in four specific ways. Position: serve the people who will not rent a VPS,
charge them, and design so that doing it is boring.

### What "the Plex problem" actually is, stated so we avoid the mechanism and not just the mood

Plex gave remote access away free for years on subsidised bandwidth. A free tier whose
**marginal cost scales with usage** is a liability that grows with success, so success forced
the claw-back: remote playback of personal media paywalled in 2025, Relay capped at a reported
1 Mbps free and 2 Mbps paid, enforcement tightened. They now carry both the cost *and* the
reputational damage — the worst of both, and it is the churn this entire project is aimed at.

> **The rule: never offer a free tier whose marginal cost scales with usage.**

### So: no free hosted tier, and that is not a paywall

The distinction matters and it is the whole moral position.

Plex's free tier existed because there was **no self-hostable alternative** — withdrawing it
took something away. Ours is different by construction:
[D71](#d71--one-relay-artefact-three-jobs-and-the-core-dials-out-to-it) makes the relay **a mode
of our own binary**, and a €5 VPS running it carries roughly 2,200 film-hours a month at
10 Mbps uncapped — **about five times the bitrate of Plex's paid relay.**

So the free path is not crippled, it is *better*. We are not withholding a feature; we are
declining to also do the sysadmin for free. That reads very differently, and it keeps the
README's promise intact: the thing that requires another machine is the only thing charged for,
and you can supply the machine yourself.

**And state the allowance; never throttle silently.** Plex's 2 Mbps cap is the specific thing
that felt like a betrayal — a hidden degradation rather than a stated limit. A published
allowance with a message when you approach it is honest; an invisible throttle is not.

### Four architectural consequences of "no hassle"

These are what make operating it boring, and each is a design constraint rather than an
operational preference:

1. **The relay holds no data.** Blind and stateless, per D71. A box is then **cattle**: it dies,
   another launches, nobody notices. There is no backup story, no restore story, and no
   migration.
2. **The relay holds no custody, which is different from holding no visibility.** It cannot be
   asked to restore data it never held. It **can** and **must** still report everything it
   observes — see [D73](#d73--each-component-reports-according-to-its-own-visibility-blind-is-not-opaque),
   which corrects an error in the original wording of this item.
3. **No accounts on the relay.** This is the big one. Clients authenticate the **core**, never
   the relay, so the relay needs no user directory — which removes password resets, account
   recovery, session management, and the data-protection surface that comes with holding
   identities. Payment identity (a billing customer) stays **separate** from access (a keypair
   the core already has): you buy a token, the token authorises a relay slot, there is no login.
4. **Payment is the abuse control.** [remote-access.md](../design/research/remote-access.md) §5
   already concluded *"the binding constraint is abuse, not cost"* for a relay that forwards
   arbitrary bytes. A card is a real barrier and a traceable one, and removing the free tier
   removes the anonymous-abuse path entirely. This is an independent reason for no free tier,
   and it is the stronger one.

### The scale sketch, so "tidy" is not hand-waving

Order of magnitude only. At 10 Mbps a stream is ~4.5 GB/hour, so a 20 TB allowance is ~4,400
stream-hours a month. If a household streams 60 remote hours a month, one €5 box covers **on
the order of seventy households** on bytes. The tighter limit is concurrency — 20 simultaneous
streams is 200 Mbps against a typical 1 Gbps port — but concurrency runs far below subscriber
count.

The conclusion that matters: **infrastructure is not the constraint, support and trust are.**
Which is precisely why items 1–4 above are the design, and why the business can be small
deliberately. A waiting list is acceptable. Regional availability is acceptable. Choosing not to
grow is a legitimate decision and should not drift.

### The commitment that makes paying us safe

Anyone paying a hosted service should ask what happens when we get greedy or get acquired — the
Plex trajectory. The answer here is structural rather than a promise:

> **The hosted relay runs the same open-source binary you can run yourself, and it holds no
> data. Leaving takes an afternoon and migrates nothing.**

That is verifiable, it cannot be quietly revoked, and it is a far stronger commitment than any
wording in a terms-of-service. It also disciplines us: a hosted tier that must stay
trivially-exitable can only compete on being *good*, which is the stated goal —
**a tidy income for a real service, not a captive audience.**

**Status:** Provisional and revisitable, being a product decision rather than a technical one.
What is *not* provisional is the architecture it implies — blind, stateless, accountless — which
should hold even if the hosted tier never exists, because every one of those properties also
makes the self-hosted relay better.

---

## D73 — Each component reports according to its own visibility; blind is not opaque

**Decided** (2026-09-19). Correcting [D72](#d72--the-hosted-relay-has-no-free-tier-holds-no-data-and-has-no-accounts),
which claimed *"being blind removes most support."* That conflated two different things and got
the conclusion backwards.

### The error

Blindness is about **content confidentiality**. It says nothing about **observability**. A relay
that cannot read the film can still see, and report: bytes forwarded, connection state and its
transitions, round-trip latency, when the tunnel dropped and how often, which peers connected
when, errors and their codes.

So the correct statement is not "we can see little, therefore support is hard." It is: **each
component reports according to its own visibility**, and the relay's visibility is substantial.
What it lacks is *custody*, not sight.

### And the part it cannot see is the part that would not have helped

This is what makes the trade free rather than merely acceptable. Nobody diagnoses stuttering
playback by looking at video frames. Every question support actually answers — *why did it
drop, why is it buffering, which leg is slow, is it the uplink or the hotel wifi* — is answered
by exactly the operational data a blind relay can see.

**Content confidentiality costs zero diagnostic power.** Blindness is therefore not a trade-off
to justify; it is free.

### Three vantage points, all landing at the core

The relay is already a role with a status record
([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle),
[D71](#d71--one-relay-artefact-three-jobs-and-the-core-dials-out-to-it)), so the mechanism
exists. What this decision adds is direction and scope:

| Vantage | Sees | Reports to |
| --- | --- | --- |
| **Client** | decode errors, buffering, chosen rendition, playback position | the core — **piped through** the relay, which is already the path |
| **Relay** | throughput, connection state, RTT per leg, drops, error codes | the core |
| **Core** | what it sent and when, plus the library and the history | itself |

Three independent views of one session, joined at the core. That is **better** diagnostics than
a content-reading relay would give, because the extra thing it would see is the useless thing.

**And the direction is the point: the relay reports to the household's core, even when we
operate the box.** The data stays with the library owner. This is
[principles §9](../principles/README.md) applied to a machine we happen to run.

### The one thing to build now: a session id

Three vantage points are only joinable if they share a key. **The core issues a session
identifier; the client and the relay tag their observations with it.** Cheap now, awkward later,
and without it the three views are three unrelated logs.

In tunnel mode the relay is a pure byte pipe and cannot annotate the stream, so it reports
**out of band** through its own status record. That is already how every other role works.

### The line that keeps "no telemetry" true while operating a service

Running a hosted relay means monitoring it, and the README promises no telemetry. The two are
compatible, and the rule is checkable:

- **Session-scoped and identifying observations go to the core**, never to us. Who connected,
  from where, watching what, how it performed.
- **Machine health goes to the operator** — CPU, memory, disk, uptime, aggregate throughput,
  aggregate error rates. About the *box*, not about *anyone*.

Monitoring our own hardware is not telemetry about users. The boundary is that nothing
session-scoped leaves the relay in our direction; it leaves in the core's direction. If a
metric cannot be aggregated to the point of being about the machine, it is not ours.

**Status:** Provisional in detail, settled in principle. The specific field list for the relay's
status record belongs with [remote-access.md](../design/plans/remote-access.md); the session id
belongs in the first version of the relay protocol.

---

## D74 — Bug reports pseudonymise rather than redact, and fan in across components

**Decided** (2026-09-19). Ship a diagnostic-report generator whose output pastes into a GitHub
issue. The structure is taken from yoloai, which has solved this; three things differ enough to
be the actual design work.

### Taken from yoloai, because it works

- **Two mechanisms.** A **flight recorder** (`--bugreport <type>` on any command: opens the file
  before the command runs, writes static sections immediately, always finalises — success,
  error or panic) and a **forensic** subcommand for a problem that already happened.
- **Two types, and the type is a required argument with no default.** `safe` — redacted,
  shareable, carries a "review before sharing" notice. `unsafe` — complete, carries a prominent
  **do not share publicly** banner.
- **Sanitisation is best-effort and says so**, including the honest instruction not to file
  credential-related bugs as `safe` reports.
- **One outer `<details>` wrapper** so a pasted report renders as a single collapsed line whose
  `<summary>` carries version, timestamp and type — triage without expanding.
- **64,000-byte threshold** against GitHub's 65,535-byte issue body limit, leaving room for the
  user's own text. Mode 0600, atomic rename from `.tmp`.
- **A deliberate-omissions table with reasons**, not a silent filter.

### What differs, and it is the centre of the design

**Our sensitive data is the library itself, and unlike a credential it is what most reports are
*about*.** A person's film and television collection is intimate — health, religion, sexuality,
politics — and it cannot be rotated after disclosure. Meanwhile "this file will not play" is the
modal bug, so the sensitive thing is also the subject.

Redaction therefore fails in both directions: `[REDACTED]` destroys the ability to correlate,
and leaving titles in leaks the library into a public issue.

> **So pseudonymise rather than redact.** Per report, a random salt; each title, path component
> and username becomes a short stable tag — `item:a7f3`, `user:2b8d`. Within one report
> correlation works: the maintainer sees `item:a7f3` fail in three places. Across reports the
> tags differ, so nobody can be tracked between issues. **The mapping table is written only
> into the local `unsafe` copy**, so the user can look up `a7f3` and say what it was, privately,
> if asked.

And the complement, which is where nearly all the diagnostic value lives:

> **Keep the shape, drop the identity.** Container, codecs, profiles, levels, bit depth, channel
> layouts, track counts, dispositions, language tags, attachment types, duration, size, file
> extension, directory depth. `hevc / Main10 / L5.1 / 3 audio / 12 subtitle / 1 attachment`
> diagnoses a playback bug completely and says nothing about which film it is.

The residual case — a *filename-parsing* bug, where the name is the evidence — is served by the
user sharing **one** filename deliberately, which is minimal disclosure rather than a library
dump.

### Second difference: the report has to fan in

We are not one process. A session touches the core, one or more nodes, possibly a relay, and a
client, and [D73](#d73--each-component-reports-according-to-its-own-visibility-blind-is-not-opaque)
just established that each reports according to its own visibility and all of it lands at the
core. So the report is a **fan-in keyed by the session id** D73 requires.

Two rules follow from decisions already made:

- **A component that does not answer is listed as absent with a reason**, never silently
  omitted — the same rule as [D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope)
  and [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret):
  unreadable is not empty. *"node `nas`: no response, last seen 4 h ago"* is diagnostic; a
  missing section is a trap.
- **The summary is what gets pasted; the bundle is what gets attached.** Four components will
  not fit in 64,000 bytes. GitHub accepts `.md`, `.txt`, `.log`, `.json` and `.zip` attachments
  up to 25 MB, so attaching the bundle is better than yoloai's gist fallback — one artefact, in
  the issue, with the same visibility rules.

### Third difference: the version inventory is load-bearing, not boilerplate

We ship our own FFmpeg, libass and FreeType builds, and several decisions already depend on
knowing exactly which:

- FreeType version — **CVE-2025-27363 covers ≤ 2.13.0**, and
  [D56](#d56--embedded-fonts-are-off-by-default-the-client-must-ask-for-them) turns on whether
  the client is in that range.
- Which `ASS_Feature`s the build actually supports —
  [D50](#d50--build-libass-correctly-and-turn-off-vsfilter-bug-compatibility-for-non-authored-subtitles)
  already has us probing `ass_track_set_feature` for a −1 return, so the answer exists.
- Which scripts the client has fonts for ([D53](#d53--font-coverage-is-not-assumable-the-server-serves-fonts-on-demand)).
- FFmpeg version *and configure flags*, because the licence ladder and the available encoders
  both hang off them.

[D61](#d61--containment-now-detection-later-and-plugin-shaped-four-affordances-to-keep-open)
wanted a staleness check for exactly these; the bug report is where that inventory surfaces, and
it should be built once and used by both.

### And legibility is a privacy feature here

The report must be **plain markdown the user can read before posting** — no base64 blobs, no
binary, nothing opaque. "Review before sharing" is meaningless advice if the thing cannot be
reviewed. This is the project's existing legibility theme
([principles §3](../principles/README.md)) doing double duty.

**Status:** Provisional in detail. The pseudonymisation scheme wants a real look before it
ships — in particular whether a 4-character tag collides annoyingly in a 50,000-item library,
and whether preserving directory *depth* leaks more structure than it is worth. Plan:
[bug-reports.md](../design/plans/bug-reports.md).

---

## D75 — One predicate language, three bindings; a typed field registry is the schema deliverable

**Decided** (2026-09-19). Research: [rules-engine.md](../design/research/rules-engine.md).
Answers the question [plans/rules-engine.md](../design/plans/rules-engine.md) posed — *one
engine or three?* — and produces the two artefacts that are actually schema.

### One predicate language, three bindings

Smart playlists, smart collections and retention policy are **the same stored query evaluated at
different times with different effects**. So: one predicate language, three bindings, and the
binding is what differs.

| Binding | Effect | Evaluated |
| --- | --- | --- |
| Smart playlist | ordered membership | on read, or on a change-journal trigger |
| Smart collection | display grouping | same |
| Retention | **marks** items | on a schedule |

**Retention marks; it never deletes.** Maintainerr and Janitorr delete, and that is precisely
what [D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index) forbids:
media is read-only and the files are the source of truth. A retention rule produces a *state*
the user can see, sort and act on — the same shape as
[D61](#d61--containment-now-detection-later-and-plugin-shaped-four-affordances-to-keep-open)'s
"quarantine is a library state, never a file operation". Deletion stays a separate, explicit,
confirmed action on a reviewed list. This is a safety property and a differentiator, and it
costs nothing.

### The schema deliverable: a closed, typed field registry

Not arbitrary column names. A map from field name to semantics — type, nullability, whether it
is multi-valued — carrying four properties, the last two of which are easy to miss:

1. **Injection defence.** A field resolves in the registry or the rule is invalid. No user
   string ever becomes a SQL identifier.
2. **Typed operators.** The legal operator set is a function of the field's type, so an invalid
   rule is rejected at parse time. Kometa's per-type modifier table is the right shape;
   Navidrome's `Numeric`/`Boolean`/`Nullable` flags are the minimum.
3. **An `alias` field is the schema-evolution mechanism.** Rename a column, register the old
   name as an alias, and every saved rule keeps working. There is no other plan that works, and
   it is nearly free if present from the start.
4. **The registry is extensible at runtime.** Navidrome's `AddTagNames()` pulls user-defined
   tags into the vocabulary. Ours must also admit **provider-materialised** fields, because
   Kometa's users want IMDb keywords and TMDB vote counts and a vocabulary of local columns only
   is *why* Kometa exists outside the server.

**One registry, not two.** Navidrome keeps semantics in `model/criteria/fields.go` and the
field-to-SQL mapping in `persistence/criteria_sql.go` — two maps with the same keys, which is
the exact shape of the drift that cost them a corrective migration in their FTS work. One table
carries both, or a test asserts the key sets are identical.

### Multi-valued fields are a distinct class

"Has genre X" and "has more than three genres" are different operators, and Kometa ships
`.count_gt`/`.count_gte`/`.count_lt`/`.count_lte` for exactly this. Count operators have to
exist in the language from the start, because they decide how tags are stored — which is a
`library-model.md` question this forces.

### Nullability is declared per field, never inferred

Two traps taken directly from Navidrome's SQL layer, both of which encode a semantic decision
rather than a mechanism:

- **Coalesce where a LEFT JOIN produces NULL.** Play count and rating live in an annotation
  table with no row for a never-played item, so `playcount < 5` without coalescing **excludes
  everything never played** — the opposite of what the user means.
- **Emptiness is per-field.** Their `lyrics` field treats the JSON `[]` as missing. The general
  rule from their own comment: numeric and boolean missing means NULL; string missing means
  NULL *or empty string*.

### The compiler has a hard ceiling and must own it

**Measured** on SQLite 3.46.1: an OR chain fails at **1000 terms** (*"Expression tree is too
large"*), and a join fails at **65 tables** (*"at most 64 tables in a join"*). Both are
reachable by a Kometa-sized rule, and they push opposite ways — a JOIN per tag condition dies at
64, an OR chain dies at ~1000.

That explains what looks like style in Navidrome and is not: **`EXISTS` subqueries rather than
joins**, with ORed conditions batched inside them (their `jsonCondBatchSize = 350`, noting *"the
EXISTS wrapper consumes ~4 levels… empirically, 496 is the maximum"*) and De Morgan merging for
the negated case.

So: **use `EXISTS`, batch, and enforce an explicit complexity bound with a comprehensible
error** — never emit SQL the database will refuse.

### Rule references: allowed, acyclic, checked at save

Rules referencing other rules is a real want (Navidrome has `inPlaylist`) and a real hazard —
**Apple shipped it and broke it**: since Big Sur a smart playlist depending on other smart
playlists no longer updates when they change.

Allow it, but detect cycles **at save time** and reject with a named path, and bound the depth.
Navidrome's `ChildPlaylistIds()` exists to extract exactly these edges; the graph is tiny and
checking it is cheap. Failing at save is the only acceptable place — failing at evaluation means
a rule that silently stops working, which is what Apple has.

### A rule that references a missing field fails loudly and is preserved

The worst outcome of a schema change is a user's smart collection quietly disappearing. So a
rule whose field no longer resolves is **kept, marked invalid, and surfaced with the offending
field named** — never dropped, never silently emptied. That is
[principles §7](../principles/README.md), migrations are a safety feature, applied to user data
that happens to be a query.

### Live updating is a property of the rule

Apple exposes it as a toggle and it is the right model: a rule is live or frozen. Live rules
re-evaluate off the change journal ([D8](#d8--a-change-journal-and-delta-sync-cursor-exist-in-the-first-schema))
rather than on a timer, because the journal already says exactly what changed. Navidrome's
`RefreshDelay` is the debounce.

### Regex is safe here, and that is a free win

Users want it — Kometa offers `.regex` on every string-ish type. SQLite has no built-in
`REGEXP`, but both pure-Go drivers can register one; measured cost is ~5× `LIKE` on 50k rows
(17.6 ms versus 3.3 ms), which is nothing at household scale.

The usual objection is denial of service, and **it does not apply to us**. Go's `regexp` is RE2:
the classic `(a+)+$` catastrophic-backtracking pattern completes in **16 µs**, and
backreferences and lookahead are rejected at compile time. Linear time, guaranteed. So we can
accept user-supplied regex **without a ReDoS guard**, which almost no other runtime could — an
unplanned payoff from [D9](#d9--server-language-go-one-language-on-the-server).

**Status:** Provisional. The complexity bound needs calibrating against a real corpus of
Kometa-scale rules, and whether tags live in a join table or JSON is a
[library-model.md](../design/plans/library-model.md) question this decision forces but does not
settle — Navidrome's JSON choice is what created their batching problem.

---

## D76 — A share is a grantee plus a predicate, and it fails closed

**Decided** (2026-09-19). Research: [sharing-model.md](../design/research/sharing-model.md).
Granular sharing — the thing
[the overview](../design/overview.md) lists as open problem #2 and that nobody in the open field
has solved — turns out to be a **binding of the rules engine**, not a new subsystem.

### The unification

Plex is the only working answer in the field, and its mechanism is a **filter**: restrict a
shared user by content-rating ceiling and by **label** allow/exclude lists, both applying
together. Not a list of items, not a directory — a predicate that is re-evaluated. (And it is
Plex Pass only, which is its own comment on how much people want it.)

[D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable)
just specified a typed predicate language over a closed field registry. So **a share is a
grantee plus a predicate**, and it is a fourth binding alongside smart playlists, smart
collections and retention. "A library", "a collection", "everything except label `private`" and
"this one film" are all predicates.

This answers two of the plan's open questions without any new mechanism:

- **A shared item that moves stays shared** — the predicate re-evaluates and item identity
  already survives a move ([D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index)).
- **A deleted item leaves the share**, because it leaves the result set. An item *list* would
  have needed cleanup; a predicate does not.

### The asymmetry that must be explicit: shares fail closed

D75 says a rule whose field no longer resolves is kept, marked invalid and surfaced. That is
right for a smart playlist, where "invalid" means the user sees an error instead of a list.

**For a share, invalid must mean grants nothing.** A migration that breaks a predicate must
never widen access. The same predicate object is doing both jobs and **the safe default is
opposite in each**, so the binding declares it rather than inheriting it.

### Enforce at every access path, not only at listing

The classic bypass is filtering the library view but not the fetch-by-id endpoint, so a grantee
sees a narrowed list and can still stream anything whose id they remember from before the share
tightened.

This needs **both halves**, and it composes with
[principles §10](../principles/README.md) rather than contradicting it: **omit** from the
manifest so a correct client never asks, **and deny** at the endpoint, because a shared user is
a different trust domain from the owner. §10's reading of the denial gets *more* valuable here
— a grantee's client asking for something outside its share is either a bug or a probe, and
both are worth surfacing.

### What we will not build, and why we can afford not to

Netflix's household enforcement uses IP address, device IDs and Wi-Fi network — *"the main TV
defines the official household network, and phones, tablets and laptops must occasionally
connect to the same Wi-Fi to remain verified"* — with a four-digit code emailed to the account
owner when you stream from elsewhere, and **$7.99/month per extra member slot** since May 2023.
It was a commercial success: **19 million new subscribers** in the following months.

**We never verify households, never count devices, never check IP addresses for entitlement.**
Not as a gesture of leniency: Netflix must do this because it licenses content per household,
and **we license nothing, because the user owns the files.** The constraint that forces the
entire apparatus does not exist here, so we can give away for free precisely the thing Netflix
charges to permit. "Your phone must occasionally connect to the living-room Wi-Fi" is a
requirement imposed on someone's life, and it is a large part of what our users are leaving.

### Sharing is intent, not enforcement, and we should say so

Anyone who can play a file can re-transmit it; no access-control design changes that, and the
alternative is DRM, which is ruled out. So **no transitive re-sharing by default** — a grantee
cannot grant onward — as a guard rail against accident and drift rather than a control against
a determined person. Plex takes the same position. Stating the limit plainly is better than
implying a guarantee we cannot keep.

### Playback state lives with the media, keyed by the grantee

The owner's server records that this person watched to 00:42:11, because it is the only party
that can know. **Do not merge state across servers**: a client aggregating several shows
several sources, and reconciling "watched" across libraries holding different copies of the
same film is a correctness problem with no payoff.

**Status:** Provisional. Three things are open and one is consequential: whether a grantee may
build their own rules over the filtered view (natural, and cheap only if designed in, since
every predicate then evaluates composed with the share predicate); what the default share is
(*"everything except items labelled private"* is probably what people mean); and whether a
grantee sees that they are looking at a filtered library, which is honest and also tells them
what to ask for.

---

## D77 — Back up with `VACUUM INTO`, back up only the irreplaceable, and test the restore

**Decided** (2026-09-19). Research: [backup-restore.md](../design/research/backup-restore.md).

### The obvious method is catastrophically wrong, and silently

A WAL database is three files and recent commits live in the `-wal`. **Measured** immediately
after committing 20,000 rows: `live.db` was **4,096 bytes** and `live.db-wal` was **333,752**.
Copying the `.db` alone produced a file that opens cleanly and reports **`no such table`**.

So `cp library.db backups/` yields a valid SQLite file containing nothing, and it fails only at
restore — the worst possible moment. Any documentation suggesting a file copy is actively
dangerous and we must not ship one.

### `VACUUM INTO`, because it is plain SQL

Measured alternatives that all work: copying all three files, the backup API,
`wal_checkpoint(TRUNCATE)` then copy, and `VACUUM INTO`. **`VACUUM INTO` wins on properties
rather than correctness** — it is ordinary SQL so it needs no driver-specific code, it emits
**one** file with no sidecars, and it compacts on the way.

And it is free at our scale: measured at **29 ms for a 2.8 MB database with a writer committing
concurrently**, five rows committed during it, no errors.

Navidrome uses the backup API instead and leaves the caution in the code — *"-1 means that
sqlite will hold a read lock until the operation finishes. This will lock out other writes."*
Both pure-Go drivers do expose the API, so [D31](#d31--sqlite-with-a-pure-go-driver-supersedes-d5)
does not force this; `VACUUM INTO` is simply less machinery.

### Two footguns to copy from Navidrome verbatim

Their comments name both, and the first is the most destructive bug available in this area:

> *"The driver opens with `SQLITE_OPEN_CREATE`, so without this check a typo in the path would
> create an empty database and 'restore' it over the live one."*

**`os.Stat` the source before restoring.** One line, and without it a mistyped path silently
replaces the live database with an empty one.

> *"The driver splits the DSN at '?', so such a path would open a different file."*

Reject paths containing `?`.

### The backup is small, which changes what is affordable

[D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index) makes the
database a rebuildable index, so most of it is not backup material. The library index, probe
results, thumbnails, trickplay, extracted subtitles, subset fonts, FTS indexes and sort keys are
all regenerable by a rescan. What is irreplaceable is **watch state, accounts and shares, rules,
track-selection preferences ([D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)),
and the change journal and sync cursors ([D8](#d8--a-change-journal-and-delta-sync-cursor-exist-in-the-first-schema))**.

That is kilobytes to a few megabytes against gigabytes of cache. So **backup is automatic,
frequent and on by default**, because it costs nothing — a product decision a project with a
monolithic database cannot make.

**And the media tree is already a partial second copy**, because D7 writes metadata corrections
back to sidecars. Restore-from-nothing — rescan, read sidecars — recovers the corrections. That
is a durability property worth testing as one rather than asserting.

**Watch state is the deliberate exception and the thing that most needs the backup.** It must
**not** be mirrored to sidecars: it is per-user, and writing it beside the media would expose
one household member's viewing history to anyone with filesystem access. The sidecar mechanism
structurally cannot carry it, which is exactly why the database copy matters.

### Restore is the half that does not exist until it is tested

A backup that has never been restored is not a backup. So: **a test that restores into an empty
instance and asserts equivalence of the irreplaceable set** — not a checksum, which proves only
that a copy happened.

Two properties fall out of [principles §7](../principles/README.md):

- **A backup carries the schema version it was taken at, and restore refuses a version gap**
  rather than auto-converting. Detect-and-refuse, as with migrations.
- **Restore is destructive, so it says what it is about to overwrite** before doing it.

Jellyfin already implements the corresponding idea from the other side — a
`JellyfinMigrationBackupAttribute` by which **a migration declares that it requires a backup**.
That is §7 implemented rather than promised, and worth copying.

**Status:** Provisional in its policy numbers. How many backups to keep and for how long is
unmeasured; whether the disposable `cache/` tree ever deserves an optional archive is a real
question, because "correct but regenerate 100 GB of trickplay" is a long wait after a restore.
And a node losing its local state should cost a rescan and nothing else
([D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret)) —
worth asserting as a test rather than assuming.

---

## D78 — Server-rendered subtitle overlays are a third rendition, and they dominate burn-in

> **Measured 2026-09-20** ([D114](#d114--a-pass-costs-what-it-costs-because-of-one-thing-whether-it-decodes-video)):
> the live-render cost this decision left open is **1.16 s per film-equivalent** at event
> granularity — 1,200 events on a 1920×1080 canvas, 515× realtime. Implemented the naive way,
> by compositing over the decoded film, it is **18.15 s**. So the rendition is free and the
> wrong implementation is fifteen times worse; the plan must say which it means.

**Decided** (2026-09-19). Add a **server-rendered overlay** subtitle rendition — libass runs in
the node, output is timed, cropped, palettised bitmaps — alongside text and burn-in. Text stays
the default. Research: [subtitle-rendering.md](../design/research/subtitle-rendering.md).

### Bandwidth is not the objection — measured

The instinctive worry is size, and it is misplaced once the bitmap is **cropped to the text
bounding box** and **palettised**:

```
PNG RGBA, full 1080p frame        30,258 B/event
PNG RGBA, cropped                 24,528 B/event
PNG 32-colour palette, cropped     8,703 B/event   mean error 0.58/255 — visually lossless
```

At 8.7 KB × 1,500 events that is **12.4 MB per film — 0.14% of a 9 GB film.** On a LAN it is
free; remotely it is a rounding error. (This is exactly PGS's shape: cropped, palettised
bitmaps with position metadata. Blu-ray solved it decades ago.)

### What it buys, and the first one is the reason it was raised

**The client never parses an attacker-supplied font.** The hostile parse happens in the node's
Landlock+seccomp jail ([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)) —
the place designed for hostile parses — instead of on a television running FreeType 2.10. That
is strictly stronger than [D56](#d56--embedded-fonts-are-off-by-default-the-client-must-ask-for-them),
which had to put the decision in the client's hands because the risk varies by client.

**It fills the gap [D57](#d57--the-embedded-font-opt-in-is-per-user-sticky-per-series-and-the-client-may-still-refuse)
left open.** D57's honest cost was that a user with an old Fire TV who wants correct fansub
typesetting must either accept the risk or lose the typesetting. This is a third answer:
*correct typesetting, and your device never touches the font.*

**It disposes of font coverage for subtitles.** [D53](#d53--font-coverage-is-not-assumable-the-server-serves-fonts-on-demand)
and [D54](#d54--the-plan-b-font-is-unicode-last-resort-he-bundled-noto-is-served-unifont-is-the-middle-option)
exist because a client may have no font for a script. If the server renders, the client needs no
CJK font for subtitles at all — only for UI text.

**A new client platform gets subtitles almost free.** "Draw this image here for this time range"
is a few lines anywhere; building libass for a platform is a project. Same economy as
[D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
six-method rule, in a different subsystem.

**And it dominates burn-in.** [D23](#d23--axis-switching-is-mid-session-by-default-burn-in-is-the-one-thing-that-breaks-it)
names burn-in as the one thing that breaks the no-restart invariant, because it is a *video*
rendition. An overlay is a separate, switchable track with no re-encode, and any client that can
play video can composite a bitmap. **So D23's exception narrows to animated ASS alone** —
karaoke and moving signs, which event-granularity bitmaps cannot express.

### What it costs, and why it is not the default

**Scaling and user styling are lost.** A bitmap has a fixed size, colour and position. Subtitle
size is an accessibility setting, not a preference, and a bitmap cannot honour it without
re-rendering. We are the server and we know the viewport and the user's settings, so rendering
per request or precomputing two or three sizes both work — but it is now *state*, where a text
track was universal. That is the whole reason text stays the default.

**Precomputation and per-user sizing pull against each other.**
[D11](#d11--subtitles-are-a-subsystem-not-a-feature) already extracts at scan time so
precomputing fits the pipeline, but per-user sizing forces a live path. libass is fast enough
that live rendering is plausible; nobody has measured it.

### The standards position, which settles the delivery question

**IMSC 1.1 defines an Image Profile** — timed PNG — so this is standardised rather than
invented. But **Apple's HLS supports only the Text Profile** (`stpp.TTML.im1t`, WWDC 2017, and
IMSC must be in fragmented MP4), so image subtitles cannot ride the standard HLS subtitle track
to AVPlayer.

So: **our own track type, composited by our own client.** No hardship — our clients already
composite rather than delegating to the platform (JASSUB on web, libass natively), and drawing
a PNG is strictly simpler than what they do now.

### When it is chosen

Automatically when embedded fonts are wanted but the client declined them (D57); when the client
lacks a font for the script and cannot or will not fetch one (D53); when the client has no
libass; or when the user prefers it.

**Status:** Provisional. The live-render cost is unmeasured and decides whether per-user sizing
is affordable or whether we precompute a small set of sizes. And whether 32-colour palettisation
survives contact with heavily-styled ASS — gradients, shadows, semi-transparent boxes — is
untested; the measurement above used ordinary dialogue with an outline.

---

## D79 — Four levels, not three: Title → Edition → Version → File, with Collections orthogonal

**Decided** (2026-09-19). Review:
[library-model-review.md](../design/research/library-model-review.md). `library-model.md` was
written 2026-09-17, before roughly thirty decisions that touch the schema. It holds up well in
most respects and is **one level short** in the one place that matters.

### The missing level

**EIDR** — the industry's identifier registry, used across the studios — models audiovisual
works as **Abstraction → Edit → Manifestation**, plus **Collection** as an orthogonal
container. That is FRBR applied to film, and it is what the industry actually registers
against. Our `Title → Edition → MediaFile` collapses Manifestation and Item into one.

The failure is concrete. *Glass Runner: The Final Cut* held as **1080p in two parts** and **a
single 4K file** is one Title, one Edition, **two manifestations**, three files. With
`Edition → MediaFile` there is nowhere to say that the two parts are one viewing and the 4K is
another: the multi-version axis and the multi-part axis hang off the same parent and cannot be
distinguished. That is **exactly the combination the plan already names** as a permanent source
of bugs in Jellyfin and Plex.

And Jellyfin shows what the absence costs. `Video.cs` carries **four overlapping mechanisms** —
`PrimaryVersionId`, `LinkedAlternateVersions`, `LocalAlternateVersions` and `AdditionalParts` —
two of them bare arrays of **path strings**, so the parts of a multi-part film are not rows at
all. The cause is structural: a `Video` is simultaneously the Edit, the Manifestation and the
Item, so every distinction becomes a side-array. We would be better than that and still short
of not repeating it.

So: **Title → Edition → Version → File.** Version is a technical representation (an encoding, a
resolution); File is the bytes, and a Version may have several.

### Collections are orthogonal, and unify five things

EIDR keeps Series, Season and Compilation **outside** the main chain as Collections rather than
as abstractions with parent pointers. Copying that unifies series, season, box set, hand-made
user collection **and smart collection** — where the last is a Collection whose membership is a
predicate rather than a list ([D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable),
[D76](#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed)).

> **A Collection is either an explicit member list or a predicate.** Everything else about it is
> the same.

### Watch state attaches at the Edition, and names attach at two levels

Both need declaring, and neither answer is obvious.

**Watch state: Edition.** Not File or Version — upgrading 1080p to 4K must not lose your place,
and it would. Not Title — the theatrical and director's cuts have different runtimes, so a
resume position across them is meaningless. Edition is the level at which *the same content* is
true. (Whether a watched *flag* rolls up to the Title is a UI question, deliberately not a
schema one; a resume position cannot roll up regardless.)

**Names: Title and Edition both.** "Glass Runner" and "The Final Cut" are both names and both
need [D45](#d45--metadata-is-multi-language-in-the-schema-chosen-per-request)'s translation
treatment. So the displayed title is a **composition of two translated strings**, which
[D47](#d47--sort-order-is-a-materialised-cldr-sort-key-per-item-per-locale)'s sort key and
[D48](#d48--two-fts5-tokenisers-routed-by-script-and-length-and-exactly-one-normaliser)'s FTS
row must both account for — both currently assume one name per item.

### Three cross-cuts the review caught

**1. The core must persist the node's raw observation, not only its interpretation.**
[D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret)'s
decisive argument for parsing in the core was that a parser improvement must be replayable over
the database **with every node switched off**. That is only true if the input is in the
database. So the File level stores the verbatim filename and the raw probe output, not merely
the parse and the normalised tracks. "Field-level provenance" is adjacent but not the same
claim: provenance says where a value came from; this says keep the input.

**2. The field registry needs a level, and cross-level predicates need a quantifier.** With four
levels, `genre = horror AND resolution > 1080` mixes a Title-level field with a Version-level
one, so the registry must record each field's level and the compiler must know the join. And —
the part that would be found late — **the cross-level predicate is ambiguous**: does
`resolution > 1080` on a Title mean *any* version exceeds it or *all* of them? Both are wanted
("films I have in 4K" versus "films I have *only* in 4K"), so the language needs an **explicit
quantifier**, the same shape as D75's tag count operators. Defaulting silently to `any` would
generate bugs.

**3. Mark the backup-critical tables in the schema.**
[D77](#d77--back-up-with-vacuum-into-back-up-only-the-irreplaceable-and-test-the-restore)
divides the database into regenerable and irreplaceable. Annotating that **in the schema**
makes the backup a query instead of a hand-maintained list that goes stale the first time
someone adds a table.

### What holds up

Recorded because most of the above is correction: UUID primary keys with
`{provider, provider_id}` in a side table; the change journal and sync cursor from day one;
`Library`/`Item`/`MediaFile` naming rather than `Movie`/`Episode`; membership as real rows
rather than serialised lists; and every late addition from the internationalisation round. All
correct, all unaffected by the level change.

**Status:** Provisional in one respect — whether **Version** and **File** are genuinely
distinct rows in the first schema or whether File carries a nullable `version_id` that is
almost always 1:1. The four-level *model* is not provisional; how thinly it is implemented on
day one is negotiable, as long as the multi-part-plus-multi-version case has somewhere to go.

---

## D80 — Licence by artefact: GPLv3 for the server, MPL-2.0 for the clients and the SDK

**Decided** (2026-09-19). Audit:
[audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §2. This supersedes the single
project-wide licence in [tech-licensing.md](../design/research/tech-licensing.md), which
reasoned correctly about the server and was never tested against the clients.

**A whole-project GPLv3 means no Apple client at all.** VLC was pulled from the iOS App Store in
January 2011 over GPL compliance; VideoLAN relicensed libVLC to LGPLv2.1 later that year,
explicitly citing "the widely publicized incompatibility of copyleft licensing with various App
Store agreements"; and the VLC app returned to the App Store in **July 2013 under MPL-2.0**.

The field has already converged, and the pattern is visible in one table of licence files read
from the local clones:

| Project | Role | Licence |
| --- | --- | --- |
| `jellyfin/jellyfin` | server | GPL-2.0 |
| `jellyfin/Swiftfin` | **iOS + tvOS** client, 4,173★ | **MPL-2.0** |
| `streamyfin/streamyfin` | iOS + Android client | **MPL-2.0** |
| `jarnedemeulemeester/findroid` | **Android-only** client, 4,257★ | GPL-3.0 |
| `jellyfin/jellyfin-androidtv` | Android TV client, 4,562★ | GPL-2.0 |

The Apple clients are MPL; the Android ones are GPL. Google Play tolerates copyleft; Apple's
terms have not.

**This bites us harder than it bit Jellyfin**, whose Apple clients are third-party and were
licensed independently from the start. Ours are first-party by design
([D10](#d10--clients-react-native-shell-native-player-modules-two-engines)), tvOS is planned ([D30](#d30--tvos-is-cheap-from-inside-the-apple-stack-and-never-has-a-web-fallback)), and
[D67](#d67--tvos-never-gates-a-feature) commits that tvOS never gates a feature — which is void if the app
cannot be distributed. Under whole-project GPLv3 the project that says *"the project is judged
on its TV app"* has no Apple TV app.

**So:**

- **Server, media node and relay: GPLv3.** Unchanged, and still where FFmpeg's ladder lands us.
- **Clients and the shared TypeScript SDK: MPL-2.0.** File-level copyleft, so our code stays
  open and the application is distributable. It is also the right licence for
  [D20](#d20--plugin-architecture-constraints-and-four-things-the-catalogue-says-belong-in-the-core)'s claim that the SDK is what third parties build clients on.

**Why not AGPL for the server.** Immich chose AGPL-3.0; we do not, because
[D71](#d71--one-relay-artefact-three-jobs-and-the-core-dials-out-to-it) makes the relay a mode of the same binary that we actively want
people to run on their own VPS, and AGPL attaches a publication obligation to that act. The
cost is that someone may host a modified relay without publishing; since the relay holds no
data ([D72](#d72--the-hosted-relay-has-no-free-tier-holds-no-data-and-has-no-accounts)) there is nothing there to defend.

**Rejected:** a single licence for everything (costs the Apple platform); LGPL for the clients
(weaker than MPL's file-level copyleft for an application rather than a library, and MPL is
what the field actually shipped through review); relicensing later (VideoLAN's took about two
years because it needs every contributor's agreement).

**Status:** Must be settled **before the first client commit**, and the CONTRIBUTING file must
state the per-directory licence from day one. Not legal advice; the App Store position is
inferred from a well-documented history and from what comparable projects ship, not from a
lawyer.

---

## D81 — Play state is an append-only event log plus a guarded position, and positions are Edition-relative

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §3.

Jellyfin keeps one row per `(user, item)` — `PlaybackPositionTicks`, `Played`, `PlayCount`,
`LastPlayedDate`. **No device, no sequence, no history.** Two clients reporting progress are two
writers to one cell and the last one wins. The tracker is the consequence: *"Most movies now
appear to be 75% watched in Continue watching"* (44 comments), *"Media playback randomly starts
over again or jumps back — STILL"* (29), *"Movie favourites are randomly unfavourited"* (33),
*"'Continue Watching' not displaying on home screen"* (30).

**So, two things rather than one:**

1. **A guarded current position** per `(user, edition)` carrying a **monotonic sequence**, so a
   stale device cannot rewind a newer one. Last-writer-wins is the bug; ordering is the fix.
2. **An append-only play-event log** — `started`, `progress`, `paused`, `seeked`, `completed`,
   `abandoned` — with device, server timestamp and the client's own monotonic clock. This is
   what makes offline reconciliation possible at all, what answers *"why does this say 75%
   watched"*, and what removes the reason Tautulli (**6,590★**) exists as a separate program.

**Positions are Edition-relative**, which closes a hole in [D79](#d79--four-levels-not-three-title--edition--version--file-with-collections-orthogonal). Watch state
attaches at the Edition, but a Version may be multi-part: "12:30 into part 2" is not an
Edition-level position. Each File carries its offset within the Version's timeline and the
stored position is in the Edition's. That is also what makes resume survive the 1080p→4K switch
D79's fixture requires.

**Reporting cadence follows Plex's**, which is the field's de-facto shape: on every play-state
change, and otherwise **every ~10 s on LAN/WAN, ~20 s on a metered link**. Cadence is the
transport; the log is the content.

**Two Jellyfin defaults we deliberately do not copy**, both read from `UserDataManager.cs`:

- `if (!hasRuntime) { data.Played = true; }` — *"If we don't know the runtime we'll just have to
  assume it was fully played."* A missing fact must never become a destructive assertion about
  the user's state. Unknown runtime means unknown state.
- `MinResumeDurationSeconds = 300`, which marks anything under five minutes fully played the
  moment it is opened and never resumes it.

Their thresholds are otherwise a reasonable starting point — `MinResumePct = 5`,
`MaxResumePct = 90` — and they are **policy in the core**
([principles §9](../principles/README.md)), configurable, never compiled into a client.

**Status:** Schema. Due before `library-model.md`. The retention policy for the event log is
unmeasured — a household at ~2 writes/second is nothing, but the log is the first table with an
unbounded row count and wants a stated horizon.

---

## D82 — Media segments are typed, attach at the Version, and carry provenance

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §4.

**We planned the plugin and never planned the table it publishes into**, which is precisely the
mistake Jellyfin corrected in 10.10. [plugins-and-workarounds.md](../design/research/plugins-and-workarounds.md)
§3 gives Intro Skipper a full section, calls it the best available specification for
[D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)'s
tier 3, and schedules intro/credit detection as the second tier-3 plugin we write. It also
records — without drawing the conclusion — that the plugin *"contains an EF migrations directory
of its own"* and that afterwards *"the core absorbed the feature (media segments became
native)"*. A plugin carrying its own schema is what a missing core model looks like.

The demand is not in doubt: `intro-skipper/intro-skipper` has **2,780★** plus **968★** on the
archived original it forked from, and Swiftfin carries an open *Skip button for media segments*
request.

**Netflix's is the reference detector** — audio fingerprinting of recurring theme music,
shot-boundary and title-card detection, subtitle timing cues, and human review for the hard
cases. We cannot build that and do not need to: almost all of the value is in the model, and
the detector is replaceable.

**Jellyfin already built the right model**, in 10.10, and their reason for not reusing chapters
is the correct one: *"Unlike chapters, which have no type, media segments can contain type
information, allowing different actions based on the type."* So we keep both — chapters are
authored container metadata, segments are typed and derived — and copy the type set
(`Intro`, `Recap`, `Preview`, `Outro`, `Commercial`, plus an escape hatch) and the
`SegmentProviderId` provenance field.

**Where we differ: segments attach at the Version, not the Item.** A timecode indexes an
encoding, not a work. A 4K remux and a web-dl of the same episode differ in lead-in; a
PAL-speedup rip differs throughout. Jellyfin's `MediaSegment.ItemId` points at a `Video`, which
is [D79](#d79--four-levels-not-three-title--edition--version--file-with-collections-orthogonal)'s conflated Edit/Manifestation/Item, so it cannot express this.
Deriving a sibling Version's segments by runtime alignment is allowed and is cheaper than a
second detection pass; it is recorded as a derivation, not as an observation.

> **Detector economics measured 2026-09-20 —
> [D115](#d115--intro-detection-is-fingerprint-first-and-chromaprint-needs-a-stereo-downmix-or-it-fails-outright).**
> Audio fingerprinting is **sixty times cheaper** than black-frame detection (802× realtime
> against 13.5×), because one is an audio pass and the other decodes every frame. So
> fingerprinting is the primary detector and black-frame the second line. And chromaprint
> **fails outright** on a 5.1 input: it needs `-ac 2` in front of it.

**Detection is a job, and a plugin-shaped one.** It reads decoded audio and video, so by
[D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp) it runs in the node's jail, and by [D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides) it reports
candidate boundaries while the core decides what to store. A **user override** wins over any
detector and survives re-detection, per [principles §3](../principles/README.md).

**Status:** Schema (the table) is due with `library-model.md`; the detector is not, and should
not be first-party at launch. The chapter-derived provider is nearly free and worth shipping
first, because a large fraction of ripped television already carries chapter marks.

---

## D83 — Loudness is measured at scan and applied as gain; downmix has one default and a dialogue control; DRC is per device

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §5.

Nothing in the design mentioned loudness, downmix or dynamic range — in a project where
[D22](#d22--video-audio-and-subtitles-negotiate-as-independent-axes) puts **audio encoding in the core**, which means we are the ones doing
the downmix and the resulting quiet dialogue is ours.

**Netflix fixes it at ingest**: **-27 LKFS ±2 LU, dialog-gated**, true peak ≤ **-2 dBTP**, per
ITU-R BS.1770. Their whole catalogue is loudness-consistent before a player sees it.

We cannot rewrite files ([principles §2](../principles/README.md)), so we take the answer the
music world found under the same constraint: **measure once, store the number, apply gain at
playback.** The node already opens every file; FFmpeg's `ebur128` filter implements EBU R128.
Per audio track we store integrated loudness, loudness range and true peak as node-observed
facts, and the core derives a playback gain.

**Stated limitation:** FFmpeg measures *programme* loudness, not Netflix's **dialogue-gated**
figure — dialogue gating needs Dolby's Dialogue Intelligence, which FFmpeg does not have. Our
number is comparable within our own library and is not the same quantity. Say so; do not imply
parity.

**Downmix gets one correct default and one control, not a dropdown of five.** Jellyfin ships
`None`, `Dave750`, `NightmodeDialogue`, `RFC7845` and `AC-4`, and its own documentation records
a dialogue complaint against nearly every one — *"too low perceived dialog"*, *"dialogues might
sound too quiet compared to music"*, *"Too quite sound effects is not ideal for movies"*,
*"too-quiet center channel"*. Five wrong answers in a dropdown is a problem that has been
handed to the user. We ship a standard matrix as the default and a **dialogue-boost** control
that is a user preference of the same kind as subtitle size, riding
[D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s preference table.

**Dynamic range is a property of the room, so it is a per-device preference.** AC-3 and E-AC-3
carry `dialnorm` and DRC metadata; FFmpeg's decoder applies DRC by default (`drc_scale = 1`),
which is right on a train and wrong in a home cinema. Kodi carried this as a bug for years and
Jellyfin has it open (#10148). The correct value depends on a fact only the device knows, which
is [D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) applied to audio: the client declares it, per device, and the core
honours it.

**Rejected:** normalising files in place (violates §2); a server-wide "night mode" setting (the
correct value differs per listener and per room); leaving it to the client (the core performs
the downmix, so the client cannot fix a mix it never receives).

**Status:** **Measured 2026-09-20** and the worry was misplaced. `ebur128` runs at **102×
realtime** and is *audio-only* — 5.35 s on six cores against 5.89 s on one, so it is one
core's work and parallelises across files rather than within them. For the reference library
of 3,167 hours that is **31 hours single-core**, against 9.2 days for trickplay. It is one of
the cheap passes, not one of the expensive ones
([D114](#d114--a-pass-costs-what-it-costs-because-of-one-thing-whether-it-decodes-video)).
Whether it is deferrable per library remains a policy question rather than a cost one.

---

## D84 — Certifications are `(system, code)` with a per-system order; a kids profile is a grantee plus a predicate

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §6.

Parental controls were absent. `jellyfin-androidtv`'s **third-most-reacted issue** is *PIN
protection for users* (+56), behind only the playback rewrite and SyncPlay.

**Netflix's model separates two levels and the second is the one homegrown versions forget.**
Per profile: a profile-lock PIN, a maturity ceiling, and explicitly blocked titles. Per
account: **a PIN required to create a profile** — without which a child simply makes an
unrestricted profile and every other control is theatre.

**The schema trap: a rating is not a number.** Netflix's own help text — *"Maturity ratings are
either set by Netflix or by a local standards organization... Different countries use different
rating systems."* BBFC 15, FSK 16, MPAA R, ACB MA15+ and PEGI 18 are not points on one scale,
and TMDB returns certifications as a list keyed by country. So a certification is
**`(system, code)` with an ordering defined per system** — the same per-locale split
[D45](#d45--metadata-is-multi-language-in-the-schema-chosen-per-request) built for names, and it fails the same way if flattened, except that
flattening a rating fails **open**.

**The access half is nearly free**: a kids profile is a grantee plus a predicate, which is
[D76](#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed) again — a fifth binding of [D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable)'s rules engine rather
than a subsystem. It inherits D76's fail-closed rule, which is the right direction here too: an
unresolvable predicate must show nothing, not everything. And an item with **no** certification
in the profile's system is *not* below the ceiling; it is unrated, and unrated is a separate,
explicit choice rather than a silent pass.

**Status:** Schema, due with `library-model.md`. The per-system orderings are a data table we
must curate — there is no authoritative machine-readable source covering every national system,
so it starts with the handful the providers actually return and grows.

---

## D85 — The API has a compatibility contract, and it covers three protocols

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §7.

Jellyfin's 12.0 cycle is the cautionary case, and the source on master (13.0.0, read
2026-09-19) shows they did **two different things** to two legacy surfaces with nothing in the
process distinguishing them. The `/emby/` and `/mediabrowser/` route prefixes are simply
**gone** — no occurrence anywhere in `Jellyfin.Api` or `Jellyfin.Server`, so a client calling
them gets a 404 mid-operation. But `X-Emby-Authorization` was **not** removed: it is read when
`EnableLegacyAuthorization` is set, a bool declared with no initialiser and therefore off by
default, recoverable with one setting. The second is a good deprecation; the first is not. Add
a runtime move to .NET 10 that broke every plugin built for 10.11. We have no policy at all.

**Three things make this worse for us than for them:**

1. **Our clients are first-party and ship through app stores.** A server fix is a `docker pull`;
   a client fix is a review queue. The server will routinely be newer than the client.
2. **Televisions do not update.**
3. **[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) puts decision logic in the client.** A thin renderer can be fixed
   from the server; a client that probes its own decoder and picks a rendition cannot be.

And [D18](#d18--the-jellyfin-compatible-surface-is-an-adapter-and-it-quarantines-device-profiles)'s Jellyfin adapter makes us additionally responsible for
*someone else's* compatibility surface, against clients we do not control.

**The contract:**

- **Additive only.** A response field may be added; an existing field never changes meaning or
  type. New behaviour arrives as a new field or a new endpoint.
- **Removal needs a deprecation window** measured in releases, announced in the OpenAPI document
  and in a `Deprecation`/`Sunset` response header while the endpoint still works.
- **A removal ships with the switch that undoes it**, for one release cycle — the pattern
  Jellyfin got right with `EnableLegacyAuthorization` and skipped for the route prefixes. It
  turns "broken, go read a forum" into "broken, and the release notes name the flag".
- **The server declares a minimum supported client revision**; the client declares the revision
  it was built against, in the handshake that already exists for capabilities.
- **A mismatch produces one legible message** — naming the client, the server, and what to do —
  at the handshake. Never a 404 in the middle of playback.
- **CI diffs the generated OpenAPI document against the last release** and fails a breaking
  change that carries no explicit waiver. The document is already generated from code and the
  build is already gated on it being current ([D20](#d20--plugin-architecture-constraints-and-four-things-the-catalogue-says-belong-in-the-core)); this adds one comparison.

**It applies to three protocols.** [D61](#d61--containment-now-detection-later-and-plugin-shaped-four-affordances-to-keep-open) already requires the node wire
format to skip unknown record kinds. Core↔relay and core↔client need the same rule stated, and
the shared version inventory of D61 and [D74](#d74--bug-reports-pseudonymise-rather-than-redact-and-fan-in-across-components) is where it belongs.

**Status:** Cheap now, and the CI gate is the part that must exist before the first public
release rather than before the first client. The deprecation window's length is unset until
there is a release cadence to measure it in.

---

## D86 — Adaptation is the client re-picking on a buffer signal, and it puts bandwidth transcoding on remote access's critical path

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §8.

[overview.md](../design/overview.md) lists "a Netflix-style encoding ladder" as not-building,
because *"bandwidth is free on a LAN"*. Correct for the LAN — and the headline feature and the
entire commercial proposition is **remote access**, where bandwidth is a hotel Wi-Fi and the
design offers a ranged GET of a 20 Mbps remux and nothing else. The rendition list is resolved
once, at the start. When the link degrades, playback stalls.

**Netflix published the correction to the obvious approach**, on their own production traffic:
Huang, Johari, McKeown, Trunnell and Watson, *A Buffer-Based Approach to Rate Adaptation*,
SIGCOMM'14. Choosing the rate from **buffer occupancy alone** — no bandwidth estimation at all —
cut the rebuffer rate by **10–20% against their own production algorithm**, and their conclusion
is exact: *"capacity estimation is unnecessary in steady state; however using simple capacity
estimation (based on immediate past throughput) is important during the startup phase."* BOLA
is the same family and is the default in dash.js.

**So we still build no encoding ladder, and we do build three things:**

1. **Say it plainly in `remote-access.md`:** direct play over a WAN has no adaptation. A user
   should not discover that in a hotel.
2. **The client already picks; let it re-pick, on its buffer.** [D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) put
   rendition choice on the client, which means the correct adaptation input is already in the
   right place and is the one Netflix's paper says to use. No server-side bandwidth estimator.
   This is D2 turning out to be more right than it was argued for.
3. **Accept the roadmap consequence.** On a constrained remote link the only lower renditions
   that exist are ones a transcoder makes. `transcoding.md` §(b) is currently Phase 4 and
   "deliberately late"; it is **on remote access's critical path**, and remote access is the
   paid tier.

**And an invariant collides, usefully.** [D23](#d23--axis-switching-is-mid-session-by-default-burn-in-is-the-one-thing-that-breaks-it) says no operation on one axis
may restart another, with video named as the expensive one — but a bandwidth downshift *is* a
video-axis change, in the one situation where a restart is least affordable. The mechanism that
fixes it is already specified in `transcoding.md` §(b): the source keyframe grid is the segment
grid for **every** rendition including passthrough, so variants are interchangeable. It was
specified without a consumer; this is the consumer. **Segment-grid alignment is therefore a
requirement of remote access, not a detail of transcoding.**

**Status:** The buffer thresholds are unmeasured and are not Netflix's — their reservoir was
tuned for a CDN and a global audience, ours has a 20 Mbps origin one hop away or a relay leg
with very different characteristics. Belongs in
[performance-targets.md](../design/plans/performance-targets.md).

---

## D87 — A download is a rendition plus a lease, and it is self-contained

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §9.

Offline download is the most-wanted client feature in the field — Swiftfin's *Local Downloads*
at **+145** is the most-reacted issue on that tracker, and six of findroid's top ten are
download features. Our design treats it as a **client** state machine
([D29](#d29--a-native-client-reimplements-the-cheapest-cross-platform-logic-is-server-side-logic)) and says nothing about what the server owes it.

Two of those requests are server-side capability in disguise: *"Transcode downloads to save
storage space"* (+32) and *"Allow transcoding of multimedia content"* (+52). People want the
download to be **smaller than the original** — which is the compatibility transcoder used for a
completely different reason than compatibility, with a different trigger and a different
retention policy.

**And there is a cross-cut the font work did not notice.** [D53](#d53--font-coverage-is-not-assumable-the-server-serves-fonts-on-demand)–[D58](#d58--dont-hand-out-a-url-the-client-may-not-use-an-authorisation-check-that-fires-is-a-signal)
build an on-demand font pipeline: Noto served by language, subsets produced at scan time,
embedded fonts fetched on explicit opt-in. **Every one of those is an HTTP fetch.** On a plane,
a downloaded film with a Japanese subtitle track renders as tofu. The same is true of the
extracted subtitle tracks, the artwork, and [D82](#d82--media-segments-are-typed-attach-at-the-version-and-carry-provenance)'s segments.

**So: a download is a rendition choice plus a lease.**

- **Self-contained.** The offline manifest enumerates every resource the client must also
  store — subtitle tracks, the font subset for each, artwork, chapters, segments — and a
  download is not complete until they are present.
- **A lease**, so the server knows a prepared rendition is still referenced and must not reap
  it, and so an abandoned device's storage is eventually reclaimed.
- **Deletions reconcile through the change journal** ([D8](#d8--a-change-journal-and-delta-sync-cursor-exist-in-the-first-schema)), which exists for
  exactly this and still has no consumer.
- **Play state reconciles through [D81](#d81--play-state-is-an-append-only-event-log-plus-a-guarded-position-and-positions-are-edition-relative)'s event log**, which is why the log
  carries the client's own monotonic clock: an offline device's events are ordered relative to
  each other even when its wall clock is wrong.

**Status:** Client work is Phase 3+; the **manifest shape and the lease** are due earlier,
because an offline manifest that cannot enumerate its dependencies is a migration. A television
does not need any of this ([shared-core.md](../design/research/shared-core.md)).

---

## D88 — Casting is remote control of another client, which is Watch Together with a group of one

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §10.

Chromecast appears once in the design, as a "v2 client". The demand is real — Swiftfin
*iOS — Chromecast* (+43, its second-most-reacted issue), Swiftfin *SharePlay* (+29), *tvOS Deep
Links* (+26), androidtv *Deep Link Support* (+22).

**We have already designed it without noticing.** [D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods) defines the
per-client Watch Together surface as **six methods** — join, leave, report position, accept
play/pause/seek, report readiness, report buffering. Casting is one device driving another
device's playback: the same six methods, group size one, a remote controller, on the LAN, with
voice off.

Saying so now costs nothing and prevents the usual outcome, which is a second, parallel,
subtly different remote-control protocol that then diverges. It also makes a **Chromecast
receiver** exactly what [D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) already says a client is, and it makes
**AirPlay** free wherever the Apple client uses AVPlayer.

**Deep links are the other half and are an API commitment**: an item needs a stable, shareable
URL form before anything publishes one, and changing it later breaks every bookmark and every
companion app.

**Status:** No new mechanism, so no new risk — but it is a claim that the six-method surface is
sufficient for a case it was not designed against, and that is only proven by building the Cast
receiver. If it needs a seventh method, that is worth knowing before the surface is frozen.

---

## D89 — Deletion is opt-in per library, two-phase, and names every file

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §11.

The tree contains no mention of deleting media — not "trash", not "recycle", not "delete". The
two positions that exist point in opposite directions: [principles §2](../principles/README.md)
mounts media read-only, and [D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable) makes retention **mark** items rather than
delete them, deliberately, because Maintainerr and Janitorr delete and people are afraid of
them.

But "mark and review" has no "act", and users delete things. Unstated, this ends with
`os.Remove` in a handler and the read-only promise becomes a comment.

**The position:**

- **A writable path is opt-in per library** and off by default. Read-only remains the default
  mount and the default answer.
- **Deletion is two-phase**: mark, then a confirmation that **names every file and its size**.
  [principles §2](../principles/README.md) already says destructive actions must say exactly
  what they will destroy; this is that rule's first real consumer.
- **It happens in the node**, never the core, because the core does not touch media filesystems
  ([D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do)) — so deletion is a node command with a node report, and it is
  visible in the journal like every other change.
- **Retention still only marks.** A rule may never delete. The user acts on the marked set.

**Status:** The interaction with a shared grantee is unspecified — presumably a grantee can
never delete, but "presumably" is not a design. Also unresolved: whether an opt-in writable
library should use a trash directory rather than unlinking, which costs free-space surprises in
exchange for an undo.

---

## D90 — Playback presentation is a persisted per-user preference set, and captions have a statutory floor

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §12.

[D78](#d78--server-rendered-subtitle-overlays-are-a-third-rendition-and-they-dominate-burn-in) said text subtitles stay the default because "a bitmap cannot honour a
user's subtitle-size setting, and subtitle size is accessibility rather than preference". That
is right and it is one row of a seven-row table that is **law** in our two largest markets.

**47 CFR §79.103 (CVAA, United States)** requires user control of: the **eight CEA-708 fonts**,
assignable per style; size from **50% to 200%**; text colour overriding the authored colour from
a palette of at least eight; background colour likewise; background opacity
(opaque/semi-transparent/transparent); caption **window** colour and opacity, separately; and
**five character-edge attributes** (none, raised, depressed, uniform, drop-shadow). Since
**2026-08-17** those settings must also be *"readily accessible"*, not merely present.
**Directive (EU) 2019/882**, applicable since **2025-06-28**, requires the player to expose
caption size and font controls and to make audio description reachable.

**Three consequences.**

1. **[D55](#d55--the-served-font-set-is-the-cta-708-style-axis-in-variable-fonts-subset-per-subtitle) is already on the regulatory path and did not know it.** The served
   font set was chosen as "the CTA-708 style axis, six upright variable faces covering the seven
   real caption styles" for typographic reasons. That *is* the statutory list. Make the
   eight-style mapping explicit and it becomes a conformance artefact instead of a coincidence.
2. **The overlay rendition forecloses every control in that table**, not just size. So it is
   never the only offer for a caption track, and where it is chosen the client says which
   controls it lost — [principles §3](../principles/README.md), applied to accessibility.
3. **Audio description is a first-class track kind**, not a disposition flag we happen to parse.
   It must appear in the client contract and in the preference axes, not only in
   [identification.md](../design/plans/identification.md)'s derivation table.

**And the presentation set is wider than captions.** Grouped here because they are one thing —
per-user settings that ride [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s preference table at the same scopes:

- caption font, size, colour, background, window, edge (above);
- **subtitle and audio sync offset**, a signed millisecond value. This appears on **four**
  trackers — androidtv *Adding audio / subtitles sync menu* (+31), findroid *Subtitle timing
  change* (+17), jellyfin-web *Subtitle offset clamped* (+8), streamyfin *Subtitle offset* (+8).
  It is not niche, and it is a column now or a migration later;
- **dialogue boost** and the per-device DRC preference ([D83](#d83--loudness-is-measured-at-scan-and-applied-as-gain-downmix-has-one-default-and-a-dialogue-control-drc-is-per-device));
- playback rate, which is local unless a Watch Together group shares it
  ([D66](#d66--no-clock-synchronisation-monotonic-local-time-and-bounded-round-trips) — a participant at 1.25× is not drifting and the reconciler must
  not fight them).

**Status:** Conformance is claimed against a reading of the regulations, not against a
certification. Nobody has tested whether libass can express all five CEA-708 edge attributes —
if it cannot, that is a gap in the renderer and must be found before the claim is made
anywhere user-facing.

---

## D91 — A `/metrics` endpoint, off by default and local when on

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §13.

Sorted by reactions, the **second-most-reacted open issue in Jellyfin's server tracker** — ahead
of every security issue and every feature — is *Improve Prometheus Exporter with JF Metrics*
(+63, open since 2020-04).

Our two observability surfaces are both incident-shaped: [D74](#d74--bug-reports-pseudonymise-rather-than-redact-and-fan-in-across-components)'s bug report
is generated after something went wrong, and [D73](#d73--each-component-reports-according-to-its-own-visibility-blind-is-not-opaque)'s status records are
consumed by the core. Neither answers "is it healthy now", and neither lets
[performance-targets.md](../design/plans/performance-targets.md) be enforced outside CI — which
is exactly how Jellyfin shipped a 30–50 second home page, because a benchmark that only runs in
CI cannot notice that the library got slower as it grew.

This audience already runs Prometheus and Grafana. The endpoint exposes what
[D73](#d73--each-component-reports-according-to-its-own-visibility-blind-is-not-opaque) already collects — per-role health, job queue depth and age, scan and
snapshot timings, stream counts, rendition decisions and their reasons, error rates — and
nothing that names a film, a path or a person, which is the same line
[D74](#d74--bug-reports-pseudonymise-rather-than-redact-and-fan-in-across-components) draws.

**Off by default, and bound to localhost when enabled unless the user says otherwise.** It is a
data-exposure surface and [D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise) is a promise rather than a preference.

**Status:** The metric set is a guess until there is a running system. The discipline that
matters is that a metric naming content is a bug, enforced the same way D74's pseudonymisation
is.

---

## D92 — A failed rendition falls to the next one, and the client remembers; the server still has no device database

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §14.

[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) is right that the client is the only party that can ask its own decoder,
and [playback-survey.md](../design/plans/playback-survey.md) is right to *"measure, do not just
ask"*. Neither covers the runtime case: the probe said yes, and the decoder failed at minute
three. That is the most common real-world playback failure and the design has no response to it.

The large operators answer it with device certification programmes and per-device platform
stacks, which are not available to us. Ours is assembled from parts that already exist:

- **A ladder, not a single pick.** The client already sends a *prioritised* list
  ([D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list)); on failure it falls to the next entry rather than to an error
  screen, and says it did.
- **A client-local memory** of (probe answer → actual failure) for its own hardware. This is not
  a device-capability database and must not become one: it never leaves the device except as an
  observation, it is keyed by nothing but that device's own history, and clearing it must be
  possible and harmless.
- **Observations fan in.** [D73](#d73--each-component-reports-according-to-its-own-visibility-blind-is-not-opaque) already routes client observations to the
  core, which is where a household can see that every Fire TV in the house fails the same way —
  and where [device-lab.md](../design/plans/device-lab.md) gets a production data source it
  currently lacks.

**[D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) holds unchanged:** the server never builds a device-capability
database and never guesses on a client's behalf. A memory the client keeps about itself is the
opposite of a table the server keeps about everyone.

**Status:** Unmeasured — the failure rate this exists to handle is one of the numbers Phase 0
should produce, and if it is very low the ladder still needs to exist but the memory may not.

---

## D93 — Device pairing is RFC 8628-shaped

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §15.

Signing in on a television is absent from the design. It is the first interaction every user
has, on the device with the worst text entry ever shipped, and the first one a *shared* user has
on somebody else's TV.

**The standard is RFC 8628**, the OAuth 2.0 Device Authorization Grant — the TV shows a short
code and a URL, the user opens it on a phone, the TV polls — and it is what the large streaming
services use. Jellyfin hand-rolled the same shape as Quick Connect
(`QuickConnectResult { Secret, Code, DateAdded, DeviceId, DeviceName, AppName, AppVersion }`),
which is a sound design that is nonetheless bespoke where a standard exists.

[D20](#d20--plugin-architecture-constraints-and-four-things-the-catalogue-says-belong-in-the-core) already puts OIDC in the core for Phase 1, so the device grant is nearly
free **if the auth design anticipates it** and awkward if it does not: it changes what a session
is, because the authorising act happens on a different device from the one being authorised. It
also interacts with sharing — a grantee's television must be pairable without the owner typing a
password into somebody else's living room — and with [D85](#d85--the-api-has-a-compatibility-contract-and-it-covers-three-protocols), because the
pairing endpoint is the first surface a very old client touches and therefore the one that must
never break.

**A consequence: there is no `auth.md`.** Identity, sessions, token scope and lifetime,
revocation, device pairing and the sharing hand-off are currently a bullet in
`library-model.md`, two open questions in `threat-model.md`, and a section in
`remote-access.md`. That is a Phase 1 subsystem with no plan.

**Status:** Shape only. The token model, its lifetimes and revocation are
[threat-model.md](../design/plans/threat-model.md)'s open question and remain open.

---

## D94 — Disk spin-down is a cost of the snapshot interval, and `archival` means untouched

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §16.

Jellyfin issue #7689 is *Reduce hard disk access during playback*, and the comparison inside it
is that Plex has nearly no disk activity where Jellyfin has a lot.

**That comparison is wrong, and finding out makes the case stronger.** Plex's own forum, read
2026-09-19: *Plex is constantly spinning HDD up and down. How do I fix this?* (22 posts),
*HDD never turns off while PMS is running*, *Plex Media Server wakes up Synology NAS every
hour*, *Certain media files accessed every day at the same time — why?*. **Both products do
it.** So this is a property of the workload rather than of one implementation, and a design
that assumes it will not happen to us is assuming its way out of the only evidence available.
For a NAS audience — and [test-hardware.md](../design/research/test-hardware.md) names an 8 GB
Celeron NAS as the honest low end — spinning drives cost power, noise and wear, and "it keeps my
array awake" is the kind of complaint that ends in an uninstall.

[D60](#d60--snapshotting-and-discarding-are-different-operations-with-different-triggers) made snapshots temporal for good reasons, and `snapshot_interval` is
currently justified against two competing pressures: a silently-dead watcher wants it short, an
expensive SMB enumeration wants it long. **There is a third and it was not written down: the
interval is how often the disks must wake.**

Two rules follow, both in `scanner.md`:

- **Batch the wake.** All roots on one physical device enumerate together, so one pass spins the
  array up once. The adaptive per-directory cadence already specified must not be allowed to
  become a continuous trickle of small wake-ups, which is strictly worse than one long pass.
- **`archival` means untouched.** The plan already allows a user to declare a library archival.
  Make the meaning concrete: no walk, no probe, no thumbnailing, no loudness pass — the journal
  only, if a watcher is available. And the node **reports** that it is honouring it
  ([D73](#d73--each-component-reports-according-to-its-own-visibility-blind-is-not-opaque)), so "why is my new file missing" has an answer rather than a
  mystery.

**Status:** Unmeasured. The right `snapshot_interval` for a spun-down array is a different
number from the right one for an SSD, which argues it should be per-root and derived from the
setup probe `scanner.md` already performs, rather than a global constant.

---

## D95 — People are entities with provider identity; a name is never a key

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §17.

`library-model.md` has no `Person`. Jellyfin has *Actors with the same name appear as one
person* (27 comments, open since 2020-06) and its music twin *Music artists with the same name
are merged while MBIDs are ignored* (29 comments) — whose title states the cause exactly: a
stable external identifier was available and the code matched on the display string.

The rule is the one [D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index) already applies to items. A Person is an entity with
`{provider, provider_id}` identity in a side table; credits are typed rows — `cast` carrying a
character name and an order, `crew` carrying a job — and **the display name is never the key**.
The person's name is also a translatable string ([D45](#d45--metadata-is-multi-language-in-the-schema-chosen-per-request)), because names are
written differently in different scripts.

**Status:** Schema, due with `library-model.md`. Small, and retrofitting it means re-matching
every credit in the library. Deduplicating two Person rows that turn out to be the same human
(one from TMDB, one from TheTVDB) is the same merge problem as elsewhere and is unspecified.

---

## D96 — An episode's position is `(ordering, season, number)`, and a series declares its orderings

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §19.

Episode ordering appeared in the tree three times and all three were about **providers** —
TheTVDB is good at it, regional providers differ, ordering is a per-field provenance question.
The **library model** had nowhere to put it.

**This is [D79](#d79--four-levels-not-three-title--edition--version--file-with-collections-orthogonal)'s disease in a different organ**, and Jellyfin's source
carries the same signature. An episode's position is `IndexNumber` + `ParentIndexNumber` — one
position. Specials, which do not fit, get three nullable escape hatches and a coalescing chain:

```csharp
public int? AirsBeforeSeasonNumber  { get; set; }
public int? AirsAfterSeasonNumber   { get; set; }
public int? AirsBeforeEpisodeNumber { get; set; }
public int? AiredSeasonNumber =>
    AirsAfterSeasonNumber ?? AirsBeforeSeasonNumber ?? ParentIndexNumber;
```

And which ordering a series uses is `Series.DisplayOrder`, **a bare string**, compared against
magic values in two places with **two different comparison modes** (`StringComparison.Ordinal`
in the TMDB client, `OrdinalIgnoreCase` in the library manager). Three nullable side-columns
plus a magic string is what a missing relation looks like — exactly as four version mechanisms
were in D79.

**So:** aired, DVD, absolute and a provider's alternate orders are all **orderings of the same
set of episodes**. A position is `(ordering, season, number)`; an episode has one per ordering
it appears in; the Collection that is the series declares which orderings exist and which is
the default; and a special that airs between two episodes is a position in an ordering rather
than three nullable columns.

**Anime makes this unavoidable rather than academic** — absolute numbering, season splits, and
providers that disagree with each other.
[tech-metadata.md](../design/research/tech-metadata.md) already records that generic providers
are simply *wrong* there.

**And it is not only cosmetic.** [play-state.md](../design/plans/play-state.md) defines Next Up
as *the earliest unwatched episode after the latest completed one*, which is a statement about
an ordering. With the ordering explicit, two people in one household can watch the same series
in different orders, and that costs nothing extra. With one position it is impossible.

**Rejected:** a per-series string (Jellyfin's; magic values and no per-user choice); storing
only the provider's preferred order and re-fetching to change it (a network call to sort a
list); computing absolute numbering on the fly (it is wrong whenever a season split disagrees
with the provider).

**Status:** Schema, due before `library-model.md`. Whether the ordering choice is per series or
per `(user, series)` is open — per user is strictly more capable and the extra column is
trivial, so the argument against it is UI, not storage.

---

## D97 — An extra is a typed relation, not an Edition and not a Version

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §20.

Trailers, deleted scenes, featurettes, behind-the-scenes, interviews, bloopers, shorts — and
**theme songs and backdrop videos**, which is `jellyfin-androidtv`'s *Video Backdrop and Theme
song playback* (+37). None of them appeared anywhere in the tree.

They break the four-level model if forced into it. A trailer is not an **Edition** — it is not
a cut of the film. It is not a **Version** — it is not an encoding of the film. It is a
different, shorter work belonging *to* the film. With nowhere to put it there are two outcomes,
both bad and both observable in the field: extras become top-level items that pollute the
library, or they become a side-array of paths.

Jellyfin's vocabulary is right even though its storage is not — `Clip, Trailer,
BehindTheScenes, DeletedScene, Interview, Scene, Sample, ThemeSong, ThemeVideo, Featurette,
Short`, with each extra a full `BaseItem` carrying an `OwnerId` and excluded from listings by
convention rather than by structure.

**So: an extra is a typed relation from a Title or an Edition to a work**, which holds its own
Versions and Files like anything else, never appears in the main listing, and can never be
confused with a cut. The type is a closed enumeration and the relation carries an order.

Two things fall out cheaply. **ThemeSong and ThemeVideo being extras** is what makes that +37
request nearly free — and gives `jellyfin-web`'s open *theme song / theme videos volume
adjustments* issue an obvious home beside
[D83](#d83--loudness-is-measured-at-scan-and-applied-as-gain-downmix-has-one-default-and-a-dialogue-control-drc-is-per-device)'s gain handling. And an extra is a work, so it gets subtitles,
loudness, segments and play state by construction rather than by special case.

**Status:** Schema, due before `library-model.md`. What is open is **discovery**: the field
identifies extras by magic folder and filename conventions (`extras/`, `behind the scenes/`,
`-trailer`), which is filename parsing and therefore belongs in the **core** over stored
observations ([D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret)), not in the node. Whether a downloaded trailer that is
not on disk at all (a provider URL) is the same kind of row is also open, and probably not.

---

## D98 — A sidecar carries what is true about the work; the database carries what is true about a person

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §21.
Resolves a three-way contradiction between [D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index), [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series) and
[D77](#d77--back-up-with-vacuum-into-back-up-only-the-irreplaceable-and-test-the-restore) that has been sitting in the tree since the backup round.

### The contradiction

- [D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index): *"Anything the user authored is mirrored to disk so it survives us."*
- [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series): track-selection preferences *"are ours, not derived from the files,
  so they mirror to sidecars too."*
- [D77](#d77--back-up-with-vacuum-into-back-up-only-the-irreplaceable-and-test-the-restore) and [backup-restore.md](../design/plans/backup-restore.md), correctly:
  watch state *"must **not** go to sidecars: it is per-user, and writing it beside the media
  would expose one household member's viewing to anyone with filesystem access. The sidecar
  mechanism structurally cannot carry it."*

D77's argument is right and it does not stop at watch state. Track preferences are per-user.
So are favourites, ratings, presentation settings and the play log. The same reasoning excludes
all of them, and D24 says the opposite.

### The rule

> **A sidecar carries what is true about the work. The database carries what is true about a
> person.**

| Sidecar-able | Database only, and therefore backup-critical |
| --- | --- |
| identification override | watch state, resume position, the play log ([D81](#d81--play-state-is-an-append-only-event-log-plus-a-guarded-position-and-positions-are-edition-relative)) |
| metadata corrections | favourites and user ratings |
| chosen artwork | track-selection and presentation preferences ([D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series), [D90](#d90--playback-presentation-is-a-persisted-per-user-preference-set-and-captions-have-a-statutory-floor)) |
| tags and labels on the item | users, credentials, shares ([D76](#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed)), rules ([D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable)) |

The test is the same one that makes the privacy argument work: **the media tree is shared**.
It is shared with everyone in the household, often with a grantee, and sometimes with a
seedbox. Anything written beside a film is visible to all of them. That is fine for "this is
actually the 1982 cut" and unacceptable for "Karl watched this at 2 a.m."

**This supersedes the parenthesis in [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)** — *"they mirror to sidecars
too"* — and nothing else in it.

### The second half: writing a sidecar is not free, and the promise is conditional

Nobody noticed that the mechanism collides with two other decisions:

- [principles §2](../principles/README.md) mounts **media read-only by default**;
- [D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) says **the core never touches a media filesystem**.

So a sidecar write requires a **writable path, opt-in per library** — the same opt-in
[D89](#d89--deletion-is-opt-in-per-library-two-phase-and-names-every-file) needs for deletion, and it should be one setting rather than two —
and it happens **in the node**, as a node command with a node report, like every other change
to the media tree.

Which means the README's *"your metadata corrections are written back to disk so they survive
us"* is **conditional and currently reads as unconditional**. The honest version is that there
are two durability mechanisms and they do different jobs:

| Mechanism | Protects against | Requires |
| --- | --- | --- |
| **Automatic database backup** ([D77](#d77--back-up-with-vacuum-into-back-up-only-the-irreplaceable-and-test-the-restore)) | disk failure, a bad migration, moving machines | nothing; on by default |
| **Sidecars beside the media** | *us* — the project dying, or you leaving for another server | an opt-in writable path |

The first is the durability answer. The second is the **portability** answer, and it is the one
the word "survive" was doing work for. Say both, and say which is on by default.

**Rejected:** writing per-user state to sidecars behind a warning (the exposure is to anyone
with filesystem access, which includes people the user cannot un-share with); a hidden
dot-directory beside the media (same filesystem, same readers, plus it breaks the "any other
media server can read it" property that was the point); requiring a writable path (it is the
read-only default that makes [principles §2](../principles/README.md) mean anything).

**Status:** The sidecar format is still undesigned — [backup-restore.md](../design/plans/backup-restore.md)
already flags that — and this decision narrows *what* goes in it without settling the format.
NFO is the field's de-facto answer and reading it is already planned; writing it is a
compatibility question nobody has examined.

---

## D99 — The core does not decode an image either, and community-uploaded artwork is why

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §22.

[D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp) keeps FFmpeg out of the core process because media is
attacker-supplied data. The rule was written about **media files** and the same argument covers
a case nobody drew the conclusion for, even though
[threat-model.md](../design/research/threat-model.md) §5 states the premise outright:

> *"posters are the one binary we fetch **from a third party by default** — TMDB — rather than
> from the user's own disk."*

And TMDB is **community-editable**. Anyone can upload a poster. So an image arriving from a
metadata provider is attacker-supplied in the ordinary sense, not the theoretical one — and
unlike a film, it arrives *unprompted*, during a background refresh, from the internet, on a
schedule.

The exploitability is not speculative either. `threat-model.md` already counts **39 entries in
CISA's Known Exploited Vulnerabilities catalogue touching media, image or font parsing**, with
Apple's Image I/O (CVE-2025-43300) and Samsung's `libimagecodec` (twice) in a five-row sample.
Add the one that hit everything: **CVE-2023-4863**, the libwebp `BuildHuffmanTable` heap
overflow, added to KEV on 2023-09-14 and exploited in a 0-click chain — a library that ships
inside Chrome, Electron, GIMP, LibreOffice, Telegram, 1Password *and FFmpeg*.

**So the rule generalises: the core never parses a media or image file.** Concretely:

- **Derived sizes are produced in the node**, in the same jail as everything else
  ([artwork.md](../design/plans/artwork.md) already puts them there; this is why).
- **A provider image fetched by the core is bytes in flight, not a decoded image.** It is
  handed to the confined worker for validation and derivation and never touched by the core's
  own process — which is also the shape Apple chose for BlastDoor, already quoted in
  `threat-model.md` §5: *isolate, parse, transcode, validate*, then let the untrusted thing out
  only as a safe re-encoding.
- **Serving bytes is not parsing them.** The core may stream a derived image from `cache/`
  without ever decoding one, exactly as it streams a film it never opened.

**The pleasing part:** the confined worker already exists for FFmpeg
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)), the node already decodes media, and
[artwork.md](../design/plans/artwork.md) already put derivation there for performance reasons.
This decision costs nothing to hold and would cost a rewrite to retrofit — which is what makes
it worth writing down before somebody reaches for an image library in a handler.

**Status:** Provisional in one respect — a pure-Go image decoder in a `Landlock`+`seccomp`
worker is a different risk profile from libwebp in C, and Go's `image/*` packages have had DoS
bugs rather than memory-corruption ones. That might argue for decoding in the core after all.
It does not change the answer, because [D9](#d9--server-language-go-one-language-on-the-server)'s single binary means the worker
is the same code either way and the confinement is nearly free — but the reasoning should be
re-checked if an image library with a C dependency is ever proposed.

---

## D100 — A destructive command names its target by identity, not by path

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §23.
Resolves a collision between [D89](#d89--deletion-is-opt-in-per-library-two-phase-and-names-every-file) and [D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config) that was
created the same day.

[D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config) puts an **ordered command list in the node's config** — `cancel`,
`scan`, `rescan <root>`, `pause`, `resume`, plus `snapshot` from
[D60](#d60--snapshotting-and-discarding-are-different-operations-with-different-triggers) — and states a rule about it in as many words:

> *"A node that loses `observed_config` re-executes. Every command in the vocabulary must
> tolerate that, which they do — and any future command that cannot must not be added to this
> channel."*

[D89](#d89--deletion-is-opt-in-per-library-two-phase-and-names-every-file) then made deletion a node command. **`delete <path>` does not
tolerate re-execution**, and the failure is not the harmless one it looks like. Re-executing a
delete after the file is gone is fine. Re-executing it after *a different file has appeared at
the same path* — a re-rip, a rename, a torrent client reusing a name — deletes the wrong film,
silently, from a channel designed to be replayed.

**So:** a destructive command carries the **identity of what it is destroying**, not only its
location — size and mtime at minimum, a content hash where one was taken — and the node
**refuses and reports** rather than proceeding when the target does not match. A refusal here
is [principles §10](../principles/README.md)'s alarm, not a branch: in a correct system it
never fires, so every firing is worth looking at.

Two smaller consequences:

- **The confirmation the user sees and the guard the node enforces are the same data.**
  D89 already requires naming every file and its size before deleting; that list *is* the
  identity set. Cheap, and it means the thing shown and the thing checked cannot drift.
- **The general rule is worth stating beyond deletion.** The D38 channel carries *desired
  state*. A command that is a one-shot event rather than a state — and every destructive
  command is — either gets an identity guard that makes replay safe, or it does not belong on
  that channel at all. Anything that cannot have one belongs in the job system.

**Status:** **Measured 2026-09-20** — hash-on-delete is the answer, and it is not even a
trade-off. A sampling hash (head, tail and size) costs **3.2 ms**
([D116](#d116--the-content-hash-is-a-sampling-hash-whole-file-hashing-is-opt-in-and-separate)),
so a destructive command carries one and there is nothing to argue about. The residual
judgement is whether a head/tail hash is enough for a *delete* guard or whether that one path
deserves the whole-file hash at 0.49 s per file — deletion is rare, so the conservative option
is affordable.

---

## D101 — A measurement that spans files is a core-supplied job; the node never chooses the set

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §24.

[D82](#d82--media-segments-are-typed-attach-at-the-version-and-carry-provenance) puts intro detection in the node, which is right — it decodes audio
and [D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp) keeps that out of the core. But the useful detector works by
finding **the segment repeated across the episodes of a season**, and that runs straight into
[principles §9](../principles/README.md)'s test:

> *Could this component determine it from what it observed, with no memory of previous
> observations and no knowledge of the library?*

**No.** "These files are episodes of one season" is library knowledge, produced by
[identification.md](../design/plans/identification.md) in the **core**, from parses the node is
forbidden to make ([D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret)). A node that groups files by itself is a node
that has re-implemented the filename parser, which is the exact thing D59 moved out.

**So the work splits along the line §9 already draws.** The core supplies the **set** and the
node supplies the **measurement**:

| Core | Node |
| --- | --- |
| "compare these twelve files" — an explicit list, from the library | "a 91-second segment repeats in all twelve, at these offsets" |
| decides that makes them intros, at which confidence, and stores them | reports a measurement over the bytes it was given |

That is a **job**, not a config command ([D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config), [D100](#d100--a-destructive-command-names-its-target-by-identity-not-by-path)) —
it carries a payload, it produces a result, and it is not desired state.

**The rule generalises past intros**, which is why it is a decision rather than a paragraph in
a plan: perceptual-hash duplicate detection, cross-version segment alignment
([D82](#d82--media-segments-are-typed-attach-at-the-version-and-carry-provenance)), re-encode detection and any future "compare these files to each
other" pass all have the same shape. **The node never chooses its own comparison set.** If it
did, every one of those features would need a private copy of the library's grouping logic, and
they would each be wrong in a different way.

**Status:** Unmeasured — the cost of shipping an explicit file list per job is trivial, but the
*scheduling* is not obvious: a season is the natural unit for intros and a library is the
natural unit for duplicate detection, and the second is O(n²) in a way the first is not.

---

## D102 — The node's work is a pipeline of independently deferrable passes

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §25.

[D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) said the node does "everything requiring access to a media file",
and at the time that was discovery, probing, subtitle and font extraction, thumbnails and
trickplay. It has since accumulated:

| Pass | From |
| --- | --- |
| enumerate (snapshot and journal) | [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret), [D60](#d60--snapshotting-and-discarding-are-different-operations-with-different-triggers) |
| probe | [D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) |
| content hash, where cheap | [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret) |
| subtitle extraction | [D11](#d11--subtitles-are-a-subsystem-not-a-feature) |
| font subsetting per subtitle track | [D55](#d55--the-served-font-set-is-the-cta-708-style-axis-in-variable-fonts-subset-per-subtitle) |
| thumbnails, trickplay, and now poster derivation | [D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do), [D99](#d99--the-core-does-not-decode-an-image-either-and-community-uploaded-artwork-is-why) |
| **loudness measurement** | [D83](#d83--loudness-is-measured-at-scan-and-applied-as-gain-downmix-has-one-default-and-a-dialogue-control-drc-is-per-device) |
| **segment detection** | [D82](#d82--media-segments-are-typed-attach-at-the-version-and-carry-provenance), [D101](#d101--a-measurement-that-spans-files-is-a-core-supplied-job-the-node-never-chooses-the-set) |
| subtitle overlay rendering | [D78](#d78--server-rendered-subtitle-overlays-are-a-third-rendition-and-they-dominate-burn-in) |
| *(later)* content-safety inspection | [D61](#d61--containment-now-detection-later-and-plugin-shaped-four-affordances-to-keep-open) |

That is nine passes over the same files, several of them full decodes, on hardware whose honest
low end is an 8 GB Celeron NAS ([test-hardware.md](../design/research/test-hardware.md)). Each
was added in isolation and each was individually cheap. **Nobody has looked at the total**, and
the two failure modes are predictable: a first scan that takes a week, and a library that never
spins down ([D94](#d94--disk-spin-down-is-a-cost-of-the-snapshot-interval-and-archival-means-untouched)).

**So the node's work is a pipeline of named passes, and the pipeline is a first-class object:**

- **Each pass is independently deferrable, per library.** [D94](#d94--disk-spin-down-is-a-cost-of-the-snapshot-interval-and-archival-means-untouched)'s
  `archival` is the extreme setting of a control that should exist at every gradation — a
  downloads folder wants everything immediately, a 2004 films folder wants enumeration and
  little else until something plays.
- **Each pass is separately reported.** [D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config)'s `task` is already a
  bounded enum (`enumerating`, `probing`, `hashing`, `extracting`) that the core baselines
  against, so this is that enum growing rather than a new mechanism — but the *user-facing*
  question changes from "is it scanning?" to "which passes have finished on this library?",
  and that is the one [sentiment.md](../design/research/sentiment.md)'s "I can't tell what the
  scanner did" complaint actually asks.
- **Each pass is restartable and idempotent**, because a NAS will be switched off mid-pass.
- **Ordering is by user value, not by convenience.** Enumeration and probe make the library
  exist; everything after that is enrichment, and a user who can watch a film in five minutes
  will forgive a trickplay sprite that arrives on Tuesday.

**What this is not:** a pipeline of *stages that draw conclusions*. That is the
"Navidrome-style scanner phases" shape [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret) removed, and it stays
removed — these are passes over bytes, all of them reporting observations, with every
interpretation still in the core.

**Status:** **Measured 2026-09-20**, and the shape is better than feared
([D114](#d114--a-pass-costs-what-it-costs-because-of-one-thing-whether-it-decodes-video)).
The nine passes are not nine costs: they are **two populations a thousand-fold apart**,
separated by whether a pass decodes video. The library is **browsable and playable after 17
minutes**; the entire cheap set is about a day and a half; **trickplay alone is 9.2 days** and
is the only unavoidable member of the expensive class. Fusing passes that share a decode is
worth **38% on six cores and 45% on one**. What remains unmeasured is the same list on real
NAS hardware, and the client side entirely.

---

## D103 — A device-scoped *preference* is stored; a device *capability* never is

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §26.

[D83](#d83--loudness-is-measured-at-scan-and-applied-as-gain-downmix-has-one-default-and-a-dialogue-control-drc-is-per-device) makes dynamic-range compression a per-device setting, because the
right value depends on the room. [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s preference table has scopes of
**user, library and series** — and no device. So there is nowhere to put it.

The obvious fix is dangerous, which is why this needs saying: adding "things the server knows
about a device" is how you get a device-profile database, and
[D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) calls that the incumbents' largest bug source.

**The line that keeps both true:**

| | Where it lives | Why |
| --- | --- | --- |
| **Capability** — what this device *can decode* | declared per request, **never stored** | it changes with an OS update, a codec pack, a cable; a stored copy is a stale copy, and a table of them is the thing [D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) exists to avoid |
| **Preference** — what this device's *owner wants here* | stored, keyed by device id | night mode on the living-room TV, subtitles always on in the bedroom; it does not change by itself and losing it annoys people |

A preference is a fact about a **person in a room**. A capability is a fact about **hardware**,
and hardware lies ([D92](#d92--a-failed-rendition-falls-to-the-next-one-and-the-client-remembers-the-server-still-has-no-device-database)). Conflating them is precisely the mistake; keeping
them apart makes a device scope safe.

**So `device` joins user, library and series as a scope** on the existing preference table, with
the same tri-valued unset/on/off discipline. The device id already exists — pairing issues one
([D93](#d93--device-pairing-is-rfc-8628-shaped)) and bug reports carry it ([D74](#d74--bug-reports-pseudonymise-rather-than-redact-and-fan-in-across-components)).

Two guards:

- **A device scope is never a default for another device.** No inheritance between devices, and
  no "apply to all my devices" that silently writes eight rows.
- **The client may still hold its own copy** ([D92](#d92--a-failed-rendition-falls-to-the-next-one-and-the-client-remembers-the-server-still-has-no-device-database)'s local memory), and
  where the two disagree the stored one wins for preferences and the local one wins for
  capability — which is the same split as the table above, applied to conflict resolution.

**Status:** Small. The open question is device *lifecycle*: a television replaced after four
years leaves a row, and a household that reinstalls an app may or may not get the same device
id. Neither is harmful, both are untidy, and the answer is probably a device list the user can
prune — which they will want anyway for session revocation ([auth.md](../design/plans/auth.md)).

---

## D104 — A Watch Together invite is a capability URL scoped to one edition; the group is never a grant

> **"Keyed on an Edition" is retired by
> [D122](#d122--the-session-is-the-object-what-plays-is-mutable-state-on-it-and-a-guest-sees-the-stream-rather-than-the-library)**
> (2026-09-21). The session is the object and what plays is mutable state on it; the bound on
> a leaked link is that a guest sees **the current stream, not the library**, which is a
> stronger guarantee than scoping to an item and does not depend on enumerating the scope.
>
> **The entitlement half is superseded by
> [D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays)**
> (2026-09-20). ~~*Every participant resolves the item through their own entitlement*~~ — the
> link **is** the grant, and a griefer who obtains one is handled by invalidating the session
> and re-forming it, not by requiring everyone to hold an account first. The *scope* half of
> this decision survives and matters more under D117, because it is what bounds the blast
> radius of a leaked link.

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §27.

[watch-together.md](../design/plans/watch-together.md) says *"anyone in a group may play, pause
and seek"* and never says **who may be in the group**. There is no mention of an invite, a
join authorisation, or an entitlement check anywhere in the plan or the research. Its
`Depends on` line names `stream-service`, `web-client` and `remote-access` — and not
[sharing-model.md](../design/plans/sharing-model.md).

**That fails open, and it fails open in the most natural implementation.** The whole point of
the feature is watching with someone who is not in your house, so the obvious build is "send
them a group link". A group link that admits anyone who holds it is an access path that routes
around [D76](#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed)'s share predicate and [D84](#d84--certifications-are-system-code-with-a-per-system-order-a-kids-profile-is-a-grantee-plus-a-predicate)'s maturity
ceiling — both of which [D76](#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed) requires to be enforced *at every access
path, not only at listing*, for exactly this reason.

**So the rule: the group is not a grant.** Every participant resolves the item through their
**own** entitlement. A user who cannot see the film cannot see the group —
[principles §10](../principles/README.md), omission rather than rejection: there is no join
button to decline.

**And the invite has a good answer already sitting in the tree.**
[remote-access.md](../design/research/remote-access.md) §6 lists four identity mechanisms, of
which the second is *"capability URLs — perfect for one film, unattributable for ongoing
access"*. A Watch Together invite is precisely that shape, and the weakness named there is not
a weakness here:

- **Scoped to one Edition**, not to a library. The invitee gets the film, not the collection.
- **Time-boxed** to the session, which is what makes "unattributable for ongoing access"
  irrelevant — there is no ongoing access.
- **Revocable by ending the group**, and listed while it is live so the owner can see who holds
  one.

So the feature that most needs an identity story turns out to need the **cheapest** of the four
([D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)'s unresolved hosted-directory question is not on its critical path),
and the motivating use case — a partner travelling, with no account — is answered without
one.

**Two consequences worth stating.**

- **`watch-together.md` depends on `sharing-model.md`**, which it did not declare. The
  capability-URL mechanism and the entitlement check are both that document's.
- **A grantee's participation is bounded by their share, not by the host's.** If the host can
  see the director's cut and the grantee's predicate only matches the theatrical, they are
  watching different Editions and the group is incoherent — so the group is keyed on an Edition
  and a participant who cannot resolve *that* Edition cannot join. Sharper than it sounds: it
  means the entitlement check happens against the group's item, once, rather than being
  re-derived per participant per action.

**Status:** The capability URL's own design is [threat-model.md](../design/plans/threat-model.md)'s
open signed-URL work, so this decision names the shape and inherits that document's gap. Also
open: whether a participant who loses entitlement mid-film (a share revoked, a rule
re-evaluated) is ejected or finishes the film. Ejecting mid-sentence is hostile; not ejecting
means a revocation is not immediate. Probably: finish the session, deny the next one, and say
so — but it is a decision, not an oversight to be discovered at implementation time.

---

## D105 — A protocol adapter acts as the user, never as itself

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §28.

[D15](#d15--two-plugin-tiers-in-process-wasm-and-out-of-process-adapters) gives out-of-process adapters *"its own auth, its own websockets
and its own route namespace"*, and [jellyfin-api-shim.md](../design/plans/jellyfin-api-shim.md)
inherits that without saying what the adapter authenticates **to the core** as. There is only
one wrong answer and it is the convenient one: a single privileged token, configured once at
install.

**That is a confused deputy, and it defeats every access control we have.**
[D76](#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed) requires the share predicate to be enforced *"at every access path,
not only at listing"*; [D84](#d84--certifications-are-system-code-with-a-per-system-order-a-kids-profile-is-a-grantee-plus-a-predicate) puts the maturity ceiling on the same
mechanism; [D104](#d104--a-watch-together-invite-is-a-capability-url-scoped-to-one-edition-the-group-is-never-a-grant) has just established that a group is not a grant. An
adapter holding the owner's token hands the whole library to anything that can speak Jellyfin's
API at it — and because the core sees a legitimate owner request, nothing fires.

**So:**

- **The adapter is a login proxy, not a principal.** A Jellyfin client's credentials are
  forwarded to the core, the core authenticates, and the adapter holds a token **scoped to that
  user** for the life of that session. It has no standing credential of its own beyond the
  right to attempt a login.
- **Every downstream request carries that user's token.** Entitlement is resolved by the core,
  once, in the place it is already resolved — so the adapter cannot get it wrong, because it
  never computes it.
- **The core knows the request came via an adapter**, and says so in the session record. That
  is needed for [D74](#d74--bug-reports-pseudonymise-rather-than-redact-and-fan-in-across-components)'s bug reports and
  [D91](#d91--a-metrics-endpoint-off-by-default-and-local-when-on)'s metrics, and it is needed for
  [principles §10](../principles/README.md): an adapter is a *cooperating* client for omission
  purposes but a **less trusted** one for alarm purposes, since its users are running software
  we did not write.
- **A redirect to a media URL carries the user's authorisation, not the adapter's.**
  [D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible) already signs the session rather than the segment; the session
  being redirected to must be the *end user's* session.

**The rule generalises to every tier-2 surface** — OpenSubsonic in Phase 7, DLNA if it ever
happens — which is why this is a decision rather than a line in one plan. It also survives the
adapter being abandoned, which D15 says it might be: an adapter with no standing credential
leaves nothing behind when it is deleted.

**One consequence worth naming for DLNA specifically:** DLNA has no user concept at all. An
adapter that cannot identify a user cannot act as one, so it can only ever be configured to act
as a **specific, explicitly chosen** user with whatever that user can see — never as the owner
by default, and never silently. That is a limitation to state in the adapter's own
documentation rather than a hole to leave open.

**Status:** Sound as a rule; the mechanism depends on [auth.md](../design/plans/auth.md)'s
token model, which is unsettled. The awkward case is a Jellyfin client that expects a
long-lived API key: the adapter must be able to hold a user-scoped long-lived token, which is
exactly the *"granular apikey permissions"* request open on Jellyfin's own tracker.

---

## D106 — The adapter synthesises the play-state sequence it cannot receive

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §29.

[D81](#d81--play-state-is-an-append-only-event-log-plus-a-guarded-position-and-positions-are-edition-relative) guards the resume position with a **monotonic sequence**, so a stale
device cannot rewind a newer one. Jellyfin's progress API has no such field, and neither will
OpenSubsonic's. So every report arriving through an adapter has no sequence — and the obvious
implementation, treating "no sequence" as "newest", reinstates last-writer-wins for exactly the
clients most likely to be running on a second device.

**The adapter mints the sequence**, per `(user, device)`, monotonically increasing for the life
of that device's session, and the core treats it like any other. That keeps the guard's
property — a given device's reports are ordered — without inventing an ordering between devices
that the wire format cannot express.

**And the honest limitation is stated rather than hidden:** two adapter-driven devices playing
the same edition are ordered only by arrival, because there is no information to do better
with. That is not a defect of this design; it is the ceiling of the protocol being adapted, and
it is one of the things [jellyfin-api-shim.md](../design/plans/jellyfin-api-shim.md) means by
*"the adapter is allowed to be lossy"*. Say it in the adapter's documentation, because the
symptom — a phone occasionally rewinding a television — is otherwise indistinguishable from the
bug [D81](#d81--play-state-is-an-append-only-event-log-plus-a-guarded-position-and-positions-are-edition-relative) exists to prevent.

**The same document should say how the levels flatten.** Jellyfin's model is
[D79](#d79--four-levels-not-three-title--edition--version--file-with-collections-orthogonal)'s conflated Edit/Manifestation/Item, so the mapping is forced and
should be written down once: an **Edition** is a Jellyfin item, a **Version** is one of its
"alternate versions", and a multi-part **File** set is `AdditionalParts`. That is Jellyfin's own
vocabulary being used for what it was reaching for, and it is lossless in the direction that
matters because we have more structure than it does, not less.

**Status:** Untested, like the whole adapter. The specific thing to check early is whether real
Jellyfin clients tolerate an item whose alternate versions differ in *runtime* — which ours can,
across Editions, and which is the first place a flattening assumption will break.

---

## D107 — The node channel is mutually authenticated, and a node's roots are local and never remotely settable

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §30.

[D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary) states the security model for the **file** transport exactly and
well: *"the permissions split **is** the security model"*, with a table that doubles as the
node's Landlock ruleset. It then says *"where HTTP exists, prefer it"* — and **says nothing
about what authenticates either end of the HTTP channel.** The preferred transport has no
stated security model at all.

The two directions fail differently and the second is worse:

- **A spoofed node** injects observations: files that do not exist, a root that has "lost" forty
  thousand entries, error records that trip [D41](#d41--diagnose-the-failure-boundary-by-walking-up-and-guard-the-silent-case-which-is-worse)'s shrinkage guard.
  Unpleasant, and bounded by the fact that [D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides) makes the core the only
  thing that decides.
- **A spoofed core** writes `config.json`. That channel carries commands, and since
  [D89](#d89--deletion-is-opt-in-per-library-two-phase-and-names-every-file) it carries **deletion**. It is the more dangerous direction, and
  it is the one nobody considered, because the whole node discussion frames the *node* as the
  less-trusted half.

**So, two rules.**

**1. Mutual authentication, with enrolment as an explicit act.** Over HTTP the node and core
authenticate each other — a secret established at enrolment, or mTLS. Enrolment is a deliberate
human step, not a discovery protocol: a node that can be adopted by whatever answers first is a
node that can be adopted by an attacker on the LAN. This is the same shape as
[D93](#d93--device-pairing-is-rfc-8628-shaped)'s device pairing and should reuse it rather than invent a second
enrolment mechanism.

**2. A node's roots are local configuration, and the core cannot add one.** The core may say
*"rescan root 2"*; it may never say *"also scan `/etc`"*. The command vocabulary already
references roots by identifier rather than by path
([media-nodes.md](../design/media-nodes.md)), which is the right shape — this makes it a rule
rather than an accident.

The reasoning is the one already in the tree, applied one level up: **the directory table is
the node's Landlock ruleset**, and a Landlock ruleset is declared at startup and cannot be
widened at runtime. The node's scope should have the same property for the same reason. The
core commands *within* a scope the node's operator declared locally; it never expands it.

Without this, a spoofed core — or simply a compromised core — turns every node into an
arbitrary filesystem enumerator reporting back over the network, on hardware whose whole
purpose is to have access to the user's files.

**Three consequences.**

- **Defence in depth for deletion.** [D100](#d100--a-destructive-command-names-its-target-by-identity-not-by-path) already makes a destructive
  command name its target by identity; local roots bound *where* it can name anything at all.
  Neither alone is sufficient and together they are unexciting, which is the goal.
- **The degraded modes in [D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary) stay correct.** A node whose core cannot
  reach it runs on its own configuration — which is now the only place its scope was ever
  defined, so nothing is lost.
- **Adding a library is a two-sided act**, and the UI has to say so: the operator declares the
  root on the machine that holds the files, and the core adopts it. That is slightly more
  friction than "type a path into the web UI", and it is the friction that makes the boundary
  real. For the default single-machine install, where the core supervises a co-located node,
  both sides are the same machine and the user sees one step.

**Status:** The enrolment mechanism is shared with [auth.md](../design/plans/auth.md) and
inherits its open token model. Unresolved: whether a node should also be able to *refuse* a
root the core knows about — the asymmetry is intentional but "I can see this root and choose
not to serve it" may be a real operator want.

---

## D108 — The interface is not ours to change out from under people, and an old client stays installable

**Decided** (2026-09-19). Evidence: [sentiment.md](../design/research/sentiment.md) §Plex's own
forum, read first-hand on 2026-09-19 —
[audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §31.

**The loudest complaint on Plex's own forum in the last year is not the price.** Sorted by
likes, *Vote to roll back to Plex Classic!* (**1,124 likes**, 304 posts, 2025-09-26) outscores
*Elephant in the room. Lifetime Plex Pass* (937 likes). The same theme recurs under at least
eight more titles, and Plex partially reverted in August 2026 after, in their own words,
*"consistent feedback from the start"* over roughly a year. The forum's verdict on the revert:
*"too little too late."*

Our own [sentiment.md](../design/research/sentiment.md) characterised Plex's problem as
"commercial, not technical", drawn from Hacker News where the price rise dominates. First-hand
data from Plex's users says otherwise, and it names something we can simply promise.

### Two commitments, both cheap because we are small

**1. A layout change is opt-in, and the previous one stays available for at least one release
cycle.** Not "we will design carefully" — a mechanism. This is the same discipline
[D85](#d85--the-api-has-a-compatibility-contract-and-it-covers-three-protocols) applies to the API: additive by default, removal behind a window,
and the switch that undoes it shipped alongside. An interface is an API whose consumer is a
person, and the person has less recourse, because they cannot pin a version.

**2. An old client stays installable.** This is the second theme in the same forum and it is
not recorded anywhere in our tree:

> *"I just reverted to the old version and turned off updates."*
> *"Please tell me how to revert to the previous Plex on Fire TV!"*
> *"videos that played fine before are unwatchable! … Now I have to try to figure a way to
> revert!"*

Enough people needed it that a pinned *Workaround: Roll back the Fire TV Plex app to version
10.30.9.4262* thread exists, and a separate thread organises users to petition **Vizio**,
because the TV vendor is the only party who can un-ship an update.

**An app store is a single point of failure for a client, and we should not pretend otherwise.**
So: **every client release is published as a directly installable artefact — a versioned APK,
a signed build — and old versions stay up.** For Android and Android TV that is free and
[tv-client.md](../design/plans/tv-client.md) already leans on sideloading as a *distribution*
advantage; this makes it a **rollback** advantage too, which is the more valuable one. Where
a platform forbids it (Apple), say so plainly rather than discovering it during an incident.

This composes with [D85](#d85--the-api-has-a-compatibility-contract-and-it-covers-three-protocols) and is the reason that decision matters: a
compatibility contract lets an old client keep working, and a published artefact lets a user
*get back to* an old client. Either alone is half the answer.

### What this does not commit us to

Not "never change the UI" — that is a promise nobody keeps and it would be dishonest to make.
It commits us to **never removing the way something worked without leaving the old way
reachable for a release**, and to never being in a position where a user's only recourse is to
petition their television manufacturer.

**Rejected:** a settings-level "classic mode" maintained forever (two UIs is two products, and
the field's evidence is that the second one rots); relying on app-store version pinning (it
does not exist for consumers on any TV platform); treating this as a UX-team concern rather
than a release-engineering one (it is the second that fails).

**Status:** The one-release window is a guess and should be set against a real release cadence.
The harder unresolved question is what "the previous layout stays available" means for a
**web** client, where there is no installed artefact and the server serves the UI — plausibly
the server keeps the previous bundle and a per-user setting selects it, which is cheap in
storage and not free in testing.

---

## D109 — Five corpus tiers, and nobody has to download media to run the suite

**Decided** (2026-09-20). Research:
[testing-strategy.md](../design/research/testing-strategy.md) §1, §6, §8.

The plan asked *"how does a contributor without 200 GB of media run the suite?"* **The premise
was wrong.** Nobody's corpus is 200 GB, and the two ends of the field are three orders of
magnitude below that guess:

| Project | Real media in its suite |
| --- | --- |
| **Jellyfin** — a media server | **none at all**; 3.9 MB of tests, 285 C# files, zero media files |
| **Navidrome** — a Go media server, our closest analogue | **13 files, 512 KB**, in-tree |
| **hls.js** — a player | none; **49 stream URLs** |
| **FFmpeg** — the codec library underneath everything | **2,875 files, 1.29 GiB**, external and opt-in |

And FFmpeg's, measured by `rsync -n --stats` on 2026-09-20, has a **median file of 58.7 KB**
with **86% under 1 MB**. A media test corpus is thousands of tiny clips, not a library.

### The tiers

- **Tier 0 — no fixtures.** Pure functions, table-driven: filename parsing, track-kind
  derivation, language normalisation, the predicate compiler, sort keys, pseudonymisation.
  Jellyfin's largest test project is exactly this — `Jellyfin.Naming.Tests`, 3,938 lines, with
  **79 `[InlineData]` cases in one file**.
- **Tier 1 — recorded JSON, and this is the main event.** Jellyfin tests its media layer with
  **fifteen recorded `ffprobe` outputs totalling 120 KB** and no media at all.
  **[D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret)
  makes that structural for us rather than incidental**: probe output is our *wire format*, so
  the core's entire interpretation layer — parsing, grouping, track kinds, segment placement,
  identification — replays from recorded node reports with no FFmpeg, no filesystem and no
  media. The thing D59 was for, cashed out.
- **Tier 2 — generated, by the test run.** Measured: 16 clips across eight codec/container
  combinations at two resolutions cost **8.9 s and 5.9 MB**. They are not committed — the
  generator is, because the generator is the specification of what each clip is.
- **Tier 3 — a small real set, in-tree, chosen for pathology.** Navidrome's 512 KB is the
  model and its filenames are the design: `._02 Invisible.mp3` (an AppleDouble sidecar a
  scanner must skip), `test-invalid-frame.mp3`, and `no_replaygain.mp3` beside
  `zero_replaygain.mp3` — *absent* versus *zero*, which is
  [D83](#d83--loudness-is-measured-at-scan-and-applied-as-gain-downmix-has-one-default-and-a-dialogue-control-drc-is-per-device)'s
  problem a decade early. Ours is the video equivalent: an AppleDouble file, a truncated
  container, a `.mkv` that is not Matroska, mismatched `ger`/`deu` codes, a file with no
  runtime, an ASS with an embedded font, a 5.1 track with no loudness metadata.
- **Tier 4 — external, opt-in, and announced.** FATE's mechanism exactly:
  `make fate-rsync SAMPLES=…`, and without it the build prints *"warning: only a subset of the
  fate tests will be run"* and runs the generated subset. **The degraded mode is announced, not
  silent** — which is the part to copy, more than the rsync.
- **Tier 5 — hardware.** [device-lab.md](../design/plans/device-lab.md) already owns it.

**Tiers 0–3 fit in the repository and total a few megabytes**, so the default `go test ./...`
needs no download, no FFmpeg and no media. That is the answer, and it is better than the
question assumed.

**Status:** The tier-3 list is a starting set and will grow the way Navidrome's did — one file
per bug, named after the bug. The thing to watch is tier 3 growing into tier 4 by accident;
the rule is that a file earns its place by reproducing a failure, not by covering a codec.

---

## D110 — Every threshold gate compares against a frozen baseline, and the assertion is chosen per test

**Decided** (2026-09-20). Research:
[testing-strategy.md](../design/research/testing-strategy.md) §2, §3.

Two rules, both stolen, both about the same failure: a gate that passes every time while the
thing it guards gets worse.

**1. Compare against a frozen baseline, never the previous run.** From VMAF practice, where the
just-noticeable difference is about **6 points** and a delta under ~2 is noise. A sequence of
96.0 → 95.6 → 95.2 → 94.8 → 94.4 → 94.0 passes every build-to-build tolerance and has lost two
points in six builds. Any threshold compared against "last time" ratchets; one compared against
a pinned reference does not.

**This is not a video rule.** It applies identically to
[performance-targets.md](../design/plans/performance-targets.md)'s latency gates, to database
size, to scan throughput, and to anything else with a tolerance. It is the single most
transferable thing in the testing research.

**2. The assertion is chosen per test from a closed set, and the loosest is never the default.**
FATE's set, from `tests/fate-run.sh`: `diff` and `rawdiff` for text; `framecrc`, `framemd5`,
`md5` where output *is* bit-exact; `oneoff` for "absolute difference ≤ threshold"; `stddev` for
audio; `tiny_psnr`, `tiny_ssim`, `audiomatch` as purpose-built comparators; and
`refcmp-metadata.awk` for float metadata within a fuzz (default 0.1). Plus `CMP_UNIT = 1` as a
*per-test* tolerance rather than a global one.

The lesson is not "use PSNR". It is that reaching for a fuzzy comparison is a decision recorded
in the test, and most tests do not need one — decoded output is bit-exact and should be
asserted that way.

**And quality gates run on seconds of video, not minutes.** Measured: VMAF on a 10-minute 1080p
file with 6 threads takes **344 s — 1.7× realtime**, the most expensive operation measured
anywhere in this project. Which agrees with the corpus finding from the other direction:
everyone's samples are tiny because long ones are unaffordable.

**Status:** Sound. What is unsettled is where the frozen baselines live and how one is
deliberately re-based — that needs a release process to hang off, and it must be an explicit,
reviewed act rather than a `--update-golden` flag somebody runs to make CI green.

---

## D111 — `assumptions/` is a test directory

**Decided** (2026-09-20). Research:
[testing-strategy.md](../design/research/testing-strategy.md) §5.

Stolen from shaka-player, which has a `test/assumptions/` directory whose only job is to pin
the platform behaviour the code rests on — their one file asserts how `Uint8Array` equality
works. It looks trivial. It is not: when a platform or library changes an assumption underneath
you, this fails **there, with a name**, instead of surfacing three layers up as a mysterious
behaviour change in a feature test.

This project has an unusual number of load-bearing assumptions, and today every one of them is
a sentence in a document:

| Assumption | From |
| --- | --- |
| Go's `\w`, `\s`, `\b`, `[[:alpha:]]` are ASCII-only | [D49](#d49--unicode-discipline-at-the-boundaries-enforced-by-tests) |
| `x/text/collate` is pinned at Unicode 6.2.0 / CLDR 23 | [D47](#d47--sort-order-is-a-materialised-cldr-sort-key-per-item-per-locale) |
| an FTS5 tokeniser is immutable once the virtual table exists | [D48](#d48--two-fts5-tokenisers-routed-by-script-and-length-and-exactly-one-normaliser) |
| SQLite's OR chain fails at 1000 terms; its join at 65 tables | [D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable) |
| `ass_track_set_feature` returns −1 when a feature is unavailable | [D50](#d50--build-libass-correctly-and-turn-off-vsfilter-bug-compatibility-for-non-authored-subtitles) |
| FFmpeg's AC-3 decoder applies DRC by default at `drc_scale = 1` | [D83](#d83--loudness-is-measured-at-scan-and-applied-as-gain-downmix-has-one-default-and-a-dialogue-control-drc-is-per-device) |
| chromaprint refuses a 5.1 input and needs a stereo downmix first | measured, 2026-09-20 |
| NVENC ignores `-force_key_frames` and needs `-force_idr 1` | [transcoding.md](../design/plans/transcoding.md) |
| `SQLITE_BUSY` and `SQLITE_BUSY_SNAPSHOT` share a message and only one is retryable | [sqlite-reconsidered.md](../design/research/sqlite-reconsidered.md) |
| copying a live WAL database's `.db` file yields a file with no tables | [D77](#d77--back-up-with-vacuum-into-back-up-only-the-irreplaceable-and-test-the-restore) |

Each is one small test. Together they are the difference between "a dependency upgrade changed
something" and "a dependency upgrade broke subtitle line-breaking in Thai and we found out
from a user".

**Status:** Cheap and obviously right; the risk is the directory becoming a dumping ground. The
entry criterion: an assumption belongs here if it is about **something we do not control** and
its violation would be **silent**.

---

## D112 — The node probes its confinement and reports it; the escape test asserts what is in force

**Decided** (2026-09-20). Research:
[testing-strategy.md](../design/research/testing-strategy.md) §7.

[D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp) confines FFmpeg
with Landlock + seccomp, and the plan asked for "an escape test, because a sandbox nobody
verified is a comment". Measured in this environment — an ordinary Linux 6.17 container:

```
landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION)  →  -1, errno 95 ENOTSUP
/proc/self/status:  Seccomp: 2   Seccomp_filters: 2
```

**Landlock is simply unavailable**, and from inside it is impossible to tell whether that is a
kernel built without it, a boot parameter, or the container runtime's own seccomp profile
denying the syscall. Docker is the most common way this software will be deployed, and Docker
is exactly where this happens.

So the requirement is larger than a test:

- **The node probes its confinement at startup and reports the result**
  ([D73](#d73--each-component-reports-according-to-its-own-visibility-blind-is-not-opaque)) —
  which mechanisms it asked for, which it got, and which it did not.
- **The core can say "this node is running FFmpeg unconfined" out loud**, because a user whose
  threat model depends on [D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)
  deserves to know it is not in force. Silence here is the failure mode.
- **The escape test asserts what is actually in force**, not that the right function was
  called: attempt the forbidden open and the forbidden syscall, and assert the denial —
  skipping loudly, not silently, where the mechanism is absent.
- **Absent confinement is a degraded mode, not an error.** Refusing to run in Docker would be
  worse than running and saying so.

**Status:** This sharpens
[threat-model.md](../design/plans/threat-model.md)'s open per-platform confinement matrix with
one concrete row — a stock container — and the row says *no Landlock*. The seccomp half was
present here (`Seccomp: 2`), so the two mechanisms fail independently and must be reported
independently.

---

## D113 — A target is a percentile at a stated concurrency, at two library sizes, against a frozen baseline

**Decided** (2026-09-20). Research:
[performance-targets.md](../design/research/performance-targets.md) §1, §2, §6.

The plan said this "cannot be finished without hardware". Half right: the **client** numbers
cannot, and the **server** ones can, because the server's workload is a database and a decode
loop. Both were measured.

### The shape, which matters more than the values

1. **A percentile at a stated concurrency.** Measured on the 100k-title database: the read p50
   moves from 0.234 ms to 0.766 ms as readers go 1 → 20, while the **p99 moves from 2.81 ms to
   101.70 ms** — a 36× tail against a 3× median. Every value in that table would satisfy a
   target stated as an average.
2. **Two library sizes**, because every failure found was a *scaling* failure invisible at one
   size.
3. **Against a frozen baseline** ([D110](#d110--every-threshold-gate-compares-against-a-frozen-baseline-and-the-assertion-is-chosen-per-test)).
4. **As a multiple of a measured baseline** where the absolute value is hardware-dependent, so
   the same gate means something on CI and on a NAS.
5. **Observable in production, not only in CI**
   ([D91](#d91--a-metrics-endpoint-off-by-default-and-local-when-on)). A gate that only runs in
   CI cannot notice the library growing, which is the failure this whole document exists for.

### What the measurements found

**Ten of twelve queries are flat from 10,000 to 100,000 titles** — keyset pagination on
materialised sort keys, the detail page across all four levels, cast lists, trigram search, and
both cross-level rules-engine predicates with their `ANY`/`ALL` quantifiers. 10× the data, no
measurable change. [D47](#d47--sort-order-is-a-materialised-cldr-sort-key-per-item-per-locale)
and [D79](#d79--four-levels-not-three-title--edition--version--file-with-collections-orthogonal)
hold under measurement.

**The two that scale linearly are both on the home page**, and each cost one line to fix:

| | 100k, before | after | ratio |
| --- | --- | --- | --- |
| Recently added (50), no index on `added_at` | 21.79 ms | **0.027 ms** | **807×** |
| FTS match + `ORDER BY bm25()`, common term | 11.66 ms | **0.017 ms** (unranked) | **686×** |

And the second one generalises into a rule worth knowing: **bm25 ranking is linear in the
number of matches, not the number of results.** The term matched 6,589 of 100,000 rows and
SQLite scored all of them to return twenty. Search latency tracks match count, and the target
must fail when a common term makes it linear.

Both are invisible on a developer's thousand-item library and fatal at a hundred thousand.
That is Jellyfin 10.11's 30–50 second home page reproduced in miniature, for the price of an
index.

### Two more numbers worth pinning

**The single-writer ceiling is 9,415 commits/s** (WAL, `synchronous=normal`, unpaced), against
the ~2 writes/s [sqlite-reconsidered.md](../design/research/sqlite-reconsidered.md) §2 budgets
— **4,700× headroom**, so [D31](#d31--sqlite-with-a-pure-go-driver-supersedes-d5) is confirmed
with a figure rather than an argument.

**Read throughput saturates at ~3,000 queries/s and does not improve with more readers** (2,878
at one reader, 3,022 at twenty), and **readers starve the writer** (483 → 135 writes/s). The
pool size is a floor, not a throttle — Navidrome's comment, extended: past a small number,
extra connections buy no read parallelism either.

**Status:** The absolute values are from an i3-1315U with a warm cache and are conservative in
one respect (the pure-Go driver is the slower one) and optimistic in another (62 GB of RAM, no
memory pressure). The numbers to set gates from are the **ratios and the scaling behaviour**,
not the milliseconds. Time to first frame — the most important user-facing number — is still
unmeasured and needs a running server.

---

## D114 — A pass costs what it costs because of one thing: whether it decodes video

**Decided** (2026-09-20). Research:
[performance-targets.md](../design/research/performance-targets.md) §3. Refines
[D102](#d102--the-nodes-work-is-a-pipeline-of-independently-deferrable-passes).

D102 said the node now runs nine passes and nobody had added them up. Adding them up, on a
10-minute 1080p H.264 + AC-3 file:

| | ×realtime |
| --- | --- |
| probe · remux · keyframe grid · thumbnail | 2,000 – 38,000× |
| chromaprint fingerprint (after stereo downmix) | 802× |
| subtitle overlay at event granularity | 515× |
| loudness (`ebur128`) | 102× |
| **trickplay** | **14.3×** (one core) |
| **blackdetect** | **13.5×** (one core) |
| full video decode, for reference | 13.7× (one core) |

**There are two populations and nothing between them.** A pass that decodes every video frame
costs about one full decode, because it *is* one. A pass that does not costs between
microseconds and a second. **The gap is a factor of a thousand, and which side a pass falls on
is the only thing about it that matters for scheduling.**

For the reference household library — 3,500 files, 3,167 hours — single-core:

| | |
| --- | --- |
| probe + keyframe grid + thumbnail | **17 minutes** |
| fingerprint + overlays + loudness | ≈ 36 hours |
| trickplay | **9.2 days** |
| blackdetect | 9.8 days |

So **the library is browsable and playable after seventeen minutes**, and the entire cheap set
is a day and a half. D102's "ordering is by user value" is now quantified: it is the difference
between usable this afternoon and usable next week.

**Three consequences.**

- **Trickplay is the deferrable pass by default.** It is the only unavoidable member of the
  expensive class, it is ~60% of the total, and its absence costs a scrub preview rather than a
  feature.
- **"Does it decode video?" is the question to ask of any tenth pass**, and the answer is worth
  a factor of a thousand. Adding one to the expensive class is a two-week decision on a real
  library, not a feature flag.
- **Passes that do share a decode should fuse.** Measured: running loudness, trickplay and
  blackdetect as one invocation with a `split` filter beats three separate runs by **38% on six
  cores and 45% on one** — the saving is larger on the machine that needs it more, because each
  separate run re-pays the decode. The scheduler must know which passes share a decode.

**Two corrections to earlier decisions fall out**, both in the reassuring direction:

- **[D83](#d83--loudness-is-measured-at-scan-and-applied-as-gain-downmix-has-one-default-and-a-dialogue-control-drc-is-per-device) worried loudness was "not free on a Celeron".** It is ~1% of
  the file's runtime and barely faster on six cores than one — audio-only, one core's work,
  parallel across files. 31 hours for the whole reference library.
- **[D78](#d78--server-rendered-subtitle-overlays-are-a-third-rendition-and-they-dominate-burn-in)
  listed live-render cost as unmeasured.** At event granularity — which is what D78 specifies —
  it is **1.16 s per film-equivalent**, against 18.15 s if implemented by compositing over
  decoded video. The overlay rendition is free; the naive implementation is fifteen times
  worse, so the plan should say which one it means.

**Status:** Measured on one file on one machine. The shape of the finding (two populations, a
thousand-fold gap) will hold anywhere; the absolute rates will not.

---

## D115 — Intro detection is fingerprint-first, and chromaprint needs a stereo downmix or it fails outright

**Decided** (2026-09-20). Research:
[performance-targets.md](../design/research/performance-targets.md) §3. Amends
[D82](#d82--media-segments-are-typed-attach-at-the-version-and-carry-provenance).

D82 ordered the detectors chapter-derived → audio fingerprint → black-frame and silence, which
is the natural order if you assume fingerprinting is the heavy one. Measured, it is not:

| | ×realtime |
| --- | --- |
| chromaprint fingerprint (audio, after `-ac 2`) | **802×** |
| blackdetect (video) | **13.5×** (one core) |

**Sixty times cheaper**, for the reason [D114](#d114--a-pass-costs-what-it-costs-because-of-one-thing-whether-it-decodes-video)
gives: one is an audio pass and the other decodes every frame. Over the reference library that
is **4 hours against 9.8 days**.

So the order is **chapter-derived → audio fingerprint → black-frame**, with black-frame
detection a **second-line pass for the cases fingerprinting misses** — a film with no repeated
theme, or a season of one — rather than a default. They do different jobs; the cheap one just
happens to do the common one.

**And a trap that would have been found in production.** Run against the 5.1 track directly,
chromaprint fails outright:

```
[af#0:0] Task finished with error: Invalid argument
[out#0/chromaprint] Nothing was written into output file
```

It needs `-ac 2` in front of it. With the downmix it produces a 19,300-byte fingerprint for ten
minutes of audio in 0.75 s. This is exactly the class of thing
[D111](#d111--assumptions-is-a-test-directory) exists for, and it is now on that list.

**Status:** The *accuracy* of fingerprint-based intro detection is unmeasured here — only its
cost. Intro Skipper's 2,780★ is the evidence that the approach works; whether our
implementation matches it is a corpus question, and the corpus is a season of real television
that [D109](#d109--five-corpus-tiers-and-nobody-has-to-download-media-to-run-the-suite) puts in
tier 4.

---

## D116 — The content hash is a sampling hash; whole-file hashing is opt-in and separate

**Decided** (2026-09-20). Research:
[performance-targets.md](../design/research/performance-targets.md) §3.

[D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret) has
the node report "size, mtime, and where cheap, a content hash", and
[D100](#d100--a-destructive-command-names-its-target-by-identity-not-by-path) wondered whether
hash-on-delete was affordable. Measured, on a 425 MB file:

| | rate | time | over the 8 TB reference library |
| --- | --- | --- | --- |
| SHA-256, whole file | 860 MB/s | 0.49 s | 2.6 h of CPU — and **8 TB of reads** |
| CRC-32, whole file | 2,928 MB/s | 0.15 s | 0.8 h of CPU — and 8 TB of reads |
| **1 MB head + 1 MB tail + size** | — | **0.0032 s** | **11 s, and 7 GB of reads** |

**The CPU cost was never the problem; the reads are.** Whole-file hashing a library means
reading all of it — roughly **15 hours of pure I/O at a spinning disk's 150 MB/s, with every
drive awake for all of it**, which is [D94](#d94--disk-spin-down-is-a-cost-of-the-snapshot-interval-and-archival-means-untouched)'s
problem in its sharpest form.

So: **the default content hash is a sampling hash** — head, tail and size — at **153× less CPU
and a thousand times fewer bytes**. It answers the question the node actually asks, which is
*is this the same file I saw before*, not *is any other file identical to this one*.

**Whole-file hashing stays, as a separate, opt-in, explicitly-scheduled pass** for the one
thing that needs it: duplicate and re-encode detection
([identification.md](../design/plans/identification.md)). That is a library-wide operation the
user asks for, not something a scanner does on every pass, and it belongs in
[D102](#d102--the-nodes-work-is-a-pipeline-of-independently-deferrable-passes)'s deferrable
set with a warning about what it will cost.

**This also settles [D100](#d100--a-destructive-command-names-its-target-by-identity-not-by-path)'s
open question.** It asked whether size-and-mtime was enough identity for a delete guard and
whether "hash-on-delete is cheap relative to the consequence". At **3.2 ms** it is not merely
cheap, it is free — so a destructive command carries a sampling hash and there is no reason to
argue about it.

**Status:** A head/tail/size hash has a known blind spot — a file edited only in the middle
with its size preserved. For change detection that is acceptable (the mtime moves, and the
periodic snapshot is the net); for a **delete guard** it is the difference between "very
probably the same file" and "certainly". Whether that matters is a judgement, and the
conservative option — whole-file hash on the delete path only, at 0.49 s per file — is
affordable precisely because deletion is rare.

---

## D117 — The session link is the grant, revocation is the control, and the creator owns what plays

**Decided** (2026-09-20), by the project owner, and it **supersedes the entitlement half of
[D104](#d104--a-watch-together-invite-is-a-capability-url-scoped-to-one-edition-the-group-is-never-a-grant)**.

D104 argued that a group link routes around the share predicate, so every participant should
have to hold entitlement independently. The counter-argument is simpler and it wins: **a
griefer who gets hold of a link is a revocation problem, not an authorisation problem.** The
creator invalidates the session and makes a new one. Requiring the other person to have an
account first is exactly the friction this feature exists to avoid — the motivating case is a
partner travelling, who does not have one.

### What changes

**The link is the grant.** Holding a valid session link admits you. No per-participant
entitlement resolution, no account required, no share predicate evaluated per person.

**Revocation is the control**, and that puts three requirements on it that "just make the link
revocable" does not:

1. **Invalidation is immediate and ejects live participants.** A revocation that only stops
   *new* joins leaves the griefer in the room, which defeats the whole mechanism.
2. ~~**"Invalidate and re-form" is one action, and it carries the remaining participants
   across.**~~ **Superseded by
   [D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak)**,
   which removes the need for it: the session is never invalidated at all. The owner **rotates
   the join link**, the old one stops admitting and keeps serving everyone already in, and
   nothing is re-formed. That works because a link admits and a **membership** persists — a
   distinction this decision conflated.
3. **The roster is visible to the creator, live.** You cannot decide to revoke if you cannot see
   that there are four people in a two-person session.

### What survives from D104

**The scope** — intent right, mechanism wrong, restated by
[D122](#d122--the-session-is-the-object-what-plays-is-mutable-state-on-it-and-a-guest-sees-the-stream-rather-than-the-library).
A leaked link must expose *that session's content, not the library*, and that is what keeps
the blast radius small enough for revocation to be a sensible control. But what delivers it is
**not** scoping the link to what the creator chose — the host may change what plays at any
time — it is that a guest is handed **the current playback and no browse surface at all**.
Time-boxed to the session, as before.

### Who controls what

Three separate permissions, because they are three separate risks:

| | Who | Default |
| --- | --- | --- |
| **What plays** — the film, or the series | **the creator only, always** | not configurable |
| **Transport** — play, pause, seek | creator's choice: **everyone**, or **creator only** | everyone |
| **Move within the series** — next episode, another film in the series | creator's choice: on or off | follows transport |

The middle row was [watch-together.md](../design/plans/watch-together.md)'s opening (2) —
*"control is a per-participant capability, defaulting to everyone"* — and it is now a
user-facing setting rather than an affordance held open.

~~The third row is new and is the interesting one…~~ **Flattened by
[D121](#d121--an-ability-is-a-row-in-a-closed-table-with-a-two-valued-grant-chosen-at-session-creation)**:
each ability is simply `group` or `host only`, chosen at session creation. In-series
navigation is not a special case with its own scope object — it becomes the **play set** row,
arriving later with the play-set feature.

**Play sets are the later version** — the host selects a set of videos (a season, a trilogy)
and grants the group the ability to move within it. Cheap later because the scope is an
object and the permission is a row.

**Status:** The ejection path is the part most likely to be got wrong, because it is the path
nobody exercises until they need it. It deserves a test:
[testing-strategy.md](../design/plans/testing-strategy.md) gains *"a revoked participant is
gone from the session within one reconciliation interval, and the remaining participants do
not notice"*.

---

## D118 — The scan is incremental, prioritised, and steerable, and playability comes first

**Decided** (2026-09-20), by the project owner, and it turns
[D114](#d114--a-pass-costs-what-it-costs-because-of-one-thing-whether-it-decodes-video)'s
measurement into a requirement.

Three things, which are one thing: **a scan is not a batch job that finishes, it is a stream
of results the user can steer.**

### 1. Playability first, by definition rather than by luck

D114 measured that the passes split a thousand-fold by whether they decode video, and that the
cheap set reaches **a browsable, playable library in 17 minutes** on a 3,500-item reference
library where the full treatment is days. That is now the **ordering rule**, not an
observation: passes are ordered so that *an item becomes playable as early as possible*, and
everything that only adds a feature to an item that already plays comes after everything that
makes an item play at all.

For one 110-minute film, single-core, from the measured rates:

| | |
| --- | --- |
| probe + keyframe grid + thumbnail → **playable** | **< 1 s** |
| subtitle extraction, font subset, overlay rendition | ~2 s |
| loudness | ~65 s |
| trickplay | **~8 minutes** |

**An item is playable in about a second and fully enriched in about eight minutes**, and the
ordering must make the first number true rather than hiding it inside the last.

### 2. A user can jump the queue for one title

*"I want to watch this one now"*, in the middle of a scan of fifty thousand files. The pass
scheduler is therefore **a priority queue keyed by (item, pass)**, not a pipeline, and a
priority boost preempts at the next pass boundary rather than at the end of the scan.

Two consequences worth stating, because they are where this gets built wrong:

- **The core expresses the request in the node's vocabulary.** A node has no items
  ([D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides)); it has paths. So
  the core translates "this title" into a set of paths and sends those —
  [D101](#d101--a-measurement-that-spans-files-is-a-core-supplied-job-the-node-never-chooses-the-set)'s
  rule again: the core supplies the set, the node does the work.
- **It joins [D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config)'s
  command vocabulary** as `prioritise <paths>`, and it satisfies that decision's rule that
  every command tolerate re-execution: prioritising something already done is a no-op.

And the UI must be honest about what it can deliver quickly: *playable now, scrub previews in
eight minutes* is a true and useful thing to say. *Preparing…* is not.

### 3. Results appear as they are produced

**This changes a rule.** [D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)
says the node writes `report-<epoch>.jsonl.partial`, atomically renames on success, and **"the
core never parses a file still being written"**. The user requirement is that scan results
appear as soon as they exist, so that rule becomes:

> **The core may *tail* a `.partial` report, and everything it reads from one is incomplete by
> definition.**

Nothing about safety changes, because the safety rule was never the filename — it was
*"unknown means incomplete means add-or-update only"*, which already covers a partial read
exactly as it covered a crashed scan. The rename keeps its job: it is the moment **removals may
be applied**, and nothing else.

Two mechanical details, both small and both the sort of thing that bites:

- **Consume only up to the last newline.** A tailed JSONL file ends mid-record about half the
  time. This is the single reason the format was chosen — *"degrades per line"*
  ([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)) —
  so it costs a `LastIndexByte`.
- **Ingest is batched, not per line.** Measured: parsing a 50,000-entry snapshot costs 1.8 s
  and the write ceiling is 9,415 commits/s
  ([D113](#d113--a-target-is-a-percentile-at-a-stated-concurrency-at-two-library-sizes-against-a-frozen-baseline)),
  so tailing must not turn one scan into fifty thousand transactions. Batch on a time or count
  bound, and the user still sees results within a second.

**And this is what makes the scan legible**, which was the original complaint — *"I can't tell
what the scanner did"*. A progress bar is a promise about the future; **items appearing in the
library is evidence about the present.**

**Status:** The preemption granularity is a guess. Preempting at a pass boundary is simple and
correct; preempting *within* trickplay — the eight-minute pass — needs the pass to be
chunk-resumable, which [D102](#d102--the-nodes-work-is-a-pipeline-of-independently-deferrable-passes)
already requires for a different reason (a NAS gets switched off mid-pass), so it is probably
free. Unverified.

---

## D119 — A join link admits; membership is what persists. Rotation is seamless, ejection is owner-only, and a cap bounds the leak

**Decided** (2026-09-20), by the project owner. Refines
[D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays),
and corrects a conflation in it.

D117 said "the link is the grant" and then reasoned as though the link stayed load-bearing for
the whole session — which is why it ended up needing *"invalidate and re-form as one action
that carries the remaining participants across"*, an awkward mechanism to build and an
alarming one to use mid-film. The sharper model removes the need for it:

> **A join link is a credential for *admission*. What persists afterwards is a
> **membership**, and a membership is independent of the link that created it.**

Everything else falls out of that.

### Rotation is seamless, because the link was never what was holding the session up

The owner generates a **new join link for the existing group**. The old link **stops admitting
new clients and keeps serving every client already admitted.** Nobody is interrupted, nobody
re-joins, nothing is re-formed. The session, its roster, its position and its voice channel
are untouched.

This is only implementable because membership is a first-class object with its own identity,
issued at join time — which is also the only thing that makes *per-participant* ejection
possible at all. You cannot remove one person who is identified solely by a link that four
people are holding.

### Ejection is owner-only, and it is a fourth permission

[D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays)
listed three permissions; this is the fourth and it is the one that is **never delegable**:

| | Who |
| --- | --- |
| What plays | creator only, always |
| Transport — play, pause, seek | creator's choice: everyone, or creator only |
| Move within the series | creator's choice |
| **Eject a participant, and rotate the link** | **creator only, never configurable** |

A participant granted transport control still cannot remove anyone. Those are different kinds
of authority and conflating them would let a griefer with control rights eject the host.

**And ejection without rotation is a revolving door.** If the intruder still holds a working
link, booting them means they press join again.

So the two are paired **by a prompt, not by coupling** — refined by the project owner the same
day. Removing someone raises:

> *If you're booting this person due to abuse, would you also like to rotate the join link so
> they can't come back?*

Three things about that wording are the decision, not the copy:

- **It asks about the reason, not the mechanism.** The owner is mid-incident and should not
  have to know what a join link is or why rotating one matters. "So they can't come back" is
  the outcome they care about; "rotate the credential" is our word for how.
- **It is not a default and not automatic**, because rotating has a cost and it falls on
  someone invisible: **a legitimate invitee who has the old link and has not joined yet.**
  Existing participants are undisturbed by a rotation ([D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak)'s
  whole point), so the only person a needless rotation hurts is the one still on their way.
  That is precisely why it cannot be inferred and has to be asked.
- **It never acquires a "don't ask again".** The right answer differs per incident — a
  housemate who has gone to bed and a stranger who found the link are the same button and
  opposite answers — so this is one of the few prompts where remembering the last choice is a
  bug.

A batch ejection (several joins from a leaked link) asks once for the batch.

The same rotate action is offered beside the capacity-refusal signal below, and both use the
same words.

### The cap is the part that makes a leak survivable

**At session creation the owner sets a maximum number of simultaneous clients, with a small
safe default.** This is the control that matters most, because it works *before* the owner
notices anything:

- **It bounds the flood.** A leaked link admits one griefer, not thirty. The damage window
  between a leak and the owner reacting has a ceiling.
- **It is a cost control as well as an abuse control.** Simultaneous clients are what the
  household uplink and the relay pay for
  ([D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible),
  [D71](#d71--one-relay-artefact-three-jobs-and-the-core-dials-out-to-it)) — one remote viewer
  on a remux can saturate an uplink, so a cap is the same number seen from two directions.
- **It is a count of admitted memberships, not of links**, and it is enforced server-side.
  **And memberships are *people*, not clients**
  ([D120](#d120--a-participant-is-a-person-devices-attach-to-them-with-roles-and-the-cap-counts-people)):
  two people with voice-companion phones are four clients, so a cap phrased in clients would
  refuse the second person's phone. The interface says people.

**The default is small**, because [D69](#d69--group-size-is-a-priority-order-not-a-capability-two-then-four-or-five-then-a-party)
says the design is best at two and the motivating case is two people. The exact number is a
product choice; what is not negotiable is that the default is small, it is **visible at
creation rather than buried**, and raising it is one action for the owner at any time.

### A refused join is a signal, not an error

[principles §10](../principles/README.md) says a check that fires in a correct system is worth
looking at. **"Join refused: at capacity" is exactly that** — in normal use it never happens,
because the owner sized the session for the people he invited. Several in a row is the
signature of a leaked link, and it is the earliest evidence available, arriving before the
owner has noticed a stranger in the roster.

So refusals are surfaced to the owner live, with a count, and the natural response — *rotate
the link* — is offered in the same place.

**Status:** The cap's default value is unset here deliberately; it wants the first real party
session to argue about. Two things are unverified: whether a rotation can be made genuinely
invisible to existing clients (it should be — nothing in
[D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
six-method surface mentions the link), and how a client that was mid-join during a rotation
behaves. That second one is the race worth writing a test for rather than discovering.

---

## D120 — A participant is a person; devices attach to them with roles, and the cap counts people

**Decided** (2026-09-21), by the project owner. Extends
[D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak).

The research already concluded that *"on a television the phone is the voice endpoint — an
Apple TV has no app-accessible microphone, and a microphone across the room from the speakers
is the worst possible AEC geometry"*, and called phone-plus-earbuds *"the only workable
configuration"*. It never said how that is **modelled**, and the implicit model — one
participant is one client — cannot express it.

So: **a participant is a person. A person attaches one or more devices, each with a role.**

| Role | Does | Implements |
| --- | --- | --- |
| `playback` | plays the video, reports position, accepts transport | [D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s six methods |
| `voice` | microphone and per-source mix, nothing else | none of the six |
| `control` | a remote: sees the roster, issues transport | none of the six |

**"Voice companion" is a mode, not a feature** — a phone that attaches with the `voice` role
only, while the television holds `playback`. It needs no player, no rendition negotiation and
no sync participation, which is why it costs almost nothing: *a voice-only client implements
none of the six methods*, so [D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
surface is untouched. That is a confirmation of D62 rather than a change to it.

**Attaching a second device reuses [D93](#d93--device-pairing-is-rfc-8628-shaped).** The first
device shows a short code; the second enters it and joins the *same membership* rather than
creating a new one. That is the device-authorisation flow already chosen for signing in on a
television, pointed at a session instead of an account — and it is why "same URL login,
multiple devices per person" does not need an account at either end.

### The cap counts people, and [D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak) said "clients"

A correction, and it would have bitten immediately: two people watching together with voice
companions are **four clients**. A cap phrased as "maximum simultaneous clients" and defaulted
small would refuse the second person's phone. **The cap is a maximum number of
*participants*** — memberships, people — and the interface says people. Device attachments are
bounded separately and generously, because a person with a TV, a phone and a tablet is
ordinary and none of it is an abuse signal.

D119's internals were already right (*"a count of admitted memberships, not of links"*); its
user-facing phrasing was not.

### Voice plus film audio on the phone, video on the television: no

Tempting, and it is a different problem wearing the same clothes. Splitting audio and video
across two devices is **lip sync**, and lip sync has a budget three orders of magnitude tighter
than anything in this feature:

| | |
| --- | --- |
| ITU-R BT.1359 **detectability** | audio leading by **45 ms**, or lagging by **125 ms** |
| ITU-R BT.1359-1 end-to-end tolerance | +90 ms to −185 ms |
| EBU R37, production | **+40 ms to −60 ms** |
| **our sync budget** ([D66](#d66--no-clock-synchronisation-monotonic-local-time-and-bounded-round-trips)) | rate-nudge absorbs **60–3000 ms**; periodic reconciliation in **minutes** |

Our *finest correction unit* is already at the detectability threshold, and our reconciliation
cadence is not in the same universe. Watch Together synchronises two people in two houses to
within a second; lip sync needs two devices in one room to within a frame. The mechanisms are
not related and reusing one for the other would produce a feature that is subtly wrong all the
time.

**The want underneath it is private listening** — headphones while the television is silent —
and the ecosystem's answer is Bluetooth headphones paired to the television, which already
works and is not ours to improve. Out of scope, stated with the number so nobody re-proposes
it without one.

**Status:** The device-attachment model is new surface and the roster UI is where it will feel
clumsy if it is wrong — "Karl (TV + phone)" is one row, not two, and getting that wrong makes
a two-person session look like a crowd.

---

## D121 — An ability is a row in a closed table with a two-valued grant, chosen at session creation

**Decided** (2026-09-21), by the project owner. Simplifies
[D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays)'s
permission prose.

D117 described three permissions in prose, with one of them defaulting by reference to another
("in-series navigation follows transport"). That is more structure than the problem has. The
model is flatter:

> **Each ability is `group` or `host only`. The host picks at session creation. That is all.**

| Ability | Grant | Default |
| --- | --- | --- |
| Transport — play, pause, seek | `group` \| `host only` | `group` |
| Choose what plays | **`host only`, not a setting** | — |
| Eject, rotate the link, change the cap | **`host only`, never configurable** ([D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak)) | — |
| *Change playback within a play set* | `group` \| `host only` | *later* |

Two rows are deliberately not settings. **Choosing what plays** is the host's library and the
host's alone. **Ejection** is never delegable, because a participant granted transport who can
also eject can eject the host.

**Keeping it a table rather than a pair of booleans is the whole of "leave the possibility
open".** Finer-grained control almost certainly is not needed — the request says so and it is
probably right — but a closed enumeration of abilities costs nothing now and means a later row
is a row, not a redesign. The play-set row below is the first one already foreseen.

**Play sets, later** ([D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays)
recorded these as "party playlists"). The host selects a set of videos — a season, a trilogy —
and grants the group the ability to move within it. Binge-watching without handing over the
library. It is cheap later precisely because the scope is an object and the permission is a
row.

**Status:** Sound and smaller than what it replaces. The one thing to watch is grants becoming
*mutable mid-session*, which sounds harmless and is how a two-value setting turns into an
access-control system: for now they are fixed at creation, and if that proves annoying the
change is "the host may change a grant", not "grants may be per-participant".

---

## D122 — The session is the object; what plays is mutable state on it, and a guest sees the stream rather than the library

**Decided** (2026-09-21), by the project owner. **Retires "the group is keyed on an Edition"**
from [D104](#d104--a-watch-together-invite-is-a-capability-url-scoped-to-one-edition-the-group-is-never-a-grant)
and [D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays).

That phrasing was wrong and it was wrong for a traceable reason. D104 was trying to bound the
blast radius of a leaked link, and reached for *scope* as the bound: if the link only reaches
one Edition, a leak is small. Then [D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak)
supplied better bounds — a cap, and seamless rotation — and the scope argument stopped earning
its complexity. It survived anyway, as a phrase, and it does not describe the feature.

> **The session is an ephemeral object. "What is playing" is mutable state on it, and the host
> may change it to anything in his library at any time — mid-video included.**
>
> **A video ending does not end the session.** The group returns to
> [D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
> `Idle` state and waits, which is what that state was always for.

### The bound that actually holds

Losing scope-keying loses nothing, because the real bound was never the item — it is the
**surface**:

> **A guest gets the session's current playback. Not a library, not a browse view, not an
> item they can address by id.**

Whatever the host puts on, the guest sees; anything else in the library is not merely
forbidden, it is **unreachable, because it is not in the guest's world at all**. That is
[principles §10](../principles/README.md) in its strongest form — omission rather than
rejection — and it is a better guarantee than scope-keying was, because it does not depend on
enumerating what the scope contains.

So the leaked-link story is complete without it: the cap bounds how many strangers arrive, the
roster shows them, ejection removes them, rotation stops more coming, and **what a stranger
could see while present is one stream that the host chose and can change.**

**Status:** This is a simplification — a concept removed, not added. The one thing it obliges
is that "change what is playing" is a **first-class group command** with the same serialisation
and version discipline as play/pause/seek
([D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)),
rather than a teardown and re-create. A change of video is a transition in the group state
machine, and every participant's player is expected to survive one.

---

## D123 — Bandwidth is per-participant; the group shares a position, not a stream

**Decided** (2026-09-21). Question raised by the project owner:
what happens when one guest's link cannot carry the stream.

Four options were on the table — transcode per guest, degrade everyone to the lowest common
denominator, let the unlucky guest suffer, or surface per-guest health and let the group apply
pressure. **The first is already the architecture and costs nothing extra here**, which
removes the dilemma.

### It is already solved, by two decisions that were not made for this

[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) puts rendition choice on the
client and [D86](#d86--adaptation-is-the-client-re-picking-on-a-buffer-signal-and-it-puts-bandwidth-transcoding-on-remote-accesss-critical-path)
makes the client re-pick on its own buffer. Applied to a group:

> **Each participant streams at whatever rendition their own link supports. The group shares a
> *position*, not a *stream*.**

Nothing in [D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
six methods mentions bitrate. One guest at 2 Mbps and another at 20 Mbps are at the same
timestamp in different renditions, and neither is aware of the other's choice.

**So no lowest-common-denominator.** Degrading everyone to the worst link punishes four people
for one person's hotel wifi, and it is the option that *feels* fair and is not.

**And the N-guests-N-transcodes worry does not materialise**, because two existing decisions
cap it: the output menu is a **closed enumeration** of about eleven named profiles
([D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list)), and the
transcoder **shares one encoder process between viewers of the same rendition**
([transcoding.md](../design/plans/transcoding.md)). Cost is bounded by the size of the menu,
not by the number of guests — five guests collapse onto at most two or three renditions, and
usually one.

### What genuinely is shared, and therefore genuinely is a group problem

**Stalls.** A participant who keeps buffering affects everyone *if and only if* the group's
stall policy says to wait for them —
[watch-together.md](../design/plans/watch-together.md) already has that as *"a group setting
with a size-dependent default"*, and Jellyfin exposes the same thing as `IgnoreBuffering`.

**So the stall policy is the actual control, and it is kinder than the alternatives.**
"Continue without them" lets a struggling guest fall behind and resynchronise instead of
holding four people at a spinner. Ejection is the last resort, not the first.

### Per-guest health, because the policy is unusable without it

The request's fourth option — a health meter per guest — is right, and it is nearly free:
[D73](#d73--each-component-reports-according-to-its-own-visibility-blind-is-not-opaque) already
routes every client's observations to the core, and
[D91](#d91--a-metrics-endpoint-off-by-default-and-local-when-on) already collects them. Showing
each participant's buffer health and stall count in the roster is a rendering of data that
exists.

It does three jobs at once: it tells the host **why** playback keeps pausing, which is
[principles §3](../principles/README.md) applied to a group; it makes the stall policy a
comprehensible choice rather than a mystery toggle; and it converts "someone is ruining this"
into a named, visible fact.

**On the social mechanic** — *"whoever is dragging the group down gets asked to leave"* — the
mechanism is right and the phrasing should not become the design. Ejection is
**host-only** ([D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak));
what the health meter adds is that the host's decision is *informed*, and that the struggling
guest can see their own meter and bow out first, which is the outcome everyone actually wants.

**Status:** Rests on a transcoder that does not exist yet
([transcoding.md](../design/plans/transcoding.md) §(b)), which
[D86](#d86--adaptation-is-the-client-re-picking-on-a-buffer-signal-and-it-puts-bandwidth-transcoding-on-remote-accesss-critical-path)
already put on remote access's critical path — this is a second consumer arriving at the same
conclusion. Until it exists, a guest whose link cannot carry the original is out of luck, and
the honest thing is to say so in the join screen rather than let them discover it at minute
four.

---

## D124 — The playback role may be a cast receiver, and for a guest that is the best path; the phone controls, it does not relay

**Decided** (2026-09-21), from the project owner's proposal to invert the companion model — the
phone as the client, casting to the television.

**It is not an alternative to [D120](#d120--a-participant-is-a-person-devices-attach-to-them-with-roles-and-the-cap-counts-people); it is D120 with a different playback device.** That decision
already says a person attaches devices with roles and that the session does not care about a
person's local topology. It assumed the `playback` device was our own television client. It
need not be:

| | `playback` | `voice` | `control` |
| --- | --- | --- | --- |
| Our TV client + phone | Android TV app | phone | phone |
| **Cast** | **cast receiver (a web app)** | **phone** | **phone** |
| Phone alone | phone | phone | phone |

Same model, same roster, same six methods
([D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)).
A new **implementation** of one role, not a new shape.

### The real argument is the guest, and it is a strong one

We choose what our own users install. **We do not choose what their friends own.** The
motivating case for this whole feature is someone else's household, and
[tech-clients.md](../design/research/tech-clients.md) is honest that covering the television
long tail is where a fifteen-year head start lives.

A cast receiver is *"cheap because it is a web app"* — and we are already building that web
app in Phase 2. So a guest with any Cast-capable television participates without us shipping a
client for their platform. That is a larger win than the voice-companion arrangement it
started as.

### What Cast actually does, which decides two things at once

**The sender does not relay bytes.** It hands the receiver a URL and commands; the **receiver
fetches the media itself**. Two consequences point in opposite directions and both matter.

**In favour, and this is the part the proposal did not claim:** the phone can sleep. If the
phone held the stream, a pocketed handset would stall the film. Because the receiver pulls, the
right assignment is that **the receiver holds the `playback` membership and speaks the six
methods directly**, while the phone holds `voice` and `control`. A sleeping phone then drops
*voice* — annoying, recoverable, and the correct failure — rather than dropping playback for
everyone. That is a better arrangement than either the original companion model or the
proposal as stated.

**Against: the fan-out does not work.** *"The phone is the fan-out point to other devices in
the guest's home"* would need the phone to serve bytes, and Cast gives it none to serve. To do
it the phone would have to become a caching HTTP proxy holding the session credential, on the
least reliable and most power-constrained device in the chain. **Declined**, and the better
answer already exists: each viewer in that household joins as a participant up to the cap
([D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak)),
and fan-out happens at the relay where
[D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible)
and [D71](#d71--one-relay-artefact-three-jobs-and-the-core-dials-out-to-it) already put it —
one stream into the house, fanned out by a component built to do that.

### The receiver probes and picks. The sender must not pick for it

This is the trap, and it is [D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)'s
entire subject. A cast receiver has a **narrower decoder set than the phone driving it**, so
if the phone negotiates the rendition, the phone is guessing on behalf of hardware it is not —
which is a device-capability database with one row, arriving through the back door.

**The receiver is our web app. It probes itself and picks**, exactly as any other client does;
the phone tells it *what to play*, never *in what form*. Cast's own SDK exposes receiver
capabilities and they should be treated as a hint, not an answer
([D92](#d92--a-failed-rendition-falls-to-the-next-one-and-the-client-remembers-the-server-still-has-no-device-database)
applies unchanged: the probe lies, the ladder is the response).

### Costs, stated

- **A sync hop.** `currentPosition()` now returns the receiver's position with Cast's status
  latency inside it. Our budget is 60–3000 ms
  ([D66](#d66--no-clock-synchronisation-monotonic-local-time-and-bounded-round-trips)) and Cast
  status is not high-frequency, so this eats a real fraction of it. **Unmeasured, and it is the
  number that decides whether this is a first-class path or a fallback.**
- **"Cast" is three protocols, not one.** Google Cast, AirPlay, and our own. Fire TV has **no
  Google Cast** ([tv-client.md](../design/plans/tv-client.md) already notes the missing Play
  Services), and Apple devices want AirPlay. So this is a family of receiver implementations
  behind one role, which is [principles §5](../principles/README.md)'s shape and should be
  built that way from the first one.
- **A Google dependency and $5.** A custom receiver needs a Cast developer account
  ([tech-clients.md](../design/research/tech-clients.md)); the Default Media Receiver needs no
  registration and cannot do our subtitles. That is an account *we* hold, not one a user holds,
  so [D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise) is untouched — but it
  is a dependency worth naming rather than discovering.
- **The session credential reaches a third-party device.** The guest's Cast hardware fetches
  from the host's server, so it holds a session token. Consistent with
  [D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays)
  — the link is already the grant and revocation is already the control — and it composes with
  [D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible)'s
  "authorise the session, not the segment". Worth stating because it is a wider exposure than a
  first-party client, and rotation is what answers it.

### And it confirms [D88](#d88--casting-is-remote-control-of-another-client-which-is-watch-together-with-a-group-of-one)

D88 said casting *is* Watch Together with a group of one. This is the same claim from the other
end: a group of several, where one participant's playback device happens to be a cast receiver.
The two compose without a seam, which is the evidence that D88 named the right abstraction.

**Status:** The sync-hop measurement is the open question and it gates how this is presented —
first-class path or documented fallback. Build order should be **web client → cast receiver**,
since the receiver is mostly the web player in a different shell, and the guest-side benefit
arrives long before our own television client is finished.

---

## D125 — Access is role-based with two roles; a Watch Together guest is not a principal

**Decided** (2026-09-21), by the project owner. Closes the open question in
[auth.md](../design/plans/auth.md): *"whether a household's users are one flat set or whether
'profiles' … is a distinct concept from 'accounts'"*.

**One concept, not two. There is no `Profile` separate from a `User`.** Netflix separates them
because one paying account fronts several viewers under one licence; we license nothing
([D76](#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed)), so the thing that forced the split does
not exist here.

### The model

| | |
| --- | --- |
| **Roles** | `admin` and `user`. Two, closed, not a permission lattice. |
| **An admin is also a user** | Same row, same play state, same preferences. Administration is a capability the row carries, not a second account to switch into. |
| **What `admin` grants** | Library configuration, node enrolment, user creation, deletion policy, the metrics endpoint — the things that change the *server*. |
| **What every user gets** | Their own play state, history, favourites, ratings, preferences, downloads and rules. Independent, always, including the admin's. |
| **What a user sees** | Whatever their predicate allows ([D76](#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed), [D84](#d84--certifications-are-system-code-with-a-per-system-order-a-kids-profile-is-a-grantee-plus-a-predicate)). |

**A grantee is a user with a predicate, and so is a child.** The three things the tree modelled
separately — a household member, a shared-with relative, a kids profile — are one row and one
rule each. [D84](#d84--certifications-are-system-code-with-a-per-system-order-a-kids-profile-is-a-grantee-plus-a-predicate) already said a kids profile is *"a grantee plus a predicate"*;
this is that observation taken all the way, and it means the fifth binding of the rules engine
is also the only access mechanism there is.

Two corrections fall out of it, both in [parental-controls.md](../design/plans/parental-controls.md),
which was written in Netflix's vocabulary:

- *"A PIN required to create a profile"* becomes **only an admin creates a user**, which is
  strictly stronger and costs nothing. Netflix needs the PIN because a profile is created from
  inside a shared account; here there is no such act.
- *"A profile-lock PIN"* stays, and it is now a **user-level** credential rather than a
  sub-account one — the thing that stops a child picking the parent's row on the television.

### A Watch Together guest is not a user, and nothing about them persists

[D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays) made the link the grant, and the consequence has to be said plainly
because it is the one place the access model has a hole by design: **a guest is authenticated
by possession of the URL and by nothing else.** That is deliberate, it is the point of the
feature, and the mitigations are the ones already decided — a cap, ejection, and rotation
([D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak)).

What follows, and is new:

- **No user row is created.** A membership ([D120](#d120--a-participant-is-a-person-devices-attach-to-them-with-roles-and-the-cap-counts-people))
  lives as long as the session and is then gone. No account, no password, no recovery, no
  lingering grant — which is also why a leaked link is survivable: there is nothing to clean up
  but the session.
- **A guest accrues no play state, history or statistics.** They are not a `(user, edition)`
  key ([D81](#d81--play-state-is-an-append-only-event-log-plus-a-guarded-position-and-positions-are-edition-relative)) because there is no user. **The host's position advances**,
  because it is the host's library, the host's session and the host's choice of what plays
  ([D122](#d122--the-session-is-the-object-what-plays-is-mutable-state-on-it-and-a-guest-sees-the-stream-rather-than-the-library)).
- **A guest is therefore outside every predicate**, which is the honest restatement of
  [D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays) superseding [D104](#d104--a-watch-together-invite-is-a-capability-url-scoped-to-one-edition-the-group-is-never-a-grant)'s
  entitlement half. It is bounded not by a rule but by what a session exposes: the current
  playback, nothing addressable, nothing enumerable.

**The test that matters** is that the two paths do not leak into each other: a guest must never
acquire a durable identity by accident — no row written on join, no preference persisted, no
resume position recorded — because a guest who becomes a user is a griefer who survived
rotation.

**Status:** The token model is still [threat-model.md](../design/plans/threat-model.md)'s open
question; this settles the *principals*, not their credentials. Unresolved: whether a second
admin is required before the first can be removed (it should be — a household with no admin is
unrecoverable), and whether `admin` should be revocable by another admin.

---

## D126 — Hostile *bytes* never reach the core; hostile *strings* reach it constantly, and that is the design

**Decided** (2026-09-21). Prompted by the project owner's summary — *"hostile data is ingested
to the nodes, never directly to the core"* — which is the right instinct and is **not literally
true**. Recorded because the literal version is the kind of claim that becomes load-bearing and
then becomes false quietly.

Three classes, and only the first is kept out:

| Class | Example | Reaches the core? | Parsed by the core? |
| --- | --- | --- | --- |
| **Media and image bytes** | the film, a TMDB poster | poster: **yes, in flight** | **never** |
| **Derived strings and structures** | filenames, track titles, probe output, node reports | **yes, all of it** | **yes, by design** |
| **Provider responses** | TMDB/TheTVDB JSON | yes | yes |

**Class 1 is the one the rule is about.** [D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)
keeps FFmpeg out of the core and [D99](#d99--the-core-does-not-decode-an-image-either-and-community-uploaded-artwork-is-why)
extends it to images — but note D99's actual shape: **the core fetches the poster.** Hostile
bytes pass *through* the core's memory and are handed to the confined worker undecoded. The
rule is *never parses*, not *never touches*, and Immich ships exactly this
([node-transport.md](../design/research/node-transport.md) §2).

**Class 2 is not an exception; it is the entire point of [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret).**
The core parses filenames, reads track titles typed by strangers, and interprets probe output —
and it does so precisely *because* that work must be replayable over the database with every
node switched off. Moving it to the node to "keep hostile data out" would undo D59 and buy
nothing, because these are not bytes fed to a decoder.

What makes class 2 safe is different machinery and it already exists:

- **Go, not C.** No heap overflow in a filename parser.
- **RE2** ([D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable)) — no catastrophic backtracking, measured at 16 µs for `(a+)+$`.
- **Parse, don't validate, at the boundary** ([media-nodes.md](../design/media-nodes.md) trap 2),
  with malformed reports expected rather than exceptional
  ([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global), [D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope)) — the routine-validation case
  [principles §10](../principles/README.md) explicitly separates from
  entitlement checks.
- **Unicode discipline** ([D49](#d49--unicode-discipline-at-the-boundaries-enforced-by-tests)),
  because the failure mode here is a silent misparse rather than a crash.

**So the correct sentence, and the one to use:** *the core never parses a media or image file;
everything else a hostile file can influence reaches the core as text and is handled as text.*

**Status:** One consequence is unbudgeted. A node report is attacker-influenced JSON and the
core parses **50.8 MB of it per scan** (measured, [node-transport.md](../design/research/node-transport.md)).
That is a parser on the hot path with hostile input — memory-safe, but a denial-of-service
surface if a node is spoofed or compromised. Bounds on record size, record count and nesting
depth belong in the wire format, and are not yet stated.

---

## D127 — The node dials out; a Unix socket is the default and the file channel is the fallback

**Decided** (2026-09-21). Research: [node-transport.md](../design/research/node-transport.md).
Answers the project owner's *"we haven't looked seriously at the full range of possibilities for
node↔core comms, or what actually makes sense for the effort."*

Three transports, ranked by how many installs use them:

**1. A Unix domain socket, for the default install.** One machine, the core supervising a
co-located node ([D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do)). No
network, no TLS, no enrolment, no ports — and the socket's file permissions are the security
model in exactly the sense [D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)
meant, with peer credentials available on top.
[media-nodes.md](../design/media-nodes.md) already says local roles should speak the same
protocol as remote ones over a Unix socket; this promotes that from an aside to the primary
path, which means **the common deployment exercises the protocol the rare one depends on**.

**2. HTTP, with the node dialling out, for a remote node.** The whole industry converged here —
GitHub's runners long-poll on a 50-second timeout, AWS's SSM agent speaks outbound 443 to a
message gateway, and **Tdarr's node**, which is this exact problem in this exact domain, holds a
persistent outbound connection with *"no inbound Node port required"*.

**The reason to copy it is not the firewall, it is [D107](#d107--the-node-channel-is-mutually-authenticated-and-a-nodes-roots-are-local-and-never-remotely-settable).** D107 found that the
dangerous direction is core→node, because `config.json` carries commands and, since
[D89](#d89--deletion-is-opt-in-per-library-two-phase-and-names-every-file), deletion. **If the node
always initiates, that direction stops existing as a network path.** A spoofed core cannot write
to a node with no inbound surface; it can only try to answer a connection the node opened and
authenticated. The NAT argument and the security argument point the same way, which is rare
enough to be worth taking.

Commands therefore arrive **as the response to a poll the node made**, not as a write the core
performed. D35's desired-state/observed-state pair survives unchanged and becomes cheaper: the
`generation` counter rides the poll response and `observed_config` rides the next request.

**3. The file exchange directory, as the fallback it was always called.** Kept, because it is
the only transport that crosses a network filesystem with no port, no certificate and no
configuration, and on a NAS with a share already mounted that is real. Two costs are now
written down rather than discovered:

- **The client attribute cache.** `nfs(5)` defaults are `acregmax`/`acdirmax` **60 s**, doubling
  towards the maximum while nothing changes. A rename is atomic on the server and **not
  immediately visible to another client**. D35 knew watching fails on a share and prescribed
  polling; **polling does not fix this**, because the poller reads the cache. The practical
  consequence: **[D118](#d118--the-scan-is-incremental-prioritised-and-steerable-and-playability-comes-first)'s
  incremental ingest is a local-filesystem feature** — over NFS, items appear in 60-second lumps.
- **The command file must be signed.** D35's *"the permissions split is the security model"* is
  correct and assumes a careful operator. A typical home NAS share is writable by everything on
  the LAN, so on this transport **D107's dangerous direction is open by default** and there is no
  transport to authenticate. The signature moves the check into the payload, which is the only
  place it can live here.

**And what the measurements say about effort.** Throughput does not discriminate: a 50,000-entry
snapshot is 50.8 MB and every transport moves it in 536–551 ms. Two things do:

- **Latency to the first usable record differs by ~5,000×** — 0.1 ms streaming over HTTP against
  551 ms waiting for a rename — because HTTP parses the body as it arrives. **That is D118's
  incremental ingest for free, and D118 invented a `.partial`-tailing mechanism to get on the
  file channel what one transport hands over.**
- **The file channel makes you rebuild acknowledgement, compression negotiation, resume,
  backpressure and error reporting**, all of which HTTP has. D35 already hand-built the first
  one. That is the effort question answered: the fallback is not cheaper, it is cheaper *to
  start* and more expensive to finish.

**Two removals.** *"Core polls the node's HTTP endpoint, for nodes behind a firewall that cannot
dial out"* ([media-nodes.md](../design/media-nodes.md)) goes: the justification is backwards —
a node that cannot dial out is less reachable inbound, not more — and every mode costs. SSH and
a message broker are declined, the first because we must authenticate the channel anyway and the
second because it is a second service at household scale
([principles §1](../principles/README.md)).

**Status:** ~~Compression is `Accept-Encoding` with **gzip level 1** (measured 5.7×, 262 ms;
level 6 buys 27% more for twice the CPU) and is worth it below about 1 Gbit and not above.~~
**Refined by [D129](#d129--zstd-on-the-node-channel-gzip-on-the-client-channel-and-nothing-on-a-socket)**:
the negotiation stands, the codec does not — **zstd** on this channel, because both ends are our
own binary and it beats gzip on size *and* both speeds at once. gzip remains the floor on the
*client* channel, where old televisions are. Unresolved:
whether the remote node's command channel is a long poll or a WebSocket — long poll is duller and
[principles §1](../principles/README.md) prefers it, but sub-second command latency
may argue otherwise, and neither matters until a remote node exists.

---

## D128 — The artefact channel is the real payload, and the node keeps what it makes

**Decided** (2026-09-21). Found by [node-transport.md](../design/research/node-transport.md) §0
while measuring something else, which is the usual way.

**The tree specifies both ends and not the middle.** [artwork.md](../design/plans/artwork.md)
says derived images are *"produced at scan time, in the node"* and that *"all of it lives in
`cache/`"*; [overview.md](../design/overview.md) draws `cache/` beside the **core** and has the
core serving from it. Nothing says how the bytes get from one to the other. In the default
install they are the same machine and the gap is invisible — which is how it survived the
2026-09-19 audit and three rounds since.

**It is not a small gap.** Over the reference library (3,500 items, 3,167 h):

| Artefact | Volume | Source |
| --- | --- | --- |
| Subtitle overlay bitmaps | **42 GB** | 12 MB/film ([D78](#d78--server-rendered-subtitle-overlays-are-a-third-rendition-and-they-dominate-burn-in)) × 3,500 |
| Trickplay sprites | **5.7 GB** | measured 30,040 B per minute of video |
| Posters at three sizes | ~1.0 GB | [artwork.md](../design/plans/artwork.md) |
| **One snapshot report** | **0.051 GB** | measured |

Three orders of magnitude between the thing the transport discussion was about and the thing it
should have been about.

**The answer is Netflix's, and half of it is already in the tree.** An Open Connect Appliance
does exactly two things: *"report their status … and what files they have stored on disk"*, and
*"serve content via HTTP/HTTPS when it is requested by a client device"* — with the control
plane steering clients **via URL**. Bytes never transit the control plane.
[media-nodes.md](../design/media-nodes.md) already contains ours, filed under *"what this
permits later, for free"*: [D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)'s
signed, scoped, expiring URLs let a node serve its own files with the core issuing the redirect.
**That stops being a later nicety and becomes the artefact channel.**

**So: the node keeps what it makes, and the core redirects — with one split.**

| | Where it lives | Why |
| --- | --- | --- |
| **Large, cold, streamed** — overlay bitmaps, trickplay sprites | **the node**, redirected to | 48 GB is not worth moving, and each is fetched at most once per viewing |
| **Small, hot, many-per-page** — posters, thumbnails | **the core's `cache/`**, uploaded at production | a home page is fifty posters and fifty redirects is a worse page ([artwork.md](../design/plans/artwork.md)) |

The rule is not size alone: it is **requests per page**. Anything the client fetches in bulk
wants to be one hop away; anything it fetches one of, per playback, can be a redirect.

**Four consequences.**

- **`cache/` is two caches**, one per side, and both keep the property that made it safe:
  deleting either costs time and nothing else ([overview.md](../design/overview.md)).
- **A node that is offline costs trickplay and overlays, not the film** — but only if the core
  serves the film, which in the default topology it does. In a topology where the node serves
  the media too, an offline node is an unavailable item, which
  [media-nodes.md](../design/media-nodes.md) trap 1 already governs: unavailable, never deleted.
- **This is why segment URLs had to stay cacheable** ([D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible)).
  The same signing rule now covers artefacts: sign the session, not the sprite, or a household
  with two viewers has two URLs for identical bytes.
- **It bounds what [performance-targets.md](../design/plans/performance-targets.md) must
  measure.** "Home page of 50 posters" is currently `unset` pending
  [artwork.md](../design/plans/artwork.md); it now also depends on which side of this split a
  poster is on, and the answer above is what makes the target measurable at all.

**Status:** Unresolved — whether the node's artefact store is addressed by content hash (which
[artwork.md](../design/plans/artwork.md) already wants for immutability, and which would make the
redirect target stable across a re-derivation) or by `(item, kind, size)`. Also unmeasured: the
redirect's cost against a direct serve on a home page, which is the number that decides whether
the split above is drawn in the right place.

---

## D129 — zstd on the node channel, gzip on the client channel, and nothing on a socket

**Decided** (2026-09-21). **Refines [D127](#d127--the-node-dials-out-a-unix-socket-is-the-default-and-the-file-channel-is-the-fallback)'s
compression clause**, which said gzip level 1. Measured on the same realistic 50,000-entry
corpus.

The question "is there enough support for zstd?" has two answers because there are two
channels, and conflating them is how a codec ends up in the wrong one.

### The node channel: yes, and it is not a support question at all

**Both ends are our own binary.** There is no third-party decoder to accommodate, so the only
question is whether a Go implementation exists that we are willing to depend on.
`github.com/klauspost/compress` is **pure Go with no cgo** (optional amd64/arm64 assembly with
generic fallbacks), BSD-3-Clause — compatible with both halves of
[D80](#d80--licence-by-artefact-gplv3-for-the-server-mpl-20-for-the-clients-and-the-sdk)'s
licence split — and about as boring as a third-party Go dependency gets
([principles §1](../principles/README.md)).

Measured, 50.76 MB snapshot, best of three, 8 cores:

| codec | bytes | ratio | compress | decompress |
| --- | --- | --- | --- | --- |
| gzip -1 (stdlib) | 8,829,427 | 5.7× | 193 ms | 129 ms |
| gzip -6 (stdlib) | 6,513,053 | 7.8× | 456 ms | 97 ms |
| **zstd fastest** | **7,904,912** | **6.4×** | **122 ms** | **68 ms** |
| zstd default | 7,513,344 | 6.8× | 145 ms | 91 ms |
| zstd better | 6,602,112 | 7.7× | 313 ms | 80 ms |

**zstd fastest beats gzip -1 on all three axes at once** — 11% smaller, 1.6× faster to
compress, 1.9× faster to decompress. There is no trade being made, which is unusual enough to
state plainly. zstd *better* matches gzip -6's ratio at 1.5× the speed.

Decompression is the number that matters most, because the **core** pays it, on every report,
while [D126](#d126--hostile-bytes-never-reach-the-core-hostile-strings-reach-it-constantly-and-that-is-the-design) has it already parsing 50.8 MB of attacker-influenced
JSON per scan.

### The client channel: no, and [D85](#d85--the-api-has-a-compatibility-contract-and-it-covers-three-protocols) is exactly why

`caniuse` for `Content-Encoding: zstd` (read 2026-09-21): **85.17% global**, Chrome/Edge 123+,
Firefox 126+ — but **Safari desktop only from 26.0**, **iOS Safari fully only at 26.3**, and
**Samsung Internet not supported at all through v30**.

That last one is disqualifying rather than inconvenient. Samsung Internet is the Tizen lineage,
and this project's stated position is that **televisions never update**
([D85](#d85--the-api-has-a-compatibility-contract-and-it-covers-three-protocols),
[D108](#d108--the-interface-is-not-ours-to-change-out-from-under-people-and-an-old-client-stays-installable)).
An old television browser is precisely the population that will not have zstd, permanently. A
global percentage computed over *phones* is the wrong denominator for us.

**It costs nothing to get right, because `Accept-Encoding` is a negotiation.** Serve zstd to a
client that asks for it and gzip to everything else. The rule is only: **never make zstd the
floor**, and never let a client's failure to advertise it become an error rather than a
fallback.

### And on a Unix socket, compress nothing

| link | none | gzip -1 | zstd fastest |
| --- | --- | --- | --- |
| 100 Mbit | 4.06 s | 1.02 s | **0.83 s** |
| 1 Gbit | 0.41 s | 0.39 s | **0.26 s** |
| loopback / socket | **0.04 s** | 0.33 s | 0.21 s |

On the default install the "link" is a memcpy and **compression is a pessimisation by 5×**.
This is not a special case to code: it is what happens naturally if the *link* picks the
encoding rather than the format picking it, which is what `Accept-Encoding` is for. Hardcoding
a codec is the mistake in all three rows.

**Status:** Dictionary-trained zstd is deliberately not taken. Node reports are extremely
self-similar across scans and a trained dictionary would likely beat these numbers
substantially — but it is a shared artefact that has to be versioned on both ends, which is a
compatibility surface ([D85](#d85--the-api-has-a-compatibility-contract-and-it-covers-three-protocols))
bought for a channel that is already sub-second. Revisit only if a report ever crosses a link
slow enough to notice.

---

## D130 — A role is a named policy object; users are assigned to one, never configured individually

**Decided** (2026-09-21), by the project owner. **Corrects
[D125](#d125--access-is-role-based-with-two-roles-a-watch-together-guest-is-not-a-principal)**,
which said two closed roles and *"a grantee is a user with a predicate, and so is a child"*.

The owner's objection is operational and it is right: *"I don't want to be manually setting up
the restrictions on each kid's account. I want one place, 'kids', and then assign each kid to
that."* **A predicate attached to a user means three children are three copies of one policy**,
and copies drift — the household raises the rating ceiling in two places and forgets the third,
which fails *open*, silently, in the one subsystem where that matters
([D84](#d84--certifications-are-system-code-with-a-per-system-order-a-kids-profile-is-a-grantee-plus-a-predicate)).

### The argument that decides the shape, from the incumbent

`MediaBrowser.Model/Users/UserPolicy.cs` on Jellyfin master carries **44 settable fields** —
`MaxParentalRating`, `BlockedTags`, `AllowedTags`, `EnabledFolders`, `BlockUnratedItems`,
`EnableContentDeletion`, `EnableContentDeletionFromFolders`, `AccessSchedules`,
`EnableRemoteAccess`, `SyncPlayAccess` and thirty-odd more — **per user, with no grouping
whatsoever.** Their own forum: *"no option exists to create groups to assign permissions more
efficiently"*, and "add user group support" is a standing request on their feature tracker.

The causation runs the way that matters to us: **44 fields exist *because* there are no roles.**
With no way to say "these people are the same", every distinction anyone ever needed became
another checkbox on every account. So a role is not only an ergonomic win — **it is what keeps
the capability list short**, and the capability list is the thing that otherwise grows without
limit.

### The model

A **Role** is a named row carrying three things:

| Part | What | Vocabulary |
| --- | --- | --- |
| **Capabilities** | a closed set of verbs | the enum below |
| **Visibility** | one predicate | [D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable)'s language over its field registry |
| **Defaults** | preference seeds for a new member | [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s axes |

**A user has exactly one role.** Not a set: two roles means combining two predicates, and
neither `AND` nor `OR` is a defensible default — one silently narrows and the other silently
widens, and the widening one breaks
[D76](#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed)'s fail-closed rule.
Household scale does not need the expressiveness, and a lattice is the thing
[D125](#d125--access-is-role-based-with-two-roles-a-watch-together-guest-is-not-a-principal)
was right to refuse. **One role, and roles are cheap to make.**

**Two built-in roles, undeletable**: `Administrator` and `Household` (everything except
administration). Undeletable because a server whose last admin role was removed is
unrecoverable — the same reasoning as D125's open question about the last admin *user*, one
level up.

**Everything else the household creates**: `Kids`, `Teens`, `Guests`, `The in-laws`. Defining
"Kids" once and assigning three children to it is the whole request, and it is one join column.

### The capability set, and the rule that keeps it short

Starting set, closed, each with a decision already behind it:

| Capability | Gates | From |
| --- | --- | --- |
| `administer` | library config, node enrolment, **user and role creation**, deletion policy, `/metrics` | [D125](#d125--access-is-role-based-with-two-roles-a-watch-together-guest-is-not-a-principal), [D91](#d91--a-metrics-endpoint-off-by-default-and-local-when-on) |
| `delete_media` | the two-phase delete, where a library permits it at all | [D89](#d89--deletion-is-opt-in-per-library-two-phase-and-names-every-file) |
| `download` | offline renditions and their leases | [D87](#d87--a-download-is-a-rendition-plus-a-lease-and-it-is-self-contained) |
| `host_watch_together` | **creating a session, which mints a link that admits strangers** | [D117](#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays) |
| `edit_metadata` | corrections, and sidecar writes where a path is granted | [D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index), [D98](#d98--a-sidecar-carries-what-is-true-about-the-work-the-database-carries-what-is-true-about-a-person) |
| `remote_access` | reaching the server from outside the house | [D14](#d14--remote-access-for-non-technical-family-is-a-headline-feature), [D71](#d71--one-relay-artefact-three-jobs-and-the-core-dials-out-to-it) |

**`host_watch_together` is the one worth noticing.** D125 established that a Watch Together
guest is authenticated by possession of a URL and nothing else — which is fine when an adult
chose to share it, and is a child handing strangers a route into the household's library
otherwise. The hole D125 accepted deliberately is only acceptable if **who may open it** is
itself a permission. A `Kids` role has this off.

**The rule that stops this becoming 44 fields: a capability is added only when a real feature
would otherwise have no gate**, never speculatively — the same rule
[D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three) applies
to plugin tiers. The six above each have one.

### What this replaces, and what it unifies

- **The rules engine's bindings bind to a role, not a user.** D76's share and D84's profile
  restriction were both *"a grantee plus a predicate"*; they become *"a role's predicate"*, and
  one evaluation serves every member. That is a simplification of both.
- **A share is a role with one member.** Sharing the same view with three relatives stops being
  three predicates. The UI need not say "role" for a one-off — *"share with…"* may create or
  reuse one invisibly — but there is one mechanism underneath.
- **Editing a role changes everybody in it**, which is the point and is also the danger. The
  confirmation **names the count and the members**: *"this widens what 3 users can see —
  Alice, Bob, Carol"*. That is [principles §2](../principles/README.md)'s "destructive actions
  say exactly what they will destroy", generalised to "policy changes say exactly whom they
  affect", and [§3](../principles/README.md)'s explain-the-decision.

### Per-user exceptions: one direction only

The temptation is an override per user, and it reintroduces exactly the drift this decision
removes. **If a child needs different rules, that is a different role** — which is why roles
must be cheap.

**The single exception is a per-user deny list**, because it can only ever *narrow*: "Alice
does not see this one film" needs no new role and cannot widen anything. Anything that would
grant is a role change. The asymmetry is deliberate and is the same fail-closed rule as D76.

**Status:** Schema, and therefore **Phase 1**
([library-model.md](../design/plans/library-model.md)): a `role` table, a `role_id` on the user,
and the predicate stored in the rules engine's serialised form. Unresolved: whether a role may
carry an **access schedule** (Jellyfin's `AccessSchedules` — "no television after 9pm" is a real
household want and is a fourth part, not a capability); and whether the built-in `Household`
role is editable or merely undeletable — editing it is how someone locks themselves out of their
own library.

---

## D131 — A restriction is a vector, not a ceiling; templates are seeds and they say what they hid

**Decided** (2026-09-21), by the project owner. Extends
[D130](#d130--a-role-is-a-named-policy-object-users-are-assigned-to-one-never-configured-individually)
with what a role's predicate must be able to *say*. Research:
[content-rating-systems.md](../design/research/content-rating-systems.md).

The four requirements: a **toddler** who watches cartoons on a rugged tablet; a **tween** who
should not watch *Sex and the City*; an **early teenager** who should not watch *The Blair
Witch Project*; **relatives** who should not see the household's private material.

**Only one of those is a number, and no ordering satisfies two of them.** A maturity slider
ranks *Sex and the City* against *Blair Witch*; the household blocks the first for one child
and the second for another, and neither film is "more mature". This is
[D84](#d84--certifications-are-system-code-with-a-per-system-order-a-kids-profile-is-a-grantee-plus-a-predicate)'s
argument raised one level: flattening national systems to an integer fails open, and flattening
*content concerns* to one axis fails the same way, on top of it.

### The axes, taken from the system that specifies them best

**Kijkwijzer** — the Dutch system PEGI was built from — separates an age level from **seven
content descriptors**: violence, fear, sex, discrimination, drugs/alcohol/smoking, coarse
language, dangerous imitable acts. We adopt those seven as our internal vocabulary, because
they are already the most carefully specified set and because their classifier computes each
axis independently and *then* takes the maximum — **the single number is a lossy projection of
a vector somebody already calculated**.

So a role's restriction may speak about:

1. **An age ceiling per system** ([D84](#d84--certifications-are-system-code-with-a-per-system-order-a-kids-profile-is-a-grantee-plus-a-predicate)'s `(system, code)`), evaluated across **every** system the item
   carries and resolved to the **most restrictive**. Deterministic, and fails closed.
2. **Per-axis ceilings** over the seven, so "no sex above level 2" and "no fear above level 1"
   are independent statements.
3. **Genre, keyword and tag predicates** — [D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable)'s existing language, which is where
   `horror` catches *Blair Witch* when no descriptor exists.
4. **Explicit allow and deny lists.**

**The relatives case needs none of this**, and that is worth noticing: *"not tagged private"* is
a pure tag predicate, it needs no rating data, and it is therefore the only one of the four
that will work perfectly. It is also the one with the strongest privacy claim, since "our more
private stuff" in a family media server means home video of the children.

### The evidence that this is the incumbent's actual bug

US TV ratings already carry the axes in the certification string — **D**ialogue, **L**anguage,
**S**ex, **V**iolence. Jellyfin ships them and throws them away. Verbatim from
`Emby.Server.Implementations/Localization/Ratings/us.json`:

```json
["TV-14-D","TV-14-L","TV-14-S","TV-14-V","TV-14-DL", … ,"TV-14-DLSV"]
   → { "score": 14, "subScore": 1 }
```

**All fifteen combinations collapse to one value.** `TV-14-S` and `TV-14-V` become
indistinguishable — the tween and the teenager get the same answer, from a string that
contained the distinction. Their `subScore` is a boolean wearing an integer: *has descriptors*,
or *does not*.

And their cross-system fallback ends in
`foreach (var dictionary in _allParentalRatings.Values)` — **the first match from any of 45
national systems, in dictionary iteration order**. A parental control whose answer depends on
hash ordering, and whose wrong answers show a child something.

### What is actually obtainable, said plainly

| Source | Age | Descriptors |
| --- | --- | --- |
| TMDB `release_dates` | **yes**, per country | a `descriptors` array exists in the schema, **untyped and empty in their own example** |
| US TV rating strings | yes | **yes, free** — parse `-DLSV` off a certification we already hold |
| Genres, keywords, our tags | — | a proxy, and the honest fallback |

**So the ceiling is obtainable and the axes mostly are not.** The design uses descriptors where
they exist, falls back to genre and keyword where they do not, and **records which of the two
it used**, so the household can see where the judgement is weak.

**Unknown is a third state, per axis.** D84 already requires it for a missing certification; it
now applies to each descriptor. *No violence descriptor* means **unknown**, never **none**, and
what happens to unknown is the household's explicit choice rather than a fall-through.

### Templates: seeds, not subscriptions

Ship a small starting set — `Young child`, `Pre-teen`, `Teenager`, `Guest` — each expressed in
the axes above and compiled against whatever rating systems the household's metadata actually
carries.

Three rules, and the first is the one that matters:

- **A template is copied into the role and then owned by the household.** It is never a live
  link. A template refresh that silently widens what a child may see is the worst failure
  available here, and it would arrive through a routine update — the same reasoning as
  [D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index)'s refusal to
  let a refresh overwrite a user's correction.
- **The template reports what it hid.** *"Hides 412 of your 3,500 items; 190 carry no rating in
  your system and are currently hidden; 47 were judged by genre because no descriptor was
  available."* That converts "not foolproof" from a disclaimer into an inspectable fact
  ([principles §3](../principles/README.md)). **The owner already knows it is approximate; the
  product has to make the approximation legible rather than repeat the warning.**
- **The youngest gets a different *shape*, not a lower number.** `Young child` seeds an
  **allowlist** — "these six shows" — not a ceiling, because "everything rated U" is not what
  anyone means by a toddler's tablet, and an allowlist fails closed by construction. The person
  least able to report a mistake gets the predicate shape that cannot leak.

**Status:** Phase 1 only insofar as it is schema — the per-axis columns and the
"which source judged this" provenance field join
[library-model.md](../design/plans/library-model.md)'s certification table. The template set
itself is **not** Phase 1 and should not be built until there is a real library to test it
against; a template whose blocked-item count was never checked against a real collection is a
guess with a progress bar. Unresolved: whether to carry a curated descriptor source for the
systems TMDB does not populate (Kijkwijzer publishes them; the licensing is unexamined), and
whether the seven axes need an eighth for the thing households actually ask about most —
jump scares distinct from sustained dread, which Kijkwijzer folds into `fear`.

---

## D132 — Authoring a role is a preview loop, and an item the rules cannot judge is denied, not shown

**Decided** (2026-09-21), by the project owner. Completes
[D130](#d130--a-role-is-a-named-policy-object-users-are-assigned-to-one-never-configured-individually)
and [D131](#d131--a-restriction-is-a-vector-not-a-ceiling-templates-are-seeds-and-they-say-what-they-hid):
those say what a role *is* and what it can *express*; this says how an admin arrives at one,
and what happens to everything the rules cannot decide.

### The loop

1. **Browse the library as the role.** Not a report — the actual grid, as that role sees it.
2. **Select what is still wrong.** Point at the offenders.
3. **The system proposes rules that would exclude them**, drawn from the metadata those items
   actually share, each shown with what it would cost.
4. **Preview the change before committing it**, in both directions.
5. **Or name the item outright**, with no metadata involved at all.

### 1. Browse as the role, and it is nearly free

**The preview must use the same evaluation path as enforcement**, or it is a simulation that
will diverge from the thing it simulates — which is the classic form of this bug and is worse
than no preview, because it is trusted. One predicate compiler, one query, a different
principal.

It is cheap for a reason already decided:
[principles §10](../principles/README.md) requires the manifest to be **built per principal**,
omitting what that principal may not have. "Browse as this role" is that construction with a
different argument. **The feature is nearly free because §10 was taken seriously earlier.**

**Browsing as a role must not expose that person's data.** The view is policy; the watch
history, resume positions and favourites are the person's
([D125](#d125--access-is-role-based-with-two-roles-a-watch-together-guest-is-not-a-principal)
gives every user their own). An admin authoring `Kids` needs to see *what is visible*, not
*what was watched*, and the two must not arrive together. This is the failure mode of the
field's workaround: neither Plex nor Jellyfin has preview-as-user, and the documented advice is
to **create a test account and log into it** — which of course exposes everything that account
holds.

### 2. From a selection, propose the generalisation

The admin picks examples; the system finds the rule. For a selection, rank candidate predicate
terms by how cleanly each separates the selection from the rest of the visible set, and show
the collateral:

> `genre = Horror` — excludes **12 of your 14**, and also removes **3 you did not select**
> `keyword = slasher` — excludes **9 of 14**, and removes **nothing else**

**Candidates come only from [D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable)'s registered fields**, which is what
makes this safe and honest at once: no user string becomes a SQL identifier, and every
suggestion is expressible in the same language that will enforce it. A proposal the engine
could not then evaluate would be a demo.

**And when nothing separates them, the proposal is to create a tag.** If no registered field
distinguishes the selection, the honest suggestion is *"these fourteen have nothing in common
that I can see — shall I tag them `scary` and exclude that?"* — which is §4, reached from the
same loop. The suggestion engine's fallback is not "give up", it is "there is no rule yet; make
one true".

Nobody hand-writes a predicate. That is the point — the admin's input is *examples*, which is
the thing a person is good at supplying.

### 3. Preview the diff, and emphasise the widening

Before committing, show what **leaves**, what **enters**, and which of those the admin did not
ask for. **Entering is the dangerous direction** and gets the emphasis: a rule edit that
accidentally reveals is the failure this whole subsystem exists to prevent, and it is the one a
satisfied admin will not look for.

Same machinery as [D131](#d131--a-restriction-is-a-vector-not-a-ceiling-templates-are-seeds-and-they-say-what-they-hid)'s *"the template reports what it hid"* — one mechanism, two
consumers. And it composes with
[D130](#d130--a-role-is-a-named-policy-object-users-are-assigned-to-one-never-configured-individually)'s rule
that editing a role changes every member: the confirmation has two halves, **whom** it affects
and **what** it changes.

### 4. The escape hatch is a tag, not a list of names

**Corrected by the project owner mid-decision**, and the correction is the better design: when
no existing metadata separates the offenders, the admin **adds his own metadata to the item**
rather than naming it in the rule. *"That's better than putting bare names in the rules because
it will immediately scale to all rules for all roles."*

Exactly so, and it is worth spelling out how much it buys:

- **One tag, every role.** Tag a film `scary` once and every role that excludes `scary` picks
  it up — now and in the future, including roles that do not exist yet. A deny list is the same
  judgement retyped per role, and it is the per-user-predicate mistake
  [D130](#d130--a-role-is-a-named-policy-object-users-are-assigned-to-one-never-configured-individually)
  already corrected, reappearing one level down.
- **There is no second mechanism.** The predicate language stays the only way to express a
  restriction ([principles §4](../principles/README.md)), and
  [D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable)
  already made the field registry *"extensible at runtime — user tags"*. The escape hatch turns
  out to be a feature we had.
- **The judgement survives us.** A tag is user-authored data about the **work**, so by
  [D98](#d98--a-sidecar-carries-what-is-true-about-the-work-the-database-carries-what-is-true-about-a-person)
  it is sidecar-able where a writable path is granted, it rides
  [D8](#d8--a-change-journal-and-delta-sync-cursor-exist-in-the-first-schema)'s change journal,
  and a full rescan must not destroy it. A deny list buried in a role has none of those
  properties. **A parent's classification of their own library is exactly the kind of work that
  should outlive this project.**
- **And it closes the loop in §5.** An item is queued because the rules cannot judge it; the
  remedy is to **give the rules something to judge**, not to grant an exception. The
  categorisation queue and the tagging mechanism are the same mechanism, which is why neither
  needs a separate UI.

**Two properties that are easy to get wrong.** The tag attaches at the **Title**
([D79](#d79--four-levels-not-three-title--edition--version--file-with-collections-orthogonal)),
because *"this film is scary"* is a statement about the work rather than about a cut or an
encoding — the same level favourites and user ratings attach at. And it is keyed on the item's
stable identity, which
[D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index) already
requires to survive a move, a rename, a re-rip and a provider change: a classification a
rescan silently drops is worse than none.

**The one safeguard this needs.** A free-text tag field produces `scary`, `Scary` and `scarey`,
and then a rule silently under-matches — which fails **open**, in the subsystem where that is
least acceptable. So the admin **picks from existing tags by default and creating a new one is
a deliberate, distinct act**. The registry's `alias` column
([D75](#d75--one-predicate-language-three-bindings-a-typed-field-registry-is-the-schema-deliverable))
already handles the rename case; this handles the duplicate case, and it is a UI affordance
rather than a schema change.

**What does *not* become a tag: anything true about a person rather than the work.** *"Alice
had a nightmare about this one"* is not a property of the film and must not be written beside
it in a shared media tree. That stays the **per-user deny** from
[D130](#d130--a-role-is-a-named-policy-object-users-are-assigned-to-one-never-configured-individually),
which may only narrow. The line is
[D98](#d98--a-sidecar-carries-what-is-true-about-the-work-the-database-carries-what-is-true-about-a-person)'s,
reused without modification: **a sidecar carries what is true about the work; the database
carries what is true about a person.**

### 5. The default inverts: unjudgeable is denied

The owner's requirement: *"anything new that's not covered by a rule must be restricted until
the admin categorizes it."* Taken literally, and it needs one definition to be implementable:

> An item is **covered** for a role when **every field that role's predicate references
> resolves to a known value** for that item. If any referenced field is unknown, the item is
> not covered — and for a restricted role it is **denied, and queued for categorisation**.
Categorising it means **tagging it** (§4), not approving it: the item leaves the queue because
the rules can now judge it, and that judgement then applies to every role at once.

**This supersedes the "household's explicit choice" half of
[D84](#d84--certifications-are-system-code-with-a-per-system-order-a-kids-profile-is-a-grantee-plus-a-predicate) and
[D131](#d131--a-restriction-is-a-vector-not-a-ceiling-templates-are-seeds-and-they-say-what-they-hid)** for restricted roles. There is no toggle that shows everything
unknown, and removing it is the point: if the toggle exists, the categorisation loop is never
forced and the subsystem decays into the thing it replaced. The remedy for an uncategorised
item is to categorise it.

**The unrestricted `Household` role is unaffected**, because it references no restricted fields
and therefore covers everything trivially. The rule bites exactly where it should.

**And the reconciliation that keeps this usable, because the naive reading does not:** *rules
do the bulk and the queue holds the residue.* "Rating at or below 12 in our system" positively
admits thousands of items in one statement — the admin is not approving three thousand films
one at a time, they are judging the handful the rules cannot. If the queue is large, that is
information: the rule set is not expressed over fields the library actually carries.

**This is the exact inverse of the incumbent's default, which is worth stating because it is
the whole disagreement.** Jellyfin's `UserPolicy()` constructor sets `EnableAllFolders = true`,
`EnableAllChannels = true`, `EnableAllDevices = true`, `EnableRemoteAccess = true`,
`EnableContentDownloading = true` and `EnablePublicSharing = true`, and
`UserEntityExtensions` adds `Permission(EnableAllFolders, true)` to every new user. So a new
user sees every library, and **a newly added library becomes visible to everyone
automatically**. Ours: a new item is visible to a restricted role only once something says it
may be.

### 6. The failure mode this creates, named in advance

**A quarantine nobody drains.** If the queue is invisible or large, the children's library
quietly stops growing and the household concludes the server is broken. Three things keep it
alive:

- **It is a state with a count, never a stream of notifications.**
  [D118](#d118--the-scan-is-incremental-prioritised-and-steerable-and-playability-comes-first)
  makes items appear during the first scan, so a per-item alert would mean thousands of them on
  day one. One number, where the admin already looks
  ([observability.md](../design/plans/observability.md),
  [D91](#d91--a-metrics-endpoint-off-by-default-and-local-when-on)).
- **Bulk categorisation is the normal operation**, not a power-user path. Initial setup *is*
  the bulk case.
- **A stale queue is a reportable condition**: "14 items have been waiting 30 days" is the
  signal that a household has stopped maintaining a role, and it should surface before a child
  notices.

**Status:** Unresolved — whether a restricted user is told that items are pending approval.
*"Three new things are waiting for Dad"* is useful for a child and is exactly what
[sharing-model.md](../design/plans/sharing-model.md) already lists as open for a grantee
(*"whether a grantee sees that the library is filtered — honest, and it tells them what to ask
for"*). Likely a per-role setting, and it should be decided in one place for both. Also
unresolved: whether rule suggestion ranks by precision, by coverage, or by a blend — it wants a
real library and a real admin to tune against, and guessing now would be the same mistake
[D131](#d131--a-restriction-is-a-vector-not-a-ceiling-templates-are-seeds-and-they-say-what-they-hid)
avoided by deferring the template set.

---

## D133 — The pending-items alert, which is how the queue actually drains

**Decided** (2026-09-21), by the project owner. Resolves the open question in
[D132](#d132--authoring-a-role-is-a-preview-loop-and-an-item-the-rules-cannot-judge-is-denied-not-shown)
— *whether a restricted user is told that items are pending approval* — and settles the
disclosure half of [sharing-model.md](../design/plans/sharing-model.md)'s equivalent question.

**Yes, and the reasoning is better than the one I had.** D132 named *"a quarantine nobody
drains"* as the failure mode and answered it with a metric. A metric reaches the admin who is
already looking at a dashboard, which is the wrong person: the one with the incentive is the
one being blocked. The owner's version, verbatim:

> *"It makes sense for restricted users because then they say 'daaaaad! Rate the movie so I can
> watch it!!!'"*

**That is the drain mechanism, and it is self-sustaining in a way a dashboard is not.** A queue
whose growth is felt by somebody gets emptied.

### Two audiences, two messages

| Role | Message | Action |
| --- | --- | --- |
| Unrestricted / admin | *"28 items need rating"* | rate them — they can already watch them, so this is housekeeping |
| Restricted | *"28 items are waiting to be rated"* | **ask** |

~~**Give the ask a button.**~~ **Withdrawn by the project owner** — see
[D134](#d134--a-pending-item-discloses-nothing-a-rule-shows-its-coverage-and-the-home-page-computes-nothing):
*"families already have their communication mediums and we don't need to add another."* The
shouting **is** the mechanism, and it already works.

### The line that makes this safe to show anyone

**The alert counts items pending categorisation. It never counts items a rule deliberately
excluded.**

That distinction is the whole of the disclosure question, and collapsing it would be the bug:

- *"28 items are waiting to be rated"* is a **queue state**. It says the household has work to
  do. Harmless, useful, and it is what makes the nag work.
- *"28 items are hidden from you"* is a **filter disclosure**. It tells the viewer that a
  deliberate exclusion exists and invites them to ask about it.

For a child the two are nearly the same and both are fine. **For a grantee they are not**: the
deliberately-excluded set is the household's private material — home video of the children
([D131](#d131--a-restriction-is-a-vector-not-a-ceiling-templates-are-seeds-and-they-say-what-they-hid)) — and announcing its existence to a relative invites exactly the
conversation the exclusion existed to avoid. So the count is over *pending* only, for everyone,
with no per-role exception needed to make it safe.

[sharing-model.md](../design/plans/sharing-model.md)'s *"whether a grantee sees that the
library is filtered"* stays open, but it is now a **separate and much smaller question**,
because the useful half has been answered without touching it.

### It is suppressed during the first scan

During initial setup every item is uncategorised, so the alert would read *"3,500 items are
waiting"* — which is noise, not a nag, and it trains the household to ignore the surface
permanently. [D118](#d118--the-scan-is-incremental-prioritised-and-steerable-and-playability-comes-first)
makes items appear throughout the first scan, so this is not an edge case, it is day one.

So: **the alert is suppressed while a root's first scan is in progress**, and initial bulk
categorisation is a setup step rather than an alert. After that, 28 is a real number about a
real week.

### Measured: it is a home-page number, and the obvious version misses the budget

The alert renders on the screen every user sees at launch, so it lands under
[D113](#d113--a-target-is-a-percentile-at-a-stated-concurrency-at-two-library-sizes-against-a-frozen-baseline)'s
home-page budget. Measured on the 100,000-title database, 7,692 titles uncovered (7.7%):

| Implementation | p50 | max |
| --- | --- | --- |
| Anti-join, no supporting index — plan is `SCAN` | **did not complete in 30 s** | — |
| Anti-join + covering index on `(system, title_id)` | **43.6 ms** | 57.6 ms |
| **Maintained counter + partial index** | **0.063 ms** | 0.064 ms |

**The indexed query already misses the 50 ms library-page target at a single reader**, before
D113's *"p99 at 5 concurrent sessions"* is applied — and this is one role, where a household
has several and an admin's view wants all of them.

So **the count is a maintained counter, not a query**: updated when an item is ingested and
when its metadata changes, one per role, which is single-digit work per change because roles
are single-digit in number. **692× faster than the indexed query**, and the difference between
a feature and a regression.

**This is the third time.** D113 found two home-page queries that scale linearly — recently-added
at **807×** and FTS `bm25` ranking at **686×**. This is a *new* one, arriving with a new
feature rather than found in an audit, which reframes that finding: **it is not a list of two
bugs, it is a hazard that every new home-page surface rediscovers.** Any number rendered on
that screen is a maintained value until measured otherwise.

**Status:** ~~Unresolved — whether the ask button carries a message from the requester…~~
**Moot**: [D134](#d134--a-pending-item-discloses-nothing-a-rule-shows-its-coverage-and-the-home-page-computes-nothing)
removed the button, and with it the text channel from a low-trust principal to a high-trust one
that would have inherited [bug-reports.md](../design/plans/bug-reports.md)'s question. The
feature that was not built is the security question that does not need answering.

---

## D134 — A pending item discloses nothing, a rule shows its coverage, and the home page computes nothing

**Decided** (2026-09-21), by the project owner. Three corrections to
[D133](#d133--the-pending-items-alert-which-is-how-the-queue-actually-drains) and
[D132](#d132--authoring-a-role-is-a-preview-loop-and-an-item-the-rules-cannot-judge-is-denied-not-shown),
and one generalisation that becomes [principles §11](../principles/README.md).

### 1. No ask button

*"Families already have their communication mediums and we don't need to add another."*

Correct, and it is the cheaper answer to the problem D133 was solving: **the shouting already
works.** Building a request channel would add a surface, a notification path, and a
moderation-shaped question — and it competes with WhatsApp, which the household is already
using and which we will not beat.

The bonus is that **the security question disappears with the feature.** D133 left open what a
restricted user may put in a message to an admin, inheriting
[bug-reports.md](../design/plans/bug-reports.md)'s trust question. There is now no message.

### 2. A pending item discloses nothing at all — and the reason is stronger than privacy

D133 said the alert counts pending items and not rule-excluded ones. That is necessary and it
is **not sufficient**: the count must also not become a *list*.

**"28 items are waiting to be rated" must never become "Shrek 2, Wing and 26 others are
waiting."**

The obvious argument is that it leaks the library's contents to a child or a grantee. The real
argument is worse, and it is the one to write down:

> **An unrated item is one we cannot vouch for, and that includes its title.**

A title is derived from a filename or from a provider match on a filename
([identification.md](../design/plans/identification.md)). For an item nobody has classified,
the title is **itself unvetted** — the file may be mislabelled, may be something the household
would never allow, and may be explicit in the name alone. Rendering it on a child's home screen
is a route to showing them precisely the thing a rating would have blocked, **through the
surface built to protect them.**

So, operationally — and this is [principles §10](../principles/README.md), omission rather
than rejection, applied to a scalar:

- the alert renders **a number and nothing else**;
- a pending item is **absent from that role's manifest entirely** — not listed, not searchable,
  not in Recently Added, no artwork, no title, and **not addressable by id**;
- there is no "show me what's waiting" affordance for a restricted role, because there is
  nothing safe to show.

The admin's categorisation queue does show titles, and that is fine: the admin is unrestricted
and can already see every one of those items.

### 3. A rule shows how many titles it currently covers

On the rules page, each rule carries its **current coverage count**. Cheap, and it answers the
two questions an author actually has: *is this rule doing anything?* and *is it doing far too
much?* A rule matching 0 items is usually a typo or a field the library does not carry; a rule
matching 3,400 of 3,500 is usually not what anyone meant.

It pairs with [D132](#d132--authoring-a-role-is-a-preview-loop-and-an-item-the-rules-cannot-judge-is-denied-not-shown)'s preview without duplicating it: **the preview shows the delta of a
change, this shows the standing state.** Together they answer "what will this do?" and "what is
this doing?", which are different questions and are both asked.

### 4. The generalisation: the home page computes nothing

*"The home page generally shouldn't be spawning potentially expensive queries that can easily
be cached — things that only change on rare events like scans or rule changes."*

This is the rule the last three findings were instances of, and stating it as a rule is worth
more than the three fixes:

| Home-page value | Naive | Maintained | Found by |
| --- | --- | --- | --- |
| Recently added | 21.79 ms | 0.027 ms — **807×** | [D113](#d113--a-target-is-a-percentile-at-a-stated-concurrency-at-two-library-sizes-against-a-frozen-baseline) audit |
| FTS `bm25` ranking | 11.66 ms | 0.017 ms — **686×** | D113 audit |
| Pending-items count | 43.6 ms (or no completion) | 0.063 ms — **692×** | [D133](#d133--the-pending-items-alert-which-is-how-the-queue-actually-drains), attached to a *feature* |
| Rule coverage count | — | — | this decision, **before** it was built |

**The fourth row is the point.** Three of these were discovered; the fourth was prevented,
because the rule now exists.

**And the invalidation story is already built.** These values change only on rare, known events
— a scan completing, a rule or role changing, a tag edit, a metadata refresh — and
[D8](#d8--a-change-journal-and-delta-sync-cursor-exist-in-the-first-schema)'s change journal
*already says exactly what changed*. [rules-engine.md](../design/plans/rules-engine.md) already
re-evaluates live rules off it with a debounce. So this is **event-driven invalidation on a
mechanism that exists**, not a TTL cache and not new machinery — which is what makes the rule
cheap enough to be absolute rather than aspirational.

**Status:** The one case this rule has to be careful about is a value that is *per user* rather
than per library — Continue Watching is maintained per user and there may be many users,
so "maintain it" is a different cost there. It was measured flat (0.055 ms) so it does not need
this today; the rule's boundary is where it would.

---

## D135 — Casting on your own is a device delegation, not an invitation; the credential is the session, and the queue is Next Up

**Decided** (2026-09-21), by the project owner. Fills the gap between
[D88](#d88--casting-is-remote-control-of-another-client-which-is-watch-together-with-a-group-of-one)
(*casting is Watch Together with a group of one*) and
[D124](#d124--the-playback-role-may-be-a-cast-receiver-and-for-a-guest-that-is-the-best-path-the-phone-controls-it-does-not-relay)
(*the playback role may be a cast receiver*), neither of which covered the ordinary case:
**casting to your own television with no session and nobody else involved.**

### The principal is different, and that answers most of it

The owner's instinct — *"basically the same URL approach as watch together, but for your cast
target only"* — is right about the mechanism and worth separating from the model:

| | Watch Together link | Cast URL |
| --- | --- | --- |
| Admits | **a person who has no account** | **nothing** — a device, on behalf of a logged-in user |
| Why it exists | there is no other way to identify them | the receiver pulls its own bytes ([D124](#d124--the-playback-role-may-be-a-cast-receiver-and-for-a-guest-that-is-the-best-path-the-phone-controls-it-does-not-relay)) and needs a credential to do it |
| Security rests on | entropy plus revocation, because nothing else exists | **scope and expiry** — the human is already authenticated |

So this is not a sharing grant. It is a **delegation to one device for one playback session**,
and it should be scoped like one: it grants *stream this session's media*, never *browse this
library*.

### On entropy: necessary, and not the threat

The owner asked for enough entropy that attackers cannot guess the URL. Yes — a CSPRNG token,
and it belongs in the same place as every other signed-URL question
([threat-model.md](../design/plans/threat-model.md)). **But guessing is not the attack this
surface has**, and the reason is specific:

- **Cast's sender channel is unauthenticated.** The device-authentication handshake on
  `urn:x-cast:com.google.cast.tp.deviceauth` verifies that the *receiver* is a genuine
  Chromecast; **nothing authenticates the sender.** Any device on the LAN may connect to
  port 8009 and speak the protocol.
- **`MEDIA_STATUS` carries `contentId` and `contentUrl`.**

**Therefore an attacker on your network does not guess the URL — they read it off the
television.** Entropy defends against off-LAN guessing and against a URL turning up in a log or
a screenshot, both real; it does nothing about the nearest adversary.

Which sets the actual requirement, and it is a bound rather than a secret: **whoever lifts that
URL gets the thing currently playing on a screen they can see, and nothing else.** Not the
library, not addressable by id, not enumerable — which is
[D122](#d122--the-session-is-the-object-what-plays-is-mutable-state-on-it-and-a-guest-sees-the-stream-rather-than-the-library)'s
bound arriving a second time, for a second reason. It also means
[D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible)
still holds: sign the **session**, not the segment, so segment paths stay cacheable.

### Expiry binds to the session, not to the video

*"The URL would expire after the video ends?"* — nearly, and the exact version matters because
the near-miss is broken in two ways: with a queue the video ends **N times**, and a television
can sit on a paused end-card for a week.

**The credential is the cast session**, which is
[D119](#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak)'s
distinction mapped exactly: *a link admits, a membership persists* — **the cast session is the
membership.** It ends on the first of:

- the receiver reporting the session stopped or finished;
- **supersession** — a new cast to that receiver, which is the owner's *"expire after the user
  casts something else to that TV"* and is free, because a Cast `LOAD` replaces what is there;
- an idle timeout, for the paused end-card;
- an absolute cap, for the television nobody turned off.

### Next and previous: yes, and the Default Media Receiver already does it

Verified against Google's documentation:

- **`QUEUE_NEXT` and `QUEUE_PREV` are defined media commands** in
  `urn:x-cast:com.google.cast.media`.
- **The Default Media Receiver supports a queue of audio or video streams** — only *image*
  queues are unsupported. So skip-to-next is not a custom-receiver feature.
- **`QueueBase.fetchItems` does lazy, windowed loading**, called by the receiver when it needs
  more items. That matters, because the media namespace caps a message at **64 KB**: a
  twenty-four-episode season with artwork and metadata does not ship in one `QUEUE_LOAD`, and
  it does not have to.

### The playlist already exists — it is Next Up

*"Auto-choose the current series as the playlist"* needs no new concept. A series is a
**Collection** ([D79](#d79--four-levels-not-three-title--edition--version--file-with-collections-orthogonal)); *what comes next* is
[play-state.md](../design/plans/play-state.md)'s **Next Up**, defined over
[D96](#d96--an-episodes-position-is-ordering-season-number-and-a-series-declares-its-orderings)'s
ordering-in-use. The cast queue is Next Up, extended past one item. Two people in one household
casting the same series in different orders already works, because D96 made the ordering
explicit.

**And this is where [D122](#d122--the-session-is-the-object-what-plays-is-mutable-state-on-it-and-a-guest-sees-the-stream-rather-than-the-library)'s deferred play sets arrive.** D122 put
*"the host selects a set of videos… binge-watching without handing over the library"* in a
future update for Watch Together. Casting needs it **now**, so the concept ships here and Watch
Together inherits it. The deferral had the consumers backwards, which is worth noticing
because it is the second time a Watch Together "later" turned out to be a casting "now"
(the first was [D124](#d124--the-playback-role-may-be-a-cast-receiver-and-for-a-guest-that-is-the-best-path-the-phone-controls-it-does-not-relay)'s receiver).

### The queue is a manifest, and §10 governs it

Easy to miss, and it reaches a surface nobody had considered: **the queue is sent to the
receiver and rendered on a television**, so it is a manifest under
[principles §10](../principles/README.md).

- It contains only what the **casting user's role** permits
  ([D130](#d130--a-role-is-a-named-policy-object-users-are-assigned-to-one-never-configured-individually)) —
  a child casting a series must not be handed the episodes their role hides.
- **A pending, unrated item must not appear in it — including its title**
  ([D134](#d134--a-pending-item-discloses-nothing-a-rule-shows-its-coverage-and-the-home-page-computes-nothing)).
  The queue carries metadata that goes on the screen, and the title of an unclassified item is
  itself unvetted. This is the pending-disclosure rule reaching the one surface that was not
  considered when it was written.

And **the receiver still probes and picks its own rendition**
([D124](#d124--the-playback-role-may-be-a-cast-receiver-and-for-a-guest-that-is-the-best-path-the-phone-controls-it-does-not-relay),
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)). The sender says what
to play and never in what form.

**Status:** Unresolved — whether a cast session survives the sender leaving the network. It
should, because the receiver pulls its own bytes, but the *control* path disappears with the
phone and "the TV is playing and nothing can stop it" is a bad state. Also unexamined: AirPlay
and Fire TV have their own queue semantics, and D124 already noted "cast" is three protocols;
whether Next Up maps onto all three is not known.

---

## D136 — Control belongs to the cast session, not to the sender; and an externally-paused receiver must say so

**Decided** (2026-09-21), from the project owner's question: *"when the TV is being cast to,
the TV's own controls can't affect playback?"* Resolves the open item in
[D135](#d135--casting-on-your-own-is-a-device-delegation-not-an-invitation-the-credential-is-the-session-and-the-queue-is-next-up)
— *whether a cast session survives the sender leaving*.

**They can, partially, and it depends on what "the TV" is.** Three device classes that get
conflated and behave differently:

| What is receiving | Its own controls | Why |
| --- | --- | --- |
| **Chromecast dongle** on any television | **play / pause only**, TV-dependent | HDMI-CEC "Deck Control" — the dongle has no IR receiver |
| **Chromecast with Google TV** / Google TV Streamer | **everything** | it ships its own remote and is an Android TV device |
| **Chromecast built-in television** (Sony, Vizio…) | **everything** | the television *is* the receiver |

For the dongle the mechanism is **HDMI-CEC Deck Control**, which can in principle carry
Play/Pause/Stop/Rewind/Fast-Forward, and in practice was reported at launch to deliver only
**play and pause**. It also requires the television to implement CEC, sometimes only on one
HDMI port, *and* to implement Deck Control specifically — many do not.

> **Dated and UNVERIFIED.** The "play and pause only" limitation is from reporting of the
> March 2015 Chromecast update and has not been re-checked against current firmware. Treat the
> *shape* as reliable — CEC, partial, television-dependent — and the *specifics* as stale until
> the [device lab](../design/plans/device-lab.md) measures them, which is exactly the kind of
> per-device fact it exists for.

### But CEC is not the answer, and the real one costs nothing

**Cast supports multiple senders on one session**, natively: *"one Sender could start a session
on a Receiver and another Sender could join the same session to control playback, load new
content, or queue more content"*, by issuing the same `CONNECT`. There is a listener for a media
session created by a different sender.

So the answer to *"the phone left and nothing can stop it"* is not a remote control — it is
that **control was never the phone's**:

**The cast session holds the control right, and any authorised device may attach to it.** That
is [D120](#d120--a-participant-is-a-person-devices-attach-to-them-with-roles-and-the-cap-counts-people)
unchanged — *a participant is a person; devices attach to them with roles*, and `control` is
one of the three. A second phone, a tablet, the web client on a laptop: each attaches to the
running session and controls it. [D135](#d135--casting-on-your-own-is-a-device-delegation-not-an-invitation-the-credential-is-the-session-and-the-queue-is-next-up)
already made the cast session the membership; this is what that buys.

**So a sender leaving is not a stuck television.** It is a membership with no `control` device
attached, and the remedy is to attach one — which any device the user is already signed in on
can do. No new mechanism, and it is better than the CEC path because it works on every
television rather than the ones whose manufacturer implemented Deck Control.

### The consequence nobody would notice until it shipped

If the television *can* pause — by CEC, by its own remote, or because someone pressed a button
on a Google TV — then **playback state changes without our sender having asked for it**, and
the receiver is the only thing that knows.

**It must report that upward, or the session's idea of what is on screen silently diverges from
what is on screen.** This is [principles §9](../principles/README.md) exactly: the receiver is
the sensor, it saw something, it reports it, and the core decides what it means. A receiver
that swallowed a local pause would leave the group state saying `Playing` over a paused
picture, which is the sort of bug that gets diagnosed as "sync is broken".

**And it needs no seventh method**, which is worth checking explicitly because
[D88](#d88--casting-is-remote-control-of-another-client-which-is-watch-together-with-a-group-of-one)
asked for precisely this verification — *"the claim to verify is that six methods are still
enough for a case they were not designed against"*. The answer is yes, because the vocabulary
already exists: an externally-initiated pause is a **suspension with a reason**, alongside
[watch-together.md](../design/plans/watch-together.md)'s `stallRecovery`,
`audioSessionInterrupted`, `userActionRequired` and `userIsChangingCurrentTime`. Add
`userPausedLocally` to that enum and the six-method surface is untouched.

That also settles a subtler thing: **a local pause must not be inferred from a stalled
position.** It would otherwise look identical to a buffering stall, and the reconciler would
fight it — the same failure [D90](#d90--playback-presentation-is-a-persisted-per-user-preference-set-and-captions-have-a-statutory-floor)
identified for a user-chosen playback rate, where a deliberate act read as drift makes the
system spend the film correcting the user.

**Status:** The device lab owes three facts per television: whether CEC reaches the receiver at
all, which commands survive, and whether the receiver can distinguish a CEC pause from a
sender pause. The third is the one that decides whether `userPausedLocally` can be reported
accurately or only inferred.
