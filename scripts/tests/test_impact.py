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
        return impact.web_lint_scope(list(paths), lambda sources: ["apps/web/src/importer.tsx", *sources])[0]

    def test_a_web_source_lints_with_what_imports_it(self):
        with Repo({"web/apps/web/src/a.ts": "export {};\n"}).cwd():
            self.assertEqual(self.scope("web/apps/web/src/a.ts", "docs/a.md"),
                             ["apps/web/src/a.ts", "apps/web/src/importer.tsx"])

    def test_nothing_eslint_reads_lints_nothing(self):
        self.assertEqual(self.scope("docs/a.md", "internal/a.go", "web/apps/web/src/a.css"), [])

    def test_a_configuration_or_a_declaration_lints_everything(self):
        for path in ("web/eslint.config.js", "web/tsconfig.base.json", "web/package-lock.json",
                     "web/apps/web/src/env.d.ts"):
            self.assertEqual(self.scope(path), ["."], path)


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
