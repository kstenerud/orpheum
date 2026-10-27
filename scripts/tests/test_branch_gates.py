# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for the branch gates: the base check-pr compares against, trailers tied to the
# ABOUTME: commits that touched a file, Python tests and moves in test-weakening, and revert-red's
# ABOUTME: kinds, units, UNCHECKED list and Refactor-Only; each is shown failing on a case built to
# ABOUTME: fail (D190).
import json
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import branch_base as bb  # noqa: E402
import browser_map  # noqa: E402
import browser_tier_needed as browser  # noqa: E402
import tree_state  # noqa: E402
import check_revert_red as rr  # noqa: E402
import check_test_weakening as tw  # noqa: E402
import revert_red_cosmetic as cosmetic  # noqa: E402
import revert_red_units as units  # noqa: E402
from gitrepo import Repo, mock_argv  # noqa: E402

REPO = Path(__file__).resolve().parent.parent.parent
STEP = "internal/store/migrations/0026_index.sql"
OTHER_STEP = "internal/store/migrations/0025_baseline.sql"
GOLDEN = "internal/store/testdata/golden/schema-v0026.sql"


class BranchBase(unittest.TestCase):
    def repo_with_history(self, commits: int) -> tuple[Repo, list[str]]:
        repo = Repo({})
        shas = [repo.commit(f"chore: {n}", {"a.txt": f"{n}\n"}) for n in range(commits)]
        return repo, shas

    def test_the_owners_mark_comes_before_origin_main(self):
        repo, shas = self.repo_with_history(4)
        repo.git("update-ref", "refs/remotes/origin/main", shas[0])
        with repo.cwd():
            self.assertEqual(bb.resolve(None), ("origin/main", shas[0], 3))
            self.assertEqual(bb.mark(shas[2]), shas[2])
            self.assertEqual(bb.resolve(None), (bb.APPLIED_REF, shas[2], 1))

    def test_a_pass_marks_where_it_ended_and_the_next_run_starts_there(self):
        repo, shas = self.repo_with_history(5)
        repo.git("update-ref", bb.APPLIED_REF, shas[0])
        with repo.cwd():
            self.assertEqual(bb.mark_checked(shas[3]), shas[3])
            self.assertEqual(bb.resolve(None), (bb.CHECKED_REF, shas[3], 1))
            bb.mark(shas[4])  # the owner took more than was checked: the newer mark wins
            self.assertEqual(bb.resolve(None), (bb.APPLIED_REF, shas[4], 0))

    def test_the_first_pass_marks_whatever_base_it_started_from(self):
        repo, shas = self.repo_with_history(4)
        repo.git("update-ref", "refs/remotes/origin/main", shas[0])
        with repo.cwd():
            self.assertEqual(bb.mark_checked(shas[3]), shas[3], "origin/main is not a checkpoint")

    def test_a_run_from_a_named_base_marks_where_it_ended_and_the_next_run_starts_there(self):
        repo, shas = self.repo_with_history(5)
        with repo.cwd():
            bb.mark_checked(shas[1])
            self.assertEqual(bb.mark_checked(shas[4]), shas[4], "a pass from a named base moves the mark")
            self.assertEqual(bb.resolve(None), (bb.CHECKED_REF, shas[4], 0))

    def test_no_base_at_all_is_refused_rather_than_guessed(self):
        repo, _ = self.repo_with_history(1)
        with repo.cwd(), self.assertRaisesRegex(bb.BaseError, "make mark-applied"):
            bb.resolve(None)

    def test_an_implicit_base_too_far_back_is_refused_and_an_explicit_one_is_not(self):
        repo, shas = self.repo_with_history(5)
        repo.git("update-ref", bb.APPLIED_REF, shas[0])
        with repo.cwd():
            with self.assertRaisesRegex(bb.BaseError, "4 commits behind HEAD, more than 3"):
                bb.resolve(None, limit=3)
            self.assertEqual(bb.resolve(shas[0], limit=3), (shas[0], shas[0], 4))
            self.assertEqual(bb.resolve(None, limit=4)[2], 4)
            with self.assertRaisesRegex(bb.BaseError, "not a commit"):
                bb.resolve("no-such-ref")


def expand_variables(makefile: str, text: str) -> str:
    """`text` with the Makefile's simple variables expanded, `$(NAME)` and `$(NAME:%=prefix%)`."""
    values = dict(re.findall(r"(?m)^([A-Z_]+) := (.*)$", makefile))

    def expand(match: re.Match[str]) -> str:
        words = values.get(match.group(1))
        if words is None:
            return match.group(0)
        prefix = match.group(2) or ""
        return " ".join(prefix + word for word in words.split())
    return re.sub(r"\$\(([A-Z_]+)(?::%=([^%)]*)%)?\)", expand, text)


def revert_red_waits_for_the_tiers(makefile: str) -> bool:
    """Whether check-pr runs check-revert-red in a make of its own after the parallel one. The
    steps are check-pr-verbose's; check-pr runs it quietly (scripts/quiet_gate.py)."""
    recipe = re.search(r"^check-pr-verbose:.*\n((?:\t.*\n)+)", makefile, re.M)
    if recipe is None:
        return False
    body = expand_variables(makefile, recipe.group(1).replace("\\\n", " "))
    parallel = body.find("-j")
    tiers = body.find("test-integration")
    revert = body.find("check-revert-red")
    return 0 <= parallel < tiers < revert and "$(MAKE)" in body[tiers:revert]


