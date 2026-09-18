> **ABOUTME:** What Netflix, Apple, Amazon and YouTube actually do, separated into the parts
> that transfer to a single-household server and the parts that are artefacts of planetary
> scale. Written to stop us cargo-culting an architecture built for a different problem.

# Commercial Streaming: What Transfers and What Doesn't

The big services solved video delivery. Most of how they did it is irrelevant to us, and
copying it is the most likely way to over-engineer this project. This document separates the
two.

## The one decision they all share, and we should take

**Nobody encodes video at playback time.** Every commercial service encodes each title *once*,
ahead of time, into a ladder of renditions, and then serves static segments. Playback is a file
read plus a manifest.

Netflix's published engineering describes the shape: ingest and validate a high-quality
mezzanine, split it into chunks, encode the chunks in parallel, assemble, and distribute. The
ladder is chosen per title (and later per shot) by analysing content complexity rather than
applying a fixed resolution/bitrate table, with VMAF as the quality metric. As of late 2025
Netflix reported AV1 carrying roughly 30% of its streaming.

Self-hosted servers inverted this. Plex, Emby and Jellyfin transcode **live, on playback**,
which is why a home media server needs a fast CPU or a GPU at all, why transcoding is the top
operational complaint in the field, and why "will it direct play?" is a question users have
learned to care about. Kyoo's on-demand-HLS design is a middle path: still live, but segment-
addressable and shared between viewers.

**What transfers:** the principle that the expensive work should happen once, off the playback
path, and that playback should be a read. For a home server the full commercial pipeline is
absurd — but *offering* a background "prepare a compatibility rendition for this title" job,
and keeping the live path for everything else, is not. See
[tech-playback.md](tech-playback.md).

**What doesn't:** per-shot optimisation, VMAF-driven ladder search, distributed chunked
encoding, a microservice per pipeline stage. Netflix's move to microservices was driven by
build/deploy coupling in a monolith worked on by many teams. We have neither the teams nor the
scale that justifies the cost.

## The second decision: the client chooses, from a manifest

Commercial services do not maintain a database of what each client model can decode. They
publish a manifest (HLS `.m3u8` or DASH `.mpd`) listing every rendition with its codec string,
resolution and bandwidth, and the **client** picks — at start, and continuously as network
conditions change (adaptive bitrate).

This works because the client can ask the platform what it can actually decode, at runtime:

- **Web:** `navigator.mediaCapabilities.decodingInfo()` (W3C Media Capabilities) answers not
  just *supported* but *smooth* and *powerEfficient* for a given codec string, resolution,
  framerate and bitrate. That last pair is the information a server-side device profile can
  never have.
- **Android:** `MediaCodecList` / `MediaCodecInfo.CodecCapabilities` gives the same, including
  hardware-vs-software and supported profiles/levels.
- **Apple:** `AVPlayer` handles HLS natively and simply plays the renditions it can.

**What transfers:** all of it, and it is the single most important structural choice available
to us. A manifest-first, client-chooses design removes the `DeviceProfile` database, removes
an entire bug class, and makes writing a new client dramatically cheaper. Kyoo has already
validated it in exactly our problem space ([competitors-video.md](competitors-video.md)).

## The third: format convergence on CMAF

Apple's HLS authoring specification requires **fMP4** (not MPEG-TS) for HEVC, and for HDR10 and
Dolby Vision carriage. fMP4 is the same container MPEG-DASH uses, and CMAF is the profile that
makes one set of segments serve both — with CBCS common encryption letting FairPlay and
Widevine license the same bytes.

**What transfers:** target **CMAF/fMP4 segments with an HLS manifest** and we get Apple
devices, Safari, every hls.js/Shaka target, and a DASH manifest over the same segments later
for free if we ever want it. Targeting MPEG-TS instead — which is what Jellyfin's older paths
do, and the cause of several "works for 4 minutes then dies" reports — buys nothing and closes
doors.

**What doesn't:** DRM. FairPlay, Widevine and PlayReady exist to stop the user from keeping
their own copy. Our user *owns* the files. Not shipping DRM is a feature, and it removes the
single largest source of licensing, key-server and platform-certification cost in commercial
streaming. **This is the biggest reason a small team can compete here at all.**

## What explicitly does not transfer

| Their thing | Why it exists | Why not us |
| --- | --- | --- |
| CDN / edge caching (Netflix Open Connect) | Serving continents | One household, one LAN. The "CDN" is a local disk. |
| Microservice estates | Many teams, independent deploy cadence | One team. A single binary is a feature, not a limitation. |
| Kafka/Cassandra-scale data infrastructure | Billions of events | A single SQLite file handles a household forever. |
| DRM and device certification | Licensing obligations to studios | The user owns the files. |
| Recommendation ML at scale | Catalogue discovery across millions of titles | A few thousand items the user chose deliberately. |
| Per-shot encoding optimisation | Bandwidth cost at planetary scale | Bandwidth is free on a LAN. |

## What the commercial players are *worse* at, and where we win

- **They cannot play your files.** Arbitrary containers, ancient codecs, rips with ten audio
  tracks and embedded ASS subtitles with custom fonts — all outside their problem space and
  squarely inside ours. This is the hard part of our project and it is the part they never
  had to solve.
- **They do not do libraries.** Folder structures, duplicate versions, multi-part files, disc
  images, and user-corrected metadata are ours alone.
- **Their offline story is DRM-shaped.** Ours can just be a file copy.

## What this settles

1. **Manifest-first, client-chooses, CMAF/fMP4, no DRM.** Take all four.
2. **Move expensive work off the playback path** where it is cheap to do so, without
   pretending we are Netflix: the live path stays, a background-prepare path gets added.
3. **Do not import their operational architecture.** Single binary, one database, no message
   bus, no service mesh. Every one of those is a cost with no corresponding benefit at
   household scale — and boring, established tech is the stated preference for this project.
