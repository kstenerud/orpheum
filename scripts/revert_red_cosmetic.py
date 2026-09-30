# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Whether a file's change cannot affect behaviour (comments, formatting), so revert-red
# ABOUTME: reports it COSMETIC rather than probing it (check_revert_red.py).
"""Changes no test could notice, which revert-red reports rather than probes."""

from __future__ import annotations

import ast
import re
import subprocess

GO_SOURCE = re.compile(r"\.go$")
PY_SOURCE = re.compile(r"\.py$")


def _git(*args: str, cwd: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], capture_output=True, text=True, cwd=cwd, check=False)


def changed_lines(base: str, head: str, path: str, cwd: str | None = None) -> list[str]:
    """The added and removed content lines for one path, without their +/- marker.

    `-U0` so context lines are not mistaken for changes, and `--ignore-all-space`
    so a reindent is not either.
    """
    out = _git(
        "diff", "-U0", "--ignore-all-space", f"{base}...{head}", "--", path, cwd=cwd
    ).stdout
    lines = []
    for line in out.splitlines():
        if line.startswith(("+++", "---", "@@", "diff ", "index ", "new file", "deleted file")):
            continue
        if line.startswith(("+", "-")):
            lines.append(line[1:])
    return lines


def is_cosmetic(tree: str, base: str, head: str, path: str) -> bool:
    """Whether the change to `path` cannot affect behaviour, so no test could notice.

    A `feat` commit that adjusts a doc comment alongside its real work leaves this
    gate reporting the comment's file as an uncovered behaviour change, because
    reverting a comment of course breaks nothing. That is a false positive, and this
    gate's whole claim on a contributor's attention is that it does not produce them
    — yoloAI's specimen was a two-line doc-comment edit on `applyBrokerOption`,
    reported identically to a real missing test.

    Both tests below are *exact within their guard* and decline to classify whenever
    they cannot be sure. Declining means the file is probed as before, so the worst
    case is the behaviour we already had.

    * **Go**: every changed line is a `//` comment or blank, and neither version of
      the file contains `/*` anywhere. Without that second condition a changed line
      inside a block comment is indistinguishable from code by inspection, and a
      guard that reads `*p = 0` as a comment would suppress a real finding.
    * **Python**: the two versions parse to the same AST. That covers `#` comments
      and formatting together. A docstring edit is *not* covered — docstrings are
      AST nodes — so it still reports as it does today.
    """
    if GO_SOURCE.search(path):
        before = _git("show", f"{base}:{path}", cwd=tree).stdout
        after = _git("show", f"{head}:{path}", cwd=tree).stdout
        if "/*" in before or "/*" in after:
            return False  # cannot tell a block-comment line from code
        changed = changed_lines(base, head, path, cwd=tree)
        return bool(changed) and all(
            not line.strip() or line.strip().startswith("//") for line in changed
        )

    if PY_SOURCE.search(path):
        try:
            before = ast.dump(ast.parse(_git("show", f"{base}:{path}", cwd=tree).stdout))
            after = ast.dump(ast.parse(_git("show", f"{head}:{path}", cwd=tree).stdout))
        except SyntaxError:
            return False  # unparseable on either side; let the probe speak
        return before == after

    return False
