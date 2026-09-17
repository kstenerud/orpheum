> **ABOUTME:** The core technical problem of a media server — getting a file on a disk onto a
> screen — covering the playback-negotiation model, streaming format, transcoding architecture,
> subtitles and hardware acceleration. The most consequential research document in this tree.

# Technology: Playback

Everything else in a media server is a database with a nice UI. This is the part that is
actually hard, and the part every incumbent gets wrong in the same places.

## 1. The negotiation model — server-decides vs client-decides

The single most consequential architectural choice available to us.

### Server-decides (Plex, Emby, Jellyfin)

The client declares its capabilities as a static `DeviceProfile` — lists of containers, codecs,
codec profiles, levels, bitrates, subtitle formats. The server matches the media against that
declaration and returns a play method (direct play / direct stream / transcode) plus a
`TranscodeReason`. In Jellyfin this is `StreamBuilder` (2,493 lines) in the
`MediaBrowser.Model.Dlna` namespace — the model is inherited from DLNA, where the server had to
decide because the renderer was a dumb appliance.

**Why it fails.** A profile is a *static claim* about a *dynamic* fact. What a device can
actually decode depends on codec profile and level, bit depth, HDR format, chroma subsampling,
resolution *and framerate together*, whether the hardware decoder is already in use, driver
state, and OS version. No profile expresses that, so profiles are approximations, and
approximations fail in both directions: unnecessary transcodes (the top user complaint) and
confident direct-play of something the client then can't render. The fallout is visible in
Jellyfin's trackers as `Force "direct play" in client` (+49) and `Feature to force transcode`
(+21) — users asking for a manual override of an automatic decision that is wrong often enough
to matter.

Maintaining the profile database is also a permanent tax: every new TV model, every browser
release, every codec is a profile change on the server.

### Client-decides (Netflix, YouTube, Apple — and Kyoo)

The server publishes an HLS/DASH manifest listing every rendition it can produce, with codec
strings, resolution and bandwidth. The client picks, and keeps picking as the network changes.
Kyoo's transcoder README states the position plainly:

> Instead of having complex logic to know which client supports which codecs, we use HLS to
> it's full potential. Each quality & codec supported by gocoder is specified in the manifest
> and the client is free to pick the one it can play.

**Why it works.** The client is the only party that can ask the platform what it can *actually*
do, at runtime:

| Platform | Runtime capability probe |
| --- | --- |
| Web | `navigator.mediaCapabilities.decodingInfo()` — returns `supported`, **`smooth`** and **`powerEfficient`** for a given codec/resolution/framerate/bitrate |
| Android | `MediaCodecList` / `MediaCodecInfo.CodecCapabilities` — hardware vs software, profiles, levels |
| Apple | `AVPlayer` consumes HLS natively and plays what it can |
| Desktop | libmpv/FFmpeg — asks the local decoder directly |

`smooth` and `powerEfficient` are information a server-side profile **cannot** contain, and they
are precisely what distinguishes "will play" from "will play well".

**The cost.** Kyoo names it: transmuxing (passthrough-as-a-rendition) becomes harder, because
the passthrough stream must be segmented on the same grid as the transcoded ones. That is a
real but bounded engineering problem — see §3 — and it is a far better problem than an
open-ended device database.

> **Decision input:** take client-decides. It is the biggest available reduction in total
> system complexity, it removes the largest single bug class in the incumbents, and it makes
> writing a new client dramatically cheaper — which matters because clients are where this
> project's cost lives ([tech-clients.md](tech-clients.md)).

A caveat to record honestly: client-decides pushes work onto every client, and a bad client
picks badly. The mitigation is a shared client SDK that implements the probe-and-pick logic
once, plus a server-side *hint* (not a mandate) in the manifest.

## 2. Streaming format: CMAF/fMP4 + HLS

**Take fMP4 (CMAF) segments with an HLS manifest.** Reasons, in order:

1. **Apple requires it.** Apple's HLS authoring specification mandates fMP4 for HEVC, and for
   HDR10 and Dolby Vision carriage. MPEG-TS cannot carry them. Anything that must reach an
   Apple TV or Safari is fMP4 or nothing.