class BrowserTier(unittest.TestCase):
    def test_what_the_tier_builds_or_runs_needs_it_and_docs_scripts_and_tests_do_not(self):
        for path in ("internal/core/playback/playback.go", "web/apps/web/src/play.tsx", "cmd/orpheum/main.go",
                     "web/e2e/play-timing.spec.ts", "go.mod", "Makefile", "deploy/dev/seed.toml"):
            self.assertFalse(browser.is_outside_the_tier(path), path)
        for path in ("docs/contributors/decisions/working-notes.md", "scripts/branch_base.py", "AGENTS.md",
                     "internal/core/playback/report_test.go", "web/apps/web/src/play.test.tsx"):
            self.assertTrue(browser.is_outside_the_tier(path), path)

    def test_a_batch_of_docs_alone_skips_the_tier_and_one_code_file_runs_it(self):
        repo = Repo({})
        base = repo.commit("docs: a", {"docs/a.md": "a\n"})
        repo.commit("docs: b", {"docs/a.md": "b\n", "scripts/x.py": "x\n"})
        with repo.cwd(), mock_argv(["browser_tier_needed.py", "--base", base]):
            self.assertEqual(browser.main(), 1)
        repo.commit("fix: c", {"internal/core/c.go": "package core\n"})
        with repo.cwd(), mock_argv(["browser_tier_needed.py", "--base", base]):
            self.assertEqual(browser.main(), 0)


class TreeState(unittest.TestCase):
    def test_an_edit_or_a_new_file_changes_the_fingerprint_and_putting_it_back_does_not(self):
        repo = Repo({})
        repo.commit("chore: a", {"a.txt": "a\n"})
        with repo.cwd():
            start = tree_state.fingerprint()
            repo.write({"a.txt": "edited\n"})
            self.assertNotEqual(tree_state.fingerprint(), start, "an edit to a tracked file")
            repo.write({"a.txt": "a\n"})
            self.assertEqual(tree_state.fingerprint(), start, "a probe that puts the file back")
            repo.write({"new.txt": "n\n"})
            self.assertNotEqual(tree_state.fingerprint(), start, "a new untracked file")
            repo.write({"new.txt": "changed\n"})
            with_new = tree_state.fingerprint()
            repo.write({"new.txt": "n\n"})
            self.assertNotEqual(tree_state.fingerprint(), with_new, "an untracked file's content")


class CheckPrWiring(unittest.TestCase):
    MAKEFILE = (REPO / "Makefile").read_text()

    def test_revert_red_runs_after_the_timing_tiers_not_beside_them(self):
        self.assertTrue(revert_red_waits_for_the_tiers(self.MAKEFILE))
        beside = ("check-pr-verbose:\n\t@$(MAKE) -j4 test-integration test-browser check-revert-red lint-commits\n")
        self.assertFalse(revert_red_waits_for_the_tiers(beside))

    def test_every_branch_gate_resolves_its_base_the_one_way(self):
        self.assertRegex(self.MAKEFILE, r"(?m)^check-pr:.*\n\t.*quiet_gate\.py .*check-pr-verbose")
        for target in ("check-pr-verbose", "check-pr-failed", "check-revert-red", "check-revert-red-failed",
                       "check-test-weakening", "check-decision-status", "lint-commits"):
            recipe = re.search(rf"^{target}:.*\n\t(.*)", self.MAKEFILE, re.M)
            self.assertIsNotNone(recipe, target)
            self.assertIn("$(RESOLVE_BASE)", recipe.group(1), target)
        self.assertRegex(self.MAKEFILE, r"(?m)^RESOLVE_BASE = .*scripts/branch_base\.py")
        self.assertRegex(self.MAKEFILE, r"(?m)^mark-applied:.*\n\t.*branch_base\.py --mark")


class WebDeps(unittest.TestCase):
    def test_a_symlinked_node_modules_is_refused_not_emptied(self):
        with tempfile.TemporaryDirectory() as scratch:
            target = Path(scratch) / "elsewhere"
            target.mkdir()
            (target / "kept").write_text("x")
            (Path(scratch) / "web").mkdir()
            (Path(scratch) / "web" / "node_modules").symlink_to(target)
            done = subprocess.run(["make", "--no-print-directory", "-f", str(REPO / "Makefile"), "web-deps"],
                                  cwd=scratch, capture_output=True, text=True, check=False)
            self.assertNotEqual(done.returncode, 0, done.stdout + done.stderr)
            self.assertIn("is a symlink", done.stdout)
            self.assertTrue((target / "kept").exists())


