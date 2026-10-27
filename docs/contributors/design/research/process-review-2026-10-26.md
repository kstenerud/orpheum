> **ABOUTME:** Every process failure of 2026-10-26, and for each the mechanism (a hook, gate, lock or
> script that fails closed) that would have stopped it, rather than a rule an agent must remember;
> with what each costs and what is already done. The input to the owner's process review.

# Process review: 2026-10-26

- **Requested by** the owner, 2026-10-26: "what should have been a couple of hours this morning has
  turned into a complete day's work"; "I want solutions that take into account the non-deterministic
  nature of LLMs, because asking you to remember things will go against your nature."
- **Builds on:** [gate-throughput-2026-10-26.md](gate-throughput-2026-10-26.md) (G0–G9, audited),
  [test-architecture-2026-10-26.md](test-architecture-2026-10-26.md) (findings 1–12), the outside
  report `/yoloai/files/yoloai-slowdown-2026-10-26.md` (R1–R6).
- **The principle:** a rule held only in an agent's memory or brief is followed unevenly
  (agent-failures A8). Each fix below is a mechanism that refuses the wrong action or does the right
  one by itself; where none exists, it says so.

## 1. What went wrong, and what would have stopped it

| # | Failure (cost) | Mechanism that would have stopped it | State |
| --- | --- | --- | --- |
| F1 | The lead installed a system font on the sandbox host; it changed text layout in the host's browsers and blocked the D340 agent's commit for a whole run | Gates never run on the host: one environment, the pinned image (G1, a gate container separate from the dev loop's) | **Open** (G1 needs its redesign) |
| F2 | check-pr re-checked 18–30 commits (26–34 min): the previous batch ended on `check-pr-failed`, whose green never advances the base, and the full run was skipped | The base advances only on a full pass (by design, makefile.md §4); what was missing is a visible debt. Proposal: `check-pr-failed`'s green writes "a full check-pr is owed since <base>", and the Stop hook (or the session's start) says so until one passes | **Open** |
| F3 | Build agents in the main tree: every end of the lead's turn gated their half-written work (a dozen blocked stops), and loaded the machine beside their own gates | Agents in worktrees, whose edits owe their own worktree's gate | **Done** |
| F4 | Every edit in an agent's worktree was refused as a gate; one agent wrote through Python, which no hook sees | Hooks judge a file from its own checkout, a worktree of the same repository only (an earlier commit, tests for the nested-repository bypass) | **Done** |
| F5 | Subagents inherit the session's `ORPHEUM_ALLOW_GATE_EDIT=1`: an agent edited `internal/archtest/` unrefused | The edit and shell hooks refuse a gate edit whose event carries `agent_id` (Claude Code puts `agent_id`/`agent_type` in every PreToolUse event a subagent makes; code.claude.com/docs/en/hooks), unless the owner names that agent; the branch gate's Gate-Changed check stays the wall | **Open**, small |
| F6 | Agent worktrees start from `origin/HEAD` = `origin/main`, frozen at the clone's 2026-10-25 commit; an agent hit a gate test main had fixed that morning | `"worktree": {"baseRef": "head"}` in `.claude/settings.json`: a worktree starts from the session's local HEAD. The default (`fresh`) fetches the remote's default branch, which this sandbox cannot reach, and falls back to the stale cached `origin/HEAD` (code.claude.com/docs/en/worktrees) | **Open**, one line |
| F7 | Revert-red overloaded the machine (load 82 on 8 vCPUs); ~24 probes failed with nothing reverted, costing a 34-min run and reruns | Each probe within its share of the CPUs; test commands built in one place, enforced by a test | **Done** (load 82 → 40, 34 → 21 min) |
| F8 | A probe that failed with nothing reverted kept no output; the failing test had to be found by reproducing the load | Revert-red keeps the clean run's output (the failing test and its message) in its report | **Open** |
| F9 | Generated data edited by hand (a statistics line trimmed) | The generator fixed; a check that the committed file equals a fresh generation, as `check-generated` does for code | Generator **done**; equality check **open** |
| F10 | A stale e2e test survived D344's removal of the feature it tested; found only by check-pr's browser tier days later | The same as F2: the full check-pr per batch, made a visible debt | **Open** (with F2) |
| F11 | An agent called D349 "Built" with two decided points unbuilt; a feature's backend landed with 7 changes no test noticed | (a) `check-commit-red` (a minute) run automatically by the commit hook for a feat/fix commit, not by a brief line; (b) a decision's numbered points each cite the tests that prove them, and `check-claims` refuses "built" while a point cites none | **Open** |
| F12 | The lead told the owner D350 needed FFmpeg rebuilt, from a handoff note, against D350's own text | Handoffs name decisions and their open items by ID, generated from the decisions' status, not free text; a claim about a decision is checked against its entry before it reaches the owner | **Open** (partly a habit; the generated handoff is the mechanism) |
| F13 | Scope drift: Diagnostics design and a gate proposal while the stopped work waited | No clean mechanism; the standing orders as a file the Stop hook prints at each turn's end would make drift visible | **Open** (cheap) |
| F14 | Agents waiting on background runs stopped and did not resume (D338, D349); the lead had to wake them | A `SubagentStop` hook (its input carries `agent_id` and `last_assistant_message`) can block a subagent's stop while a run it started is still going, or note it for the lead; otherwise a Claude Code behaviour to report | **Open** (outside the repo) |
| F15 | The stamp race: a tree edited during `make check` is recorded as passed, and a commit of it skips the gate | Stamp the tree taken before the run, write it only if unchanged (gate-throughput A) | **Open** |
| F16 | The Stop hook re-runs the gate on a tree the commit just passed | The Stop hook honours the stamp (G2, after F15) | **Open** |
| F17 | Two gates at once (check-pr beside make check, a subagent's beside the lead's) | One re-entrant lock both sides take (G5/R3) | **Open** |
| F18 | The weakening check cannot follow a helper moved to another file, so a pure move needs a Test-Weakened trailer | The check follows moved helpers (by body hash) as it follows renamed tests | **Open** |
| F19 | A test passed on another test's leftovers (a module-level lazy component warmed earlier in the file) | Lazy screens reset per mount; module singletons reset by the harness | **Done** for the screens |
| F20 | The lead's own load reproduction ran the integration tier without its tools, a wasted run | The tests already fail loudly without the tools; no mechanism needed beyond using the make targets | **Accepted** |

