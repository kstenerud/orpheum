> **ABOUTME:** Why every player's "download subtitles" button produces a list of guesses and a
> coin-flip result, and the measurement that removes the guess — 128 candidate evaluations
> across two films, 64 real subtitle files, zero misclassifications.

# Subtitle sync: the guess, and what replaces it

Researched and measured **2026-09-21**. Backs
[D147](../../decisions/working-notes.md) and [D148](../../decisions/working-notes.md).
Prompted by the project owner describing the experience in every player:

> *"They offer to download the subtitles through whatever servers they're internally
> configured to grab from, you get a list of them with some names more similar to your exact
> filename than others (not that a normal user would know what the original filename actually
> was). You have no idea which ones work, try them one-at-a-time until one of them actually
> successfully downloads (about half of them fail to download), then it's either out of sync
> right off the bat, or drifts over time, or maybe — just maybe — it actually works."*

Every clause of that is a separate defect, and they have separate causes. The important
finding is that **the field already contains the fix, measured and packaged, and does not run
it** — and that the reason it does not is a design choice, not a missing capability.

---

## 1. Why the list is a list of guesses

**The ranking function in this field scores filenames, not sync.** Bazarr — the tool everyone
uses, and the one [tech-metadata.md](tech-metadata.md) already names — carries a vendored
`subliminal` whose entire candidate score is metadata
(`custom_libs/subliminal_patch/score.py`, an earlier commit, 2026-09-15):

| Movie term | Points | Episode term | Points |
| --- | --- | --- | --- |
| `hash` | **179** | `hash` | **359** |
| `title` | 60 | `series` | 160 |
| `year` | 40 | `year` | 90 |
| `source` | 30 | `season` / `episode` | 30 each |
| `edition` | 30 | `source` | 25 |
| `release_group` | 15 | `release_group` | 20 |
| `audio_codec`, `resolution`, `video_codec`, `hearing_impaired`, `streaming_service` | 1 each | *(same)* | 1 each |

`hash` is deliberately set to `sum(everything else) − 1`, so **a file-hash match outranks every
other combination** — correct, because it means the uploader had the identical file. But when
there is no hash match, and for a re-encode or a differently-sized rip there never is, the
ranking is **entirely derived from the uploader's filename**: title, year, source, release
group.

**Not one term in either column measures whether the subtitle lines up with the audio.** That
is exactly the list the project owner describes — *"some names more similar to your exact
filename than others"* — and the metric the user actually cares about contributes **zero** to
the ordering. The user is then asked to do the search by hand, one download at a time, against
a budget: [tech-licensing.md](tech-licensing.md) records OpenSubtitles at **5 downloads/day
anonymous, 20/day with a free account**.

### 1a. The hash that would fix it is free, and we already read the bytes

`hash` outranks everything because it means the uploader had the identical file. Read from
`custom_libs/subliminal/utils.py`:

```python
def hash_opensubtitles(video_path):          # file size + sum of little-endian int64s
    filehash = filesize                      # over the FIRST 64 KiB and the LAST 64 KiB
    for _ in range(65536 // 8): ...          # truncated to 64 bits
    f.seek(max(0, filesize - 65536), 0)
    ...
def hash_thesubdb(video_path):               # md5(first 64 KiB + last 64 KiB)
    readsize = 64 * 1024
```

**Both are a strict subset of the window our own content hash already reads.**
[D116](../../decisions/working-notes.md) measured the sampling hash — **1 MB head, 1 MB tail
and the size** — at 3.2 ms per file and 7 GB across the whole reference library, against 8 TB
for whole-file hashing. The provider hashes are arithmetic over bytes already in memory:
**no second read, no extra pass**. The open cost question in the plan was a non-question.

## 2. Why about half of them fail to download

Not an anecdote — it is designed for. Bazarr ships **68 provider modules** and a
`PROVIDER_THROTTLE_MAP` keyed on failure type, with per-provider back-offs:

| Exception | Typical back-off |
| --- | --- |
| `DownloadLimitExceeded` | **3–6 hours** |
| `TooManyRequests` | 1 minute – 1 hour |
| `ServiceUnavailable` | 20 minutes |
| `ParseResponseError` | 6 hours |
| `IPAddressBlocked` | per provider |

