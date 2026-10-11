# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for scripts/run_gate.py: the gate runs directly where the tools are, through
# ABOUTME: `./dev` where only a container engine is, and both hooks say how to fix a bare machine.
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / "scripts"))
import run_gate  # noqa: E402

LAUNCHER = str(REPO / "dev")


def which_only(*present: str):
    return lambda name: f"/usr/bin/{name}" if name in present else None


class GateCommand(unittest.TestCase):
    def test_a_host_with_go_and_node_runs_make_directly(self):
        self.assertEqual(run_gate.gate_command("check", {}, which_only("go", "node", "docker")),
                         ["make", "check"])

    def test_inside_the_dev_image_make_runs_directly(self):
        env = {run_gate.IN_DEV_IMAGE: "1"}
        self.assertEqual(run_gate.gate_command("check-docs", env, which_only()), ["make", "check-docs"])

    def test_a_host_missing_a_tool_runs_the_gate_through_dev(self):
        for present in (("docker",), ("go", "docker"), ("node", "podman")):
            with self.subTest(present=present):
                self.assertEqual(run_gate.gate_command("check", {}, which_only(*present)),
                                 [LAUNCHER, "make", "check"])

    def test_the_engine_dev_would_use_is_the_one_asked(self):
        env = {"ORPHEUM_DEV_ENGINE": "podman"}
        self.assertEqual(run_gate.gate_command("check", env, which_only("podman")),
                         [LAUNCHER, "make", "check"])
        with self.assertRaises(run_gate.NoWayToRun):
            run_gate.gate_command("check", env, which_only("docker"))

    def test_neither_tools_nor_an_engine_says_how_to_fix_it(self):
        with self.assertRaises(run_gate.NoWayToRun) as raised:
            run_gate.gate_command("check", {}, which_only("go"))
        message = str(raised.exception)
        self.assertIn("node not on PATH", message)
        self.assertIn("Install Docker or Podman", message)
        self.assertIn("CONTRIBUTING.md", message)

    def test_only_the_value_the_image_sets_counts(self):
        with self.assertRaises(run_gate.NoWayToRun):
            run_gate.gate_command("check", {run_gate.IN_DEV_IMAGE: "0"}, which_only())


def bare_machine() -> tuple[Path, dict[str, str]]:
    """A git repository and an environment whose PATH has git and python3 and nothing else."""
    root = Path(tempfile.mkdtemp())
    bin_dir = root / "bin"
    bin_dir.mkdir()
    for tool in ("git", "python3"):
        (bin_dir / tool).symlink_to(shutil.which(tool) if tool == "git" else sys.executable)
    env = {"PATH": str(bin_dir), "HOME": str(root), "GIT_CONFIG_GLOBAL": "/dev/null",
           "GIT_CONFIG_NOSYSTEM": "1"}
    repo = root / "repo"
    repo.mkdir()
    subprocess.run(["git", "init", "-q"], cwd=repo, env=env, check=True)
    (repo / "scripts").symlink_to(REPO / "scripts")
    return repo, env


class Hooks(unittest.TestCase):
    """Both hooks go through gate_command, so a machine with neither is told what to install."""

    def test_the_pre_commit_hook_says_how_to_fix_a_bare_machine(self):
        repo, env = bare_machine()
        done = subprocess.run([shutil.which("bash"), str(REPO / ".githooks" / "pre-commit")], cwd=repo,
                              env=env, capture_output=True, text=True, check=False)
        self.assertEqual(done.returncode, 127)
        self.assertIn("Install Docker or Podman", done.stderr)

    def test_the_stop_hook_blocks_with_how_to_fix_a_bare_machine(self):
        repo, env = bare_machine()
        (repo / ".claude" / ".cache").mkdir(parents=True)
        (repo / ".claude" / ".cache" / "check-pending-code").write_text("")
        done = subprocess.run([sys.executable, str(REPO / ".claude" / "hooks" / "on_stop.py")],
                              input="{}", env={**env, "CLAUDE_PROJECT_DIR": str(repo)},
                              capture_output=True, text=True, check=True)
        result = json.loads(done.stdout)
        self.assertEqual(result["decision"], "block")
        self.assertIn("Install Docker or Podman", result["reason"])


if __name__ == "__main__":
    unittest.main()
