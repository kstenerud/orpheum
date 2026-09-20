> **ABOUTME:** Measurements of the actual workload — the four-level schema at 10k and 100k
> titles, the node's pass pipeline on real video, walk and report throughput — and the target
> shape those numbers imply. The evidence behind D113–D116.

# Performance research

Measured **2026-09-20**. The plan ([performance-targets.md](../plans/performance-targets.md))
said it *"cannot be finished without hardware"*, which was half right: the **client** numbers
cannot, and the **server** ones can, because the server's workload is a database and a decode
loop and both are here.

## 0. The rig, and how much to trust it

Everything below ran on an **Intel Core i3-1315U** (2 P-cores + 4 E-cores, 8 threads, 4.5 GHz
boost), 62 GB RAM, Linux 6.17, filesystem warm. That is a low-power laptop part — closer to a
modern NAS than a desktop is, and still faster than the 8 GB Celeron
[test-hardware.md](test-hardware.md) names as the honest low end.

Where it matters, **each measurement was also taken pinned to one core** (`taskset -c 0`) as a
weak-machine proxy. It is a proxy, not a NAS: it shares cache and memory bandwidth with an idle
system, and one i3 P-core is worth perhaps three of a Celeron's.

Database work used **Go with `modernc.org/sqlite` v1.59.0** — the actual candidate driver
([sqlite-reconsidered.md](sqlite-reconsidered.md) §4) — which is the *slower* pure-Go one, so
those numbers are conservative rather than flattering.

## 1. The schema at scale: two queries scale linearly and both are on the home page

A synthetic library was built to the real shape — four levels
([D79](../../decisions/working-notes.md)), translation rows and sort keys per locale
([D45](../../decisions/working-notes.md), [D47](../../decisions/working-notes.md)), both FTS5
tokenisers ([D48](../../decisions/working-notes.md)), people and credits
([D95](../../decisions/working-notes.md)), certifications
([D84](../../decisions/working-notes.md)), segments
([D82](../../decisions/working-notes.md)), per-track loudness
([D83](../../decisions/working-notes.md)), play state and the event log
([D81](../../decisions/working-notes.md)).

| | 10,000 titles | 100,000 titles |
| --- | --- | --- |
| database file | **23.0 MB** | **234.8 MB** |
| rows: titles / translations / sort keys | 10k / 20k / 20k | 100k / 200k / 200k |
| rows: credits | 120,000 | 1,200,000 |
| rows: versions / files / segments | 14,928 / 16,093 / 6,009 | 149,282 / 161,338 / 60,076 |
| ingest, one transaction | 1.04 s | 15.69 s |

≈ **2.35 KB per title**, everything included. A 100k-item library is a 235 MB file — which is
what makes [D77](../../decisions/working-notes.md)'s "back up automatically, frequently, on by
default" a product decision rather than an aspiration.

**Query latency, best of 20, warm, single reader:**

| Query | 10k | 100k | scaling |
| --- | --- | --- | --- |
| Continue Watching (20) | 0.051 ms | **0.055 ms** | flat |
| Library page 1, ordered by locale sort key (50) | 0.059 ms | **0.067 ms** | flat |
| Library keyset page at 50% depth (50) | 0.077 ms | **0.084 ms** | flat |
| Detail page — title, edition, version, file | 0.021 ms | **0.050 ms** | flat |
| Cast list for one title (12) | 0.035 ms | **0.082 ms** | flat |
| Trigram substring search (20) | 0.045 ms | **0.106 ms** | flat |
| Rule: `tag=horror AND ANY version ≥2160p` (500) | 1.974 ms | **2.294 ms** | flat |
| Rule: `tag=horror AND ALL versions ≥2160p` (500) | 3.074 ms | **3.729 ms** | flat |
| **Search, FTS5 + `ORDER BY bm25()` (20)** | 1.433 ms | **16.144 ms** | **linear** |
| **Recently added (50)** | 2.978 ms | **31.653 ms** | **linear** |
| Progress report (event insert + guarded update) | 0.018 ms | 0.050 ms | flat |

**Ten of twelve are flat from 10k to 100k.** The sort-key design
([D47](../../decisions/working-notes.md)) and keyset pagination hold exactly as designed: 10×
the data, no measurable change, including the cross-level rules-engine predicates with their
`ANY`/`ALL` quantifiers ([D79](../../decisions/working-notes.md)).

**The two that are not flat are both on the home page**, and both were fixed in one line each:

