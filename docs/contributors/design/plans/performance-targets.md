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

**And then a third one arrived with a new feature** ([D133](../../decisions/working-notes.md),
2026-09-21), which reframes this section. The pending-items alert renders on the launch screen,
and at 100,000 titles with 7.7% uncovered the anti-join **did not complete in 30 s** without a
supporting index, ran at **43.6 ms** with a covering one — already missing the 50 ms budget at
a *single* reader — and **0.063 ms** as a maintained counter. **692×.**

So the finding here is not "two home-page queries scale linearly". It is that **every new
surface on that screen rediscovers the hazard**, and this one was not found by an audit — it
arrived attached to a feature request.

**That is now [principles §11](../../principles/README.md)** — *the home page computes
nothing*: anything on the launch screen is a maintained value, updated on the rare event that
changes it and read as a lookup. The invalidation is not the hard part and that is why the
rule can be absolute: these values change only on a scan completing, a rule or role changing,
a tag edit or a metadata refresh, and [D166](../../decisions/working-notes.md)'s change journal
already says exactly what changed. Event-driven invalidation on machinery that exists.

The fourth instance was **prevented rather than discovered**: `rules-engine.md`'s per-rule
coverage count ([D134](../../decisions/working-notes.md)) was specified as a maintained value
before anyone wrote the query.

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
| **Poster grid, 50 films, watched badges rolled up File → Title** ([D185](../../decisions/working-notes.md)) | < 50 ms | **Measured**: **0.84 ms** computed live (p95 1.49 ms); **0.91 ms** through the shared `item` table ([D186](../../decisions/working-notes.md)); a cache would save 0.5 ms and is not built. Badly shaped, **7.2 ms** — the shape is the risk |
| Album grid at **500,000 tracks** ([D186](../../decisions/working-notes.md)) | < 50 ms | **Measured**: 0.35 ms with the listing first in the sort key; **9.4 ms** without |
| Continue Watching | < 50 ms | 0.055 ms |
| Search, worst-case common term | < 200 ms | 16.1 ms ranked at 100k |
| Rules-engine predicate, cross-level | < 250 ms | 3.7 ms |
| Progress report write | < 20 ms | 0.050 ms |
| Scan → library browsable and playable | < 1 h per 3,500 items | 17 min single-core |
| **Prioritised title → playable** ([D118](../../decisions/working-notes.md)) | **< 10 s from the request, mid-scan** | < 1 s of work, single-core |
| Prioritised title → fully enriched except trickplay | < 5 min | ~67 s single-core |
| Home page of 50 posters | **unset** | needs [artwork.md](artwork.md) |
| Pending-items alert, per role ([D133](../../decisions/working-notes.md)) | **< 5 ms**, on the home page | **Measured**: 0.063 ms maintained; 43.6 ms as a query, which fails |
| Time to first frame, direct play | **unset** | needs a server and a client |
| **Focus latency** — D-pad press to selection moving ([D138](../../decisions/working-notes.md)) | **unset**, worst device in the house | needs [device-lab.md](device-lab.md); the complaint it answers is *"lags by an entire second"* |
| **Click depth** — home to playing a chosen title ([D138](../../decisions/working-notes.md)) | **unset** | a count, not a latency; the complaint is *"one click instead of 30"* |
| **Subtitle candidate scoring** ([D147](../../decisions/working-notes.md)) | **< 100 ms** for ten candidates | **Measured**: 3–8 ms each for the full ±60 s × 7-ratio search, given a stored speech map |
| **Speech-map pass**, per 2-hour film ([D148](../../decisions/working-notes.md)) | **< 2 min** | **Measured**: ~32 s — 2,065× realtime to decode, 233–387× for the detector |
| **Collection page: Next Up over the largest realistic sequence** ([D160](../../decisions/working-notes.md)) | < 50 ms | **Measured**: **14.8 ms** computed at 976 members (every Star Wren). Flatten alone is 1.2 ms and nesting depth is free |
| **Continue Watching row for a collection**, on the home page | **< 5 ms** | **Measured**: **0.01 ms** maintained; 14.8 ms computed, which fails [D134](../../decisions/working-notes.md) |

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
| Subtitle-verification acceptance threshold | [D147](../../decisions/working-notes.md) | **Measured**: `r ≥ 0.25` gave 64/64 accepted right-film and 0/64 wrong-film over two films. Needs re-basing against a real provider pool, where the negatives are other cuts of the *same* film rather than a different film |
| Speech-map storage, per library | [D148](../../decisions/working-notes.md) | **Measured**: 1–3 kB per film RLE; **1.2–3.2 MB per 1,000 films**, against trickplay's 5.7 GB |

## The walk, and the mistake that is invisible locally

Measured on 50,000 files in 2,001 directories, warm, local: `WalkDir` with no stat is **28.6 ms**;
adding a `stat` per file makes it **76.5 ms**. **A 2.7× penalty is why the mistake ships.**
[scanner.md](scanner.md)'s rule — one round trip per directory, never a `stat` per file — is
invisible in a local benchmark and decisive over a share: at a 1 ms round trip that is 50,000
round trips against 2,001, **50 s against 2 s**. That last part is arithmetic, not
measurement, and measuring it properly needs a real SMB mount. It stays on the list.

## Still open

**The artefact channel, which is the payload** ([D128](../../decisions/working-notes.md)). A
50,000-entry node report is **50.8 MB** and moves in ~550 ms on any transport; the *derived
artefacts* over the reference library are **42 GB of subtitle overlays and 5.7 GB of trickplay**
(measured 30,040 B per minute of video). Unmeasured: the cost of a redirect against a direct
serve on a home page, which is the number deciding where D128 draws the line between "stays on
the node" and "uploaded to the core".

Time to first frame — the most important user-facing number and untouched by any of this.
Walk throughput over real SMB. Memory ceiling on a 2 GB machine (unmeasurable on a rig with
62 GB). Artwork. Hardware transcoding ([test-hardware.md](../research/test-hardware.md)).
Everything client-side ([device-lab.md](device-lab.md)).

## Done when

The gates above run in CI against frozen baselines and fail a real regression; the same names
appear on [observability.md](observability.md)'s `/metrics` endpoint so the target is visible
in production as well as in CI; and a scan of the reference library reaches "browsable" inside
the stated hour on the low-end reference machine.
