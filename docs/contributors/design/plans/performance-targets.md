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
  throughput; memory ceiling on a 2 GB NAS.
- **At what library sizes**, and where the reference libraries come from.
- **On what hardware.** A Raspberry Pi and an old Intel NUC are the honest low end for this
  audience; targets that only hold on a desktop are not targets.
- **How they are enforced.** A benchmark that nobody runs is documentation. Gate what can be
  gated in CI; schedule the rest.
- **Periodic-walk throughput**, promoted from a fallback to a requirement by
  [D27](../../decisions/working-notes.md): most installs cannot use native filesystem watching,
  so walking speed *is* the change-detection latency for them.
- **Trickplay and thumbnail generation**, which an HN user reports taking *days* for a large
  library on Jellyfin. That is a background job, so the target is throughput and
  interruptibility rather than latency.
