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
- **The node channel, in both directions** ([D107](../../decisions/working-notes.md)).
  [D35](../../decisions/working-notes.md) states the security model for the file transport
  precisely and says nothing about the HTTP one it prefers. The dangerous direction is
  core→node: `config.json` carries commands and, since
  [D89](../../decisions/working-notes.md), deletion. Mutual authentication, enrolment as a
  deliberate act, and **roots that are local and never remotely settable**.

  **[D127](../../decisions/working-notes.md) removes most of this surface rather than guarding
  it.** If the node always dials out — the shape GitHub's runners, AWS's SSM agent and Tdarr's
  node all converged on — then core→node **stops existing as a network path**: a node with no
  inbound surface cannot be written to, only answered. What remains to guard is the one
  transport where that is impossible. On a **file exchange directory** there is no transport to
  authenticate, and D35's *"the permissions split is the security model"* assumes an operator
  careful enough to lock down a share that a typical home NAS leaves writable to the whole LAN.
  So there the command file is **signed**, and the node verifies before obeying. See
  [node-transport.md](../research/node-transport.md) §5b.
- **Adapters and who they act as** ([D105](../../decisions/working-notes.md)). A tier-2
  protocol adapter with a standing privileged token is a confused deputy that defeats every
  predicate in the project. The rule is that it is a login proxy, not a principal; the *token
  model* that makes it implementable is this document's open question.
- **The image path, which D26 did not cover.** A poster fetched from TMDB is
  community-uploaded, arrives unprompted during a background refresh, and is decoded by
  something. [D99](../../decisions/working-notes.md) puts that decode in the confined worker
  with FFmpeg; the per-platform confinement matrix below has to cover it.
- **What is the trust boundary for a media file?** Media is attacker-supplied data fed to
  FFmpeg, which is a large C codebase with a long CVE history. That is the sharpest edge in the
  product and it is the *normal* path, not an edge case. Sandboxing the decode/probe path
  deserves real consideration — and note it interacts with
  [D19](../../decisions/working-notes.md): the transcoder is already out-of-process.

  **State the boundary precisely, because the convenient summary is false**
  ([D126](../../decisions/working-notes.md)). "Hostile data goes to the nodes, never the core"
  is the right instinct and not literally true. **The core never *parses* a media or image
  file** — but it *fetches* the poster ([D99](../../decisions/working-notes.md)), so hostile
  bytes transit its memory undecoded; and it parses **filenames, track titles, probe output and
  50.8 MB of node report per scan**, by design, because
  [D59](../../decisions/working-notes.md) exists to make that work replayable with every node
  switched off. What makes that safe is different machinery — Go, RE2
  ([D75](../../decisions/working-notes.md)), parse-don't-validate at the boundary, and
  [D49](../../decisions/working-notes.md)'s Unicode discipline, since the failure here is a
  silent misparse rather than a crash. **The report parser is the unbudgeted part**: hostile
  JSON on the hot path needs stated bounds on record size, count and nesting depth, and has
  none.
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
