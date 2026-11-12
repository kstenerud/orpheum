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


def docker_build(run: mock.Mock) -> list[str]:
    """The `docker build` among the commands a mocked run was given."""
    return next(call.args[0] for call in run.call_args_list if call.args[0][:2] == ["docker", "build"])


class Quick(unittest.TestCase):
    def test_the_binary_and_tunnel_script_go_where_the_runtime_stage_puts_them_over_the_full_image(self):
        runtime = (REPO / "deploy" / "Containerfile").read_text().split("AS runtime", 1)[1]
        self.assertIn(f"COPY --from=build /src/bin/orpheum {staging.BINARY_IN_IMAGE}\n", runtime)
        self.assertIn(f"COPY deploy/staging/tunnel.sh {staging.TUNNEL_IN_IMAGE}\n", runtime)
        self.assertEqual(staging.quick_containerfile("orpheum-runtime:full").splitlines(), [
            "FROM orpheum-runtime:full", f"COPY orpheum {staging.BINARY_IN_IMAGE}", f"COPY tunnel.sh {staging.TUNNEL_IN_IMAGE}"])

    def test_a_full_build_tags_the_image_quick_lays_over(self):
        inspected = subprocess.CompletedProcess([], 0, stdout="sha256:full\n")
        with mock.patch.object(staging.subprocess, "run", return_value=inspected) as run, \
                mock.patch.object(staging.shutil, "copy2"):
            staging.build_image()
        built = docker_build(run)
        tags = [built[index + 1] for index, word in enumerate(built) if word == "--tag"]
        self.assertEqual(tags, [staging.IMAGE, staging.FULL_IMAGE])

    def test_a_full_build_takes_the_binary_dev_built_in_place_of_the_build_stage(self):
        inspected = subprocess.CompletedProcess([], 0, stdout="sha256:full\n")
        with mock.patch.object(staging.subprocess, "run", return_value=inspected) as run, \
                mock.patch.object(staging.shutil, "copy2") as copied:
            staging.build_image()
        self.assertEqual(run.call_args_list[0].args[0], [str(staging.ROOT / "dev"), "make", "build"])
        built = docker_build(run)
        context = built[built.index("--build-context") + 1]
        self.assertTrue(context.startswith("build="), context)
        self.assertEqual(copied.call_args.args[1], Path(context.removeprefix("build=")) / "src" / "bin" / "orpheum")
        self.assertIn("FROM toolchain AS build", (REPO / "deploy" / "Containerfile").read_text(),
                      "the stage a named context named build takes the place of")

    def test_a_local_ffmpeg_is_the_build_context_the_runtime_image_takes_in_place_of_the_pin(self):
        runtime = (REPO / "deploy" / "Containerfile").read_text().split("AS runtime", 1)[1]
        self.assertIn("COPY --from=ffmpeg-local / /opt/orpheum/ffmpeg-local/\n", runtime)
        inspected = subprocess.CompletedProcess([], 0, stdout="sha256:full\n")
        with mock.patch.object(staging.subprocess, "run", return_value=inspected) as run, \
                mock.patch.dict(staging.os.environ, {}, clear=False), mock.patch.object(staging.shutil, "copy2"):
            staging.os.environ.pop("ORPHEUM_FFMPEG_LOCAL", None)
            staging.build_image()
        self.assertFalse(any(word.startswith("ffmpeg-local=") for word in docker_build(run)))
        local = Path(tempfile.mkdtemp())
        for name in ("ffmpeg", "ffprobe"):
            (local / name).write_text("")
            (local / name).chmod(0o755)
        with mock.patch.object(staging.subprocess, "run", return_value=inspected) as run, \
                mock.patch.dict(staging.os.environ, {"ORPHEUM_FFMPEG_LOCAL": str(local)}), mock.patch("sys.stderr"), \
                mock.patch.object(staging.shutil, "copy2"):
            staging.build_image()
        self.assertIn(f"ffmpeg-local={local.resolve()}", docker_build(run))

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


class StateDirectory(unittest.TestCase):
    """Staging's directory is outside the checkout, where a sandbox reset cannot delete it
    (agent-failures A71)."""

    def test_it_is_named_by_its_variable_else_the_state_directory_and_never_in_the_checkout(self):
        self.assertEqual(staging.state_directory({"ORPHEUM_STAGING_DIR": "/srv/stage"}), Path("/srv/stage"))
        self.assertEqual(staging.state_directory({"XDG_STATE_HOME": "/st"}), Path("/st/orpheum-staging"))
        self.assertEqual(staging.state_directory({}), Path.home() / ".local" / "state" / "orpheum-staging")
        self.assertNotIn(staging.ROOT, staging.state_directory().parents)

    def test_a_directory_left_in_the_checkout_is_named_with_the_move_and_a_running_one_stopped_first(self):
        root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, root)
        old, new = root / "checkout" / ".staging", root / "state" / "orpheum-staging"
        self.assertIsNone(staging.left_in_checkout(old, new))
        old.mkdir(parents=True)
        self.assertEqual(staging.left_in_checkout(old, new),
                         f"staging: its directory is still in the checkout, {old}; move it: mv {old} {new}")
        (old / "compose.yaml").write_text("{}\n")
        self.assertIn("stop it (./staging down) if it runs on this machine, then move it",
                      staging.left_in_checkout(old, new))
        new.mkdir(parents=True)
        self.assertIsNone(staging.left_in_checkout(old, new))


