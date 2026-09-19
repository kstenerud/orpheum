> **ABOUTME:** The unsolved product problem — letting non-technical family watch from their own
> house without a third-party account and without configuring anyone's router.

# Remote access

- **Status:** PARTLY SPECIFIED — researched 2026-09-18 ([remote-access.md](../research/remote-access.md), [D25](../../decisions/working-notes.md)); NAT traversal and the relay shape settled 2026-09-19 ([D68](../../decisions/working-notes.md), [D71](../../decisions/working-notes.md)). **The mechanism is now chosen**; the hosted-service question remains a product decision, not a technical one.
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
payment identity stays separate from access. Payment is also the abuse control, which
[the research](../research/remote-access.md) §5 identified as the binding constraint. And the
hosted tier must stay **trivially exitable** — same open-source binary, no data to migrate —
which is a structural commitment rather than a promise, and the only honest answer to "what
stops you becoming Plex".

**And the obvious shortcut is forbidden.** Cloudflare's terms: *"Cloudflare reserves the right to
disable or limit your access to or use of the CDN … if you use or are suspected of using the CDN
without such Paid Services to serve video or a disproportionate percentage of pictures, audio
files, or other large files."* The documentation must say so, rather than letting people find
out by being banned. `frp`, `rathole` or a plain VPS are fine.

## Why it matters more than its size suggests

It is simultaneously the thing Plex users most resent — a cloud account mediating access to a
server in their own house — and the single thing keeping them on Plex. Verbatim from the
research: "the sore point right now is outside access, which Jellyfin really isn't made for...
I can't give friends and family access, either, which was dead easy with Plex", and "Tailscale
is great for me, but not feasible for family".

The self-hosted field's uniform answer is "set up a VPN". That is not an answer for the
audience this project is for.

## What the research round must cover

The mechanisms and their tradeoffs: UPnP/NAT-PMP port mapping and why it is unreliable; NAT
traversal and hole punching; WireGuard-based meshes and what they demand of the *remote* user;
relay through a rendezvous server and what that implies for a no-account promise; the CGNAT
case; certificates and names for a server with no domain; and how each degrades when it fails.

The constraint that makes it hard: **D13 promises no required third-party account**, and most
of the mechanisms that work well for non-technical users involve a third party somewhere. The
research must find out whether an *optional*, self-hostable rendezvous is enough, or whether
the promise and the feature are in genuine tension — and if they are, say so plainly rather
than quietly weakening one.
