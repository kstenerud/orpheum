> **ABOUTME:** Principle §12: Defer the work, never the seam.

# §12 — Defer the work, never the seam

**A feature you have decided not to build still constrains the design, and the constraint is
cheap exactly once.** Rank what to build by demand; decide what to *design around* by
**retrofit cost**, which is a different number and frequently points the other way.

The four costs, in the order they hurt:

| Cost to add later | Example | Pay it |
| --- | --- | --- |
| **free** — additive, usually client-side | shuffle, sleep timer | never; ignore it until there is room |
| **seam** — needs an extension point to exist | a DLNA adapter, a transcoder | when the first *real* consumer arrives ([D19](../decisions/working-notes.md)) |
| **schema** — needs a column or table | external ratings, artwork types | **before the model freezes**, or it is a migration |
| **invariant** — needs a rule *not* to have been assumed | *"every item has a File"* | **now**, because it costs nothing to keep and cannot be recovered |

**The last row is the one that gets missed**, because keeping an invariant open looks like
doing nothing. It is the cheapest work in the project and the only kind that becomes
impossible. "Every item has a File" is one sentence today and a rewrite the day somebody wants
live TV — and *we have declined live TV*, which is exactly why nobody would think to protect
it.

**So a decline is a statement about effort, never about possibility.** Recording *what keeps
the door open* beside each declined feature is what makes changing your mind later a decision
rather than an excavation — which is what [`FEATURES.md`](../../FEATURES.md)'s last column is
for.

**Bites when:** you are freezing a schema, closing an enumeration, or writing "must" in an
interface, while a feature you have just declined quietly depended on the opposite.
([D153](../decisions/working-notes.md).)
