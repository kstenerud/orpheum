> **ABOUTME:** How media software is actually tested — four reference projects read locally,
> what their corpora really contain and cost, and the measurements that decide our tiers.
> The evidence behind D109–D112.

# Testing strategy research

Researched **2026-09-20**. The plan this feeds
([testing-strategy.md](../plans/testing-strategy.md)) had been `UNSPECIFIED` since 2026-09-17
with a list of questions; this answers them from four codebases read locally and from
measurements taken here.

## 1. The corpus question was framed wrongly, and the reference projects say so

The plan asked *"how does a contributor without 200 GB of media run the suite?"* The premise is
false. **Nobody's media test corpus is 200 GB**, and the two ends of the spectrum are an order
of magnitude apart from each other and three from that guess.

| Project | What it is | Real media in the suite | Size |
| --- | --- | --- | --- |
| **Jellyfin** | a media server, C# | **none at all** | 3.9 MB of tests, 0 media files |
| **Navidrome** | a media server, Go — our closest analogue | **13 files** | **512 KB**, in-tree |
| **hls.js** | a player | none — **49 stream URLs** | 0 bytes, by reference |
| **FFmpeg** | the codec library underneath everything | **2,875 files** | **1.29 GB**, external, opt-in |

### Jellyfin tests the media layer with no media

Read from `tests/` on master (2026-09-19): 285 C# files, ~42,000 lines, **3.9 MB total, and not
one video or audio file.** The entire non-code fixture set is 133 files:

```
 88 json    20 nfo    11 xml     2 txt     2 srt     2 jpg
  1 zip      1 ssa     1 ass     1 m3u8    1 elrc    1 html
```

The media-layer fixtures are **fifteen recorded `ffprobe` JSON outputs**, 120 KB in total, with
names that are a bug list in themselves: `video_missing_video_bitrate.json`,
`video_nanosecond_duration_bitrate.json`, `video_progressive_no_field_order.json`,
`video_single_frame_mjpeg.json`, `video_interlaced.json`, `video_ts.json`.

**This transfers to us better than it does to them**, and
[D59](../../decisions/working-notes.md) is why. For Jellyfin, probe output is an internal
detail. For us it is a **wire format**: the node reports it and the core interprets it. So the
core's entire media-interpretation surface — filename parsing, track-kind derivation, language
normalisation, version grouping, segment placement — is testable from recorded JSON with no
media files, no FFmpeg, and no filesystem. That is not a convenience; it is the thing
[D59](../../decisions/working-notes.md) was for, cashed out in the test suite.

### The biggest test project in Jellyfin is the filename parser

`Jellyfin.Naming.Tests` is 28 files and 3,938 lines, and it is table-driven:
`SeasonPathParserTests.cs` carries **79** `[InlineData]` cases, `EpisodeNumberTests.cs` **77**,
`MultiEpisodeTests.cs` **63**. [identification.md](../plans/identification.md) already calls the
parser *"the easiest component in the project to test"*; this is what that looks like at scale,
and it is the shape to copy.

(It also contains the extras-folder vocabulary — `theme-music`, `behind the scenes`,
`deleted scenes`, `interviews`, `scenes`, `samples`, `shorts`, `trailers` — which is
[D97](../../decisions/working-notes.md)'s discovery mechanism, already written down by someone
else as a test table.)

### Navidrome's real files are chosen for pathology, not coverage

13 files, 512 KB, in-tree, and the filenames are the design:

```
no_replaygain.mp3          zero_replaygain.mp3        invalid-files/test-invalid-frame.mp3
._02 Invisible.mp3         01 Invisible (RED) Edit Version.m4a
mixed-lyrics.flac          bom-test.ttml              artist/an-album/test.mp3
```

`._02 Invisible.mp3` is a macOS AppleDouble sidecar — junk a scanner must skip.
`test-invalid-frame.mp3` is deliberately corrupt. `no_replaygain`/`zero_replaygain` distinguish
*absent* from *zero*, which is [D83](../../decisions/working-notes.md)'s exact problem a decade
early. A filename with spaces and parentheses. A file inside a nested directory.

**None of these test a codec. They test the boundary**, which is all an application layer
above FFmpeg ever needs real files for.

### FFmpeg needs a real corpus because it *is* the codec, and even that is 1.3 GB

Measured with `rsync -n --stats rsync://fate-suite.ffmpeg.org/fate-suite/` on 2026-09-20:

| | |
| --- | --- |
| files | **2,875** (2,531 regular) |
| total | **1,353,652,880 B = 1.29 GiB** |
| median file | **58.7 KB** |
| mean | 470.9 KB |
| p90 | 1.24 MB |
| largest | 18.5 MB |
| under 1 MB | **86%** |

