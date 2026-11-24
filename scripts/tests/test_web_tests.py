# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for web_tests.py: make check runs only the web tests a change since their last pass
# ABOUTME: reaches, and the whole suite when what every test reads changed or a file was deleted.
import re
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

    def test_a_tree_another_worktree_passed_is_measured_from_the_nearest_pass(self) -> None:
        # The record is shared by every worktree: with one tree kept, the lead's checkout and an agent's
        # worktree each found the other's pass and ran the tests of both trees' differences (2026-11-04).
        ran: list[list[str]] = []

        def run(command, *args, **kwargs):  # type: ignore[no-untyped-def]
            if command[0] == "git":
                return REAL_RUN(command, *args, **kwargs)
            ran.append(command)
            return mock.Mock(returncode=0)

        with self.repo.cwd(), mock.patch.object(web_tests.subprocess, "run", side_effect=run):
            self.repo.write({"web/apps/web/src/a.ts": "export const a = 2;"})
            self.assertEqual(web_tests.main([]), 0)  # the lead's tree: the first pass runs all
            self.repo.write({"web/apps/web/src/a.ts": "export const a = 1;", "web/apps/web/src/b.ts": "export const b = 2;"})
            self.assertEqual(web_tests.main([]), 0)  # an agent's tree
            self.repo.write({"web/apps/web/src/a.ts": "export const a = 2;", "web/apps/web/src/b.ts": "export const b = 1;"})
            self.assertEqual(web_tests.main([]), 0)  # the lead's again: passed already, nothing runs
            self.repo.write({"web/apps/web/src/a.ts": "export const a = 3;"})
            self.assertEqual(web_tests.main([]), 0)
        self.assertEqual(len(ran), 3)
        self.assertEqual(ran[2][len(web_tests.RELATED):][:1], ["apps/web/src/a.ts"],
                         "measured from the lead's pass, not the agent's: b.ts is not run again")
        self.assertNotIn("apps/web/src/b.ts", ran[2])
        with self.repo.cwd():
            self.assertEqual(len(web_tests.passed_trees(web_tests.stamp_path(web_tests.UNIT.step))), 3)

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
        self.assertEqual(ran, web_tests.runs_of(web_tests.BROWSER.whole, web_tests.BROWSER))

    def test_each_browser_runs_in_a_run_of_its_own_one_after_the_other_stopping_at_a_failure(self) -> None:
        # Run together, vitest opened Firefox pages as Chromium's files ended, and each took the
        # focus from a running test's frame, which lost the keys it pressed (2026-11-20).
        ran: list[list[str]] = []
        failing = "chromium"

        def run(command, *args, **kwargs):  # type: ignore[no-untyped-def]
            if command[0] == "git":
                return REAL_RUN(command, *args, **kwargs)
            ran.append(command)
            return mock.Mock(returncode=1 if command[-1] == failing else 0)

        whole = web_tests.BROWSER.whole
        self.assertEqual(web_tests.runs_of(whole, web_tests.BROWSER),
                         [[*whole, "--", "--project", "chromium"], [*whole, "--", "--project", "firefox"]])
        related = [*web_tests.BROWSER.related, "apps/web/src/b.ts"]
        self.assertEqual(web_tests.runs_of(related, web_tests.BROWSER),
                         [[*related, "--project", "chromium"], [*related, "--project", "firefox"]])
        self.assertEqual(web_tests.runs_of(web_tests.UNIT.whole, web_tests.UNIT), [web_tests.UNIT.whole])
        with self.repo.cwd(), mock.patch.object(web_tests.subprocess, "run", side_effect=run):
            self.assertEqual(web_tests.main(["--browser"]), 1)
            self.assertEqual([command[-1] for command in ran], ["chromium"], "Firefox's run waits on Chromium's")
            self.assertEqual(web_tests.main(["--browser", "--due"]), 0, "a failed run records no pass")
            failing = "none"
            self.assertEqual(web_tests.main(["--browser"]), 0)
            self.assertEqual([command[-1] for command in ran], ["chromium", "chromium", "firefox"])


