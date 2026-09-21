> **ABOUTME:** The playback API — manifest generation, direct play, the transcode session
> registry and the subtitle pipeline — promoted from the spike into the server.

# Stream service

- **Status:** PLANNED — designed 2026-09-17; **adaptation, audio presentation and the accessibility floor added 2026-09-19** ([D86](../../decisions/working-notes.md), [D83](../../decisions/working-notes.md), [D90](../../decisions/working-notes.md)).
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
- the subtitle pipeline (D11, [D23](../../decisions/working-notes.md)) — extraction at scan time
  (which is what makes mid-playback switching possible, not merely fast), format conversion,
  font attachments served as addressable resources, **closed captions decoded out of the video
  stream into a real track**, and forced/default/preferred track semantics;
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
- **Language is a preference axis, resolved server-side** ([D24](../../decisions/working-notes.md)).
  The client sends an `Accept-Language`-style prioritised list; the server resolves the chain
  (explicit → series → library → user → forced-subtitle rule → file default) and **reports what
  it pre-selected and why**, per principles §3. Capability travels per-request because it is
  device-scoped; language preference is user state and syncs.
- **Three subtitle renditions, not two** ([D78](../../decisions/working-notes.md)): text,
  **server-rendered overlay**, and burn-in. The overlay is libass running in the node, emitting
  timed bitmaps cropped to the text bounding box and palettised — **measured at 8.7 KB per
  1080p event at 32 colours (visually lossless), so ~12 MB per film, 0.14% of a 9 GB film.**
  Bandwidth is not the objection.

  **And it is cheap to produce: 1.16 s per film-equivalent** at event granularity, measured
  2026-09-20 — against 18.15 s if it is implemented by compositing over the decoded film, which
  is the trap ([D114](../../decisions/working-notes.md)). Render the events, not the movie.

  It exists because it moves the attacker-supplied font parse into the node's jail instead of
  onto a television running an old FreeType, which is strictly stronger than
  [D56](../../decisions/working-notes.md)'s client-decides model and fills the gap
  [D57](../../decisions/working-notes.md) left: *correct typesetting, and your device never
  touches the font.* It also removes the need to serve script fonts for subtitles at all
  ([D53](../../decisions/working-notes.md)), and makes a new client platform nearly free —
  drawing a PNG is a few lines where porting libass is a project.

  **Text stays the default**, because a bitmap cannot honour a user's subtitle-size setting
  without re-rendering, and subtitle size is accessibility rather than preference. Overlay is
  chosen automatically when embedded fonts are wanted but declined, when the client lacks a
  font for the script, or when the client has no libass.

  Delivery is **our own track type**: IMSC 1.1 does define an Image Profile, but Apple's HLS
  supports only the Text Profile (`stpp.TTML.im1t`), and our clients already composite
  subtitles themselves rather than delegating to the platform.
- **The invariant: no operation on one axis may restart another**
  ([D23](../../decisions/working-notes.md)). Turning captions on or changing language is a
  rendition fetch, not a new session. Every track must therefore be enumerated and materialised
  before playback begins. A burned-in stream is the single labelled exception and is a *video*
  rendition, flagged so clients can say the subtitle choice is locked. **[D78](../../decisions/working-notes.md)
  shrinks that exception to animated ASS alone** — karaoke and moving signs, which
  event-granularity bitmaps cannot express. Every other case burn-in used to serve is now served
  by the overlay rendition, which stays switchable.

- **The subtitle renderer's defaults are wrong for us** ([D50](../../decisions/working-notes.md)).
  libass ships bug-compatible with VSFilter: with `ASS_FEATURE_WHOLE_TEXT_LAYOUT` off — the
  default — runs are *"laid out left-to-right, even if they contain right-to-left text"*, which
  is the same defect as the browser-native WebVTT rendering we chose libass to avoid. So build
  it with HarfBuzz, a current FriBidi and **libunibreak**, and enable `WHOLE_TEXT_LAYOUT`,
  `BIDI_BRACKETS` and `WRAP_UNICODE` for everything that is not authored `.ass`/`.ssa`, passing
  the **track language** — which [D24](../../decisions/working-notes.md) already normalises —
  because `WRAP_UNICODE` needs it to break lines in Thai, Japanese and Chinese. JASSUB, our web
  path, ships no libunibreak and never calls `ass_track_set_feature`, so this is a build patch,
  not a configuration flag. `ass_track_set_feature` returns −1 when a feature is unavailable:
  probe once and report, rather than silently rendering wrong.

