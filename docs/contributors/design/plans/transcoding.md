> **ABOUTME:** Transcoding as a separable optional component — the compatibility path and the
> bandwidth path, which are different problems, built in that order and only once measured.

# Transcoding

- **Status:** PLANNED — designed 2026-09-17. Scheduled after the clients, pending measurement.
- **Depends on:** playback-survey.md, stream-service.md

## Position

Transcoding is **not** part of the core server (D16, D17). It ships as a separate optional
component with the full GPL FFmpeg encoder build. The core demuxes, probes, extracts subtitles
and makes thumbnails, and needs no encoder to do any of that.

This is a packaging boundary doing architectural work: it keeps the default install small, keeps
the default install's licence and patent surface small, and mechanically prevents encoder logic
from spreading through the core — which is how `EncodingHelper.cs` reached 8,042 lines.

## Two components, not one

### (a) Compatibility transcoding — a library problem

The client cannot decode the file at all. Solve it **offline, once per file**, as a background
job producing a stored alternate version that then direct-plays forever. It does not need to be
fast and it must not be on the playback path.

Trigger it from the playback survey's ranked failure list, not from a guess. If the tail is
"a few hundred old DivX files", a one-off batch job is the entire feature.

**Build this first.** It is the simpler problem, it needs no segment alignment, no keyframe grid
and no ABR, and it matches the user's actual situation: a number of old videos that need
converting once.

### (b) Bandwidth transcoding — a network problem

The client decodes the file fine but cannot receive it fast enough. This is the case that
genuinely requires live, on-demand, keyframe-aligned segmented transcoding with adaptive
bitrate, and it is the design researched in [tech-playback.md](../research/tech-playback.md) §3:

- extract the source keyframe grid from packet-level `ffprobe` data, cached and versioned;
- use it as the segment grid for **every** rendition including passthrough, so variants are
  interchangeable;
- force IDR frames exactly there — **NVENC ignores `-force_key_frames` and needs `-force_idr 1`**;
- start transcodes at the requested segment, not from zero;
- share one encoder process between viewers of the same rendition.

**Build this second, and treat it as coupled to remote access (D14), not to the library.** On a
LAN it is almost never needed. Prefer AV1/Opus for output where client support allows — they are
royalty-free by design (D16).

## What the core must already provide

None of this is allowed to require changes to the client contract:

- the rendition-list API shape, which simply grows entries (D17);
- a job system that can host an encode job;
- a **transcode reason** field, present in v1, so a client can display *why* — including "this
  server cannot transcode";
- a capability handshake, so a core without this component says so honestly. That path has to
  exist anyway for hardware too weak to transcode.

## Hardware acceleration

Every accelerator (VAAPI, QSV, NVENC/NVDEC, VideoToolbox, AMF, V4L2) is a driver behind **one**
interface, with a startup capability probe and an explicit, logged fallback to software. No
accelerator branches outside its own implementation file — principles §5.

Ship a pinned, self-built FFmpeg and publish the configure line (D16). This is where correctness
actually lives; Jellyfin maintains an entire FFmpeg fork largely for these patches.

## Done when

The playback survey's ranked failure list is empty for path (a), and a remote client on a
constrained link plays smoothly with rendition switching for path (b).
