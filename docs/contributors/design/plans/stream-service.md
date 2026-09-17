> **ABOUTME:** The playback API — manifest generation, direct play, the transcode session
> registry and the subtitle pipeline — promoted from the spike into the server.

# Stream service

- **Status:** PLANNED — designed 2026-09-17.
- **Depends on:** transcode-core-spike.md, library-model.md

## Scope

Turns the spike into a service: per-item HLS manifest generation from the library model,
ranged direct-play, the on-demand segment transcoder with a shared-session registry, the
subtitle pipeline (D11), trickplay sprite generation, and the hardware-acceleration interface
with capability probing and logged fallback.

## Constraints carried in

- Direct play is always advertised as a rendition (D4).
- Audio renditions are independently selectable — transcode audio, pass video through — because
  a large share of real "video transcodes" are actually a TrueHD/DTS problem.
- Every accelerator is a driver behind **one** interface. `EncodingHelper.cs` is 8,042 lines
  because Jellyfin let accelerator branches spread through the argument builder; that is the
  specific outcome to avoid.
- The server computes a transcode *reason* and exposes it, because clients must be able to show
  the user why something is transcoding.
- The segment cache is size-capped and always safe to delete.

## Done when

A client can play every file in the spike corpus, direct-play where possible, switch renditions
mid-stream, seek without restarting the transcode, and display correct ASS subtitles with
embedded fonts.
