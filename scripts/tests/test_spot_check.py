# SPDX-License-Identifier: AGPL-3.0-or-later
"""Tests for scripts/spot_check.py: what a change runs (D279)."""

from __future__ import annotations

import os
import subprocess
import sys
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parent.parent
REPO = SCRIPTS.parent
sys.path.insert(0, str(SCRIPTS))

import spot_check  # noqa: E402

MODULE = "example.com/m"
IMPORTERS = {
    f"{MODULE}/internal/a": {f"{MODULE}/internal/b", f"{MODULE}/cmd/c"},
    f"{MODULE}/internal/b": {f"{MODULE}/cmd/c"},
}


class Planning(unittest.TestCase):
    def test_a_go_change_runs_its_package_and_its_direct_importers_only(self):
        spot = spot_check.plan(["internal/a/x.go"], MODULE, IMPORTERS)
        self.assertEqual(spot.go_packages, [f"{MODULE}/cmd/c", f"{MODULE}/internal/a", f"{MODULE}/internal/b"])
        spot = spot_check.plan(["internal/b/y_test.go"], MODULE, IMPORTERS)
        self.assertEqual(spot.go_packages, [f"{MODULE}/cmd/c", f"{MODULE}/internal/b"])
        self.assertIn("what imports the importers", " ".join(spot.not_run))

    def test_web_sources_run_their_related_tests_and_a_stylesheet_is_named_not_run(self):
        spot = spot_check.plan(["web/apps/web/src/a.tsx", "web/apps/web/src/a.css"], MODULE, {})
        self.assertEqual(spot.web_files, ["web/apps/web/src/a.tsx"])
        gaps = " ".join(spot.not_run)
        self.assertIn("web/apps/web/src/a.css", gaps)
        self.assertIn("type check", gaps)
        self.assertEqual(spot.go_packages, [])

    def test_a_gate_script_runs_the_python_tests_and_markdown_the_docs_gate(self):
        self.assertTrue(spot_check.plan(["scripts/x.py"], MODULE, {}).python)
        self.assertTrue(spot_check.plan(["Makefile"], MODULE, {}).python)
        self.assertTrue(spot_check.plan(["docs/a.md"], MODULE, {}).docs)
        self.assertFalse(spot_check.plan(["docs/a.md"], MODULE, {}).python)

    def test_what_it_cannot_run_is_named(self):
        gaps = " ".join(spot_check.plan(["internal/store/queries/q.sql", "go.mod"], MODULE, {}).not_run)
        self.assertIn("make generate", gaps)
        self.assertIn("go.mod", gaps)

    def test_the_runs_are_the_planned_commands(self):
        spot = spot_check.plan(["internal/a/x.go", "web/apps/web/src/a.tsx", "scripts/x.py"], MODULE, IMPORTERS)
        commands = [run.command for run in spot_check.runs(spot, MODULE)]
        # As make check's go-test runs them, so Go's test cache serves one from the other.
        self.assertEqual(commands[0][:4], ["bash", "-o", "pipefail", "-c"])
        self.assertRegex(commands[0][4], r"^go test -json ./cmd/c ./internal/a ./internal/b \| .*"
                                         r"scripts/go_test_timings.py go-spot$")
        self.assertEqual(commands[1][1:], ["scripts/py_tests.py"], "no web test imports a.tsx here")

    def test_a_go_change_lints_its_own_package_and_not_its_importers(self):
        # The changed package must exist for lint to run on it: this repository's own.
        spot = spot_check.plan(["cmd/orpheum/x.go"], MODULE, {f"{MODULE}/cmd/orpheum": {f"{MODULE}/z"}})
        self.assertEqual(spot.lint_packages, [f"{MODULE}/cmd/orpheum"])
        previous = os.getcwd()
        os.chdir(REPO)
        try:
            commands = [run.command for run in spot_check.runs(spot, MODULE)]
        finally:
            os.chdir(previous)
        self.assertIn(["golangci-lint", "run", "--allow-serial-runners", "--build-tags", "integration", "./cmd/orpheum"],
                      commands)

    def test_a_deleted_package_is_not_linted(self):
        spot = spot_check.plan(["internal/gone/x.go"], MODULE, {})
        commands = [run.command for run in spot_check.runs(spot, MODULE)]
        self.assertFalse([c for c in commands if c[0] == "golangci-lint"])



class Wiring(unittest.TestCase):
    def test_make_spot_runs_it_and_help_lists_it(self):
        makefile = (REPO / "Makefile").read_text()
        self.assertRegex(makefile, r"(?m)^spot:.*\n(\t.*\n)*\t.*scripts/spot_check\.py")
        recipe = makefile.split("\nspot: ", 1)[1].split("\n\n", 1)[0]
        self.assertIn("scripts/in_dev_image.py spot", recipe, "in the gate's container, as make check")
        listed = subprocess.run(["make", "-f", str(REPO / "Makefile"), "help"], capture_output=True, text=True,
                                check=True, cwd=REPO).stdout
        self.assertIn("  spot ", listed)
        self.assertEqual(spot_check.DEFAULT_JOBS, 2, "two runs at once by default, beside staging")


if __name__ == "__main__":
    unittest.main()


class WebTests(unittest.TestCase):
    def test_a_changed_test_runs_and_a_source_runs_only_the_tests_importing_it_directly(self):
        import tempfile
        with tempfile.TemporaryDirectory() as scratch:
            src = Path(scratch) / "web/apps/web/src"
            src.mkdir(parents=True)
            (src / "a.tsx").write_text("export const a = 1;\n")
            (src / "b.tsx").write_text('import { a } from "./a.tsx";\nexport const b = a;\n')
            (src / "a.test.tsx").write_text('import { a } from "./a.tsx";\n')
            (src / "b.test.tsx").write_text('import { b } from "./b.tsx";\n')
            (src / "c.test.tsx").write_text("\n")
            saved = Path.cwd()
            os.chdir(scratch)
            try:
                self.assertEqual(spot_check.web_tests(["web/apps/web/src/a.tsx"]), ["apps/web/src/a.test.tsx"])
                self.assertEqual(spot_check.web_tests(["web/apps/web/src/c.test.tsx", "web/apps/web/src/b.tsx"]),
                                 ["apps/web/src/b.test.tsx", "apps/web/src/c.test.tsx"])
            finally:
                os.chdir(saved)

    def test_a_browser_mode_test_is_left_to_make_check_and_named_not_run(self):
        # A commit of browser tests alone ran the plain vitest, which excludes them, and failed on no
        # test file (2026-10-30).
        import tempfile
        with tempfile.TemporaryDirectory() as scratch:
            src = Path(scratch) / "web/apps/web/src"
            src.mkdir(parents=True)
            (src / "a.tsx").write_text("export const a = 1;\n")
            (src / "a.browser.test.tsx").write_text('import { a } from "./a.tsx";\n')
            (src / "a.test.tsx").write_text('import { a } from "./a.tsx";\n')
            saved = Path.cwd()
            os.chdir(scratch)
            try:
                self.assertEqual(spot_check.web_tests(["web/apps/web/src/a.browser.test.tsx"]), [])
                self.assertEqual(spot_check.web_tests(["web/apps/web/src/a.tsx"]), ["apps/web/src/a.test.tsx"])
            finally:
                os.chdir(saved)
        planned = spot_check.plan(["web/apps/web/src/a.browser.test.tsx"], "m", {})
        self.assertIn("browser-mode tests, which run in a browser (`make check`)", planned.not_run)
