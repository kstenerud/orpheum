#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: make go-test-lock: points the user-wide GOFLAGS' -exec at the main tree's go_test_exec,
# ABOUTME: whatever worktree runs it, replacing an older one in place of adding a second.
"""Points `go test`, typed anywhere, at the repository's test lock (scripts/go_test_exec).

Go reads no repository setting, so `make setup` writes `-exec=<tree>/scripts/go_test_exec` into the
user-wide GOFLAGS (`go env -w`). An agent's worktree running it appended its own path; the last
`-exec` wins, so once the worktree was removed every `go test` failed with "fork/exec …: no such
file or directory" (2026-11-09). This writes the main tree's path, which outlives every worktree,
and drops any earlier `-exec` naming a go_test_exec, so there is only ever one.
"""

import os
import subprocess
import sys
from pathlib import Path

EXEC_SCRIPT = "scripts/go_test_exec"


def main_tree(here: Path) -> Path:
    """The main working tree of the repository holding here: the folder its common git directory is
    in. A linked worktree's common directory is the main tree's `.git`."""
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], cwd=here,
                            capture_output=True, text=True, check=True).stdout.strip()
    return Path(common).parent


def flags_with_lock(current: str, tree: Path) -> str:
    """current's flags with every -exec naming a go_test_exec dropped, and tree's own last."""
    ours = f"-exec={tree / EXEC_SCRIPT}"
    kept = [flag for flag in current.split() if not (flag.startswith("-exec=") and flag.endswith("/go_test_exec"))]
    return " ".join([*kept, ours])


def main() -> int:
    tree = main_tree(Path.cwd())
    env = {name: value for name, value in os.environ.items() if name != "GOFLAGS"}
    current = subprocess.run(["go", "env", "GOFLAGS"], env=env, capture_output=True, text=True,
                             check=True).stdout.strip()
    wanted = flags_with_lock(current, tree)
    if wanted != current:
        subprocess.run(["go", "env", "-w", f"GOFLAGS={wanted}"], env=env, check=True)
        print(f"go-test-lock: -exec={tree / EXEC_SCRIPT}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
