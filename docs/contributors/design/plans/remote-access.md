> **ABOUTME:** The unsolved product problem — letting non-technical family watch from their own
> house without a third-party account and without configuring anyone's router.

# Remote access

- **Status:** PARTLY SPECIFIED — **adaptation gap identified 2026-09-19** ([D86](../../decisions/working-notes.md)); researched 2026-09-18 ([remote-access.md](../research/remote-access.md), [D25](../../decisions/working-notes.md)); NAT traversal and the relay shape settled 2026-09-19 ([D68](../../decisions/working-notes.md), [D71](../../decisions/working-notes.md)). **The mechanism is now chosen**; the hosted-service question remains a product decision, not a technical one.
- **Depends on:** —

## The mechanism: a `relay` role the core dials out to

Settled by [D71](../../decisions/working-notes.md). A third mode of the same binary, run on any
publicly reachable host — your VPS, or eventually ours.

**The property that matters is not the relaying, it is the direction.** The core dials *out*, so
there is **no port forwarding, no dynamic DNS, no UPnP, and it works behind CGNAT** — which is
the case nothing else solves. Port forwarding is the largest support burden in self-hosted
anything, so this is plausibly the recommended setup rather than a fallback for hard networks.

Two modes: **tunnel** (a byte pipe; TLS passes through, the relay terminates nothing, reads
nothing, cannot tamper; bandwidth is `N ×` the stream) and **cache** (the
[D70](../../decisions/working-notes.md) fan-out; terminates HTTP to see segment URLs while
payloads stay encrypted end to end, so it caches ciphertext; origin bandwidth `1 ×`). **Build
tunnel mode**; cache mode stays speculative.

It is a role, so it arrives with a status record, observation-based liveness, hardened units and
the report-don't-exit rule already specified ([D34](../../decisions/working-notes.md),
[D37](../../decisions/working-notes.md), [D43](../../decisions/working-notes.md),
[D44](../../decisions/working-notes.md)). It is **untrusted by construction** — a rented box —
so clients authenticate the *core*, never the relay. Place it **near the home server**: the
core↔relay link is persistent and carries everything.

**The economics are the argument.** Plex's own support page now says accessing personal video
remotely *"is not available for free"*, and its Relay is reported capped at 1 Mbps free / 2 Mbps
paid — heavy transcoding by construction. A €5 VPS with 20 TB carries roughly **2,200 film-hours
a month at 10 Mbps, uncapped**: about five times the bitrate of Plex's paid relay, for the price
of a coffee. The difference is household versus platform economics, not cleverness.

**If we ever host one: no free tier, no data, no accounts**
([D72](../../decisions/working-notes.md)). Blind and stateless, so a box is cattle and there is
no backup, restore or migration story; clients authenticate the core, so the relay needs no user
directory and inherits none of the password-reset, account-recovery or data-protection surface;
payment identity stays separate from access.

**Blind is not opaque** ([D73](../../decisions/working-notes.md)). The relay reports everything
it *can* see — throughput, connection state and its transitions, RTT per leg, drops, error
codes — through its status record, **to the household's core, even when we operate the box**.
Client telemetry is piped through, since the relay is already the path. So the core joins three
independent views of one session: the client's, the relay's and its own. That is *better*
diagnostics than a content-reading relay would give, because the part a blind relay cannot see
is the part that would not have helped — nobody diagnoses stuttering by looking at video
frames.

Two requirements follow. **The core issues a session id** and the client and relay tag their
observations with it, or the three views are three unrelated logs — cheap now, awkward later.
And the line that keeps the README's "no telemetry" true while operating a service:
**session-scoped and identifying observations go to the core, never to us; only machine health
— CPU, disk, uptime, aggregate throughput and error rates — is the operator's.** If a metric
cannot be aggregated until it is about the box rather than about a person, it is not ours. Payment is also the abuse control, which
[the research](../research/remote-access.md) §5 identified as the binding constraint. And the
hosted tier must stay **trivially exitable** — same open-source binary, no data to migrate —
which is a structural commitment rather than a promise, and the only honest answer to "what
stops you becoming Plex".

