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

    def test_a_removed_packages_files_are_left_out_and_the_rest_kept(self):
        present = {"internal/kept", "."}
        paths = ["internal/gone/x.go", "internal/kept/y.go", "internal/gone/README.md", "main.go"]
        self.assertEqual(spot_check.without_removed_packages(paths, is_dir=lambda d: d in present),
                         ["internal/kept/y.go", "internal/gone/README.md", "main.go"])

    def test_web_sources_run_their_related_tests_and_a_stylesheet_is_named_not_run(self):
        spot = spot_check.plan(["web/apps/web/src/a.tsx", "web/apps/web/src/a.css"], MODULE, {})
        self.assertEqual(spot.web_files, ["web/apps/web/src/a.tsx"])
        gaps = " ".join(spot.not_run)
        self.assertIn("web/apps/web/src/a.css", gaps)
        # The type check and lint are its own (the owner, 2026-11-24), not left to make check.
        self.assertNotIn("type check", gaps)
        self.assertEqual(spot.web_typed, ["web/apps/web/src/a.tsx"])
        self.assertEqual(spot.go_packages, [])

    def test_a_gate_script_runs_the_python_tests_and_markdown_the_docs_gate(self):
        self.assertTrue(spot_check.plan(["scripts/x.py"], MODULE, {}).python)
        self.assertTrue(spot_check.plan(["Makefile"], MODULE, {}).python)
        self.assertTrue(spot_check.plan(["mk/dev.mk"], MODULE, {}).python)
        self.assertTrue(spot_check.plan(["docs/a.md"], MODULE, {}).docs)
        self.assertFalse(spot_check.plan(["docs/a.md"], MODULE, {}).python)

    def test_a_script_runs_only_the_gate_tests_reaching_it_and_what_it_cannot_follow_runs_them_all(self):
        def reach(paths):
            return None if "Makefile" in paths else [f"scripts/tests/test_{Path(p).stem}.py" for p in paths]

        spot = spot_check.plan(["scripts/a.py", "docs/b.md"], MODULE, {}, python_reach=reach)
        commands = [run.command for run in spot_check.runs(spot, MODULE)]
        self.assertIn([sys.executable, "scripts/py_tests.py", "scripts/tests/test_a.py"], commands)
        spot = spot_check.plan(["scripts/a.py", "Makefile"], MODULE, {}, python_reach=reach)
        self.assertIn([sys.executable, "scripts/py_tests.py"], [run.command for run in spot_check.runs(spot, MODULE)])
        # A script no test reaches runs none, and says so.
        spot = spot_check.plan(["scripts/a.py"], MODULE, {}, python_reach=lambda paths: [])
        self.assertFalse(any("py_tests.py" in " ".join(run.command) for run in spot_check.runs(spot, MODULE)))
        self.assertIn("scripts/a.py", " ".join(spot.not_run))

    def test_the_real_reach_runs_a_scripts_own_test_and_not_every_module(self):
        spot = spot_check.plan(["scripts/gate_lock.py"], MODULE, {})
        self.assertIn("scripts/tests/test_gate_lock.py", spot.python_tests)
        self.assertLess(len(spot.python_tests), len(list(Path("scripts/tests").glob("test_*.py"))))

    def test_what_it_cannot_run_is_named(self):
        gaps = " ".join(spot_check.plan(["internal/store/queries/q.sql", "go.mod"], MODULE, {}).not_run)
        self.assertIn("make generate", gaps)
        self.assertIn("go.mod", gaps)

    def test_the_media_name_gate_reads_the_changed_files_or_every_file_for_its_own_change(self):
        spot = spot_check.plan(["docs/a.md"], MODULE, {})
        commands = [run.command for run in spot_check.runs(spot, MODULE)]
        self.assertIn([sys.executable, "scripts/check_media_names.py", "docs/a.md"], commands)
        spot = spot_check.plan(["docs/a.md", "scripts/media_names.allow"], MODULE, {})
        commands = [run.command for run in spot_check.runs(spot, MODULE)]
        self.assertIn([sys.executable, "scripts/check_media_names.py"], commands)

    def test_a_shipped_source_runs_the_domain_test_unless_its_package_runs_whole(self):
        domain = [*spot_check.GO_TEST_ONE, f"^{spot_check.DOMAIN_TEST}$", f"./{spot_check.DOMAIN_TEST_PACKAGE}"]
        for shipped in ("web/apps/web/src/a.tsx", "deploy/omv/compose.yaml", "install.sh", "internal/a/x.go"):
            spot = spot_check.plan([shipped], MODULE, IMPORTERS)
            self.assertIn(domain, [run.command for run in spot_check.runs(spot, MODULE)], shipped)
        for unshipped in ("docs/a.md", "internal/a/x_test.go", "web/apps/web/src/a.test.tsx", "scripts/x.py"):
            spot = spot_check.plan([unshipped], MODULE, IMPORTERS)
            self.assertNotIn(domain, [run.command for run in spot_check.runs(spot, MODULE)], unshipped)
        # The package's own change runs its every test, this one among them.
        spot = spot_check.plan([f"{spot_check.DOMAIN_TEST_PACKAGE}/suggested.go"], MODULE, {})
        self.assertNotIn(domain, [run.command for run in spot_check.runs(spot, MODULE)])

    def test_the_runs_are_the_planned_commands(self):
        spot = spot_check.plan(["internal/a/x.go", "web/apps/web/src/a.tsx", "scripts/x.py"], MODULE, IMPORTERS,
                               python_reach=lambda paths: None)
        commands = [run.command for run in spot_check.runs(spot, MODULE)]
        # As make check's go-test runs them, so Go's test cache serves one from the other.
        self.assertEqual(commands[0][:4], ["bash", "-o", "pipefail", "-c"])
        self.assertRegex(commands[0][4], r"^go test -json ./cmd/c ./internal/a ./internal/b \| .*"
                                         r"scripts/go_test_timings.py go-spot$")
        # a.tsx is not here: a deleted file calls for the whole web suite, make check's, and has nothing to lint.
        self.assertEqual(commands[1], spot_check.WEB_TYPECHECK)
        self.assertEqual(commands[2][1:], ["scripts/py_tests.py"])
        self.assertIn(spot_check.WHOLE_WEB_SUITE, spot.not_run)

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

    def test_a_change_to_a_file_a_go_test_reads_runs_that_package_and_lints_nothing(self):
        readers = {"web/packages/sdk/src": {f"{MODULE}/internal/r"}, "docs/users/install.md": {f"{MODULE}/d"},
                   "internal/store/sqlcgen/*.sql.go": {f"{MODULE}/internal/s"}}
        spot = spot_check.plan(["web/packages/sdk/src/rtc-request.ts"], MODULE, {}, readers=readers)
        self.assertEqual(spot.go_packages, [f"{MODULE}/internal/r"])
        self.assertEqual(spot.lint_packages, [])
        self.assertEqual(spot_check.plan(["docs/users/install.md"], MODULE, {}, readers=readers).go_packages,
                         [f"{MODULE}/d"])
        self.assertEqual(spot_check.plan(["internal/store/sqlcgen/q.sql.go"], MODULE, {}, readers=readers).go_packages,
                         [f"{MODULE}/internal/s", f"{MODULE}/internal/store/sqlcgen"])
        self.assertEqual(spot_check.plan(["web/packages/sdk/srcx/a.ts", "docs/users/other.md"], MODULE, {},
                                         readers=readers).go_packages, [], "a sibling of what is read is not it")