class TrailersTiedToCommits(unittest.TestCase):
    TEST = "internal/a/a_test.go"
    TWO = ('func TestA(t *testing.T) { t.Error("a") }\n'
           'func TestB(t *testing.T) { t.Error("b") }\n')

    def run_gate(self, repo: Repo, base: str) -> tuple[int, str]:
        with repo.cwd() as out, mock_argv(["check_test_weakening.py", "--base", base]):
            status = tw.main()
        return status, out.getvalue()

    def test_a_weakening_is_explained_only_by_a_commit_that_touched_its_file(self):
        repo = Repo({self.TEST: self.TWO, "a.go": "package a\n"})
        base = repo.commit("chore: base")
        repo.commit("test(a): drop B", {self.TEST: 'func TestA(t *testing.T) { t.Error("a") }\n'})
        repo.commit("chore: tidy\n\nTest-Weakened: B tested nothing", {"a.go": "package a\n\n"})
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 1, out)
        self.assertIn("No commit that touched these explains them:\n  internal/a/a_test.go", out)
        repo.commit("test(a): say why\n\nTest-Weakened: B tested nothing",
                    {self.TEST: 'func TestA(t *testing.T) { t.Error("a") }\n\n'})
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 0, out)

    def test_a_renamed_test_is_moved_not_removed_and_a_changed_one_is_removed(self):
        repo = Repo({self.TEST: self.TWO})
        base = repo.commit("chore: base")
        repo.commit("test(a): rename", {self.TEST: self.TWO.replace("TestB", "TestB_SaysWhatItChecks")})
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 0, out)
        self.assertIn("test 'TestB' is now internal/a/a_test.go: 'TestB_SaysWhatItChecks'", out)
        repo.commit("test(a): rename and weaken",
                    {self.TEST: self.TWO.replace("TestB", "TestB_SaysWhatItChecks").replace('t.Error("b")', "")})
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 1, out)
        self.assertIn("test 'TestB' was removed", out)

    def test_a_rename_that_reaches_into_the_body_holds_only_as_a_claim_that_checks_out(self):
        """Renaming a setting renamed the test naming it, body and all (2026-10-01)."""
        repo = Repo({self.TEST: self.TWO})
        base = repo.commit("chore: base")
        renamed = self.TWO.replace('TestB(t *testing.T) { t.Error("b") }',
                                   'TestB_NewKey(t *testing.T) { t.Error("new key") }')
        repo.commit("test(a): rename", {self.TEST: renamed})
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 1, out)
        self.assertIn("test 'TestB' was removed", out)
        claim = f'Test-Renamed: {self.TEST}: "TestB" -> "TestB_NewKey"'
        repo.commit(f"chore: say so\n\n{claim}", {"notes.txt": "the claim names its file\n"})
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 0, out)
        self.assertIn("test 'TestB' is now 'TestB_NewKey' (Test-Renamed), its checks kept", out)

    def test_a_rename_claim_that_does_not_hold_is_listed(self):
        repo = Repo({self.TEST: self.TWO})
        base = repo.commit("chore: base")
        weaker = self.TWO.replace('TestB(t *testing.T) { t.Error("b") }', "TestB_NewKey(t *testing.T) { }")
        repo.commit(f'test(a): rename\n\nTest-Renamed: {self.TEST}: "TestB" -> "TestB_NewKey"', {self.TEST: weaker})
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 1, out)
        self.assertIn("but its checks went from 1 to 0", out)
        elsewhere = Repo({self.TEST: self.TWO})
        base = elsewhere.commit("chore: base")
        renamed = self.TWO.replace('TestB(t *testing.T) { t.Error("b") }', 'TestB_NewKey(t *testing.T) { t.Error("k") }')
        elsewhere.commit(f'test(a): rename\n\nTest-Renamed: {self.TEST}: "TestB" -> "TestC"', {self.TEST: renamed})
        status, out = self.run_gate(elsewhere, base)
        self.assertEqual(status, 1, out)
        self.assertIn("no test of that name is in the file", out)

    def test_a_test_moved_to_another_file_and_changed_holds_as_a_claim_naming_that_file(self):
        """A test moved out of a file that went away, and renamed there (Phase 4.0, 2026-10-02)."""
        repo = Repo({self.TEST: self.TWO})
        base = repo.commit("chore: base")
        moved = 'func TestB_Elsewhere(t *testing.T) { t.Error("moved") }\n'
        repo.commit("test(a): move", {
            self.TEST: 'func TestA(t *testing.T) { t.Error("a") }\n', "internal/a/b_test.go": moved,
        })
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 1, out)
        self.assertIn("test 'TestB' was removed", out)
        claim = f'Test-Renamed: {self.TEST}: "TestB" -> internal/a/b_test.go: "TestB_Elsewhere"'
        repo.commit(f"chore: say so\n\n{claim}", {"notes.txt": "the claim names both files\n"})
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 0, out)
        self.assertIn("test 'TestB' is now internal/a/b_test.go: 'TestB_Elsewhere' (Test-Renamed), its checks kept",
                      out)

    def test_a_claim_naming_another_file_holds_only_where_the_test_is_with_its_checks(self):
        for claimed_file, why in [("internal/a/c_test.go", "no test of that name is in the file (internal/a/c_test.go)"),
                                  ("internal/a/b_test.go", "but its checks went from 1 to 0")]:
            repo = Repo({self.TEST: self.TWO})
            base = repo.commit("chore: base")
            repo.commit("test(a): move", {
                self.TEST: 'func TestA(t *testing.T) { t.Error("a") }\n',
                "internal/a/b_test.go": "func TestB_Elsewhere(t *testing.T) { }\n",
            })
            claim = f'Test-Renamed: {self.TEST}: "TestB" -> {claimed_file}: "TestB_Elsewhere"'
            repo.commit(f"chore: claim\n\n{claim}", {"notes.txt": "the claim\n"})
            status, out = self.run_gate(repo, base)
            self.assertEqual(status, 1, out)
            self.assertIn(why, out)

    def test_a_test_moved_to_a_new_file_takes_its_checks_with_it(self):
        repo = Repo({self.TEST: self.TWO})
        base = repo.commit("chore: base")
        repo.commit("test(a): split", {
            self.TEST: 'func TestA(t *testing.T) { t.Error("a") }\n',
            "internal/a/b_test.go": 'func TestB(t *testing.T) { t.Error("b") }\n',
        })
        status, out = self.run_gate(repo, base)
        self.assertEqual(status, 0, out)
        self.assertIn("is now internal/a/b_test.go: 'TestB'", out)