- **The font chain under the renderer** ([D53](../../decisions/working-notes.md),
  [D54](../../decisions/working-notes.md)). Correct bidi and Unicode line breaking are worthless
  if the glyphs are boxes — libass will happily break lines perfectly *between* tofu. So: the
  subtitle's embedded fonts first; then Noto served on demand, chosen **by language rather than
  by script**, because Han unification means a script-keyed choice renders Japanese with Chinese
  letterforms; then bundled Unicode Last Resort HE as the floor.

  The served tier is the **CTA-708 style axis** — six upright variable faces covering the seven
  real caption styles for Latin, Greek and Cyrillic, 5.63 MB raw and ~2.42 MB as woff2, cached
  once per client. Variable `wght`+`wdth` axes are what keep it to six files rather than a
  weight × width matrix, and small capitals is the `smcp` feature rather than a face. Everything
  else is **subset per subtitle track at scan time**: a film's Japanese dialogue uses ~1,200
  distinct code points, so `hb-subset` — already in the build for libass — turns 15.7 MB of Noto
  CJK into ~441 KB. The subset is a scan-time artifact beside the extracted track, so no font
  parser ever enters the core's request path ([D55](../../decisions/working-notes.md)).

  **Fonts embedded in the media file are off by default** ([D56](../../decisions/working-notes.md)).
  The manifest advertises that a track carries them; using them is an explicit client request,
  remembered per series like any other track preference. Two FreeType bugs are in CISA's
  exploited-in-the-wild catalogue — one from March 2025, affecting every version ≤ 2.13.0, which
  is what the old TV boxes carry — and a server that pushes attachments to every client is an
  amplifier for exactly that. Netflix's own subtitle spec permits one generic font family and no
  named fonts at all, from suppliers it has contracts with. When a user does opt in, the subset
  pass strips the TrueType hinting VM (`fpgm`/`prep`/`cvt `) and the bitmap/colour tables where
  CVE-2020-15999 lived, which measures **14% smaller**, not larger.

  The opt-in is **per user, sticky at series scope**, riding the existing track-preference
  machinery ([D57](../../decisions/working-notes.md)) — and a user who has not opted in gets a
  manifest that **does not contain the font resource at all**
  ([D58](../../decisions/working-notes.md)). Omission, not rejection: there is no URL for a
  correct client to decline to fetch. The server-side check stays as a backstop, and because it
  never fires in a correct system, every firing is worth looking at. And the preference says the user wants correct
  typesetting, not that this device should parse it: a browser's WASM-sandboxed FreeType may
  honour it where an old TV box's in-process FreeType declines, under one setting. A client that
  declines says so in the same "why is this different?" surface that explains transcoding.

- **Segment URLs stay cacheable** ([D70](../../decisions/working-notes.md)). Authorise the
  **session**, not the segment: the manifest is per-user and authorised, segment paths are
  stable and content-addressed, and the authorisation rides in a token the cache key ignores.
  Signing each segment URL per user is the obvious-looking application of the signed-URL work
  and it would give every viewer a unique URL for identical bytes — silently foreclosing both
  ordinary caching and any future fan-out, long before anyone tried to build either. Ordinary
  CDN practice; free if done now.

## The cast path is remux-by-default, and it is a first-release dependency

[D137](../../decisions/working-notes.md). Cast's supported containers are **MP2T, MP3, MP4,
OGG, WAV and WebM** — **there is no Matroska**, and WebM is a subset an ordinary `.mkv` of
H.264 + AC-3 does not satisfy. MKV is the dominant container in the libraries this project is
for, so **casting inverts [D4](../../decisions/working-notes.md)'s expectation**: the common
case is a remux, not a direct play.

Architecturally that costs nothing — [D22](../../decisions/working-notes.md)'s per-axis remux
is LGPL and needs no encoder — but it changes the schedule: **the remuxer has to be solid in
the first release** rather than eventually, because the cast path leans on it constantly.

Three more consequences land here:

- **Subtitles.** Cast accepts TTML, WebVTT and CEA-608/708, and **nothing else** — no SRT, no
  ASS, no PGS. SRT converts, which this service already does. **ASS and PGS take
  [D78](../../decisions/working-notes.md)'s overlay rendition**, which was argued for moving a
  hostile font parse off an old television and turns out to be the only route onto a platform
  that will never support ASS.
- **Audio.** **AC-3 and E-AC-3 pass through**, which is exactly
  [D83](../../decisions/working-notes.md)'s rule — a passed-through bitstream takes the gain as
  metadata, never a re-encode for a volume change. **DTS and TrueHD do not**, so they take the
  ordinary audio transcode, which is the commonest real transcode anyway.
- **Video.** Chromecast 1st–3rd generation decode **H.264 and VP8 only**; HEVC arrives with the
  Ultra. So a 10-bit HEVC file to an older dongle needs a *video* transcode
  ([transcoding.md](transcoding.md) §(a), Phase 4) and until then the honest answer is the
  transcode-reason field rather than a stall.

