> **ABOUTME:** What Google Cast actually provides and actually refuses — read against our own
> design to find where we are rebuilding the platform and where we have underestimated it. The
> answer differs in both directions.

# The Cast protocol, read against our design

Researched **2026-09-21** from Google's Cast developer documentation. Backs
[D137](../../decisions/working-notes.md). Prompted by the project owner: *"go through the whole
Chromecast spec and make sure we're not overcomplicating here. There may also be issues with
things like subtitles and audio tracks."*

Both halves turned out to be right, in opposite directions: **the receiver should be simpler
than we specified, and the server path is heavier.**

## 1. What Cast already provides, and we had planned to build

| We specified | Cast provides | Verdict |
| --- | --- | --- |
| A codec probe on the receiver ([D2](../../decisions/working-notes.md), [D124](../../decisions/working-notes.md)) | **`CastReceiverContext.canDisplayType()`** — check a codec/media-type string before loading | **use it**; D2's probe-and-pick is a documented API here |
| Caption presentation controls ([D90](../../decisions/working-notes.md)) | **`TextTrackStyle`** — `fontFamily`, `fontGenericFamily`, `fontScale`, `fontStyle`, `foregroundColor`, `backgroundColor`, `edgeType`, `edgeColor`, `windowType`, `windowColor` | **use it** — see §2, this is the important one |
| Audio and subtitle track switching as separate axes ([D22](../../decisions/working-notes.md)) | **`activeTrackIds`** plus the **`EDIT_TRACKS_INFO`** message | use it |
| A queue with next/previous ([D135](../../decisions/working-notes.md)) | **`QUEUE_NEXT`/`QUEUE_PREV`**, supported even by the **Default** Media Receiver for audio and video | use it |
| Paging a long queue | **`QueueBase.fetchItems`** — windowed, fetched on demand | use it; the namespace caps a message at **64 KB** |
| A credential for the receiver ([D135](../../decisions/working-notes.md)) | **`LoadRequestData.credentials`** and `credentialsType` | use it rather than inventing a header |
| Multi-device control ([D136](../../decisions/working-notes.md)) | multiple senders may `CONNECT` to one session | use it |

**Seven things we were going to specify are already specified.** That is the answer to "are we
overcomplicating": on the control surface, yes — the protocol is richer than the design assumed.

## 2. `TextTrackStyle` is the statutory caption set, implemented by the platform

The single most useful finding. Google's documentation says of `TextTrackStyle`: *"The window
concept is defined in CEA-608 and CEA-708"*, and *"if the font is not available in the
receiver, the `fontGenericFamily` will be used instead."*

Compare it against 47 CFR §79.103, which [accessibility.md](../plans/accessibility.md) lists as
our statutory floor:

| CVAA requires | `TextTrackStyle` |
| --- | --- |
| the eight CEA-708 fonts | `fontFamily` + `fontGenericFamily` |
| 50–200% sizing | `fontScale` |
| text colour, ≥8 | `foregroundColor` (`#RRGGBBAA`) |
| background colour, ≥8 | `backgroundColor` (`#RRGGBBAA`) |
| background opacity | the alpha channel of the above |
| caption **window** colour and opacity, separately | `windowType`, `windowColor` |
| five edge attributes | `edgeType`, `edgeColor` |

**Every row is covered.** [D90](../../decisions/working-notes.md) observed that
[D55](../../decisions/working-notes.md)'s served font set *"is already that list"* by accident;
this is the same coincidence from the other side, and it means **on the cast path the platform
implements the conformance surface for us** — more faithfully than we would, since it is the
reference implementation of the model the regulation was written around.

**So the cast receiver should not render text subtitles itself.** Serve WebVTT, set
`TextTrackStyle`, and stop.

## 3. What Cast refuses, which is more than we assumed

### Containers: no Matroska

Supported containers are **MP2T, MP3, MP4, OGG, WAV, WebM**. `MKV` is absent, and WebM is a
Matroska *subset* that an ordinary `.mkv` of H.264 + AC-3 does not satisfy.

