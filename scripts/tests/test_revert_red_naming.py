# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_naming.py: a Go file's changed declarations are read from its two versions,
# ABOUTME: and the tests whose code names them, nearest first, run before the probe's usual runs, never instead.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_naming as naming  # noqa: E402
from gitrepo import Repo  # noqa: E402

SOURCE = '''package lib

import (
	"fmt"
	_ "embed"
)

// Helper is named in a comment: "Open".
func helper() string { return fmt.Sprint("Open(") }

// Open opens.
func Open() string { return helper() }

func (s *Store) Close() {}

type Store struct{}

const (
	First = 1
	Second = 2
)

var registry = map[string]int{}
'''


class Reading(unittest.TestCase):
    def test_comments_and_literals_name_nothing(self):
        stripped = naming.strip('a := "Open" // Close\n/* Store */ b := `x\ny` + \'c\'\n')
        self.assertNotIn("Open", stripped)
        self.assertNotIn("Close", stripped)
        self.assertNotIn("Store", stripped)
        self.assertEqual(stripped.count("\n"), 3)

    def test_a_files_declarations_imports_and_methods_are_read(self):
        parsed = naming.parse(SOURCE)
        self.assertEqual(parsed.package, "lib")
        self.assertEqual(parsed.imports, {"fmt": "fmt"})
        self.assertEqual(parsed.blank_imports, frozenset({"_ embed"}))
        self.assertEqual([declaration.names for declaration in parsed.declarations],
                         [("helper",), ("Open",), ("Store.Close",), ("Store",), ("First", "Second"), ("registry",)])

    def test_an_aliased_and_a_dot_import_are_used_by_their_names(self):
        parsed = naming.parse('package x\n\nimport (\n\tst "example.com/m/store"\n\t. "example.com/m/dot"\n)\n',
                              {"example.com/m/store": "store"})
        self.assertEqual(parsed.imports, {"st": "example.com/m/store", ".example.com/m/dot": "example.com/m/dot"})


class ChangedNames(unittest.TestCase):
    def test_a_changed_body_and_a_changed_method_are_named_and_a_comment_is_not(self):
        after = (SOURCE.replace("// Open opens.", "// Open opens a store.")
                 .replace('return fmt.Sprint("Open(")', 'return "x"').replace("Close() {}", "Close() { _ = 1 }"))
        self.assertEqual(naming.changed_names(SOURCE, after), {"helper", "Store.Close"})

    def test_a_new_file_names_everything_it_declares(self):
        self.assertEqual(naming.changed_names(None, SOURCE),
                         {"helper", "Open", "Store.Close", "Store", "First", "Second", "registry"})

    def test_what_no_declaration_holds_cannot_be_placed(self):
        for after in (SOURCE.replace('_ "embed"', '_ "unsafe"'),
                      SOURCE + "\nfunc init() { registry[\"a\"] = 1 }\n",
                      SOURCE + "\nvar _ = fmt.Sprint()\n",
                      SOURCE.replace("package lib", "package lib2")):
            self.assertIsNone(naming.changed_names(SOURCE, after), after[-60:])

    def test_the_callers_of_a_changed_helper_are_the_next_level(self):
        levels = naming.callers_by_level([naming.parse(SOURCE)], {"helper"})
        self.assertEqual(levels, [{"helper"}, {"helper", "Open"}])


MODULE = "example.com/m"
def library(open_body: str = "helper()") -> str:
    return SOURCE.replace("return helper()", f"return {open_body}")


TESTS = {
    "go.mod": f"module {MODULE}\n\ngo 1.26\n",
    "lib/lib.go": library(),
    # An internal test naming Open bare; one through a helper of the test files; one naming nothing.
    "lib/open_test.go": ("package lib\n\nimport \"testing\"\n\nfunc TestOpen(t *testing.T) { _ = Open() }\n\n"
                         "func TestThroughAHelper(t *testing.T) { opened(t) }\n\n"
                         "func TestOther(t *testing.T) {}\n\nfunc TestOther2(t *testing.T) {}\n\n"
                         "func TestOther3(t *testing.T) {}\n"),
    "lib/helpers_test.go": "package lib\n\nimport \"testing\"\n\nfunc opened(t *testing.T) string { return Open() }\n",
    # The external test package names it through its import.
    "lib/lib_test.go": ("package lib_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/lib\"\n)\n\n"
                        "func TestExternal(t *testing.T) { _ = lib.Open() }\n"),
    # A dependent: one test names lib.Open, one a method Close, and two neither.
    "app/app.go": "package app\n\nimport \"example.com/m/lib\"\n\nfunc Run() string { return lib.Open() }\n",
    "app/app_test.go": ("package app\n\nimport (\n\t\"testing\"\n\n\tlb \"example.com/m/lib\"\n)\n\n"
                        "func TestNamesIt(t *testing.T) { _ = lb.Open() }\n\n"
                        "func TestCloses(t *testing.T) { var s lb.Store; s.Close() }\n\n"
                        "func TestRuns(t *testing.T) { _ = Run() }\n\nfunc TestNothing(t *testing.T) {}\n\n"
                        "func TestNothing2(t *testing.T) {}\n"),
}