**And the obvious shortcut is forbidden.** Cloudflare's terms: *"Cloudflare reserves the right to
disable or limit your access to or use of the CDN … if you use or are suspected of using the CDN
without such Paid Services to serve video or a disproportionate percentage of pictures, audio
files, or other large files."* The documentation must say so, rather than letting people find
out by being banned. `frp`, `rathole` or a plain VPS are fine.

## The thing that does not work yet, said out loud

**Direct play over a WAN has no adaptation** ([D86](../../decisions/working-notes.md)). The
rendition is chosen once, at the start; if the link degrades, playback stalls. On a LAN that is
fine and is the whole point. Over a relay, on a hotel Wi-Fi, it is the common case.

Three consequences, none of them optional:

- **Buffer-driven re-picking** ([stream-service.md](stream-service.md)). The client already
  picks the rendition ([D2](../../decisions/working-notes.md)) and already knows its own buffer
  level, so the adaptation signal Netflix's own SIGCOMM'14 study says to use — buffer occupancy,
  not bandwidth estimation — is already in the right place. What is missing is that it re-picks.
- **There has to be something lower to pick.** That is [transcoding.md](transcoding.md) §(b),
  which is scheduled at Phase 4 and is in fact on this document's critical path. An install with
  no transcoder registered can only offer the original, and must **say so** through the
  transcode-reason field rather than letting the user discover it in a hotel.
- **Variants must be segment-aligned** or a downshift is a reposition
  ([D23](../../decisions/working-notes.md)). The mechanism is already specified in
  `transcoding.md`; it is a requirement of *this* document.

**Bandwidth limits per grantee** follow from the same place and are unspecified: the relay's
economics (~2,200 film-hours a month at 10 Mbps on a €5 VPS) assume nobody is streaming a
30 Mbps remux to five houses. A ceiling per share is a rules-engine-adjacent setting and it is
the honest alternative to discovering the bill.

**One option worth naming for the technical user:** mTLS / client certificates, requested on
findroid's tracker (+28). It is not an answer for a family — nothing requiring a certificate
install ever is — but it costs a paragraph in the documentation and it is strictly better than
the "just use a VPN" answer we reject elsewhere, for the people who can use it.

## Why it matters more than its size suggests

It is simultaneously the thing Plex users most resent — a cloud account mediating access to a
server in their own house — and the single thing keeping them on Plex. Verbatim from the
research: "the sore point right now is outside access, which Jellyfin really isn't made for...
I can't give friends and family access, either, which was dead easy with Plex", and "Tailscale
is great for me, but not feasible for family".

The self-hosted field's uniform answer is "set up a VPN". That is not an answer for the
audience this project is for.

## What the research round covered, and how it came out

Done — [research/remote-access.md](../research/remote-access.md), 2026-09-18, extended
2026-09-19. Kept here because the *answers* are what the rest of this plan rests on:
UPnP/NAT-PMP port mapping and why it is unreliable; NAT traversal and hole punching (§3: a
single hard NAT is beaten by port prediction in about two seconds, two of them need ~170,000
probes per side); WireGuard-based meshes and what they demand of the *remote* user; relay
through a rendezvous server; the CGNAT case; and certificates and names for a server with no
domain, which turns out to need a signpost even on the LAN.

The constraint that made it hard: **[D13](../../decisions/working-notes.md) promises no
required third-party account**, and most mechanisms that work for non-technical users involve
a third party somewhere. **It resolved without weakening either side**, which was not
guaranteed: the dial-out relay is a mode of our own binary
([D71](../../decisions/working-notes.md)), so the third party is *optional and self-hostable*
by construction, and the README's promise was restated honestly as *"we would only ever charge
for things that genuinely require another machine"* rather than as a flat "no accounts ever"
([D25](../../decisions/working-notes.md)). What is still open is above and in
[overview.md](../overview.md) §Open problems: whether to operate a hosted one at all, which is
a product decision.