**MKV is the dominant container in the library this project is for.** So **casting is
remux-by-default**, which inverts [D4](../../decisions/working-notes.md)'s direct-play
expectation for this one client. Not an architectural problem —
[D22](../../decisions/working-notes.md) already puts per-axis remuxing in the core, LGPL and
encoder-free — but it makes the cast path depend on that remuxer being solid from the first
release rather than eventually.

### Text tracks: TTML, WebVTT, CEA-608/708 — and nothing else

**No SRT. No ASS/SSA. No PGS.**

- **SRT** → convert to WebVTT, which [stream-service.md](../plans/stream-service.md) already
  does.
- **ASS/SSA and PGS** → [D78](../../decisions/working-notes.md)'s **server-rendered overlay**,
  composited as an image layer over the video. A CAF receiver is a web app, so that is an
  element on top of a `<video>` rather than a project.

**D78 was argued for a different reason entirely** — moving the hostile font parse off an old
television's FreeType — and it turns out to be *the* answer for a platform that will never
support ASS. A decision earning its keep on a client that did not exist when it was written.

### Audio: passthrough for Dolby, nothing for DTS

Decoded: **FLAC, HE-AAC, LC-AAC, MP3, Opus, Vorbis, LPCM**. **AC-3 and E-AC-3 are supported as
passthrough** — the Web Receiver SDK passes the compressed bitstream to an HDMI receiver that
can take it. **DTS and TrueHD: no**, and reports of DTS behaviour are inconsistent.

The passthrough case validates [D83](../../decisions/working-notes.md) precisely: *"a
passed-through bitstream gets the gain as metadata for the client to apply, never a re-encode
for the sake of a volume change."* The DTS case is the ordinary audio transcode D22 already
covers, and it is the most common real transcode anyway.

### Video: the old dongles are H.264 only

| Device | Video |
| --- | --- |
| Chromecast 1st/2nd gen | H.264 ≤1080p30, VP8 |
| Chromecast 3rd gen | H.264 ≤1080p60, VP8 |
| Chromecast Ultra | + HEVC, VP9, Dolby Vision, HDR10 |
| Chromecast with Google TV | H.264, HEVC, VP9, 4K |
| Google TV Streamer | + AV1 |

A 10-bit HEVC file to a third-generation dongle needs a **video** transcode, which is Phase 4
([transcoding.md](../plans/transcoding.md) §(a)). **So casting inherits the transcoding
schedule**, and until then the honest answer is the transcode-reason field, not a stall.

### And one unresolved risk

`HlsVideoSegmentFormat` defines both `FMP4` and `MPEG2_TS`, and separate audio via
`#EXT-X-MEDIA` is documented as supported. But there are reports that **HLS with CMAF/fMP4
fails to load on some CAF receivers, which accept only HLS-TS**. That collides directly with
[D22](../../decisions/working-notes.md)'s separate-axis `EXT-X-MEDIA` output.

**UNVERIFIED and device-dependent** — it belongs in [device-lab.md](../plans/device-lab.md)
alongside the CEC questions, and it is the same shape as the demuxed-HLS risk
[playback-survey.md](../plans/playback-survey.md) already lists for Tizen and webOS.

## 4. Net

**The receiver gets simpler than [D124](../../decisions/working-notes.md) implied** — no
libass, no caption-styling code, no codec database, no bespoke track switching. It is the web
player minus its subtitle renderer, plus an image layer.

**The server path gets heavier** — remux almost always, audio transcode often, video transcode
sometimes. All of it already specified and mostly already scheduled, which is the good trade:
the expensive work stays on the machine we control rather than in a client we ship.

**And the custom receiver's justification changes.** D124 said the $5 developer account is
forced because *"the default one can't render our subtitles"*. More precisely: it is forced by
**Watch Together's six methods and by bitmap overlays**. Plain text subtitles work on the
**Default Media Receiver**, which makes that a genuine degraded mode — an MP4 of H.264 + AAC
with WebVTT casts with no custom receiver at all.
