> **ABOUTME:** How the lead agent works in this project: the owner's standing rules on cost,
> agents, gates, commits, staging and design questions, each with the incident behind it. Read the
> section for the job at hand, not the whole file. Agents' own rules are in `AGENTS.md`; this file
> is for the session that runs them.

# The lead's handbook

The lead is the agent session the owner talks to: it plans, briefs agents, merges, deploys and
reports. These rules come from the owner's corrections (most cite the date said); before 2026-10-31
they lived in the lead's private memory, which is outside the repository and unseen by reviewers.
Machines and access are in the sibling repository `orpheum-ops`, file `machines.md`.
The current work is in `.claude/standing-orders.md` and the plan files.

## 1. Tokens and context

Cost is (model calls) × (context per call): every call re-reads the whole context. Measured
2026-10-31 (plan file "Token use",
[`navigation-and-widgets-answers-2026-10-27.md`](design/plans/navigation-and-widgets-answers-2026-10-27.md)):
cache reads were 85% of the bill, agents 77% of it.

- **Keep tool output small.** Ask for the lines needed (`grep -n … | head`, `sed -n a,bp`), never a
  whole table to read by eye. Analysis is a script that writes its result to a file and prints the
  conclusion.
- **Fewer, fuller calls.** Related reads and greps in one command.
- **Wide read-only sweeps go to an Explore agent**, whose reading never enters the lead's context.
- **A fresh session per phase** (a batch, a merge round, a move), with a handoff memory; never a day
  on one context.
- **At most 2–3 agents at once**, more only when the owner asks for that scale (2026-10-26: seven
  Opus agents took the whole 5-hour quota and 20% of the week; 2026-10-31: fifteen took the week to
  91%).
- **No polling loops**, for agents or for gates: wait for the notification.
- **The hooks hold the limits, not memory:** the prompt hook warns past 120k of context, an agent launch
  needs a model and a free slot of three, an agent past 150 calls is told to stop (`CLAUDE.md`, "On
  cost"). At a batch's end, read `python3 scripts/token_usage.py --since <the batch's first day>` and put
  its totals in the batch's report.

## 2. Agents

- **Model by the job:** Sonnet for mechanical work (renames, test fixes, docs, small features with a
  clear brief), Opus only for design-heavy or wide-reaching work; read-only searches on Explore.
- **Small changes the lead makes itself.** A "small" logging change briefed to an agent took 41
  minutes and 269 tool calls (2026-10-29); the same kind of fix done directly took 10.
- **A brief names the files, the mechanism and the exact report text**, drops optional parts, and
  says: end at `make spot` (the lead runs `make check` once at the merge), no polling, never walk
  `/` (`find / -path /tank -prune -o …`), regenerate generated files rather than edit them, classify
  each value it adds (setting, config key, constant: §7).
- **Every agent that can edit gets `isolation: "worktree"`**; read-only work goes to Explore
  (2026-10-26: five agents in one tree starved commits for 90 minutes; 2026-10-31: one sharing the
  tree failed the Stop hook three times, so `pre_agent.py` now refuses the launch). The lead rebases each finished branch onto main and fast-forwards. Each agent
  gets its own D-number range, with "Reserved" entries committed on main first (check-docs fails a
  numbering gap), and its own block of VQ numbers.
- **In a worktree, install web dependencies for real** (`npm --prefix web ci`); a symlinked
  `node_modules` gets emptied by `npm ci` through the link (2026-10-01).
- **Never stage beside an agent working in the same tree:** its commit takes the whole index
  (2026-09-30), and a bare `git add -A` takes its half-written files (2026-10-05). Stage by path.

## 3. Gates and test cadence

The owner (2026-10-27): "Every second added to our dev cycle reduces our velocity quadratically. The
optimal dev cycle cadence is 30 seconds. It can still be acceptable up to 5 minutes, provided the
protection really is worth it."

- **While working:** the change's own narrow tests. **A commit** runs `make spot`. **The turn's end**
  runs `make check` (the Stop hook). **One `make check-pr` per batch**, before the owner applies,
  never per item (2026-10-01, again 2026-10-11).
- **check-pr's budget is 5 minutes.** Measure every run's wall time and report an overrun; omit
  `BASE=` (an old base re-probes every commit since). While it runs it works in its own tree under a
  lock: edit freely; if it fails, `git stash`, fix, commit, `make check-pr-failed`, `git stash pop`.
- **Fail fast:** `make spot` lints the changed Go packages (since 2026-10-31: four gate runs in one
  session went to line lengths); eslint the changed web files and `make fmt` before `make check`; chain with `&&` and `set -o pipefail` (a `make check | tail`
  once let a failed check commit). A load-flaky failure is diagnosed by running the one test alone,
  never retried blind.
- **Slow tests:** find out why, fix it when cheap, record the mechanism in
  [`test-weight.md`](test-weight.md). A regime-wide audit happens when the owner asks (2026-10-31).
- **No gates while the owner tests on staging** (it shares the sandbox's CPUs; three times a gate
  run spoiled a live test). "Watching now" counts. After a deploy, check staging's core log for a
  non-headless browser before any heavy run, and run heavy measurements under `nice -n 19`.
- **Design cycles defer the gate** for the whole cycle:
  [`standards/design-cycle.md`](standards/design-cycle.md).
- **Run gates through `./dev` with the dev loop's exact `ORPHEUM_DEV_*` environment** while a loop is
  up, or `./dev` replaces the container and kills the loop (2026-10-01). Stop a loop with
  `./dev --stop` before starting another.

## 4. Commits in the sandbox

The owner takes commits out with `yoloai apply` (`git am`); branches do not travel, tags do with
`--tags`.

- **Never rewrite a commit** (amend, reset, rebase of made commits): fix forward (2026-09-29, an
  applied series diverged). **Never an empty commit**: `git am` stops on it; a trailer rides on a real
  change.
- **Before every commit:** `git diff --cached --stat` lists only this commit's files (`git mv`/`git rm`
  stage at once), and `git status --short` shows no `??` a path-limited commit would leave out.
