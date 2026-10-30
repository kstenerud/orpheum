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
  tree but it is not the one the gate last passed: a shell write, a picked commit) (or only
  `make check-documentation`, which runs no test of the code, after a documentation-only turn; the
  pre-commit hook and check-pr's tiers skip the code's tests for it too). On failure it **blocks the turn** and hands you the
  output. Fix it; don't argue with it.
- **While `make check-pr` runs**, it runs on HEAD in its own worktree (`scripts/check_pr_snapshot.py`)
  and holds a lock: edit freely, but `.claude/hooks/pre_bash.py` refuses tests, lints and commits
  until it ends, and the Stop hook leaves its `make check` for the first turn after. If it fails
  while you have worked ahead, `git stash`, fix and commit, re-run what failed, then `git stash pop`.
- **Before an edit**, `.claude/hooks/pre_edit.py` refuses changes to a gate (configuration,
  `Makefile`, `scripts/`, hooks, allowlists, baselines, `internal/archtest/`), to generated code
  and to committed migrations, and says what to do instead. The list is
  `scripts/gate_paths.py`.
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
