# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for py_tests.py: every module runs, a failure fails the run and is shown, a pass
# ABOUTME: prints one line, and a module sees only the environment the runner names.
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

    def test_a_module_sees_only_the_environment_the_runner_names(self):
        """Only PATH and Go's caches pass through: not a hook's GIT_DIR, an owner's flag or the caller's
        home."""
        sees = ("import os\nimport unittest\n\n\nclass T(unittest.TestCase):\n    def test_env(self):\n"
                "        self.assertEqual(sorted(os.environ), ['GIT_CONFIG_GLOBAL', 'GIT_CONFIG_NOSYSTEM', "
                "'GOCACHE', 'GOMODCACHE', 'GOPROXY', 'HOME', 'LC_ALL', 'PATH', 'PYTHONDONTWRITEBYTECODE', 'TMPDIR', "
                "'TZ'])\n"
                "        self.assertNotEqual(os.environ['HOME'], '/home/caller')\n")
        root = Path(tempfile.mkdtemp(prefix="py-tests-"))
        (root / "scripts" / "tests").mkdir(parents=True)
        (root / "scripts" / "tests" / "test_env.py").write_text(sees)
        caller = {"PATH": os.environ["PATH"], "GIT_DIR": "/nowhere/.git", "HOME": "/home/caller",
                  "ORPHEUM_ALLOW_GATE_EDIT": "1", "SOMETHING_ELSE": "x"}
        done = subprocess.run([sys.executable, str(RUNNER)], cwd=root, capture_output=True, text=True,
                              check=False, env=caller)
        self.assertEqual(done.returncode, 0, done.stdout)

    def test_a_module_builds_go_from_the_machines_caches_and_never_fetches(self):
        """A home of its own once got a fresh Go module cache each run, ~500 MB (2026-10-25)."""
        sees = ("import os\nimport unittest\n\n\nclass T(unittest.TestCase):\n    def test_go(self):\n"
                "        self.assertEqual(os.environ['GOPROXY'], 'off')\n"
                "        self.assertEqual(os.environ['GOMODCACHE'], '/caches/mod')\n"
                "        self.assertEqual(os.environ['GOCACHE'], '/caches/build')\n")
        root = Path(tempfile.mkdtemp(prefix="py-tests-"))
        (root / "scripts" / "tests").mkdir(parents=True)
        (root / "scripts" / "tests" / "test_go.py").write_text(sees)
        caller = {"PATH": os.environ["PATH"], "GOMODCACHE": "/caches/mod", "GOCACHE": "/caches/build"}
        done = subprocess.run([sys.executable, str(RUNNER)], cwd=root, capture_output=True, text=True,
                              check=False, env=caller)
        self.assertEqual(done.returncode, 0, done.stdout)

    def test_a_modules_temporary_files_are_deleted_with_its_home(self):
        """Tests that never removed their mkdtemp directories left thousands in /tmp (2026-10-28)."""
        leaks = ("import os\nimport tempfile\nimport unittest\n\n\nclass T(unittest.TestCase):\n"
                 "    def test_leak(self):\n"
                 "        made = tempfile.mkdtemp(prefix='leaked-')\n"
                 "        self.assertTrue(made.startswith(os.environ['HOME'] + os.sep), made)\n"
                 "        print(made)\n")
        root = Path(tempfile.mkdtemp(prefix="py-tests-"))
        (root / "scripts" / "tests").mkdir(parents=True)
        (root / "scripts" / "tests" / "test_leak.py").write_text(leaks)
        outside = Path(tempfile.mkdtemp(prefix="py-tests-tmp-"))
        caller = {"PATH": os.environ["PATH"], "TMPDIR": str(outside)}
        done = subprocess.run([sys.executable, str(RUNNER), "scripts/tests/test_leak.py"], cwd=root,
                              capture_output=True, text=True, check=False, env=caller)
        self.assertEqual(done.returncode, 0, done.stdout)
        self.assertEqual([path.name for path in outside.iterdir() if path.name.startswith("leaked-")], [])
        self.assertEqual(list(outside.glob("py-tests-home-*")), [], "the home went with what it held")

    def test_a_pass_is_one_line(self):
        done = self.run_in({"test_a.py": PASSES, "test_b.py": PASSES})
        self.assertEqual(done.returncode, 0)
        self.assertEqual(done.stdout.strip(), "py_tests: 2 modules, 0 failed")


if __name__ == "__main__":
    unittest.main()
