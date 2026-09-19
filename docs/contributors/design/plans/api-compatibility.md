> **ABOUTME:** The contract between a server that updates whenever the user pulls and clients
> that update when an app store and a television feel like it — additive change, deprecation
> windows, a declared floor, and a CI gate that fails a breaking change.

# API compatibility

- **Status:** PLANNED — designed 2026-09-19 ([D85](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §7).
- **Depends on:** —

## Why this is not premature

Jellyfin's 12.0 cycle is the cautionary case, and the source on master (13.0.0, read
2026-09-19) is more instructive than the write-ups, because they did **two different things**
to two legacy surfaces:

| Surface | What happened | Verdict |
| --- | --- | --- |
| `/emby/` and `/mediabrowser/` route prefixes | **gone** — no occurrence in `Jellyfin.Api` or `Jellyfin.Server`; a client calling them gets a 404 mid-operation | a break |
| `X-Emby-Authorization`, and the `Emby` scheme name | **behind `EnableLegacyAuthorization`** — a bool with no initialiser, so off by default and recoverable with one setting | a deprecation |

Nothing in their process distinguishes those two, which is precisely the thing a policy is for.
Add a runtime move to .NET 10 that broke binary compatibility with every plugin built for
10.11, and the support forum thread is titled *"New server version incompatible with clients?"*.
"Upgrades must be boring" is one of the strongest findings in
[sentiment.md](../research/sentiment.md).

**Three things make our exposure larger than theirs:**

1. **Our clients are first-party and ship through app stores.** A server fix is a `docker pull`;
   a client fix is a review queue. The server will routinely be newer than the client, and
   sometimes by months.
2. **Televisions do not update.** A TV app sits at whatever version the household installed, for
   years, and nobody in the household knows how to change that.
3. **[D2](../../decisions/working-notes.md) puts decision logic in the client.** A thin renderer
   can be fixed from the server. A client that probes its own decoder, intersects a menu and
   picks a rendition cannot be.

And [D18](../../decisions/working-notes.md)'s Jellyfin adapter makes us responsible for *someone
else's* compatibility surface too, against clients we do not control and cannot update.

## The rules

- **Additive only.** A response field may be added; an existing field never changes meaning,
  type or nullability. New behaviour arrives as a new field or a new endpoint. A client must
  ignore fields it does not know — the same rule
  [D61](../../decisions/working-notes.md) already imposes on the node wire format.
- **Removal needs a window.** Measured in releases, announced in the OpenAPI document and in a
  `Deprecation` / `Sunset` response header **while the endpoint still works**. Removal without a
  preceding window is the breaking change; removal after one is maintenance.
- **A removal ships with the switch that undoes it.** For one release cycle after a breaking
  change, a single named setting restores the old behaviour — the pattern Jellyfin got *right*
  with `EnableLegacyAuthorization` and skipped for the route prefixes. It converts "your setup
  is broken and you are reading a forum" into "your setup is broken and the release notes name
  the flag", which is the whole difference.
- **Both sides declare a revision.** The server publishes a minimum supported client revision;
  the client declares the revision it was built against, in the capability handshake that
  already exists ([stream-service.md](stream-service.md)).
- **A mismatch produces one legible message** — naming the client, the server, and what the user
  should do — at the handshake. **Never a 404 in the middle of playback**, which is how this
  failure normally presents and why it is so hard to diagnose.
- **The pairing and handshake endpoints are the frozen core.** They are the first surfaces a very
  old client touches ([D93](../../decisions/working-notes.md)); if they break, the client cannot
  even report that it is too old.

## The gate

The OpenAPI document is already generated from the code and the build is already gated on it
being current ([D20](../../decisions/working-notes.md)). This adds one comparison: **CI diffs
the generated document against the last release and fails on a breaking change that carries no
explicit waiver in the commit.** A waiver is a line in the changelog, which is where a user
looking for the cause will go.

That turns "we try to be compatible" into a thing that can fail, which is the same move
[performance-targets.md](performance-targets.md) makes for speed.

## Three protocols, one rule

| Protocol | Who is behind | Already covered? |
| --- | --- | --- |
| core ↔ client | app stores, televisions | **no** — this plan |
| core ↔ node | the user's own upgrade laziness | partly — [D61](../../decisions/working-notes.md) requires skipping unknown record kinds |
| core ↔ relay | a rented box someone forgot | **no** — and it is the one nobody will notice is stale |

The shared version inventory of [D61](../../decisions/working-notes.md) and
[D74](../../decisions/working-notes.md) is where all three are recorded, so a bug report says
what every component's revision was without anyone asking.

## Done when

A server one major version ahead of a client either works or says so in one sentence; a removed
endpoint was deprecated in a prior release with a header saying so; CI fails a pull request that
changes a field's type; and a bug report states every component's protocol revision without
being asked.
