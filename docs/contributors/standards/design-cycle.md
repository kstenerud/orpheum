> **ABOUTME:** How the owner and an agent iterate on a look and feel at speed: changes shown on staging
> within a minute, the gate deferred for the whole cycle and taken up once at its end, intrusive code
> kept in worktrees, and commits made only at the owner's go. Each rule names what enforces it.

# The design cycle

A cycle of visual tweaks (a page's layout, its icons, a dialog's shape) is one change made in many
small looks, the owner judging each on staging. Checking every look is checking code that will change
again: the owner (2026-10-30), "Treat this whole cycle as if it were a single change you were making,
with the checks deferred until I'm happy with the look and feel", and after the first such cycle,
"I think it works well for keeping velocity high".

## The cycle

1. **Start: defer the gate.** `touch .claude/.cache/check-deferred`. While the mark is there the Stop
   hook runs no gate at a turn's end (`.claude/hooks/on_stop.py`, `DEFERRED`; its test
   `StopDeferred` in `scripts/tests/test_hooks.py`). The mark is in the ignored `.claude/.cache/`, so
   nothing of it is ever committed.
2. **Each look: edit, then `./staging quick`** (about 35 seconds), the type check alone run before it
   (`npm --prefix web run typecheck`), so a look never fails to build. No commit, no lint, no tests.
   *Review-only.*
3. **Intrusive code waits in a worktree.** A change to the server, the API or the schema that a look
   needs is built by an agent in a worktree of its own and held there until the owner says; the look
   on staging may stand in for it meanwhile (the owner, 2026-10-30: "We'll keep the more intrusive code
   changes in worktrees for now and concentrate on getting the visual aspects right"). *Review-only.*
4. **The owner's go ends it.** At "I'm happy with the look" (and not before):
   - drop what only stood in for the worktrees' work, and anything the looks left unused;
   - remove the mark: `rm .claude/.cache/check-deferred`. `make check` refuses to run while it is
     there (`scripts/check_stamp.py`, `DEFERRED_MARK`; its test
     `test_the_gate_refuses_to_begin_while_a_design_cycle_defers_it`): a make check ends the cycle
     with every safeguard back in place (the owner, 2026-10-30);
   - run `make check` once; update the tests the new look changed, and add the tests a behaviour
     changed by the cycle owes (AGENTS.md rule 4);
   - commit in logical pieces, each its own subject and body, the gate-file changes with their
     `Gate-Changed:` trailer;
   - then merge the worktrees' work the owner agreed.

## What it is not

- Not a way past a gate: the cycle ends in the full `make check`, and the batch in `make check-pr`.
- Not for behaviour changes beyond the look: those are built and checked as any change is.
