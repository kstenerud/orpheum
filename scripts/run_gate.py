#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Runs a gate target (`make check`, `make check-docs`) where the tools are: directly in the
# ABOUTME: development image or on a host with Go and Node, otherwise through `./dev` (D235).
"""Where the hooks run the gate.

The git pre-commit hook and the Claude Code Stop hook both run the gate, and a machine set up the
D235 way has only git and a container engine: no Go, no Node. So the choice is made here, once,
for both (AGENTS rule 11):

1. Inside the development image (it sets IN_DEV_IMAGE), or on a host with every tool in
   TOOLCHAIN on PATH, `make <target>` runs directly.
2. Otherwise, when `./dev` finds a container engine, `./dev make <target>` runs it in the image.
3. Otherwise NoWayToRun says how to fix the machine.

    python3 scripts/run_gate.py check
"""

from __future__ import annotations

import importlib.machinery
import importlib.util
import os
import shutil
import sys
from collections.abc import Callable, Mapping
from pathlib import Path
from types import ModuleType

ROOT = Path(__file__).resolve().parents[1]
LAUNCHER = ROOT / "dev"
# Set to 1 by deploy/Containerfile; scripts/tests/test_dev_image.py holds the two together.
IN_DEV_IMAGE = "ORPHEUM_IN_DEV_IMAGE"
# The tools the gate needs that only the image or a set-up host has (python3 runs this script).
# A host with them but without the Makefile's linters is told by `make check` to run `make tools`.
TOOLCHAIN = ("go", "node")


class NoWayToRun(Exception):
    """Neither the tools nor a container engine are here."""


def load_launcher() -> ModuleType:
    """`./dev` as a module. It has no .py suffix, so it is loaded by path."""
    loader = importlib.machinery.SourceFileLoader("dev_launcher", str(LAUNCHER))
    spec = importlib.util.spec_from_loader("dev_launcher", loader)
    module = importlib.util.module_from_spec(spec)
    loader.exec_module(module)
    return module


def gate_command(target: str, env: Mapping[str, str],
                 which: Callable[[str], str | None] = shutil.which) -> list[str]:
    """The command that runs `make target` here, or NoWayToRun."""
    if env.get(IN_DEV_IMAGE) == "1" or all(which(tool) for tool in TOOLCHAIN):
        return ["make", target]
    if load_launcher().find_engine(dict(env), which) is not None:
        return [str(LAUNCHER), "make", target]
    missing = ", ".join(tool for tool in TOOLCHAIN if not which(tool))
    raise NoWayToRun(
        f"make {target} cannot run here: {missing} not on PATH, and ./dev found no container "
        "engine. Install Docker or Podman so ./dev runs the gate in the development image, or "
        "install the toolchain on this machine (CONTRIBUTING.md, Setting up)."
    )


def main(argv: list[str]) -> int:
    if len(argv) != 1:
        print("usage: run_gate.py <make target>", file=sys.stderr)
        return 2
    try:
        command = gate_command(argv[0], os.environ)
    except NoWayToRun as missing:
        print(missing, file=sys.stderr)
        return 127
    os.execvp(command[0], command)
    return 0  # not reached: execvp replaces this process


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
