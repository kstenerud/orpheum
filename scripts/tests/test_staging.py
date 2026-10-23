# SPDX-License-Identifier: AGPL-3.0-or-later
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
        command = staging.write_deployment_command("orpheum-runtime:full", 1000, 1001, "devnas", has_gpu=False)
        self.assertEqual(command[:2], ["docker", "run"])
        self.assertIn("--rm", command)
        self.assertEqual(command[command.index("--user") + 1], "1000:1001")
        self.assertEqual(command[command.index("--hostname") + 1], "devnas")
        volumes = [command[i + 1] for i, part in enumerate(command) if part == "--volume"]
        self.assertEqual(volumes, [f"{staging.DIR}:{staging.MOUNT}", f"{staging.DIR / 'run'}:{staging.RUN_MOUNT}"])
        self.assertEqual(command[command.index("orpheum-runtime:full") + 1:],
                         ["write-deployment", "--deployment", "staging", "--image", "orpheum-runtime:full"])

    def test_the_gpu_is_mounted_when_the_machine_has_one(self):
        command = staging.write_deployment_command("orpheum-runtime:full", 1, 1, "devnas", has_gpu=True)
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


class Quick(unittest.TestCase):
    def test_the_binary_and_tunnel_script_go_where_the_runtime_stage_puts_them_over_the_full_image(self):
        runtime = (REPO / "deploy" / "Containerfile").read_text().split("AS runtime", 1)[1]
        self.assertIn(f"COPY --from=build /src/bin/orpheum {staging.BINARY_IN_IMAGE}\n", runtime)
        self.assertIn(f"COPY deploy/staging/tunnel.sh {staging.TUNNEL_IN_IMAGE}\n", runtime)
        self.assertEqual(staging.quick_containerfile("orpheum-runtime:full").splitlines(), [
            "FROM orpheum-runtime:full", f"COPY orpheum {staging.BINARY_IN_IMAGE}", f"COPY tunnel.sh {staging.TUNNEL_IN_IMAGE}"])

    def test_a_full_build_tags_the_image_quick_lays_over(self):
        inspected = subprocess.CompletedProcess([], 0, stdout="sha256:full\n")
        with mock.patch.object(staging.subprocess, "run", return_value=inspected) as run:
            staging.build_image()
        built = run.call_args_list[0].args[0]
        tags = [built[index + 1] for index, word in enumerate(built) if word == "--tag"]
        self.assertEqual(tags, [staging.IMAGE, staging.FULL_IMAGE])

    def test_without_a_full_image_quick_builds_nothing_and_says_so(self):
        missing = subprocess.CompletedProcess([], 1, stdout="")
        with mock.patch.object(staging.subprocess, "run", return_value=missing) as run, \
                mock.patch("sys.stderr") as stderr:
            self.assertIsNone(staging.quick_image())
        self.assertEqual(len(run.call_args_list), 1)  # the inspect, no build
        self.assertIn("run ./staging once first", "".join(call.args[0] for call in stderr.write.call_args_list))


if __name__ == "__main__":
    unittest.main()


class WhereReached(unittest.TestCase):
    # In a container the machine sees only its own Docker address, which nothing at home reaches;
    # staging printed it, and an empty https:// for a staging with no name (2026-10-23).
    def test_it_names_the_values_files_address_at_home_and_the_discovery_when_there_is_no_name(self):
        said = staging.where_reached({"lan": {"address": "192.168.1.199"}, "domain": ""}, "172.17.0.2")
        self.assertIn("http://192.168.1.199:8440", said)
        self.assertNotIn("172.17.0.2", said)
        self.assertNotIn("https://", said)
        self.assertIn("through its discovery", said)

    def test_a_name_is_reached_by_https_and_a_local_publish_by_loopback(self):
        said = staging.where_reached({"domain": "media.example.org", "http": {"publish": "127.0.0.1", "port": 18440}},
                                     "172.17.0.2")
        self.assertIn("https://media.example.org", said)
        self.assertIn("http://127.0.0.1:18440", said)
