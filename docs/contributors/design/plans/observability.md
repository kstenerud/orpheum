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
- **items awaiting categorisation, and the age of the oldest**, per role
  ([D132](../../decisions/working-notes.md)). A restricted role hides anything its rules cannot
  judge, so this queue is the thing that silently stops a child's library growing if nobody
  drains it. A count and an oldest-age, never a per-item notification — the first scan would
  otherwise emit thousands on day one ([D118](../../decisions/working-notes.md)). Note this is
  the *secondary* surface: [D133](../../decisions/working-notes.md) puts the same number on the
  launch screen, because the person who will actually chase it is the one being blocked by it.
  **It is read from a maintained counter** — measured, the query form is 43.6 ms at 100k
  against 0.063 ms maintained, so scraping it must not become a home-page regression;
- database size, WAL size, and busy-retry counts.

## And there is no push half, deliberately

[D149](../../decisions/working-notes.md) proposed one and
[D151](../../decisions/working-notes.md) withdrew it the next day. Everything here is **pull**,
and the rule above — *"a count and an oldest-age, never a per-item notification"* — turns out
to be the right rule for the whole product and not only for this endpoint.

The reason is worth keeping where a future reader will hit it: **a notification is worth
sending when someone is waiting on an asynchronous outcome they cannot see**, which is why
Sonarr, Radarr and Overseerr all have one and why their notification demand is real. A media
server serves what is already there, so every state change it could announce is already a row
on a screen. Measured, notifications drew **8 reactions across the ten video repositories**,
against 479 for subtitles.

The one question a UI genuinely cannot answer — *is the server reachable from outside?* — is an
**outside-in health check** rather than a notification, and it belongs to
[remote-access.md](remote-access.md), where it collides with
[D72](../../decisions/working-notes.md).

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
