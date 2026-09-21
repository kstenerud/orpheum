> **ABOUTME:** Index for the decisions directory — the append-only, D-numbered decision log
> that principles, standards and design docs cite by number.

# Decisions

The append-only decision log. Each entry is a proto-ADR — what was decided, what was rejected,
why, and the consequences — with a stable `D`-number that other docs cite.

- **[working-notes.md](working-notes.md)** — the live log. New decisions append to the bottom.

## Conventions

- One D-number per meaningful decision; trivial choices don't get one. Numbers are never reused
  or renumbered.
- Superseded decisions are marked `Superseded by Dnn` with a forward link, not deleted.
- **Partial supersession is marked in place.** Where a later decision overturns one clause of an
  earlier one, the clause is struck through with the superseding number beside it, and a short
  blockquote at the top of the decision says what changed — rather than marking the whole entry
  superseded, which would be false. [D7](working-notes.md) and [D24](working-notes.md) carry
  the first two examples, both from [D98](working-notes.md).
- **A supersession is not finished until every citation is chased.** Marking the log is the
  cheap half; the expensive half is `grep -rn 'Dnn' docs/` and reading each hit, because a
  plan that cites the superseded clause goes on asserting it as current and nothing in the
  log's strikethrough reaches it. The 2026-09-21 coherence pass found four such survivors of
  [D104](working-notes.md)'s superseded entitlement half — including one in
  `testing-strategy.md` that had become an assertion of the **opposite** of the design, which
  is worse than a missing test because it would have failed a correct implementation.
- A decision that has not been validated against real hardware or a real library says so, in
  its own **Status** line. Most of the early ones have not been.
