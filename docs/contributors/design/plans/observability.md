> **ABOUTME:** Knowing the server is healthy before something breaks — a metrics endpoint, the
> three-view session join, and where they sit relative to the bug report, which is the surface
> for after it has already broken.

# Observability

- **Status:** PLANNED — designed 2026-09-19 ([D91](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §13).
- **Depends on:** —
- **See also:** [bug-reports.md](bug-reports.md) — the incident surface

## The evidence that this is wanted

Sorted by reaction count, the **second-most-reacted open issue in the entire Jellyfin server
tracker** — ahead of every security issue, every metadata bug and every feature — is
***Improve Prometheus Exporter with JF Metrics*** (+63, open since 2020-04).

That is unusual, and it says something specific about this audience: they already run
Prometheus and Grafana, and a self-hosted service that does not expose metrics is a blind spot
in a dashboard they already have.

## The gap it fills

We have two observability surfaces and both are **incident-shaped**:

| Surface | Answers | When |
| --- | --- | --- |
| [bug-reports.md](bug-reports.md) ([D74](../../decisions/working-notes.md)) | "what happened" | after it broke |
| status records ([D34](../../decisions/working-notes.md), [D73](../../decisions/working-notes.md)) | "is this component alive" | continuously, but consumed by the core |
| **`/metrics`** | **"is it healthy, and is it getting worse"** | **continuously, by the user** |

The third is missing, and its absence has a specific cost:
[performance-targets.md](performance-targets.md) can only be enforced in CI. A benchmark that
runs only in CI cannot notice that the library got slower as it grew — which is exactly how
Jellyfin 10.11 shipped a 30–50 second home page on large libraries.

## What it exposes

Nothing new is collected. [D73](../../decisions/working-notes.md) already has every component
reporting according to its own visibility, and this is that data in a format a dashboard eats:

- per-role health and uptime — core, each node, each relay;
- job queue depth and oldest-job age;
- scan and snapshot timings, per root, and staleness since last successful deep scan;
- stream counts, and **rendition decisions by outcome and reason** (direct play, remux, audio
  transcode, refused) — which is principles §3's explanation, aggregated;
- playback failures by reason ([D92](../../decisions/working-notes.md));
- error rates and validation-rejection counts at each boundary;
- database size, WAL size, and busy-retry counts.

## What it must never expose

**Nothing that names a film, a path, a person or a household member.** That is the same line
[D74](../../decisions/working-notes.md) draws for the bug report, and it is stricter here
because a metrics endpoint is scraped continuously and stored somewhere else. A label carrying
a title is a bug, enforced the same way D74's pseudonymisation is: a test, not a review habit.

Cardinality is the mechanism that makes this easy to get right — a per-item label would blow up
a Prometheus instance anyway, so the correct engineering and the correct privacy answer agree.

## Off by default, local when on

It is a data-exposure surface and [D13](../../decisions/working-notes.md) is a promise rather
than a preference. Default off; when enabled, bound to localhost unless the user explicitly
says otherwise; never on the same listener as the public API.

## The three-view session join

[D73](../../decisions/working-notes.md) already requires it and it belongs here: the core
issues a **session id**, and the client, the relay and the core each tag their observations
with it, so one playback session can be looked at from three independent vantage points. That
is the thing a content-reading relay could not give and a blind one can.

## Done when

A Grafana dashboard shows library size, scan staleness, stream counts and rendition reasons
with no plugin; enabling the endpoint is one setting and it is off by default; a test fails if a
metric label can contain a title or a path; and a slow home page is visible as a trend before it
is visible as a complaint.
