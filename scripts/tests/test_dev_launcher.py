# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for `./dev`, the development container launcher (D235 §3): the image tag follows
# ABOUTME: its inputs, the container gets the mounts and ports it needs, and the image's pins agree.
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / "scripts"))
import run_gate  # noqa: E402

launcher = run_gate.load_launcher()


def fake_root() -> Path:
    root = Path(tempfile.mkdtemp())
    for rel in launcher.BUILD_INPUTS:
        (root / rel).parent.mkdir(parents=True, exist_ok=True)
        (root / rel).write_text(f"contents of {rel}\n")
    return root


def pairs(args: list[str], flag: str) -> list[str]:
    return [args[i + 1] for i, a in enumerate(args) if a == flag]


class ImageTag(unittest.TestCase):
    def test_the_same_inputs_give_the_same_tag(self):
        root = fake_root()
        self.assertEqual(launcher.image_tag(root, 1000, 1000), launcher.image_tag(root, 1000, 1000))
        self.assertRegex(launcher.image_tag(root, 1000, 1000), r"^orpheum-dev:[0-9a-f]{16}$")

    def test_a_change_to_any_input_changes_the_tag(self):
        root = fake_root()
        before = launcher.image_tag(root, 1000, 1000)
        for rel in launcher.BUILD_INPUTS:
            with self.subTest(rel=rel):
                original = (root / rel).read_text()
                (root / rel).write_text(original + "changed\n")
                self.assertNotEqual(launcher.image_tag(root, 1000, 1000), before)
                (root / rel).write_text(original)

    def test_another_uid_or_gid_is_another_image(self):
        root = fake_root()
        tags = {launcher.image_tag(root, 1000, 1000), launcher.image_tag(root, 1001, 1000),
                launcher.image_tag(root, 1000, 1001)}
        self.assertEqual(len(tags), 3)

    def test_the_build_inputs_include_everything_the_containerfile_copies(self):
        text = (REPO / launcher.CONTAINERFILE).read_text()
        copied = set()
        for line in text.splitlines():
            if line.startswith("COPY ") and "--from=" not in line:
                copied.update(line.split()[1:-1])
        self.assertTrue(copied)
        self.assertLessEqual(copied, set(launcher.BUILD_INPUTS))
        for rel in launcher.BUILD_INPUTS:
            self.assertTrue((REPO / rel).is_file(), rel)

    def test_every_build_input_is_one_the_containerfile_copies_so_other_edits_reuse_the_image(self):
        """A Makefile edit rebuilt the image and its ten-minute tool layer (2026-10-01)."""
        text = (REPO / launcher.CONTAINERFILE).read_text()
        copied = {launcher.CONTAINERFILE}
        for line in text.splitlines():
            if line.startswith("COPY ") and "--from=" not in line:
                copied.update(line.split()[1:-1])
        self.assertEqual(set(launcher.BUILD_INPUTS), copied)


class Engine(unittest.TestCase):
    def test_docker_comes_first_then_podman(self):
        self.assertEqual(launcher.find_engine({}, lambda n: n in ("docker", "podman")), "docker")
        self.assertEqual(launcher.find_engine({}, lambda n: n == "podman"), "podman")

    def test_none_found_is_none(self):
        self.assertIsNone(launcher.find_engine({}, lambda n: False))

    def test_the_environment_can_name_one_and_only_that_one_is_tried(self):
        env = {"ORPHEUM_DEV_ENGINE": "podman"}
        self.assertEqual(launcher.find_engine(env, lambda n: True), "podman")
        self.assertIsNone(launcher.find_engine(env, lambda n: n == "docker"))

    def test_no_engine_prints_what_to_install(self):
        with mock.patch.object(launcher, "find_engine", return_value=None), \
                mock.patch("sys.stderr") as err:
            self.assertEqual(launcher.main([]), 127)
        self.assertIn("Install Docker", "".join(c.args[0] for c in err.write.call_args_list))


