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
        self.assertEqual(scope, [*web.VITEST_RUN, "apps/web/src/other.test.tsx"])


class AddedFileProbe(unittest.TestCase):
    def test_an_added_web_file_is_probed_first_against_the_tests_importing_it(self):
        repo = Repo({path: text for path, text in FILES.items() if not path.endswith("sign.tsx")})
        base = repo.commit("chore: base")
        repo.commit("feat(web): a sign\n\nThere was none.", {f"{SRC}/sign.tsx": FILES[f"{SRC}/sign.tsx"]})
        ran: list[list[str]] = []

        # A probe's test runs go through run_whole_group (revert_red_run.py); its git through subprocess.
        def fake_run(command, _cwd, _timeout, _env=None):
            ran.append(list(command))
            return subprocess.CompletedProcess(command, 1, "FAIL apps/web/src/row.test.tsx", "")

        with repo.cwd(), mock.patch.object(rr, "run_whole_group", side_effect=fake_run), \
                mock.patch.object(rr, "browser_tier_command", return_value=["make", "browser-tier"]):
            outcome = rr.probe_unclaimed(str(repo.root), base, "HEAD", rr.Change(f"{SRC}/sign.tsx", "A"), 60)
        self.assertEqual(ran[0], [*web.VITEST_RUN, "apps/web/src/list.test.tsx", "apps/web/src/row.test.tsx",
                                  "packages/sdk/src/api.test.ts"])
        self.assertEqual((outcome.verdict, len(ran)), ("RED_TEST", 1), "red from the importing tests, no whole suite")
        self.assertTrue((repo.root / SRC / "sign.tsx").exists(), "the file is put back")


if __name__ == "__main__":
    unittest.main()
