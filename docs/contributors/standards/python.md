> **ABOUTME:** How Python is written here — only for repository tooling (the gate scripts and
> agent hooks), standard library only, typed, split into pure checks and a thin I/O shell, and
> tested with unittest, including the tests that make each gate fail. Each rule names its
> enforcement.

# Python Standard

Python is the language of **repository tooling**: the gates under `scripts/` (`check_docs.py`,
`check_source.py` and those to come) and agent hooks under `.claude/hooks/` when a hook
outgrows shell. **The product contains no Python.** The server and node are Go, the clients
TypeScript ([principles §4](../principles/README.md#4--one-way-to-do-it)); a new Python surface
anywhere else is a decision, not a convenience.

Every rule names its enforcement; *(planned)* marks a mechanism that does not exist yet.

## 1. Standard library only, on the CI interpreter

- **No third-party packages**, including for tests: no pip, no `requirements.txt`, no virtual
  environment. A gate that needs an install is a gate that can fail to run.
- The floor is **Python 3.12**, the system `python3` of CI's `ubuntu-24.04` runner. Development
  hosts run newer (3.13); nothing newer than 3.12 is used.

**Enforced by:** CI installs no Python packages (`.github/workflows/ci.yml`), so a third-party
import or a newer-only feature fails `make check` there.

## 2. The header and the docstring

```python
#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Source hygiene gate run by `make check`: every source file carries the SPDX
# ABOUTME: licence identifier its directory requires, and no file outgrows one read.
"""Check licence headers and file sizes across the tracked source tree.

Why it exists: the failure it catches, and where that failure happened.
What it deliberately does not check, and why.
"""

from __future__ import annotations
```

- The shebang only on files meant to be executed; the SPDX line within the first five lines;
  the ABOUTME says what the file is **for**.
- **The module docstring of a gate carries its reason**: the failure it exists to catch, with
  the specimen, and what it deliberately leaves unchecked. A gate whose scope is not written
  down gets widened until it fires on things it was never meant to police, and then gets
  switched off.
- `from __future__ import annotations` in every module.

**Enforced by:** `scripts/check_source.py` requires the SPDX line and checks its value against
the directory; ABOUTME and docstring are **review-only**.

## 3. Types

- **Every function outside `scripts/tests/` annotates its parameters and its return.** Modern
  forms: `list[str]`, `dict[str, int]`, `X | None`, `Path`.
- Records are `@dataclass(frozen=True)`, not tuples indexed by position or dicts with fixed
  keys.
- No `Any` where the shape is known.

**Enforced by:** **review-only**. No type checker is installed, and installing one would break
§1. The mechanical form, if annotations start to go missing, is an `ast` walk added to
`check_source.py` that fails on an unannotated function.

## 4. Shape: pure checks, a thin shell around them

- **The checking logic is pure functions** that take text or parsed structures and return
  findings: `def links(body: str) -> list[Link]`. They open no files and run no commands, so
  a test feeds them a string.
- **I/O lives at the edge**: one function lists files (from `git ls-files`, never a hand-written
  list or a directory walk that sees `node_modules`), one reads them, and `main()` wires the
  two and returns an exit code.
- **Inputs are derived from the thing they describe.** A gate that needs the Makefile's test
  roots parses the Makefile rather than restating them, so it cannot drift from what it checks.
- The script ends with `if __name__ == "__main__": sys.exit(main())`.

**Enforced by:** **review-only**; the tests of §6 are only easy to write when this holds, which
is the practical pressure.

## 5. Output and errors

- **One finding per line, `path:line: message`**, the form editors and CI annotate. The message
  says what is wrong and what to do: *"link '../x.md' names docs/x.md, which does not exist"*.
- A summary line, then **exit 1 on any failure**, 0 otherwise. A warning that does not fail is
  printed with `warning:` and is rare; a gate that only warns is not a gate.
- **Subprocesses** run as `subprocess.run([...], check=True, capture_output=True, text=True)`
  with an argument list; never `shell=True`, never `os.system`. When one fails, the error
  includes its stderr.
- **Raise specific exceptions**; no bare `except:`, no `except Exception: pass`. A gate that
  hits something it cannot parse **fails**, naming the file. It never treats unparseable input as
  clean.

**Enforced by:** **review-only**.

## 6. Tests: `unittest`, and each gate shown failing

- Tests live in `scripts/tests/test_<script>.py` and run with
  `python3 -m unittest discover -s scripts/tests`, which `make check` and CI both run.
- **Every `check_*.py` has a test file, and every check in it has a test that makes it fail and
  one that makes it pass** ([testing.md](testing.md#2-every-check-is-first-seen-to-fail)). A
  gate only ever observed passing cannot be told apart from one that cannot fail.
- **A matcher is tested on what it must ignore** — code spans, fenced blocks, comments, string
  literals — as well as what it must catch. yoloAI's first line-scanning drafts flagged their
  own source.
- **Table cases use `self.subTest`**, or a dict of input to expected output as
  `test_check_docs.py` does for slugs.
- **Synthetic trees** are built in `tempfile.TemporaryDirectory()`. Tests never read the real
  repository's documents as fixtures, because then an edit to a document breaks a gate's tests.
- **git is hermetic**: a test that runs git sets `GIT_CONFIG_GLOBAL=/dev/null` and passes its
  identity explicitly.

**Enforced by:** `make check-scripts` runs the tests in `make check` and CI. That every gate
has failing cases is **review-only**; a check that each `scripts/check_*.py` is imported by some
test is cheap to add if one ever ships without.

## 7. Claude Code hooks

The hooks in `.claude/hooks/` are how the gates reach an agent, and they are Python so their
decisions can be tested (`scripts/tests/test_hooks.py`). They share `hooklib.py`. Each one:

- **reads its event as JSON from stdin**, never prompts, and finishes in well under a second —
  except the Stop hook, whose job is to run the gate;
- **exits 0 on every path except a deliberate refusal or block.** A hook that crashes has
  stopped gating without saying so, so an event it cannot read is treated as nothing to act on
  rather than raised;
- **stamps, then acts.** The post-edit hook records whether code or only documents changed; the
  Stop hook then runs `make check`, or `make check-documentation` after a documents-only turn, once, where
  the tools are (`scripts/run_gate.py`), and blocks the turn with the output if it fails;
- **says what to do instead** when it refuses. The pre-edit hook's refusal of a gate file
  names why the file is protected and how to propose the change.

**Enforced by:** `make check-scripts` runs the hooks' tests; the pre-edit hook refuses an agent's
edit under `.claude/`, so the hooks cannot be loosened by the agent they govern.

## 8. Style

- PEP 8, **4-space indentation** (`.editorconfig`), lines up to **100 columns**.
- `pathlib.Path` for paths, f-strings for formatting, explicit imports (no `*`), imports grouped
  standard-library then local.
- Names spelled out: `decision_id`, not `did`.

**Enforced by:** `.editorconfig` for indentation; the rest is **review-only**. No formatter is
installed, for the reason in §1.
