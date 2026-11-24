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


if __name__ == "__main__":
    unittest.main()