| | 10k | 100k | after |
| --- | --- | --- | --- |
| Recently added — no index on `added_at` | 1.915 ms | **21.790 ms** | — |
| Recently added — `CREATE INDEX ix_added ON title(added_at DESC)` | 0.025 ms | **0.027 ms** | **807× faster** |
| FTS match + `ORDER BY bm25()`, common term | 0.928 ms | **11.659 ms** | — |
| Same match, no bm25 ordering | 0.015 ms | **0.017 ms** | **686× faster** |

The search finding is the subtler one and worth stating as a rule: **bm25 ranking is linear in
the number of *matches*, not in the number of results returned.** The term used matched 6,589
of 100,000 rows; SQLite scores all 6,589 before taking twenty. The answer is not to drop
ranking — it is to know that search latency tracks match count, and to have a target that
*fails* when a common term makes it linear.

Both of these are invisible on a developer's thousand-item library and fatal at a hundred
thousand. That is Jellyfin 10.11's 30–50 second home page, reproduced in miniature and costing
one index.

## 2. Concurrency: the median is fine and the tail is the product

Eight-second runs against the 100k database, tight loops with no think time — a saturation
test, not a household:

| readers | writer | reads/s | read p50 | **read p99** | writes/s | write p99 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | no | 2,878 | 0.234 ms | 2.81 ms | — | — |
| 1 | yes | 1,210 | 0.401 ms | 7.02 ms | 483 | 5.96 ms |
| 4 | no | 3,152 | 0.379 ms | 14.72 ms | — | — |
| 4 | yes | 2,077 | 0.455 ms | 16.23 ms | 158 | 23.21 ms |
| 20 | no | 3,022 | 0.706 ms | 57.00 ms | — | — |
| 20 | yes | 1,622 | 0.766 ms | **101.70 ms** | 135 | 52.14 ms |

Three findings.

**Read throughput saturates at ~3,000 queries/s and does not improve with more readers.** One
reader gets 2,878/s; twenty get 3,022/s. That is Navidrome's comment — *"more connections buy
no write parallelism"* — extended: past a small number they buy no read parallelism either,
because the work is CPU-bound inside SQLite. The pool size is a floor, not a throttle.

**The p99 degrades 130× while the p50 barely moves** — 0.234 → 0.766 ms at the median, 2.8 →
101.7 ms at the tail. A target expressed as an average would be met throughout this table. So:
**every latency target is a percentile at a stated concurrency, or it is not a target.**

**Readers starve the writer.** 483 → 135 writes/s as readers go from 1 to 20. The writer is
progress reporting and scan ingest, and neither is allowed to fall behind because somebody is
browsing.

**The single-writer ceiling is 9,415 commits/s** (WAL, `synchronous=normal`, one transaction
per commit, unpaced). Against the ~2 writes/s
[sqlite-reconsidered.md](sqlite-reconsidered.md) §2 budgets for a household, that is **4,700×
headroom** — the claim was right and now it has a number.

## 3. The node's passes, measured on real video

A synthetic 10-minute 1080p H.264 + AC-3 5.1 file, 425 MB, 5.67 Mbps — a fair proxy for a
1080p web-dl.

| Pass | all cores | 1 core | × realtime |
| --- | --- | --- | --- |
| probe (`ffprobe`) | 0.01 s | 0.02 s | 38,000× |
| **remux to fMP4 (copy, no encode)** | **0.28 s** | — | **2,163×** |
| keyframe grid (packet probe) | 0.13 s | — | 4,500× |
| thumbnail (seek + 1 frame) | 0.14 s | — | 4,200× |
| **chromaprint fingerprint** (after stereo downmix) | **0.75 s** | — | **802×** |
| **subtitle overlay, event granularity** (1,200 events) | **1.16 s** | — | **515×** |
| loudness (`ebur128`) | 5.35 s | **5.89 s** | 102× |
| subtitle overlay, every frame at 24 fps | 14.01 s | — | 43× |
| full video decode (reference) | 15.29 s | **43.77 s** | 13.7× (1 core) |
| **trickplay** (1 frame/10 s @ 320 px) | 17.04 s | **41.91 s** | 14.3× (1 core) |
| subtitle overlay composited over decoded video | 18.15 s | — | 33× |
| **blackdetect** (segment candidates) | 18.41 s | **44.50 s** | 13.5× (1 core) |

**There are two populations and nothing in between: passes that decode every video frame, and
passes that do not.** The first cost about one full decode, because they *are* one full decode
plus a filter. The second cost between microseconds and a second. The gap is roughly **a factor
of a thousand**, and which side a pass lands on is the only thing about it that matters for
scheduling.

Three of those rows are corrections to assumptions in the tree.