class GoTestReaders(unittest.TestCase):
    def setUp(self):
        import tempfile  # noqa: PLC0415
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: subprocess.run(["rm", "-rf", str(self.root)], check=False))
        self.cwd = os.getcwd()
        os.chdir(self.root)
        self.addCleanup(os.chdir, self.cwd)

    def readers(self, files: dict[str, str]) -> dict[str, list[str]]:
        for name, text in files.items():
            Path(name).parent.mkdir(parents=True, exist_ok=True)
            Path(name).write_text(text)
        return {path: sorted(packages) for path, packages in
                spot_check.go_test_readers(MODULE, sorted(files)).items()}

    def test_a_climbing_join_or_literal_maps_what_it_names_to_the_package(self):
        found = self.readers({
            "internal/core/r/r_test.go": 'os.ReadFile(filepath.Join("..", "..", "..", "web", "sdk",\n "src", file))\n'
                                         'const page = "../../../docs/users/install.md"\n',
            "internal/s/s_test.go": 'filepath.Glob(filepath.Join("..", "sqlcgen", "*.sql.go"))\n',
        })
        self.assertEqual(found, {"web/sdk/src": [f"{MODULE}/internal/core/r"],
                                 "docs/users/install.md": [f"{MODULE}/internal/core/r"],
                                 "internal/sqlcgen/*.sql.go": [f"{MODULE}/internal/s"]})

    def test_its_own_package_the_root_and_what_names_no_climb_are_left_out(self):
        found = self.readers({
            "internal/a/a_test.go": 'filepath.Join("..", "a", "testdata")\nname := "../../" + route.Path()\n'
                                    'filepath.Join("..", "..", "..", "outside")\nfilepath.Join(root, "..", "x")\n'
                                    'filepath.Join("testdata", "y")\n',
        })
        self.assertEqual(found, {})


