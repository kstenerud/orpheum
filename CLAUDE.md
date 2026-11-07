@AGENTS.md

# Claude Code specifics

Everything above is the shared agent contract, imported so that Claude Code and agents following
the `AGENTS.md` convention read the same rules. Only Claude-specific mechanics belong here.

## The gate runs itself

`.claude/settings.json` registers these hooks, committed so every clone gets them:

- **After an edit**, `.claude/hooks/post_edit.py` formats the file with the gate's formatters
  (`scripts/format_files.py`) and records that the tree changed: `docs` for documentation (`docs/`
  and any Markdown file, `scripts/documentation_only.py`), `code` for the rest. The pre-commit hook
  formats what is staged too, so an edit made by a shell command is formatted before the gate looks.
- **At the end of a turn**, `.claude/hooks/on_stop.py` runs `make check` (also when no edit marked the
  tree but it is not the one the gate last passed: a shell write, a picked commit). While the owner iterates on a
  design, `touch .claude/.cache/check-deferred` defers it until the owner is happy, and `make check`
  refuses to run until the file is gone (docs/contributors/standards/design-cycle.md) (or only
  `make check-documentation`, which runs no test of the code, after a documentation-only turn; the
  pre-commit hook and check-pr's tiers skip the code's tests for it too). On failure it **blocks the turn** and hands you the
  output. Fix it; don't argue with it. When the gate cannot run at all (no tools, or `./dev`'s image needs
  building), it blocks once; the next stop ends the turn with a warning, so a declined `./dev true` cannot loop. Each run it starts is recorded in `.claude/.cache/gate-runs.jsonl`: its start, length,
  exit, the load before and after, and the FFmpeg jobs running as it started, so a slow turn can be explained.
  While it runs, `.claude/.cache/gate-running.json` marks it, and `.claude/hooks/gate_status.py <project
  folder>` prints `make check: 2m10s` from the mark (nothing otherwise) for a personal status line to show,
  with its `refreshInterval` set. A project `statusLine` would replace each person's own, so the project
  ships the segment, not the line.
- **After a commit**, `.githooks/post-commit` starts revert-red's code part over the batch so far in
  the background, niced, on two probe trees (`scripts/background_probe.py`), so check-pr finds most
  files already probed; any other test run wanting the repository's test lock stops it. `touch .git/revert-red/background-off`
  while the owner tests on staging, and remove it after.
- **While `make check-pr` runs**, it runs on HEAD in its own worktree (`scripts/check_pr_snapshot.py`)
  and holds a lock: edit freely, but `.claude/hooks/pre_bash.py` refuses tests, lints and commits
  until it ends, and the Stop hook leaves its `make check` for the first turn after. If it fails
  while you have worked ahead, `git stash`, fix and commit, re-run what failed, then `git stash pop`.
- **Before an edit**, `.claude/hooks/pre_edit.py` refuses changes to a gate (configuration,
  `Makefile`, `scripts/`, hooks, allowlists, baselines, `internal/archtest/`), to generated code
  and to committed migrations, and says what to do instead. The list is
  `scripts/gate_paths.py`.
- **On cost** (docs/contributors/lead.md §1–2): the prompt hook shows the standing orders only when they
  changed or a compaction dropped them, and says when the context passes 120k; `pre_agent.py` refuses an
  agent launch naming no model, a fourth running, or one that can edit without a worktree; `on_tool_done.py` warns an agent at four fifths of the call budget its brief states (`Call budget: N`, 150 if none) and past it tells it
  to commit, report and stop. `scripts/token_usage.py` is the ledger, read at a batch's end.
- **One test run at a time** (the owner, 2026-11-04): `make check`, `make spot`, check-pr, revert-red,
  the background probe and a test command typed in a session (`go test`, `vitest`, `npm test`,
  `playwright`, …, which `pre_bash.py` runs through `scripts/gate_lock.py`, in its own shell, so a `cd`
  in it does not carry) hold one lock shared by every worktree; a test run started any other way (a script, an editor, `npx`, the
  owner's terminal) takes it too, through `scripts/gate_lock.py --for-pid`: vitest and Playwright in their
  global setup (`web/e2e/test-lock.ts`), `go test` through the `-exec` `make setup` adds to GOFLAGS
  (`scripts/go_test_exec`). A run that finds it held says what holds
  it and waits; `python3 scripts/gate_lock.py --held` prints the holder. The command holds the lock too,
  so a killed wrapper cannot let its tests run on outside it, and ending the wrapper ends the command's
  whole process group. Each kind of run has a lease (a hung run is ended at it). A foreground command
  whose wait for the lock would use more than half its timeout is refused, naming the holder and the
  queue: run it in the background, or with the longer timeout it names.
- **Before a shell command**, `.claude/hooks/pre_bash.py` refuses commands that add a
  dependency, `--no-verify` or repointing `core.hooksPath`, force-pushes, and shell writes whose
  target — a redirect, `tee`, `cp`/`mv`'s destination, `sed -i`, `rm` — is a gate file in this
  project. Mentioning a gate, reading one, or writing a scratch copy outside the project is fine.

A refusal is information: it names the sanctioned route. Take that route. Don't look for a way
around the hook. The shell guard is a speed bump, not a wall: a script can write a file without
naming it on the command line. The wall is the branch gate, which fails any change to a gate file that
lacks the owner's `Gate-Changed:` trailer.

**For the owner:** a session started with `ORPHEUM_ALLOW_GATE_EDIT=1` may edit gates, and one
started with `ORPHEUM_ALLOW_DEPS=1` may add dependencies. `1` covers the session alone: its subagents,
which inherit the environment, stay refused unless the value is `agents`. Both are read from Claude Code's own
environment, which nothing the agent runs can change. Use them for sessions whose job is gate or
dependency work, not for feature work.
