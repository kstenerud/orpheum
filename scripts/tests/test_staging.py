# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for `./staging` (D249 §3, D272 §3): it runs the image's write-deployment as its usage
# ABOUTME: says, brings an old staging directory forward, and keeps the logs of the run it replaces.
import importlib.machinery
import importlib.util
import re
import sys
import shutil
import subprocess
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path
from unittest import mock

REPO = Path(__file__).resolve().parents[2]
DEPLOYMENT_COMMAND = REPO / "cmd" / "orpheum" / "deployment.go"


def load_staging():
    """`./staging` as a module. It has no .py suffix, so it is loaded by path."""
    loader = importlib.machinery.SourceFileLoader("staging_launcher", str(REPO / "staging"))
    spec = importlib.util.spec_from_loader("staging_launcher", loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules["staging_launcher"] = module  # its dataclass looks itself up there
    loader.exec_module(module)
    return module


staging = load_staging()


def go_constant(name: str) -> str:
    """A string constant of cmd/orpheum/deployment.go, which the script must agree with."""
    found = re.search(rf'^\s*{name}\s*=\s*"([^"]*)"', DEPLOYMENT_COMMAND.read_text(), re.M)
    assert found, name
    return found.group(1)


class WriteDeployment(unittest.TestCase):
    def test_it_mounts_the_directory_where_the_binary_looks_for_it(self):
        self.assertEqual(staging.MOUNT, go_constant("deploymentMount"))
        self.assertEqual(staging.RUN_MOUNT, go_constant("deploymentRun"))

    def test_it_runs_as_the_owner_with_the_machine_s_name_and_names_the_image_by_its_id(self):
        command = staging.write_deployment_command("sha256:abc", 1000, 1001, "devnas", has_gpu=False)
        self.assertEqual(command[:2], ["docker", "run"])
        self.assertIn("--rm", command)
        self.assertEqual(command[command.index("--user") + 1], "1000:1001")
        self.assertEqual(command[command.index("--hostname") + 1], "devnas")
        volumes = [command[i + 1] for i, part in enumerate(command) if part == "--volume"]
        self.assertEqual(volumes, [f"{staging.DIR}:{staging.MOUNT}", f"{staging.DIR / 'run'}:{staging.RUN_MOUNT}"])
        self.assertEqual(command[command.index("sha256:abc") + 1:],
                         ["write-deployment", "--deployment", "staging", "--image", "sha256:abc"])

    def test_the_gpu_is_mounted_when_the_machine_has_one(self):
        command = staging.write_deployment_command("sha256:abc", 1, 1, "devnas", has_gpu=True)
        self.assertIn("/dev/dri:/dev/dri:ro", command)


class OldLayout(unittest.TestCase):
    def directory(self) -> Path:
        directory = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, directory)
        return directory

    def test_an_old_values_file_moves_and_says_it_is_staging(self):
        directory = self.directory()
        (directory / "staging.toml").write_text('domain = "media.orpheum-app.com"\n')
        (directory / "compose.media.yaml").write_text("{}\n")
        with mock.patch.multiple(staging, VALUES=directory / "values.toml", OLD_VALUES=directory / "staging.toml",
                                 OLD_OVERRIDE=directory / "compose.media.yaml"):
            self.assertTrue(staging.adopt_old_layout())
            self.assertFalse(staging.adopt_old_layout())
        self.assertEqual((directory / "values.toml").read_text(),
                         'deployment = "staging"\ndomain = "media.orpheum-app.com"\n')
        self.assertFalse((directory / "staging.toml").exists())
        self.assertFalse((directory / "compose.media.yaml").exists())

    def test_a_new_directory_is_left_alone(self):
        directory = self.directory()
        with mock.patch.multiple(staging, VALUES=directory / "values.toml", OLD_VALUES=directory / "staging.toml",
                                 OLD_OVERRIDE=directory / "compose.media.yaml"):
            self.assertFalse(staging.adopt_old_layout())
        self.assertEqual(list(directory.iterdir()), [])

    def test_a_media_root_missing_on_this_machine_is_named(self):
        directory = self.directory()
        text = f'[media]\nroots = ["{directory}", "/no/such/root", "relative"]\n'
        self.assertEqual(staging.missing_media(text), ["/no/such/root"])
        self.assertEqual(staging.missing_media("not = [toml"), [])


class SavedLogs(unittest.TestCase):
    def run_logs(self, output: str) -> tuple["Path | None", Path]:
        """save_logs into a fresh directory, with compose's logs answering output."""
        directory = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, directory)
        answered = subprocess.CompletedProcess([], 0, stdout=output)
        with mock.patch.object(staging, "LOGS", directory / "logs"), \
                mock.patch.object(staging.subprocess, "run", return_value=answered) as run:
            saved = staging.save_logs(datetime(2026, 10, 6, 13, 5, 9, tzinfo=timezone.utc))
        self.assertIn("logs", run.call_args.args[0])
        return saved, directory

    def test_a_deploy_keeps_the_run_it_replaces_in_a_file_named_by_when(self):
        saved, directory = self.run_logs("core-1  | a line\n")
        self.assertEqual(saved, directory / "logs" / "2026-10-06T13-05-09Z.log")
        self.assertEqual(saved.read_text(), "core-1  | a line\n")

    def test_nothing_running_keeps_no_file(self):
        saved, directory = self.run_logs("")
        self.assertIsNone(saved)
        self.assertFalse((directory / "logs").exists())


if __name__ == "__main__":
    unittest.main()
