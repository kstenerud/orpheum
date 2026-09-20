> **ABOUTME:** The numbers this server holds itself to, how they are shaped, and which of the
> guesses other decisions were making have now been measured — absent until 2026-09-20, which
> is how performance regressions become release-sized events.

# Performance targets

- **Status:** PLANNED for the server half — measured 2026-09-20 ([performance-targets.md](../research/performance-targets.md), [D113](../../decisions/working-notes.md)–[D116](../../decisions/working-notes.md)). The **client** half stays UNSPECIFIED and needs [device-lab.md](device-lab.md).
- **Depends on:** —

## Why it matters more than it sounds

Jellyfin 10.11's EF Core migration produced reported home-page loads of 30–50 seconds on large
libraries, collections taking 10–15 seconds that had been instant, and 16,000-item libraries
that would not load at all. That shipped because nothing said what "fast enough" was, so
nothing failed when it stopped being true.

We chose a database, an access layer and a query style partly to avoid it
([D6](../../decisions/working-notes.md), [D31](../../decisions/working-notes.md)). Those
choices are worth nothing without a number that fails.

## The shape of a target

[D113](../../decisions/working-notes.md). Five rules, and the shape matters more than the
values:

1. **A percentile at a stated concurrency.** Measured on a 100k-title database, the read p50
   moves 3× as readers go from 1 to 20 while the **p99 moves 36×**, from 2.81 ms to 101.70 ms.
   Every row of that table satisfies a target stated as an average.
2. **At two library sizes.** Every failure found was a scaling failure, invisible at one size.
3. **Against a frozen baseline, never the previous run**
   ([D110](../../decisions/working-notes.md)).
4. **As a multiple of a measured baseline** where the value is hardware-dependent, so one gate
   means something on CI and on a NAS.
5. **Observable in production** ([D91](../../decisions/working-notes.md)), because a gate that
   runs only in CI cannot notice the library growing — which is this document's whole reason
   for existing.

## What is measured, and what it says

Full detail in [performance-targets.md](../research/performance-targets.md). The headlines:

**Ten of twelve queries are flat from 10,000 to 100,000 titles** — keyset pagination over
materialised sort keys, the four-level detail page, cast lists, trigram search, and both
cross-level rules-engine predicates. A 100k library is a **234.8 MB** file, ≈2.35 KB per title
with translations, sort keys, credits, segments and the play log included, which is what makes
[D77](../../decisions/working-notes.md)'s automatic frequent backup a product decision rather
than an aspiration.

**The two that are linear are both on the home page**, and each cost one line:

| | 100k before | after |
| --- | --- | --- |
| Recently added, no index on `added_at` | 21.79 ms | **0.027 ms** (807×) |
| FTS match + `ORDER BY bm25()`, common term | 11.66 ms | **0.017 ms** unranked (686×) |

**bm25 ranking is linear in the number of matches, not results** — 6,589 matches scored to
return 20. Search latency tracks match count, and the target must fail when it does.

**The node's passes split into two populations a thousand-fold apart**
([D114](../../decisions/working-notes.md)), decided entirely by whether a pass decodes video.
Probe, remux, keyframe grid, thumbnail, fingerprint and subtitle overlay are 500–38,000×
realtime; trickplay and blackdetect are ~14×. On the reference library the cheap set is
**17 minutes to browsable** and about a day and a half complete; trickplay alone is 9.2 days
single-core.

## The targets

Initial, on the reference machine, at the reference library, to be re-based when real hardware
exists. Every one is **p99 at 5 concurrent sessions** unless stated.

| Target | Value | Measured baseline |
| --- | --- | --- |
| Library page, any depth | < 50 ms | 0.084 ms single-reader at 100k |
| Detail page, all four levels | < 50 ms | 0.050 ms |
| Continue Watching | < 50 ms | 0.055 ms |
| Search, worst-case common term | < 200 ms | 16.1 ms ranked at 100k |
| Rules-engine predicate, cross-level | < 250 ms | 3.7 ms |
| Progress report write | < 20 ms | 0.050 ms |
| Scan → library browsable and playable | < 1 h per 3,500 items | 17 min single-core |
| **Prioritised title → playable** ([D118](../../decisions/working-notes.md)) | **< 10 s from the request, mid-scan** | < 1 s of work, single-core |
| Prioritised title → fully enriched except trickplay | < 5 min | ~67 s single-core |
| Home page of 50 posters | **unset** | needs [artwork.md](artwork.md) |
| Time to first frame, direct play | **unset** | needs a server and a client |