**And one unresolved risk that collides with [D22](../../decisions/working-notes.md).**
`HlsVideoSegmentFormat` defines `FMP4` as well as `MPEG2_TS` and `#EXT-X-MEDIA` is documented,
but there are reports that **HLS with CMAF/fMP4 fails on some CAF receivers, which accept only
HLS-TS** — UNVERIFIED and device-dependent. It is the same shape as the demuxed-HLS risk
[playback-survey.md](playback-survey.md) already carries for Tizen and webOS, and it belongs
in the same measurement.

## Adaptation: the client re-picks, on its own buffer

Added by [D86](../../decisions/working-notes.md). The design resolved the rendition list **once,
at the start**, which is correct on a LAN and is a stall on a hotel Wi-Fi — and remote access is
the headline feature and the paid tier.

**Netflix published the correction to the obvious approach**, on their own production traffic:
Huang, Johari, McKeown, Trunnell and Watson, *A Buffer-Based Approach to Rate Adaptation*,
SIGCOMM'14. Choosing the rate from **buffer occupancy alone** — no bandwidth estimation — cut
the rebuffer rate by **10–20% against their own production algorithm**, with the conclusion:
*"capacity estimation is unnecessary in steady state; however using simple capacity estimation
(based on immediate past throughput) is important during the startup phase."*

We still build no encoding ladder. We do this:

- **The client re-runs the negotiation it already runs**, driven by its own buffer level. Under
  [D2](../../decisions/working-notes.md) the picking logic and the buffer are both already on
  the client, so the correct adaptation input is in the correct place and there is no
  server-side bandwidth estimator anywhere.
- **Segment-grid alignment becomes a requirement of remote access**, not a detail of
  transcoding. [transcoding.md](transcoding.md) §(b) already specifies using the source keyframe
  grid as the segment grid for *every* rendition including passthrough, so variants are
  interchangeable. It was specified with no consumer; this is the consumer, and without it a
  downshift is a reposition.
- **[D23](../../decisions/working-notes.md)'s invariant collides and is resolved by the above.**
  A bandwidth downshift is a video-axis change, in the one situation where a restart is least
  affordable. Aligned variants make it a switch rather than a restart.
- **Where adaptation is impossible — no transcoder registered — say so.** The transcode-reason
  field already exists for this; "this server cannot produce a lower rendition" is the honest
  answer and it belongs in the remote-access documentation, not in a hotel.

## Audio presentation

[audio-presentation.md](audio-presentation.md) owns this; two consequences land here.

- **Loudness is applied as gain at playback** from the per-track measurement the node made at
  scan ([D83](../../decisions/working-notes.md)). A passed-through bitstream gets the gain as
  metadata for the client to apply, never a re-encode for the sake of a volume change.
- **Dynamic-range compression is a per-device declaration**, not a server setting. AC-3 and
  E-AC-3 carry `dialnorm`/DRC metadata and FFmpeg applies it by default (`drc_scale = 1`), which
  is right on a train and wrong in a home cinema. The correct value depends on the room, so the
  client declares it — [D2](../../decisions/working-notes.md) applied to audio.

## The caption controls are a contract, not a client detail

[D90](../../decisions/working-notes.md), [accessibility.md](accessibility.md). 47 CFR §79.103
requires user control of caption font (the eight CEA-708 fonts), size 50–200%, text and
background colour from at least eight, background and window opacity, and five character-edge
attributes; the EU Accessibility Act has applied since 2025-06-28. Two things follow for the
manifest:

- **[D55](../../decisions/working-notes.md)'s served font set is already that list.** Make the
  eight-style mapping explicit and the font endpoint becomes a conformance artefact rather than
  a coincidence.
- **The overlay rendition forecloses every one of those controls**, not only size as
  [D78](../../decisions/working-notes.md) noted. So the manifest never offers the overlay as the
  *only* form of a caption track where a text form exists, and a client using it reports what it
  lost, in the same surface that explains a transcode.

## Done when

A client direct-plays everything the playback survey says it should; a client that cannot decode
TrueHD or DTS-HD still plays the film with its video untouched and its audio transcoded; ASS
subtitles render correctly with their embedded fonts; **an Arabic subtitle keeps its punctuation
on the correct side and a Thai one wraps at word boundaries**; **captions and subtitle language
can be switched forty minutes in without the video stuttering**; **two films from different
decades play at the same perceived level**; **a caption track always offers a text form where
one exists**; **our HLS output passes Apple's `mediastreamvalidator`** — every segment starting
on an IDR, which is also what makes variants interchangeable for
[D86](../../decisions/working-notes.md); resume is correct; and anything that still cannot play
says why.