A media test corpus is **thousands of tiny clips**, not a library of films. And FFmpeg's
in-tree part — 114 test-group makefiles and **4,681 reference files totalling 24 MB** — ships
with the source; only the samples are external.

The mechanism is worth copying exactly. `make fate-rsync SAMPLES=...` fetches them, and
without `SAMPLES` the build prints *"warning: only a subset of the fate tests will be run"* and
runs the subset that uses **generated** inputs — `videogen`, `audiogen` and `rotozoom` produce
`vsynth1/2/3` and `asynth` from nothing. **Two tiers, generated by default, real opt-in**, with
the degraded mode announced rather than silent.

## 2. What FFmpeg asserts, when the output is not reproducible

The plan asked what replaces bit-exactness. FATE's answer is a graded set, from
`tests/fate-run.sh`:

| Method | Used for |
| --- | --- |
| `diff` / `rawdiff` | text output — probe dumps, playlists |
| `framecrc`, `framemd5`, `md5` | decoded output, where it **is** bit-exact |
| `oneoff` | "absolute difference ≤ threshold" for a single value |
| `stddev` | audio, against a tolerance |
| `tiny_psnr`, `tiny_ssim`, `audiomatch` | purpose-built comparators, in-tree |
| `refcmp-metadata.awk` | float metadata within a **fuzz** (default 0.1) |

Note `CMP_UNIT = 1` on the encoder tests: a per-test tolerance unit rather than a global one.

**The lesson is not "use PSNR".** It is that *the assertion is chosen per test from a small
closed set*, and the loosest one available is never the default.

## 3. VMAF, and the regression-gate trap

Netflix's VMAF is the industry answer for perceptual quality, and the number that matters is
the **just-noticeable difference of about 6 VMAF points**; a delta under ~2 is noise.

The trap is more useful than the metric, and it generalises far past video: **compare against a
frozen baseline, not against the previous build.** A sequence like 96.0 → 95.6 → 95.2 → 94.8 →
94.4 → 94.0 passes every build-to-build tolerance and has lost 2 points in six builds. Any
threshold gate compared against "last time" ratchets; a gate compared against a pinned
reference does not.

That applies to our **latency** targets identically, and it is the rule
[performance-targets.md](performance-targets.md) needs most.

`libvmaf` is compiled into ordinary FFmpeg builds (confirmed in the build used here), so this
costs no extra tooling.

## 4. Apple's HLS validators are two tools, and they are macOS-only

`mediastreamvalidator` checks the **HLS specification**; `hlsreport` consumes its JSON
(`--validation-data`) and checks the **HLS Authoring Specification**. Apple's own guidance is to
run both — they check different documents. That is the conformance gate for
[D86](../../decisions/working-notes.md)'s "segments must start with an IDR, key frames about
every two seconds".

**They are macOS-only**, which places them: a **release gate on a Mac**, not a per-commit step,
unless a macOS runner is available. Worth knowing before it is assumed into a CI pipeline.

## 5. Two patterns worth stealing outright

**`test/assumptions/` (shaka-player).** A directory whose only job is to pin the assumptions the
code rests on. Their single file asserts how `Uint8Array` equality behaves. It looks trivial and
it is not: when a platform or a library changes an assumption underneath you, this fails
*there*, with a name, instead of surfacing as a mysterious behaviour change in a feature test.

We have an unusual number of these. A non-exhaustive list drawn from the decision log:
Go's `\w`/`\s`/`\b` are ASCII-only ([D49](../../decisions/working-notes.md));
`x/text/collate` is pinned at Unicode 6.2.0 / CLDR 23 ([D47](../../decisions/working-notes.md));
an FTS5 tokeniser is immutable once the table exists ([D48](../../decisions/working-notes.md));
SQLite's OR chain fails at 1000 terms and its join at 65 tables
([D75](../../decisions/working-notes.md));
`ass_track_set_feature` returns −1 when a feature is unavailable
([D50](../../decisions/working-notes.md));
FFmpeg's AC-3 decoder applies DRC by default at `drc_scale = 1`
([D83](../../decisions/working-notes.md));
NVENC ignores `-force_key_frames` ([transcoding.md](../plans/transcoding.md)).
Every one of those is a sentence in a document today and a silent breakage tomorrow.

**A skip list keyed by platform (hls.js).** Their `test-streams.js` describes 49 streams by URL
with per-stream flags, including `skip_ua` — *this stream is known not to work on that user
agent*. A structured, reviewable record of known-broken combinations beats a commented-out test,
and it is the natural home for what [D92](../../decisions/working-notes.md)'s failure reports
discover.

## 6. Measured: what a generated corpus costs, and what a VMAF gate costs

Both numbers were assumptions in the plan. Measured here on 2026-09-20 with a static FFmpeg
N-126655 build (see [performance-targets.md](performance-targets.md) §0 for the rig):

