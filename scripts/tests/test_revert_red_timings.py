# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_timings.py: each probe's runs are timed by kind, kept beside the
# ABOUTME: failures and summed in the report, so a slow revert-red names what took the time.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import last_run  # noqa: E402
import revert_red_timings as timings  # noqa: E402
from gitrepo import Repo, mock_argv  # noqa: E402

COVERED, TEST = "scripts/a.py", "scripts/tests/test_a.py"
TEST_HEAD = ("import sys\nimport unittest\nsys.path.insert(0, 'scripts')\nimport a\n\n\n"
             "class T(unittest.TestCase):\n    def test_a(self):\n        a.f()\n")


class Kinds(unittest.TestCase):
    def test_each_run_is_named_by_what_it_runs(self):
        cases = [
            (["make", "--no-print-directory", "browser-tier", "FFMPEG_DIR=/x"], None, "browser tier"),
            ([*rr.VITEST_RELATED, "apps/web/src/a.ts"], None, "vitest related"),
            (rr.WEB_SUITE, None, "vitest whole suite"),
            (rr.WEB_BUILD, None, "web build"),
            (["npx", "--prefix", "web", "vitest", "run", "--root", "web", "a.test.ts"], None,
             "vitest, an added file's importers"),
            (["npm", "--prefix", "web", "run", "typecheck"], None, "web type check"),
            (["go", "test", "./internal/core/auth/..."], None, "go test, package"),
            (["go", "test", "./..."], None, "go test, whole suite"),
            (["go", "test", "./internal/core/api", "./internal/archtest"], None, "go test, its dependents"),
            (rr.go_test("./internal/core/api", "./cmd/orpheum", tests=["TestA"]), None, "go test, the tests that name it"),
            (["go", "test", "-tags", "dev", "./..."], None, "go test, extra tags"),
            (rr.STORE_INTEGRATION, None, "go test, store integration"),
            (["go", "vet", "./..."], {"GOOS": "darwin"}, "go compile, another system"),
            (rr.PY_TESTS_COMMAND, None, "python tests"),
            ([*rr.PY_TESTS_COMMAND, "scripts/tests/test_x.py"], None, "python tests, those reaching it"),
        ]
        for command, env, kind in cases:
            self.assertEqual(timings.kind_of(command, "", env), kind, command)

    def test_the_summary_sums_by_kind_and_names_the_slowest_files_with_their_runs(self):
        kept = timings.Timings()
        kept.note_run("a.ts", timings.Run("vitest related", 12, 0, True))
        kept.note_run("a.ts", timings.Run("browser tier", 170, 95, False))
        kept.note_probe("a.ts", 280)
        kept.note_run("b.go", timings.Run("go test, package", 4, 0, False))
        kept.note_probe("b.go", 5)
        text = timings.summary(kept, wall_seconds=300, jobs=4)
        self.assertIn("How long it took: 300 s on 4 worktree(s), 285 s of probing in all, 95 s of it waiting", text)
        self.assertRegex(text, r"(?m)^\s+170 s\s+1 run\(s\)  browser tier$")
        self.assertLess(text.index("browser tier"), text.index("vitest related"), "the longest kind first")
        self.assertRegex(text, r"(?m)^\s+280 s  a\.ts  — vitest related 12 s, browser tier 170 s \(red\), "
                         r"waited 95 s$")
        self.assertLess(text.index("  a.ts"), text.index("  b.go"), "the slowest file first")


    def test_the_summary_says_what_came_before_probing_and_each_trees_busy_time(self):
        # check-pr, 2026-11-03: 480 s waiting for a background probe and 1000 s in one tree of four,
        # neither in the report (the owner: "make the testing process more observable").
        kept = timings.Timings()
        kept.note_before("waiting for the background probe to end", 482)
        kept.note_before("preparing the worktrees", 3)
        kept.note_tree("/repo/.git/revert-red/wt-0", 1000)
        kept.note_tree("/repo/.git/revert-red/wt-1", 219)
        text = timings.summary(kept, wall_seconds=1001, jobs=4)
        self.assertIn("Before probing, 485 s: 482 s waiting for the background probe to end, "
                      "3 s preparing the worktrees", text)
        self.assertRegex(text, r"(?m)^\s+1000 s  wt-0  \(100% of the run\)$")
        self.assertLess(text.index("wt-0"), text.index("wt-1"), "the busiest tree first")

    def test_a_stylesheet_settled_with_others_is_ranked_by_its_own_runs(self):
        # Its own browser-mode runs, two minutes each, were listed at 0 s: it has no probe of its own.
        kept = timings.Timings()
        kept.note_run("app-shell.css", timings.Run("vitest related (browser mode)", 127, 0, False))
        kept.note_run("a.go", timings.Run("go test, package", 4, 0, True))
        kept.note_probe("a.go", 5)
        text = timings.summary(kept, wall_seconds=200, jobs=2)
        self.assertIn("132 s of probing in all", text)
        self.assertRegex(text, r"(?m)^\s+127 s  app-shell\.css  — vitest related \(browser mode\) 127 s \(red\)$")
        self.assertLess(text.index("app-shell.css  —"), text.index("a.go  —"))


class Recorded(unittest.TestCase):
    def test_a_run_keeps_each_files_runs_and_reports_the_time(self):
        repo = Repo({COVERED: "def f():\n    return 1\n", TEST: TEST_HEAD})
        base = repo.commit("chore: base")
        repo.commit("fix(a): answer two\n\nIt answered one.", {
            COVERED: "def f():\n    return 20\n", TEST: TEST_HEAD + "        assert a.f() == 20\n"})
        with repo.cwd() as out, mock_argv(["check_revert_red.py", "--base", base, "--jobs", "1"]):
            status = rr.main()
            record = last_run.read(timings.GATE, timings.NAME)
        self.assertEqual(status, 0, out.getvalue())
        self.assertIn("How long it took:", out.getvalue())
        self.assertIn(f"  {COVERED}  — python tests", out.getvalue())
        files = {each["path"]: each for each in record["files"]}
        # The revert's run, then the clean run that lets its red count, timed as a kind of its own
        # (the caching audit's finding 5, 2026-10-28).
        self.assertEqual([run["kind"] for run in files[COVERED]["runs"]],
                         ["python tests, those reaching it", "python tests, those reaching it, nothing reverted"])
        self.assertFalse(files[COVERED]["runs"][0]["passed"], "the revert's run went red")
        self.assertTrue(files[COVERED]["runs"][1]["passed"], "and passed with nothing reverted")
        self.assertGreater(files[COVERED]["seconds"], 0)
        self.assertEqual(record["jobs"], 1)
        self.assertEqual(set(record["before"]), {"waiting for the background probe to end",
                                                 "waiting for another run's probe trees", "preparing the worktrees"})
        self.assertEqual(list(record["trees"]), ["wt-0"])
        self.assertIn("Before probing, ", out.getvalue())
        self.assertIn("Each probe tree's busy time", out.getvalue())


if __name__ == "__main__":
    unittest.main()