class PythonTests(unittest.TestCase):
    PATH = "scripts/tests/test_x.py"
    BEFORE = (
        "class A(unittest.TestCase):\n"
        "    def test_one(self):\n        self.assertEqual(f(1), 1)\n        self.assertTrue(g())\n"
        "    def test_two(self):\n        self.assertIn('a', h())\n"
        "class B(unittest.TestCase):\n"
        "    def test_one(self):\n        assert f(2) == 2\n"
    )

    def test_a_removed_python_test_and_its_assertions_are_found(self):
        after = self.BEFORE.replace("    def test_two(self):\n        self.assertIn('a', h())\n", "")
        whats = [w.what for w in tw.compare(self.PATH, self.BEFORE, after)]
        self.assertIn("test 'A.test_two' was removed", whats)
        self.assertIn("assertions went from 4 to 3", whats)

    def test_the_same_name_in_two_classes_is_two_tests(self):
        after = self.BEFORE.replace("class B(unittest.TestCase):\n    def test_one(self):\n        assert f(2) == 2\n", "")
        whats = [w.what for w in tw.compare(self.PATH, self.BEFORE, after)]
        self.assertIn("test 'B.test_one' was removed", whats)

    def test_a_python_skip_and_a_dropped_assertion_are_found(self):
        after = self.BEFORE.replace("    def test_two(self):\n",
                                    "    @unittest.skip('later')\n    def test_two(self):\n")
        after = after.replace("        self.assertTrue(g())\n", "        g()\n")
        whats = [w.what for w in tw.compare(self.PATH, self.BEFORE, after)]
        self.assertIn("1 new skip/only/todo", whats)
        self.assertIn("assertions went from 4 to 3", whats)
        skipped = self.BEFORE.replace("self.assertIn('a', h())", "self.skipTest('flaky')")
        self.assertIn("1 new skip/only/todo", [w.what for w in tw.compare(self.PATH, self.BEFORE, skipped)])

    def test_a_renamed_python_test_with_the_same_body_is_not_removed(self):
        after = self.BEFORE.replace("def test_two", "def test_two_finds_a")
        weakened, moves = tw.compare_with_moves(self.PATH, self.BEFORE, after)
        self.assertEqual(weakened, [])
        self.assertEqual(moves, [f"{self.PATH}: test 'A.test_two' is now {self.PATH}: 'A.test_two_finds_a', "
                                 "with the same body"])

    def test_typescript_moves_are_seen_too(self):
        before = "it('plays', () => { expect(a).toBe(1) })\nit('pauses', () => { expect(b).toBe(2) })\n"
        after = "it('plays', () => { expect(a).toBe(1) })\n"
        elsewhere = {tw.same_body_key("pauses when asked", "it('pauses when asked', () => { expect(b).toBe(2) })"):
                     "b.test.ts: 'pauses when asked'"}
        self.assertEqual(tw.compare("a.test.ts", before, after, elsewhere), [])
        self.assertEqual([w.what for w in tw.compare("a.test.ts", before, after)],
                         ["test 'pauses' was removed", "assertions went from 2 to 1"])


class RetiredNamesList(unittest.TestCase):
    LIST = ("| Retired | Now | By |\n| --- | --- | --- |\n"
            "| `serve_debug_pprof` | `should_serve_debug_pprof` | D227 §6 |\n| `afterSeq` | `afterCommandSeq` | D227 |\n")

    def test_taking_a_name_off_the_list_is_a_weakening(self):
        fewer = self.LIST.replace("| `afterSeq` | `afterCommandSeq` | D227 |\n", "")
        whats = [w.what for w in tw.retired_names_weakening(self.LIST, fewer)]
        self.assertEqual(whats, ["retired name 'afterSeq' left the list, so the tree is no longer searched for it"])
        self.assertEqual(tw.retired_names_weakening(fewer, self.LIST), [])


