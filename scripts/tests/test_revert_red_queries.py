# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_queries.py: a generated query file's changed queries are found by their
# ABOUTME: stretches of code, and their callers are the packages outside the generated code naming them.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_queries as queries  # noqa: E402
from gitrepo import Repo  # noqa: E402

GENERATED = '''package sqlcgen

const getA = `-- name: GetA :one
SELECT a FROM t
`

func (q *Queries) GetA(ctx context.Context) (int64, error) {
	return 0, nil
}

const listB = `-- name: ListB :many
SELECT b FROM t
`

func (q *Queries) ListB(ctx context.Context) ([]int64, error) {
	return nil, nil
}
'''


class ChangedQueries(unittest.TestCase):
    def test_a_query_whose_sql_or_method_changed_or_is_new_is_found_and_the_rest_are_not(self):
        after = GENERATED.replace("SELECT b FROM t", "SELECT b FROM t WHERE b > 0") + (
            "\nconst getC = `-- name: GetC :one\nSELECT c\n`\n")
        self.assertEqual(queries.changed_queries(GENERATED, after), ["GetC", "ListB"])

    def test_nothing_changed_is_no_query(self):
        self.assertEqual(queries.changed_queries(GENERATED, GENERATED), [])


class Callers(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({
            "internal/store/sqlcgen/q.sql.go": GENERATED,
            "internal/store/sqlcgen/other.go": "package sqlcgen\n\nfunc use() { q.ListB(ctx) }\n",
            "internal/core/a/a.go": "package a\n\nfunc f() { queries.ListB(ctx) }\n",
            "internal/core/b/b_test.go": "package b\n\nfunc g() { queries.GetA(ctx) }\n",
            "internal/core/c/c.go": "package c\n\nfunc h() { ListB(ctx) }\n",
        })
        self.base = self.repo.commit("base")
        self.head = self.repo.commit("change B", {
            "internal/store/sqlcgen/q.sql.go": GENERATED.replace("SELECT b FROM t", "SELECT b FROM t LIMIT 1")})

    def test_the_callers_of_the_changed_queries_are_the_packages_naming_them_as_a_method(self):
        found = queries.callers_of_change(str(self.repo.root), self.base, self.head,
                                          "internal/store/sqlcgen/q.sql.go")
        self.assertEqual(found, ["./internal/core/a"])

    def test_a_generated_query_files_probe_runs_its_callers_tests_first(self):
        with self.repo.cwd():
            attempts = rr.attempts_for(rr.Change("internal/store/sqlcgen/q.sql.go", "M"), rr.STORE_INTEGRATION,
                                       self.base, self.head)
        self.assertEqual(attempts[0].command, ["go", "test", rr.FIRST_RED_GO, "./internal/core/a"])
        self.assertEqual(attempts[1].command, rr.STORE_INTEGRATION)


class ExportedNames(unittest.TestCase):
    def test_an_exported_declaration_that_changed_or_went_is_found_and_unexported_ones_are_not(self):
        before = ("package x\n\nfunc A() int { return 1 }\n\nfunc (s *S) Method() {}\n\ntype S struct{}\n\n"
                  "const Gone = 2\n\nvar hidden = 3\n")
        after = before.replace("return 1", "return 2").replace("const Gone = 2\n\n", "").replace(
            "hidden = 3", "hidden = 4")
        self.assertEqual(queries.changed_exported_names(before, after), ["A", "Gone"])

    def test_a_grouped_name_is_found_by_its_group(self):
        before = "package x\n\nconst (\n\tKept = 1\n\tChanged = 2\n)\n"
        after = before.replace("Changed = 2", "Changed = 3")
        self.assertIn("Changed", queries.changed_exported_names(before, after))


class Users(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({
            "internal/store/jobs.go": "package store\n\nfunc Open() int { return 1 }\n\nfunc Other() {}\n",
            "internal/core/a/a.go": "package a\n\nfunc f() { store.Open() }\n",
            "internal/core/b/b.go": "package b\n\nfunc g() { store.Other() }\n",
            "internal/core/c/c.go": "package c\n\nfunc h() { store.Open() }\n",
        })
        self.base = self.repo.commit("base")
        self.head = self.repo.commit("change Open", {
            "internal/store/jobs.go": "package store\n\nfunc Open() int { return 2 }\n\nfunc Other() {}\n"})

    def test_the_users_are_the_dependents_naming_a_changed_exported_name(self):
        found = queries.users_of_change(str(self.repo.root), self.base, self.head, "internal/store/jobs.go",
                                        ["./internal/core/a", "./internal/core/b"])
        self.assertEqual(found, ["./internal/core/a"])  # c names it but is no dependent; b names another

    def test_a_go_files_probe_runs_its_users_tests_after_its_own_and_before_every_dependent(self):
        dependents = ["./internal/core/a", "./internal/core/b"]
        saved = rr.go_dependents
        rr.go_dependents = lambda _root, _path: dependents
        try:
            with self.repo.cwd():
                own = ["go", "test", rr.FIRST_RED_GO, "./internal/store/..."]
                attempts = rr.attempts_for(rr.Change("internal/store/jobs.go", "M"), own, self.base, self.head)
        finally:
            rr.go_dependents = saved
        self.assertEqual([attempt.command for attempt in attempts[:3]], [
            own, ["go", "test", rr.FIRST_RED_GO, "./internal/core/a"],
            ["go", "test", rr.FIRST_RED_GO, *dependents],
        ])


if __name__ == "__main__":
    unittest.main()
