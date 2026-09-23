> **ABOUTME:** How the decision log is kept — numbering, marking a supersession, when an entry
> moves to the archive, and who may cite an archived one.

# Decision log conventions

- One D-number per meaningful decision; trivial choices don't get one. Numbers are never reused
  or renumbered.
- Superseded decisions are marked `Superseded by Dnn` with a forward link, not deleted.
- **An entry moves to the archive when nothing in it is still the design**: wholly superseded,
  or superseded piece by piece until nothing is left. An entry with one surviving clause stays
  live, marked in place (below) — D156 stays because its speech-map measurement is still
  cited. A withdrawal that is itself the rule, such as
  [D151](working-notes.md#d151--withdrawn-notifications-a-media-server-has-nothing-you-are-waiting-for-and-its-ui-is-the-notification)
  ("no notifications"), stays live, because it is what stops the feature coming back. Moving an
  entry means: cut it whole, add a `> **Archived <date>.**` note naming the successor under its
  heading, and **re-read every citation** — which the checker lists.
- **Only the logs and `design/research/` may cite an archived decision.** Research is a dated
  record of what was known then; plans, principles, standards, the overview, FEATURES and the
  ROADMAP describe the design now, so a citation there of a decision that no longer holds is
  probably a sentence that no longer holds. `scripts/check_docs.py` fails on one.
- **Partial supersession is marked in place.** Where a later decision overturns one clause of an
  earlier one, the clause is struck through with the superseding number beside it, and a short
  blockquote at the top of the decision says what changed — rather than marking the whole entry
  superseded, which would be false. [D7](working-notes.md) and [D24](working-notes.md) carry
  the first two examples, both from [D98](working-notes.md).
- **A supersession is not finished until every citation is chased.** Marking the log is the
  cheap half; the expensive half is `grep -rn 'Dnn' docs/` and reading each hit, because a
  plan that cites the superseded clause goes on asserting it as current and nothing in the
  log's strikethrough reaches it. The 2026-09-21 coherence pass found four such survivors of
  [D104](working-notes-archive.md)'s superseded entitlement half — including one in
  `testing-strategy.md` that had become an assertion of the **opposite** of the design, which
  is worse than a missing test because it would have failed a correct implementation. The
  2026-09-23 archive pass found two more — D77 still listing the change journal as
  irreplaceable and D87 still reconciling deletions only through it, both un-marked after
  D166 — and plans citing D8 for a journal design D166 had replaced. For archived entries the
  checker now does the chasing; for partial supersession it is still `grep`.
- A decision that has not been validated against real hardware or a real library says so, in
  its own **Status** line. Most of the early ones have not been.
