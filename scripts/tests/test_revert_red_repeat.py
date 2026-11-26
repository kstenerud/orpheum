# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_repeat.py: a red from browser-mode tests stands only when they fail again
# ABOUTME: with the file still reverted, so a test that failed under load passes no stylesheet.
import subprocess
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import revert_red_repeat as repeat  # noqa: E402
import revert_red_stylesheets as sheets  # noqa: E402
from revert_red_timings import Timings  # noqa: E402

FIREFOX_FAILED = (" FAIL   firefox  apps/web/src/title.browser.test.tsx > the title > fades in\n"
                  "Error: Test timed out in 30000ms.\n")
UNIT_FAILED = " FAIL  apps/web/src/title.test.ts > the title > is named\n"


def done(code: int, output: str = "") -> subprocess.CompletedProcess[str]:
    return subprocess.CompletedProcess([], code, output, "")


class Repeats(unittest.TestCase):
    def test_runs_the_failed_browser_files_alone_in_the_browser_configuration(self):
        asked: list[list[str]] = []
        self.assertTrue(repeat.repeats(FIREFOX_FAILED, lambda command: asked.append(command) or done(1)))
        self.assertEqual(len(asked), 1)
        self.assertIn("apps/web/src/title.browser.test.tsx", asked[0])
        self.assertIn("vitest.browser.config.ts", asked[0])
        self.assertIn("run", asked[0])

    def test_a_red_that_passes_when_run_again_does_not_repeat(self):
        self.assertFalse(repeat.repeats(FIREFOX_FAILED, lambda _command: done(0)))

    def test_a_red_naming_no_browser_test_is_not_run_again(self):
        asked: list[list[str]] = []
        self.assertIsNone(repeat.repeats(UNIT_FAILED, lambda command: asked.append(command) or done(1)))
        self.assertEqual(asked, [])

    def test_a_repeat_past_its_timeout_lets_no_red_stand(self):
        def hangs(*_args, **_kwargs):
            raise subprocess.TimeoutExpired("vitest", 1)
        with mock.patch.object(repeat, "run_whole_group", hangs), mock.patch("builtins.print"):
            self.assertTrue(repeat.did_not_repeat(Timings(), "web/apps/web/src/title.css", ["vitest", "related"],
                                                   FIREFOX_FAILED, "/t", 1))


class AStylesheetsOwnBrowserTests(unittest.TestCase):
    """check-pr's second run took title.css's revert as red from a test that failed beside it, and
    the third found nothing testing it (2026-11-24)."""

    def noticed(self, again: int, first: str = FIREFOX_FAILED) -> tuple[dict[str, str], list[list[str]]]:
        repeated: list[list[str]] = []

        def run_again(command, *_args, **_kwargs):
            repeated.append(command)
            return done(again)
        with mock.patch.object(sheets, "git_run", lambda *_a, **_k: done(0)), \
                mock.patch.object(sheets, "restore", lambda *_a, **_k: None), \
                mock.patch.object(sheets, "run_whole_group", lambda *_a, **_k: done(1, first)), \
                mock.patch.object(repeat, "run_whole_group", run_again), mock.patch("builtins.print"):
            found = sheets.own_browser_mode_reds("/t", "base", "head", ["web/apps/web/src/title.css"],
                                                 ["vitest", "related"], 60, Timings())
        return found, repeated

    def test_is_red_when_its_failed_tests_fail_again_with_it_reverted(self):
        found, repeated = self.noticed(again=1)
        self.assertEqual(list(found), ["web/apps/web/src/title.css"])
        self.assertEqual(len(repeated), 1)

    def test_is_not_red_when_its_failed_tests_pass_run_again(self):
        found, repeated = self.noticed(again=0)
        self.assertEqual(found, {}, "a test that failed under load would pass the sheet with nothing testing it")
        self.assertEqual(len(repeated), 1)

    def test_a_red_naming_no_browser_test_stands_as_before(self):
        found, repeated = self.noticed(again=0, first=UNIT_FAILED)
        self.assertEqual(list(found), ["web/apps/web/src/title.css"])
        self.assertEqual(repeated, [])


