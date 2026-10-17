> **ABOUTME:** Principle §14: A cache may always be wrong, so the path that ignores it is the
> ordinary one.

# §14 — A cache may always be wrong, so the path that ignores it is the ordinary one

**Assume every cache breaks, for a reason nobody thought of.** A local copy, a sync cursor, a
delta stream, a memoised result — each is an optimisation that will eventually be stale,
truncated, corrupt, or from a version that no longer exists. Design for that as the normal
case, not as an incident.

The rule that makes it survivable:

> **Nothing may live only in the cache, and the path that does not use it must be one that
> runs every day.**

An escape hatch used once a year is code that has never been tested, invoked at the worst
moment by the person least able to debug it. The fix is not to make the cache more reliable —
that is the hard problem, and network communication makes it harder — it is to make *not
having it* cheap enough to be unremarkable.

**Cheap enough is a measurement, not a hope.** The whole browsable state of the reference
library is **215 KB gzipped and 0.2 s on a 10 Mbit link**
([D166](../decisions/working-notes.md)), which is why the sync design can answer any request
with *"here is everything"* and treat that as an ordinary reply rather than a failure. Where
the fallback turns out to be expensive, that is the thing to fix — not the cache's reliability.

Three consequences that keep showing up:

- **A cache failure must be a performance event, never a correctness one.** If a stale cache
  can produce a *wrong answer* rather than a *slow* one, it is not a cache.
- **Let it be lossy on purpose.** A journal that may be incomplete can be capped, truncated,
  dropped on restart or never written; one that must be authoritative needs durability,
  ordering, retention and compaction. The second is a project; the first is a table.
- **Partial is usable.** Nothing should wait for a cache to be *complete* before a person may
  interact — the copy fills in behind a product that already works.

**This is [§11](11-hot-path-values-are-cached.md)'s
other half.** §11 says what to cache and how to invalidate it; this says what must remain true
when the invalidation is missed — and [D161](../decisions/working-notes.md)'s reason for
preferring cache-and-invalidate over incremental maintenance is exactly this test applied to
one mechanism: *a missed invalidation is stale, a missed update is wrong.*

**Bites when:** a client cannot start without a sync, an error path exists only for "cache
unavailable", or the answer to "what if this is stale?" is a guarantee rather than a fallback.