A tool does not build a typed back-off table with multi-hour penalties for a failure mode that
happens occasionally. **Download failure is the normal case being managed**, and the user sees
it as "about half of them fail" because the UI surfaces the attempt rather than the budget.

## 3. Why the ones that download are out of sync — and the measured rate

`alass`'s author measured it directly, on random OpenSubtitles downloads:

> *"Downloading random subtitles from `OpenSubtitles.org` had an error rate of about **50%**
> (sample size N=118)."*

Two distinct defects hide under "out of sync", and they need different corrections:

- **A constant offset.** Different release, different intro length, a rip that starts at a
  different frame. One number fixes it.
- **Drift.** Almost always a **framerate conversion**: a 23.976 fps subtitle against a 25 fps
  PAL transfer runs **4.27% fast** — *seven seconds off by the two-hour mark, and perfect at
  the start*, which is exactly what "drifts over time" feels like. One ratio fixes it.
- **A structural break** — an ad-break-segmented broadcast rip, or a different cut. No single
  offset or ratio fixes it; it needs a piecewise alignment, which is what `alass`'s split
  penalty and `ffsubsync`'s `--split-penalty` exist for.

## 4. The fix already exists, is already vendored, and is off by default

**Bazarr bundles `ffsubsync`** — the whole package, at `libs/ffsubsync/`. It is MIT-licensed.
And then (`bazarr/app/config.py`):

| Setting | Default | Effect |
| --- | --- | --- |
| `subsync.use_subsync` | **`False`** | **automatic syncing is off** |
| `subsync.no_fix_framerate` | **`True`** | even when on, **the drift fix is off** |
| `subsync.use_subsync_threshold` | `False` | |
| `subsync.subsync_threshold` | `90` (episode) | |
| `subsync.subsync_movie_threshold` | `70` (movie) | |
| `subsync.gss` | `True` | continuous framerate search |
| `subsync.max_offset_seconds` | `60` | |

And the gate in `bazarr/subtitles/sync.py` is:

```python
if not settings.subsync.use_subsync and not force_sync:
    return False
...
if not use_subsync_threshold or (use_subsync_threshold and percent_score <= float(subsync_threshold)):
```

Read that carefully, because it is the whole finding:

1. **Syncing is opt-in**, in a separate program most users never install.
2. When enabled, it is **conditioned on the *metadata* score being poor** — sync is a *repair
   for a bad-looking match*, not a *check on every download*.
3. **The framerate fix — the drift half of the complaint — is separately off.**
4. **The result of syncing is never fed back into which subtitle to keep.** Nothing measures
   the candidates and reorders them.

So the capability is present, packaged, and free, and it is wired in as a repair tool behind
two disabled switches instead of as the selection criterion. **That is the gap.**

## 5. Why nobody uses the score to rank: it is not comparable

There is a real technical reason, and `ffsubsync` states it in its own source
(`ffsubsync/constants.py`):

> *"The score's sign is meaningful even though its magnitude is not, so 0.0 rejects only
> anti-correlated alignments."*

Its aligner maximises a raw FFT convolution peak, whose magnitude scales with how much speech
and how many cues there are. You can compare two alignments **of the same pair**; you cannot
compare **two candidates** with it. Hence `DEFAULT_MIN_SCORE = 0.0` — the only threshold the
magnitude supports is "not actively anti-correlated".

**A normalised score fixes that**, and that is the piece worth building. The rest of
`ffsubsync`'s design is worth adopting wholesale:

| `ffsubsync` constant | Value | Why it matters to us |
| --- | --- | --- |
| `SAMPLE_RATE` | **100** | speech is modelled at **10 ms** bins |
| `FRAMERATE_RATIOS` | `24/23.976`, `25/23.976`, `25/24` | the discrete drift set, searched before anything continuous |
| `MIN/MAX_FRAMERATE_RATIO` | `0.9` / `1.1` | bound on the continuous search |
| `DEFAULT_MAX_OFFSET_SECONDS` | `60` | bound on the offset search |
| `DEFAULT_MAX_SUBTITLE_SECONDS` | `10` | one long cue cannot dominate |
| `DEFAULT_VAD` | **`subs_then_webrtc`** | **prefer an existing subtitle track over audio** |
| `--serialize-speech` → `.npz` | | the speech map is **already a storable artefact**, and a `.npy`/`.npz` may be passed as the reference |

