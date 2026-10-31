# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_web.py: an added web file is probed against the tests that import
# ABOUTME: it, found before its revert deletes it, not the whole suite (D279).
import subprocess
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_web as web  # noqa: E402
from gitrepo import Repo  # noqa: E402

SRC = "web/apps/web/src"
FILES = {
    f"{SRC}/sign.tsx": "export function Sign() { return null; }\n",
    f"{SRC}/row.tsx": 'import { Sign } from "./sign.tsx";\nexport const Row = Sign;\n',
    f"{SRC}/list.tsx": 'export { Row } from "./row.tsx";\n',
    f"{SRC}/list.test.tsx": 'import { Row } from "./list.tsx";\nRow;\n',
    f"{SRC}/row.test.tsx": 'import { Row } from "./row";\nRow;\n',
    f"{SRC}/other.test.tsx": 'import "./other.tsx";\n',
    f"{SRC}/other.tsx": "export {};\n",
    "web/packages/sdk/src/api.test.ts": 'import { x } from "../../../apps/web/src/sign.tsx";\nx;\n',
}


class ImportingTests(unittest.TestCase):
    def test_the_tests_that_import_a_file_directly_or_through_others_and_no_other(self):
        repo = Repo(FILES)
        self.assertEqual(web.importing_tests(str(repo.root), f"{SRC}/sign.tsx"),
                         ["apps/web/src/list.test.tsx", "apps/web/src/row.test.tsx", "packages/sdk/src/api.test.ts"])
        self.assertEqual(web.importing_tests(str(repo.root), f"{SRC}/other.tsx"), ["apps/web/src/other.test.tsx"])

    def test_a_file_no_test_imports_or_that_is_not_there_has_no_scope(self):
        repo = Repo({**FILES, f"{SRC}/alone.tsx": "export {};\n"})
        self.assertIsNone(web.added_file_scope(str(repo.root), f"{SRC}/alone.tsx"))
        self.assertIsNone(web.added_file_scope(str(repo.root), f"{SRC}/missing.tsx"))
        scope = web.added_file_scope(str(repo.root), f"{SRC}/other.tsx")
        self.assertEqual(scope, [*web.PROBE_RUN, "apps/web/src/other.test.tsx"])


class ChangedFileOrder(unittest.TestCase):
    def test_a_changed_web_files_probe_runs_its_direct_tests_then_the_type_check_before_its_related_set(self):
        # The test-regime audit of 2026-10-31, R3: eight files' related runs, 110-140 s green each,
        # came before a type check red in 30 s.
        repo = Repo(FILES)
        path = f"{SRC}/sign.tsx"
        with repo.cwd():
            runs = [attempt.command for attempt in rr.attempts_for(rr.Change(path, "M"), rr.scope_for(path))]
        direct = web.importing_tests(str(repo.root), path, depth=1)
        self.assertEqual(runs[:3], [*([[*web.PROBE_RUN, *direct]] if direct else []), rr.TYPECHECK,
                                    rr.scope_for(path)][:3])
        self.assertEqual(runs.count(rr.TYPECHECK), 1, "the type check once, early")


class ShippedSourceReaders(unittest.TestCase):
    """The Go tests that read every shipped source as text run for a web or shell file's revert:
    check-pr called en-lab.ts green on revert though the owner's-domains guard fails with it reverted
    (2026-10-31)."""

    def test_a_web_or_shell_files_probe_runs_them_after_its_own_tests_and_before_the_wide_run(self):
        repo = Repo({**FILES, "install.sh": "echo hello\n"})
        guards = [rr.go_test(package, tests=[test]) for package, test in rr.GO_SHIPPED_SOURCE_READERS]
        for path, first in ((f"{SRC}/sign.tsx", rr.scope_for(f"{SRC}/sign.tsx")),
                            ("install.sh", [*rr.PY_TESTS_COMMAND, "scripts/tests/test_install_script.py"])):
            with self.subTest(path), repo.cwd():
                runs = [attempt.command for attempt in rr.attempts_for(rr.Change(path, "M"), first)]
                for guard in guards:
                    self.assertIn(guard, runs)
                    self.assertGreater(runs.index(guard), runs.index(first))
                    wide = rr.wide_scope(path)
                    if wide is not None and wide in runs:
                        self.assertLess(runs.index(guard), runs.index(wide))

    def test_each_one_named_is_a_test_in_its_package(self):
        # A run of a name no test has passes, matching nothing, so a renamed guard would go unnoticed.
        root = Path(__file__).resolve().parents[2]
        for package, test in rr.GO_SHIPPED_SOURCE_READERS:
            sources = "".join(file.read_text(encoding="utf-8") for file in (root / package).glob("*_test.go"))
            self.assertIn(f"func {test}(t *testing.T)", sources, f"{package} has no {test}")