2. **One set of segments serves both protocols.** fMP4 is DASH's container too; CMAF is the
   interop profile. A DASH manifest over the same segment files is nearly free if we ever want
   it.
3. **HLS is the most universally consumed manifest format.** Native on Apple and Android,
   `hls.js` on every other browser, supported by every TV platform's player, and by mpv/VLC.
4. **MPEG-TS is a legacy trap.** Jellyfin's older TS paths are implicated in several "plays for
   four minutes, then errors" reports, and TS carries per-segment overhead and no HDR path.

**No DRM.** The user owns the files. Skipping FairPlay/Widevine/PlayReady removes key servers,
licence proxies, platform certification and an entire class of playback failure. This is the
main reason a small team can compete in this space at all.

## 2a. Why this makes transcoding deferrable

A consequence of §1 that is easy to miss and worth a lot: **once the client picks from a
manifest, the manifest's contents can grow without any client ever changing.**

The contract is "ask the server what renditions exist for this item; pick one". A server that
can only offer the untouched original answers that question with a one-entry list. A server
that later grows an encoder answers it with a longer list. **Same endpoint, same client code,
no version negotiation, no migration.** Under a device-profile design this is not true — the
profile *is* the negotiation, so the transcode decision is welded into the protocol from the
first release.

This is what makes "ship direct play first, add transcoding later" a schedule decision rather
than an architectural one. The only thing that must exist in v1 is the *shape* of the answer.

## 2b. The two independent drivers of transcoding

The incumbents conflate these; they have different causes, different frequencies, and different
best solutions.

| | **Compatibility transcode** | **Bandwidth transcode** |
| --- | --- | --- |
| Cause | The client cannot decode this codec/profile/container at all | The client decodes it fine but cannot receive it fast enough |
| Frequency | Falling. Modern libraries are H.264/HEVC in MP4/MKV, which everything plays | Situational — remote streaming, weak Wi-Fi, mobile data |
| Depends on | The file, and the client's decoder | The network, and nothing about the file |
| Best solution | A **pre-prepared alternate version** (background job, once per file) | **Live, adaptive** renditions — genuinely needs a ladder and ABR |
| Needed on a LAN? | Only for old/odd files | Almost never |

The useful conclusions:

- Compatibility transcoding is a **library problem**, solvable offline and once per file. It
  does not need to be fast, and it should not be on the playback path.
- Bandwidth transcoding is a **network problem** and is the case that actually requires live
  on-demand segmented transcoding and adaptive bitrate. It is therefore coupled to remote
  access, not to the library.
- Neither is needed for the common case of watching a modern file on the LAN. That case is a
  ranged `GET`.

## 2c. The cheapest transcoding-avoidance strategy is a better client engine

Before building any encoder, note that **the client's choice of playback engine changes the
direct-play rate more than anything the server can do.** A client using libmpv or libVLC — which
carry FFmpeg's full demuxer and decoder set — direct-plays close to everything, including the
old and odd files. A client restricted to the platform's own decoders (AVPlayer, Media3 alone)
does not.

Swiftfin states this as its design rationale: built "to maximize direct play with the power of
VLC". It is why [tech-clients.md](tech-clients.md) recommends shipping two engines per client
with libmpv as the fallback — that recommendation is not only about format breadth, it is
**server capacity policy implemented on the client**, and it costs no server CPU at all.

## 2d. The output menu: inputs are wild, outputs are a short list

Inputs are unbounded — a home library contains everything anyone has ever encoded. **Outputs are
not.** We only ever need to produce what contemporary clients decode, and that set is small,
slow-moving, and — the decisive part — **already chosen for us by silicon**.

### The hardware has already picked the output set

Every hardware encoder that matters supports the same three video codecs and nothing else worth
having:

| Encoder | Video encode support |
| --- | --- |
| NVIDIA NVENC | H.264, HEVC, AV1 (AV1 on RTX 40-series and later) |
| Intel QSV | H.264, HEVC, AV1 (AV1 on Arc/Xe and later), plus MJPEG/MPEG-2 |
| AMD AMF | H.264, HEVC, AV1 (AV1 on RDNA 3 and later) |
| VAAPI (Linux) | H.264, HEVC, AV1 |
| Apple VideoToolbox | H.264, HEVC |