class RevertRedKinds(unittest.TestCase):
    def test_python_outside_the_tests_is_probed_by_them_whatever_its_name(self):
        repo = Repo({"dev": "#!/usr/bin/env python3\nprint(1)\n", ".claude/hooks/on_stop.py": "x = 1\n",
                     "deploy/run.sh": "#!/bin/sh\necho\n"})
        repo.commit("chore: base")
        with repo.cwd():
            rr.is_python.cache_clear()
            for path in ("dev", ".claude/hooks/on_stop.py", "scripts/tool.py"):
                self.assertTrue(rr.is_probed_kind(path), path)
                self.assertEqual(rr.scope_for(path), rr.PY_TESTS_COMMAND, path)
            self.assertFalse(rr.is_python("deploy/run.sh"))
            self.assertTrue(rr.is_test_file("scripts/tests/test_tool.py"))
            rr.is_python.cache_clear()

    def test_a_shell_script_is_probed_by_the_python_tests_that_name_it(self):
        repo = Repo({"deploy/run.sh": "#!/bin/sh\necho\n", "deploy/lone.sh": "#!/bin/sh\necho\n",
                     "scripts/tests/test_run.py": 'SCRIPT = "deploy/run.sh"\n',
                     "scripts/tests/test_other.py": "x = 1\n"})
        base = repo.commit("chore: base")
        repo.commit("fix: run says more", {"deploy/run.sh": "#!/bin/sh\necho more\n"})
        with repo.cwd():
            self.assertTrue(rr.is_probed_kind("deploy/run.sh"))
            self.assertEqual(rr.scope_for("deploy/run.sh"), [*rr.PY_TESTS_COMMAND, "scripts/tests/test_run.py"])
            self.assertIsNone(rr.scope_for("deploy/lone.sh"), "no test names it, so none covers it")
            self.assertEqual(rr.unchecked_behavioral_files(base, "HEAD"), [])
            self.assertTrue(rr.is_test_file("internal/testutil/testdata/fake.sh"))

    def test_css_is_probed_in_the_browser_tier_alone(self):
        css = "web/apps/web/src/play.css"
        self.assertTrue(rr.is_probed_kind(css))
        self.assertFalse(rr.is_test_file(css))
        self.assertEqual(rr.scope_for(css)[:3], ["make", "--no-print-directory", "browser-tier"])
        self.assertEqual(rr.other_builds(css), [])
        self.assertEqual(rr.browser_tier(css), [])  # the scope is already the browser tier
        self.assertFalse(rr.is_probed_kind("web/apps/web/index.css"))  # outside src/

    def test_a_migration_step_is_one_unit_with_its_checksum_line_and_golden(self):
        for schema in (STEP, rr.STORE_SCHEMA):
            self.assertTrue(rr.is_probed_kind(schema), schema)
            self.assertEqual(rr.scope_for(schema), rr.STORE_INTEGRATION, schema)
        self.assertEqual(units.companions(STEP), [units.MIGRATION_SUMS, GOLDEN])
        self.assertEqual(units.companions(rr.STORE_SCHEMA), [])
        self.assertFalse(rr.is_probed_kind("internal/store/migrations/README.md"))

    def test_only_the_steps_own_checksum_line_is_undone(self):
        base = f"{'a' * 64}  0025_baseline.sql\n"
        head = f"{'b' * 64}  0025_baseline.sql\n{'c' * 64}  0026_index.sql\n"
        self.assertEqual(units.reverted_sums(base, head, "0026_index.sql"), f"{'b' * 64}  0025_baseline.sql\n")
        self.assertEqual(units.reverted_sums(base, head, "0025_baseline.sql"),
                         f"{'a' * 64}  0025_baseline.sql\n{'c' * 64}  0026_index.sql\n")

    def test_a_probe_undoes_the_whole_unit_and_puts_it_back(self):
        repo = Repo({OTHER_STEP: "CREATE TABLE t (id INTEGER);\n",
                     units.MIGRATION_SUMS: f"{'a' * 64}  0025_baseline.sql\n"})
        base = repo.commit("chore: base")
        head = repo.commit("feat(store): an index", {
            OTHER_STEP: "CREATE TABLE t (id INTEGER, x TEXT);\n",
            STEP: "CREATE INDEX i ON t (id);\n", GOLDEN: "schema\n",
            units.MIGRATION_SUMS: f"{'b' * 64}  0025_baseline.sql\n{'c' * 64}  0026_index.sql\n"})
        tree = str(repo.root)
        (repo.root / STEP).unlink()
        for companion in units.companions(STEP):
            units.revert_companion(tree, base, STEP, companion)
        self.assertFalse((repo.root / GOLDEN).exists())
        self.assertEqual((repo.root / units.MIGRATION_SUMS).read_text(), f"{'b' * 64}  0025_baseline.sql\n")
        for path in (STEP, *units.companions(STEP)):
            units.restore(tree, head, path)
        self.assertEqual(repo.git("status", "--porcelain"), "")


class RevertRedClaims(unittest.TestCase):
    CODE = "scripts/tool.py"
    TEST = "scripts/tests/test_tool.py"

    def repo(self) -> tuple[Repo, str]:
        repo = Repo({self.CODE: "def f():\n    return 1\n",
                     self.TEST: "import unittest\nimport sys\nsys.path.insert(0, 'scripts')\nimport tool\n\n\n"
                                "class T(unittest.TestCase):\n    def test_f(self):\n        tool.f()\n"})
        return repo, repo.commit("chore: base")

    def probe(self, repo: Repo, base: str) -> rr.Outcome:
        with repo.cwd():
            return rr.probe(str(repo.root), base, "HEAD", rr.Change(self.CODE, "M"), timeout=60)

    def test_a_behaviour_change_nothing_notices_is_green_until_the_author_says_refactor(self):
        repo, base = self.repo()
        repo.commit("fix(tool): answer two\n\nIt answered one.", {self.CODE: "def f():\n    return 2\n"})
        self.assertEqual(self.probe(repo, base).verdict, "GREEN")
        repo.commit("docs: note\n\nRefactor-Only: scripts/other.py — the rename moved no behaviour",
                    {"notes.md": "x\n"})
        self.assertEqual(self.probe(repo, base).verdict, "GREEN", "the trailer names another file")
        # A made commit is never rewritten, so the claim may ride on any commit of the branch.
        repo.commit(f"docs: note\n\nRefactor-Only: {self.CODE} — the rename moved no behaviour",
                    {"notes.md": "y\n"})
        outcome = self.probe(repo, base)
        self.assertEqual((outcome.verdict, outcome.detail),
                         ("CLAIMED", "Refactor-Only: the rename moved no behaviour"))
        self.assertIn("On the author's word", rr.report([outcome]))

    def test_a_claim_on_its_commit_does_not_excuse_a_file_a_test_covers(self):
        repo, base = self.repo()
        repo.commit("fix(tool): answer two\n\nIt answered one.\n\nVerified-By: VQ99", {
            self.CODE: "def f():\n    return 2\n",
            self.TEST: (repo.root / self.TEST).read_text() + "        assert tool.f() == 2\n"})
        self.assertEqual(self.probe(repo, base).verdict, "RED_TEST",
                         "an unticked VQ item failed a file its test covers")

    def test_a_red_from_a_probe_that_fails_with_nothing_reverted_says_nothing(self):
        # Every browser-mode test failed to import in revert-red's worktrees (Vite refused .git/), so
        # each of its probes was red whatever was reverted, and web files passed as covered (A45).
        repo, base = self.repo()
        repo.commit("fix(tool): answer two\n\nIt answered one.", {
            self.CODE: "def f():\n    return 2\n",
            self.TEST: (repo.root / self.TEST).read_text() + "        assert False, 'broken here'\n"})
        outcome = self.probe(repo, base)
        self.assertEqual(outcome.verdict, "PROBE_BROKEN", outcome.detail)
        self.assertIn("broken here", outcome.detail, "the clean run's failure is kept (F8)")
        self.assertTrue(outcome.is_failure)
        self.assertIn("fix the environment", rr.report([outcome]))
        self.assertEqual(repo.git("status", "--porcelain"), "", "the reverted file is put back")

    def test_a_ticked_verification_covers_a_file_no_test_notices(self):
        repo, base = self.repo()
        repo.commit("docs: queue", {rr.VQ_FILE: "- [x] **VQ7** — seen on the television\n"})
        repo.commit("fix(tool): answer two\n\nIt answered one.\n\nVerified-By: VQ7",
                    {self.CODE: "def f():\n    return 2\n"})
        self.assertEqual(self.probe(repo, base).verdict, "CLAIMED")

    def test_every_file_a_behaviour_commit_changed_and_no_probe_undid_is_listed(self):
        repo, base = self.repo()
        repo.commit("fix(tool): answer two\n\nIt answered one.", {
            self.CODE: "def f():\n    return 2\n", "dev/seed.toml": "x = 1\n",
            self.TEST: (repo.root / self.TEST).read_text() + "        assert tool.f() == 2\n"})
        repo.commit("docs: words", {"README.md": "words\n"})
        with repo.cwd():
            owed = [c for c in rr.changed_sources(base, "HEAD")]
            rows = rr.unprobed(base, "HEAD", owed)
        self.assertEqual([(o.path, o.verdict) for o in rows], [("dev/seed.toml", "UNCHECKED")])
        self.assertIn("no probe for this kind of file (.toml)", rows[0].detail)