**`ebur128` is audio-only, so it is one core's work.** 5.35 s on six cores against 5.89 s on
one — it does not parallelise within a file and does not need to, because it parallelises
across them. [D83](../../decisions/working-notes.md) worried this was "not free on a Celeron";
it is ~1% of the file's runtime, single-threaded.

**Chromaprint needs a stereo downmix first, and then it is cheap.** Run directly on the 5.1
track it fails outright — `Invalid argument`, no output — which is the kind of thing that is
discovered in production. With `-ac 2` in front of it: **0.75 s for ten minutes, 802×
realtime**, producing a 19,300-byte fingerprint. That **inverts the detector ordering in
[D82](../../decisions/working-notes.md)**, which listed chapter-derived first, then audio
fingerprinting, then "black-frame and silence detection" — reasonable on the assumption that
fingerprinting is the heavy one. It is not: it is **sixty times cheaper** than black-frame
detection, because one is an audio pass and the other is a video pass, and that is the only
thing that decides cost here.

**[D78](../../decisions/working-notes.md)'s overlay rendition costs about a second per film,
and the naive measurement is fifteen times worse.** Compositing subtitles over decoded video
is 18.15 s; rasterising onto a transparent canvas at 24 fps is 14.01 s; rendering **one frame
per event**, which is what D78 actually specifies — *"timed bitmaps cropped to the text
bounding box"* — is **1.16 s for 1,200 events.** The measurement D78 listed as outstanding is
therefore reassuring: server-side subtitle rendering is not an expensive rendition, it is a
free one, provided nobody implements it by decoding the film.

### Fusing a decode is worth 38–45%, and more on weak hardware

When two full-decode passes do coexist, running them as one FFmpeg invocation with a `split`
filter and multiple sinks beats running them separately:

| | separate | fused | saving |
| --- | --- | --- | --- |
| all cores (loudness + trickplay + blackdetect) | 38.08 s | **23.42 s** | **38%** |
| one core | 92.30 s | **50.97 s** | **45%** |

**The saving is larger on the machine that needs it more**, because each separate run re-pays
the decode and a weak machine pays more for each decode. That is the mechanism behind
[D102](../../decisions/working-notes.md)'s pipeline: the scheduler has to know which passes
share a decode.

### What that means for a first scan

For a reference household library — **500 films at 110 min and 3,000 episodes at 45 min**;
3,500 files, 3,167 hours of content, ≈8 TB at this bitrate — single-core, from the rates above:

| Work | single-core time |
| --- | --- |
| probe + keyframe grid + thumbnail, all 3,500 files | **17 minutes** |
| chromaprint fingerprint (whole library) | 4.0 hours |
| subtitle overlay renditions | ~70 minutes |
| loudness | 31 hours |
| **— everything above, together —** | **≈ 37 hours** |
| trickplay | **9.2 days** |
| blackdetect | **9.8 days** |

**The pipeline is not nine expensive passes. It is one expensive pass, one optional expensive
pass, and seven cheap ones.** Everything except trickplay and blackdetect totals about a day
and a half of single-core work on a 3,500-item library; those two are nineteen days between
them.

And the two are not equally necessary. They do **different** jobs — the fingerprint finds the
repeated opening theme across a season, black-frame detection finds credit and scene
boundaries where there is no repeated theme — but the *primary* intro detector is the cheap
one. So blackdetect is a **second-line pass for the cases fingerprinting misses**, not the
default, which leaves **trickplay as the only unavoidably expensive thing the node does.**

On a four-core NAS processing one file per core: the cheap set is about **9 hours**, trickplay
about **2.3 days**.

Three conclusions, all of them [D102](../../decisions/working-notes.md)'s, now with numbers:

- **The library is browsable and playable after 17 minutes.** Everything after that is
  enrichment, and ordering passes by user value is the difference between "usable this
  afternoon" and "usable next week".
- **A pass's cost is decided entirely by whether it decodes video.** That is the question to
  ask of any tenth pass, and the answer is a factor of a thousand.
- **Trickplay should be the deferrable pass by default** — it is the only one whose absence
  costs a scrub preview rather than a feature, and it is 60% of the total.

### And the hash pass is the one that wakes the disks

Hashing throughput on the 425 MB file:

| | rate | time | vs library (8 TB) |
| --- | --- | --- | --- |
| SHA-256, whole file | 860 MB/s | 0.49 s | 2.6 h CPU |
| CRC-32, whole file | 2,928 MB/s | 0.15 s | 0.8 h CPU |
| **1 MB head + 1 MB tail + size** | — | **0.0032 s** | **11 s** |

