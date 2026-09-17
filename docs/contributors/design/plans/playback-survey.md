> **ABOUTME:** The cheap measurement that decides how urgent transcoding is — what fraction of a
> real library direct-plays on real clients, and what the failures actually are.

# Playback survey

- **Status:** PLANNED — designed 2026-09-17. Gates the transcoding schedule.
- **Depends on:** —

## Why this, and why first

D17 bets that direct play covers the common case and transcoding can be deferred. That bet rests
on a number **nobody has published**: the real-world direct-play rate. Tautulli exposes it
per-server; no aggregate survey exists. Measuring it is days of work and it re-orders the entire
roadmap in either direction.

This replaces the original `transcode-core-spike` as Phase 0. That plan ordered the segmented
transcoder first because it was the riskiest assumption; measuring the assumption is cheaper
than building against it.

## What to build

A throwaway tool — not part of the product — that:

1. Walks a real library and `ffprobe`s every file into `{container, video codec/profile/level/
   bit depth/HDR type, audio codecs/channels, subtitle formats, attachments}`.
2. For each target client environment, evaluates whether that combination is decodable, using
   the client's own runtime probe rather than a table: `mediaCapabilities.decodingInfo()` in a
   browser page, `MediaCodecList` in a tiny Android app, and libmpv's own reporting on desktop.
3. Reports the direct-play rate per client, and — more usefully — **a ranked list of the
   specific reasons for each failure**.

## What it must answer

- **What fraction direct-plays**, per client, with the platform decoder alone?
- **What fraction direct-plays with libmpv in the client?** The delta between these two numbers
  is the value of D10's two-engine recommendation, quantified. It may be most of the problem.
- **What are the actual failure reasons**, ranked? If the tail is "old DivX/XviD/VC-1" that is a
  one-off compatibility-transcode job. If it is "every 10-bit HEVC file on a Chromecast" it is a
  structural problem that changes the plan.
- **How much of the tail is audio-only** — video fine, TrueHD/DTS unsupported? That is cheap to
  fix and does not need a video encoder at all.
- **What does a bandwidth-constrained client actually need?** Measure real bitrates against real
  remote link speeds rather than assuming.
- **Does every target platform handle demuxed HLS?** [D22](../../decisions/working-notes.md)
  negotiates video and audio as separate axes, which means separate HLS renditions
  (`EXT-X-MEDIA:TYPE=AUDIO` + `GROUP-ID`). Older Tizen, webOS and Roku firmware are *reported* to
  be less reliable with demuxed than with muxed audio+video — unverified, and it is the one
  assumption D22 rests on. Test it on real devices. If it fails on a platform we care about, the
  fallback is a muxed variant flagged on the video axis, and that concession has to be reviewed
  against D21's no-grammar rule.
- **Which audio profiles does each device's *audio path* accept?** Not the device — the path.
  Test HDMI/ARC into a receiver, the TV's own speakers, and Bluetooth, because D22 claims these
  answer differently and that separating the axes is what makes a mid-session change cheap.

## Corpus

Not synthetic, and deliberately unrepresentative in one direction: the real library **plus** the
old and awkward files, so the tail is visible rather than averaged away. H.264 and HEVC, 8-bit
and 10-bit, HDR10 and Dolby Vision, MKV/MP4/AVI and a disc rip, TrueHD/DTS-HD/E-AC-3/AAC,
embedded ASS with attached fonts, embedded PGS, external SRT, a multi-part file.

## Done when

Numbers are written into `../research/playback-survey-results.md`, and D17 is either confirmed
or superseded. If the direct-play rate is high, transcoding moves to Phase 4 as planned. If it
is low, the segmented transcoder returns to Phase 1 and the roadmap is rewritten.
