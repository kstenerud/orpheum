> **ABOUTME:** The numbers this server holds itself to, and how they are measured — absent
> today, which is how performance regressions become release-sized events.

# Performance targets

- **Status:** UNSPECIFIED — no numbers stated anywhere.
- **Depends on:** —

## Why it matters more than it sounds

Jellyfin 10.11's EF Core migration produced reported home-page loads of 30–50 seconds on large
libraries, collections taking 10–15 seconds that had been instant, and 16,000-item libraries
that would not load at all — with the root cause being client-side enumeration rather than
pushing filters into SQL. That shipped because nothing said what "fast enough" was, so nothing
failed when it stopped being true.

We have deliberately chosen a database, an access layer and a query style partly to avoid that
([D5](../../decisions/working-notes.md), [D6](../../decisions/working-notes.md)). Those choices
are worth nothing without a number that fails.

## What the research must settle

- **Which numbers.** Candidates: time to first frame for direct play and for a transcode; home
  page and library page at 10k / 100k items; search latency; full-scan and incremental-scan
  throughput; memory ceiling on a 2 GB NAS. **Add the home page's fifty posters**
  ([artwork.md](artwork.md)) — the most-made request in the product, and an image-serving
  benchmark rather than a metadata one.
- **And where they are measured from.** Targets enforced only in CI cannot notice that the
  library got slower as it grew, which is precisely how Jellyfin shipped a 30–50 second home
  page. [observability.md](observability.md)'s `/metrics` endpoint
  ([D91](../../decisions/working-notes.md)) is what makes a target observable in production,
  and the two documents should agree on names.
- **At what library sizes**, and where the reference libraries come from.
- **On what hardware.** A Raspberry Pi and an old Intel NUC are the honest low end for this
  audience; targets that only hold on a desktop are not targets.
- **How they are enforced.** A benchmark that nobody runs is documentation. Gate what can be
  gated in CI; schedule the rest.
- **Periodic-walk throughput**, promoted from a fallback to a requirement by
  [D27](../../decisions/working-notes.md): most installs cannot use native filesystem watching,
  so walking speed *is* the change-detection latency for them. Measure it **over a real SMB
  mount, not locally** — [D32](../../decisions/working-notes.md) notes the cost is round trips
  rather than bytes, so a local benchmark will be off by orders of magnitude and will hide the
  one mistake that matters (a `stat` per file instead of one listing per directory).
- **Trickplay and thumbnail generation**, which an HN user reports taking *days* for a large
  library on Jellyfin. That is a background job, so the target is throughput and
  interruptibility rather than latency.

## Thresholds other decisions are currently guessing at

Each of these is a number some decision needs and nobody has measured. They are listed here so
the guesses are in one place and get retired together.

| Threshold | Needed by | Guessing because |
| --- | --- | --- |
| `snapshot_interval` — how often a node re-enumerates | [D60](../../decisions/working-notes.md) | It is the correctness floor for a silently-dead watcher, so it wants to be short; a full enumeration over SMB is expensive, so it wants to be long |
| `journal_retention` — how long superseded journal is kept | [D60](../../decisions/working-notes.md) | Must survive a plausible core outage. Nearly free on a quiet library, so the cost is entirely churn-dependent and needs a real churn profile |
| `journal_max_bytes` — the disk-exhaustion valve | [D60](../../decisions/working-notes.md) | Depends on what a pathological rename loop actually produces |
| Wedged-detection bounds — how long without progress is stuck | [D37](../../decisions/working-notes.md) | The core learns a baseline by observation, but the initial tolerance is a guess |
| Shrinkage guard — how much loss is suspicious | [D41](../../decisions/working-notes.md), [D42](../../decisions/working-notes.md) | Needs a real distribution of legitimate library shrinkage |
| Search routing floor — where `LIKE` gives way to trigram | [D48](../../decisions/working-notes.md) | Measured at 1.5 ms worst case on 20k rows; the crossover on a real corpus is unknown |
| Sort keys per locale — how many locales a household uses | [D47](../../decisions/working-notes.md) | Cheap at one to three, annoying at ten |
| Playback-position drift per platform — sets the watch-together reconciliation interval | [D66](../../decisions/working-notes.md) | Derived from a 50 ppm clock assumption; playback-rate error probably dominates and is unmeasured |
| Loudness-measurement cost per file, and whether it is deferrable per library | [D83](../../decisions/working-notes.md) | An extra decode pass, free on a desktop and not free on a Celeron with 50k files |
| Buffer reservoir and upper threshold for client re-picking | [D86](../../decisions/working-notes.md) | Netflix's were tuned for a CDN and a planetary audience; ours is a 20 Mbps origin one hop away, or a relay leg with very different characteristics |
| `snapshot_interval` **per root**, derived from the setup probe | [D94](../../decisions/working-notes.md) | It is now also how often a spun-down array must wake, and an SSD root and a sleeping array want opposite answers |
| Play-event log retention horizon | [D81](../../decisions/working-notes.md) | The first table with an unbounded row count; cheap per row, unknown per year |
| Artwork: how many derived sizes, in what format, and the cache ceiling | [artwork.md](artwork.md) | A 20k-item library at three sizes in two languages is a real number nobody has computed; AVIF-vs-WebP is a device-lab result |
