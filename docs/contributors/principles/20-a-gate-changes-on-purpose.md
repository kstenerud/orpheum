> **ABOUTME:** Principle §20: A gate changes on purpose, never to land a change.

# §20 — A gate changes on purpose, never to land a change

The lint configuration, a complexity threshold, a frozen baseline, a golden file, a tolerance and
a test's assertion change **only as the subject of their own change, reviewed by the owner** —
never as the step that turns the current change green. A suppression states why the finding does
not apply here; "the linter complained" is not a reason. Complexity findings are never
suppressed: the answer is a named function.

Loosening the gate is the cheapest route to green, and agents take the cheapest route:
ImpossibleBench (arXiv 2510.20270) documents agents that *"delete failing tests rather than fix
the underlying bug"*. In yoloAI a lint fix
narrowed a test to satisfy a security linter, and on macOS the test silently stopped testing
anything (A19); elsewhere "extract a function" drifted into "silence the linter" until a gate
banned complexity suppressions. [D110](../decisions/working-notes.md) already says re-basing a
baseline is a reviewed act, not a flag somebody runs; this extends it to every gate. The mechanism
is a pre-edit hook that refuses an agent's edit to a gate file and says what to do instead
([cut-a.md](../design/plans/cut-a.md) §8, M0).

**Bites when:** the quickest way to a green check is editing `.golangci.yml`, adding a `//nolint`
or an `eslint-disable`, re-recording a baseline or golden, widening a tolerance, or weakening the
assertion itself.
