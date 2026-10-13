# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Revert-red's probe worktrees: kept in the git directory, held by one run at a time, and
# ABOUTME: given the web packages the working tree installed, as links (check_revert_red.py).
"""Revert-red's probe worktrees, and the installed web packages mirrored into them as links."""

from __future__ import annotations

import contextlib
import fcntl
import os
import subprocess
from pathlib import Path


def _git(*args: str, cwd: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], capture_output=True, text=True, check=False, cwd=cwd)

# Package caches a run writes, which the worktree keeps its own of rather than share.
PACKAGE_CACHES = (".vite", ".vite-temp", ".vitest", ".cache")


def link_installed_packages(source: Path, tree: Path) -> None:
    """Give the worktree the web packages the working tree has installed, without installing.

    git worktree copies only tracked files, so node_modules is absent, and the web suite cannot
    run. Each node_modules under web/ is made in the worktree as a directory of links: an
    installed package links to the working tree's copy, and a workspace link (@orpheum/sdk ->
    ../../packages/sdk) is copied as the relative link it is, so it resolves to the worktree's
    own package, the one a revert changes.
    """
    web = source / "web"
    if not web.is_dir():
        return
    for installed in sorted(web.rglob("node_modules")):
        relative = installed.relative_to(source)
        if "node_modules" in relative.parts[:-1] or not installed.is_dir():
            continue
        _mirror_packages(installed, tree / relative, scoped=True)


def _mirror_packages(installed: Path, into: Path, scoped: bool) -> None:
    into.mkdir(parents=True, exist_ok=True)
    for entry in installed.iterdir():
        if entry.name in PACKAGE_CACHES:
            continue
        target = into / entry.name
        if entry.is_symlink():
            target.symlink_to(os.readlink(entry))
        elif scoped and entry.name.startswith("@") and entry.is_dir():
            _mirror_packages(entry, target, scoped=False)
        else:
            target.symlink_to(entry.resolve())


@contextlib.contextmanager
def hold_probe_trees():
    """The kept trees are shared by every worktree of the repository, so two runs at once (two
    agents' worktrees, or a hook beside a manual run) would reset each other's trees mid-probe.
    A run holds this lock on the git common directory for as long as it uses them."""
    common = Path(_git("rev-parse", "--path-format=absolute", "--git-common-dir").stdout.strip())
    (common / "revert-red").mkdir(exist_ok=True)
    with open(common / "revert-red" / "lock", "w", encoding="utf-8") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print("Another revert-red run is using the probe trees; waiting for it to finish.", flush=True)
            fcntl.flock(lock, fcntl.LOCK_EX)
        yield


def probe_trees(head: str, count: int) -> list[str]:
    """`count` worktrees to probe in, at fixed paths inside the repository's git directory, made
    once and kept: a Go build or test cached in one is found again next run, which a fresh path
    would miss. Each is set back to `head` and cleaned of untracked files, keeping the ignored
    ones (the linked node_modules).

    `head` is resolved to a commit here, in the working tree: a ref such as HEAD names a
    different commit inside each kept tree (the one it was last set to), so passing it through
    would set every tree back to where it already was and probe a stale copy of the code."""
    resolved = _git("rev-parse", "--verify", "--quiet", f"{head}^{{commit}}")
    if resolved.returncode != 0:
        raise RuntimeError(f"{head!r} is not a commit this repository can resolve")
    head = resolved.stdout.strip()
    common = Path(_git("rev-parse", "--path-format=absolute", "--git-common-dir").stdout.strip())
    source = Path(_git("rev-parse", "--show-toplevel").stdout.strip())
    trees = []
    for index in range(count):
        tree = common / "revert-red" / f"wt-{index}"
        if (tree / ".git").exists():
            reset = _git("checkout", "--detach", "--force", head, cwd=str(tree))
            cleaned = _git("clean", "-fdq", cwd=str(tree))
            if reset.returncode != 0 or cleaned.returncode != 0:
                raise RuntimeError(f"could not reset {tree}: {reset.stderr}{cleaned.stderr}")
        else:
            _git("worktree", "prune")
            added = _git("worktree", "add", "--detach", str(tree), head)
            if added.returncode != 0:
                raise RuntimeError(f"could not make {tree}: {added.stderr}")
            link_installed_packages(source, tree)
        trees.append(str(tree))
    return trees