class TheSheetsTogether(unittest.TestCase):
    """The sheets go into one browser-mode run, every one reverted: seventeen runs of a sheet each were
    5795 s of one check-pr, and all passed (2026-11-24). A red still stands only on a run with its
    sheet alone reverted."""

    SHEETS = ["web/apps/web/src/a.css", "web/apps/web/src/b.css", "web/apps/web/src/c.css"]

    def runs(self, red_with: str | None, names_a_test: bool = True,
             together_stops_at_a_passing_test: bool = False) -> tuple[dict[str, str], list[tuple]]:
        """What own_browser_mode_reds found, and each run it made: its vitest mode, the sheets it named
        and the sheets reverted as it ran. A run fails while red_with is reverted; with
        together_stops_at_a_passing_test, the run of every sheet fails on a test that passes run again,
        before it reaches red_with's."""
        reverted: set[str] = set()
        ran: list[tuple] = []
        failed = FIREFOX_FAILED if names_a_test else UNIT_FAILED

        def checkout(*args, **_kwargs):
            reverted.add(args[-1])
            return done(0)

        def run(command, *_args, **_kwargs):
            mode = "run" if "run" in command and "related" not in command else "related"
            named = tuple(sorted(part for part in command if part.endswith(".css")))
            ran.append((mode, named, tuple(sorted(reverted))))
            if together_stops_at_a_passing_test and len(reverted) == len(self.SHEETS):
                return done(1, failed) if mode == "related" else done(0)
            return done(1, failed) if red_with in reverted else done(0)
        with mock.patch.object(sheets, "git_run", checkout), \
                mock.patch.object(sheets, "restore", lambda _tree, _head, path: reverted.discard(path)), \
                mock.patch.object(sheets, "run_whole_group", run), mock.patch.object(repeat, "run_whole_group", run), \
                mock.patch("builtins.print"):
            found = sheets.own_browser_mode_reds("/t", "base", "head", self.SHEETS, ["vitest", "related"], 60,
                                                 Timings())
        return found, ran

    def test_one_run_settles_them_all_when_it_passes(self):
        found, ran = self.runs(red_with=None)
        self.assertEqual(found, {})
        every = tuple(sorted(path.removeprefix("web/") for path in self.SHEETS))
        self.assertEqual(ran, [("related", every, tuple(self.SHEETS))])

    def test_a_red_is_put_on_the_sheet_whose_revert_alone_fails_the_tests_that_failed(self):
        found, ran = self.runs(red_with="web/apps/web/src/b.css")
        self.assertEqual(list(found), ["web/apps/web/src/b.css"])
        # Together, repeated; then each sheet alone runs the failed test only, b's red repeated; then
        # the other two together, which pass.
        alone = [(mode, reverted) for mode, _named, reverted in ran if mode == "run" and len(reverted) == 1]
        self.assertEqual(sorted(reverted for _mode, reverted in alone),
                         [("web/apps/web/src/a.css",), ("web/apps/web/src/b.css",), ("web/apps/web/src/b.css",),
                          ("web/apps/web/src/c.css",)])
        self.assertEqual(ran[-1], ("related", ("apps/web/src/a.css", "apps/web/src/c.css"),
                                   ("web/apps/web/src/a.css", "web/apps/web/src/c.css")))
        self.assertTrue(all(len(reverted) == 1 for mode, _named, reverted in ran if mode == "run"
                            and reverted != tuple(self.SHEETS)),
                        "a red stands only on a run with its sheet alone reverted")

    def test_a_run_together_that_stopped_at_a_test_that_then_passed_settles_nothing(self):
        # The run stops at its first failure: the tests after it never ran, so it is no pass, and each
        # sheet takes its own run (check-pr called cast.css green on revert so, 2026-11-26).
        found, ran = self.runs(red_with="web/apps/web/src/b.css", together_stops_at_a_passing_test=True)
        self.assertEqual(list(found), ["web/apps/web/src/b.css"])
        own = [(named, reverted) for mode, named, reverted in ran if mode == "related" and len(reverted) == 1]
        self.assertEqual(own, [((path.removeprefix("web/"),), (path,)) for path in self.SHEETS])

    def test_a_red_naming_no_browser_test_sends_each_sheet_to_its_own_run(self):
        found, ran = self.runs(red_with="web/apps/web/src/c.css", names_a_test=False)
        self.assertEqual(list(found), ["web/apps/web/src/c.css"])
        self.assertEqual([(named, reverted) for _mode, named, reverted in ran[1:]],
                         [((path.removeprefix("web/"),), (path,)) for path in self.SHEETS])


if __name__ == "__main__":
    unittest.main()