class RunArgs(unittest.TestCase):
    def setUp(self):
        self.root = Path("/home/k/orpheum")
        self.none = Path(tempfile.mkdtemp()) / "absent"

    def run_args(self, engine="docker", env=None, dri=None, gitconfig=None):
        return launcher.run_args(engine, "orpheum-dev:abc", self.root, env or {}, dri or self.none,
                                 gitconfig or self.none)

    def test_the_clone_is_mounted_at_its_own_path(self):
        self.assertIn("type=bind,src=/home/k/orpheum,dst=/home/k/orpheum", pairs(self.run_args(), "--mount"))

    def test_the_caches_are_named_volumes(self):
        mounts = pairs(self.run_args(), "--mount")
        for volume, path in launcher.VOLUMES.items():
            self.assertIn(f"type=volume,src={volume},dst={path}", mounts)
        self.assertEqual(set(launcher.VOLUMES.values()),
                         {"/home/dev/go", "/home/dev/.cache", "/home/dev/.npm"})

    def test_the_ports_are_published_on_loopback_unless_the_environment_says_otherwise(self):
        self.assertEqual(pairs(self.run_args(), "--publish"), ["127.0.0.1:8432:8432", "127.0.0.1:5173:5173"])
        lan = self.run_args(env={"ORPHEUM_DEV_PUBLISH": "0.0.0.0"})
        self.assertEqual(pairs(lan, "--publish"), ["0.0.0.0:8432:8432", "0.0.0.0:5173:5173"])

    def test_the_loop_listens_on_every_interface_inside_so_a_published_port_reaches_it(self):
        self.assertIn("DEV_LISTEN=0.0.0.0:8432", pairs(self.run_args(), "--env"))

    def test_media_is_mounted_read_only_and_only_when_named(self):
        self.assertFalse([m for m in pairs(self.run_args(), "--mount") if "readonly" in m])
        media = Path(tempfile.mkdtemp())
        mounts = pairs(self.run_args(env={"ORPHEUM_DEV_MEDIA": str(media)}), "--mount")
        self.assertIn(f"type=bind,src={media.resolve()},dst={media.resolve()},readonly", mounts)

    def test_the_dev_loop_inside_is_told_where_the_media_is_so_it_offers_it_as_a_root(self):
        self.assertNotIn("ORPHEUM_DEV_MEDIA", " ".join(pairs(self.run_args(), "--env")))
        media = Path(tempfile.mkdtemp())
        self.assertIn(f"ORPHEUM_DEV_MEDIA={media.resolve()}",
                      pairs(self.run_args(env={"ORPHEUM_DEV_MEDIA": str(media)}), "--env"))

    def test_dri_is_passed_with_its_groups_only_when_present(self):
        self.assertEqual(pairs(self.run_args(), "--device"), [])
        dri = Path(tempfile.mkdtemp())
        (dri / "renderD128").write_text("")
        args = self.run_args(dri=dri)
        self.assertEqual(pairs(args, "--device"), [str(dri)])
        self.assertEqual(pairs(args, "--group-add"), [str(os.getgid())])

    def test_the_gitconfig_is_mounted_read_only_when_there_is_one(self):
        gitconfig = Path(tempfile.mkdtemp()) / ".gitconfig"
        gitconfig.write_text("[user]\n")
        self.assertIn(f"type=bind,src={gitconfig},dst=/home/dev/.gitconfig,readonly",
                      pairs(self.run_args(gitconfig=gitconfig), "--mount"))

    def test_podman_maps_the_user_and_docker_does_not(self):
        self.assertIn("--userns=keep-id", self.run_args(engine="podman"))
        self.assertNotIn("--userns=keep-id", self.run_args())

    def test_the_container_idles_in_the_image_so_commands_exec_into_it(self):
        self.assertEqual(self.run_args()[-3:], ["orpheum-dev:abc", "sleep", "infinity"])

    def test_a_changed_configuration_is_a_changed_hash(self):
        a = launcher.config_hash(self.run_args())
        self.assertEqual(a, launcher.config_hash(self.run_args()))
        self.assertNotEqual(a, launcher.config_hash(self.run_args(env={"ORPHEUM_DEV_PUBLISH": "0.0.0.0"})))


class ExecArgs(unittest.TestCase):
    root = Path("/home/k/orpheum")

    def exec_args(self, cwd, command, env=None, tty=False):
        return launcher.exec_args("docker", "c", self.root, cwd, command, env or {}, tty)

    def test_a_command_runs_where_you_are_inside_the_clone(self):
        args = self.exec_args(self.root / "web", ["make", "check"])
        self.assertEqual(pairs(args, "--workdir"), ["/home/k/orpheum/web"])
        self.assertEqual(args[-3:], ["c", "make", "check"])

    def test_outside_the_clone_it_runs_at_the_root(self):
        self.assertEqual(pairs(self.exec_args(Path("/tmp"), []), "--workdir"), ["/home/k/orpheum"])
        self.assertEqual(pairs(self.exec_args(Path("/home/k/orpheum2"), []), "--workdir"), ["/home/k/orpheum"])

    def test_no_command_is_a_shell(self):
        self.assertEqual(self.exec_args(self.root, [])[-1], "bash")

    def test_a_terminal_only_when_there_is_one(self):
        self.assertNotIn("--tty", self.exec_args(self.root, []))
        self.assertIn("--tty", self.exec_args(self.root, [], tty=True))

    def test_only_the_terminal_and_test_capabilities_cross_over(self):
        env = {"TERM": "xterm", "ORPHEUM_TEST_UNCONTROLLED": "mkvtoolnix", "HOME": "/home/k", "PATH": "/x"}
        self.assertEqual(pairs(self.exec_args(self.root, [], env=env), "--env"),
                         ["ORPHEUM_TEST_UNCONTROLLED", "TERM"])

    def test_the_names_vite_answers_to_cross_over(self):
        """Vite refused devnas.example: the names it may answer to stayed outside (2026-10-01)."""
        env = {"ORPHEUM_DEV_ALLOWED_HOSTS": "devnas.example", "HOME": "/home/k"}
        self.assertEqual(pairs(self.exec_args(self.root, [], env=env), "--env"), ["ORPHEUM_DEV_ALLOWED_HOSTS"])


if __name__ == "__main__":
    unittest.main()
