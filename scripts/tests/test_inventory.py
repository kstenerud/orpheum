# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for inventory.py: every tier runs to its end, one list names each failure by its file and
# ABOUTME: first failing line, and a failure turns the background probe off for the sweep.
import re
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import background_probe  # noqa: E402
import inventory  # noqa: E402
from gitrepo import Repo  # noqa: E402

REPO = Path(__file__).resolve().parent.parent.parent

# Each as the tool prints it, from the logs of the fixes after the 11-23 design cycle.
VITEST = """\
     × reaches every control over the film by arrows 8638ms
 FAIL  |chromium| apps/web/src/input-player.browser.test.tsx > the player by a remote's keys > reaches every control
AssertionError: expected [ <div> ] to have a length of 2 but got 1
 ❯ Proxy.<anonymous> node_modules/.vite/vitest/deps/index.js?v=f54b4757:2635:20
 ❯ apps/web/src/input-player.browser.test.tsx:210:19
    210|     expect(menus).toHaveLength(2);
"""
PLAYWRIGHT = """\
  1) [chromium] › web/e2e/shell.spec.ts:11:5 › the shell › draws the banner ──────────

    Error: expect(locator).toBeVisible() failed
    > 13 |         await expect(banner).toBeVisible();
        at /home/karl/orpheum/web/e2e/shell.spec.ts:13:36

  2) [chromium] › web/e2e/shell.spec.ts:11:5 › the shell › draws the banner ──────────
        at /home/karl/orpheum/web/e2e/shell.spec.ts:13:36
"""
GO = """\
--- FAIL: TestQueue_Orders (0.01s)
    --- FAIL: TestQueue_Orders/first (0.00s)
        queue_test.go:45: got 2, want 1
FAIL
FAIL\tgithub.com/x/orpheum/internal/core/queue\t0.123s
"""
PYTHON = """\
======================================================================
FAIL: test_it_runs (test_impact.Scope.test_it_runs)
----------------------------------------------------------------------
Traceback (most recent call last):
  File "/home/karl/orpheum/scripts/tests/test_impact.py", line 88, in test_it_runs
    self.assertEqual(a, b)
"""
LINTS = """\
apps/web/src/title.tsx(12,5): error TS2322: Type 'string' is not assignable to type 'number'.
internal/core/queue/queue.go:30:2: ineffectual assignment to err (ineffassign)
/home/karl/orpheum/web/apps/web/src/title.tsx
  14:3  error  'unused' is assigned a value but never used  @typescript-eslint/no-unused-vars
make[1]: *** [Makefile:200: web-typecheck] Error 2
"""


class FailuresIn(unittest.TestCase):
    """The list each fix starts from names each failure by its file and first failing line (the owner,
    2026-11-24, the test fixes after a design cycle, item 4)."""

    def test_a_vitest_failure_is_named_by_its_test_and_its_tests_own_failing_line(self) -> None:
        self.assertEqual(inventory.failures_in(VITEST), [inventory.Failure(
            "vitest apps/web/src/input-player.browser.test.tsx > the player by a remote's keys > reaches every control",
            "apps/web/src/input-player.browser.test.tsx:210")])

    def test_an_end_to_end_failure_is_named_once_by_its_browser_and_its_failing_line_not_its_declaration(self) -> None:
        self.assertEqual(inventory.failures_in(PLAYWRIGHT), [inventory.Failure(
            "e2e [chromium] the shell › draws the banner", "web/e2e/shell.spec.ts:13")])

    def test_a_go_failure_is_named_by_its_test_and_line_and_its_own_lines_are_no_findings(self) -> None:
        self.assertEqual(inventory.failures_in(GO), [inventory.Failure("go TestQueue_Orders", "queue_test.go:45")])

    def test_a_python_failure_is_named_by_its_test_and_line(self) -> None:
        self.assertEqual(inventory.failures_in(PYTHON), [inventory.Failure(
            "python test_impact.Scope.test_it_runs", "/home/karl/orpheum/scripts/tests/test_impact.py:88")])

    def test_type_checker_linter_and_eslint_findings_are_named_by_their_lines(self) -> None:
        self.assertEqual([failure.where for failure in inventory.failures_in(LINTS)], [
            "apps/web/src/title.tsx:12", "internal/core/queue/queue.go:30",
            "/home/karl/orpheum/web/apps/web/src/title.tsx:14"])
        self.assertEqual(inventory.failed_steps(LINTS), ["web-typecheck"])

    def test_a_passing_run_names_nothing(self) -> None:
        self.assertEqual(inventory.failures_in(" ✓ apps/web/src/title.test.tsx (3 tests) 40ms\nok\tpkg\t0.1s\n"), [])


