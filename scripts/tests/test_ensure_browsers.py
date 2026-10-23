# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for ensure_browsers.py: a browser that cannot start gets its system libraries
# ABOUTME: installed on Linux, through sudo unless root, and what still fails is reported.
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import ensure_browsers  # noqa: E402

MISSING = "chromium FAILED chrome-headless-shell: error while loading shared libraries: libnspr4.so"


class FakeMachine:
    """Answers the probe with each answer in turn, and records every command run."""

    def __init__(self, probes, install_code=0):
        self.probes = list(probes)
        self.install_code = install_code
        self.commands = []

    def __call__(self, argv):
        self.commands.append(argv)
        if argv[0] == "node":
            out = self.probes.pop(0)
            return subprocess.CompletedProcess(argv, 1 if out else 0, out, "")
        return subprocess.CompletedProcess(argv, self.install_code, "", "apt failed" if self.install_code else "")

    def installs(self):
        return [c for c in self.commands if "install-deps" in c]


class EnsureBrowsers(unittest.TestCase):
    def ensure(self, machine, system="Linux", is_root=False):
        said = []
        code = ensure_browsers.ensure(machine, system, is_root, said.append)
        return code, said

    def test_browsers_that_start_install_nothing(self):
        machine = FakeMachine([""])
        code, _ = self.ensure(machine)
        self.assertEqual(code, 0)
        self.assertEqual(machine.installs(), [])

    def test_a_browser_missing_its_libraries_gets_them_through_sudo_and_starts(self):
        machine = FakeMachine([MISSING, ""])
        code, said = self.ensure(machine)
        self.assertEqual(code, 0, said)
        self.assertEqual(machine.installs(), [["sudo", "-n", "npx", "--no-install", "playwright", "install-deps",
                                               "chromium", "firefox"]])
        self.assertEqual(sum(c[0] == "node" for c in machine.commands), 2)

    def test_root_installs_without_sudo(self):
        machine = FakeMachine([MISSING, ""])
        code, _ = self.ensure(machine, is_root=True)
        self.assertEqual(code, 0)
        self.assertEqual(machine.installs()[0][0], "npx")

    def test_a_failed_install_fails_and_names_the_command(self):
        machine = FakeMachine([MISSING], install_code=1)
        code, said = self.ensure(machine)
        self.assertEqual(code, 1)
        self.assertTrue(any("install-deps" in line for line in said), said)

    def test_a_browser_that_still_cannot_start_fails(self):
        machine = FakeMachine([MISSING, MISSING])
        code, said = self.ensure(machine)
        self.assertEqual(code, 1)
        self.assertTrue(any("still cannot start" in line for line in said), said)

    def test_off_linux_nothing_is_installed(self):
        machine = FakeMachine([MISSING])
        code, _ = self.ensure(machine, system="Darwin")
        self.assertEqual(code, 1)
        self.assertEqual(machine.installs(), [])


if __name__ == "__main__":
    unittest.main()
