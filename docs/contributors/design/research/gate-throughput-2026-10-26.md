> **ABOUTME:** Where the gates, tests and hooks do the same work twice, fight each other for the
> machine, or trust a result no gate saw, and what would keep every guarantee while making the
> edit–check–commit loop and check-pr faster. A proposal for the owner, audited by an independent
> agent the same day: nearly all of it lands in `Makefile`, `scripts/` and `.claude/`, which are the
> owner's (AGENTS.md rule 2); no agent applies it.

# Gate throughput: the same safety, less waiting

- **Date:** 2026-10-26.
- **Requested by** the owner, 2026-10-26: "take a holistic view of the gates, testing, hooks, etc
  and look for ways to keep our safety but improve throughput"; "I suspect … there is probably work
  being run multiple times that doesn't have to."
- **Builds on:** the outside report `/yoloai/files/yoloai-slowdown-2026-10-26.md` (CPU saturation,
  leaked worktrees, the one-sided check-pr lock; its R1–R6 are referred to by number),
  [makefile.md](../../standards/makefile.md) §4, [test-weight.md](../../test-weight.md) §7, §15, §23.
- **Audit:** an independent agent checked every claim against the code (2026-10-26). It found two
  claims of the first draft wrong (Go's caches across worktrees: `-trimpath` is already set,
  `Makefile:38-43`, since 58294774; check-pr's step memory: it exists, `check_pr_steps.py`,
  `revert_red_known.py`), one live safety hole the draft missed (§2.1), and hardening each change
  needs. This version carries its corrections; items it found are marked *(audit)*.
- **Labels:** **[M]** measured on this sandbox on 2026-10-26; **[R]** read in the code at
  An earlier commit; **[I]** inference, to be confirmed before building on it.
- **The rule kept:** no check is dropped. A run is removed only when its result is already known
  for the same inputs in the same environment, or when it fails for load rather than for the code.
  `make check-everything` (nightly, trusting no stamp) stays the backstop for every stamp.

---

## 1. What happened today

| Event | Cost | Cause |
| --- | --- | --- |
| `check-pr` on an earlier commit | **1545 s** (budget 300), failed **[M]** | base an earlier commit, 18 commits: the previous batch ended on `check-pr-failed` without the full `check-pr` makefile.md §4 step 3 requires, so the base never advanced. A process slip, not a gate defect *(audit)*. |
| its browser revert-red probe | failed with **nothing reverted**; the same 120 tests passed alone in its worktree minutes later **[M]** | 4 probes at once, each with an unlimited test run (§2.4) |
| two agents in worktrees | every `Edit`/`Write` refused; one agent wrote files through `python3` in Bash instead **[M]** | the hooks' root is the main checkout (§2.5) |
| a commit's browser tests | 2 tests failed on text width **[M]** | the gate ran on the host, which had a font the image does not (§2.7) |

---

## 2. Findings and recommendations, in the order to build them

### 2.0 G0 — `make check` says how long each step took

