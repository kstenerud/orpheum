> **ABOUTME:** The playback API — manifest generation, direct play, the transcode session
> registry and the subtitle pipeline — promoted from the spike into the server.

# Stream service

- **Status:** PLANNED — designed 2026-09-17.
- **Depends on:** library-model.md

## Scope — direct play only

Per [D17](../../decisions/working-notes.md) this service ships **no encoder**. It provides:

- ranged direct-play `GET` of the original file;
- the **rendition-list endpoint**, answering "what can I play this as" with, initially, one
  entry;
- the subtitle pipeline (D11) — extraction at scan time, format conversion, font attachments
  served as addressable resources;
- trickplay sprite generation and chapter data;
- playback-progress reporting.

FFmpeg is present for `ffprobe`, subtitle extraction and thumbnailing. None of that needs an
encoder, which is what keeps this component off the GPL encoder rung
([D16](../../decisions/working-notes.md)).

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
- **Audio and video renditions are separately addressable** from the start, because a large
  share of real "video transcodes" are actually a TrueHD/DTS problem, and fixing those needs no
  video encoder at all.

## Done when

A client direct-plays everything the playback survey says it should, with correct ASS subtitles
and their embedded fonts, correct resume, and an honest, displayable reason for anything it
cannot play.
