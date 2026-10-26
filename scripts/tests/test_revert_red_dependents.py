# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_dependents.py: every Go package whose tests parse the whole module's
# ABOUTME: sources is listed as a source reader, so any Go file's revert-red probe runs its tests.
import re
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import revert_red_dependents as dependents  # noqa: E402

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


if __name__ == "__main__":
    unittest.main()