That last row matters more than it looks: the tool already treats the speech map as something
you compute once and keep. See [D148](../../decisions/working-notes.md).

---

## 6. What we measured

**Rig:** Intel i3-1315U (6c/8t), 62 GB, Linux 6.17.4, ffmpeg N-126655, Go 1.26.2,
onnxruntime 1.30.0, single-threaded unless stated.

**Corpus.** Two open films with official multi-language subtitle sets published by their
producers, which gives a positive set (many independent human timings against *the same*
audio) and a negative set (each film's subtitles against the *other* film's audio):

| | Duration | Subtitle files | Cues |
| --- | --- | --- | --- |
| **Sintel** (Blender, 2010) — animated, continuous orchestral score, sparse dialogue | 888 s | 41 | 26 |
| **Tears of Steel** (Blender, 2012) — live action, dense dialogue | 734 s | 24 | 76 |

Sintel additionally publishes a **music-and-effects master with the dialogue removed**
(`sintel-m+e-st.flac`), which is a perfect control: any detector that reports speech on that
track is reporting music.

### 6.1 Real subtitle files are not well-formed — 7 of 65, from official releases

Before any alignment: **7 of 65 files** from *the producers' own distributions* needed
tolerant parsing.

- **`sintel_fi_ep.srt` is an HTML page.** It begins `<!-- saved from url=(0040)... --><html><body><pre …>`. A strict parser yields **zero cues**.
- **Several use `- >` instead of `-->`** as the cue separator (`TOS-PT-BR.srt`), which a literal `-->` match silently drops to zero cues.
- **Several put the text on the same line as the timestamp** (`sintel_nl.srt`, `sintel_no.srt`, `sintel_mk.srt`, `sintel_cn.srt`), with no newline between.

If official releases are 11% irregular, an anonymous upload pool is worse. **A subtitle parser
is an adversarial-input parser**, and "it failed to download" in the field is sometimes "it
downloaded and parsed to nothing". This belongs with
[threat-model.md](threat-model.md)'s hostile-data-to-the-node rule.

### 6.2 The detector is the whole game, and energy is not a detector

The alignment score is a normalised binary correlation (a φ coefficient) between the
subtitle-on indicator and a speech map, maximised over the discrete ratio set and every offset
in ±60 s at 10 ms. **Separation** below is the worst score any *right*-film subtitle got, minus
the best score any *wrong*-film subtitle got: positive means a threshold exists, and the size
is the margin.

| Speech map | Speech fraction | Worst right | Best wrong | **Separation** | offset error |
| --- | --- | --- | --- | --- | --- |
| Sintel, downmix, **energy VAD** | 0.42 | 0.004 | — | **fails** | rails at 60 s |
| Sintel, downmix, **webrtcvad mode 3** | 0.361 | 0.106 | 0.193 | **−0.087** | rails at 60 s |
| Sintel, **centre channel**, webrtcvad | 0.256 | 0.149 | 0.147 | **0.002** | 0.89 s |
| Sintel, downmix, **silero** | 0.027 | 0.365 | 0.143 | **0.222** | 0.26 s |
| **Sintel, centre channel, silero** | 0.035 | **0.428** | 0.118 | **0.309** | **0.26 s** |
| ToS, downmix, webrtcvad mode 3 | 0.119 | 0.302 | 0.113 | 0.188 | 0.07 s |
| ToS, centre channel, webrtcvad | 0.167 | 0.488 | 0.164 | 0.324 | 0.03 s |
| ToS, downmix, silero | 0.082 | 0.444 | 0.211 | 0.232 | 0.17 s |
| **ToS, centre channel, silero** | 0.136 | **0.547** | 0.161 | **0.387** | **0.13 s** |

Three things fall out:

- **A plain energy VAD is worthless on scored content.** On Sintel it marked **42%** of the
  film as speech and the alignment ran to the edge of the search window on every file. The
  control proves why: run the same detector on the **dialogue-free** music-and-effects master
  and it reports essentially the *same* speech fraction as the full mix. It was measuring the
  orchestra.