class AddedFileProbe(unittest.TestCase):
    def test_an_added_web_file_is_probed_first_against_the_tests_importing_it(self):
        repo = Repo({path: text for path, text in FILES.items() if not path.endswith("sign.tsx")})
        base = repo.commit("chore: base")
        repo.commit("feat(web): a sign\n\nThere was none.", {f"{SRC}/sign.tsx": FILES[f"{SRC}/sign.tsx"]})
        ran: list[list[str]] = []

        # A probe's test runs go through run_whole_group (revert_red_run.py); its git through subprocess.
        # They fail only while the added file is gone, as tests that notice it do (A45).
        def fake_run(command, _cwd, _timeout, _env=None):
            ran.append(list(command))
            failed = 0 if (repo.root / SRC / "sign.tsx").exists() else 1
            return subprocess.CompletedProcess(command, failed, "FAIL apps/web/src/row.test.tsx", "")

        with repo.cwd(), mock.patch.object(rr, "run_whole_group", side_effect=fake_run), \
                mock.patch.object(rr, "browser_tier_command", return_value=["make", "browser-tier"]):
            outcome = rr.probe_unclaimed(str(repo.root), base, "HEAD", rr.Change(f"{SRC}/sign.tsx", "A"), 60)
        self.assertEqual(ran[0], [*web.PROBE_RUN, "apps/web/src/list.test.tsx", "apps/web/src/row.test.tsx",
                                  "packages/sdk/src/api.test.ts"])
        # The test file that failed once more with the file back, which passes, so the red stands;
        # never the whole suite (the owner, 2026-10-28: only the tests that failed run again).
        self.assertEqual((outcome.verdict, ran[1:]), ("RED_TEST", [[*web.PROBE_RUN, "apps/web/src/row.test.tsx"]]),
                         "red from the importing tests, no whole suite")
        self.assertTrue((repo.root / SRC / "sign.tsx").exists(), "the file is put back")


if __name__ == "__main__":
    unittest.main()


class ModulesRead(unittest.TestCase):
    """What a vitest run of some tests reads: their imports, relative and through the workspace's
    packages, and the configurations; unknown when a test's module reads files by path."""

    def test_the_tests_imports_through_relative_paths_and_packages_and_the_configuration(self):
        repo = Repo({**FILES, f"{SRC}/uses-sdk.test.tsx": 'import { x } from "@orpheum/sdk";\nx;\n',
                     "web/packages/sdk/src/index.ts": 'export * from "./x.ts";\n',
                     "web/packages/sdk/src/x.ts": "export const x = 1;\n",
                     "web/vitest.config.ts": 'import "./e2e/timings.ts";\n',
                     "web/e2e/timings.ts": 'import "node:fs";\n'})
        root = str(repo.root)
        self.assertEqual(web.modules_read(root, ["apps/web/src/list.test.tsx"]),
                         [f"{SRC}/list.test.tsx", f"{SRC}/list.tsx", f"{SRC}/row.tsx", f"{SRC}/sign.tsx",
                          "web/e2e/timings.ts", "web/vitest.config.ts"])
        self.assertEqual(web.modules_read(root, ["apps/web/src/uses-sdk.test.tsx"]),
                         [f"{SRC}/uses-sdk.test.tsx", "web/e2e/timings.ts", "web/packages/sdk/src/index.ts",
                          "web/packages/sdk/src/x.ts", "web/vitest.config.ts"])

    def test_a_test_whose_modules_read_files_by_path_or_that_is_not_there_reads_what_is_unknown(self):
        repo = Repo({**FILES, f"{SRC}/reads.test.ts": 'import { readFileSync } from "node:fs";\nreadFileSync;\n',
                     f"{SRC}/globs.test.ts": 'import "./globbed.ts";\n',
                     f"{SRC}/globbed.ts": 'export const all = import.meta.glob("./*.ts");\n'})
        root = str(repo.root)
        self.assertIsNone(web.modules_read(root, ["apps/web/src/reads.test.ts"]))
        self.assertIsNone(web.modules_read(root, ["apps/web/src/globs.test.ts"]))
        self.assertIsNone(web.modules_read(root, ["apps/web/src/missing.test.ts"]))
