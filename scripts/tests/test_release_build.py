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
        self.assertIn(f'roots = ["{self.media}"]', (self.deployment / "values.toml").read_text())

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
