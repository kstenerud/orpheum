# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for the branch gates: the base check-pr compares against, trailers tied to the
# ABOUTME: commits that touched a file, Python tests and moves in test-weakening, and revert-red's
# ABOUTME: kinds, units, UNCHECKED list and Refactor-Only; each is shown failing on a case built to
# ABOUTME: fail (D190).
import re
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import branch_base as bb  # noqa: E402
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
    """Whether check-pr runs check-revert-red in a make of its own after the parallel one."""
    recipe = re.search(r"^check-pr:.*\n((?:\t.*\n)+)", makefile, re.M)
    if recipe is None:
        return False
    body = expand_variables(makefile, recipe.group(1).replace("\\\n", " "))
    parallel = body.find("-j")
    tiers = body.find("test-integration")
    revert = body.find("check-revert-red")
    return 0 <= parallel < tiers < revert and "$(MAKE)" in body[tiers:revert]


class CheckPrWiring(unittest.TestCase):
    MAKEFILE = (REPO / "Makefile").read_text()

    def test_revert_red_runs_after_the_timing_tiers_not_beside_them(self):
        self.assertTrue(revert_red_waits_for_the_tiers(self.MAKEFILE))
        beside = ("check-pr:\n\t@$(MAKE) -j4 test-integration test-browser check-revert-red lint-commits\n")
        self.assertFalse(revert_red_waits_for_the_tiers(beside))

    def test_every_branch_gate_resolves_its_base_the_one_way(self):
        for target in ("check-pr", "check-pr-failed", "check-revert-red", "check-revert-red-failed",
                       "check-test-weakening", "check-decision-status", "lint-commits"):
            recipe = re.search(rf"^{target}:.*\n\t(.*)", self.MAKEFILE, re.M)
            self.assertIsNotNone(recipe, target)
            self.assertIn("$(RESOLVE_BASE)", recipe.group(1), target)
        self.assertRegex(self.MAKEFILE, r"(?m)^RESOLVE_BASE = .*scripts/branch_base\.py")
        self.assertRegex(self.MAKEFILE, r"(?m)^mark-applied:.*\n\t.*branch_base\.py --mark")


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
        repo.commit(f"docs: note\n\nRefactor-Only: {self.CODE} — the rename moved no behaviour",
                    {"notes.md": "x\n"})
        self.assertEqual(self.probe(repo, base).verdict, "GREEN", "the trailer's commit did not touch the file")
        repo.commit(f"refactor(tool): name\n\nRefactor-Only: {self.CODE} — the rename moved no behaviour",
                    {self.CODE: "def f():\n    return 2\n\n"})
        outcome = self.probe(repo, base)
        self.assertEqual((outcome.verdict, outcome.detail),
                         ("CLAIMED", "Refactor-Only: the rename moved no behaviour"))
        self.assertIn("On the author's word", rr.report([outcome]))

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


if __name__ == "__main__":
    unittest.main()
