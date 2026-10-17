# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Revert-red's record of the files its last run failed on (GREEN, NO_RUNNER, TIMED_OUT), and the
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


# The code part's record of the files whose cheap runs all passed, owing the browser part their
# browser runs (check_revert_red.py's BROWSER_OWED).
OWED_BROWSER = "owed-browser"


def owe_browser(base: str, probed: list[str], owing: list[str]) -> None:
    """Records which of the code part's probed files owe the browser part their browser runs: the
    rows of the paths it probed replaced, so a targeted re-run keeps the others'."""
    def change(old: dict[str, Any] | None) -> dict[str, Any]:
        kept = set(old.get("paths", [])) if old is not None and old.get("base") == base else set()
        return {"base": base, "paths": sorted((kept - set(probed)) | set(owing))}
    last_run.update(GATE, OWED_BROWSER, change)


def owed_browser(base: str) -> frozenset[str]:
    """The files the code part's last run against base left owing their browser runs."""
    record = last_run.read(GATE, OWED_BROWSER)
    if record is None or record.get("base") != base:
        return frozenset()
    return frozenset(record.get("paths", []))
