> **ABOUTME:** Whether to render subtitles server-side and ship them as timed image overlays —
> measured bandwidth, what it buys, what it costs, and why it dominates burn-in.

# Server-Side Subtitle Rendering

Researched **2026-09-19**, prompted by the observation that rendering server-side would move the
attacker-supplied font parse off the client. Bandwidth figures are **measured**.

## 1. It is far cheaper than intuition suggests

Rendered 1080p dialogue lines with a white fill and black outline, then measured. The two things
that matter are **cropping to the bounding box** and **palettising**:

| Encoding | Bytes/event | Mean error (0–255) |
| --- | --- | --- |
| PNG RGBA, full 1920×1080 frame | 30,258 | — |
| PNG RGBA, cropped to text bbox | 24,528 | 0 (reference) |
| PNG, 2-colour palette | 4,217 | 42.35 — visibly bad |
| PNG, 4-colour palette | 6,531 | 5.58 |
| PNG, 8-colour palette | 8,392 | 2.04 |
| PNG, 16-colour palette | 10,658 | 0.79 |
| **PNG, 32-colour palette** | **8,703** | **0.58 — visually lossless** |
| WebP lossless, cropped | 12,856 | 0 |

32 colours is the sweet spot and is *smaller* than 16 because the octree quantiser picks a
better palette. So:

```
~8.7 KB per event  ×  1,500 events  =  12.4 MB per film
                                     =  0.14% of a 9 GB film
```

Against an `.ass` source of 50–150 KB that is ~100×, and against the video it is noise. **On a
LAN it is free; over a remote link it is a rounding error.** Bandwidth is not the objection.

This is, not coincidentally, exactly what PGS does — cropped, palettised bitmaps with position
metadata — so Blu-ray solved this shape decades ago.

## 2. What it buys

**The client never parses an attacker-supplied font.** The hostile parse happens in the node's
Landlock+seccomp jail ([D26](../../decisions/working-notes.md)), which is the place designed for
hostile parses, instead of on a television running FreeType 2.10. That is strictly stronger than
[D56](../../decisions/working-notes.md)/[D57](../../decisions/working-notes.md), which put the
risk decision in the client's hands because the risk varies by client.

**It fills the exact gap D56 created.** D57's honest cost was that a user with an old Fire TV who
wants correct fansub typesetting has to either accept the risk or lose the typesetting. Server
rendering is a third answer: *correct typesetting, and your device never touches the font.*

**It also disposes of font coverage for subtitles.** [D53](../../decisions/working-notes.md) and
[D54](../../decisions/working-notes.md) exist because a client may have no font for a script.
If the server renders, the client needs no CJK font at all for subtitles — only for UI text.

**A new client platform gets subtitles almost free.** "Draw this image at these coordinates for
this time range" is a few lines on any platform; building libass for it is a project. That is
the same economy as [D62](../../decisions/working-notes.md)'s six-method rule, applied to a
different subsystem.

**And it dominates burn-in.** [D23](../../decisions/working-notes.md) names burn-in as the one
thing that breaks the no-restart invariant, because it is a *video* rendition. An overlay track
is a separate, switchable track with no video re-encode — so every case where burn-in was the
answer for a weak client is now served by something better. Any client that can play video can
composite a bitmap.

## 3. What it costs, honestly

**Scaling and user styling are lost.** A rendered bitmap has a fixed size, colour and position.
Subtitle size is a real accessibility setting, not a preference, and a bitmap cannot honour it
without re-rendering. Mitigations exist — we are the server and we know the client's viewport
and the user's settings, so render per request, or precompute two or three sizes — but this is
now *state*, where a text track was universal.

**Animation is gone.** Karaoke timing and moving signs animate per frame; an overlay at
subtitle-event granularity cannot express them. Only burn-in can. So D23's exception **narrows
to animated ASS** rather than vanishing.

**It is another rendition on the menu** ([D21](../../decisions/working-notes.md),
[D22](../../decisions/working-notes.md)), which the menu is designed to absorb but which is not
free.

**Precomputation conflicts with per-user sizing.** [D11](../../decisions/working-notes.md)
already extracts subtitles at scan time, so precomputing overlays fits the existing pipeline —
but per-user font size forces a live path. libass is fast enough that live rendering is
plausible; it is a measurement nobody has taken.

## 4. The standards position

**IMSC 1.1 defines an Image Profile** — timed PNG subtitles — alongside the Text Profile. So
this shape is a W3C standard and not an invention.

But **Apple's HLS supports only the Text Profile** (`CODECS="stpp.TTML.im1t"`, announced at
WWDC 2017, and IMSC content must be in fragmented MP4). So image subtitles cannot ride the
standard HLS subtitle track to AVPlayer.

Which means: **deliver as our own track type and composite in our own client.** That is no
hardship — our clients already composite subtitles rather than delegating to the platform
(JASSUB on the web, libass in the native players), and compositing a PNG is strictly simpler
than what they do today.

## 5. What this suggests

A **third subtitle rendition** — text, server-rendered overlay, burn-in — not a replacement for
the first. Text stays the default because scaling and user styling matter. Overlay becomes the
automatic choice when:

- embedded fonts are needed and the client declined them
  ([D57](../../decisions/working-notes.md));
- the client has no font for the script and cannot or will not fetch one
  ([D53](../../decisions/working-notes.md));
- the client has no libass and no wish to acquire one;
- or the user simply prefers it.

And burn-in retreats to animated ASS alone.
