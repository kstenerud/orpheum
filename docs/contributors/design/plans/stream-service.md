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
- **The invariant: no operation on one axis may restart another**
  ([D23](../../decisions/working-notes.md)). Turning captions on or changing language is a
  rendition fetch, not a new session. Every track must therefore be enumerated and materialised
  before playback begins. A burned-in stream is the single labelled exception and is a *video*
  rendition, flagged so clients can say the subtitle choice is locked.

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

## Done when

A client direct-plays everything the playback survey says it should; a client that cannot decode
TrueHD or DTS-HD still plays the film with its video untouched and its audio transcoded; ASS
subtitles render correctly with their embedded fonts; **an Arabic subtitle keeps its punctuation
on the correct side and a Thai one wraps at word boundaries**; **captions and subtitle language
can be switched forty minutes in without the video stuttering**; resume is correct; and anything
that still cannot play says why.
