> **ABOUTME:** The security model we are actually building to — who the adversary is, what is
> exposed, and what we promise — which does not yet exist and should before anything listens on
> a port.

# Threat model

- **Status:** PLANNED — **researched 2026-09-18** ([threat-model.md](../research/threat-model.md), [D26](../../decisions/working-notes.md)). The containment stance is chosen; ~~the per-platform confinement matrix and the signed-URL design remain open~~ **the credential model closed by [D154](../../decisions/working-notes.md)** (no credential in a URL); **2026-09-23: FFmpeg is confined per invocation, the node's file channel is deleted, and Cut A adds an embedded TURN listener on one public HTTPS name** ([D193](../../decisions/working-notes.md) §4–§5). The confinement matrix per platform is what remains open.
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
  transport where that is impossible. ~~On a **file exchange directory** there is no transport
  to authenticate, so there the command file is **signed**, and the node verifies before
  obeying.~~ **The file exchange channel and its signing are deleted**
  ([D193](../../decisions/working-notes.md) §4): the one node transport is HTTP+JSONL over a
  Unix socket, with dial-out TCP later.
- **Adapters and who they act as** ([D105](../../decisions/working-notes.md)). A tier-2
  protocol adapter with a standing privileged token is a confused deputy that defeats every
  predicate in the project. The rule is that it is a login proxy, not a principal; the *token
  model* that makes it implementable ~~is this document's open question~~ **is
  [D154](../../decisions/working-notes.md)'s grant table**, where `adapter` is one of the kinds.
- **The image path, which D26 did not cover.** A poster fetched from TMDB is
  community-uploaded, arrives unprompted during a background refresh, and is decoded by
  something. [D99](../../decisions/working-notes.md) puts that decode in the confined worker
  with FFmpeg; the per-platform confinement matrix below has to cover it. **Confinement is now
  per FFmpeg invocation** ([D193](../../decisions/working-notes.md) §4, refining
  [D43](../../decisions/working-notes.md)): a re-exec shim applies, in rank order, argument
  hardening, read-only mounts, a seccomp denylist and Landlock where the kernel has it, and a
  node reports itself *unconfined* where it cannot ([D112](../../decisions/working-notes.md)).
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
  **And one hostile parser arrives from an unglamorous direction**
  ([D148](../../decisions/working-notes.md)): a **downloaded subtitle file**, which is
  anonymous third-party text the user asked us to fetch and then render. The interesting part
  is that it does not need an attacker to be malformed. **7 of 65 subtitle files in two
  *official* Blender releases needed tolerant parsing** — one is an HTML page saved as `.srt`,
  several write `- >` instead of `-->`, several put the cue text on the timestamp line — so
  the ordinary corpus already exercises the tolerant path, and an anonymous upload pool is
  strictly worse. Two consequences: the parser is a **parse-don't-validate boundary** like the
  report parser, with bounds on cue count and file size; and a strict parser's *zero cues* is
  a result the user sees as *"the download failed"*, which is half of the complaint the
  feature exists to fix ([subtitle-acquisition.md](subtitle-acquisition.md)).
- ~~**Credential and token model.**~~ **Answered 2026-09-21 by
  [D154](../../decisions/working-notes.md)** ([token-model.md](../research/token-model.md)) —
  this document's oldest open question, and the one five decisions said they were waiting on.
  **One opaque, hashed, revocable grant row** with a closed `kind` enum and a closed scope
  vocabulary; **no credential ever appears in a URL** — bytes take a path-scoped cookie, the
  API takes a header, and a URL meant to be exported gets a narrow `link` grant minted on
  purpose. That closes *"Copy Stream URL leaks the session key"* by construction: Jellyfin
  reads `?ApiKey=`/`?api_key=` and stores a never-expiring, unscoped, admin-equivalent token
  in plaintext; we read neither and store none. Sessions expire on **idle**, not absolutely;
  refresh tokens do not exist because revocation costs 4.2 µs.
- **The TURN listener** ([D193](../../decisions/working-notes.md) §4–§5). Cut A's voice runs
  through a TURN server embedded in our binary, on its TLS port under the one public HTTPS name.
  It is a new internet-facing listener and belongs in the surface inventory; an open relay is
  TURN's classic misconfiguration, so what it will relay for, and with what credential, is for
  this document to state.
- **What a metrics endpoint exposes.** [observability.md](observability.md) adds one
  ([D91](../../decisions/working-notes.md)): off by default, localhost when on, never a label
  that can carry a title or a path. It is a new listener and belongs in the surface inventory.
- **What do shared links actually grant**, and can they be revoked? (Couples to
  `sharing-model.md`.)
- **What we promise.** [D13](../../decisions/working-notes.md) is a privacy promise; this is the
  security one, and it should be as specific.

## One prohibition, from a feature the incumbent removed

[D150](../../decisions/working-notes.md), 2026-09-21. Jellyfin removed its `EasyPassword`
(PIN) in 10.9 as *"a big security risk especially for administrator accounts"* — and *PIN
protection for users* is simultaneously the **highest-scoring single feature request** in a
932-issue sweep of this field. Both facts are correct, because two different things wear the
same name.

**The rule: a PIN may gate a switch to a more-privileged profile on a device that is already
authenticated. It may never authenticate a session, issue or refresh a token, or admit an
administrator.** A four-digit secret has about 13 bits and no rate-limit story that survives a
LAN attacker; as a *selector* between identities already admitted on a trusted device it is
carrying no security weight at all, which is exactly why it is fine there.

This is a **fails-closed** assertion, not a guideline: no code path accepts a PIN as an
authentication factor ([testing-strategy.md](testing-strategy.md)), and the object it gates
is [D120](../../decisions/working-notes.md)'s person-plus-device rather than
[auth.md](auth.md)'s credential.

## The cast credential, where entropy is not the control

[D135](../../decisions/working-notes.md). A cast URL lets a television fetch its own bytes, and
the instinct is to make it unguessable. Necessary, and **not the threat on this surface**:

- **Cast's sender channel is unauthenticated.** The `deviceauth` handshake verifies that the
  *receiver* is a genuine Chromecast; nothing authenticates the sender, and any device on the
  LAN may connect on port 8009 and speak the protocol.
- **`MEDIA_STATUS` carries `contentId` and `contentUrl`.**

So the nearest adversary does not guess the URL — **they read it off the television.** A
CSPRNG token still earns its place against off-LAN guessing and against a URL surfacing in a
log or a screenshot, but the controls that carry the weight are **scope** and **expiry**:
whoever lifts it gets the thing currently playing on a screen they can already see, and
nothing else — no library, nothing addressable by id, nothing enumerable. And the credential
is the **cast session**, expiring on stop, on supersession by the next cast to that receiver,
on idle, or at an absolute cap.

Worth stating as the general lesson, because it will recur: **on a LAN surface, secrecy of a
URL is a weaker control than the smallness of what it grants.**

## One constraint from elsewhere, so the signed-URL design does not collide with it

Signed, scoped and expiring URLs are the right answer for media access, with one shape
requirement: **sign the session, not each segment** ([D70](../../decisions/working-notes.md)).
Per-segment per-user signatures give every viewer a unique URL for identical bytes, which
defeats caching and any future fan-out silently. The manifest is per-user and authorised;
segment paths stay stable and content-addressed; the token rides where the cache key does not
look.
