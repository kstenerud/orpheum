#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Generated-code gate run by `make check`: regenerates, and fails if any generated
# ABOUTME: file differs from what was there before — stale or hand-edited output.
"""Check that the committed generated code is exactly what its sources generate.

The first form of this gate ran `make generate` and then `git diff`, which compares the working
tree with git's index. That confused two things: generated code that is correct but not yet
staged failed as "stale", and the only way through was to stage it before the gate ran.

This version compares the generated files with themselves: it records every generated file
before, runs `make generate`, records them again, and fails if anything changed, appeared or
disappeared. A hand edit fails (the generator overwrites it); output left stale by an edited
source fails (the generator rewrites it); correct output passes whether or not it is staged.
Like the first form, it leaves the regenerated files in place, so a failure is fixed by
reviewing and committing them.
"""

from __future__ import annotations

import argparse
import difflib
import hashlib
import subprocess
import sys
from pathlib import Path

# Every path generated code lives under; scripts/gate_paths.py lists the same for the hooks.
GENERATED = ("internal/store/sqlcgen", "internal/store/cachegen", "api/openapi.json", "web/packages/sdk/src/gen")


def snapshot(root: Path, paths: tuple[str, ...] = GENERATED) -> dict[str, bytes]:
    """Every generated file's contents, by path relative to root."""
    found: dict[str, bytes] = {}
    for rel in paths:
        target = root / rel
        files = [target] if target.is_file() else sorted(p for p in target.rglob("*") if p.is_file())
        for file in files:
            found[file.relative_to(root).as_posix()] = file.read_bytes()
    return found


def differences(before: dict[str, bytes], after: dict[str, bytes]) -> list[tuple[str, str]]:
    """What regenerating changed: one `path:1: message` finding per file, and its diff (or "")."""
    found = []
    for path in sorted(set(before) | set(after)):
        if path not in before:
            found.append((f"{path}:1: generated, but not there before; commit it", ""))
        elif path not in after:
            found.append((f"{path}:1: there before, but the generator no longer writes it; delete it", ""))
        elif before[path] != after[path]:
            found.append((f"{path}:1: differs from what its source generates", short_diff(before[path], after[path])))
    return found


def short_diff(before: bytes, after: bytes, limit: int = 20) -> str:
    try:
        old, new = before.decode().splitlines(), after.decode().splitlines()
    except UnicodeDecodeError:
        return f"  (binary: {hashlib.sha256(before).hexdigest()[:12]} -> {hashlib.sha256(after).hexdigest()[:12]})"
    lines = list(difflib.unified_diff(old, new, "before", "regenerated", lineterm="", n=1))
    shown = "\n".join("  " + line for line in lines[:limit])
    return shown + (f"\n  … {len(lines) - limit} more lines" if len(lines) > limit else "")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--root", default=".", help="the repository root")
    args = parser.parse_args()
    root = Path(args.root)
    before = snapshot(root)
    done = subprocess.run(["make", "--no-print-directory", "generate"], cwd=root, check=False)
    if done.returncode != 0:
        print("check_generated: make generate failed", file=sys.stderr)
        return 1
    found = differences(before, snapshot(root))
    if found:
        for finding, _ in found:
            print(finding)
        print(f"check_generated: {len(found)} problem(s): generated code is stale or hand-edited. The regenerated "
              "files are in place; review them, and change the source, not the output, if they are wrong")
        for finding, diff in found:
            if diff:
                print(f"\n{finding.split(':', 1)[0]}:\n{diff}")
        return 1
    print(f"check_generated: {len(before)} generated files match their sources")
    return 0


if __name__ == "__main__":
    sys.exit(main())
