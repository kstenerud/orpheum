> **ABOUTME:** Principle §25: Velocity is a resource, and the dev cycle's time costs it quadratically.

# §25 — Velocity is a resource, and the dev cycle's time costs it quadratically

**Every second added to the dev cycle costs velocity, and the cost grows faster than the time.**
The owner, 2026-10-27: *"Every second added to our dev cycle reduces our velocity quadratically.
The optimal dev cycle cadence is 30 seconds. It can still be acceptable up to 5 minutes, provided
the protection really is worth it. Beyond that, velocity falls off a cliff."* And: *"Velocity is a
resource to be balanced like all other resources we have."*

The day before showed what it costs: a fix that should have taken two hours took a day, largely
because each fix ran its own test runs, which could have been batched for the same protection. Two
small commits each followed by `check-pr` cost 42 minutes where one batched run gives the same
protection in half.

So:

- **30 seconds is the cadence to aim for** while working: the change's own tests (§23's spot check).
- **Up to 5 minutes only when the protection is worth it**, and then once per batch: iterate many
  small changes quickly, then pay for one comprehensive run (`make check` once and one commit for the
  batch, `check-pr` once for the batch). A debt this leaves, such as an owed full `check-pr`, is
  shown, never forced in the middle of an iteration.
- **A new check states its cost on the happy path**, and is weighed against the protection it buys:
  robustness has diminishing returns, and past them a check is a tax. The owner budgets it (for
  instance, 15 seconds for decisions' test citations, 2026-10-27).
- **An agent's run counts as the cycle's**: briefs say narrow runs while working and one gate at
  the end; an agent that re-runs a gate per item spends the owner's velocity, not its own.

**Bites when:** you commit each small fix separately so each runs the gate, rerun a full suite to
confirm a one-line change, add a per-commit check whose protection the batch's run already gives,
or accept a cycle over 5 minutes as how things are.
