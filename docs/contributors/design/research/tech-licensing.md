> **ABOUTME:** The licensing surface of shipping an open-source media server that can transcode
> — FFmpeg's licence ladder, codec patents, and metadata-provider terms — and the distribution
> posture that follows. Not legal advice; a map of where the landmines are.

# Technology: Licensing and Distribution

Researched **2026-09-17**, prompted by the decision to release this as open source. Nothing
here is legal advice. It is a map of which decisions have licence consequences, what the
largest project in this space actually does, and where a real lawyer would be needed.

## 1. FFmpeg's licence ladder

FFmpeg is not one licence. It is a ladder, and **your build configuration chooses your rung**:

| Rung | Configure | Result |
| --- | --- | --- |
| LGPL v2.1+ | default | Most of FFmpeg. Dynamic linking allowed; you must let users replace the library with their own build. |
| GPL v2+ | `--enable-gpl` | Required to link **x264** and **x265** (both on FFmpeg's `EXTERNAL_LIBRARY_GPL_LIST`). The whole resulting binary is GPL. |
| GPL v3+ | `--enable-version3` | Pulls in GPLv3-only components. |
| Non-redistributable | `--enable-nonfree` | Produces a binary FFmpeg says **may not be redistributed at all**. |

x264 and x265 are both dual-licensed — GPL, or a paid commercial licence. That second licence
exists precisely because the GPL boundary is real enough that companies pay to stand on the
other side of it.

**What Jellyfin actually does**, read from `jellyfin-ffmpeg/debian/rules` on 2026-09-17:

```
--enable-gpl --enable-version3 ... --enable-libx264 --enable-libx265 --enable-libfdk-aac
```

Two things worth noting. First, Jellyfin — a GPL-2.0 project — ships a **GPLv3** FFmpeg build.
Second, and more interesting: `libfdk-aac` normally requires `--enable-nonfree`, which would
make the binary non-redistributable. Jellyfin carries a patch named, verbatim,
**`0026-remove-fdk-aac-from-nonfree.patch`**, which removes that requirement so the build can be
distributed.

Draw the conclusion carefully. Whether that patch is *legally sound* is not a question this
document can answer. What it demonstrates is that **the licence surface here is real enough
that the biggest project in the field patches around it**, in the open, deliberately.

**Position for this project:**

- **Licence the project GPLv3.** It is the licence we end up downstream of anyway once x264/x265
  are in the picture, it is what Jellyfin's FFmpeg build already is, and picking GPLv2 would
  create a one-way incompatibility with our own dependency.
- **Do not enable `--enable-nonfree`, and do not carry a patch that reclassifies a nonfree
  component.** Use the native AAC encoder or `libmp3lame`/`libopus` rather than `libfdk-aac`.
  The quality difference does not justify standing on a contested point.
- **Build FFmpeg ourselves and pin it**, as Jellyfin does — not because of licensing but because
  the hardware-acceleration and tone-mapping patches are where the correctness lives
  ([tech-playback.md](tech-playback.md) §5). Publish the exact configure line; it *is* the
  licence declaration.

## 2. Codec patents — separate from, and independent of, copyright licence

H.264 and HEVC carry patent claims (MPEG-LA / Access Advance pools and others) that are
**entirely separate** from FFmpeg's copyright licence. Having a GPL-clean build says nothing
about patents. This is jurisdiction-dependent and is worst in the United States.

The practical landscape, stated as observation rather than advice:

- Every Linux distribution, and every open-source media project in the survey, ships H.264 and
  HEVC encode/decode. The pools have historically pursued commercial device and service
  vendors, not distributors of free software.
- **Decoding is cheaper risk than encoding.** A server that only ever direct-plays never
  encodes anything. This is a real, concrete benefit of the direct-play-first direction
  (D17) that is worth stating out loud.
- **AV1 and Opus are royalty-free by design.** Where we get to choose an output codec — a
  bandwidth-reduction rendition, for instance — AV1/Opus is the choice with no patent question
  attached, at the cost of encode speed and older-client support.

**Position:** ship the codecs, follow the field's practice, document it plainly, and get actual
legal advice before anything commercial ever happens. Do not pretend the question does not
exist, and do not answer it in a README.

## 3. A consequence worth taking: the encoder lives in a plugin

Because the happy path is direct play (D17), the encoder is not on the critical path for most
playback. That makes a genuinely useful split available, and
[D19](../../decisions/working-notes.md) makes it structural rather than a packaging convention:

- **Core server: demux, probe, extract, thumbnail.** Needs FFmpeg for `ffprobe`, keyframe
  extraction, subtitle extraction and thumbnailing — **none of which requires an encoder**, and
  little of which requires the GPL rung.
- **Transcoder: a first-party plugin** with its own full GPL encoder build, installed
  deliberately, and possibly not even on the same machine.

What this buys on the licensing axis specifically:

- **A smaller licence surface for the default install.** x264/x265 — and therefore the
  `--enable-gpl` rung — are not in it.
- **Almost no patent exposure for the default install**, because it never encodes, and encoding
  is where the exposure concentrates (§2).
- **Someone else can ship a different one.** A commercially-licensed encoder build, a vendor-SDK
  build, a hardware appliance — none of it requires our involvement or our licence posture. A
  user in a jurisdiction where this matters can simply not install a transcoder, and the server
  still works.

And on the engineering axis: a boundary that mechanically stops encoder logic leaking into the
core — which is exactly how Jellyfin ended up with an 8,042-line `EncodingHelper.cs`.

The cost is an interface to design and a capability-negotiation path ("this server cannot
transcode; here is what it can do"). Both are small, and the negotiation path has to exist
anyway for hardware too weak to transcode.

## 4. Metadata provider terms under an open-source release

Being open source resolves the largest worry here but not all of it. From
[tech-metadata.md](tech-metadata.md):

| Provider | The term that matters | Under an OSS release |
| --- | --- | --- |
| **TMDB** | Free for **non-commercial** use with attribution and logo display; commercial use needs a written agreement | A free, non-commercial project is squarely inside the free tier. The trap is **shipping our own API key** — that makes *us* the API consumer for every install. **The user supplies their own key.** |
| **TheTVDB** | Tiered by the *licensee's* revenue: <$50k/yr free with attribution. Per-user subscriptions at $11.99/yr supply a PIN | Zero revenue means the free tier, with attribution. Jellyfin already requires a user-supplied key here; follow that. |
| **OpenSubtitles** | 5 downloads/day anonymous, 20/day with a free account, 1000/day VIP | A per-account budget. Must be the user's account. |
| **MusicBrainz** | Open data; 1 req/sec per IP, self-hosted mirror lifts it | No problem. Requires an identifying user-agent. |

**The single rule that covers all of them: provider credentials belong to the user, not to the
binary.** It is better legally, better for rate limits, better for privacy, and it is what
D13's no-account promise implies anyway. The cost is one more first-run step, and that step can
be made pleasant.

## 5. What this settles

1. **GPLv3 for the project.** It is where the dependency chain lands us.
2. **No `--enable-nonfree`, no `libfdk-aac`, no reclassification patches.** Native AAC, Opus,
   `libmp3lame`.
3. **Ship a self-built, pinned FFmpeg and publish the configure line.**
4. **Codec patents are a real and separate question.** Follow the field's practice, say so
   plainly, get advice before anything commercial.
5. **The encoder lives in a plugin, not the core** (D19) — which shrinks the default install's
   licence *and* patent surface, lets third parties ship alternatives under their own terms, and
   enforces the architectural boundary for free.
6. **Prefer AV1/Opus wherever we choose an output codec** — royalty-free by design.
7. **Provider API keys are always the user's.**
