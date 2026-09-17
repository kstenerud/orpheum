> **ABOUTME:** Evaluating a Jellyfin-compatible API surface as the answer to the client
> cold-start problem — what it would buy, what it would cost, and whether to do it.

# Jellyfin API shim

- **Status:** UNSPECIFIED — the strategic question is open; no design yet.
- **Depends on:** library-model.md

## The question

A new server has no clients, and nobody writes a client for a server with no users. Navidrome
solves the mirror-image problem by implementing **both** the Subsonic API and a
Jellyfin-compatible API (`server/jellyfin/` — items, browsing, playlists, sessions, images,
lyrics, streaming, websocket) purely to inherit other people's clients.

If a compatibility shim over our model makes **Findroid, Swiftfin, Infuse, Streamyfin and
Wolphin** work against our server on day one, it is worth more than any client we could write
in the same time — and those are precisely the clients users already prefer to Jellyfin's own.

## What it would cost

- Our model must be expressible in Jellyfin's API, which is shaped by the `DeviceProfile`
  design we explicitly rejected (D2). The shim would have to *translate*: accept a device
  profile, and answer it from our manifest rather than from a capability database.
- Those clients will only ever expose our features as fast as they expose Jellyfin's.
- It is a compatibility surface with its own bug surface and its own test burden.

## The position to evaluate against

**Bridge, not foundation.** Our own clients speak our own API; the shim exists to make the
first year survivable and is allowed to be lossy. If it starts constraining the native API, it
gets frozen rather than extended.

## Resolve before

Starting `tv-client.md`. A working shim could make the first-party TV client a v2 concern
instead of a v1 one, which changes the whole schedule.