So a "wide range of output formats" was never available in hardware anyway. Choosing a small,
fixed output set **costs nothing we could otherwise have had** — it just stops us writing
software-encoder paths for formats no client wants and no GPU can accelerate.

### Named output profiles, not a capability grammar

The unit is a **named profile** — codec plus profile, level, bit depth and transfer
characteristics bundled under one identifier — because those dimensions are not independently
selectable in practice, and naming the combination is what keeps this from becoming a grammar.

A workable starting menu, deliberately short:

| Video profile | Meaning |
| --- | --- |
| `h264-sdr` | H.264 High @ L4.1, 8-bit, BT.709 — **the mandatory baseline** |
| `hevc-sdr` | HEVC Main @ L5.1, 8-bit, BT.709 |
| `hevc-hdr10` | HEVC Main10, 10-bit, PQ / BT.2020 |
| `av1-sdr` | AV1 Main, 8-bit |
| `av1-hdr10` | AV1 Main10, 10-bit, PQ / BT.2020 |

| Audio profile | Meaning |
| --- | --- |
| `aac-stereo` | AAC-LC 2.0 — **the mandatory baseline** |
| `aac-multi` | AAC-LC 5.1 |
| `ac3` / `eac3` | Dolby Digital / Plus, for TVs and receivers |
| `opus` | Opus, any channel count — royalty-free (D16) |
| `flac` | Lossless, for music |

Container is always fMP4/CMAF (D3), so it is not a dimension. Resolution and bitrate are **not**
menu entries — they are what ABR varies *within* a chosen profile, and conflating the two is
part of what makes device profiles unmanageable.

**Dolby Vision and Atmos are deliberately absent from the menu.** They are passthrough-only:
direct-play them to a client that handles them, and fall back to a menu profile otherwise. We do
not re-encode into them.

**AV1 is a menu entry, not a baseline.** Hardware decode now covers roughly 80% of consumer
devices and ~88% of large-screen devices certified 2021–2025 — but the remaining fifth is
pre-2020 TVs and older streaming sticks, which is precisely the long tail a home media server
exists to serve. It earns its place on the menu; H.264 keeps the floor.

### Three axes, not one vocabulary

Video, audio and subtitles negotiate **independently** ([D22](../../decisions/working-notes.md)).
HLS already works this way — `EXT-X-MEDIA:TYPE=AUDIO|SUBTITLES` with a `GROUP-ID` that video
variants reference by name — and every player we target handles it natively, so separation is the
conventional shape and muxing is the special case.

It matters for five reasons, in descending order of force:

1. **A joint menu would be a grammar.** Five video and six audio profiles are eleven names as
   axes and **thirty** as pairs — and `hevc-hdr10+eac3` is a rule about a combination, which is
   precisely the failure mode this section ends with.
2. **The most common real transcode is audio-only**: video fine, audio TrueHD or DTS-HD. Joint
   negotiation makes that one indivisible choice and risks re-encoding video that was already
   perfect. Separate axes copy the video and transcode only the audio, at roughly 1% of the CPU.
3. **Multi-track is intrinsic.** Three to eight audio tracks per film, switched mid-playback.
   Under a joint vocabulary, changing language is a different variant and can restart the video.
4. **Audio capability is dynamic in a way video capability is not.** The same device answers
   differently for HDMI/ARC into a receiver, the TV's own speakers, or Bluetooth headphones — and
   that path changes mid-session. Video capability is a property of the decoder and does not move.
5. **Music needs only the audio axis**, with no null-video special case.

**Audio segmentation is easier than video, not harder**: audio frames are independently
decodable, so audio segments can always be cut on the video keyframe grid §3 already computes.
The bounded wrinkle is frame granularity — AAC is 1024 samples, ~21.3 ms at 48 kHz — so
boundaries drift slightly. Standard, solved.