class Main(unittest.TestCase):
    def setUp(self) -> None:
        self.repo = Repo({"a.txt": "a\n"})
        self.repo.commit("chore: base")
        self.ran: list[str] = []

    def inventory(self, failing: set[str]) -> int:
        def run_tier(name, command, log):  # type: ignore[no-untyped-def]
            self.ran.append(name)
            text = VITEST if name in failing else "ok\n"
            log.write_text(text)
            return (1 if name in failing else 0), text
        with self.repo.cwd(), mock.patch.object(inventory, "run_tier", run_tier), mock.patch("builtins.print"):
            return inventory.main(["--base", "abc", "--quick", "lint-commits"])

    def sweep_mark(self) -> Path:
        with self.repo.cwd():
            here = background_probe.folder()
        assert here is not None
        return here / background_probe.SWEEP_OFF

    def test_every_tier_runs_after_one_fails_and_the_list_names_its_failure(self) -> None:
        self.assertEqual(self.inventory({"make-check"}), 1)
        self.assertEqual(self.ran, [name for name, _ in inventory.tiers("abc", 8, [])], "none stopped early")
        listing = (self.repo.root / inventory.OUT / "failures.md").read_text()
        self.assertIn("## make-check (exit 1", listing)
        self.assertIn("- apps/web/src/input-player.browser.test.tsx:210: vitest", listing)
        self.assertNotIn("## browser-mode", listing)
        self.assertTrue(self.sweep_mark().exists(), "the sweep's commits are not probed")

    def test_a_clean_inventory_passes_and_leaves_the_probe_on(self) -> None:
        self.assertEqual(self.inventory(set()), 0)
        self.assertIn("Every tier passed", (self.repo.root / inventory.OUT / "failures.md").read_text())
        self.assertFalse(self.sweep_mark().exists())

    def test_the_stamps_make_check_trusts_are_cleared_first(self) -> None:
        with self.repo.cwd():
            stamps = background_probe.folder().parent / "orpheum-area-stamps"  # type: ignore[union-attr]
        stamps.mkdir(parents=True)
        (stamps / "web-test").write_text("tree\n")
        self.inventory(set())
        self.assertFalse(stamps.exists())

    def test_the_tiers_run_every_test_and_no_revert_red(self) -> None:
        commands = {name: " ".join(command) for name, command in inventory.tiers("abc", 4, ["lint-commits"])}
        self.assertIn("-k", commands["make-check"].split(), "every step of make check runs")
        self.assertIn("BASE=abc lint-commits", commands["check-pr-quick"])
        self.assertIn("test-browser BASE=", commands["browser-tier"], "the whole tier, not a reach")
        self.assertFalse([name for name, command in commands.items() if "revert" in command])


class Wiring(unittest.TestCase):
    def test_make_inventory_holds_the_lock_as_check_pr_and_check_pr_turns_the_probe_on_again(self) -> None:
        makefile = (REPO / "Makefile").read_text()
        target = re.search(r"(?ms)^inventory:.*?\n\n", makefile)
        assert target is not None
        self.assertIn("$(GATE_LOCK) --as check-pr $(PYTHON) scripts/inventory.py", target.group(0))
        passed = re.search(r"(?m)^.*check_pr_debt.py paid.*\n.*\n", makefile)
        assert passed is not None
        self.assertIn(f"revert-red/{background_probe.SWEEP_OFF}", passed.group(0))


if __name__ == "__main__":
    unittest.main()
