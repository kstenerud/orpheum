# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Revert-red's record of the files its last run failed on (GREEN, NO_RUNNER), and the
# ABOUTME: base and head it ran on, which `check_revert_red.py --rerun-failed` probes again.
"""The files revert-red's last run failed on.

Every run records, at `<git-common-dir>/revert-red/last-failures`, the base and head it ran on
and the paths whose outcome was a failure. A targeted run (`--only`, `--rerun-failed`) against
the same base replaces only the rows of the paths it probed, so fixing one file and re-running it
does not forget the others. A run marks the record unfinished when it starts and finished when
it has judged every file: a run that stopped part way (a timeout, a tree it could not prepare)
leaves a record `--rerun-failed` refuses, since its list would say less than it seems to.

The head is recorded but not required to match: the fixes a re-run checks are new commits. The
base must match, because the same path judged against another base is a different question.
"""

from __future__ import annotations

from typing import Any

import last_run

GATE = "revert-red"
NAME = "last-failures"


def begin(base: str) -> None:
    """Marks the record unfinished for a run against `base`, keeping its paths if the base is the
    same (a targeted run that finishes merges into them)."""
    def change(old: dict[str, Any] | None) -> dict[str, Any]:
        same = old is not None and old.get("base") == base
        return {"base": base, "head": old.get("head", "") if same else "",
                "paths": old.get("paths", []) if same else [], "finished": False}
    last_run.update(GATE, NAME, change)


def finish(base: str, head: str, probed: list[str], failed: list[str], targeted: bool) -> None:
    """Records a run that judged every file it set out to: a full run's failures replace the list;
    a targeted run's replace the rows of the paths it probed."""
    def change(old: dict[str, Any] | None) -> dict[str, Any]:
        paths = set(failed)
        if targeted and old is not None and old.get("base") == base:
            paths |= set(old.get("paths", [])) - set(probed)
        return {"base": base, "head": head, "paths": sorted(paths), "finished": True}
    last_run.update(GATE, NAME, change)


def recorded_failures(base: str) -> tuple[list[str], str, str]:
    """The paths the last run against `base` failed on and the head it ran on, or a problem that
    says why there is no list to re-run."""
    record = last_run.read(GATE, NAME)
    where = last_run.record_path(GATE, NAME)
    if record is None:
        return [], "", f"No revert-red run is recorded ({where}); run the full check first."
    if record.get("base") != base:
        return [], "", (f"The last revert-red run was against base {str(record.get('base'))[:12]}, not "
                        f"{base[:12]}; its failures answer a different question. Run the full check "
                        "against this base first.")
    if not record.get("finished"):
        return [], "", ("The last revert-red run against this base did not finish, so its list of "
                        "failures is incomplete. Run the full check first.")
    return list(record.get("paths", [])), str(record.get("head", "")), ""
