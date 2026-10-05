> **ABOUTME:** Video transcoding as a media-node capability — the compatibility path and the
> bandwidth path, which are different problems, built in that order and only once measured, on
> a machine that need not be the one serving the files. Audio transcoding runs in the same node
> job, not in the core.

# Video transcoding

- **Status:** PLANNED — designed 2026-09-17. Scheduled after the clients, pending measurement — **except that §(b) is now on remote access's critical path** ([D86](../../decisions/working-notes.md), 2026-09-19), and §(a) gained a second consumer ([D87](../../decisions/working-notes.md)). **Reshaped for Cut A 2026-09-23** ([D193](../../decisions/working-notes.md)): the encoder is a **node capability**, not a tier-3 plugin (retired, §3); §(a) arrives first as Cut A's **prepare** job, which needs a video encoder for the `h264-8m` profile (Quick Sync, NVENC or x264, from a pinned jellyfin-ffmpeg); §(b), live bandwidth transcoding, is still later. [cut-a.md](cut-a.md) §6 is the mechanism.
- **§(b) is being built** for [D247](../../decisions/working-notes.md) (2026-10-05): lower
  qualities cut at the source's keyframes and made on request; [chunks.md](chunks.md) is the plan.
- **Depends on:** playback-survey.md, stream-service.md

## Scope: video, and where audio went

**Audio transcoding is not part of this plan**, but ~~it lives in the core~~ it does not live in
the core either: the core never runs FFmpeg ([D26](../../decisions/working-notes.md),
[D193](../../decisions/working-notes.md) §1). It runs in the node's **prepare** job — one FFmpeg
command per (File, profile) that copies or encodes the video, converts the audio (with a
dialogue-boost variant) and writes a growing HLS `EVENT` playlist
([stream-service.md](stream-service.md)). Every GPL trigger in FFmpeg is still a video encoder
and the entire audio menu (AAC, AC-3, E-AC-3, FLAC, ALAC, Opus) is producible from a plain LGPL
build ([D22](../../decisions/working-notes.md),
[tech-licensing.md](../research/tech-licensing.md) §3).

The split that survives: since the most common real-world transcode is audio-only — video fine,
audio TrueHD or DTS-HD — **prepare's `remux` profile handles the common case on any node, and a
node's video encoder handles the rare, heavy, licence-encumbered one.** A node with no encoder
can already fix the TrueHD-on-a-cheap-TV problem, which is most of what users actually hit.

## Position

~~The video transcoder is **a plugin we happen to maintain** ([D19](../../decisions/working-notes.md)) —
a tier-3 capability service on the public plugin API.~~ **Plugin tier 3 is retired**
([D193](../../decisions/working-notes.md) §3). The encoder is a **capability a media node
advertises** — `remux`, `aac`, `h264_qsv`, `h264_nvenc`, `x264` — and the core sends a job to a
node that can do it. D19's argument (rffmpeg; the machine that encodes need not be the one that
serves) was written before the node model existed, and the node model already satisfies it.

Four things followed from the plugin framing; read them through the node:

1. ~~**It keeps the plugin API honest.**~~ Withdrawn with tier 3: there is no public
   transcoder API to keep honest. The node's job protocol is internal.
2. **The licence and patent boundary gets a mechanism instead of a convention.** The encoders
   live in the node's FFmpeg subprocess — a pinned jellyfin-ffmpeg build — never linked into the
   core (D16).
3. **The transcoder does not have to be on the same machine.** See below.
4. **The degraded path is exercised.** "This server cannot transcode video" is what an install
   answers when no node advertises an encoder, so the reason field is tested rather than being a
   rarely-taken branch.

## Remote and distributed transcoding