**A generated tier-2 corpus is seconds and megabytes.** Eight codec/container combinations —
`libx264`/AAC/mp4, `libx264`/AC-3/mkv, `libx265`/AAC/mp4, `mpeg4`/MP3/avi, VP9/Opus/webm,
AV1/Opus/mkv, MJPEG/PCM/mov, Theora/Vorbis/ogv — at two resolutions, 2 s each:

| | |
| --- | --- |
| clips | 16 |
| **generation time** | **8.9 s** |
| **total size** | **5.9 MB** |

So the answer to *"are synthetic files enough for most of it"* is that they cost **nine seconds
and six megabytes**, which removes the question. They need not even be committed: generating
them in the test run is cheaper than checking them out, and it keeps the generator — which is
the actual specification of what each clip is — in the repository instead of a binary.

**A VMAF gate cannot run on anything long.** `libvmaf` with 6 threads, comparing a 10-minute
1080p file against itself:

| | |
| --- | --- |
| VMAF, 10 min 1080p, 6 threads | **344.4 s** — **1.7× realtime** |

That is the most expensive operation measured anywhere in this project: slower per frame than
decoding, encoding or rendering subtitles. A quality gate therefore runs on **seconds of
video, not minutes**, which agrees with what §1 found independently — FATE's median sample is
58.7 KB, and nobody's corpus is long.

**And the cheap end is very cheap.** A remux to fMP4 with no re-encode is **0.28 s for a
10-minute film — 2,163× realtime** — so the conformance tests that matter most for
[D86](../../decisions/working-notes.md) (segment grid, IDR alignment, variant
interchangeability) are effectively free to run on every commit.

## 7. Measured: the sandbox may simply not be there

[D26](../../decisions/working-notes.md) confines FFmpeg with Landlock + seccomp and
[threat-model.md](../plans/threat-model.md) lists the per-platform confinement matrix as open.
Measured in this environment, a Linux 6.17 container:

```
landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION)  →  -1, errno 95 (ENOTSUP)
/proc/self/status: Seccomp: 2   Seccomp_filters: 2
```

Landlock is unavailable — and from inside it is **not possible to tell** whether that is a
kernel built without it, a boot parameter, or the container runtime's own seccomp profile
denying the syscall. Two conclusions, and the second is the important one:

1. **The confinement test must assert what is actually in force**, not that the code called the
   right function. "A sandbox nobody verified is a comment" was already in the plan; this adds
   that a sandbox nobody *probed at runtime* may not exist at all on the user's machine.
2. **The node must report its confinement state** ([D73](../../decisions/working-notes.md)), and
   the core must be able to say "this node is running FFmpeg unconfined" out loud. Docker is the
   most common deployment for this software, and Docker is exactly where this happens.

## 8. What the tiers should be, from the above

- **Tier 0 — no fixtures.** Pure functions: the filename parser, track-kind derivation,
  language normalisation, the predicate compiler, sort keys, pseudonymisation. Table-driven,
  thousands of cases, milliseconds.
- **Tier 1 — recorded JSON.** The core's whole interpretation layer, replayed from node reports
  and probe output. No FFmpeg, no filesystem, no media. This is Jellyfin's fifteen probe
  fixtures, generalised by [D59](../../decisions/working-notes.md) into the main event.
- **Tier 2 — generated clips.** FFmpeg `lavfi` sources, produced by the test run itself. Covers
  the node pipeline, the stream service, remux and subtitle extraction.
- **Tier 3 — a small real corpus, in-tree, chosen for pathology.** Navidrome's 512 KB, scaled to
  video: an AppleDouble file, a truncated container, a `.mkv` that is not Matroska, mismatched
  ISO 639-2 codes, a file with no runtime, an ASS with an embedded font, a 5.1 track with no
  loudness metadata.
- **Tier 4 — external, opt-in, announced.** FATE's model for anything needing real codec
  diversity, with the suite saying out loud which tests it is skipping.
- **Tier 5 — hardware.** [device-lab.md](../plans/device-lab.md) already owns this.

## 9. Sources

Read locally from the depth-1 clones on 2026-09-19/20: `jellyfin/tests/` (all 17 test
projects), `navidrome/tests/`, `hls.js/tests/`, `shaka-player/test/`,
`jellyfin-ffmpeg/tests/` (`Makefile`, `fate.sh`, `fate-run.sh`, `fate/`, `ref/`,
`refcmp-metadata.awk`).

Measured here on 2026-09-20: the FATE sample suite via `rsync -n --stats`; Landlock and seccomp
state via `syscall(444)` and `/proc/self/status`.

Web, 2026-09-20: VMAF JND guidance and the frozen-baseline argument; Apple's HLS Tools
documentation for `mediastreamvalidator`/`hlsreport` and their macOS requirement.