SRC = "web/apps/web/src"
# The modules the reach is tried on: a.ts's exports, re-exported by name through index.ts; b.ts using
# one of them; c.ts's type and function; load.ts registering a use of a.ts as it loads.
REACH_BASE = {
    "web/package.json": "{}",
    f"{SRC}/a.ts": ("export function one(): number {\n  return 1;\n}\n"
                    "export function two(): number {\n  return 2;\n}\n"
                    "const table = new Map<string, number>();\n"
                    "export function remember(key: string): void {\n  table.set(key, 1);\n}\n"
                    "export function recall(key: string): number | undefined {\n  return table.get(key);\n}\n"),
    f"{SRC}/index.ts": ('export { one, two, remember, recall } from "./a.ts";\n'
                        'export { made } from "./b.ts";\n'
                        'export const packageName = "sdk";\n'),
    f"{SRC}/b.ts": ('import { two } from "./index.ts";\n'
                    "export const made = (): number => two() * 2;\n"
                    "export const other = (): number => 3;\n"),
    f"{SRC}/c.ts": "export interface Thing {\n  x: number;\n}\nexport function c(): number {\n  return 1;\n}\n",
    f"{SRC}/registry.ts": "const kept: (() => number)[] = [];\nexport function register(f: () => number): void {\n  kept.push(f);\n}\n",
    f"{SRC}/load.ts": ('import { one } from "./index.ts";\nimport { register } from "./registry.ts";\n'
                       "register(() => one());\n"),
    f"{SRC}/one.test.ts": 'import { one } from "./index.ts";\nvoid one;\n',
    f"{SRC}/two.test.ts": 'import { two } from "./index.ts";\nvoid two;\n',
    f"{SRC}/made.test.ts": 'import { made } from "./index.ts";\nvoid made;\n',
    f"{SRC}/other.test.ts": 'import { other } from "./b.ts";\nvoid other;\n',
    f"{SRC}/recall.test.ts": 'import { recall } from "./index.ts";\nvoid recall;\n',
    f"{SRC}/thing.test.ts": 'import type { Thing } from "./c.ts";\nconst t: Thing = { x: 1 };\nvoid t;\n',
    f"{SRC}/mocked.test.ts": 'import { vi } from "vitest";\nvi.mock("./c.ts");\n',
    f"{SRC}/loads.test.ts": 'import "./load.ts";\n',
    f"{SRC}/poster.css": "a {}\n",
    "web/e2e/setup.ts": 'import { register } from "../apps/web/src/registry.ts";\nvoid register;\n',
}
TYPESCRIPT = Path(__file__).resolve().parents[2] / "web" / "node_modules" / "typescript"
SETUP = web_tests.Suite("unit", [], [], [], setup=("e2e/setup.ts",))


class BrowserLockTest(unittest.TestCase):
    """A run left behind held the browser-mode lock for about 12 hours, and every make check in that time
    hung at the browser tests (the owner, 2026-11-24, the test-time stock-take's item 3)."""

    def setUp(self) -> None:
        import tempfile
        self.path = Path(tempfile.mkdtemp(prefix="browser-lock-")) / "browser-mode.lock"

    def held_by(self, started: float):  # type: ignore[no-untyped-def]
        import fcntl
        import json
        holder = open(self.path, "a+", encoding="utf-8")  # noqa: SIM115 (released by the test)
        self.addCleanup(holder.close)
        fcntl.flock(holder, fcntl.LOCK_EX)
        holder.write(json.dumps({"pid": 4242, "started": started}))
        holder.flush()
        return holder

    def test_a_holder_past_its_lease_is_named_and_the_run_fails_rather_than_waiting(self) -> None:
        self.held_by(started=1000.0)

        def sleep(_seconds: float) -> None:
            raise AssertionError("it waited on a holder past its lease")
        with mock.patch("builtins.print") as printed, \
                web_tests.browser_lock(self.path, lease=60, sleep=sleep, now=lambda: 1061.0) as held:
            self.assertFalse(held)
        said = printed.call_args.args[0]
        self.assertIn("did not run", said)
        self.assertIn("pid 4242", said)

    def test_a_holder_within_its_lease_is_waited_for_and_the_lock_then_held(self) -> None:
        import fcntl
        holder = self.held_by(started=1000.0)

        def sleep(_seconds: float) -> None:
            fcntl.flock(holder, fcntl.LOCK_UN)  # the holder ends
        with web_tests.browser_lock(self.path, lease=60, sleep=sleep, now=lambda: 1030.0) as held:
            self.assertTrue(held)
            self.assertIn('"started": 1030.0', self.path.read_text(), "it says it holds it, and since when")

    def test_a_holder_that_wrote_nothing_is_dated_from_when_it_was_found_and_a_release_leaves_no_record(self) -> None:
        import fcntl
        with web_tests.browser_lock(self.path, now=lambda: 1.0) as held:
            self.assertTrue(held)
        self.assertEqual(self.path.read_text(), "", "a finished holder's record would date the next")
        holder = open(self.path, "a+", encoding="utf-8")  # noqa: SIM115 (closed below)
        self.addCleanup(holder.close)
        fcntl.flock(holder, fcntl.LOCK_EX)  # as flock(1) holds it: no record
        import os
        os.utime(self.path, (0, 0))  # the file itself is far older than the lease
        clock = iter([5000.0, 5010.0, 5020.0])

        def sleep(_seconds: float) -> None:
            fcntl.flock(holder, fcntl.LOCK_UN)
        with web_tests.browser_lock(self.path, lease=60, sleep=sleep, now=lambda: next(clock)) as held:
            self.assertTrue(held, "waited for, though the file is older than the lease")

    def test_make_checks_browser_run_takes_the_leased_lock(self) -> None:
        makefile = (Path(__file__).resolve().parents[2] / "Makefile").read_text()
        recipe = re.search(r"(?m)^web-test-browser-run:.*\n\t(.*)", makefile)
        assert recipe is not None
        self.assertIn("web_tests.py --browser --lock $(BROWSER_MODE_LOCK)", recipe.group(1))
        self.assertNotIn("flock", recipe.group(1))