class RevertRedStylesheetsTogether(unittest.TestCase):
    """A skin's stylesheets are reverted at once and run once; a pass settles only what it decides."""

    OLD = "web/apps/web/src/a.css"
    CLAIMED = "web/apps/web/src/b.css"
    UNCLAIMED = "web/apps/web/src/c.css"
    ADDED = "web/apps/web/src/new.css"
    # Passes only while every sheet is as the base has it: the run saw them all reverted at once.
    # A new sheet is left as it is: its revert deletes it, which broke the run's build (2026-10-13).
    ALL_REVERTED = ["sh", "-c", f"test -e {ADDED} && grep -q old {OLD} && grep -q old {CLAIMED} "
                                f"&& grep -q old {UNCLAIMED}"]

    def repo(self) -> tuple[Repo, str]:
        repo = Repo({path: ".x { color: var(--old); }\n" for path in (self.OLD, self.CLAIMED, self.UNCLAIMED)})
        base = repo.commit("chore: base")
        repo.commit("docs: queue", {rr.VQ_FILE: "- [x] **VQ7** — seen in a browser\n"})
        repo.commit("feat(web): a skin\n\nIt had none.\n\nVerified-By: VQ7",
                    {self.OLD: ".x { color: var(--new); }\n", self.CLAIMED: ".x { color: var(--new); }\n",
                     self.ADDED: ".y { color: var(--new); }\n"})
        repo.commit("fix(web): one more\n\nIt was wrong.", {self.UNCLAIMED: ".x { color: var(--new); }\n"})
        return repo, base

    def changes(self) -> list[rr.Change]:
        return [rr.Change(self.OLD, "M"), rr.Change(self.CLAIMED, "M"), rr.Change(self.UNCLAIMED, "M"),
                rr.Change(self.ADDED, "A")]

    def test_a_pass_with_every_sheet_reverted_settles_every_sheet_and_an_unclaimed_one_is_green(self):
        repo, base = self.repo()
        with repo.cwd():
            settled = rr.stylesheets_together(str(repo.root), base, "HEAD", self.changes(), 60,
                                              self.ALL_REVERTED)
        self.assertEqual({path: outcome.verdict for path, outcome in settled.items()},
                         {self.OLD: "CLAIMED", self.CLAIMED: "CLAIMED", self.UNCLAIMED: "GREEN"})
        self.assertIn("Verified-By: VQ7", settled[self.OLD].detail)
        self.assertTrue(settled[self.UNCLAIMED].is_failure, "a sheet nothing noticed and nothing claims fails, "
                        "with no browser run of its own (D279)")
        self.assertEqual(repo.git("status", "--porcelain"), "", "every sheet is put back")

    def test_a_new_sheet_is_probed_alone_by_building_the_web_client_and_not_by_a_browser(self):
        added = rr.Change(self.ADDED, "A")
        self.assertFalse(rr.is_stylesheet(added), "a new sheet left in the batch broke its build")
        self.assertTrue(rr.is_stylesheet(rr.Change(self.OLD, "M")))
        attempts = rr.attempts_for(added, rr.scope_for(self.ADDED))
        self.assertEqual([(a.command, a.compile_only) for a in attempts], [(rr.WEB_BUILD, True)])
        changed = rr.attempts_for(rr.Change(self.OLD, "M"), rr.scope_for(self.OLD))
        # The browser-mode tests that import it first, seconds; the browser tier last (the owner, 2026-10-18).
        self.assertEqual(changed[0].command, [*rr.BROWSER_MODE_RELATED, self.OLD.removeprefix("web/")])
        self.assertIn("browser-tier", " ".join(changed[-1].command))

    def test_a_failing_run_is_halved_down_to_the_sheet_a_test_noticed_and_one_sheet_alone_is_not_batched(self):
        repo, base = self.repo()
        ran = Path(tempfile.mkdtemp()) / "ran"
        # Fails only while the unclaimed sheet is reverted; counts its runs.
        noticing = ["sh", "-c", f"echo x >> {ran}; ! grep -q old {self.UNCLAIMED}"]
        with repo.cwd():
            settled = rr.stylesheets_together(str(repo.root), base, "HEAD", self.changes(), 60, noticing)
            self.assertEqual(rr.stylesheets_together(str(repo.root), base, "HEAD",
                                                     [rr.Change(self.CLAIMED, "M")], 60, ["true"]), {})
        self.assertEqual({path: outcome.verdict for path, outcome in settled.items()},
                         {self.OLD: "CLAIMED", self.CLAIMED: "CLAIMED", self.UNCLAIMED: "RED_TEST"})
        self.assertEqual(len(ran.read_text().splitlines()), 4, "all three at once, then each: three is too few "
                         "to halve")
        self.assertEqual(repo.git("status", "--porcelain"), "")

    def test_the_browser_mode_tests_importing_the_sheets_run_before_the_tier_and_their_red_counts(self):
        # The tier alone passed app-shell.css and tokens.css, each red by the browser-mode tests that
        # import it (check-pr, 2026-10-26).
        repo, base = self.repo()
        named = Path(tempfile.mkdtemp()) / "named"
        # Red only while the unclaimed sheet is reverted, and records the sheets it was handed.
        importing = ["sh", "-c", f'echo "$@" >> {named}; ! grep -q old {self.UNCLAIMED}', "browser-mode"]
        with repo.cwd():
            settled = rr.stylesheets_together(str(repo.root), base, "HEAD", self.changes(), 60, ["true"],
                                              importing)
        self.assertEqual(settled[self.UNCLAIMED].verdict, "RED_TEST")
        self.assertEqual({settled[self.OLD].verdict, settled[self.CLAIMED].verdict}, {"CLAIMED"})
        self.assertIn("apps/web/src/c.css", named.read_text().splitlines()[0].split(),
                      "handed the reverted sheets as vitest related's paths, web/ left off")
        self.assertEqual(repo.git("status", "--porcelain"), "")

    def test_a_run_that_times_out_leaves_its_sheets_to_their_own_runs(self):
        repo, base = self.repo()
        with repo.cwd():
            self.assertEqual(rr.stylesheets_together(str(repo.root), base, "HEAD", self.changes(), 1,
                                                     ["sleep", "5"]), {})
        self.assertEqual(repo.git("status", "--porcelain"), "")