`scripts/quiet_gate.py` keeps no durations; check-pr prints its steps' (`check_pr_steps.py
durations`) **[R]**. Without them no later item can be ranked by measurement. Cheap, safe.

### 2.1 A — The stamp records the tree the gate saw, not the tree after it *(audit; a safety fix)*

`check-verbose` writes the stamp after the run, from the tree as it is then (`Makefile:79`,
`check_stamp.py:51`) **[R]**. An edit made while `make check` ran (a subagent in the same tree, an
editor's save) is recorded as passed, and the next commit of that tree skips the gate
(`.githooks/pre-commit:12`). check-pr does not re-run the unit tests or lint, so only the nightly
would catch it. `area_stamp.py:10-12` already takes its tree *before* the command for exactly this
reason. **Change:** take `working_tree_id()` before the parallel steps; write it at the end only if
the tree still matches. Strictly strengthens the gate. Prerequisite for G2.

### 2.2 G2 — The Stop hook honours the stamp

`on_stop.py:22-31` runs `make check` whenever a pending mark exists, without consulting the stamp
**[R]**; the pre-commit hook does consult it. A turn that commits and then ends runs the gate twice
on the same tree. **Change (after A):** when the stamp equals `working_tree_id()` (not `fresh`,
which also needs the index to match, `check_stamp.py:58-59`), clear the marks and return. **Gain:**
in one environment the second run is mostly cached (Go's test cache, the area stamps, lint's cache),
so tens of seconds per committing turn; a full gate when the two runs were in different
environments (§2.7) *(audit)*.

### 2.3 B — The web's static checks skip a turn that did not touch them *(audit)*

`web-typecheck`, `web-lint` and `web-format-check` run on every `make check`, Go-only turns
included (`Makefile:207-212`) **[R]**; `tsc -b`, eslint and prettier were among the top CPU users
the outside report saw. **Change:** put them behind `area_stamp.py` on `web` and `api`, as
`web-deps-check` and `web-test-browser` already are. A Go change that needs regenerated SDK types
still fails `check-generated`, and the regenerated file lands under `web/`, which invalidates the
stamp. Likely a larger gain than any of §2.8.

### 2.4 G6 — Revert-red's probes share the machine

Revert-red probes `cpu_count // 2` files at once (`check_revert_red.py:716`), each running
`go test` or `vitest` with no `-p` or worker limit (`revert_red_attempts.py:38-39`) **[R]**: up to
about 32 CPU-hungry processes on 8 vCPUs. Today a probe failed with nothing reverted and passed alone
**[M]**. **Change:** probes × per-probe parallelism ≤ CPUs (for example 4 probes, `go test -p 2`,
vitest `--maxWorkers 2`), the principle test-weight.md §15 already applied to check-everything.
Fewer load reds, a lower memory peak (the 24 GiB guest swapped), no loss of coverage.

### 2.5 G4 — The hooks work inside a worktree, without opening a way around them

`hooklib.ROOT` is `CLAUDE_PROJECT_DIR` (`hooklib.py:14`); agent worktrees are at
`.claude/worktrees/<name>/`; `gate_paths.relative()` (`:111`) makes every file there match
`^\.claude/` (`:31`) **[R]**, so every edit is refused **[M]**, and today's agent then wrote through
Python, which no hook sees: the real safety failure. **Change, hardened** *(audit)*: resolve a
per-file root with `git -C <nearest existing dir> rev-parse --show-toplevel`, but accept it only
when its `--git-common-dir` equals ROOT's, i.e. a linked worktree of this repository; otherwise use
ROOT. Without that check, `git init scripts` would make `scripts/check_docs.py` read as
`check_docs.py` and pass. Route `is_committed_migration` (`gate_paths.py:97`), `post_edit`'s
formatter and `pre_bash.gate_writes` (`pre_bash.py:213`) through the same root.

### 2.6 G5 — One gate at a time, by a lock both sides take (R3)

check-pr takes an `flock` (`check_pr_snapshot.py:112-115`); `make check` and the hooks only look
for it **[R]**. **Change:** R3, with three conditions *(audit)*: the lock is **re-entrant**
(check-pr's own sub-makes, `pr-step-%`, `test-browser`, revert-red's browser runs, run under the
lock their parent holds; a plain `flock` deadlocks them), by an inherited descriptor or an
environment mark; `BROWSER_MODE_LOCK` moves from `/tmp` (`Makefile:227`) to the git common dir, so
it holds across the host and the container; and a second gate **queues** in make rather than being
refused by `pre_bash` (refusing would stop the lead whenever a subagent gates).

### 2.7 G1 — One environment for every gate run (redesign first)

