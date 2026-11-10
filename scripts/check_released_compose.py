#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Release gate for D462 §6: each compose-file version is frozen in
# ABOUTME: cmd/orpheum/testdata/released/compose-v<n>/, so a test holds every later start to it.
"""Check that the published compose files are their version's frozen copy (D462 §6, amends D461 §1).

A household pastes a compose file once and keeps it; every later release must still start from it
(D455). `TestReleasedCompose_EveryReleasedFileStillStartsThisServer` holds `orpheum start` to each
file frozen under `cmd/orpheum/testdata/released/compose-v<n>/`, one folder for each version of the
files, `deployment.ComposeFileVersion`. Files are compared without their comment lines, so a
comment's wording needs no new version (D462 §10.8).

* **Run by check-source** in make check and check-pr, and by the release workflow with `--tag` on the
  tag's checkout before anything is published: the current version must be frozen, and each published
  file must be its frozen copy. A file changed without a bump fails here, naming the steps.
* **Without `--tag`** it also checks every release tag this clone has that carried a compose-v folder:
  the version it published must be frozen as it published it, so a frozen folder is never changed.
* **`--freeze`**, run by `make freeze-compose`, writes the current version's folder, and refuses to
  overwrite one frozen with other contents: households may have that version, so change it by bumping.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

RELEASED = Path("cmd/orpheum/testdata/released")
VERSION_SOURCE = Path("internal/deployment/values.go")

# Each compose file a household is sent to paste, by its route (internal/deployment/household.go),
# and its name in the frozen folder.
PUBLISHED = {
    "deploy/omv/compose.yaml": "omv.yaml",
    "deploy/synology/compose.yaml": "synology.yaml",
    "deploy/portainer/compose.yaml": "portainer.yaml",
    "deploy/catalogue/casaos/docker-compose.yml": "casaos.yaml",
    "deploy/docker/compose.yaml": "docker.yaml",
}

_TAG = re.compile(r"^v(\d+)\.(\d+)\.(\d+)$")
_VERSION = re.compile(r"(?m)^const ComposeFileVersion = (\d+)$")
_BUMP = ("bump deployment.ComposeFileVersion, say on the System page what the new file brings, run "
         "make generate and make freeze-compose (D462 §6)")


def version(tag: str) -> tuple[int, int, int] | None:
    """A release tag's numbers, or None for a name that is not vX.Y.Z."""
    match = _TAG.match(tag)
    if match is None:
        return None
    return int(match[1]), int(match[2]), int(match[3])


def compose_version(source: str) -> int:
    """The compose-file version values.go names."""
    match = _VERSION.search(source)
    if match is None:
        raise ValueError(f"{VERSION_SOURCE} names no `const ComposeFileVersion = <n>`")
    return int(match[1])


def without_comments(text: str) -> str:
    """A compose file's text with its comment lines left out, as the gate compares it."""
    return "\n".join(line for line in text.splitlines() if not line.lstrip().startswith("#"))


def frozen_folder(number: int) -> Path:
    return RELEASED / f"compose-v{number}"


def check_tree(root: Path) -> list[str]:
    """Problems with the tree's published files against the current version's frozen folder."""
    number = compose_version((root / VERSION_SOURCE).read_text())
    problems = [f"{frozen_folder(earlier)}/ is missing: every version households were given stays frozen"
                for earlier in range(1, number) if not (root / frozen_folder(earlier)).is_dir()]
    folder = frozen_folder(number)
    if not (root / folder).is_dir():
        return [*problems, f"{folder}/ is missing: run make freeze-compose (D462 §6)"]
    for published, frozen in PUBLISHED.items():
        source, target = root / published, root / folder / frozen
        if not source.is_file():
            problems.append(f"{published} is missing; make generate writes it")
        elif not target.is_file() or without_comments(target.read_text()) != without_comments(source.read_text()):
            problems.append(f"{published} is not {folder / frozen}, compose-file version {number} as it was "
                            f"frozen: {_BUMP}")
    return problems


def _git(root: Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], cwd=root, capture_output=True, text=True, check=False)


def check_tags(root: Path) -> list[str]:
    """Problems with the frozen folder of each version a release tag this clone has published."""
    problems = []
    for tag in sorted((t for t in _git(root, "tag", "--list", "v*").stdout.split() if version(t)),
                      key=lambda t: version(t) or (0, 0, 0)):
        source = _git(root, "show", f"{tag}:{VERSION_SOURCE}")
        carried = _git(root, "ls-tree", "-d", "--name-only", f"{tag}:{RELEASED}").stdout.split()
        if source.returncode != 0 or not any(name.startswith("compose-v") for name in carried):
            continue  # before D462 §6: no household had pasted a file (D455)
        try:
            number = compose_version(source.stdout)
        except ValueError:
            continue
        folder = frozen_folder(number)
        for published, frozen in PUBLISHED.items():
            shown = _git(root, "show", f"{tag}:{published}")
            if shown.returncode != 0:
                continue
            target = root / folder / frozen
            if not target.is_file() or without_comments(target.read_text()) != without_comments(shown.stdout):
                problems.append(f"{folder / frozen} is not {published} as {tag} published it: a frozen "
                                "version is never changed; " + _BUMP)
    return problems


def freeze(root: Path) -> list[str]:
    """Write the current version's frozen folder, refusing to change one already frozen."""
    number = compose_version((root / VERSION_SOURCE).read_text())
    folder = root / frozen_folder(number)
    changed = [frozen for published, frozen in PUBLISHED.items() if (folder / frozen).is_file()
               and without_comments((folder / frozen).read_text()) != without_comments((root / published).read_text())]
    if changed:
        return [f"{frozen_folder(number)} is frozen already with other files ({', '.join(changed)}); households "
                f"may have them: {_BUMP}"]
    folder.mkdir(parents=True, exist_ok=True)
    for published, frozen in PUBLISHED.items():
        (folder / frozen).write_text((root / published).read_text())
    return []


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--tag", help="the release being published, checked in this tree")
    parser.add_argument("--freeze", action="store_true", help="freeze the current version's files")
    parser.add_argument("--root", type=Path, default=Path("."), help=argparse.SUPPRESS)
    arguments = parser.parse_args(argv)
    if arguments.tag is not None and version(arguments.tag) is None:
        print(f"check_released_compose: {arguments.tag} is not a release tag, vX.Y.Z", file=sys.stderr)
        return 2
    if arguments.freeze:
        problems = freeze(arguments.root)
    else:
        problems = check_tree(arguments.root)
        if arguments.tag is None:
            problems += check_tags(arguments.root)
    for problem in problems:
        print(problem, file=sys.stderr)
    if problems:
        print(f"check_released_compose: {len(problems)} problem(s)", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
