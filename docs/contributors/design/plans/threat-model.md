> **ABOUTME:** The security model we are actually building to — who the adversary is, what is
> exposed, and what we promise — which does not yet exist and should before anything listens on
> a port.

# Threat model

- **Status:** PLANNED — **researched 2026-09-18** ([threat-model.md](../research/threat-model.md), [D26](../../decisions/working-notes.md)). The containment stance is chosen; the per-platform confinement matrix and the signed-URL design remain open.
- **Depends on:** remote-access.md
- **See also:** [auth.md](auth.md) — identity, sessions and device pairing, split out 2026-09-19 ([D93](../../decisions/working-notes.md)); [content-safety.md](content-safety.md) — proactive detection, deliberately deferred ([D61](../../decisions/working-notes.md)), with four affordances due now.

## Why this is a gap and not a formality

We have documented Jellyfin's security failures in detail — a cluster of unauthenticated
endpoints, FFmpeg argument injection through `/Videos/{id}/stream` reaching arbitrary file write,
an unauthenticated `/System/Restart` reachable by spoofing a LAN IP, non-expiring dashboard API
keys — and its maintainers' position that the server should sit behind a VPN.

We have then designed a product whose **headline feature is letting non-technical family watch
from their own house** ([D14](../../decisions/working-notes.md)). That is the opposite posture,
and it means Jellyfin's threat model is not available to us. We have not written our own.

## What the research must settle

- **Who is the adversary?** The internet at large; a malicious guest with a shared link; a
  household member escalating; a hostile metadata provider or plugin; someone on the LAN.
- **What is the trust boundary for a media file?** Media is attacker-supplied data fed to
  FFmpeg, which is a large C codebase with a long CVE history. That is the sharpest edge in the
  product and it is the *normal* path, not an edge case. Sandboxing the decode/probe path
  deserves real consideration — and note it interacts with
  [D19](../../decisions/working-notes.md): the transcoder is already out-of-process.
- **Credential and token model.** Session lifetime, API key scoping and expiry, and the
  "Copy Stream URL leaks the session key" failure Jellyfin has open. The *shape* of signing in
  now has a home — [auth.md](auth.md), and device pairing is RFC 8628
  ([D93](../../decisions/working-notes.md)) — but the token model itself is still this
  document's question.
- **What a metrics endpoint exposes.** [observability.md](observability.md) adds one
  ([D91](../../decisions/working-notes.md)): off by default, localhost when on, never a label
  that can carry a title or a path. It is a new listener and belongs in the surface inventory.
- **What do shared links actually grant**, and can they be revoked? (Couples to
  `sharing-model.md`.)
- **What we promise.** [D13](../../decisions/working-notes.md) is a privacy promise; this is the
  security one, and it should be as specific.

## One constraint from elsewhere, so the signed-URL design does not collide with it

Signed, scoped and expiring URLs are the right answer for media access, with one shape
requirement: **sign the session, not each segment** ([D70](../../decisions/working-notes.md)).
Per-segment per-user signatures give every viewer a unique URL for identical bytes, which
defeats caching and any future fan-out silently. The manifest is per-user and authorised;
segment paths stay stable and content-addressed; the token rides where the cache key does not
look.
