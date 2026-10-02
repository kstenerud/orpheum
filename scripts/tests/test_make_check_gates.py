# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for make check's new checks: retired names, TestArch_ citations in every file, no
# ABOUTME: decision recorded as built, the Playwright pin and eslint's alternatives; each is shown
# ABOUTME: failing on a case built to fail (D190).
import contextlib
import io
import json
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_claims as cc  # noqa: E402
import check_deps as cdeps  # noqa: E402
import check_docs as cd  # noqa: E402
from gitrepo import Repo  # noqa: E402

REPO = Path(__file__).resolve().parent.parent.parent


class PlaywrightPin(unittest.TestCase):
    LOCK = {"packages": {"node_modules/@playwright/test": {"version": "1.63.0"}}}

    def test_the_pin_must_be_the_lockfiles_version(self):
        self.assertEqual(cdeps.playwright_pin_problems("PLAYWRIGHT_VERSION := 1.63.0\n", self.LOCK), [])
        drifted = cdeps.playwright_pin_problems("PLAYWRIGHT_VERSION := 1.62.0\n", self.LOCK)
        self.assertIn("installs @playwright/test 1.63.0", drifted[0].message)
        self.assertIn("pins nothing", cdeps.playwright_pin_problems("", self.LOCK)[0].message)

    def test_the_browser_tier_and_ci_install_the_pinned_browsers(self):
        makefile = (REPO / "Makefile").read_text()
        lock = json.loads((REPO / "web/package-lock.json").read_text())
        self.assertEqual(cdeps.playwright_pin_problems(makefile, lock), [])
        self.assertRegex(makefile, r"(?m)^test-browser: browsers\b")
        self.assertRegex(makefile, r"(?m)^\t@npm --prefix web exec -- playwright install .*chromium firefox$")
        # CI installs them in the development image, which holds their system libraries (D235 §2).
        self.assertIn("run: ./dev make browsers", (REPO / ".github/workflows/ci.yml").read_text())

    def test_eslint_may_send_the_reader_only_to_an_installed_package(self):
        config = ('  ["swr", "@tanstack/react-query"],\n'
                  '  ["axios", "the generated SDK client in @orpheum/sdk (openapi-fetch)"],\n')
        problems = cdeps.lint_alternative_problems(config, {"@orpheum/sdk", "openapi-fetch"})
        self.assertEqual(len(problems), 1)
        self.assertIn("'swr' names @tanstack/react-query", problems[0].message)


class RetiredNames(unittest.TestCase):
    LIST = ("| Retired | Now | By |\n| --- | --- | --- |\n"
            "| `serve_debug_pprof` | `should_serve_debug_pprof` | D227 §6 |\n| `afterSeq` | `afterCommandSeq` | D227 |\n")

    def test_the_list_is_read_by_its_first_cell(self):
        self.assertEqual(cc.retired_names(self.LIST), ["serve_debug_pprof", "afterSeq"])
        real = cc.retired_names((REPO / cc.RETIRED_NAMES).read_text())
        self.assertIn("sourceTrack", real)
        self.assertEqual(len(real), len(set(real)))

    def test_a_retired_name_left_live_fails_and_a_longer_word_or_history_does_not(self):
        files = {
            "internal/core/a.go": 'var q = "?afterSeq=1"\n',
            "internal/core/b.go": "// should_serve_debug_pprof is the switch\n",
            "docs/contributors/agent-failures.md": "sent `afterSeq`\n",
            "docs/contributors/design/research/old.md": "afterSeq\n",
            "internal/store/migrations/0030_rename.sql": "UPDATE setting SET key = 'x' WHERE key = 'serve_debug_pprof';\n",
            "internal/store/steps_test.go": "('serve_debug_pprof', 'false')\n",
            cc.RETIRED_NAMES: self.LIST,
        }
        problems = cc.check_retired(files, cc.retired_names(self.LIST))
        self.assertEqual([p.where for p in problems], ["internal/core/a.go:1"])
        self.assertIn("uses `afterSeq`", problems[0].message)


class ArchCitationsEverywhere(unittest.TestCase):
    def test_a_comment_in_any_file_naming_a_missing_test_fails(self):
        files = {
            "internal/archtest/arch_test.go": "func TestArch_Defined(t *testing.T) {}\n",
            "internal/archtest/doc.go": "// Enforced by TestArch_Defined.\n",
            ".golangci.yml": "# see TestArch_Gone\n",
            "scripts/check_x.py": "# TestArch_Defined keeps this true\n",
            "scripts/tests/test_x.py": "'TestArch_Synthetic'\n",
            "docs/contributors/decisions/working-notes.md": "TestArch_Historic\n",
        }
        problems = cc.check_arch(files)
        self.assertEqual([(p.where, p.message.split(",")[0]) for p in problems],
                         [(".golangci.yml", "cites `TestArch_Gone`")])

    def test_a_citation_outside_the_docs_does_not_count_as_telling_anyone(self):
        files = {"internal/archtest/arch_test.go": "func TestArch_Quiet(t *testing.T) {}\n",
                 ".golangci.yml": "# TestArch_Quiet\n"}
        self.assertIn("which no doc cites", cc.check_arch(files)[0].message)

    def test_binary_files_are_not_read(self):
        repo = Repo({"a.txt": "TestArch_X\n"})
        (repo.root / "b.bin").write_bytes(b"\x00TestArch_Y")
        self.assertEqual(sorted(cc.read_text_files(repo.root, ["a.txt", "b.bin"])), ["a.txt"])


class DecisionBuiltOnArrival(unittest.TestCase):
    LOG = "# Log\n\n## D1 — One\n\n**Status:** Decided; built in M1.\n\nText.\n"

    def entry(self, status: str) -> str:
        return f"## D2 — Two\n\n**Status:** {status}\n\nText.\n"

    def test_a_decision_recorded_as_built_fails_and_one_decided_does_not(self):
        for status in ("Decided; built in M3.", "Decided, and built after the owner approved it.",
                       "Decided; §1 built in M2, §2 in M3."):
            self.assertEqual(cd.built_on_arrival(self.LOG, self.LOG + self.entry(status)), [2], status)
        for status in ("Decided.", "Decided; to be built before the first release.", "Decided; being built in M2.",
                       "Decided; not built (after Cut A).", "Recorded; nothing is built for it yet."):
            self.assertEqual(cd.built_on_arrival(self.LOG, self.LOG + self.entry(status)), [], status)

    def test_a_later_commit_may_say_built(self):
        decided = self.LOG + self.entry("Decided.")
        self.assertEqual(cd.built_on_arrival(decided, decided.replace("Decided.", "Decided; built in M3.")), [])

    def test_the_branch_check_names_the_commit_that_recorded_it(self):
        repo = Repo({cd.LIVE_LOG: self.LOG})
        base = repo.commit("docs: base")
        built = repo.commit("docs: D2", {cd.LIVE_LOG: self.LOG + self.entry("Decided; built in M3.")})
        with mock.patch.object(cd, "ROOT", repo.root), contextlib.redirect_stdout(io.StringIO()) as out, \
                contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(cd.check_branch(base), 1)
            self.assertEqual(cd.check_branch(built), 0)
        self.assertIn(f"{built[:12]} {cd.LIVE_LOG}: D2 is recorded with a Status saying it is built", out.getvalue())


if __name__ == "__main__":
    unittest.main()