class ReachTest(unittest.TestCase):
    """web_test_reach.cjs, run as make check runs it, on REACH_BASE and a change to it."""

    def setUp(self) -> None:
        if not TYPESCRIPT.is_dir():
            self.fail(f"{TYPESCRIPT} is missing: run `npm --prefix web ci`")  # a missing capability fails
        self.repo = Repo(REACH_BASE)
        self.base = self.repo.commit("chore: base")

    def reach(self, edit: dict[str, str], suite: web_tests.Suite = web_tests.UNIT,
              depth: int | None = None) -> list[str] | None:
        self.repo.write(edit)
        with self.repo.cwd():
            tests, _ = web_tests.reached_tests(self.base, [path.removeprefix("web/") for path in edit], suite, TYPESCRIPT,
                                               depth)
        return None if tests is None else sorted(path.removeprefix("apps/web/src/") for path in tests)

    def edited(self, path: str, old: str, new: str) -> dict[str, str]:
        self.assertIn(old, REACH_BASE[path])
        return {path: REACH_BASE[path].replace(old, new)}

    def test_a_body_s_change_reaches_the_tests_using_it_by_name_through_a_barrel(self) -> None:
        self.assertEqual(self.reach(self.edited(f"{SRC}/a.ts", "return 2;", "return 3;")),
                         ["made.test.ts", "two.test.ts"],
                         "made uses two; one.test.ts and recall.test.ts load index.ts, which loads a.ts, "
                         "but use nothing that changed")

    def test_a_depth_keeps_the_tests_using_a_change_themselves_through_a_barrel(self) -> None:
        self.assertEqual(self.reach(self.edited(f"{SRC}/a.ts", "return 2;", "return 3;"), depth=1),
                         ["two.test.ts"], "two.test.ts uses two through index.ts, a barrel though it holds a "
                         "literal; made.test.ts uses b.ts's made, which uses two: one further")

    def test_a_change_to_what_writes_shared_state_reaches_what_reads_it(self) -> None:
        self.assertEqual(self.reach(self.edited(f"{SRC}/a.ts", "table.set(key, 1)", "table.set(key, 2)")),
                         ["recall.test.ts"], "remember writes the table recall reads")

    def test_a_use_registered_as_a_module_loads_reaches_what_loads_it(self) -> None:
        self.assertEqual(self.reach(self.edited(f"{SRC}/a.ts", "return 1;", "return 4;")),
                         ["loads.test.ts", "one.test.ts"],
                         "load.ts hands one to another module as it loads, so what loads it may run it")

    def test_a_changed_top_level_reaches_everything_loading_it(self) -> None:
        self.assertEqual(self.reach({f"{SRC}/a.ts": REACH_BASE[f"{SRC}/a.ts"] + 'console.log("loaded");\n'}),
                         ["loads.test.ts", "made.test.ts", "one.test.ts", "other.test.ts", "recall.test.ts",
                          "two.test.ts"], "every importer of index.ts loads a.ts, and other.test.ts's b.ts too")

    def test_a_type_only_import_is_not_reached_and_a_mock_takes_the_module_whole(self) -> None:
        self.assertEqual(self.reach(self.edited(f"{SRC}/c.ts", "return 1;", "return 2;")), ["mocked.test.ts"])

    def test_a_comment_reaches_no_test(self) -> None:
        self.assertEqual(self.reach(self.edited(f"{SRC}/index.ts", 'export { made }', '// made\nexport { made }')), [])

    def test_what_the_setup_files_load_reaches_every_test(self) -> None:
        self.assertEqual(self.reach(self.edited(f"{SRC}/registry.ts", "kept.push(f)", "kept.unshift(f)"), SETUP),
                         ["*"])
        self.assertEqual(self.reach(self.edited(f"{SRC}/registry.ts", "kept.push(f)", "kept.unshift(f)")),
                         ["loads.test.ts"], "without the setup file, only what loads registry.ts")

    def test_what_the_reach_cannot_read_falls_back(self) -> None:
        self.assertIsNone(self.reach({f"{SRC}/poster.css": "a { color: red; }\n"}))

    def test_the_browser_suite_s_setup_files_are_its_configuration_s(self) -> None:
        config = (TYPESCRIPT.parents[1] / "vitest.browser.config.ts").read_text()
        match = re.search(r"setupFiles:\s*\[([^\]]*)\]", config)
        self.assertIsNotNone(match)
        files = tuple(name.removeprefix("./") for name in re.findall(r'"([^"]+)"', match.group(1)))
        self.assertEqual(web_tests.BROWSER.setup, files)


