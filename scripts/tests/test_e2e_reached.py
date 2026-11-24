# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for e2e_reached.py: make check runs the end-to-end specs a change to the web client reaches
# ABOUTME: since they last passed, both passes, and nothing for a change the browser tier does not read.
import json
import re
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import check_stamp  # noqa: E402
import e2e_reached  # noqa: E402
from gitrepo import Repo  # noqa: E402

REPO = Path(__file__).resolve().parent.parent.parent
CLIENT = "web/apps/web/src/title.tsx"


class E2EReached(unittest.TestCase):
    """Ten specs a design cycle left stale were first seen in check-pr's browser tier (the owner,
    2026-11-24)."""

    def setUp(self) -> None:
        self.repo = Repo({CLIENT: "export const a = 1;\n", "web/apps/web/src/title.test.tsx": "a\n",
                          "web/e2e/shell.spec.ts": "a\n", "internal/a.go": "package a\n", "docs/a.md": "a\n"})
        self.repo.commit("chore: base")
        self.asked: list[dict] = []
        self.ran: list[list[str] | None] = []

    def gate(self, reached: list[str] | None = None, status: int = 0) -> int:
        def scope_of(**given):  # type: ignore[no-untyped-def]
            self.asked.append(given)
            return reached, "by the map"

        def run_tests(tests):  # type: ignore[no-untyped-def]
            self.ran.append(tests)
            return status
        with self.repo.cwd(), mock.patch("builtins.print"):
            return e2e_reached.main(scope_of=scope_of, run_tests=run_tests)

    def test_before_any_pass_it_asks_from_the_maps_commit_and_a_pass_is_recorded_a_failure_not(self) -> None:
        self.assertEqual(self.gate(status=1), 1)
        self.assertEqual(self.asked, [{}], "every change since the map's commit")
        self.assertEqual(self.ran, [None])
        self.assertEqual(self.gate(), 0)
        self.assertEqual(self.asked, [{}, {}], "the failure recorded no pass")
        self.assertEqual(self.gate(), 0)
        self.assertEqual(len(self.asked), 2, "nothing changed since the pass: not asked, not run")
        self.assertEqual(len(self.ran), 2)

    def test_a_change_the_browser_tier_does_not_read_runs_nothing(self) -> None:
        self.gate()
        self.repo.write({"internal/a.go": "package b\n", "docs/a.md": "b\n", "web/apps/web/src/title.test.tsx": "b\n",
                         "web/e2e/acceptance.spec.ts": "b\n", "web/README.md": "b\n"})
        self.assertEqual(self.gate(), 0)
        self.assertEqual(len(self.asked), 1)
        self.assertEqual(len(self.ran), 1)

    def test_a_client_change_since_the_pass_is_asked_of_the_map_from_the_passed_tree(self) -> None:
        with self.repo.cwd():
            passed = check_stamp.working_tree_id()
        self.gate()
        self.repo.write({CLIENT: "export const a = 2;\n", "internal/a.go": "package b\n"})
        self.assertEqual(self.gate(reached=["chromium title", "shell.spec.ts"]), 0)
        self.assertEqual(self.asked[1], {"paths": [CLIENT], "since": passed})
        self.assertEqual(self.ran[1], ["chromium title", "shell.spec.ts"])

    def test_a_change_no_test_loaded_runs_none_and_passes(self) -> None:
        self.gate()
        self.repo.write({CLIENT: "export const a = 2;\n"})
        self.assertEqual(self.gate(reached=[]), 0)
        self.assertEqual(len(self.ran), 1)
        self.assertEqual(self.gate(), 0)
        self.assertEqual(len(self.asked), 2, "recorded as passed")

    def test_the_run_is_the_browser_tiers_both_passes_narrowed_to_the_reached_tests(self) -> None:
        ran = []

        def run(command, env, check):  # type: ignore[no-untyped-def]
            ran.append((command[-1], env.get("ORPHEUM_E2E_ONLY"), "ORPHEUM_BROWSER_MAP_DIR" in env))
            return mock.Mock(returncode=0)
        with mock.patch.object(e2e_reached.subprocess, "run", run), \
                mock.patch.dict("os.environ", {"ORPHEUM_BROWSER_MAP_DIR": "/m"}):
            self.assertEqual(e2e_reached.run(["chromium title"]), 0)
            self.assertEqual(e2e_reached.run(None), 0)
        self.assertEqual(ran, [("browsers", json.dumps(["chromium title"]), False),
                               ("browser-tier", json.dumps(["chromium title"]), False),
                               ("browsers", None, False), ("browser-tier", None, False)])


class Wiring(unittest.TestCase):
    def test_make_check_runs_it_after_the_browser_mode_tests(self) -> None:
        makefile = (REPO / "Makefile").read_text()
        check = re.search(r"(?ms)^check-verbose:.*?\n\n", makefile)
        assert check is not None
        self.assertRegex(check.group(0), r"web-test-browser\n(?:.*\n)*?.*test-e2e-reached\n(?:.*\n)*?.*check_stamp.py write")
        self.assertRegex(makefile, r"(?m)^test-e2e-reached:.*\n\t@\$\(PYTHON\) scripts/e2e_reached.py$")
        tier = re.search(r"(?ms)^browser-tier:.*?\n\n", makefile)
        assert tier is not None
        self.assertIn("ORPHEUM_E2E_TIMING=1 $(BROWSER_PASS)", tier.group(0), "both passes")


if __name__ == "__main__":
    unittest.main()