The gap between target and baseline is deliberate and large: these are **regression gates**,
not performance goals. A number ten to a thousand times the measured baseline fails only when
something has genuinely broken, which is the only kind of gate that survives contact with CI.

## Thresholds other decisions were guessing at

Each is a number some decision needed. **Six are now measured.**

| Threshold | Needed by | Status |
| --- | --- | --- |
| Loudness-measurement cost, and whether it is deferrable | [D83](../../decisions/working-notes.md) | **Measured**: 102× realtime, audio-only, one core's work. 31 h for the reference library. Not the problem it was thought to be |
| Total first-scan cost across all passes | [D102](../../decisions/working-notes.md) | **Measured**: 17 min to browsable; ~36 h for the cheap set; 9.2 days for trickplay ([D114](../../decisions/working-notes.md)) |
| Content-hash cost | [D59](../../decisions/working-notes.md), [D100](../../decisions/working-notes.md) | **Measured**: sampling hash 3.2 ms vs 0.49 s whole-file, and 7 GB of reads vs 8 TB ([D116](../../decisions/working-notes.md)) |
| Subtitle-overlay render cost | [D78](../../decisions/working-notes.md) | **Measured**: 1.16 s per film at event granularity; 18.15 s if implemented by decoding the film |
| Search routing floor — where `LIKE` gives way to trigram | [D48](../../decisions/working-notes.md) | **Measured**: trigram is 0.106 ms at 100k; the crossover is not where the cost is. Ranking is |
| Snapshot and journal sizing | [D60](../../decisions/working-notes.md) | **Partly measured**: a 50k-file snapshot is **43.9 MB** (878 B/entry) and costs the core **1.8 s** to parse. A journal allowed past a few multiples of that should have triggered a snapshot |
| `snapshot_interval`, per root | [D60](../../decisions/working-notes.md), [D94](../../decisions/working-notes.md) | Open — and now also bounded by how often a spun-down array may wake |
| `journal_retention`, `journal_max_bytes` | [D60](../../decisions/working-notes.md) | Open; needs a real churn profile |
| Wedged-detection bounds | [D37](../../decisions/working-notes.md) | Open |
| Shrinkage guard | [D41](../../decisions/working-notes.md) | Open; needs a distribution of legitimate shrinkage |
| Sort keys per locale | [D47](../../decisions/working-notes.md) | Open, and cheap: sort keys are ~2% of the database |
| Playback-position drift per platform | [D66](../../decisions/working-notes.md) | Open; needs devices |
| Buffer reservoir for client re-picking | [D86](../../decisions/working-notes.md) | Open; needs a network |
| Play-event log retention horizon | [D81](../../decisions/working-notes.md) | Open; the write ceiling is 9,415 commits/s, so the constraint is disk, not throughput |
| Artwork sizes, format, cache ceiling | [artwork.md](artwork.md) | Open; needs the pipeline |

## The walk, and the mistake that is invisible locally

Measured on 50,000 files in 2,001 directories, warm, local: `WalkDir` with no stat is **28.6 ms**;
adding a `stat` per file makes it **76.5 ms**. **A 2.7× penalty is why the mistake ships.**
[scanner.md](scanner.md)'s rule — one round trip per directory, never a `stat` per file — is
invisible in a local benchmark and decisive over a share: at a 1 ms round trip that is 50,000
round trips against 2,001, **50 s against 2 s**. That last part is arithmetic, not
measurement, and measuring it properly needs a real SMB mount. It stays on the list.

## Still open

Time to first frame — the most important user-facing number and untouched by any of this.
Walk throughput over real SMB. Memory ceiling on a 2 GB machine (unmeasurable on a rig with
62 GB). Artwork. Hardware transcoding ([test-hardware.md](../research/test-hardware.md)).
Everything client-side ([device-lab.md](device-lab.md)).

## Done when

The gates above run in CI against frozen baselines and fail a real regression; the same names
appear on [observability.md](observability.md)'s `/metrics` endpoint so the target is visible
in production as well as in CI; and a scan of the reference library reaches "browsable" inside
the stated hour on the low-end reference machine.