**The licence boundary falls between the axes**, which is the finding with real consequences —
see [tech-licensing.md](tech-licensing.md) §3. Every GPL trigger in FFmpeg is a *video* encoder;
the whole audio menu is native LGPL. So audio transcoding belongs in the core and the plugin is
a *video* transcoder.

### Negotiation: the client sends a prioritised list

Because the menu is a short enumeration rather than an open-ended description, the client can
simply say which entries it accepts, in preference order — **HTTP `Accept` with q-values, applied
to a media menu.** The server returns a manifest containing only the intersection of what the
client asked for and what it can produce, always including the untouched original when that
matches.

**This is not a device profile, and the distinction is the whole point:**

| | `DeviceProfile` | Output menu |
| --- | --- | --- |
| What the client sends | An open-ended *description* of its capabilities — containers, codecs, profiles, levels, bitrates, conditional rules | A *subset of the server's published enumeration*, in preference order |
| What the server does | Evaluates the **media** against the **client's rules** to decide a play method | Intersects **two sets of names** |
| Grows by | Users and devices inventing new conditions | The project adding a menu entry, deliberately |
| Failure mode | Silently wrong decisions in both directions | Empty intersection — impossible, see below |

The server never reasons about whether this client can play this file. It computes a set
intersection over a closed vocabulary.

### The mandatory baseline makes negotiation total

Every client **must** include `h264-sdr` + `aac-stereo` in its list, and every transcoder
**must** be able to produce them. Given both, the intersection can never be empty and negotiation
cannot fail. That turns "a reasonable assumption that we'll handle at least one of them" into a
guarantee stated in the contract, which is worth considerably more.

### What this buys, and what it does not

**Buys:**

- **Dumb clients become cheap.** A Tizen, webOS or Roku client with a poor capability API does not
  need to implement probe-and-pick from §1 — it ships a static compiled-in list and is correct.
  This directly answers §1's honest caveat that "a bad client picks badly".
- **The manifest never advertises something that will fail**, because it is filtered to the
  client's own list before it is built.
- **The transcoder's capability declaration becomes a set of names** — which makes the
  rendition-list join a three-way set intersection (`client ∩ transcoder ∩ {original}`) rather
  than an open negotiation. This is the hard part of D19's interface, substantially defused.
- **The Jellyfin adapter gets easier** (D18): mapping an inbound `DeviceProfile` onto our menu is
  a *classification* into ~11 buckets, not an evaluation engine.

**Does not buy:** most of `EncodingHelper.cs`. Being honest about scope — that file is 8,042
lines mainly because of the **hardware-acceleration matrix** (per-backend filter chains, pixel
format upload/download, tone-mapping paths), not because of output-format breadth. A fixed menu
removes a real slice of the complexity and removes it permanently; the hwaccel matrix remains the
dominant cost and is addressed separately in §5.

### The failure mode to watch

**The menu must stay an enumeration and must never become a grammar.** The pressure will be
constant and reasonable-sounding: "this TV needs H.264 but capped at Level 4.0", "this one needs
8-bit HEVC but only at 30fps". Each concession is a condition, and conditions are what
`DeviceProfile` is made of.

The discipline: the menu is owned by the project and versioned. A device that needs something not
on it takes the baseline, or someone adds a menu entry as a deliberate, reviewed change. If we
ever find ourselves adding a *condition* rather than an *entry*, we have rebuilt the thing we
rejected.

## 3. Transcoding architecture

Three viable models. They are not exclusive.

### (a) Direct play — hand over the file

A ranged HTTP GET of the original file. Zero server cost, perfect quality, and what users
want. **Must be the default and must always be offered as a rendition in the manifest.** For
clients using a real media framework (libmpv, ExoPlayer, AVPlayer, VLC), this covers the large
majority of a typical library.

### (b) Live, on-demand, segment-addressed transcoding

The Kyoo model, and the right general-purpose fallback. Mechanics, as verified in
`kyoo/transcoder/src/`:

1. **Extract every keyframe timestamp from the source up front**, using packet-level `ffprobe`
   data rather than decoding frames (the author reports roughly 20× from this alone). Cache the
   result in the database — Kyoo versions it (`KeyframeVersion = 2`) so the extraction logic can
   change.
