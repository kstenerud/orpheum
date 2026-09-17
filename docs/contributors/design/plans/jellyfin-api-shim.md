> **ABOUTME:** Evaluating a Jellyfin-compatible API surface as the answer to the client
> cold-start problem — what it would buy, what it would cost, and whether to do it.

# Jellyfin API shim

- **Status:** PLANNED — designed 2026-09-17 as an out-of-process adapter ([D18](../../decisions/working-notes.md)).
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
