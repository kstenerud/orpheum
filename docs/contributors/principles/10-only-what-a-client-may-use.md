> **ABOUTME:** Principle §10: Give a client only what it may use; a refusal is an alarm, not a
> branch.

# §10 — Give a client only what it may use; a refusal is an alarm, not a branch

If a client is not entitled to a resource, **omit it from the manifest** rather than serving a
URL and rejecting the request. The illegal action should be unreachable, not merely forbidden.

The server-side check still exists — a client is not a trust boundary — but its job changes. A
check that fires during normal operation is not enforcement, it is control flow in a security
costume, and it generates exactly the traffic that would hide a real attempt. A check that never
fires in a correct system is one whose every firing is worth looking at.

**This governs authorisation of our own cooperating client, and nothing else.** Validation of
untrusted input is the opposite case and *should* fire routinely: node reports arrive malformed
([D39](../decisions/working-notes.md), [D42](../decisions/working-notes.md)), and media is
attacker-supplied and FFmpeg's confinement is expected to be load-bearing
([D26](../decisions/working-notes.md)). Conflating the two leaves you with no signal in either,
because the security events drown in the normal ones.

**And it pays for something unexpected.** Because the manifest is *constructed per principal*
rather than filtered after the fact, "show me the library as this role sees it" is that same
construction with a different argument — so an admin can preview a restriction before
committing it, on the enforcement path rather than a simulation of it
([D132](../decisions/working-notes.md)). Neither Plex nor Jellyfin has that, and the reason is
visible in their shape: when entitlement is a check in a handler, there is nothing to run for a
principal who is not making the request.

**Bites when:** an entitlement is easier to express as a 403 in the handler than as a branch in
whatever builds the manifest. ([D58](../decisions/working-notes.md), from
[D57](../decisions/working-notes.md).)
