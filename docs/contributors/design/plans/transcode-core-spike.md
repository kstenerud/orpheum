> **ABOUTME:** The gating experiment — prove that keyframe-aligned, on-demand, segment-addressed
> transcoding works on a real library on real hardware, before anything else is built.

# Transcode core spike

- **Status:** PLANNED — designed 2026-09-17. Gates every other plan.
- **Depends on:** —

## Why this first

The whole design rests on D2 + D4: the server advertises renditions in an HLS manifest and
produces segments on demand, aligned to the source's own keyframe grid so renditions are
interchangeable. That is verified as *working software* in Kyoo, but not by us, and not against
this project's constraints. If it does not hold up, the architecture changes.

## What to build

A standalone Go binary — no database, no API, no UI — that:

1. Takes a media file path and extracts keyframe timestamps from packet-level `ffprobe` data.
   Measure this against frame-decoding extraction; Kyoo reports roughly 20×.
2. Serves an HLS master playlist advertising: passthrough, plus two transcoded video renditions,
   plus separate audio rendition groups.
3. Serves segments on demand, transcoding from the requested segment rather than from zero,
   with FFmpeg forced to emit IDR frames exactly on the keyframe grid.
4. Shares one FFmpeg process between concurrent requests for the same rendition.
5. Extracts embedded subtitle tracks and font attachments and serves them.

## What it must answer

- **Does segment alignment actually hold** across passthrough and transcoded renditions, such
  that `hls.js`, AVPlayer and ExoPlayer all switch renditions without artefacts?
- **How long is the cold-start latency** from "user presses play" to "first frame", for a file
  that needs transcoding, on the target hardware? This is the number users judge.
- **What is the keyframe-extraction cost** on a 60 GB remux, and can it be made incremental so
  playback can start before it completes?
- **How much does hardware acceleration actually help**, per backend, and what breaks? Confirm
  the NVENC `-force_idr 1` trap and find the others.
- **What fraction of a real library direct-plays** on each target client without any server
  work at all? If it is very high, path (b) matters less than assumed and the design can
  simplify.

## Corpus

Not synthetic. A real library covering at minimum: H.264 and HEVC; 8-bit and 10-bit; HDR10 and
Dolby Vision; MKV, MP4, AVI and a disc rip; TrueHD, DTS-HD, E-AC-3 and stereo AAC; embedded
ASS with attached fonts, embedded PGS, and external SRT; a multi-part file; and something with
pathological keyframe spacing (Kyoo notes sources ranging from every frame to none for three
minutes).

## Done when

The questions above have measured answers written into
`../research/transcode-spike-results.md`, and either D4 is confirmed or a superseding decision
is recorded.