- **`webrtcvad` is not enough either** — on the dialogue-free master it still reports
  **31.5%** speech, and on Sintel its separation is **0.002**, i.e. none. It is fine on ToS,
  where the dialogue is dense and the score is quieter. **The field's default detector works
  on the easy half of the content.**
- **`silero` reports `0.000` speech on the dialogue-free master.** It is detecting voice, not
  energy, and it is the difference between working and not working on Sintel.

**And the centre channel is free accuracy.** Dialogue is mixed to `FC` on every 5.1 master;
`-af "pan=mono|c0=FC"` costs nothing at decode and improves separation on both films and both
detectors — Sintel 0.222 → 0.309, ToS 0.232 → 0.387.

### 6.3 Correction is lossless, whatever was wrong

Taking only the files that align, injecting a known corruption, and re-running the full search.
*Residual* is the **worst** cue in the file after correction, not the median cue:

| Injected | Ratio recovered | Worst-cue residual (ToS) | (Sintel) |
| --- | --- | --- | --- |
| nothing | 23/24 | **0.130 s** | 0.886 s |
| offset +12.7 s | 24/24 | **0.130 s** | 0.889 s |
| offset −7.3 s | 24/24 | **0.130 s** | 0.889 s |
| PAL 25/23.976 | 23/24 | **0.130 s** | 0.420 s |
| offset **and** PAL | 24/24 | **0.128 s** | 0.422 s |
| 0.08% drift, *not in the ratio set* | 0/24 | 0.293 s | 0.486 s |

**A corrupted subtitle corrects to the same accuracy as one that was never corrupted.** Offset,
framerate, or both — it makes no difference to the result. The only row that degrades is the
drift rate deliberately chosen to be outside the discrete set, and even that stays under
0.3 s; it is the case `--gss` exists for.

*(Sintel's residual is uniformly ~0.9 s and **improves to 0.42 s when a PAL ratio is
injected** — the published Sintel timings are themselves slightly rate-mismatched against this
audio master, and the corrector finds it. Real, and a reminder that "the shipped subtitle" is
not ground truth.)*

### 6.4 The ranking simulation — the result that matters

Every file from both films offered as a candidate for one target, with **half the right-film
candidates deliberately desynced** by a random offset in ±45 s *and* a random framerate ratio.
Fixed acceptance threshold **r ≥ 0.25**:

| Target | Candidates | Accepted right | Accepted wrong | Worst right | Best wrong |
| --- | --- | --- | --- | --- | --- |
| Sintel | 64 (40 right, 20 of them desynced; 24 wrong) | **40/40** | **0** | 0.427 | 0.118 |
| Tears of Steel | 64 (24 right, 12 of them desynced; 40 wrong) | **24/24** | **0** | 0.547 | 0.161 |

**128 candidate evaluations, zero misclassifications, one fixed threshold, a 3.4–3.6× margin
between the worst accepted and the best rejected.** The top-1 pick for Sintel was a file that
had been desynced by −10.9 s with a 0.959 ratio — and that is *fine*, because it is corrected
before it is served.

Which is the real conclusion: **once correction runs first, desync stops being a quality signal
at all.** It is not a property of the candidate; it is a property of nothing. The only question
left is *does this subtitle belong to this film*, and that question has a clean answer.

### 6.5 An existing subtitle track is a better reference than the audio, and free

Aligning against another correct subtitle's cue timings instead of a speech map:

| Reference | Right film, worst | Wrong film, best | **Separation** |
| --- | --- | --- | --- |
| `sintel_nl.srt` (26 cues) | 0.824 | 0.169 | **0.655** |
| `TOS-en.srt` (76 cues) | 0.795 | 0.147 | **0.647** |

**Twice the margin of the audio path, and it needs no decode whatsoever.** Whenever the file
already carries one correct subtitle track — which for anything with embedded subtitles is
most of the library — this is the reference to use. `ffsubsync` reaches the same conclusion
from the other direction: its default VAD is literally named **`subs_then_webrtc`**.

### 6.6 Cost