`run_gate.py:55` runs the gate on the host when the host has Go and Node, else through `./dev`
**[R]**. In this sandbox the Stop hook therefore gates on the host, while `./dev git commit` gates in
the image, whose Go, npm and Playwright caches are named volumes (`dev:64-65`) separate from the
host's **[R]**. Each environment repeats what the other did, and they drift (the font **[M]**;
rtcapi's IPv6 earlier). **Not as a switch** *(audit)*: `./dev` replaces its container when its
configuration hash changes, and the hash includes the dev loop's `ORPHEUM_DEV_*` variables
(`dev:200-210`), so a Stop hook running `./dev` without the dev loop's exact environment kills the
dev loop and staging; a linked worktree's `.git` points into the main `.git/worktrees/`, which
`./dev` does not mount (`dev:130`), so git-reading steps would fail in a worktree container
**[I]**; and an image rebuild would land inside the Stop hook's 600 s. **Change:** a separate gate
container (not the dev loop's), mounting the git common directory, with the image kept built ahead.
The largest gain of the list once done, and the largest change.

### 2.8 Small, measure first

- **`go vet`** runs six times and golangci-lint five times per `make check` (`go-fmt`, two `go-lint`,
  two in `go-speculative`) **[R]**. golangci's `govet` is `enable-all` but `fieldalignment`
  (`.golangci.yml:264-267`) and no file is `//go:build !integration`, so the amd64 `go vet ./...` and
  `go vet -tags integration` look subsumed by the integration-tag lint *(audit)*; drop them only
  after showing the same findings. The arm64, riscv64, darwin and dev passes stay (D205). Go caches
  vet, so the warm gain is small. `--allow-serial-runners` on `go-lint` is a warning sign by
  test-weight.md §7.
- **`browsers` and `web-deps`** run several times per check-pr (`Makefile:282`, `:465`, the
  documentation shortcut) *(audit)*: run once; skip on the documentation path.

### 2.9 Considered and not proposed

- **Cross-worktree Go caches (`-trimpath`)**: already done, `Makefile:38-43`.
- **Per-session pending marks** (first draft's G3): the lead's `make check` gates the whole tree
  whatever the marks, so it would help only a lead that edited nothing, and it would end the
  guarantee that a turn ends only on a passing tree *(audit)*. Moot once build agents work in
  worktrees (G4).
- **check-pr skipping steps by input fingerprints, or a targeted green counting as a pass** (first
  draft's G7): makefile.md §4 step 3 decided against it ("a targeted green is never the gate's"),
  and the commit-range steps (`check-test-weakening`, `lint-commits`, `check-decision-status`) have
  no file inputs: a new commit with a bad trailer would be skipped. Only if the owner revises §4,
  and never for those three.
- **`check-commit-red` beside check-pr's revert-red**: a deliberate duplicate (makefile.md §4, the
  owner, 2026-10-15), already shared through `revert_red_known.py` (test-weight.md §23).
- **`check-pr-documentation` repeating `check-documentation`**: seconds.

## 3. The outside report's recommendations

| | Verdict |
| --- | --- |
| R1 cleanup | After the unfinished work in D338/D349/D350's worktrees is finished (the owner, 2026-10-26). |
| R2 lossless reclaim | As an explicit command at session start, not at every gate: git's worktree lock is no liveness signal (the D340 agent's stayed locked after it finished). |
| R3 symmetric lock | G5, with re-entrancy, which the report lacks *(audit)*. |
| R4 cap subagents by hook | Deferred: matcher name (`Agent` or `Task`) and lifetime unverified; the lead keeps the 2–3 agent budget. |
| R5 yoloai `SIZE`, R6 host headroom | Agreed; outside this repository. The guest's page cache is host memory the host cannot reclaim (virtiofs), so a smaller guest costs only cache once the gates' peak fits; G6 lowers that peak. |

## 4. Order

1. **G0** durations and **A** the stamp race (a safety fix).
2. **G2** the Stop hook's stamp; **B** the web's area stamps.
3. **G6** revert-red's sizing.
4. **G4** hooks in worktrees, hardened.
5. **G5** the re-entrant lock.
6. **G1** the gate container, once designed.
7. §2.8 after measuring.