class Wiring(unittest.TestCase):
    def test_make_spot_runs_it_and_help_lists_it(self):
        makefile = (REPO / "Makefile").read_text()
        self.assertRegex(makefile, r"(?m)^spot:.*\n(\t.*\n)*\t.*scripts/spot_check\.py")
        recipe = makefile.split("\nspot: ", 1)[1].split("\n\n", 1)[0]
        self.assertIn("scripts/in_dev_image.py spot", recipe, "in the gate's container, as make check")
        listed = subprocess.run(["make", "-f", str(REPO / "Makefile"), "help"], capture_output=True, text=True,
                                check=True, cwd=REPO).stdout
        self.assertIn("  spot ", listed)
        self.assertIn("  cover ", listed, "and the targets of the Makefile's parts in mk/, unprefixed")
        self.assertEqual(spot_check.DEFAULT_JOBS, 2, "two runs at once by default, beside staging")


if __name__ == "__main__":
    unittest.main()


class WebTests(unittest.TestCase):
    """The web tests the spot check runs are make check's for the change (web_tests.command_for): by
    depth 1 alone, a gallery test reading a changed component's markup through the page drawing it ran
    first in check-pr (the owner, 2026-11-24)."""

    def in_scratch(self, answers: list, typescript: bool = True):  # type: ignore[no-untyped-def]
        """A scratch web client holding a.tsx and its tests, and a reach giving answers' last."""
        import tempfile
        asked: list[tuple] = []

        def reach(base, changed, suite, typescript, depth=None):  # type: ignore[no-untyped-def]
            asked.append((base, changed, suite.step, depth))
            return answers[-1], ""
        scratch = tempfile.TemporaryDirectory()
        self.addCleanup(scratch.cleanup)
        src = Path(scratch.name) / "web/apps/web/src"
        src.mkdir(parents=True)
        if typescript:
            (Path(scratch.name) / "web/node_modules/typescript").mkdir(parents=True)
        (src / "a.tsx").write_text("export const a = 1;\n")
        (src / "a.test.tsx").write_text('import { a } from "./a.tsx";\n')
        (src / "a.browser.test.tsx").write_text('import { a } from "./a.tsx";\n')
        saved = Path.cwd()
        os.chdir(scratch.name)
        self.addCleanup(os.chdir, saved)
        return reach, asked

    def test_a_source_runs_every_unit_test_make_check_would_for_it_at_any_distance(self):
        answers: list = [["apps/web/src/ui-gallery.test.tsx", "apps/web/src/a.browser.test.tsx"]]
        reach, asked = self.in_scratch(answers)
        run = spot_check.web_test_run(["web/apps/web/src/a.tsx"], "base", reach)
        assert isinstance(run, spot_check.Run)
        self.assertEqual(run.command, [*spot_check.UNIT.named, "apps/web/src/ui-gallery.test.tsx"],
                         "what the reach names, the browser suite's left to make check")
        self.assertEqual(asked, [("base", ["apps/web/src/a.tsx"], "web-test", None)], "no depth: make check's reach")
        answers.append(None)
        run = spot_check.web_test_run(["web/apps/web/src/a.tsx"], "base", reach)
        assert isinstance(run, spot_check.Run)
        self.assertEqual(run.command, [*spot_check.UNIT.related, "apps/web/src/a.tsx"],
                         "a reach that cannot tell: vitest related, as make check")
        answers.append(["*"])
        self.assertEqual(spot_check.web_test_run(["web/apps/web/src/a.tsx"], "base", reach), spot_check.WHOLE_WEB_SUITE)
        self.assertEqual(spot_check.web_test_run(["web/apps/web/src/gone.tsx"], "base", reach),
                         spot_check.WHOLE_WEB_SUITE, "a deleted file: the tests importing it would fail")
        self.assertEqual(len(asked), 3, "a deleted file is not asked of the reach")

    def test_a_change_the_reach_cannot_follow_runs_vitest_related_as_make_check_does(self):
        # No TypeScript to read with: a changed test and a source go to vitest related, test and all.
        reach, asked = self.in_scratch([[]], typescript=False)
        run = spot_check.web_test_run(["web/apps/web/src/a.tsx"], "base", reach)
        assert isinstance(run, spot_check.Run)
        self.assertEqual(run.command, [*spot_check.UNIT.related, "apps/web/src/a.tsx"])
        run = spot_check.web_test_run(["web/apps/web/src/a.test.tsx", "web/apps/web/src/a.tsx"], "base", reach)
        assert isinstance(run, spot_check.Run)
        self.assertEqual(run.command, [*spot_check.UNIT.related, "apps/web/src/a.test.tsx", "apps/web/src/a.tsx"])
        self.assertEqual(asked, [])

    def test_a_browser_mode_test_is_left_to_make_check_and_named_not_run(self):
        # A commit of browser tests alone ran the plain vitest, which excludes them, and failed on no
        # test file (2026-10-30).
        reach, _ = self.in_scratch([["apps/web/src/a.browser.test.tsx"]])
        self.assertIsNone(spot_check.web_test_run(["web/apps/web/src/a.browser.test.tsx"], "base", reach))
        planned = spot_check.plan(["web/apps/web/src/a.browser.test.tsx"], "m", {})
        self.assertIn("browser-mode tests, which run in a browser (`make check`)", planned.not_run)

    def test_the_type_check_and_the_lint_of_the_changed_files_run_for_the_webs_typescript(self):
        # A needless `!` failed check-pr's lint, a round of it (the owner, 2026-11-24).
        lint = "web/apps/web/src/offer-card.tsx"  # this repository's own: a file the lint can read
        spot = spot_check.plan([lint, "web/apps/web/src/offer-card.css", "web/tsconfig.json"], MODULE, {})
        runs = {run.label: run for run in spot_check.runs(spot, MODULE)}
        self.assertEqual(runs["the web type check"].command, spot_check.WEB_TYPECHECK)
        self.assertEqual(runs["the web type check"].command[2], "web-typecheck", "make check's record, to share its pass")
        linted = runs["the web lint, 1 file(s)"]
        self.assertEqual(linted.command, [*spot_check.WEB_LINT, "apps/web/src/offer-card.tsx"])
        self.assertEqual(linted.cwd, "web", "eslint finds its configuration from web/")
        labels = [run.label for run in spot_check.runs(spot_check.plan(["docs/a.md"], MODULE, {}), MODULE)]
        self.assertFalse([label for label in labels if "web" in label], "nothing of the web's for a document")