2. **Use those timestamps as the segment grid for every rendition**, including the passthrough
   one. This is what makes variants switchable: HLS requires segment boundaries to align across
   renditions, and boundaries must fall on keyframes.
3. **Force the encoder to emit IDR frames exactly there** (`-force_key_frames`). Sharp edge:
   **NVENC ignores `-force_key_frames` and needs `-force_idr 1`.** Expect one of these per
   hardware backend.
4. **Start a transcode at the requested segment**, not from zero, so seeking and mid-stream
   quality switches don't replay from the beginning.
5. **Share one transcode between viewers** of the same title and rendition.

Known sharp edges to design for: a client that switches rendition mid-stream can outrun the new
rendition's progress; keyframe extraction on a huge file is slow the first time and should be
incremental and streamed (Kyoo's `Keyframe` type has listeners that fire as keyframes arrive).

### (c) Background pre-preparation

The commercial model at household scale ([commercial-streaming.md](commercial-streaming.md)).
Not a full ladder — a single "compatibility rendition" per title, produced by a background job,
for content that is *known* to be awkward (10-bit HEVC where a client needs H.264, TrueHD where
a client needs AC-3). Storage is cheap; CPU at playback time is not, and it is what makes a
home server feel slow.

> **Decision input (revised — see [D17](../../decisions/working-notes.md)):** build **(a) only**
> for v1, with the manifest shape that lets (b) and (c) be added later without touching any
> client (§2a). Measure the real direct-play rate first (`plans/playback-survey.md`) — it is the
> number that decides how urgent the rest is. Then add **(c)** for compatibility and **(b)** for
> bandwidth, in that order, because (c) is the simpler problem and (b) is coupled to remote
> access. Do not build a Netflix-style ladder generator at any point.

### (d) Somebody else's machine

A model no incumbent offers, and the one that falls out of treating the transcoder as a
registered service rather than linked-in code ([D19](../../decisions/working-notes.md)): the
box serving the files need not be the box doing the encoding. A NAS with a weak CPU serves; a
desktop with a GPU transcodes; the client is redirected to whichever produced the segments.

**The demand is proven, and so is the absence of anywhere to put it.** `rffmpeg` — 1,073 stars,
created 2019, still maintained in 2025, and **written by a Jellyfin founder** — is an
out-of-tree Python wrapper that impersonates the `ffmpeg` binary and SSHes to a remote host to
run it. That hack exists because Jellyfin's architecture has no seam for remote transcoding,
and a founder writing an impersonation shim outside his own project is strong evidence that
the seam should have been there.

Kyoo's `gocoder` is the positive case: a standalone transcoding service, already reused by two
unrelated projects (Meelo, Blee). A transcoder that is a service is reusable; a transcoder that
is a subsystem is not.

### Audio is the under-appreciated half

A large fraction of real-world "video transcodes" are actually **audio-only** problems: the
video is fine, but the client can't do TrueHD/DTS-HD/E-AC-3, or wants stereo from 7.1. Handling
audio as an independently-selectable HLS rendition group — transcode the audio, pass through
the video — is a large, cheap win that the incumbents handle only partially. `Add a fallback to
AC3/EAC3 option` (+22) and `HD Audio Passthrough on Nvidia Shield` (+20) are Jellyfin's
versions of this complaint.

## 4. Subtitles — a first-class concern, not a detail

The evidence says this deserves its own subsystem: the most-reacted Jellyfin server issue is
subtitle desync on resume (+103, open since 2020), the top `jellyfin-web` bug is subtitle
desync, and subtitles are the most frequently named pain point in HN threads.

**The formats, and what each demands.**

| Format | Nature | Requirement |
| --- | --- | --- |
| SRT / WebVTT | Text | Trivial. Convert to WebVTT, serve as an HLS subtitle rendition. |
| ASS / SSA | Text + a full styling and animation language | Must be rendered by **libass** or it is wrong. Needs the fonts, which are often *attachments inside the MKV*. |
| PGS (Blu-ray) / VobSub (DVD) | Bitmap | Must be composited, or OCR'd, or burned in. |

**Consequences for the design.**

1. **Ship libass on every client.** `jassub` (libass compiled to WASM with WebGL/WebGPU
   acceleration; successor to JavaScriptSubtitlesOctopus, which Jellyfin forked as
   `@jellyfin/libass-wasm`) for web; native libass on desktop via mpv; libass bindings on
   mobile. Kyoo uses `jassub` + `libpgs` in its React Native front. Anything less means ASS
   subtitles render wrong, which is most anime and a lot of fansubbed content.
2. **Extract and serve font attachments.** An ASS file without its fonts is not the same
   subtitle. Kyoo's transcoder has an explicit attachment endpoint for this.
3. **Pre-extract embedded subtitle tracks on scan, not at playback.** The workaround an HN user
   arrived at for Jellyfin — extract everything to SRT, disable serving embedded subs — should
   be the *default behaviour*, because extraction from a large MKV at playback start is exactly
   the "progress indicator loops forever" complaint.
4. **Never burn in unless there is no alternative**, because burning in forces a video
   transcode of content that would otherwise direct-play. PGS on a client without a bitmap
   renderer is the case that forces it.
5. **Offset/sync adjustment is a client control**, and must exist. It is requested on every
   client tracker.

## 5. Hardware acceleration

The support matrix — VAAPI (Intel/AMD on Linux), QSV (Intel), NVENC/NVDEC (NVIDIA),
VideoToolbox (Apple), AMF (AMD), V4L2 (Raspberry Pi) — each with its own quirks, driver
requirements and container-passthrough problems. Jellyfin maintains an entire **FFmpeg fork**
(`jellyfin-ffmpeg`) largely to carry hardware and tone-mapping patches, and `EncodingHelper.cs`
is 8,042 lines mostly because of this matrix.

**Stance:**

- Treat each accelerator as a **driver behind one interface**, with a capability probe at
  startup and an explicit, logged fallback to software. Do not let accelerator branches spread
  through the encoder-argument builder — that is precisely how `EncodingHelper.cs` happened.
- **Ship a known-good FFmpeg build** rather than accepting whatever the distro has. Jellyfin's
  fork is evidence that the alternative is a permanent support burden; `jellyfin-ffmpeg` is
  GPL and can be used directly.
- **HDR tone-mapping is its own hazard.** `HDR Tone mapping - Dark videos` (Immich, +15),
  Dolby Vision regressions on Android TV (+17), `webOS playing Dolby Vision direct fails`.
  Prefer passing HDR through to a capable display over tone-mapping it, and make tone-mapping
  an explicit, testable path.
- **Test on real hardware or don't claim support.** This is untestable in CI on a laptop, and
  every incumbent's tracker proves it.

## 6. What this settles

1. **Client-decides, via an HLS manifest.** No `DeviceProfile` database. (D2)
2. **CMAF/fMP4 segments, HLS manifest, no DRM.** (D3)
3. **Direct play is v1; the manifest shape makes transcoding additive, not architectural.**
   Compatibility transcoding (pre-prepared, offline) and bandwidth transcoding (live, adaptive)
   are separate problems and are scheduled separately. (D4, refined by D17)
3a. **The client's engine choice is the first lever on the direct-play rate** — cheaper and more
   effective than any server-side work. (§2c)
3b. **Outputs are a short, fixed, named menu; inputs are unbounded.** The client sends a
   prioritised subset of the menu and the server intersects. A mandatory `h264-sdr`/`aac-stereo`
   baseline makes negotiation total. The menu must never become a grammar. (§2d, D21)
3c. **Video, audio and subtitles are independent axes.** Additive rather than multiplicative,
   audio-only transcoding becomes expressible, mid-playback track switching stays cheap — and the
   GPL boundary falls between the axes, so audio transcoding belongs in the core. (§2d, D22)
4. **Audio-only renditions are a first-class case**, not a special case of video transcoding.
5. **Subtitles get a subsystem**: extract on scan, serve fonts, libass on every client, never
   burn in by default.
6. **Hardware acceleration behind one interface**, with a shipped FFmpeg and honest fallback.
