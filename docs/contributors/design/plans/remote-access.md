> **ABOUTME:** The unsolved product problem — letting non-technical family watch from their own
> house without a third-party account and without configuring anyone's router.

# Remote access

- **Status:** UNSPECIFIED — **researched 2026-09-18** ([remote-access.md](../research/remote-access.md), [D25](../../decisions/working-notes.md)); the topology is settled and a tiered shape proposed, but no mechanism is chosen and the hosted-service question is a product decision, not a technical one.
- **Depends on:** —

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