The capability nobody in this field has, available for free because the encoder is a node
capability. The NAS serves files; the desktop with a GPU (the owner's RTX 3090) becomes a second
node and encodes. Several nodes may advertise encoders and the core picks by capability.

**The demand is proven and the missing seam is proven with it:** `rffmpeg` — 1,073 stars, created
2019, still maintained 2025, **written by a Jellyfin founder** — is an out-of-tree wrapper that
impersonates the `ffmpeg` binary and SSHes to another host, because Jellyfin offers nowhere to
put this. Kyoo's `gocoder` is the positive case: a standalone transcoding service already reused
by two unrelated projects.

This is not extra work. It is what a node already is, provided the job protocol does not
assume a shared filesystem *by default*.

## Two components, not one

### (a) Compatibility transcoding — a library problem

The client cannot decode the file at all. Solve it **once per file**, as a node job producing
a rendition that is then reused. ~~It does not need to be fast and it must not be on the
playback path.~~ **In Cut A this is prepare** ([D193](../../decisions/working-notes.md) §2): the
`h264-8m` profile, started by pressing play, by "get this ready", or when a Watch Together
session is created. It writes a growing `EVENT` playlist, so playback starts once the first
segments exist, and an encode runs at a few times real time on Quick Sync — so remote viewing
prepares in advance. The result is a cache, not a stored Version
([D169](../../decisions/working-notes.md)), and its play state is the source File's
([D180](../../decisions/working-notes.md)).

Trigger it from the playback survey's ranked failure list, not from a guess. If the tail is
"a few hundred old DivX files", a one-off batch job is the entire feature.

**Build this first** — Cut A does. It is the simpler problem, it needs no segment alignment, no keyframe grid
and no ABR, and it matches the user's actual situation: a number of old videos that need
converting once.

**It has a second consumer, which changes its priority**
([D87](../../decisions/working-notes.md)): a **download rendition**. Two of the most-reacted
requests on the Jellyfin client trackers are *"Transcode downloads to save storage space"*
(+32) and *"Allow transcoding of multimedia content"* (+52) — people want the copy on their
phone to be smaller than the original. Same machinery, different trigger, different retention
policy, and it is often the only thing a phone user wants from a transcoder at all.

### (b) Bandwidth transcoding — a network problem

The client decodes the file fine but cannot receive it fast enough. This is the case that
genuinely requires live, on-demand, keyframe-aligned segmented transcoding with adaptive
bitrate, and it is the design researched in [tech-playback.md](../research/tech-playback.md) §3:

- produce only **video menu profiles** (D21) — never an arbitrary codec string, and always at
  least the mandatory `h264-sdr` baseline, which is what makes negotiation total on that axis;
- extract the source keyframe grid from packet-level `ffprobe` data, cached and versioned;
- use it as the segment grid for **every** rendition including passthrough, so variants are
  interchangeable;
- force IDR frames exactly there — **NVENC ignores `-force_key_frames` and needs `-force_idr 1`**;
- start transcodes at the requested segment, not from zero;
- share one encoder process between viewers of the same rendition.

**Build this second, and treat it as coupled to remote access (D14), not to the library.** On a
LAN it is almost never needed. **Not in Cut A** ([D178](../../decisions/working-notes.md) §4):
Cut A's remote viewer gets prepare's fixed ~8 Mbps rendition instead, and live
segment-addressed transcoding waits for a measurement that says first-play latency hurts
(D193 §2).

**[D86](../../decisions/working-notes.md) sharpens that from "coupled" to "required".** Remote
access is the headline feature and the paid tier, and on a degraded remote link the only lower
renditions that exist are the ones this component makes. Without it, remote playback over a
constrained link is a ranged GET of a 20 Mbps remux and a stall. So "deliberately late" is
true relative to the library and false relative to remote access — whichever of those ships
first sets this component's real deadline.

**And the segment-grid requirement has a consumer now.** "Use the source keyframe grid as the
segment grid for every rendition including passthrough, so variants are interchangeable" was
specified above with nothing depending on it. Buffer-driven adaptation depends on it: without
aligned variants a mid-film downshift is a reposition, in the one situation where a restart is
least affordable ([D23](../../decisions/working-notes.md)). Prefer AV1/Opus for output where client support allows — they are
royalty-free by design (D16).

**This also owns remote "party viewing"**, which is not a
[watch-together](watch-together.md) feature despite looking like one
([D69](../../decisions/working-notes.md)). Watch-together's cost at scale is voice, and voice is
trivial — `N(N−1)` Opus streams is 1.3 Mbps at five people and 24 Mbps at twenty. The film is
the constraint: a 1080p web-dl direct-plays at 5–10 Mbps and a remux at 20–30, so **one** remote
viewer on a remux can saturate a household uplink. Serving five remote viewers is a bitrate
problem, which is this component and nothing else. A **LAN** party costs no uplink at all and
needs none of this.

## What the core must already provide

None of this is allowed to require changes to the client contract:

- the rendition-list API shape, which simply grows entries (D17);
- a job system that can host an encode job;
- a **transcode reason** field, present in v1, so a client can display *why* — including "this
  server cannot transcode";
- a capability handshake, so a node without an encoder says so honestly. That path has to
  exist anyway for hardware too weak to transcode;
- ~~**probe results and keyframe data exposed through the public API**~~ — moot: the node that
  encodes is the node that probed;
- **signed segment URLs**, so redirecting a client to a remote node later does not create an
  open proxy. In Cut A the core relays instead of redirecting.

## The interface, and how not to design it

**Who owns the rendition list** was the whole problem, and
[D21](../../decisions/working-notes.md) mostly solved it. With a closed, named output menu the
node declares *the set of menu profiles it can produce on this hardware* — a list of ~11
possible names, narrowed by what the GPU present can actually encode — and the rendition list is
a set intersection: `client's prioritised list ∩ the nodes' capabilities ∩ {the original}`.

That is a small interface over a closed vocabulary rather than an open negotiation, and it is
checkable: a transcoder that claims `h264-sdr` and cannot produce it fails a conformance test
rather than a user's playback.

**Conformance must also cover closed captions.** CEA-608/708 captions live inside the video
elementary stream as SEI side data, and a transcode destroys them unless the side data is
explicitly carried through (FFmpeg provides `vf_ccrepack` and A53 side-data handling). A
transcoder that silently drops captions produces a bug that only appears under load, on only
some content — so carry-through is a test, not a guideline
([D23](../../decisions/working-notes.md)).

Other questions the node protocol must still answer: how a node that does not hold the file
reads it (a shared mount, or bytes streamed to it — the latter is what makes a genuinely remote
encoding node work); how capabilities are re-probed when hardware changes; and how a job dying
mid-stream is reported to the client (in Cut A: the FFmpeg exit and its stderr tail become the
job's reason, and a crash restarts the job from zero, because the output is a cache).

~~**Do not design this API up front.** Build the transcoder against a *private* interface, get
it working on real hardware, and promote the interface to public only once it has survived
that.~~ There is no public interface to promote (D193 §3); the node job protocol is internal
and specified in [cut-a.md](cut-a.md) §4.

## Hardware acceleration

Every accelerator (VAAPI, QSV, NVENC/NVDEC, VideoToolbox, AMF, V4L2) is a driver behind **one**
interface, with a startup capability probe and an explicit, logged fallback to software. No
accelerator branches outside its own implementation file — principles §5.

Ship a pinned FFmpeg and publish the configure line (D16). ~~self-built~~ **Cut A bundles a
pinned jellyfin-ffmpeg portable build, checked by hash and never downloaded at runtime**
([D193](../../decisions/working-notes.md) §6); our own hardened build is deferred (§4). This is
where correctness actually lives; Jellyfin maintains an entire FFmpeg fork largely for these
patches, and using theirs inherits them.

**The hwaccel matrix is now the dominant remaining complexity**, and D21 does not reduce it —
`EncodingHelper.cs` is 8,042 lines mostly because of per-backend filter chains, pixel-format
upload/download and tone-mapping, not because of output breadth. Note the reinforcing fact from
D21 though: every hardware encoder supports the same three codecs (H.264/HEVC/AV1; VideoToolbox
only the first two), so the menu costs nothing and each backend implements the same short list.

## Done when

The playback survey's ranked failure list is empty for path (a), and a remote client on a
constrained link plays smoothly with rendition switching for path (b).
