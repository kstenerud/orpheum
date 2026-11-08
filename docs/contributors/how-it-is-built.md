# How Orpheum is built

Orpheum is built by Claude agents directed by an engineer. Almost every commit in the history
carries a `Co-Authored-By: Claude` trailer. The project owner sets direction, makes the design
decisions, tests on real devices and reviews; the agents research, design within those decisions,
wrire the code, tests, deployment and documentation, and run the operations.

Writing code quickly is the part AI already does well. Keeping it correct as it grows to
hundreds of thousands of lines, across sessions that each start with no memory of the last, is
not. Most of the engineering in this repository is the machinery that makes that work. This page
describes that machinery, and names the files that show it.

## One contract, read by every agent

[`AGENTS.md`](../../AGENTS.md) is the contract every agent reads before changing the code. It holds 17
rules, and **each rule names what enforces it**: a hook, a lint, a test or a review step. A rule
that nothing enforces is a wish ([principles §17](principles/17-a-rule-nothing-runs.md)).
Each package states its own invariants in its `doc.go` (Go) or `README.md` (web), with the test
that keeps each one true; [`internal/store/doc.go`](../../internal/store/doc.go) is a good example.

## Gates the agent cannot skip

Claude Code's hooks ([`.claude/settings.json`](../../.claude/settings.json),
[`.claude/hooks/`](../../.claude/hooks/)) put the checks in the agent's path rather than in its
memory:

- **At the end of every turn,** `on_stop.py` runs `make check` and blocks the turn with the
  failures until they are fixed.
- **Every commit** runs `make spot`, the tests of what changed and of what imports it, in about
  30 seconds; a batch of commits passes `make check-pr`, the full integration tier, before it is
  done ([principles §23](principles/23-proportionate-test-runs.md)).
- **The gates guard themselves.** `pre_edit.py` and `pre_bash.py` refuse an agent's edits to lint
  configuration, the `Makefile`, the checks in `scripts/`, the hooks, the architecture tests,
  dependency allowlists and generated code, and say what to do instead. A refusal is a speed bump;
  the wall is the branch gate, which fails any change to a gate that lacks the owner's
  `Gate-Changed:` trailer ([principles §20](principles/20-a-gate-changes-on-purpose.md)).
- **A new dependency is the owner's decision**, enforced by allowlists in [`deps/`](../../deps/).
- **Cost is managed too:** a hook refuses an agent launch that names no model or would make a
  fourth running at once, and warns an agent nearing the call budget its brief states.

The gates themselves are about 24,000 lines of Python with their own 670 tests
([`scripts/`](../../scripts/), [`scripts/tests/`](../../scripts/tests/)).

## Every change must be shown to matter

An AI writes tests readily; the risk is tests that pass whether or not the code is right.

- **Revert-red.** Every behaviour change carries a test that fails when the change is reverted
  ([D190](decisions/working-notes.md)). `make check-revert-red` proves it by
  reverting each changed file and running the tests, and a post-commit hook starts that work in
  the background so the batch's check finds most of it done
  ([principles §18](principles/18-a-check-never-seen-failing.md)).
- **No weakening.** `check-test-weakening` fails a commit that deletes a test, adds a skip, drops
  an assertion or widens a tolerance, unless it carries a `Test-Weakened:` trailer saying why. The
  history has a few hundred; read them and nearly all say a test went with the feature it tested,
  when a redesign removed that feature.
- **A missing capability fails; it does not skip.** A test that cannot run is a failure, so no
  check can pass by quietly not running.
- **Architecture is tested.** The `TestArch_*` tests in
  [`internal/archtest/`](../../internal/archtest/) hold load-bearing claims: the core never runs
  a program and never imports the media node, only the store package touches SQL, a release
  binary reads no environment, each table has one writing package, a home address is never a
  credential.

## What only a device can confirm

Playback on a Fire TV, a Chromecast or a phone cannot be fully checked by a test. A change only
a device can confirm adds an item to the
[verification queue](verification-queue.md) and a `Verified-By: VQ<n>` trailer; the
owner works through the queue on real devices, and the results go back into the queue.
[`idiosyncrasies.md`](idiosyncrasies.md) records each platform quirk met along the
way (a browser, hls.js, a television's decoder, the kernel), with its workaround, so the next
agent looks it up instead of rediscovering it.

## Learning from mistakes

When the owner corrects a claim an agent made, the correction is recorded in
[`agent-failures.md`](agent-failures.md) before work goes on
([principles §21](principles/21-an-owners-correction.md)). Its entries are the input
from which the next gate is built: several of the gates above exist because an entry there
showed a prose rule being followed unevenly.

## Decisions, written down

Every design decision is recorded with the evidence and reasoning behind it, and cited by number
wherever the code relies on it: 446 of them, in the
[decision index](decisions/index.md). When evidence shows one wrong, a new decision
amends it and says so ([principles §27](principles/27-decisions-are-revisable.md)). The
[principles](principles/README.md) and [standards](standards/README.md)
are distilled from them, and the research behind them is in
[`contributors/design/research/`](design/research/).

## Running more than one agent

The lead session the owner talks to delegates larger pieces to agents, each in its own git
worktree, with a brief naming the files and mechanism and a call budget. Only one test run holds
the repository at a time, so parallel agents do not spoil each other's timings. The lead's own
handbook, each rule with the incident behind it, is [`contributors/lead.md`](lead.md).

## Operations

The same agents built and run the deployment: container images
([`deploy/Containerfile`](../../deploy/Containerfile)), a staging server every change reaches
first, a production server in the owner's home, automatic backups, and remote access through a
small hosted discovery service. Some of that operations material lives in private companion
repositories (`orpheum-ops`, `orpheum-connect`, `orpheum-security`); references to them in these
documents point there.
