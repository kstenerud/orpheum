> **ABOUTME:** The playback API — manifest generation, direct play, the transcode session
> registry and the subtitle pipeline — promoted from the spike into the server.

# Stream service

- **Status:** PLANNED — designed 2026-09-17.
- **Depends on:** library-model.md

## Scope — direct play, plus audio

Per [D17](../../decisions/working-notes.md) this service ships **no video encoder**. Per
[D22](../../decisions/working-notes.md) it *does* ship audio encoding, because every GPL trigger
in FFmpeg is a video encoder and the whole audio menu is native LGPL — so audio costs us nothing
in licence or patent surface, and it covers the most common real transcode (video fine, audio
TrueHD or DTS-HD).

It provides:

- ranged direct-play `GET` of the original file;
- the **rendition-list endpoint**, answering "what can I play this as" with, initially, one
  entry;
- the subtitle pipeline (D11) — extraction at scan time, format conversion, font attachments
  served as addressable resources;
- trickplay sprite generation and chapter data;
- playback-progress reporting;
- **audio transcoding and per-axis remuxing** — copying a video track into fMP4 segments needs a
  muxer and no encoder, so it belongs here too.

FFmpeg is present for `ffprobe`, subtitle extraction, thumbnailing, remuxing and audio encoding.
None of that needs `--enable-gpl`, which is what keeps this component on the LGPL rung
([D16](../../decisions/working-notes.md), [D22](../../decisions/working-notes.md)).

## The seams that keep transcoding additive

These must exist in v1 even though nothing uses them yet. They are the whole reason
[`transcoding.md`](transcoding.md) can be a later, separate component rather than a rewrite:

- **The rendition list is a list.** Growing it requires no client change, no protocol version
  and no migration (see [tech-playback.md](../research/tech-playback.md) §2a).
- **The output menu and its negotiation exist in v1** ([D21](../../decisions/working-notes.md)),
  even though the only producible entry is `original`. The client sends its prioritised subset of
  the named menu; the server intersects. Getting this shape in early is what lets a transcoder be
  added later as a pure capability announcement, and it is what lets dumb clients ship a static
  list instead of implementing runtime probe-and-pick.
- **A transcode-reason field**, populated even when the answer is "this server cannot
  transcode". Clients must be able to explain the failure — principles §3.
- **A capability handshake**, so a core without the transcoding component says so honestly.
  This path has to exist anyway for hardware too weak to transcode.
- **Video, audio and subtitles are separate negotiation axes** from the start
  ([D22](../../decisions/working-notes.md)) — separate HLS renditions via `EXT-X-MEDIA` groups.
  This is what makes audio-only transcoding expressible, keeps mid-playback audio-track switching
  from disturbing the video, and keeps the menu additive rather than multiplicative.

## Done when

A client direct-plays everything the playback survey says it should; a client that cannot decode
TrueHD or DTS-HD still plays the film with its video untouched and its audio transcoded; ASS
subtitles render correctly with their embedded fonts; resume is correct; and anything that still
cannot play says why.
