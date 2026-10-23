#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Runs a make target in the development image: at once when already in it, otherwise through
# ABOUTME: the main checkout's ./dev, in this directory, so a gate that needs the image runs itself there.
"""Run `make <target> [VAR=value...]` in the development image, from this directory.

    in_dev_image.py <target> [VAR=value...]

The acceptance shapes the container's loopback with tc and checks its MTU is a real link's, so it runs
only in the development image; check-pr ran it on the host and it failed asking to be run through
./dev, which a person then had to do (2026-10-23; the owner: prerequisites install themselves). In
the image (it sets ORPHEUM_IN_DEV_IMAGE) the target runs at once. Otherwise it runs through the main
checkout's ./dev, never a worktree's: check-pr's tree is a worktree inside the main checkout, which
./dev mounts at its own path, and whose web/node_modules the tree's links point into, where a
container of the tree's own could not see them.
"""

from __future__ import annotations

import os
import subprocess
import sys
from collections.abc import Mapping
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from run_gate import IN_DEV_IMAGE  # noqa: E402

# Run by ./dev's shell in the image: into the directory, then make with the rest of the arguments.
IN_DIRECTORY = 'cd "$1" && shift && exec make --no-print-directory "$@"'


def main_checkout(directory: Path) -> Path:
    """The main checkout this directory's git repository belongs to, a worktree's included."""
    done = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], cwd=directory,
                          capture_output=True, text=True, check=True)
    return Path(done.stdout.strip()).parent


def command(arguments: list[str], directory: Path, env: Mapping[str, str], main: Path) -> list[str]:
    """The command that runs `make arguments` in the development image from directory."""
    if env.get(IN_DEV_IMAGE) == "1":
        return ["make", "--no-print-directory", *arguments]
    if not directory.resolve().is_relative_to(main.resolve()):
        raise SystemExit(f"{directory} is outside {main}, which ./dev mounts; run it from inside the checkout")
    return [str(main / "dev"), "sh", "-c", IN_DIRECTORY, "sh", str(directory), *arguments]


def main(argv: list[str]) -> int:
    if not argv:
        print("usage: in_dev_image.py <target> [VAR=value...]", file=sys.stderr)
        return 2
    directory = Path.cwd()
    run = command(argv, directory, os.environ, main_checkout(directory))
    os.execvp(run[0], run)
    return 0  # not reached: execvp replaces this process


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
