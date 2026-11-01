# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_known.py: a red revert is taken again only for the same blobs over the
# ABOUTME: same trees of what its run read, and a probe takes it without running unless told to trust none;
# ABOUTME: a green is kept for its part, and the tests an earlier red failed are tried first.
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_known as known  # noqa: E402
from gitrepo import Repo  # noqa: E402


class DirsRead(unittest.TestCase):
    def test_a_runs_directories_are_its_go_packages_the_web_client_or_the_scripts(self):
        self.assertEqual(known.dirs_read(["go", "test", "-failfast", "./internal/a/...", "./internal/b"]),
                         ["internal/a", "internal/b"])
        self.assertEqual(known.dirs_read(["npx", "--prefix", "web", "vitest", "related", "x.ts"]), ["web"])
        self.assertEqual(known.dirs_read([sys.executable, "/r/scripts/py_tests.py"]), ["scripts", ".claude/hooks"])
        self.assertEqual(known.dirs_read(["go", "vet", "./..."]), [])


class Known(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({"internal/a/a.go": "package a\n", "internal/a/a_test.go": "package a\n",
                          "internal/b/b.go": "package b\n"})
        self.base = self.repo.commit("base")
        self.head = self.repo.commit("change a", {"internal/a/a.go": "package a\n\nvar x = 1\n"})
        self.root = str(self.repo.root)
        self.key = known.revert_key(self.root, self.base, self.head, ["internal/a/a.go"])

    def remember(self) -> None:
        known.remember(self.root, self.head, self.key, ["go", "test", "./internal/a/..."], "RED_TEST", "a test fails")

    def test_a_red_is_known_while_what_its_run_read_is_unchanged(self):
        with self.repo.cwd():
            self.remember()
            later = self.repo.commit("change b", {"internal/b/b.go": "package b\n\nvar y = 2\n"})
            self.assertEqual(known.known(self.root, later, self.key)["verdict"], "RED_TEST")

    def test_a_red_is_not_known_once_a_test_it_ran_changed(self):
        with self.repo.cwd():
            self.remember()
            later = self.repo.commit("weaken", {"internal/a/a_test.go": "package a\n\n// nothing\n"})
            self.assertIsNone(known.known(self.root, later, self.key))

    def test_a_red_is_not_known_for_another_revert_of_the_file(self):
        with self.repo.cwd():
            self.remember()
            later = self.repo.commit("again", {"internal/a/a.go": "package a\n\nvar x = 2\n"})
            other = known.revert_key(self.root, self.base, later, ["internal/a/a.go"])
            self.assertIsNone(known.known(self.root, later, other))

    def test_a_probe_takes_a_known_red_without_running_and_trusting_none_runs(self):
        with self.repo.cwd():
            self.remember()
            change = rr.Change("internal/a/a.go", "M")
            taken = rr.probe_unclaimed(self.root, self.base, self.head, change, 60)
            self.assertEqual(taken.verdict, "RED_TEST")
            self.assertIn("as its probe at", taken.detail)
            rr.TRUST_RECORD = False
            try:
                ran = rr.probe_unclaimed(self.root, self.base, self.head, change, 60)
            finally:
                rr.TRUST_RECORD = True
            self.assertNotIn("as its probe at", ran.detail)


    def test_a_green_is_known_for_its_part_alone_and_never_hides_a_red(self):
        with self.repo.cwd():
            known.remember(self.root, self.head, self.key, [["go", "test", "./internal/a"]], "GREEN", "green",
                           rr.BROWSER_DEFERRED)
            self.assertEqual(known.known(self.root, self.head, self.key, rr.BROWSER_DEFERRED)["verdict"], "GREEN")
            self.assertIsNone(known.known(self.root, self.head, self.key, rr.BROWSER_ONLY),
                              "a green in the code part says nothing of the browser part's runs")
            self.remember()
            self.assertEqual(known.known(self.root, self.head, self.key, rr.BROWSER_DEFERRED)["verdict"], "RED_TEST")

    def test_a_green_whose_runs_read_something_unknown_is_not_kept(self):
        with self.repo.cwd():
            known.remember(self.root, self.head, self.key, [["go", "test", "./internal/a"], ["make", "browser-tier"]],
                           "GREEN", "green", rr.BROWSER_INLINE)
            self.assertIsNone(known.known(self.root, self.head, self.key, rr.BROWSER_INLINE))


class GoReads(unittest.TestCase):
    """A Go run read every package it compiled, not only those it named (the caching audit, finding 3)."""

    def test_a_red_is_not_known_once_a_package_its_tests_import_changed(self):
        repo = Repo({"go.mod": "module m\n\ngo 1.26\n",
                     "internal/a/a.go": 'package a\n\nimport "m/internal/c"\n\nvar X = c.Y\n',
                     "internal/c/c.go": "package c\n\nvar Y = 1\n", "internal/d/d.go": "package d\n"})
        base = repo.commit("base")
        head = repo.commit("change a", {"internal/a/a.go": 'package a\n\nimport "m/internal/c"\n\nvar X = c.Y + 1\n'})
        root = str(repo.root)
        key = known.revert_key(root, base, head, ["internal/a/a.go"])
        with repo.cwd():
            known.remember(root, head, key, ["go", "test", "./internal/a"], "RED_TEST", "a test fails")
            unrelated = repo.commit("change d", {"internal/d/d.go": "package d\n\nvar Z = 0\n"})
            self.assertIsNotNone(known.known(root, unrelated, key))
            imported = repo.commit("change c", {"internal/c/c.go": "package c\n\nvar Y = 2\n"})
            self.assertIsNone(known.known(root, imported, key))


class WebReads(unittest.TestCase):
    """A web red is kept by the modules of the tests it failed, not the whole web client (the owner,
    2026-10-30: every web commit threw every web red away)."""

    def test_a_web_red_outlives_a_change_elsewhere_in_the_client_and_not_one_its_test_imports(self):
        src = "web/apps/web/src"
        repo = Repo({f"{src}/a.ts": "export const a = 1;\n", f"{src}/a.test.ts": 'import { a } from "./a.ts";\na;\n',
                     f"{src}/b.ts": "export const b = 1;\n", f"{src}/c.ts": "export const c = 1;\n"})
        base = repo.commit("base")
        head = repo.commit("change a", {f"{src}/a.ts": "export const a = 2;\n"})
        root = str(repo.root)
        key = known.revert_key(root, base, head, [f"{src}/a.ts"])
        with repo.cwd():
            known.remember(root, head, key, ["npx", "--prefix", "web", "vitest", "related", "a.ts"], "RED_TEST",
                           "a test fails", output=" FAIL  apps/web/src/a.test.ts > a > is two\n")
            elsewhere = repo.commit("change b", {f"{src}/b.ts": "export const b = 2;\n"})
            self.assertEqual(known.known(root, elsewhere, key)["verdict"], "RED_TEST")
            weakened = repo.commit("weaken", {f"{src}/a.test.ts": 'import { a } from "./a.ts";\n'})
            self.assertIsNone(known.known(root, weakened, key))

    def test_a_web_red_whose_output_names_no_test_keeps_the_whole_client(self):
        src = "web/apps/web/src"
        repo = Repo({f"{src}/a.ts": "export const a = 1;\n", f"{src}/b.ts": "export const b = 1;\n"})
        base = repo.commit("base")
        head = repo.commit("change a", {f"{src}/a.ts": "export const a = 2;\n"})
        root = str(repo.root)
        key = known.revert_key(root, base, head, [f"{src}/a.ts"])
        with repo.cwd():
            known.remember(root, head, key, ["npx", "--prefix", "web", "vitest", "related", "a.ts"], "RED_TEST",
                           "a test fails", output="Error: something\n")
            elsewhere = repo.commit("change b", {f"{src}/b.ts": "export const b = 2;\n"})
            self.assertIsNone(known.known(root, elsewhere, key))


GO_OUTPUT = """--- FAIL: TestPlayback_Stalls (0.20s)
    --- FAIL: TestPlayback_Stalls/sub (0.10s)
--- FAIL: TestPlayback_Starts (0.10s)
FAIL
FAIL\tgithub.com/kstenerud/orpheum/internal/core/api\t3.1s
ok  \tgithub.com/kstenerud/orpheum/internal/store\t1.0s
"""
VITEST_OUTPUT = " FAIL  |default| apps/web/src/play.test.tsx > the trail > goes up\n FAIL  apps/web/src/up-trail.browser.test.tsx\n"


class CaughtBefore(unittest.TestCase):
    def test_a_red_runs_failed_tests_are_read_by_package_and_file(self):
        failed = known.failed_tests(GO_OUTPUT + VITEST_OUTPUT, "github.com/kstenerud/orpheum")
        self.assertEqual(failed["go"], {"./internal/core/api": ["TestPlayback_Stalls", "TestPlayback_Starts"]})
        self.assertEqual(failed["vitest"], ["apps/web/src/play.test.tsx", "apps/web/src/up-trail.browser.test.tsx"])

    def test_a_browser_mode_runs_failed_files_are_named_and_rerun_in_browser_mode(self):
        # The test-regime audit of 2026-10-31, R5: unnamed, the confirming run was every browser-mode
        # test that imports the file, 224 s.
        output = (" \x1b[31mFAIL\x1b[39m  \x1b[46m chromium \x1b[49m apps/web/src/app-shell.browser.test.tsx > lays out\n"
                  " FAIL   firefox  apps/web/src/app-shell.browser.test.tsx > lays out\n")
        failed = known.failed_tests(output, "github.com/kstenerud/orpheum", known.commands.vitest("related", browser=True))
        self.assertEqual(failed, {"vitest": ["apps/web/src/app-shell.browser.test.tsx"]})
        runs = known.runs_of(failed)
        self.assertEqual(len(runs), 1)
        self.assertIn("vitest.browser.config.ts", runs[0].command)

    def test_a_runs_failures_are_its_own_runners_alone(self):
        # A gate script's tests print vitest's and Go's failures as fixtures (check-pr, 2026-10-30).
        python_run = [sys.executable, "scripts/py_tests.py"]
        output = VITEST_OUTPUT + GO_OUTPUT + "--- test_revert_red_web.py failed ---\n"
        self.assertEqual(known.failed_tests(output, "github.com/kstenerud/orpheum", python_run),
                         {"python": ["test_revert_red_web.py"]})
        self.assertEqual(list(known.failed_tests(output, "github.com/kstenerud/orpheum",
                                                 known.commands.vitest("run"))), ["vitest"])

    @staticmethod
    def tree_with(scratch: str, *files: str) -> str:
        for file in files:
            (Path(scratch) / file).parent.mkdir(parents=True, exist_ok=True)
            (Path(scratch) / file).write_text("")
        return scratch

    def test_a_later_probe_of_the_file_runs_them_first(self):
        record: dict = {}
        with mock.patch.object(known.last_run, "update", side_effect=lambda gate, name, change: record.update(change(dict(record)))), \
                mock.patch.object(known.last_run, "read", side_effect=lambda gate, name: record), \
                tempfile.TemporaryDirectory() as scratch:
            tree = self.tree_with(scratch, "internal/core/api/x_test.go", "web/apps/web/src/play.test.tsx",
                                  "web/apps/web/src/up-trail.browser.test.tsx")
            known.remember_failed("internal/core/api/x.go", GO_OUTPUT + VITEST_OUTPUT, "github.com/kstenerud/orpheum")
            runs = known.caught_before("internal/core/api/x.go", tree)
            self.assertEqual(known.caught_before("internal/core/api/other.go", tree), [])
        self.assertEqual(runs[0].command, ["go", "test", "-failfast", "-run", "^(TestPlayback_Stalls|TestPlayback_Starts)$",
                                           "./internal/core/api"])
        self.assertIn("apps/web/src/play.test.tsx", runs[1].command)
        self.assertIn("vitest.browser.config.ts", runs[2].command)
        self.assertTrue(all("caught its revert before" in run.label for run in runs))

    def test_a_hint_naming_a_test_the_tree_no_longer_has_is_not_run(self):
        # The stale main.tsx hint named a deleted test file, whose run failed with nothing reverted
        # too, and the probe called its environment broken (the 11-01 deploy).
        record: dict = {}
        with mock.patch.object(known.last_run, "update", side_effect=lambda gate, name, change: record.update(change(dict(record)))), \
                mock.patch.object(known.last_run, "read", side_effect=lambda gate, name: record), \
                tempfile.TemporaryDirectory() as scratch:
            tree = self.tree_with(scratch, "web/apps/web/src/play.test.tsx")
            known.remember_failed("apps/web/src/main.tsx", GO_OUTPUT + VITEST_OUTPUT + "--- test_gone.py failed ---\n",
                                  "github.com/kstenerud/orpheum")
            runs = known.caught_before("apps/web/src/main.tsx", tree)
        self.assertEqual(len(runs), 1)
        self.assertIn("apps/web/src/play.test.tsx", runs[0].command)
        self.assertNotIn("apps/web/src/up-trail.browser.test.tsx", runs[0].command)


if __name__ == "__main__":
    unittest.main()
