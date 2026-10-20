#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Formats files in place with the formatters the gate checks, so a formatting slip is fixed
# ABOUTME: before any check runs: the hook after an edit, and the pre-commit hook on what is staged.
"""Format files with the gate's own formatters, before the gate looks (the owner, 2026-10-20).

    python3 scripts/format_files.py <path>...   format these files, as the hook after an edit does
    python3 scripts/format_files.py --staged    format the staged files and stage them again, as the
                                                pre-commit hook does

Go files go through `golangci-lint fmt` (gofumpt and gci, what `make go-fmt` checks), the web's
through its own Prettier (what `make web-format-check` checks). Generated files are never touched:
their source makes them (AGENTS rule 7). A staged file that also has changes not staged is left as it
is, so a commit never takes in work its author did not stage; the gate then says it needs `make fmt`.

A formatter that is not here is skipped: the gate, which runs where the tools are, still checks.
"""

from __future__ import annotations

import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

from gate_paths import is_generated  # noqa: E402

WEB = "web/"
PRETTIER = Path("web") / "node_modules" / ".bin" / "prettier"


def go_files(rels: list[str], root: Path) -> list[str]:
    """The Go files among rels that are there and not generated."""
    return [rel for rel in rels if rel.endswith(".go") and not is_generated(rel) and (root / rel).is_file()]


def web_files(rels: list[str], root: Path) -> list[str]:
    """The web's files among rels that are there and not generated, relative to web/; Prettier's own
    ignore file and --ignore-unknown leave out the rest."""
    return [rel[len(WEB):] for rel in rels
            if rel.startswith(WEB) and not is_generated(rel) and "/node_modules/" not in rel
            and (root / rel).is_file()]


def format_files(rels: list[str], root: Path = ROOT) -> None:
    """Format rels, paths relative to root, in place with whichever formatters are here."""
    go = go_files(rels, root)
    if go and shutil.which("golangci-lint"):
        subprocess.run(["golangci-lint", "fmt", *go], cwd=root, check=False, capture_output=True)
    web = web_files(rels, root)
    if web and (root / PRETTIER).is_file():
        subprocess.run([str(root / PRETTIER), "--write", "--ignore-unknown", *web], cwd=root / "web",
                       check=False, capture_output=True)


def git_lines(root: Path, *args: str) -> list[str]:
    done = subprocess.run(["git", *args], cwd=root, capture_output=True, text=True, check=True)
    return [line for line in done.stdout.splitlines() if line]


def format_staged(root: Path = ROOT) -> list[str]:
    """Format the staged files that have no changes beyond what is staged, and stage again the ones
    the formatters changed. Returns those left as they were for their unstaged changes."""
    staged = git_lines(root, "diff", "--cached", "--name-only", "--diff-filter=ACMR")
    unstaged = set(git_lines(root, "diff", "--name-only"))
    left = [rel for rel in staged if rel in unstaged]
    wholly = [rel for rel in staged if rel not in unstaged]
    before = {rel: (root / rel).read_bytes() for rel in wholly if (root / rel).is_file()}
    format_files(wholly, root)
    changed = [rel for rel, text in before.items() if (root / rel).read_bytes() != text]
    if changed:
        subprocess.run(["git", "add", "--", *changed], cwd=root, check=True)
    return left


def main(argv: list[str]) -> int:
    if argv == ["--staged"]:
        for rel in format_staged():
            print(f"format_files: {rel} has changes not staged, so it was left as staged", file=sys.stderr)
        return 0
    if not argv or argv[0].startswith("-"):
        print(__doc__, file=sys.stderr)
        return 2
    format_files(argv)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
