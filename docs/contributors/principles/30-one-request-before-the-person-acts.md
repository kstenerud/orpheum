> **ABOUTME:** Principle §30: What a person waits on comes in as few requests as it can, and what
> only looks better (posters, artwork) follows on a lower rung, eventually.

# §30 — What a person waits on comes in one request; what is only cosmetic follows

**Every request a person must wait on before they can act costs a round trip, and away from home
over WebRTC a round trip is long** (the owner, 2026-11-13: "As a general principle of client
server architecture, we want to reduce the number of requests that would prevent user operation.
This means getting as much as we can in one request rather than multiple, and then putting the
bigger data that's only cosmetic (such as posters) on a lower priority \"eventual consistency\"
rung."). The case that raised it: the alphabet rail's jump on Films waited on a request for the
letter's place and then on page after page of the list, though the owner had already asked for the
whole list's text up front (the plan file's 11-11 answers).

The rule:

> **What a person needs to act on (names, order, places, states, controls) comes in as few
> requests as it can, as early as it can; what only makes it look better (images, artwork,
> backdrops, people's photos) is fetched after, at a lower priority, and the page works without
> it.**

**The ideal: one page, one "essential data" fetch** (the owner, 2026-11-13: "Ideally one page does
one \"essential data\" fetch, and then other fetches for the rest as matches the situation."). A
page's first request answers everything the person needs to act on it; every other request follows
as the situation calls for it (images nearing the view, a menu opened, a section scrolled to), and
nothing a person acts on waits for those.

What that asks:

- **A page that can be read in one request is read in one,** not in a chain where each request
  waits for the one before; a list's text is one answer, not its pages in a row.
- **An action uses what the client already holds** before it asks the server; a press answered
  from memory makes no request.
- **The server's answer carries what the client would otherwise ask for next** (a place, a count,
  a letter), so the client does not come back for it.
- **Cosmetic data never blocks**: a poster still to come is drawn as a placeholder, and its fetch
  yields to anything a person waits on.
- **Too big for one request is the exception, measured**, and then split into a few large parts,
  not many small ones (the owner, 2026-11-11: "If we get issues with larger libraries, we could
  split up fetching to 1/2 or 1/3 of the library later").

The audit that applies it across the client and the server: the plan file's "The client-server
audit (the owner, 2026-11-13)".
