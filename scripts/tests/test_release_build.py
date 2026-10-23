# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for deploy/release/build.sh (D272 §4, §7): it builds only a version tag of its own
# ABOUTME: checkout, from a clone with its own .git, and installs it with the household's install script.
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SCRIPT = REPO / "deploy" / "release" / "build.sh"
INSTALL = REPO / "internal" / "deployment" / "templates" / "install.sh"

# A docker that records its arguments, one call per line, and begins a values file the first time a
# write-deployment runs over a directory that has none, as the image does.
FAKE_DOCKER = """#!/bin/sh
printf '%s\\n' "$*" >> "$DOCKER_CALLS"
if [ "$1" = run ]; then
    for arg in "$@"; do
        case $arg in *:/var/lib/orpheum) dir=${arg%%:/var/lib/orpheum} ;; esac
    done
    [ -f "$dir/values.toml" ] || printf '[media]\\nroots = []\\n[lan]\\naddress = ""\\n' > "$dir/values.toml"
fi
"""


class Build(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)
        self.checkout = self.root / "checkout"
        (self.checkout / "deploy" / "release").mkdir(parents=True)
        shutil.copy(SCRIPT, self.checkout / "deploy" / "release" / "build.sh")
        (self.checkout / "internal" / "deployment" / "templates").mkdir(parents=True)
        shutil.copy(INSTALL, self.checkout / "internal" / "deployment" / "templates" / "install.sh")
        git = ["git", "-C", str(self.checkout), "-c", "user.name=t", "-c", "user.email=t@t"]
        subprocess.run(["git", "init", "-q", str(self.checkout)], check=True)
        subprocess.run([*git, "add", "-A"], check=True)
        subprocess.run([*git, "commit", "-q", "-m", "x"], check=True)
        subprocess.run([*git, "tag", "v0.1.0"], check=True)
        subprocess.run([*git, "tag", "nightly"], check=True)
        bin_dir = self.root / "bin"
        bin_dir.mkdir()
        (bin_dir / "docker").write_text(FAKE_DOCKER)
        (bin_dir / "docker").chmod(0o755)
        self.calls = self.root / "docker-calls"
        self.env = {**os.environ, "PATH": f"{bin_dir}:{os.environ['PATH']}", "DOCKER_CALLS": str(self.calls)}
        self.deployment = self.root / "deployment"
        self.media = self.root / "media"
        self.media.mkdir()

    def build(self, tag: str) -> subprocess.CompletedProcess:
        return subprocess.run([str(self.checkout / "deploy" / "release" / "build.sh"), tag, str(self.deployment),
                               "--media", str(self.media)],
                              env=self.env, capture_output=True, text=True, check=False)

    def docker_calls(self) -> list[list[str]]:
        return [line.split() for line in self.calls.read_text().splitlines()] if self.calls.exists() else []

    def test_a_version_tag_is_built_and_its_image_writes_the_deployment(self):
        done = self.build("v0.1.0")
        self.assertEqual(done.returncode, 0, done.stderr)
        calls = self.docker_calls()
        build = next(call for call in calls if call[0] == "build")
        run = next(call for call in calls if call[0] == "run")
        self.assertEqual(build[:2], ["build", "--file"])
        self.assertEqual(build[build.index("--tag") + 1], "orpheum:v0.1.0")
        self.assertEqual(build[build.index("--target") + 1], "runtime")
        self.assertEqual(run[0], "run")
        self.assertEqual(run[run.index("--user") + 1], f"{os.getuid()}:{os.getgid()}")
        volumes = [run[i + 1] for i, part in enumerate(run) if part == "--volume"]
        self.assertIn(f"{self.deployment}:/var/lib/orpheum", volumes)
        self.assertIn(f"{self.deployment}/run:/run/orpheum", volumes)
        self.assertEqual(run[run.index("orpheum:v0.1.0") + 1:],
                         ["write-deployment", "--deployment", "release", "--image", "orpheum:v0.1.0"])
        self.assertTrue((self.deployment / "run").is_dir())
        # Built here, so nothing is pulled; installed as a household's is, the media filled in and started.
        self.assertNotIn("pull", [call[0] for call in calls])
        self.assertIn(["compose", "up", "-d", "--remove-orphans"], calls)
        # Once installed, the build cache a week of builds has not used goes.
        prune = ["builder", "prune", "--force", "--filter", "until=168h"]
        self.assertIn(prune, calls)
        self.assertGreater(calls.index(prune), calls.index(["compose", "up", "-d", "--remove-orphans"]))
        self.assertIn(f'roots = ["{self.media}"]', (self.deployment / "values.toml").read_text())

    def deploy(self, *flags: str, answer: str = "") -> subprocess.CompletedProcess:
        return subprocess.run([str(self.checkout / "deploy" / "release" / "build.sh"), "--checkout",
                               str(self.deployment), *flags, "--media", str(self.media)],
                              env=self.env, input=answer, capture_output=True, text=True, check=False)

    def built_image_and_context(self) -> tuple[str, str]:
        build = next(call for call in self.docker_calls() if call[0] == "build")
        return build[build.index("--tag") + 1], build[-1]

    def commit(self, name: str) -> str:
        (self.checkout / name).write_text(name)
        git = ["git", "-C", str(self.checkout), "-c", "user.name=t", "-c", "user.email=t@t"]
        subprocess.run([*git, "add", "-A"], check=True)
        subprocess.run([*git, "commit", "-q", "-m", name], check=True)
        return subprocess.run([*git, "rev-parse", "HEAD"], check=True, capture_output=True, text=True).stdout[:12]

    def test_the_checkouts_tagged_commit_deploys_by_its_tag_from_a_clone_without_asking(self):
        done = self.deploy()
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertNotIn("warning", done.stderr)
        image, context = self.built_image_and_context()
        self.assertEqual(image, "orpheum:v0.1.0")
        self.assertNotEqual(context, str(self.checkout))  # a clone of the commit, never the working files
        self.assertIn(["compose", "up", "-d", "--remove-orphans"], self.docker_calls())

    def test_an_untagged_commit_is_warned_of_and_deploys_only_when_confirmed_by_a_pseudo_version(self):
        short = self.commit("later")
        refused = self.deploy(answer="n\n")
        self.assertNotEqual(refused.returncode, 0)
        self.assertIn("carries no version tag", refused.stderr)
        self.assertIn("FORCE=yes", refused.stderr)
        self.assertEqual(self.docker_calls(), [])
        done = self.deploy(answer="y\n")
        self.assertEqual(done.returncode, 0, done.stderr)
        image, _ = self.built_image_and_context()
        self.assertRegex(image, rf"^orpheum:v0\.0\.0-\d{{14}}-{short}$")

    def test_a_modified_tree_deploys_its_working_files_with_force_by_the_time_of_the_build(self):
        (self.checkout / "changed").write_text("not committed")
        done = self.deploy("--force")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertIn("uncommitted changes", done.stderr)
        image, context = self.built_image_and_context()
        self.assertRegex(image, r"^orpheum:v0\.0\.0-\d{14}-[0-9a-f]{12}-dirty$")
        self.assertEqual(context, str(self.checkout))

    def test_a_first_install_naming_no_media_is_refused_before_anything_is_built(self):
        for arguments in (["v0.1.0", str(self.deployment)], ["--checkout", str(self.deployment), "--force"]):
            done = subprocess.run([str(self.checkout / "deploy" / "release" / "build.sh"), *arguments],
                                  env=self.env, capture_output=True, text=True, check=False)
            self.assertEqual(done.returncode, 2, done.stderr)
            self.assertIn("holds no deployment yet", done.stderr)
            self.assertEqual(self.docker_calls(), [])
        # Once there is a deployment, its values file names the media, and none need be given.
        (self.deployment / "values.toml").write_text('[media]\nroots = ["/films"]\n[lan]\naddress = ""\n')
        done = subprocess.run([str(self.checkout / "deploy" / "release" / "build.sh"), "--checkout",
                               str(self.deployment), "--force"], env=self.env, capture_output=True, text=True,
                              check=False)
        self.assertEqual(done.returncode, 0, done.stderr)

    def test_a_tag_that_is_not_a_version_is_refused(self):
        done = self.build("nightly")
        self.assertNotEqual(done.returncode, 0)
        self.assertIn("not a version tag", done.stderr)
        self.assertEqual(self.docker_calls(), [])

    def test_a_version_the_checkout_has_no_tag_for_is_refused(self):
        done = self.build("v9.9.9")
        self.assertNotEqual(done.returncode, 0)
        self.assertIn("git fetch --tags", done.stderr)
        self.assertEqual(self.docker_calls(), [])


if __name__ == "__main__":
    unittest.main()
