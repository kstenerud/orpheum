> **ABOUTME:** How shell scripts are written here — which contexts get which interpreter and
> strict mode, the header every script carries, quoting and ShellCheck, and when a script
> should have been Python or Go instead. Each rule names what enforces it.

# Shell Standard

Shell is glue: it strings together `git`, `go`, `jq` and the other tools, and hands anything
with logic to a language that has types and tests. The product never runs a shell: the server
and node start subprocesses through `internal/node/execx` with an argument list, never
`sh -c` (`go.md`). A shell between us and FFmpeg turns a space in a path into a parse
error that looks exactly like a refusal (yoloAI A22).

Every rule names its enforcement; *(planned)* marks a mechanism that does not exist yet.

## 1. Four contexts, four contracts

| Context | Where | Shebang | Strict mode | Why |
| --- | --- | --- | --- | --- |
| **Developer tooling** | `scripts/*.sh` | `#!/usr/bin/env bash` | `set -euo pipefail` | runs on developer machines and CI; any failure should stop it loudly |
| **Git hooks** | `.githooks/*` | `#!/usr/bin/env bash` | `set -euo pipefail` | run the same gates as CI before a commit is recorded; a failure should refuse the commit |
| **Test fakes** | `internal/testutil/testdata/*.sh` (the fake `ffmpeg`) | `#!/bin/sh` | `set -eu` | executed by tests on any POSIX host; stands in for a real binary |
| **Package maintainer scripts** | `packaging/` (later) | `#!/bin/sh` | `set -e` | run by `dpkg`, which Debian policy says may use a minimal `/bin/sh` |

Do not unify these: a maintainer script with bashisms breaks an install on a system whose
`/bin/sh` is dash, and a fake with `bash` in its shebang fails on a host without it. Claude
Code hooks are Python here (`.claude/hooks/*.py`, [python.md](python.md#7-claude-code-hooks)),
because their decisions need tests.

**Enforced by:** ShellCheck reads the shebang and checks the dialect it names (§3). The strict
mode line is **review-only**.

## 2. The header

Every script opens with its shebang, its licence line and an ABOUTME that says what it is for
and why it exists:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Prints the next free decision number by scanning every file that can define one.
# ABOUTME: A hand-written grep that misses a file is what mints duplicate IDs.
set -euo pipefail
```

The licence identifier is the one the directory requires ([D80](../decisions/working-notes.md)):
MPL-2.0 under `web/`, AGPL-3.0-or-later elsewhere. The ABOUTME says **why**, not a restatement
of the filename.

**Enforced by:** `scripts/check_source.py` requires the SPDX line within the first five lines
and checks it against the directory. The ABOUTME is **review-only**.

## 3. ShellCheck is clean, including files nobody has committed yet

Every `.sh` file passes `shellcheck` with no findings at the default severity. The file list
is **derived** — `git ls-files --cached --others --exclude-standard '*.sh'` — so a script is
checked from the moment it exists, not from the moment it is staged. A script is least
reviewed before its first commit.

- A suppression names the finding and says why it does not apply here, on the line above:

  ```bash
  # shellcheck disable=SC2086  # the list is space-separated flags, split on purpose
  go vet $vet_flags ./...
  ```

  "ShellCheck complained" is not a reason
  ([principles §20](../principles/README.md#20--a-gate-changes-on-purpose-never-to-land-a-change)).
- **The version should be pinned.** Locally `/usr/bin/shellcheck` is 0.10.0; CI installs
  whatever its runner's package archive carries. yoloAI let seven findings reach its main
  branch through a green local run because the two versions disagreed (DF187), so CI should
  install a pinned release by checksum and `check-tools` should compare versions *(planned)*.
- A missing `shellcheck` **fails** `make check`. It never prints "skipping".

**Enforced by:** `make check-scripts` runs `shellcheck` over every tracked or untracked `.sh`
file and everything in `.githooks/`; `check-tools` fails when `shellcheck` is absent.

## 4. Writing the script

- **Quote every expansion**: `"$path"`, `"${files[@]}"`. Paths here contain spaces — media
  libraries are full of them.
- **Arrays for lists**, not space-joined strings.
- **`$(…)`**, never backticks.
- **Temporary files** come from `mktemp`, and a `trap 'rm -rf "$tmp"' EXIT` removes them.
- **Read JSON with `jq`**, never with `grep` and `sed`. Hooks receive JSON on stdin.
- **No `curl | sh`**, no downloads at run time, and no `sudo` inside a script.
- **Lowercase names** for local variables; `UPPER_CASE` only for exported environment
  variables. `ORPHEUM_` prefixes this project's own.
- **Two-space indentation**, as `.editorconfig` sets.
- **Output:** results to stdout, diagnostics and errors to stderr, exit non-zero on failure.

**Enforced by:** ShellCheck catches unquoted expansions (SC2086), backticks (SC2006) and
masked return values (SC2155); the rest is **review-only**.

## 5. Git hooks

`.githooks/` holds the hooks `make setup` installs. They:

- **run the same targets CI runs** — `pre-commit` runs the gate, `commit-msg` the commit-message
  rules — so a human's commit meets the same bar as an agent's;
- **never prompt**, and print what failed and the command that reproduces it;
- **stay thin**: the logic lives in the Makefile and `scripts/`, where it is tested.

**Enforced by:** shellcheck (§3); the pre-edit hook refuses an agent's edit under `.githooks/`.
The rest is **review-only**.

## 6. When shell is the wrong language

Move to **Python** (for repository tooling, [python.md](python.md)) or **Go** (for anything the
product runs) when a script:

- passes about 100 lines, or has functions calling functions;
- parses anything structured other than JSON through `jq`;
- needs a data structure beyond a flat list;
- needs tests of its own logic. A gate always does, so gates are Python from the start.

**Enforced by:** `scripts/check_source.py` warns at 400 lines and fails at 800 for every source
file; the 100-line judgement is **review-only**.
