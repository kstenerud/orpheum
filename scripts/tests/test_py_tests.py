# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for py_tests.py: every module runs, a failure fails the run and is shown, and a
# ABOUTME: pass prints one line.
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

RUNNER = Path(__file__).resolve().parent.parent / "py_tests.py"
PASSES = "import unittest\n\n\nclass T(unittest.TestCase):\n    def test_ok(self):\n        pass\n"
FAILS = "import unittest\n\n\nclass T(unittest.TestCase):\n    def test_bad(self):\n        self.fail('broken')\n"


class Runner(unittest.TestCase):
    def run_in(self, modules: dict[str, str]) -> subprocess.CompletedProcess[str]:
        root = Path(tempfile.mkdtemp(prefix="py-tests-"))
        (root / "scripts" / "tests").mkdir(parents=True)
        for name, body in modules.items():
            (root / "scripts" / "tests" / name).write_text(body)
        return subprocess.run([sys.executable, str(RUNNER)], cwd=root, capture_output=True, text=True,
                              check=False)

    def test_a_failing_module_fails_the_run_and_is_shown(self):
        done = self.run_in({"test_a.py": FAILS, "test_b.py": PASSES})
        self.assertEqual(done.returncode, 1)
        self.assertIn("--- test_a.py failed ---", done.stdout)
        self.assertIn("broken", done.stdout)
        self.assertNotIn("test_b.py failed", done.stdout)
        self.assertIn("py_tests: 2 modules, 1 failed", done.stdout)

    def test_no_module_sees_the_hooks_repository(self):
        sees = ("import os\nimport unittest\n\n\nclass T(unittest.TestCase):\n    def test_env(self):\n"
                "        self.assertNotIn('GIT_DIR', os.environ)\n")
        root = Path(tempfile.mkdtemp(prefix="py-tests-"))
        (root / "scripts" / "tests").mkdir(parents=True)
        (root / "scripts" / "tests" / "test_env.py").write_text(sees)
        done = subprocess.run([sys.executable, str(RUNNER)], cwd=root, capture_output=True, text=True,
                              check=False, env={**os.environ, "GIT_DIR": "/nowhere/.git"})
        self.assertEqual(done.returncode, 0, done.stdout)

    def test_a_pass_is_one_line(self):
        done = self.run_in({"test_a.py": PASSES, "test_b.py": PASSES})
        self.assertEqual(done.returncode, 0)
        self.assertEqual(done.stdout.strip(), "py_tests: 2 modules, 0 failed")


if __name__ == "__main__":
    unittest.main()
