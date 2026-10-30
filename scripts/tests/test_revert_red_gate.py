# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert-red (check_revert_red.py): which files it probes, its scopes, its claims,
# ABOUTME: and its build lines; split from test_gates.py at the 800-line limit.
import fcntl
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
from gitrepo import git_repo  # noqa: E402


class RevertRed(unittest.TestCase):
    def test_commit_types(self):
        self.assertEqual(rr.commit_type("fix(scan): keep identity"), "fix")
        self.assertTrue(rr.claims_behavior_change({"feat"}))
        self.assertTrue(rr.claims_behavior_change({None}))  # unparseable owes red
        self.assertFalse(rr.claims_behavior_change({"docs", "refactor"}))

    def test_test_files_and_scopes_per_language(self):
        self.assertTrue(rr.is_test_file("internal/core/x_test.go"))
        self.assertTrue(rr.is_test_file("internal/testutil/image.go"))
        self.assertFalse(rr.is_test_file("internal/core/testutility.go"))
        self.assertTrue(rr.is_test_file("web/packages/sdk/src/pick.test.ts"))
        self.assertFalse(rr.is_test_file("web/packages/sdk/src/pick.ts"))
        self.assertEqual(rr.scope_for("internal/core/api/x.go"), ["go", "test", "-failfast", "./internal/core/api/..."])
        self.assertEqual(rr.scope_for("web/apps/web/src/App.tsx"),
                         ["npx", "--prefix", "web", "vitest", "related", "--run", "--passWithNoTests", "--root", "web",
                          "--bail=1", "apps/web/src/App.tsx"])
        # Past its importers' tests, only the tests that read the sources by path, never the whole suite.
        wide = rr.wide_scope("web/apps/web/src/App.tsx")
        self.assertEqual(wide[:len(rr.VITEST_RUN)], rr.VITEST_RUN)
        self.assertIn("apps/web/src/idempotent.test.ts", wide)
        self.assertNotIn("apps/web/src/App.test.tsx", wide)
        self.assertIsNone(rr.scope_for("docs/x.md"))
        self.assertEqual(rr.scope_for("web/packages/sdk/src/gen/api.ts"), rr.TYPECHECK,
                         "types alone, which vitest strips: the type check, not its importers' tests")
        self.assertEqual(rr.scope_for("internal/store/sqlcgen/browse.sql.go"),
                         ["go", "test", "-failfast", "-tags", "integration", "./internal/store/..."])

    def test_a_go_file_past_its_package_reaches_its_importers_never_the_whole_suite(self):
        wide = rr.wide_scope("internal/core/playback/report.go")
        self.assertEqual(wide[:2], ["go", "test"])
        self.assertIn("./internal/core/api", wide, "api imports playback")
        self.assertIn("./internal/archtest", wide, "the arch tests read every package's sources")
        self.assertNotIn("./...", wide)
        self.assertNotIn("./internal/node/chunks", wide, "the node never imports the core's playback")
        self.assertNotIn("./internal/core/playback", wide, "its own package ran first")

    def test_a_gate_script_is_probed_by_the_test_modules_that_import_it_then_by_them_all(self):
        narrow = rr.scope_for("scripts/web_tests.py")
        self.assertEqual(narrow[:len(rr.PY_TESTS_COMMAND)], rr.PY_TESTS_COMMAND)
        self.assertIn("scripts/tests/test_web_tests.py", narrow, "imports it")
        self.assertNotIn("scripts/tests/test_hooks.py", narrow, "neither imports nor names it")
        self.assertEqual(rr.wide_scope("scripts/web_tests.py"), rr.PY_TESTS_COMMAND)
        reaching = rr.scope_for("scripts/check_stamp.py")[len(rr.PY_TESTS_COMMAND):]
        self.assertIn("scripts/tests/test_area_stamp.py", reaching, "imports it")
        self.assertIn("scripts/tests/test_web_tests.py", reaching, "imports web_tests, which imports it")
        self.assertNotIn("scripts/tests/test_revert_red_stylesheets.py", reaching)

    def test_a_new_file_of_a_package_clause_and_comments_is_cosmetic(self):
        with tempfile.TemporaryDirectory() as tree:
            Path(tree, "doc.go").write_text("// Package x does things.\n//\n/* more */\npackage x\n")
            Path(tree, "x.go").write_text("// Package x.\npackage x\n\nconst y = 1\n")
            self.assertTrue(rr.is_package_clause_only(tree, "doc.go"))
            self.assertFalse(rr.is_package_clause_only(tree, "x.go"))

    def test_every_probe_run_stops_at_its_first_red(self):
        # A red settles a probe, so each test run it makes stops at its first failure: a locale file's
        # red probe took 34 s run to its end, 6 s stopped at its first (2026-10-20). Builds, the type
        # check and vet, which report no test, are left as they are.
        for path in ("internal/core/api/x.go", "web/apps/web/src/App.tsx", "web/apps/web/src/x.css",
                     "internal/store/sqlcgen/x.go"):
            runs = [rr.scope_for(path), rr.wide_scope(path), *[a.command for a in rr.attempts_for(
                rr.Change(path, "M"), rr.scope_for(path) or [])]]
            for command in (run for run in runs if run):
                text = " ".join(command)
                if command[:2] == ["go", "test"]:
                    self.assertIn("-failfast", command, text)
                elif "vitest" in text or command[-2:] == ["run", "test"] or "--" in command:
                    self.assertIn("--bail=1", command, text)
                elif "browser-tier" in text:
                    self.assertIn("E2E_ARGS=--max-failures=1", command, text)

    def test_files_only_other_builds_compile_are_run_against_those_builds(self):
        dev = rr.other_builds("cmd/orpheum/dev_seed.go")
        # Of its own package and what its revert can reach (the source-reading arch tests, as nothing
        # imports a main package), only those the tag builds otherwise: the arch tests compile the same
        # under it and ran untagged (the caching audit's finding 6, 2026-10-28).
        self.assertEqual([a.command for a in dev], [["go", "test", "-failfast", "-tags", "dev", "./cmd/orpheum"]])
        # A package the tag does not touch is run tagged only through what the tag does touch: the
        # binary, which imports it.
        tagged = [["go", "test", "-failfast", "-tags", "dev", "./cmd/orpheum"]]
        self.assertEqual([a.command for a in rr.other_builds("internal/store/doc.go")], tagged)
        darwin = rr.other_builds("internal/store/space_darwin.go")
        self.assertEqual(darwin[-1].command, ["go", "vet", "./..."])
        self.assertEqual(darwin[-1].env, {"GOOS": "darwin"})
        self.assertTrue(darwin[-1].compile_only)
        self.assertEqual(len(rr.other_builds("internal/store/space_linux.go")), 1)
        self.assertEqual(rr.query_source("internal/store/sqlcgen/preference.sql.go"),
                         "internal/store/queries/preference.sql")
        self.assertEqual(rr.query_source("internal/store/cachegen/job.sql.go"), "internal/store/cache/queries/job.sql")
        self.assertIsNone(rr.query_source("internal/store/sqlcgen/models.go"))
        self.assertIsNone(rr.query_source("internal/store/store.go"))
        arm = rr.other_builds("internal/node/confine/arch_linux_arm64.go")
        self.assertEqual(arm[-1].env, {"GOOS": "linux", "GOARCH": "arm64"})
        self.assertTrue(arm[-1].compile_only)
        self.assertEqual([a.command for a in rr.other_builds("internal/node/confine/arch_linux_amd64.go")], tagged)
        typed = rr.other_builds("web/apps/web/src/App.tsx")
        self.assertEqual([a.command for a in typed], [["npm", "--prefix", "web", "run", "typecheck"]])
        self.assertTrue(typed[0].compile_only)

    def test_web_source_is_tried_last_in_the_browser_tier_which_alone_renders_its_styles(self):
        # With no browser map to narrow it (test_browser_map_scripts.py), whatever .dev holds.
        with mock.patch.object(rr.browser_map, "tests_loading", return_value=None):
            tier = rr.browser_tier("web/apps/web/src/sound.tsx")
        self.assertEqual(len(tier), 1)
        self.assertEqual(tier[0].command[:3], ["make", "--no-print-directory", "browser-tier"])
        self.assertTrue(tier[0].command[3].startswith("FFMPEG_DIR="))
        self.assertFalse(tier[0].compile_only)
        self.assertEqual(rr.browser_tier("internal/core/api/x.go"), [])

    def test_verified_by_needs_a_queue_item_and_an_unticked_one_is_owed_not_refused(self):
        root = Path(tempfile.mkdtemp())
        queue = root / rr.VQ_FILE
        queue.parent.mkdir(parents=True)
        queue.write_text("- [x] **VQ1** — film plays on her TV\n- [ ] **VQ2** — not yet\n")
        self.assertEqual(rr.verification_queue_problem("VQ1", str(root)), "")
        self.assertEqual(rr.verification_queue_problem("VQ2", str(root)), "", "an unticked item is owed (D378)")
        self.assertIn("is not in", rr.verification_queue_problem("VQ3", str(root)))
        self.assertIn("names no VQ", rr.verification_queue_problem("ran it by hand", str(root)))
        self.assertEqual(rr.unticked_items("VQ1; VQ2, VQ2", str(root)), ["VQ2"])


    def test_the_worktree_gets_the_installed_web_packages_and_its_own_workspace_links(self):
        source, tree = Path(tempfile.mkdtemp()), Path(tempfile.mkdtemp())
        installed = source / "web" / "node_modules"
        (installed / "vitest").mkdir(parents=True)
        (installed / "@orpheum").mkdir()
        (installed / "@orpheum" / "sdk").symlink_to("../../packages/sdk")
        (installed / ".vite").mkdir()
        (installed / "vitest" / "node_modules").mkdir()  # a package's own, reached through it
        (source / "web" / "packages" / "sdk" / "node_modules" / "typescript").mkdir(parents=True)
        rr.link_installed_packages(source, tree)
        mirrored = tree / "web" / "node_modules"
        self.assertEqual((mirrored / "vitest").resolve(), (installed / "vitest").resolve())
        self.assertEqual(os.readlink(mirrored / "@orpheum" / "sdk"), "../../packages/sdk")
        self.assertFalse((mirrored / ".vite").exists())
        self.assertTrue((tree / "web" / "packages" / "sdk" / "node_modules" / "typescript").is_symlink())
        self.assertEqual(list((installed / "vitest" / "node_modules").iterdir()), [])  # untouched

    def test_the_worktree_gets_the_working_trees_type_builds_in_place_of_its_own(self):
        # A probe's type check starts from them, incremental, where it was cold each time (2026-10-20).
        source, tree = Path(tempfile.mkdtemp()), Path(tempfile.mkdtemp())
        (source / "web" / "apps" / "web" / ".tsbuild").mkdir(parents=True)
        (source / "web" / "apps" / "web" / ".tsbuild" / "tsbuildinfo").write_text("now")
        (source / "web" / "node_modules" / "x" / ".tsbuild").mkdir(parents=True)
        (tree / "web" / "apps" / "web" / ".tsbuild").mkdir(parents=True)
        (tree / "web" / "apps" / "web" / ".tsbuild" / "stale.d.ts").write_text("old")
        self.assertEqual(rr.seed_type_builds(source, tree), 1)
        built = tree / "web" / "apps" / "web" / ".tsbuild"
        self.assertEqual((built / "tsbuildinfo").read_text(), "now")
        self.assertFalse((built / "stale.d.ts").exists())
        self.assertFalse((tree / "web" / "node_modules").exists())

    def test_a_kept_tree_is_set_to_the_working_trees_head_not_its_own(self):
        root = git_repo({"a.txt": "one\n"})
        commit = ["git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qam", "c"]
        subprocess.run(["git", "add", "-A"], cwd=root, check=True)
        subprocess.run(commit, cwd=root, check=True)
        cwd = os.getcwd()
        try:
            os.chdir(root)
            [tree] = rr.probe_trees("HEAD", 1)
            (root / "a.txt").write_text("two\n")
            subprocess.run(commit, cwd=root, check=True)
            [tree] = rr.probe_trees("HEAD", 1)
        finally:
            os.chdir(cwd)
        self.assertEqual((Path(tree) / "a.txt").read_text(), "two\n")

    def test_one_run_at_a_time_holds_the_shared_probe_trees(self):
        root = git_repo({})
        cwd = os.getcwd()
        try:
            os.chdir(root)
            with rr.hold_probe_trees():
                lock = root / ".git" / "revert-red" / "lock"
                with open(lock, "w", encoding="utf-8") as other:
                    with self.assertRaises(BlockingIOError):
                        fcntl.flock(other, fcntl.LOCK_EX | fcntl.LOCK_NB)
        finally:
            os.chdir(cwd)

    def test_a_suite_that_could_not_start_fails_rather_than_passing_as_red(self):
        self.assertEqual(rr.RUNNER_MISSING, 127)
        self.assertTrue(rr.Outcome("web/x.ts", "NO_RUNNER", "").is_failure)
        self.assertFalse(rr.Outcome("web/x.ts", "RED_TEST", "").is_failure)