## 2. Test and gate work done today (for the record)

G6 revert-red sizing; G4 hooks in worktrees; revert-red Go probes run
naming tests first (6ad0186d); locale probes and vitest's isolation split (f38558fd..949b9aa1);
servedBy fails on unexpected requests, waits hold under load (an earlier commit..c73296ca); ExecRaw, LogLimit,
archtest's go list once (an earlier commit..b5719e24); served tests from the template; D351 one
assembly shared with the tests (an earlier commit..48b805c6); sleeps → signals, logs by key, subtitles tests
(an earlier commit..ef947d0e); fixture builders (in progress).

## 3. Proposed order

1. **F15 + F16** (stamp race, Stop hook stamp): a safety hole and the commonest duplicate run; small.
2. **F5** (subagents and the gate flag): a safety hole; small.
3. **F6** (worktree base): every agent today started stale; small once the mechanism is found.
4. **F2/F10** (check-pr debt made visible) and **F11a** (commit-red at commit): the two that let
   untested or stale work through a batch.
5. **F17** (the lock), **F8** (keep clean-failure output), **F9** (statistics equality), **F18**.
6. **F1** (the gate container), the largest.
7. **F11b**, **F12**, **F13**: the documentation-side mechanisms.

## 4. State, 2026-10-27

The owner approved sets 1–3 (F11a dropped: check-pr's revert-red already catches what a per-commit
run would, and a run per commit is the cost principle §25 forbids). Done: F15, F16, F5, F6;
F17 the gate lock, F8 a broken probe's output kept, F9 the statistics compared on every reference
run, F18 moved helpers followed, F11b decisions' **Proven by:** (conventions.md), F2 the owed
check-pr shown at a session's start, F13 the standing orders at every prompt, F14 the SubagentStop
hook, and principle §25; F12 `make handoff` (scripts/handoff.py) and principle §26 with
code-debt.md; F1, every gate in one gate container apart from the dev loop's. Nothing
is open.
