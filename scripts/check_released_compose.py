#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Release gate for D461 §1: every released compose file is frozen at its tag in
# ABOUTME: cmd/orpheum/testdata/released/<tag>/, so a test holds every later start to it.
"""Check that each release's compose files are frozen as the test fixtures D461 §1 names.

A household pastes a compose file once and keeps it; every later release must still start from it
(D455, D461). `TestReleasedCompose_EveryReleasedFileStillStartsThisServer` holds `orpheum start` to
each file frozen under `cmd/orpheum/testdata/released/<tag>/`, but only to the files someone froze:
a tag released without its folder leaves its households unguarded, silently. This makes the folder
part of the release, not a step to remember (principles §17):

* **With `--tag <tag>`**, run by the release workflow on the tag's own checkout before anything is
  published: the folder must be there, each published compose file must be frozen in it as it is in
  the tree, and its `start-flags.txt` must name exactly the flags this tree's `start` takes.
* **Without it**, run by check-pr: every tag from the first frozen one on that this clone has must
  have its folder, each published file frozen as the tag has it. A sandbox fetches no tags, so it
  checks the ones it has.

Tags before `FIRST_FROZEN` came before the first beta install, when no household had pasted a file
and releases were free to break them (D455).
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

RELEASED = Path("cmd/orpheum/testdata/released")
START_SOURCE = Path("cmd/orpheum/start.go")
START_FLAGS = "start-flags.txt"
FIRST_FROZEN = "v0.3.3"

# Each compose file a household is sent to paste, and its name in the frozen folder.
PUBLISHED = {
    "deploy/omv/compose.yaml": "omv.yaml",
    "deploy/synology/compose.yaml": "synology.yaml",
}

_TAG = re.compile(r"^v(\d+)\.(\d+)\.(\d+)$")
_FLAG = re.compile(r'flags\.\w+\((?:&options\.\w+, )?"([a-z][a-z-]*)"')


def version(tag: str) -> tuple[int, int, int] | None:
    """A release tag's numbers, or None for a name that is not vX.Y.Z."""
    match = _TAG.match(tag)
    if match is None:
        return None
    return int(match[1]), int(match[2]), int(match[3])


def frozen_tags(tags: list[str]) -> list[str]:
    """The release tags from FIRST_FROZEN on, oldest first."""
    first = version(FIRST_FROZEN)
    kept = [tag for tag in tags if (number := version(tag)) is not None and first is not None
            and number >= first]
    return sorted(kept, key=lambda tag: version(tag) or (0, 0, 0))


def start_flags(source: str) -> list[str]:
    """The flags start.go's parseStart defines."""
    return _FLAG.findall(source)


def check_tree(root: Path, tag: str) -> list[str]:
    """Problems with the tag's frozen folder, checked against the tree checked out at the tag."""
    folder = root / RELEASED / tag
    if not folder.is_dir():
        return [f"{RELEASED / tag}/ is missing: freeze the release's compose files there before "
                f"tagging (D461 §1; cmd/orpheum/doc.go)"]
    problems = []
    for published, frozen in PUBLISHED.items():
        source = root / published
        if not source.is_file():
            continue
        target = folder / frozen
        if not target.is_file() or target.read_text() != source.read_text():
            problems.append(f"{RELEASED / tag / frozen} is not {published} as {tag} publishes it")
    flags_file = folder / START_FLAGS
    takes = sorted(start_flags((root / START_SOURCE).read_text()))
    named = sorted(flags_file.read_text().split()) if flags_file.is_file() else []
    if named != takes:
        problems.append(f"{RELEASED / tag / START_FLAGS} names {named}; {tag}'s start takes {takes}, "
                        "one a line")
    return problems


def _git(root: Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], cwd=root, capture_output=True, text=True, check=False)


def check_tags(root: Path) -> list[str]:
    """Problems with the frozen folder of each release tag this clone has, from FIRST_FROZEN on."""
    tags = _git(root, "tag", "--list", "v*").stdout.split()
    problems = []
    for tag in frozen_tags(tags):
        folder = root / RELEASED / tag
        if not folder.is_dir():
            problems.append(f"{RELEASED / tag}/ is missing: {tag} was released without its compose "
                            "files frozen (D461 §1); freeze them from the tag")
            continue
        for published, frozen in PUBLISHED.items():
            shown = _git(root, "show", f"{tag}:{published}")
            if shown.returncode != 0:
                continue
            target = folder / frozen
            if not target.is_file() or target.read_text() != shown.stdout:
                problems.append(f"{RELEASED / tag / frozen} is not {published} as {tag} published it")
    return problems


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--tag", help="the release being published, checked in this tree")
    parser.add_argument("--root", type=Path, default=Path("."), help=argparse.SUPPRESS)
    arguments = parser.parse_args(argv)
    if arguments.tag is not None and version(arguments.tag) is None:
        print(f"check_released_compose: {arguments.tag} is not a release tag, vX.Y.Z", file=sys.stderr)
        return 2
    problems = (check_tree(arguments.root, arguments.tag) if arguments.tag is not None
                else check_tags(arguments.root))
    for problem in problems:
        print(problem, file=sys.stderr)
    if problems:
        print(f"check_released_compose: {len(problems)} problem(s)", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