class RevertRedBuildLines(unittest.TestCase):
    def test_a_file_whose_build_line_leaves_linux_out_is_compiled_for_darwin(self):
        directory = Path(tempfile.mkdtemp())
        other = directory / "shim_other.go"
        other.write_text("// SPDX-License-Identifier: AGPL-3.0-or-later\n\n//go:build !linux\n\npackage x\n")
        plain = directory / "shim.go"
        plain.write_text("package x\n")
        self.assertEqual(rr.other_builds(str(other))[-1].env, {"GOOS": "darwin"})
        self.assertEqual(rr.other_builds(str(plain)), [], "no constraint and no tagged build of its own")

    def test_a_file_the_host_never_compiles_is_judged_by_its_platforms_compile_alone(self):
        # Its revert can turn no host test red: the dependents of a _linux_arm64.go file ran 610 s
        # before the arm64 compile that went red (check-pr, 2026-10-22).
        directory = Path(tempfile.mkdtemp())
        stub = directory / "arch_linux_other.go"
        stub.write_text("//go:build linux && !amd64 && !arm64\n\npackage x\n")
        shim = directory / "pipe_other.go"
        shim.write_text("//go:build !linux\n\npackage x\n")
        plain = directory / "pipe.go"
        plain.write_text("package x\n")
        foreign = "internal/node/confine/arch_linux_s390x.go"
        for path, env in ((foreign, {"GOOS": "linux", "GOARCH": "s390x"}),
                          (str(stub), {"GOOS": "linux", "GOARCH": "riscv64"}),
                          (str(shim), {"GOOS": "darwin"})):
            attempts = rr.attempts_for(rr.Change(path, "M"), ["go", "test", "./x"])
            self.assertEqual([attempt.env for attempt in attempts], [env], path)
            self.assertTrue(all(attempt.compile_only for attempt in attempts), path)
        self.assertEqual(rr.attempts_for(rr.Change(str(plain), "M"), ["go", "test", "./x"])[0].command,
                         ["go", "test", "./x"])

    def test_a_linux_file_whose_build_line_leaves_amd64_out_is_compiled_for_riscv64(self):
        directory = Path(tempfile.mkdtemp())
        stub = directory / "arch_linux_other.go"
        stub.write_text("// SPDX-License-Identifier: AGPL-3.0-or-later\n\n//go:build linux && !amd64 && !arm64\n\n"
                        "package x\n")
        built = rr.other_builds(str(stub))
        self.assertEqual(built[-1].env, {"GOOS": "linux", "GOARCH": "riscv64"})
        self.assertTrue(built[-1].compile_only)
        self.assertEqual(len(built), 2)


if __name__ == "__main__":
    unittest.main()