| | Measured | Extrapolated to a 2-hour film |
| --- | --- | --- |
| decode to mono 16 kHz s16le | 0.43 s for 888 s = **2,065× realtime** | 3.5 s |
| `webrtcvad` pass | **6,800–8,800× realtime** | 1 s |
| **`silero` pass** (onnx, CPU, 1 thread) | **233–387× realtime** | **~29 s** |
| **full search** — 7 ratios × ±60 s at 10 ms = 84,007 evaluations | **3.0 ms** (26 cues) / **7.9 ms** (76 cues) | unchanged |

**Storage of the speech map**, at 10 ms bins:

| | Sintel (888 s) | ToS (734 s) | Per 2-hour film | Per 1,000-film library |
| --- | --- | --- | --- | --- |
| bit-packed | 11,100 B | 9,176 B | 87.9 kB | 90 MB |
| **run-length encoded** | **143 B** | **331 B** | **1.1–3.2 kB** | **1.2–3.2 MB** |

Put beside the artefact budget [D114](../../decisions/working-notes.md) measured over the same
reference library — **trickplay 5.7 GB, overlays 42 GB, posters ~1 GB** — a run-length-encoded
speech map for the whole library is **3 MB**. It is the cheapest artefact in the pipeline by
three orders of magnitude, and it turns every later sync question into a **3–8 ms** array
operation with no decode at all.

Against [D114](../../decisions/working-notes.md)'s two-population finding, the speech pass is
squarely in the cheap class: it never touches the video stream. Sized against the same
reference library: [D114](../../decisions/working-notes.md) measured loudness at **31 hours**
at **102× realtime**, which puts **≈3,160 hours of content** in that library. Decode and
detector compose to **≈223× realtime**, so a whole-library first pass is **≈14 hours
single-threaded, ≈2.5 hours on six cores** — against **9.2 days** for trickplay. The stored
maps for that library total **≈7 MB**.

---

## 7. What this says to do

1. **Measure, do not rank by filename.** The score belongs in the candidate ordering, not in a
   repair tool behind two off-by-default switches.
2. **Normalise the score** so candidates are comparable — `ffsubsync` says in its own source
   that its magnitude is not, and that single property is what blocks ranking.
3. **Correct before offering, not after complaining.** Offset and framerate correction is
   lossless (§6.3), so a desynced candidate is not a worse candidate.
4. **Use the best reference available**: an existing correct subtitle track (§6.5) → the
   **centre channel** of the audio → the downmix.
5. **Use a real VAD.** `webrtcvad` fails on scored content and it is the field's default;
   silero is 250× realtime and does not.
6. **Keep the speech map.** 3 kB per film, and every later question is 8 ms.
7. **A low score is an answer**, not a failure. Sintel under a weak detector scored ~0.15 on
   *everything* — the system can tell it does not know, and refusing is the correct output.
8. **Parse defensively.** 11% of official releases are irregular; one is an HTML page.
9. **Emit the provider hashes from the existing read.** They are a subset of the sampling
   hash's window (§1a), so the one ranking term the field already gets right costs us nothing.

## Sources

- **Bazarr** `morpheus65535/bazarr` @ an earlier commit (2026-09-15) — `custom_libs/subliminal_patch/score.py`, `bazarr/app/config.py`, `bazarr/subtitles/sync.py`, `bazarr/subtitles/tools/subsyncer.py`, `bazarr/app/get_providers.py`, `libs/ffsubsync/`
- **ffsubsync** `smacke/ffsubsync` (MIT) — `constants.py`, `aligners.py`, `speech_transformers.py`, README
- **alass** `kaegi/alass` — README, including the N=118 OpenSubtitles error-rate measurement and the post-sync accuracy distribution
- **silero-vad** `snakers4/silero-vad` — `silero_vad.onnx`, run under onnxruntime 1.30.0
- **Corpus** — Blender Foundation, *Sintel* (2010) and *Tears of Steel* (2012): audio masters from `media.xiph.org` and `download.blender.org`, subtitle sets from the same
- Harness: `/yoloai/cache/subsync/` — `go/` (parser, speech map, aligner, experiments), `vadmap.py`, `silmap.py`, `results.txt`
