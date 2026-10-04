# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for check_architecture_map.py: the map's package table against the packages, and
# ABOUTME: schema.md's owners table against the schema's tables, each shown failing (D190).
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_architecture_map as cam  # noqa: E402

REPO = Path(__file__).resolve().parent.parent.parent

MAP = """# Map

## 1. Package table

| Package | Resp. | Second job(s) |
| --- | --- | --- |
| `cmd/orpheum` | COMPOSE | ⚑ calls `internal/core/jobs` |
| `internal/core/api`, `internal/core/auth` | R9 | — |
| `web/apps/web`: session.ts | R5 | — |

## 2. Storage table

| `internal/core/gone` | not the package table |
"""


class Packages(unittest.TestCase):
    def test_the_first_cell_of_each_row_names_the_packages(self):
        self.assertEqual(cam.mapped_packages(MAP),
                         {"cmd/orpheum", "internal/core/api", "internal/core/auth", "web/apps/web"})

    def test_a_package_without_a_row_and_a_row_without_a_package_fail(self):
        existing = {"cmd/orpheum", "internal/core/api", "internal/core/auth", "web/apps/web"}
        self.assertEqual(cam.package_problems(existing, cam.mapped_packages(MAP)), [])
        problems = cam.package_problems((existing | {"internal/core/new"}) - {"internal/core/auth"},
                                        cam.mapped_packages(MAP))
        self.assertEqual(len(problems), 2)
        self.assertIn("internal/core/new has no row", problems[0])
        self.assertIn("names internal/core/auth, which is not a package", problems[1])
        self.assertIn("found no package table", cam.package_problems(existing, set())[0])

    def test_packages_are_directories_with_a_go_file_that_is_not_a_test(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ("cmd/orpheum/main.go", "internal/archtest/doc.go", "internal/only/a_test.go",
                         "internal/core/api/testdata/x.go", "web/apps/web/package.json"):
                (root / name).parent.mkdir(parents=True, exist_ok=True)
                (root / name).write_text("")
            self.assertEqual(cam.go_packages(root), {"cmd/orpheum", "internal/archtest"})
            self.assertEqual(cam.web_packages(root), {"web/apps/web"})


class Tables(unittest.TestCase):
    SCHEMA = ('CREATE TABLE file (id INTEGER);\nCREATE TABLE "grant" (id INTEGER);\n'
              "CREATE VIRTUAL TABLE item_fts USING fts5(name);\n"
              "CREATE TABLE 'item_fts_data'(id INTEGER PRIMARY KEY, block BLOB);\n"
              "CREATE TABLE sqlite_sequence(name,seq);\n")
    PLAN = ("| Table | Database | Writing package | Notes |\n| --- | --- | --- | --- |\n"
            "| `file` | persistent.db | `internal/core/library` | |\n"
            "| `grant` | persistent.db | `internal/core/auth` | |\n"
            "| `item_fts` | persistent.db | `internal/core/identify` | |\n"
            "| `job` | cache.db | `internal/core/jobs` | |\n")

    def test_the_schemas_tables_leave_out_shadow_and_sqlite_tables(self):
        self.assertEqual(cam.schema_tables(self.SCHEMA, "CREATE TABLE job (id INTEGER);\n"),
                         {"file", "grant", "item_fts", "job"})

    def test_a_table_missing_gone_or_listed_twice_fails(self):
        tables = cam.schema_tables(self.SCHEMA, "CREATE TABLE job (id INTEGER);\n")
        self.assertEqual(cam.table_problems(tables, cam.listed_tables(self.PLAN)), [])
        problems = cam.table_problems((tables | {"tag"}) - {"job"},
                                      cam.listed_tables(self.PLAN) + ["file"])
        self.assertEqual(len(problems), 3)
        self.assertIn("table tag has no row", problems[0])
        self.assertIn("lists job, which no schema creates", problems[1])
        self.assertIn("lists file more than once", problems[2])
        self.assertIn("found no owners table", cam.table_problems(tables, [])[0])


class Storage(unittest.TestCase):
    MAP = """## 2. Storage table

| What | Where | Tier | Backed up | Flag |
| --- | --- | --- | --- | --- |
| user, role (the built-in two, admin, user) | persistent.db | T1 | yes | — |
| item_text (name), item_text.search_alternates | persistent.db | T4 | yes | — |
| job | cache.db | T4 | no | — |
| secret.key | instance's persistent/ | T1 | only when asked | — |

## 3. Seams
"""
    TABLES = {"persistent.db": {"user", "role", "item_text"}, "cache.db": {"job"}}

    def test_each_database_row_names_its_tables_and_a_column_or_a_file_is_not_one(self):
        self.assertEqual(cam.stored_tables(self.MAP), [("persistent.db", "user"), ("persistent.db", "role"),
                                                       ("persistent.db", "item_text"), ("cache.db", "job")])
        self.assertEqual(cam.storage_problems(self.TABLES, cam.stored_tables(self.MAP)), [])

    def test_a_table_missing_in_the_wrong_database_or_named_twice_fails(self):
        stored = cam.stored_tables(self.MAP) + [("persistent.db", "user")]
        tables = {"persistent.db": {"user", "item_text", "tag"}, "cache.db": {"job", "role"}}
        problems = cam.storage_problems(tables, stored)
        self.assertEqual(len(problems), 4)
        self.assertIn("persistent.db's table tag has no row", problems[0])
        self.assertIn("names role in persistent.db, which has no such table", problems[1])
        self.assertIn("names persistent.db's user more than once", problems[2])
        self.assertIn("cache.db's table role has no row", problems[3])
        self.assertIn("found no storage table", cam.storage_problems(self.TABLES, [])[0])


class TheRepository(unittest.TestCase):
    def test_the_map_and_the_owners_table_match_the_code(self):
        problems = cam.package_problems(cam.go_packages(REPO) | cam.web_packages(REPO),
                                        cam.mapped_packages((REPO / cam.MAP).read_text()))
        tables = cam.schema_tables(cam.newest_golden(REPO).read_text(), (REPO / cam.CACHE_SCHEMA).read_text())
        problems += cam.table_problems(tables, cam.listed_tables((REPO / cam.SCHEMA_PLAN).read_text()))
        problems += cam.storage_problems(
            {"persistent.db": cam.schema_tables(cam.newest_golden(REPO).read_text()),
             "cache.db": cam.schema_tables((REPO / cam.CACHE_SCHEMA).read_text())},
            cam.stored_tables((REPO / cam.MAP).read_text()))
        self.assertEqual(problems, [])


if __name__ == "__main__":
    unittest.main()
