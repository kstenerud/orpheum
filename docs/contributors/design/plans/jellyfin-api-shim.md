> **ABOUTME:** Evaluating a Jellyfin-compatible API surface as the answer to the client
> cold-start problem — what it would buy, what it would cost, and whether to do it.

# Jellyfin API shim

- **Status:** PLANNED — designed 2026-09-17 as an out-of-process adapter ([D18](../../decisions/working-notes.md)); identity and play-state semantics settled 2026-09-19 ([D105](../../decisions/working-notes.md), [D106](../../decisions/working-notes.md)).
- **Depends on:** stream-service.md

## The question

A new server has no clients, and nobody writes a client for a server with no users. Navidrome
solves the mirror-image problem by implementing **both** the Subsonic API and a
Jellyfin-compatible API (`server/jellyfin/` — items, browsing, playlists, sessions, images,
lyrics, streaming, websocket) purely to inherit other people's clients.

If a compatibility shim over our model makes **Findroid, Swiftfin, Infuse, Streamyfin and
Wolphin** work against our server on day one, it is worth more than any client we could write
in the same time — and those are precisely the clients users already prefer to Jellyfin's own.

## Shape: an out-of-process adapter

Not core, and not an in-process plugin. A **separate process** speaking our public REST API and
owning its own listener (D15's second plugin tier). In this repository, shipped separately,
optional.

Consequences of that shape:

- It constrains our model **not at all**. It translates what it can and returns honest failures
  for what it cannot; its limitations are visibly its own.
- It can be developed, versioned, shipped and **abandoned** independently. A bug in it cannot
  take the server down.
- It is written against exactly the API a third party would use, which keeps that API honest.
- Byte-streaming is a **redirect** to the core's media URLs, never a proxy. No gigabytes pass
  through the adapter.

## The part that makes it more than a bridge

Jellyfin clients negotiate playback with a `DeviceProfile` — the design D2 rejects. The adapter
is therefore where a device-profile evaluator has to live: it accepts a profile and answers it
from our rendition list rather than from a capability database.

**That puts the 8,000-line problem permanently outside the core, in a component we are free to
abandon**, while still getting Findroid, Swiftfin, Infuse, Streamyfin and Wolphin working. It is
the best available outcome: the clients without the architecture.

## It acts as the user, never as itself

[D105](../../decisions/working-notes.md). [D15](../../decisions/working-notes.md) gives an
adapter *"its own auth"*, which leaves open what it authenticates **to the core** as — and the
convenient answer, one privileged token configured at install, is a confused deputy that hands
the whole library to anything that can speak Jellyfin's API at it. [D76](../../decisions/working-notes.md)
requires the share predicate at *every* access path; an owner-token adapter defeats it, and
because the core sees a legitimate owner request, nothing fires.

So the adapter is a **login proxy, not a principal**: credentials are forwarded to the core,
the core authenticates, and the adapter holds a token **scoped to that user** for that session.
Every downstream request carries it, so entitlement is resolved once by the core and the
adapter cannot get it wrong because it never computes it. A media redirect carries the *end
user's* session authorisation ([D70](../../decisions/working-notes.md)). And the core records
that the request arrived via an adapter — needed for
[bug reports](bug-reports.md), for [metrics](observability.md), and because
[principles §10](../../principles/README.md) treats an adapter as cooperating for *omission*
and less trusted for *alarms*: its users run software we did not write.

## Two places the translation is lossy, and both must be documented

- **Play state has no sequence number to carry** ([D106](../../decisions/working-notes.md)).
  [D81](../../decisions/working-notes.md)'s monotonic guard has no field in Jellyfin's progress
  API, and treating "no sequence" as "newest" reinstates last-writer-wins for exactly the
  clients most likely to be a second device. The adapter **mints** the sequence per
  `(user, device)`. Two adapter-driven devices are then ordered only by arrival — the ceiling
  of the protocol, not a defect of ours, and it must be said in the adapter's documentation
  because the symptom (a phone rewinding a television) is otherwise indistinguishable from the
  bug D81 exists to prevent.
- **Four levels flatten into Jellyfin's one.** The mapping is forced and is written down once:
  an **Edition** is a Jellyfin item, a **Version** is one of its "alternate versions", and a
  multi-part **File** set is `AdditionalParts` — Jellyfin's own vocabulary used for what
  [D79](../../decisions/working-notes.md) says it was reaching for. The first thing to test is
  whether real clients tolerate alternate versions with *different runtimes*, which ours can
  have across Editions.

## Still true: bridge, not foundation

Our own clients speak our own API. The adapter is allowed to be lossy. If it ever starts
constraining the native API, it gets frozen rather than extended.

Under D17 it can only offer what the core can produce, so a profile demanding a transcode we
cannot perform gets an honest failure rather than a broken stream — a correctness improvement
over guessing, and another reason the transcode-reason field must exist in v1.

## Open: whether it changes the client schedule

A working adapter could make the first-party TV client a later concern than planned. That trade
should be re-examined **once the adapter exists and its real fidelity is known**, not assumed
now in either direction.