- **A hand revert probe** copies the file to the scratchpad first and back after; never
  `git checkout` a file holding uncommitted work (2026-09-27).
- **orpheum-connect has no pre-commit hook:** commit only inside `if make check; then … fi`, never
  after a pipe. After an Orpheum commit touching `discovery/`, run `make pin` in connect and commit
  the pin.
- **New migration goldens** are generated and committed with `Gate-Changed:` citing D201 §4's
  standing approval; changing an existing golden is the owner's call. **Allowlisted Go modules** come
  in by import plus `go mod tidy`.

## 5. Staging and live work

- **Debugging live:** targeted test, `./staging quick` (~35 s), tell the owner; gates and commits
  after they confirm. During UI iteration the lead fixes small things itself, in minutes; agents only
  for larger pieces.
- **Never redeploy while the owner may be watching:** check the core log for a recent playback; once
  the owner has been told a build is ready, ask before any further deploy (2026-10-29: a restart
  ended their Watch Together set-up). `./staging quick` ships the whole working tree, an agent's
  half-written code included.
- **Staging is disposable** (wipe its data freely), but `./staging wipe` also loses its discovery
  claim: to start its data over, move only `persistent.db*` and `cache.db` aside.
- **The dev loop and `./staging` apply uncommitted draft migrations:** never deploy while an agent has
  one.
- **The discovery's loader and sign-in page come from connect's build:** after changing
  `loader-*.tsx` or sign-in/discovery web code, run connect's `deploy/staging/up.sh`.
- **nas.example** is the owner's stable demo: updated only for major changes, built untagged from main,
  at the owner's go. No release tags until the owner asks.

## 6. Working with the owner

- **Finish what is asked; propose side work, don't start it** (2026-10-26, a lost day).
- **Remind the owner of every question left unanswered**; a non-answer is never a default.
- **A design gap is asked conceptually first** (what kind of gap, one question); concrete options
  after. **Search the decisions before offering options** (`working-notes.md`); a settled question
  is not re-asked.
- **Decisions are revisable** (principles §27): when evidence shows one wrong, say so at once with
  the change proposed as a new decision.
- **Report every architectural issue noticed**, now, while it is cheap (2026-10-21).
- **Propose the properly designed fix**, even when it is a gate edit; never a workaround around a gate
  (2026-09-24, 09-26, 09-30). Gates stay the owner's; the fix goes to them as a tested patch.
- **Break early:** no compatibility shims, migrations-for-safety or kept legacy names during the beta;
  strip such compromises from agents' recommendations (2026-10-31).
- **No outside commitments during the beta:** upstream patches are drafted and parked.
- **Batching:** a bug found mid-batch joins it only if it seriously hurts using the app; anything else
  is filed under "Next batch" in the plan file.
- **Verify a claim against the code before writing it** into a proposal or report; a correction from
  the owner goes into [`agent-failures.md`](agent-failures.md).
- **`yoloai files orpheum get <file>`** is how the owner fetches from the shared files (sandbox name
  first).

## 7. Code habits the owner asked for

- **Names say what they hold**, quantity and unit, read from the use site (audit checklist items).
- **Values:** an admin's choice is a DB setting with a starting value and its source note; a
  deployment fact a config key; a constant needs a stated reason (2026-10-20).
- **Digging means missing logs:** when explaining a report took digging, the fix adds the log line
  that would have said it.
- **One path for one kind of decision:** a bug in a second trigger of a fixed mechanism is fixed by
  routing both through one path.
- **Copy, don't publish, internal helpers** to public code or connect: byte-identical twins at the same
  path, listed in [`look-alike-watch-list.md`](look-alike-watch-list.md).
- **Generated data is regenerated, never hand-edited.**
- **Browser-specific policies are fine**, by feature detection; bounds per browser.
- **Platform misbehaviour:** read [`idiosyncrasies.md`](idiosyncrasies.md) and search the trackers
  before forming theories; add an entry with the workaround.
- **Research documents go in `design/research/`**, never only in the shared files.
- **Cite a decision after reading its heading** (`grep -n "^## D107 "`); anchors are not guessed.
- **Every Makefile defaults to `help`**; every target has a `##` line. Scripts find sibling repositories
  by relative path.
- **Between milestones, audit** with [`audit-checklist.md`](audit-checklist.md) and add to
  [`plan-deviations.md`](plan-deviations.md).

## 8. Hazards

- **Never touch `/tank/media`** (stat, ls, find, ffprobe): a hard NFS mount that froze the whole turn
  when nas.example was away (2026-10-21). Judge the share by staging's node.
- **Never change the sandbox machine** (packages, fonts, global configuration): a system font broke
  the browser tests for a day (2026-10-26).
- **No `pgrep -f` waits or kills:** it matches its own shell. Use `ps aux | grep "[x]"` and a separate
  kill.
- **nas.example's third library (id 3) is private:** never write a title, name or path from it anywhere;
  file ids only, logs redacted.
- **After `git stash` under a running dev loop, restart Vite** (it served stale modules, a white
  screen).
