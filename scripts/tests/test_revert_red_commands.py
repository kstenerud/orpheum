# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_commands.py: revert-red's test runs keep their exact form, and no other
# ABOUTME: revert-red script spells one out itself, so a CPU share and every reader see each run.
import re
import sys
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(SCRIPTS))
import revert_red_commands as commands  # noqa: E402

# A test command spelled out as a list: `["go", "test"` or the start of a vitest or npm-run list.
HAND_BUILT = re.compile(r"""\[\s*["']go["'],\s*["']test["']|["']vitest["'],\s*["'](related|run)["']""")


class Builders(unittest.TestCase):
    def test_go_tests_stop_at_their_first_failure_with_their_tags(self) -> None:
        self.assertEqual(commands.go_test("./a/..."), ["go", "test", "-failfast", "./a/..."])
        self.assertEqual(commands.go_test("./a", "./b", tags="integration"),
                         ["go", "test", "-failfast", "-tags", "integration", "./a", "./b"])

    def test_a_go_run_of_named_tests_runs_those_alone_and_says_so(self) -> None:
        narrowed = commands.go_test("./a", "./b", tests=["TestB", "TestA", "TestB"])
        self.assertEqual(narrowed, ["go", "test", "-failfast", "-run", "^(TestA|TestB)$", "./a", "./b"])
        self.assertTrue(commands.is_narrowed(narrowed))
        self.assertFalse(commands.is_narrowed(commands.go_test("./a")))

    def test_vitest_runs_keep_their_form(self) -> None:
        self.assertEqual(commands.vitest("related", "a.ts"),
                         ["npx", "--prefix", "web", "vitest", "related", "--run", "--passWithNoTests",
                          "--root", "web", "--bail=1", "a.ts"])
        self.assertEqual(commands.vitest("related", browser=True),
                         ["npx", "--prefix", "web", "vitest", "related", "--run", "--passWithNoTests",
                          "--root", "web", "--config", "vitest.browser.config.ts", "--bail=1"])
        self.assertEqual(commands.vitest("run", first_red=False), ["npx", "--prefix", "web", "vitest", "run", "--root", "web"])
        self.assertEqual(commands.web_suite(), ["npm", "--prefix", "web", "run", "test", "--", "--bail=1"])

    def test_every_built_run_is_recognised_as_its_kind(self) -> None:
        self.assertTrue(commands.is_go_test(commands.go_test("./a")))
        for run in (commands.vitest("related"), commands.vitest("run"), commands.web_suite()):
            self.assertTrue(commands.is_vitest(run), run)
        self.assertFalse(commands.is_vitest(["npm", "--prefix", "web", "run", "typecheck"]))


class OneWayEach(unittest.TestCase):
    def test_no_other_revert_red_script_spells_out_a_test_run(self) -> None:
        """The share and the readers go by what revert_red_commands builds; a run spelled out elsewhere
        would be missed by both (fourteen were, 2026-10-26)."""
        spelled = [f"{path.name}:{number}"
                   for path in sorted([*SCRIPTS.glob("revert_red_*.py"), SCRIPTS / "check_revert_red.py"])
                   if path.name != "revert_red_commands.py"
                   for number, line in enumerate(path.read_text().splitlines(), 1)
                   if HAND_BUILT.search(line)]
        self.assertEqual(spelled, [], "build these with revert_red_commands")


if __name__ == "__main__":
    unittest.main()