The CPU cost is irrelevant; **the 8 TB of reads is not**. At a spinning disk's ~150 MB/s that
is roughly 15 hours of pure I/O with every drive awake, which is
[D94](../../decisions/working-notes.md)'s problem in its sharpest form. The sampling hash is
**153× cheaper** than SHA-256 and reads 2 MB instead of 8 GB per file — and it is exactly the
right instrument for [D100](../../decisions/working-notes.md)'s "name what you are deleting by
identity", which needs *this is the same file*, not *no other file is like it*.

## 4. Walking, and the mistake that is invisible locally

50,000 files in 2,001 directories, warm cache, local disk:

| | time | entries/s |
| --- | --- | --- |
| `WalkDir`, no stat (`getdents` only) | **28.6 ms** | 1,750,000 |
| `WalkDir` + `d.Info()` per file | 74.2 ms | 674,000 |
| `WalkDir` + `os.Stat(path)` per file | 76.5 ms | 654,000 |
| `filepath.Walk` (lstats every entry) | 76.7 ms | 652,000 |
| directory mtimes only (pruned-walk probe) | 30.5 ms | — |

**Locally the mistake costs 2.7×**, which is precisely why it ships.
[scanner.md](../plans/scanner.md)'s rule — *one round trip per directory, never a `stat` per
file* — is invisible in a local benchmark and decisive over a share.

The share case is **arithmetic, not measurement**, and is labelled as such: at a 1 ms round
trip, a `stat` per file is 50,000 round trips ≈ **50 s**; one listing per directory is 2,001
round trips ≈ **2 s**. Twenty-five times, before any bytes move. Measuring this properly needs
a real SMB mount and remains open.

## 5. Node reports

50,000 entries with the real record shape — verbatim path, NFC path, size, mtime, container,
duration and five streams with language, title and disposition:

| | |
| --- | --- |
| snapshot size | **43.9 MB** (878 B/entry) |
| write | 412 ms (121,000 entries/s) |
| **parse** | **1,754 ms** (28,500 entries/s) |

So a 50k-file snapshot costs the core **about 1.8 s of JSON parsing** to ingest, and a node's
snapshot is tens of megabytes. That sizes `journal_max_bytes` and the exchange directory's
retention ([D60](../../decisions/working-notes.md)) against something real for the first time:
the snapshot is the floor, and a journal allowed to grow past a few multiples of it is a
journal that should have triggered a snapshot instead.

## 6. What the targets should look like

The numbers above argue for a particular *shape*, which matters more than the values:

1. **Percentile at a stated concurrency.** §2 shows a workload where the p50 moves 3× and the
   p99 moves 36×. A target without both numbers stated is unfalsifiable.
2. **Against a frozen baseline, not the last run.** Borrowed from VMAF practice
   ([testing-strategy.md](testing-strategy.md) §3): build-to-build tolerances ratchet, and six
   passing builds can hide a doubling.
3. **Two library sizes, because the interesting failures are the linear ones.** Everything in
   §1 that broke, broke by scaling. A target measured only at 10k would have passed both.
4. **Expressed as a multiple of a measured baseline** where the absolute value is
   hardware-dependent, so the same gate is meaningful on CI and on a NAS.
5. **Observable in production, not only in CI** ([D91](../../decisions/working-notes.md)) —
   a gate that only runs in CI cannot notice the library growing, which is the failure mode
   this whole document exists for.

## 7. Still open, and what it would take

- **Time to first frame.** Needs the server, a client and a network. The single most important
  user-facing number and the one nothing here touches.
- **Walk throughput over a real SMB mount** (§4) — arithmetic is not measurement.
- **Memory ceiling on a 2 GB machine.** Nothing here was memory-constrained; 62 GB of RAM makes
  that unmeasurable on this rig.
- **Artwork**: derived-size counts, format choice and the cache ceiling
  ([artwork.md](../plans/artwork.md)) — needs the image pipeline to exist.
- **Hardware transcoding throughput** — [test-hardware.md](test-hardware.md) owns it.
- **Everything client-side** — [device-lab.md](../plans/device-lab.md) owns it.

## 8. Method

Benchmarks are in the working cache, not the repository: a Go program building and querying
the schema with `modernc.org/sqlite` v1.59.0; a Go walk benchmark; a Go JSONL and hashing
benchmark; Python harnesses driving a static FFmpeg N-126655 build (`libass`, `libvmaf`,
`chromaprint`, x264/x265) over a `lavfi`-generated 10-minute 1080p file. Each latency figure is
the best of 20 warm runs; each media figure is a single wall-clock run of a process that
dominates the machine. Library-scale figures in §3 are **arithmetic on measured rates**, stated
as such.
