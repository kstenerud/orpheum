# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_dependents.py: every Go package whose tests parse the whole module's
# ABOUTME: sources is listed as a source reader, so any Go file's revert-red probe runs its tests;
# ABOUTME: a web test reading sources, itself or through its helper, is a web source reader.
import re
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import revert_red_dependents as dependents  # noqa: E402
from gitrepo import Repo  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent.parent
# A test that parses Go source and walks directories or lists packages reads beyond its own one.
PARSES = re.compile(r'"go/parser"')
WALKS = re.compile(r'WalkDir\(|filepath\.Walk\(|"go", "list"|"list", "-')


def module_source_readers() -> set[str]:
    """The packages, "./x/y", with a test file that parses Go source and walks the module."""
    found = set()
    for top in ("cmd", "internal", "discovery"):
        for test in (ROOT / top).rglob("*_test.go"):
            text = test.read_text(errors="replace")
            if PARSES.search(text) and WALKS.search(text):
                found.add("./" + str(test.parent.relative_to(ROOT)))
    return found


class SourceReaders(unittest.TestCase):
    def test_every_package_whose_tests_walk_the_modules_sources_is_listed(self):
        # internal/store/storesources was made (e3b120a0) without being listed, so a Go file's
        # probe never ran its tests: a revert they would catch passed as "nothing to notice".
        self.assertEqual(module_source_readers() - set(dependents.GO_SOURCE_READERS), set())

    def test_every_listed_package_exists(self):
        for package in dependents.GO_SOURCE_READERS:
            self.assertTrue((ROOT / package).is_dir(), package)


class WebSourceReaders(unittest.TestCase):
    """A web test that reads sources through a helper of its own is a reader as one that reads them
    itself: check-pr called eight stylesheets green on revert though stylesheet-choices.test.ts, which
    reads them through test-sources.ts, failed with them reverted (2026-11-12)."""

    def readers(self) -> tuple[tuple[str, ...], tuple[str, ...]]:
        repo = Repo({
            "web/src/test-sources.ts": 'import { readFileSync } from "node:fs";\nexport const s = readFileSync;\n',
            "web/src/server.ts": "export const server = 1;\n",
            "web/src/sheets.test.ts": 'import { s } from "./test-sources";\ns("base.css");\n',
            "web/src/deep/widgets.test.ts": 'import { s } from "../test-sources.ts";\ns("x.tsx");\n',
            "web/src/itself.test.ts": 'import { readdirSync } from "node:fs";\nreaddirSync(".");\n',
            "web/src/other/server.ts": 'import { readFileSync } from "node:fs";\n',
            "web/src/page.test.tsx": 'import { server } from "./server";\n',
        })
        repo.commit("chore: base")
        root = str(repo.root)
        dependents.web_source_readers.cache_clear()
        return dependents.web_source_readers(root), dependents.web_stylesheet_readers(root)

    def test_a_test_reading_through_its_helper_is_a_reader_and_one_importing_a_namesake_is_not(self):
        readers, _ = self.readers()
        self.assertEqual(readers, ("src/deep/widgets.test.ts", "src/itself.test.ts", "src/sheets.test.ts"),
                         "page.test.tsx imports ./server, not the other/server.ts that reads")

    def test_the_stylesheet_readers_are_the_readers_naming_a_stylesheet(self):
        _, sheets = self.readers()
        self.assertEqual(sheets, ("src/sheets.test.ts",))


if __name__ == "__main__":
    unittest.main()
