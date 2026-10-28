# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for check_pr_steps.py's kept passes: a pure step whose files, base and toolchain passed
# ABOUTME: before is not run again; any change to what it reads, a failure, or another step runs it.
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_pr_steps as steps  # noqa: E402
from gitrepo import Repo  # noqa: E402


class KeptPasses(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({"web/a.ts": "export const a = 1;\n", "internal/x.go": "package x\n",
                          "docs/q.md": "- [ ] VQ1\n"})
        self.base = self.repo.commit("chore: base")
        self.repo.commit("feat: a change", {"web/a.ts": "export const a = 2;\n"})
        patched = mock.patch.object(steps, "toolchain", return_value="go1.26 v22")
        patched.start()
        self.addCleanup(patched.stop)
        with self.repo.cwd():
            steps.start(self.base)
        self.ran = self.repo.root / "ran"

    def record(self, step: str, status: int = 0) -> bool:
        """Records step with a command that notes it ran; whether it ran."""
        self.ran.unlink(missing_ok=True)
        with self.repo.cwd():
            self.assertEqual(steps.record(step, ["sh", "-c", f"touch {self.ran}; exit {status}"]), status)
        return self.ran.exists()

    def test_a_pure_step_that_passed_on_the_same_files_is_not_run_again(self):
        self.assertTrue(self.record("web-lint-full"))
        self.assertFalse(self.record("web-lint-full"), "same files, base and toolchain: the pass is kept")

    def test_a_change_to_what_it_reads_runs_it_and_one_elsewhere_does_not(self):
        self.record("web-lint-full")
        self.record("test-integration")
        self.repo.commit("fix: go", {"internal/x.go": "package x\n\nvar y = 1\n"})
        self.assertFalse(self.record("web-lint-full"), "a Go-only fix leaves the web lint's files alone")
        self.assertTrue(self.record("test-integration"), "but not the integration tier's")
        self.repo.commit("docs: tick", {"docs/q.md": "- [x] VQ1\n"})
        self.assertFalse(self.record("test-integration"), "a VQ tick changes nothing a step reads")
        self.assertFalse(self.record("web-lint-full"))

    def test_another_toolchain_or_base_runs_it(self):
        self.record("web-lint-full")
        with mock.patch.object(steps, "toolchain", return_value="go1.27 v24"):
            self.assertTrue(self.record("web-lint-full"))
        with self.repo.cwd():
            steps.start(self.repo.git("rev-parse", "HEAD").strip())
        self.assertTrue(self.record("web-lint-full"))

    def test_a_failure_is_never_kept_and_the_commit_steps_always_run(self):
        self.assertTrue(self.record("web-lint-full", status=1))
        self.assertTrue(self.record("web-lint-full"), "a failure kept nothing")
        self.assertTrue(self.record("lint-commits"))
        self.assertTrue(self.record("lint-commits"), "it reads the commits, not files: never kept")


if __name__ == "__main__":
    unittest.main()