class ReachedRunTest(unittest.TestCase):
    """What command_for and main make of the reach's answer."""

    def setUp(self) -> None:
        self.repo = Repo({"web/package.json": "{}", "web/apps/web/src/a.ts": "export const a = 1;",
                          "web/apps/web/src/reads.test.ts": "readFileSync('x');",
                          "web/node_modules/typescript/package.json": "{}"})
        self.repo.commit("first")
        self.changes = [("M", "web/apps/web/src/a.ts")]

    def command(self, tests: list[str] | None, suite: web_tests.Suite = web_tests.UNIT) -> list[str] | None:
        return web_tests.command_for(self.changes, str(self.repo.root), suite, "base",
                                     lambda base, changed, suite, typescript: (tests, "" if tests else "why"))

    def test_each_suite_runs_its_own_reached_tests_by_name_and_the_source_readers(self) -> None:
        reached = ["apps/web/src/b.test.ts", "apps/web/src/b.browser.test.tsx"]
        self.assertEqual(self.command(reached), [*web_tests.UNIT.named, "apps/web/src/b.test.ts",
                                                 "apps/web/src/reads.test.ts"])
        self.assertEqual(self.command(reached, web_tests.BROWSER),
                         [*web_tests.BROWSER.named, "apps/web/src/b.browser.test.tsx"])

    def test_none_of_a_suite_s_tests_reached_is_no_run_and_every_one_the_whole(self) -> None:
        self.assertEqual(self.command([], web_tests.BROWSER), [], "vitest given no file runs every test")
        self.assertEqual(self.command(["*"]), web_tests.UNIT.whole)
        self.assertEqual(self.command(None)[:len(web_tests.RELATED)], web_tests.RELATED)

    def test_a_run_of_none_records_the_pass(self) -> None:
        ran: list[list[str]] = []

        def run(command, *args, **kwargs):  # type: ignore[no-untyped-def]
            if command[0] == "git":
                return REAL_RUN(command, *args, **kwargs)
            ran.append(command)
            return mock.Mock(returncode=0)

        with self.repo.cwd(), mock.patch.object(web_tests.subprocess, "run", side_effect=run), \
                mock.patch.object(web_tests, "reached_tests", return_value=([], "")):
            self.assertEqual(web_tests.main(["--browser"]), 0)  # no record: the whole suite
            self.repo.write({"web/apps/web/src/a.ts": "export const a = 2;"})
            self.assertEqual(web_tests.main(["--browser"]), 0)
            self.assertEqual(web_tests.main(["--browser", "--due"]), 1, "the run of none was recorded as a pass")
        self.assertEqual(ran, web_tests.runs_of(web_tests.BROWSER.whole, web_tests.BROWSER))


REAL_RUN = web_tests.subprocess.run


if __name__ == "__main__":
    unittest.main()