class RevertRedBrowserMap(unittest.TestCase):
    """A stylesheet's revert runs in only the browser tests the map says its changed rules can reach,
    and in none when no test's pages held what they select; a map of another commit is not read."""

    SHEETS = RevertRedStylesheetsTogether

    def map_at(self, repo: Repo, commit: str, tokens: list[str]) -> Path:
        directory = Path(tempfile.mkdtemp()) / "browser-map"
        (directory / "chromium").mkdir(parents=True)
        (directory / "chromium" / "1.json").write_text(json.dumps({"test": "chromium a.spec.ts a", "tokens": tokens}))
        (directory / "firefox").mkdir()
        (directory / "firefox" / "2.json").write_text(json.dumps({"test": "firefox b.spec.ts b", "tokens": ["tag:p"]}))
        (directory / browser_map.COMPLETE).write_text(commit + "\n")
        return directory

    def together(self, directory: Path, repo: Repo, base: str, command: list[str]) -> dict[str, rr.Outcome]:
        with mock.patch.object(browser_map, "BROWSER_MAP", directory), repo.cwd():
            return rr.stylesheets_together(str(repo.root), base, "HEAD", self.SHEETS.changes(self.SHEETS()), 60,
                                           command)

    def test_no_run_when_no_test_held_what_the_changed_rules_select(self):
        repo, base = self.SHEETS.repo(self.SHEETS())
        ran = Path(tempfile.mkdtemp()) / "ran"
        settled = self.together(self.map_at(repo, repo.git("rev-parse", "HEAD").strip(), ["tag:div", "class:y"]),
                                repo, base, ["sh", "-c", f"echo x >> {ran}; false"])
        self.assertFalse(ran.exists(), "no browser run: no test can notice these reverts")
        self.assertEqual({path: outcome.verdict for path, outcome in settled.items()},
                         {self.SHEETS.OLD: "CLAIMED", self.SHEETS.CLAIMED: "CLAIMED", self.SHEETS.UNCLAIMED: "GREEN"})
        self.assertIn("the browser map", settled[self.SHEETS.UNCLAIMED].detail)
        self.assertTrue(settled[self.SHEETS.UNCLAIMED].is_failure, "unclaimed, it still needs a test or a claim")

    def test_a_run_in_only_the_tests_whose_pages_held_them(self):
        repo, base = self.SHEETS.repo(self.SHEETS())
        ran = Path(tempfile.mkdtemp()) / "ran"
        command = ["sh", "-c", f'printf "%s\\n" "${browser_map.ONLY_THESE_TESTS}" >> {ran}; ' + self.SHEETS.ALL_REVERTED[2]]
        self.together(self.map_at(repo, repo.git("rev-parse", "HEAD").strip(), ["tag:div", "class:x"]),
                      repo, base, command)
        self.assertEqual(ran.read_text().splitlines(), ['["chromium a.spec.ts a"]'])

    def test_a_map_recorded_at_another_commit_runs_every_test(self):
        repo, base = self.SHEETS.repo(self.SHEETS())
        ran = Path(tempfile.mkdtemp()) / "ran"
        command = ["sh", "-c", f'printf "%s\\n" "${browser_map.ONLY_THESE_TESTS}" >> {ran}; ' + self.SHEETS.ALL_REVERTED[2]]
        self.together(self.map_at(repo, base, ["class:y"]), repo, base, command)
        self.assertEqual(ran.read_text().splitlines(), [""], "one run, of every test")

    def test_one_sheet_alone_is_settled_by_the_map_too(self):
        repo, base = self.SHEETS.repo(self.SHEETS())
        ran = Path(tempfile.mkdtemp()) / "ran"
        directory = self.map_at(repo, repo.git("rev-parse", "HEAD").strip(), ["class:y"])
        # Its browser-mode tests pass, as none imports it: the map then settles it with no browser run.
        with mock.patch.object(browser_map, "BROWSER_MAP", directory), repo.cwd(), \
                mock.patch.object(rr, "browser_tier_command", lambda: ["sh", "-c", f"echo x >> {ran}", "browser-tier"]), \
                mock.patch.object(rr, "browser_mode_tests", lambda _path: [rr.Attempt(["true"], "")]):
            outcome = rr.probe_unclaimed(str(repo.root), base, "HEAD", rr.Change(self.SHEETS.UNCLAIMED, "M"), 60)
        self.assertFalse(ran.exists())
        self.assertEqual((outcome.verdict, outcome.detail), ("GREEN", browser_map.NO_TEST_REACHES))


