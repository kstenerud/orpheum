> **ABOUTME:** Principle §23: A test run is proportionate to the change.

# §23 — A test run is proportionate to the change

**A small change is checked in seconds; the whole battery runs for what has accumulated.** The
owner, after a day in which each small fix waited on a 4–19 minute `make check-pr`: *"Running
check-pr after every little change is slowing down the dev cycle … Test runs should be
proportionate to the changes. If that means refactoring/rearchitecting the tests to support this,
I'm fine with that."* And: *"Ideally a 'spot check' from a small change shouldn't exceed 30
seconds"* ([D279](../decisions/working-notes.md)).

So there are three sizes of run, each with a budget (testing.md §10):

- **The spot check**, while working: the tests of what changed, and what they need to build. At
  most **30 seconds**. It answers "did I break what I touched?"
- **`make check`**, at each commit (the hooks run it): every gate that needs only the pinned
  toolchain, about **90 seconds**.
- **`make check-pr`**, for an accumulated batch or before the owner applies: the integration and
  browser tiers and the gates against a base, at most **5 minutes**.

A run that cannot fit its budget is a defect of the tests, not a reason to run less: the tests
are restructured until it fits, as the owner allows. Running a bigger battery than the change
needs is not caution; it is the dev cycle's time spent on nothing.

**Bites when:** you reach for `check-pr` after a one-file fix, wait on a full suite to learn
whether the line you changed works, or accept a budget overrun as how things are.
