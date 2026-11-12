# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Revert-red's probe worktrees: kept in the git directory, held by one run at a time, and
# ABOUTME: given the web packages the working tree installed, as links (check_revert_red.py).
"""Revert-red's probe worktrees, and the installed web packages mirrored into them as links."""

from __future__ import annotations

import contextlib
import fcntl
import os
import shutil
import subprocess
from pathlib import Path


def _git(*args: str, cwd: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], capture_output=True, text=True, check=False, cwd=cwd)

# Package caches a run writes, which the worktree keeps its own of rather than share.
PACKAGE_CACHES = (".vite", ".vite-temp", ".vitest", ".vitest-cache", ".cache")


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
    """Brings `into` to the links `installed` calls for, whether it is new or a kept tree's: a
    package installed since gets its link, one removed loses it, a changed link is made afresh, and
    a run's own caches are left alone."""
    into.mkdir(parents=True, exist_ok=True)
    wanted = set()
    for entry in installed.iterdir():
        if entry.name in PACKAGE_CACHES:
            continue
        wanted.add(entry.name)
        target = into / entry.name
        if not entry.is_symlink() and scoped and entry.name.startswith("@") and entry.is_dir():
            if target.is_symlink():
                target.unlink()
            _mirror_packages(entry, target, scoped=False)
            continue
        link = os.readlink(entry) if entry.is_symlink() else str(entry.resolve())
        if target.is_symlink() and os.readlink(target) == link:
            continue
        _remove(target)
        target.symlink_to(link)
    for stale in into.iterdir():
        if stale.name not in wanted and stale.name not in PACKAGE_CACHES:
            _remove(stale)


def _remove(path: Path) -> None:
    if path.is_symlink() or path.is_file():
        path.unlink()
    elif path.is_dir():
        shutil.rmtree(path)


def seed_type_builds(source: Path, tree: Path) -> int:
    """Give the worktree the working tree's TypeScript build outputs (each web `.tsbuild`), so the type
    check a probe runs is incremental: a probe tree's app build info was found missing, its type
    check cold, 22 s alone and 122 s beside three other trees' probes (check-pr, 2026-10-20), where
    a seeded one takes 4 s. Sound however stale the copy: `tsc -b` checks each
    source against the hash its build info records and rebuilds what differs, and an importer of a
    changed export still fails (measured 2026-10-20, an SDK export renamed). Returns how many it
    seeded."""
    web = source / "web"
    seeded = 0
    for built in sorted(web.rglob(".tsbuild")) if web.is_dir() else []:
        relative = built.relative_to(source)
        if "node_modules" in relative.parts or not built.is_dir():
            continue
        target = tree / relative
        shutil.rmtree(target, ignore_errors=True)
        shutil.copytree(built, target, symlinks=True)
        seeded += 1
    return seeded


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
        set_kept_tree(source, tree, head)
        trees.append(str(tree))
    return trees


def set_kept_tree(source: Path, tree: Path, head: str) -> None:
    """Sets the kept worktree at `tree` to the commit `head`, cleaned of untracked files but keeping
    the ignored ones (the linked node_modules), or makes it there; then links the working tree's
    installed packages into it and seeds its type builds from `source`, the working tree. The links
    are brought up to date at every set, not only when the tree is made: a package installed after
    a kept tree was made was missing from it, and revert-red failed on "Failed to resolve import"
    (the release's check-pr, 2026-11-12)."""
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
    seed_type_builds(source, tree)
