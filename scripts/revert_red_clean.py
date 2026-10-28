# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: A probe's red counts only if the same kind of run passes in that worktree with nothing
# ABOUTME: reverted: a run that fails on a clean tree too says nothing of the revert (A45).
"""Whether a kind of probe can pass in a worktree, asked once per worktree and kind.

Vite refused every file under a `.git` directory, where revert-red's worktrees live, so every
browser-mode probe failed before a test ran, and each came back red whatever was reverted: web files
passed as covered with no test having noticed (A45, 2026-10-26). A red now stands only once the same
command, run with the file and its companions restored, has passed there. The first red of each kind
in each worktree pays for that run; the rest reuse its answer, so the check costs a run per kind, not
per file. It catches a broken environment, not a test that fails at HEAD, which make check refuses.

A pass is also kept on disk, by the worktree, the commit its tree holds, the kind and the toolchain
(Go's and Node's versions), so check-pr-failed and a full re-run at the same commit do not pay for
it again: these runs were most of 1670 s of a check-pr's probing that no timing named (the caching
audit's finding 5, 2026-10-28). Each is timed as a run of its own kind, "<kind>, nothing reverted".
A failure is never kept: it is asked again.

**The tests that failed, again** (the owner, 2026-10-28, "go with your recommendations"): that run
once per worktree and kind proved only that the environment works; a red from a test that fails
under load, or at HEAD, still counted for every file after the first. So a red whose failed tests
its output names (Go's, vitest's, the gate scripts') now has just those run again with nothing
reverted, a few seconds: they must pass for it to count. That catches a broken environment too,
A45's, since those tests then fail as well. Only a red whose tests cannot be named (a build that
fails, a compile, the browser tier, output it cannot read) still takes the run once per kind above.
"""

from __future__ import annotations

import functools
import json
import subprocess
import threading
from collections.abc import Callable
from pathlib import Path

from revert_red_timings import kind_of

_CLEAN: dict[tuple[str, str], bool] = {}
# What a clean run that failed printed, its end: the failing test had to be found by reproducing the
# load by hand when it was not kept (2026-10-26, process review F8).
_FAILED_OUTPUT: dict[tuple[str, str], str] = {}
KEPT_LINES = 20
_LOCK = threading.Lock()


def probe_kind(command: list[str], label: str, env: dict[str, str] | None) -> str:
    """The kind of run a probe is, as the report names it, and the browser-mode tests apart from
    vitest's others: they run in a browser of their own, where an environment breaks on its own."""
    kind = kind_of(command, label, env)
    return f"{kind} (browser mode)" if "vitest.browser.config.ts" in command else kind


@functools.cache
def toolchain() -> str:
    """Go's and Node's versions here, which a kept pass holds for."""
    found = []
    for command in (["go", "env", "GOVERSION"], ["node", "--version"]):
        try:
            found.append(subprocess.run(command, capture_output=True, text=True, check=False).stdout.strip())
        except OSError:
            found.append("")
    return " ".join(found)


def kept_path(tree: str) -> Path | None:
    """Where the passes are kept: beside revert-red's other records, in the git common directory."""
    done = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], cwd=tree,
                          capture_output=True, text=True, check=False)
    return Path(done.stdout.strip()) / "revert-red" / "clean-runs.json" if done.returncode == 0 else None


def _key(tree: str, head: str, kind: str) -> str:
    return "\0".join((tree, head, kind, toolchain()))


def _kept(path: Path | None) -> dict[str, bool]:
    try:
        return json.loads(path.read_text(encoding="utf-8")) if path is not None else {}
    except (OSError, ValueError):
        return {}


def runs_clean(tree: str, kind: str, run: Callable[[], subprocess.CompletedProcess[str]], head: str = "") -> bool:
    """Whether a run of kind passes in tree with nothing reverted: run asks it, the first time for
    this tree and kind, with the probe's files already restored, unless a pass at head is kept. A
    run past its timeout has not passed."""
    with _LOCK:
        known = _CLEAN.get((tree, kind))
    if known is not None:
        return known
    path = kept_path(tree) if head else None
    if head and _kept(path).get(_key(tree, head, kind)):
        with _LOCK:
            _CLEAN[(tree, kind)] = True
        return True
    output = ""
    try:
        done = run()
        passed = done.returncode == 0
        output = (done.stdout or "") + (done.stderr or "")
    except subprocess.TimeoutExpired:
        passed = False
        output = "it ran past its timeout"
    with _LOCK:
        _CLEAN[(tree, kind)] = passed
        if not passed:
            _FAILED_OUTPUT[(tree, kind)] = "\n".join(output.strip().splitlines()[-KEPT_LINES:])
        elif head and path is not None:
            kept = _kept(path)
            # Only this commit's: an older one's passes can serve no later run.
            kept = {key: value for key, value in kept.items() if f"\0{head}\0" in key}
            kept[_key(tree, head, kind)] = True
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(json.dumps(kept, indent=1, sort_keys=True), encoding="utf-8")
    return passed


def clean_failure(tree: str, kind: str) -> str:
    """The end of what the clean run of kind in tree printed when it failed, or "" when it passed."""
    with _LOCK:
        return _FAILED_OUTPUT.get((tree, kind), "")


def failed_pass_clean(failed: list, run: Callable[[list[str]], subprocess.CompletedProcess[str]]) -> tuple[bool | None, str]:
    """Whether the runs of a red's failed tests (failed, revert_red_known.runs_of's) all pass with
    nothing reverted, run by run, and the end of what a failing one printed (F8); None when there are
    none to run, and the kind's clean run decides."""
    if not failed:
        return None, ""
    for attempt in failed:
        try:
            done = run(attempt.command)
        except subprocess.TimeoutExpired:
            return False, "it ran past its timeout"
        if done.returncode != 0:
            output = (done.stdout or "") + (done.stderr or "")
            return False, "\n".join(output.strip().splitlines()[-KEPT_LINES:])
    return True, ""
