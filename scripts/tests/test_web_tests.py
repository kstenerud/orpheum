# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for web_tests.py: make check runs only the web tests a change since their last pass
# ABOUTME: reaches, and the whole suite when what every test reads changed or a file was deleted.
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import check_stamp  # noqa: E402
import web_tests  # noqa: E402
from gitrepo import Repo  # noqa: E402


class WebTestsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.repo = Repo({
            "web/package.json": "{}",
            "web/apps/web/src/a.ts": "export const a = 1;",
            "web/apps/web/src/a.test.ts": "import { a } from './a';",
            "web/apps/web/src/b.ts": "export const b = 1;",
            "web/apps/web/src/reads.test.ts": "readFileSync('x');",
            "internal/x.go": "package x",
        })
        self.repo.commit("first")

    def changes_after(self, edit: dict[str, str | None]) -> list[tuple[str, str]]:
        with self.repo.cwd():
            before = check_stamp.working_tree_id()
            for path, text in edit.items():
                if text is None:
                    (self.repo.root / path).unlink()
                else:
                    self.repo.write({path: text})
            return web_tests.changes_since(before, check_stamp.working_tree_id())

    def test_a_source_change_runs_its_related_tests_and_the_tests_that_read_sources(self) -> None:
        changes = self.changes_after({"web/apps/web/src/b.ts": "export const b = 2;", "internal/x.go": "package y"})
        command = web_tests.command_for(changes, str(self.repo.root))
        self.assertEqual(command[:len(web_tests.RELATED)], web_tests.RELATED)
        self.assertEqual(command[len(web_tests.RELATED):], ["apps/web/src/b.ts", "apps/web/src/reads.test.ts"],
                         "the changed file, then the source readers; the Go change is not the web's")

    def test_nothing_under_web_changed_runs_nothing(self) -> None:
        self.assertIsNone(web_tests.command_for(self.changes_after({"internal/x.go": "package y"}),
                                                str(self.repo.root)))

    def test_what_every_test_reads_or_a_deletion_runs_the_whole_suite(self) -> None:
        for edit in ({"web/package.json": '{"x": 1}'}, {"web/apps/web/src/b.ts": None},
                     {"web/vitest.config.ts": "export default {};"}, {"web/tsconfig.base.json": "{}"}):
            self.setUp()
            with self.subTest(edit=edit):
                self.assertEqual(web_tests.command_for(self.changes_after(edit), str(self.repo.root)),
                                 web_tests.WHOLE_SUITE)

    def test_the_first_run_runs_everything_and_a_pass_is_recorded_a_failure_not(self) -> None:
        ran: list[list[str]] = []
        code = {"now": 1}

        def run(command, *args, **kwargs):  # type: ignore[no-untyped-def]
            if command[0] == "git":  # the trees' and diffs' own runs
                return REAL_RUN(command, *args, **kwargs)
            ran.append(command)
            return mock.Mock(returncode=code["now"])

        with self.repo.cwd(), mock.patch.object(web_tests.subprocess, "run", side_effect=run):
            self.assertEqual(web_tests.main([]), 1)
            code["now"] = 0
            self.assertEqual(web_tests.main([]), 0)
            self.assertEqual(web_tests.main([]), 0)  # passed, nothing changed: not run
            self.repo.write({"web/apps/web/src/a.ts": "export const a = 2;"})
            self.assertEqual(web_tests.main([]), 0)
        self.assertEqual(ran[:2], [web_tests.WHOLE_SUITE, web_tests.WHOLE_SUITE], "no pass recorded yet: all")
        self.assertEqual(len(ran), 3)
        self.assertEqual(ran[2][len(web_tests.RELATED)], "apps/web/src/a.ts")

    def test_the_browser_mode_tests_run_their_related_ones_in_their_own_configuration(self) -> None:
        changes = self.changes_after({"web/apps/web/src/b.ts": "export const b = 2;"})
        command = web_tests.command_for(changes, str(self.repo.root), web_tests.BROWSER)
        self.assertEqual(command[:len(web_tests.BROWSER.related)], web_tests.BROWSER.related)
        self.assertIn("vitest.browser.config.ts", command)
        self.setUp()
        edit = {"web/vitest.browser.config.ts": "export default {};"}
        self.assertEqual(web_tests.command_for(self.changes_after(edit), str(self.repo.root), web_tests.BROWSER),
                         web_tests.BROWSER.whole)

    def test_due_says_whether_a_run_is_owed_and_runs_nothing(self) -> None:
        ran: list[list[str]] = []

        def run(command, *args, **kwargs):  # type: ignore[no-untyped-def]
            if command[0] == "git":
                return REAL_RUN(command, *args, **kwargs)
            ran.append(command)
            return mock.Mock(returncode=0)

        with self.repo.cwd(), mock.patch.object(web_tests.subprocess, "run", side_effect=run):
            self.assertEqual(web_tests.main(["--browser", "--due"]), 0, "no pass recorded: due")
            self.assertEqual(ran, [])
            self.assertEqual(web_tests.main(["--browser"]), 0)
            self.assertEqual(web_tests.main(["--browser", "--due"]), 1, "nothing changed since: not due")
            self.assertEqual(web_tests.main(["--due"]), 0, "the unit suite's record is its own")
        self.assertEqual(ran, [web_tests.BROWSER.whole])


REAL_RUN = web_tests.subprocess.run


if __name__ == "__main__":
    unittest.main()
