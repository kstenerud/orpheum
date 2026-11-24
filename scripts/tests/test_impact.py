# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for impact.py: check-pr's integration tier, full web lint and browser tier run only
# ABOUTME: what a change can reach, and everything when a change may reach anything.
import json
import re
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import browser_map  # noqa: E402
import impact  # noqa: E402
from gitrepo import Repo  # noqa: E402

REPO = Path(__file__).resolve().parent.parent.parent
PACKAGES = ["./internal/node/media", "./internal/core/scan"]


class Integration(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({"internal/node/media/media.go": "package media\n",
                          "internal/node/media/testdata/clip.txt": "a\n",
                          "internal/core/scan/scan.go": "package scan\n",
                          "internal/store/store.go": "package store\n"})
        self.base = self.repo.commit("chore: base")

    def scope(self, *paths: str) -> list[str]:
        deps = {"./internal/node/media": frozenset({"./internal/node/media", "./internal/store"}),
                "./internal/core/scan": frozenset({"./internal/core/scan"})}
        with self.repo.cwd():
            return impact.integration_scope(list(paths), PACKAGES, self.base, lambda _: deps)[0]

    def test_a_package_reaches_the_integration_packages_whose_tests_build_it(self):
        self.assertEqual(self.scope("internal/store/store.go"), ["./internal/node/media"])
        self.assertEqual(self.scope("internal/core/scan/scan.go"), ["./internal/core/scan"])

    def test_a_file_under_a_package_is_the_package_s_though_not_go(self):
        self.assertEqual(self.scope("internal/node/media/testdata/clip.txt"), ["./internal/node/media"])

    def test_what_builds_into_no_go_test_reaches_none(self):
        self.assertEqual(self.scope("docs/a.md", "web/apps/web/src/a.ts", "scripts/check_docs.py"), [])

    def test_what_may_reach_any_package_runs_them_all(self):
        self.assertEqual(self.scope("go.mod"), PACKAGES)
        self.assertEqual(self.scope("Makefile"), PACKAGES)
        self.assertEqual(self.scope("scripts/fetch_ffmpeg.py"), PACKAGES, "it pins the tier's FFmpeg")

    def test_a_deleted_package_s_files_are_still_its(self):
        self.repo.commit("refactor: no store", {"internal/store/store.go": None})
        self.assertEqual(self.scope("internal/store/store.go"), ["./internal/node/media"])


class WebLint(unittest.TestCase):
    def scope(self, *paths: str) -> list[str]:
        return impact.web_lint_scope(list(paths), "base",
                                     lambda base, sources: (["apps/web/src/importer.tsx", *sources], ""))[0]

    def test_a_web_source_lints_with_what_its_reach_names(self):
        self.assertEqual(self.scope("web/apps/web/src/a.ts", "docs/a.md"),
                         ["apps/web/src/importer.tsx", "apps/web/src/a.ts"])

    def test_nothing_eslint_reads_lints_nothing(self):
        self.assertEqual(self.scope("docs/a.md", "internal/a.go", "web/apps/web/src/a.css"), [])

    def test_a_configuration_or_a_declaration_lints_everything(self):
        for path in ("web/eslint.config.js", "web/tsconfig.base.json", "web/package-lock.json",
                     "web/apps/web/src/env.d.ts"):
            self.assertEqual(self.scope(path), ["."], path)

    def test_a_reach_that_cannot_tell_lints_everything(self):
        self.assertEqual(impact.web_lint_scope(["web/apps/web/src/a.ts"], "base",
                                               lambda base, sources: (None, "a.ts declares a global"))[0], ["."])


SRC = "web/apps/web/src"
# The base every Reach test changes: a.ts exports two things; b.ts uses one, c.ts the other; d.ts
# passes a's Body on in a declaration of its own, which e.ts uses; index.ts re-exports a by name and
# whole, and f.ts and g.ts use one name of each.
REACH_BASE = {
    f"{SRC}/a.ts": "export type Body = { a: number };\n"
                   "export const make = (a: number): Body => ({ a });\n"
                   "export const count = (a: number): number => a + 1;\n",
    f"{SRC}/b.ts": 'import { make } from "./a.ts";\nexport const b: number = make(1).a;\n',
    f"{SRC}/c.ts": 'import { count } from "./a.ts";\nexport const c: number = count(1);\n',
    f"{SRC}/d.ts": 'import type { Body } from "./a.ts";\nexport const wrap = (body: Body): { body: Body } => ({ body });\n'
                   "export const plain = (n: number): number => n;\n",
    f"{SRC}/e.ts": 'import { wrap } from "./d.ts";\nexport const e: number = wrap({ a: 1 }).body.a;\n',
    f"{SRC}/e2.ts": 'import { plain } from "./d.ts";\nexport const e2: number = plain(1);\n',
    f"{SRC}/index.ts": 'export { make as made } from "./a.ts";\nexport * from "./c.ts";\n',
    f"{SRC}/f.ts": 'import { made } from "./index.ts";\nexport const f: number = made(1).a;\n',
    f"{SRC}/g.ts": 'import { c } from "./index.ts";\nexport const g: number = c;\n',
}


class Reach(unittest.TestCase):
    def setUp(self):
        self.repo = Repo(REACH_BASE)
        self.base = self.repo.commit("chore: base")

    def reach(self, files: dict[str, str | None]) -> list[str] | None:
        self.repo.write(files)
        with self.repo.cwd():
            scoped, why = impact.reach(self.base, [path.removeprefix("web/") for path in files])
        return None if scoped is None else sorted(path.removeprefix("apps/web/src/") for path in scoped)

    def test_a_change_inside_a_body_lints_the_file_alone(self):
        self.assertEqual(self.reach({f"{SRC}/a.ts": REACH_BASE[f"{SRC}/a.ts"].replace("a + 1", "a + 2")}), ["a.ts"])

    def test_an_export_s_changed_type_lints_what_uses_it_and_not_what_uses_another(self):
        changed = REACH_BASE[f"{SRC}/a.ts"].replace("count = (a: number): number", "count = (a: number): string")
        self.assertEqual(self.reach({f"{SRC}/a.ts": changed.replace("a + 1", "`${a}`")}), ["a.ts", "c.ts"],
                         "c.ts uses count, and its own c is declared a number still, so g.ts sees nothing new")

    def test_a_change_passes_through_a_star_re_export(self):
        self.assertEqual(self.reach({f"{SRC}/c.ts": 'import { count } from "./a.ts";\n'
                                                    "export const c: string = String(count(1));\n"}),
                         ["c.ts", "g.ts", "index.ts"], "g.ts uses c through index.ts's export *")

    def test_a_changed_type_follows_a_declaration_naming_it(self):
        changed = REACH_BASE[f"{SRC}/a.ts"].replace("{ a: number }", "{ a: number; b?: string }")
        self.assertEqual(self.reach({f"{SRC}/a.ts": changed}),
                         ["a.ts", "b.ts", "d.ts", "e.ts", "f.ts", "index.ts"],
                         "make and Body changed: b.ts and d.ts use them, e.ts d's wrap of Body, f.ts index's "
                         "made; c.ts, e2.ts and g.ts use none of it")

    def test_a_new_export_lints_nothing_but_its_file(self):
        self.assertEqual(self.reach({f"{SRC}/a.ts": REACH_BASE[f"{SRC}/a.ts"] + "export const more = 1;\n"}),
                         ["a.ts"], "index.ts re-exports only a.ts's make")

    def test_a_deprecation_reaches_what_uses_it(self):
        changed = REACH_BASE[f"{SRC}/a.ts"].replace("export const count", "/** @deprecated Use make. */\nexport const count")
        self.assertIn("c.ts", self.reach({f"{SRC}/a.ts": changed}))

    def test_a_namespace_or_dynamic_import_uses_every_export(self):
        self.repo.commit("chore: more", {f"{SRC}/n.ts": 'import * as a from "./a.ts";\nexport const n: number = a.count(1);\n',
                                         f"{SRC}/l.ts": 'export const l = async (): Promise<number> => (await import("./a.ts")).count(1);\n'})
        self.base = self.repo.git("rev-parse", "HEAD")
        changed = REACH_BASE[f"{SRC}/a.ts"].replace("{ a: number }", "{ a: number; b?: string }")
        reached = self.reach({f"{SRC}/a.ts": changed})
        self.assertIn("n.ts", reached)
        self.assertIn("l.ts", reached)

    def test_a_new_or_undeclarable_file_changes_every_export(self):
        inferred = REACH_BASE[f"{SRC}/a.ts"].replace("(a: number): number => a + 1", "(a: number) => a + 2")
        self.assertEqual(self.reach({f"{SRC}/a.ts": inferred}), ["a.ts", "b.ts", "c.ts", "d.ts", "e.ts", "f.ts", "index.ts"],
                         "count's type is inferred, so every export of a.ts may have changed: e.ts sees Body "
                         "through d.ts's wrap; g.ts uses c.ts's c, declared a number still")

    def test_a_global_lints_everything(self):
        self.assertIsNone(self.reach({f"{SRC}/a.ts": REACH_BASE[f"{SRC}/a.ts"] + "declare global { var x: number }\n"}))

    def test_a_gone_file_lints_what_changed_with_it(self):
        self.assertEqual(self.reach({f"{SRC}/c.ts": None, f"{SRC}/g.ts": None,
                                     f"{SRC}/index.ts": 'export { make as made } from "./a.ts";\n'}),
                         ["f.ts", "index.ts"], "index.ts's export * changed, so each of its exports may have")
        self.assertEqual(self.reach({f"{SRC}/e2.ts": None}), [])


CLIENT = "web/apps/web/src/a.ts"
OTHER = "web/apps/web/src/b.ts"
WORKER = "web/apps/web/src/sw.ts"
SHEET = "web/apps/web/src/poster.css"
SPEC = "web/e2e/shell.spec.ts"


class Browser(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({CLIENT: "1\n", OTHER: "1\n", WORKER: "1\n", "internal/a.go": "package a\n",
                          SHEET: ".poster { color: red; }\n", SPEC: "a\n", ".gitignore": ".dev/\n"})
        self.commit = self.repo.commit("chore: base")
        self.map = self.repo.root / ".dev" / "browser-map"
        self.map.mkdir(parents=True)
        (self.map / browser_map.COMPLETE).write_text(self.commit + "\n")
        (self.map / "modules-client.json").write_text(json.dumps({"assets/a.js": [CLIENT], "assets/b.js": [OTHER]}))
        (self.map / browser_map.SERVICE_WORKER_MODULES).write_text(json.dumps({"sw.js": [WORKER]}))
        for n, (test, tokens, scripts) in enumerate([("chromium a", [], ["assets/a.js"]),
                                                     ("chromium b", ["class:poster", "tag:div"], ["assets/b.js"]),
                                                     ("firefox unvouched", [], None)]):
            (self.map / "chromium").mkdir(exist_ok=True)
            (self.map / "chromium" / f"{n}.json").write_text(json.dumps({"test": test, "tokens": tokens,
                                                                       "scripts": scripts}))

    def scope(self, changes: dict[str, str | None]) -> list[str] | None:
        self.repo.commit("fix: a change", changes)
        with self.repo.cwd():
            return impact.browser_scope(self.map)[0]

    def test_a_client_source_runs_the_tests_that_loaded_it(self):
        self.assertEqual(self.scope({CLIENT: "2\n", "docs/a.md": "x\n"}), ["chromium a", "firefox unvouched"])

    def test_a_change_of_types_alone_runs_no_test_its_chunk_loaded(self):
        mapped = self.repo.commit("chore: a is code", {CLIENT: "export const a = 1;\n"})
        (self.map / browser_map.COMPLETE).write_text(mapped + "\n")
        self.assertEqual(self.scope({CLIENT: "export const a: number = 1;\n"}), ["firefox unvouched"])

    def test_the_change_is_read_from_the_map_s_commit_not_check_pr_s_base(self):
        self.repo.commit("fix: b, checked before", {OTHER: "2\n"})
        self.assertEqual(self.scope({CLIENT: "2\n"}), ["chromium a", "chromium b", "firefox unvouched"])

    def test_what_the_map_cannot_place_runs_every_test(self):
        for path, why in (("internal/a.go", "the server changed"), ("web/e2e/pages.ts", "every spec's helpers"),
                          (WORKER, "every page registers the service worker"),
                          ("web/apps/web/src/new.ts", "no chunk holds a new file"),
                          ("web/apps/web/vite.config.ts", "the build's configuration")):
            self.setUp()
            self.assertIsNone(self.scope({path: "changed\n"}), why)

    def test_a_stylesheet_runs_the_tests_whose_pages_drew_what_its_changed_rules_select(self):
        self.assertEqual(self.scope({SHEET: ".poster { color: blue; }\n"}), ["chromium b", "firefox unvouched"])

    def test_a_new_stylesheet_or_a_rule_for_every_page_runs_every_test(self):
        self.assertIsNone(self.scope({"web/apps/web/src/new.css": ".poster { color: red; }\n"}))
        self.setUp()
        self.assertIsNone(self.scope({SHEET: ".poster { color: red; }\n:root { --a: 1; }\n"}))

    def test_a_changed_spec_runs_whole_beside_the_tests_the_map_names(self):
        self.assertEqual(self.scope({SPEC: "b\n", CLIENT: "2\n"}), ["chromium a", "firefox unvouched", "shell.spec.ts"])
        self.setUp()
        self.assertEqual(self.scope({SPEC: None, "web/e2e/acceptance.spec.ts": "x\n"}), ["firefox unvouched"])

    def test_given_paths_and_a_tree_it_places_those_changes_as_they_differ_from_that_tree(self):
        # make check's end-to-end step asks from the tree it last passed on (e2e_reached.py).
        self.repo.commit("fix: b, since the map", {OTHER: "2\n", SHEET: ".poster { color: blue; }\n"})
        with self.repo.cwd():
            passed = self.repo.git("write-tree")
            self.repo.write({CLIENT: "2\n"})
            self.assertEqual(impact.browser_scope(self.map, paths=[CLIENT], since=passed)[0],
                             ["chromium a", "firefox unvouched"], "not b, changed before the pass")
            self.repo.write({SHEET: ".poster { color: blue; }\n"})
            self.assertEqual(impact.browser_scope(self.map, paths=[SHEET], since=passed)[0], ["firefox unvouched"],
                             "the sheet is as it was at the pass: no rule changed since")

    def test_a_map_of_no_commit_behind_head_runs_every_test(self):
        (self.map / browser_map.COMPLETE).write_text("0" * 40 + "\n")
        self.assertIsNone(self.scope({CLIENT: "2\n"}))
        (self.map / browser_map.COMPLETE).unlink()
        self.assertIsNone(self.scope({CLIENT: "3\n"}))


class Wiring(unittest.TestCase):
    MAKEFILE = (REPO / "Makefile").read_text()

    def recipe(self, target: str) -> str:
        found = re.search(rf"(?ms)^{re.escape(target)}:.*?\n\n", self.MAKEFILE)
        assert found is not None, target
        return found.group(0)

    def test_check_pr_s_whole_steps_run_what_the_change_reaches_and_everything_by_hand(self):
        lint = self.recipe("web-lint-full")
        self.assertIn("scripts/impact.py web-lint --base '$(BASE)')\" || files=.", lint)
        self.assertIn("elif [ -z '$(BASE)' ]; then npm --prefix web run --silent lint:full", lint)
        integration = self.recipe("test-integration")
        self.assertIn("scripts/impact.py integration --base '$(BASE)' $(INTEGRATION_PACKAGES))\" ||", integration)
        self.assertIn('test-integration-run INTEGRATION_PACKAGES="$$packages"', integration)

    def test_a_narrowed_browser_run_records_no_map_and_a_whole_one_does(self):
        browser = self.recipe("test-browser")
        self.assertIn('only="$$($(PYTHON) scripts/impact.py browser)" || only=all', browser)
        narrowed = browser.split("*) ", 1)[1]
        self.assertIn('ORPHEUM_E2E_ONLY="$$only" $(MAKE) --no-print-directory browser-tier', narrowed)
        self.assertNotIn("ORPHEUM_BROWSER_MAP_DIR", narrowed)
        self.assertIn('all) rm -rf $(BROWSER_MAP_DIR) && \\\n\t    ORPHEUM_BROWSER_MAP_DIR=', browser)


if __name__ == "__main__":
    unittest.main()