class RevertRedDocTags(unittest.TestCase):
    """A struct tag's doc text is the API document's prose: rewording it is cosmetic, nothing else is."""

    PATH = "internal/api/nodes.go"
    BEFORE = ("package api\n\ntype NodeBody struct {\n"
              "\tName        string `json:\"name\"`\n"
              "\tIsConnected bool   `doc:\"Whether it said hello\" json:\"isConnected\"`\n"
              "}\n\nfunc tag() string {\n\treturn `doc:\"a\"`\n}\n")

    def changed(self, old: str, new: str) -> str:
        assert old in self.BEFORE, old
        return self.BEFORE.replace(old, new)

    def test_only_the_doc_text_and_comments_changing_is_cosmetic(self):
        reworded = self.changed('doc:"Whether it said hello" json:"isConnected"`',
                                'doc:"Whether it is \\"connected\\" now" json:"isConnected"` '
                                '//nolint:lll // one struct tag')
        self.assertTrue(cosmetic.same_but_doc_text(self.BEFORE, reworded))

    def test_anything_else_in_the_tag_or_the_code_is_not(self):
        for old, new in (
            ('json:"isConnected"', 'json:"connected"'),
            ('json:"isConnected"`', 'json:"isConnected" pattern:"x"`'),
            ('`doc:"Whether it said hello" ', '`'),
            ('`json:"name"`', '`doc:"x" json:"name"`'),
            ('bool   `doc', 'int    `doc'),
            ('return `doc:"a"`', 'return `doc:"b"`'),
        ):
            with self.subTest(new=new):
                self.assertFalse(cosmetic.same_but_doc_text(self.BEFORE, self.changed(old, new)))

    def test_revert_red_reports_a_doc_text_change_as_cosmetic_and_a_block_comment_declines(self):
        for extra, expected in (("", True), ("/* a block comment */\n", False)):
            repo = Repo({self.PATH: self.BEFORE + extra})
            base = repo.commit("chore: base")
            repo.commit("fix(api): say what connected means\n\nIt said hello.",
                        {self.PATH: self.changed("said hello", "is connected now") + extra})
            with self.subTest(extra=extra), repo.cwd():
                self.assertEqual(cosmetic.is_cosmetic(str(repo.root), base, "HEAD", self.PATH), expected)


class RevertRedPythonDocstrings(unittest.TestCase):
    """A function's or class's docstring is cosmetic; a module's, which is --help, and code are not."""

    PATH = "scripts/tool.py"
    BEFORE = ('"""Usage: tool PATH."""\n\nimport argparse\n\n\n'
              'def where(root):\n    """Where it is kept: root/.tools."""\n    return root + "/.tools"\n\n\n'
              'class Tool:\n    """A tool."""\n\n    name = "t"\n\n\n'
              'def main():\n    return argparse.ArgumentParser(description=__doc__)\n')

    def is_cosmetic_change(self, old: str, new: str) -> bool:
        assert old in self.BEFORE, old
        repo = Repo({self.PATH: self.BEFORE})
        base = repo.commit("chore: base")
        repo.commit("fix(tool): say it\n\nSaid.", {self.PATH: self.BEFORE.replace(old, new)})
        with repo.cwd():
            return cosmetic.is_cosmetic(str(repo.root), base, "HEAD", self.PATH)

    def test_a_function_or_class_docstring_reworded_is_cosmetic(self):
        self.assertTrue(self.is_cosmetic_change("root/.tools.", "root/.tools, or the image's."))
        self.assertTrue(self.is_cosmetic_change('"""A tool."""', '"""A tool, pinned."""'))

    def test_the_module_docstring_and_code_are_not(self):
        self.assertFalse(self.is_cosmetic_change("Usage: tool PATH.", "Usage: tool [PATH]."))
        self.assertFalse(self.is_cosmetic_change('"/.tools"', '"/tools"'))
        self.assertFalse(self.is_cosmetic_change('name = "t"', 'name = "u"'))

if __name__ == "__main__":
    unittest.main()
