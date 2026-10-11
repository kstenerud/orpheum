# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Gives revert-red's probe worktrees the web packages the working tree installed, as
# ABOUTME: links, so the web suite runs in them without an install (check_revert_red.py).
"""The installed web packages, mirrored into a probe worktree as links."""

from __future__ import annotations

import os
from pathlib import Path

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