class AfterUp(unittest.TestCase):
    """A deploy ends by running the staging directory's after-up (VQ301): what else serves the
    checkout, such as a loader's copy of the web client, follows each deploy."""

    def directory_with(self, hook: "str | None") -> Path:
        directory = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, directory)
        if hook is not None:
            (directory / staging.AFTER_UP).write_text(hook)
            (directory / staging.AFTER_UP).chmod(0o755)
        return directory

    def test_the_hook_runs_from_the_checkout_and_its_success_is_the_deploy_s(self):
        directory = self.directory_with('#!/bin/sh\npwd > "$(dirname "$0")/ran"\n')
        with mock.patch("sys.stdout"):
            self.assertEqual(staging.after_up(directory), 0)
        self.assertEqual((directory / "ran").read_text().strip(), str(staging.ROOT))

    def test_a_failing_hook_fails_the_deploy_and_says_staging_is_up(self):
        directory = self.directory_with("#!/bin/sh\nexit 3\n")
        with mock.patch("sys.stdout"), mock.patch("sys.stderr") as err:
            self.assertEqual(staging.after_up(directory), 3)
        self.assertIn("staging itself is up", "".join(call.args[0] for call in err.write.call_args_list))

    def test_no_hook_ends_the_deploy_as_before(self):
        with mock.patch.object(staging.subprocess, "run") as run:
            self.assertEqual(staging.after_up(self.directory_with(None)), 0)
        run.assert_not_called()

    def test_a_deploy_ends_with_the_hook(self):
        source = (REPO / "staging").read_text().split("def up(", 1)[1].split("\ndef ", 1)[0]
        self.assertTrue(source.rstrip().endswith("return after_up(DIR)"))


class Remote(unittest.TestCase):
    # Staging moved to the staging host so the gate has the sandbox's processors (the owner, 2026-11-01:
    # "Yes teach to deploy to stagehost"); remote.toml in staging's directory names it.
    stagehost = staging.Target(Path("/home/orpheum/orpheum-staging"), "orpheum@192.168.1.16", "~/.ssh/stagehost")

    def test_no_remote_file_is_this_machine_and_one_names_the_host_its_key_and_its_directory(self):
        self.assertEqual(staging.load_target(None), staging.LOCAL)
        self.assertEqual(staging.load_target('host = "orpheum@192.168.1.16"\nidentity = "~/.ssh/stagehost"\n'
                                             'directory = "/home/orpheum/orpheum-staging"\n'), self.stagehost)

    def test_a_remote_without_a_host_or_an_absolute_directory_is_refused(self):
        for text in ('directory = "/srv/staging"\n', 'host = "h"\ndirectory = "~/staging"\n'):
            with self.assertRaises(SystemExit):
                staging.load_target(text)

    def test_a_command_runs_there_as_one_ssh_line_that_never_asks_for_a_password(self):
        ssh = self.stagehost.command(["test", "-d", "/tank/media/my films"])
        self.assertEqual(ssh[:3], ["ssh", "-o", "BatchMode=yes"])
        self.assertEqual(ssh[ssh.index("-i") + 1], str(Path("~/.ssh/stagehost").expanduser()))
        self.assertEqual(ssh[-2], "orpheum@192.168.1.16")
        self.assertEqual(ssh[-1], "test -d '/tank/media/my films'")
        self.assertEqual(staging.LOCAL.command(["docker", "ps"]), ["docker", "ps"])

    def test_compose_and_write_deployment_name_the_remote_directory(self):
        self.assertIn("--project-directory /home/orpheum/orpheum-staging ps", self.stagehost.compose("ps")[-1])
        command = staging.write_deployment_command("sha256:x", 1001, 1001, "stagehost", True, self.stagehost.directory)
        volumes = [command[i + 1] for i, part in enumerate(command) if part == "--volume"]
        self.assertEqual(volumes[:2], [f"/home/orpheum/orpheum-staging:{staging.MOUNT}",
                                       f"/home/orpheum/orpheum-staging/run:{staging.RUN_MOUNT}"])

    def test_the_remote_machine_s_user_name_and_gpu_are_read_from_one_answer(self):
        for said, gpu in (("1001 1001 stagehost gpu\n", True), ("1001 1002 stagehost\n", False)):
            answered = subprocess.CompletedProcess([], 0, stdout=said)
            with mock.patch.object(staging.subprocess, "run", return_value=answered) as run:
                machine = staging.remote_machine(self.stagehost)
            self.assertEqual(run.call_args.args[0][0], "ssh")
            self.assertEqual((machine.uid, machine.host_name, machine.has_gpu), (1001, "stagehost", gpu))
        self.assertEqual(machine.gid, 1002)

    def test_a_media_root_missing_on_the_remote_is_named_as_the_remote_says(self):
        text = '[media]\nroots = ["/tank/media/video/movies", "/tank/media/video/gone"]\n'
        self.assertEqual(staging.missing_media(text, lambda root: root.endswith("movies")),
                         ["/tank/media/video/gone"])

    def test_the_image_is_piped_from_docker_save_here_into_docker_load_there(self):
        saver = mock.Mock(stdout=mock.Mock(), **{"wait.return_value": 0})
        with mock.patch.object(staging.subprocess, "Popen", return_value=saver) as popen, \
                mock.patch.object(staging.subprocess, "run",
                                  return_value=subprocess.CompletedProcess([], 0)) as run, \
                mock.patch("sys.stdout"):
            staging.send_image("sha256:x", self.stagehost)
        self.assertEqual(popen.call_args.args[0], ["docker", "save", staging.IMAGE])
        self.assertEqual(run.call_args.args[0][-1], "docker load --quiet")
        self.assertIs(run.call_args.kwargs["stdin"], saver.stdout)
        saver.wait.return_value = 1
        with mock.patch.object(staging.subprocess, "Popen", return_value=saver), \
                mock.patch.object(staging.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)), \
                mock.patch("sys.stdout"), self.assertRaises(subprocess.CalledProcessError):
            staging.send_image("sha256:x", self.stagehost)