class Tests(unittest.TestCase):
    def setUp(self):
        self.repo = Repo(TESTS)
        self.base = self.repo.commit("base")

    def naming_of(self, files: dict[str, str], path: str = "lib/lib.go",
                  dependents: tuple[str, ...] = ("./app",)) -> naming.Naming | None:
        head = self.repo.commit("change", files)
        return naming.go_tests_naming(str(self.repo.root), self.base, head, path, list(dependents))

    def test_the_tests_naming_the_changed_function_bare_through_a_helper_or_an_import(self):
        found = self.naming_of({"lib/lib.go": library('helper() + "!"')})
        self.assertEqual(sorted(found.own), ["TestExternal", "TestOpen", "TestThroughAHelper"])
        self.assertEqual(found.dependents, {"./app": ["TestNamesIt"]})

    def test_a_changed_helper_is_found_through_its_caller_the_nearest_level_any_test_names(self):
        found = self.naming_of({"lib/lib.go": library().replace('fmt.Sprint("Open(")', '"x"')})
        self.assertEqual(sorted(found.own), ["TestExternal", "TestOpen", "TestThroughAHelper"])

    def test_a_method_is_named_by_any_selector_of_its_name(self):
        found = self.naming_of({"lib/lib.go": library().replace("Close() {}", "Close() { _ = 1 }")})
        self.assertEqual((found.own, found.dependents), ([], {"./app": ["TestCloses"]}))

    def test_a_package_where_most_tests_name_it_is_left_to_its_whole_run(self):
        found = self.naming_of({"lib/lib.go": library('helper() + "!"'),
                                "app/app_test.go": TESTS["app/app_test.go"].replace(
                                    "func TestNothing(t *testing.T) {}", "func TestNothing(t *testing.T) { lb.Open() }")
                                .replace("func TestNothing2(t *testing.T) {}",
                                         "func TestNothing2(t *testing.T) { lb.Open() }")})
        self.assertEqual(found.dependents, {})

    def test_no_test_naming_it_or_a_change_it_cannot_place_narrows_nothing(self):
        self.assertIsNone(self.naming_of({"lib/lib.go": library().replace("= 2", "= 3")}))
        self.assertIsNone(self.naming_of({"lib/lib.go": library() + "\nfunc init() {}\n"}))

    def test_a_test_that_reads_the_sources_is_left_to_its_own_run(self):
        found = self.naming_of({"lib/lib.go": library('helper() + "!"'), "internal/archtest/arch_test.go": (
            "package archtest\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/lib\"\n)\n\n"
            "func TestArch_Reads(t *testing.T) { _ = lib.Open() }\n\nfunc TestArch_Other(t *testing.T) {}\n\n"
            "func TestArch_Third(t *testing.T) {}\n")}, dependents=("./app", "./internal/archtest"))
        self.assertEqual(found.dependents, {"./app": ["TestNamesIt"]})


class Attempts(unittest.TestCase):
    """The tests that name a change come first and decide only a red: every run the probe made before
    still follows, so a green verdict rests on the same runs as ever."""

    def setUp(self):
        self.repo = Repo(TESTS)
        self.base = self.repo.commit("base")
        self.head = self.repo.commit("change", {"lib/lib.go": library('helper() + "!"')})

    def attempts(self) -> list[list[str]]:
        saved = rr.go_dependents
        rr.go_dependents = lambda _root, _path: ["./app"]
        try:
            with self.repo.cwd():
                return [attempt.command for attempt in rr.attempts_for(
                    rr.Change("lib/lib.go", "M"), rr.scope_for("lib/lib.go"), self.base, self.head)]
        finally:
            rr.go_dependents = saved

    def test_its_own_packages_naming_tests_first_then_its_whole_run_then_the_dependents_naming_tests(self):
        runs = self.attempts()
        self.assertEqual(runs[:4], [
            rr.go_test("./lib", tests=["TestExternal", "TestOpen", "TestThroughAHelper"]),
            rr.go_test("./lib/..."),
            rr.go_test("./app", tests=["TestNamesIt"]),
            rr.go_test("./app"),
        ])

    def test_every_run_the_probe_made_without_them_still_follows(self):
        saved = naming.go_tests_naming
        naming.go_tests_naming = lambda *_args: None
        try:
            without = self.attempts()
        finally:
            naming.go_tests_naming = saved
        self.assertEqual([run for run in self.attempts() if not rr.revert_red_commands.is_narrowed(run)], without)


if __name__ == "__main__":
    unittest.main()
