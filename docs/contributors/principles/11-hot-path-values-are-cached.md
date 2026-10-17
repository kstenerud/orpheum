> **ABOUTME:** Principle §11: A value on a hot path is cached against a known event, not computed
> per request.

# §11 — A value on a hot path is cached against a known event, not computed per request

**The home page computes nothing.** Anything rendered on the screen every user sees at launch
is **computed once and kept until something known changes it**, then read as a lookup.
Computing it per request is the default that keeps being wrong.

**Cache it; do not incrementally maintain it** ([D161](../decisions/working-notes.md)). The
two perform identically on read and fail completely differently: a missed *invalidation*
leaves a value briefly **stale**, while a missed *update path* leaves it **wrong for ever,
with nothing to notice**. Incremental maintenance means enumerating every path that can move
the number — for the pending count that is a scan, a rule edit, a role edit, a tag edit, a
re-identification and a removal — and missing one silently stops a child's library growing.
The invalidation signal already exists: [D166](../decisions/working-notes.md)'s change journal
says what changed.

This is a principle rather than a note because it has now been rediscovered four times
independently, each time by a different route:

| Value | Per request | Kept | Mechanism | Found by |
| --- | --- | --- | --- | --- |
| Recently added | 21.79 ms | 0.027 ms — **807×** | cached result | the 2026-09-20 benchmark |
| FTS `bm25` ranking | 11.66 ms | 0.017 ms — **686×** | **derived column** — the query takes a term, so there is no single result to cache | the same benchmark |
| Pending-items count | 43.6 ms, or **no completion at all** without an index | 0.063 ms — **692×** | cached result | a *feature request*, 2026-09-21 |
| Subtitle-sync check | an audio decode, **~32 s** — and a sleeping node to wake | **3–8 ms** from a stored **1–3 kB** map | **stored artefact** | a *user complaint*, 2026-09-21 |
| A collection's sequence and progress | 14.8 ms at 976 members | 0.01 ms | cached result | the 2026-09-22 benchmark |

Two of those were audit findings, the third arrived attached to a new feature and the fourth
attached to a complaint about somebody else's product, which is what makes it a hazard rather
than a bug list: **every new surface rediscovers it.**

**And the fourth row widens the principle past the home page**, which is where the first three
found it. The speech map ([D148](../decisions/working-notes.md)) is not on any screen; the
hot path is *a user waiting for a subtitle*. The shape is identical — a value that is
expensive to derive, cheap to keep, and invalidated by a known rare event — so read "hot path"
as **anything a person is waiting on**, not as "the home page". The storage argument is the
same one every time, and it is never close: **1–3 kB per film, 3 MB for a thousand-film
library**, against re-deriving it on demand.

**The invalidation is not the hard part, which is why this can be absolute.** These values
change only on rare, known events — a scan completing, a rule or role changing, a tag edit, a
metadata refresh — and [D166](../decisions/working-notes.md)'s change journal already says
exactly what changed, with `rules-engine.md` already consuming it on a debounce. Event-driven
invalidation on a mechanism that exists, not a TTL and not new machinery.

**Three mechanisms, and incremental counters are not among them**
([D161](../decisions/working-notes.md)): a **cached result** is the default; a **derived
column** is for queries that take a parameter, so there is no single result to keep; a
**stored artefact** is for bytes derived from bytes. Read "maintained" anywhere in this tree
as "cached" unless it says otherwise.

**The boundary worth knowing:** per-*library* values are cheap to maintain because there is one
of each; per-*user* values are not automatically so, because there may be many. Continue
Watching measured flat and needs nothing today ([D113](../decisions/working-notes.md)); the
rule bites where it would not.

**Bites when:** you add a count, a badge or a "new since" marker to a screen, and the query
looks harmless at the size of your test library. ([D113](../decisions/working-notes.md),
[D133](../decisions/working-notes.md), [D134](../decisions/working-notes.md).)
