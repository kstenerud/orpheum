#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Source hygiene gate run by `make check`: every source file carries the SPDX
# ABOUTME: licence identifier its directory requires (D80), and no hand-written file
# ABOUTME: outgrows what an agent reads whole (warn at 400 lines, fail at 800).
"""Check licence headers and file sizes across the tracked source tree.

Two rules, both about what a reader — human or agent — can rely on without asking:

* **The directory is the licence** (D80). Everything under `web/` is MPL-2.0; everything
  else is GPL-3.0-or-later. Each source file states its licence in an
  `SPDX-License-Identifier:` line within its first five lines, and the identifier must be
  the one its directory requires. A client file that says GPL, or a server file that says
  MPL, is exactly the drift that makes relicensing impossible later.
* **A file fits in one read.** A file that is read whole is understood whole; one that is
  read in pieces is where an agent sizes a fix from the part it happened to see. Files over
  400 lines are reported, files over 800 fail, and an exception needs a line in
  `scripts/filesize.allow` with its reason. Generated code is exempt: nobody edits it.

Files are taken from `git ls-files` (tracked plus untracked-but-not-ignored), so the gate
sees exactly what a commit would contain and never walks `node_modules`.
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

GPL = "GPL-3.0-or-later"
MPL = "MPL-2.0"

# Extensions that are source code and can carry a comment. JSON cannot hold one, and
# Markdown is documentation, which the docs gate covers.
SOURCE_SUFFIXES = frozenset(
    {".go", ".ts", ".tsx", ".js", ".mjs", ".cjs", ".css", ".html", ".py", ".sh", ".sql"}
)
SOURCE_NAMES = frozenset({"Makefile"})

# Paths never checked: generated output, vendored text, fixtures that must stay verbatim.
EXEMPT_PREFIXES = (
    "internal/store/sqlcgen/",
    "web/packages/sdk/src/gen/",
    "testdata/",
    "docs/",
)
EXEMPT_PARTS = ("/testdata/", "/node_modules/", "/dist/")

HEADER_WINDOW = 5
WARN_LINES = 400
FAIL_LINES = 800


@dataclass(frozen=True)
class Problem:
    path: str
    message: str


def required_licence(path: str) -> str:
    """The SPDX identifier a path's directory requires (D80)."""
    return MPL if path.startswith("web/") else GPL


def is_source(path: str) -> bool:
    if path.startswith(EXEMPT_PREFIXES) or any(part in f"/{path}" for part in EXEMPT_PARTS):
        return False
    p = Path(path)
    return p.suffix in SOURCE_SUFFIXES or p.name in SOURCE_NAMES


def is_generated(text: str) -> bool:
    """Go's convention for generated files, which sqlc and our generators follow."""
    head = "\n".join(text.splitlines()[:HEADER_WINDOW])
    return "Code generated" in head and "DO NOT EDIT" in head


def check_header(path: str, text: str) -> Problem | None:
    want = required_licence(path)
    for line in text.splitlines()[:HEADER_WINDOW]:
        if "SPDX-License-Identifier:" in line:
            got = line.split("SPDX-License-Identifier:", 1)[1].strip().rstrip("*/-> ").strip()
            if got == want:
                return None
            return Problem(path, f"says SPDX {got!r}, but its directory requires {want!r} (D80)")
    return Problem(
        path, f"no `SPDX-License-Identifier: {want}` in its first {HEADER_WINDOW} lines (D80)"
    )


def load_size_allowlist(text: str) -> dict[str, str]:
    """`path  # reason` per line; a line without a reason is itself a problem."""
    allowed: dict[str, str] = {}
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        path, _, reason = line.partition("#")
        allowed[path.strip()] = reason.strip()
    return allowed


def check_size(path: str, text: str, allowed: dict[str, str]) -> tuple[Problem | None, str | None]:
    """Return (failure, warning) for one file's length."""
    lines = text.count("\n") + (0 if text.endswith("\n") or not text else 1)
    if lines <= WARN_LINES:
        return None, None
    if path in allowed:
        if not allowed[path]:
            return Problem(path, "is in scripts/filesize.allow without a reason"), None
        return None, None
    if lines > FAIL_LINES:
        return (
            Problem(
                path,
                f"is {lines} lines (limit {FAIL_LINES}). Split it by concern, or add it to "
                "scripts/filesize.allow with the reason it must stay whole.",
            ),
            None,
        )
    return None, f"{path}: {lines} lines (over {WARN_LINES}; consider splitting)"


def tracked_files(root: Path) -> list[str]:
    out = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard"],
        cwd=root, capture_output=True, text=True, check=True,
    ).stdout
    return sorted({line for line in out.splitlines() if line})


def check(root: Path, paths: list[str]) -> tuple[list[Problem], list[str]]:
    allow_file = root / "scripts" / "filesize.allow"
    allowed = load_size_allowlist(allow_file.read_text()) if allow_file.exists() else {}
    problems: list[Problem] = []
    warnings: list[str] = []
    for path in paths:
        if not is_source(path):
            continue
        full = root / path
        if not full.is_file():
            continue  # deleted in the working tree but still in the index
        text = full.read_text(errors="replace")
        if is_generated(text):
            continue
        header = check_header(path, text)
        if header:
            problems.append(header)
        failure, warning = check_size(path, text, allowed)
        if failure:
            problems.append(failure)
        if warning:
            warnings.append(warning)
    return problems, warnings


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--root", default=".")
    args = ap.parse_args()
    root = Path(args.root).resolve()
    paths = tracked_files(root)
    problems, warnings = check(root, paths)
    for w in warnings:
        print(f"warning: {w}")
    if problems:
        for p in problems:
            print(f"{p.path}: {p.message}")
        print(f"check_source: {len(problems)} problem(s)")
        return 1
    print(f"check_source: {sum(is_source(p) for p in paths)} source files, 0 problems")
    return 0


if __name__ == "__main__":
    sys.exit(main())
