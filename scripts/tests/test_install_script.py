# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for the install script a household installs a core with (D314): run against fake
# ABOUTME: docker and ip commands, it fills in the media and the address, starts it, and gives the code.
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
TEMPLATE = REPO / "internal" / "deployment" / "templates" / "install.sh"

# A docker that logs each call, begins a values file the first time a write-deployment runs over a
# directory that has none, failing then as the real one does to say what to fill in, and answers
# setup-code with a code.
FAKE_DOCKER = """#!/bin/sh
echo "docker $*" >> "$FAKE_LOG"
case "$1 $2" in
"run "*)
    for arg in "$@"; do
        case $arg in *:/var/lib/orpheum) dir=${arg%%:/var/lib/orpheum} ;; esac
    done
    if [ "$FAKE_BEGIN_FAILS" = 1 ]; then
        echo "write the values file: permission denied" >&2
        exit 1
    fi
    if [ ! -f "$dir/values.toml" ]; then
        printf '[media]\\nroots = []\\n[lan]\\naddress = ""\\n' > "$dir/values.toml"
        echo "began a release deployment. Before it can start: fill in values.toml" >&2
        exit 1
    fi
    ;;
"compose exec") echo "abcd-efgh" ;;
esac
exit 0
"""

# The routes the install asks for: IPv4's answered with an address at home, or with FAKE_NO_HOME=1
# with none; IPv6's with a public address, or with FAKE_NO_IPV6=1 with none.
FAKE_IP = """#!/bin/sh
if [ "$1" = -6 ]; then
    [ "$FAKE_NO_IPV6" = 1 ] && exit 2
    echo "2606:4700:4700::1111 from :: via fe80::1 dev eth0 src 2a0f:2000:4753:8afc::20 metric 100"
    exit 0
fi
[ "$FAKE_NO_HOME" = 1 ] && exit 0
echo "1.1.1.1 via 192.168.1.1 dev eth0 src 192.168.1.20 uid 1000"
"""


class Install(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)
        bin_dir = self.root / "bin"
        bin_dir.mkdir()
        for name, body in (("docker", FAKE_DOCKER), ("ip", FAKE_IP)):
            (bin_dir / name).write_text(body)
            (bin_dir / name).chmod(0o755)
        self.log = self.root / "log"
        self.env = {**os.environ, "PATH": f"{bin_dir}:{os.environ['PATH']}", "HOME": str(self.root),
                    "FAKE_LOG": str(self.log)}
        self.script = TEMPLATE.read_text().replace("{{.Image}}", "ghcr.io/example/orpheum:v1.2.3")
        self.dir = self.root / "orpheum"

    def install(self, *args: str, **env: str) -> subprocess.CompletedProcess:
        return subprocess.run(["sh", "-s", "--", *args], input=self.script, env={**self.env, **env},
                              capture_output=True, text=True, check=False)

    def logged(self) -> str:
        return self.log.read_text() if self.log.exists() else ""

    def test_it_fills_in_the_media_and_the_address_starts_it_and_gives_the_setup_code(self):
        films = self.root / 'my "films" \\ and more'
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir))
        self.assertEqual(done.returncode, 0, done.stderr)
        quoted = '"' + str(films).replace("\\", "\\\\").replace('"', '\\"') + '"'
        values = (self.dir / "values.toml").read_text()
        self.assertIn(f"roots = [{quoted}]", values)
        self.assertIn('address = "192.168.1.20"', values)
        self.assertIn("pull --quiet ghcr.io/example/orpheum:v1.2.3", self.logged())
        self.assertIn("compose up -d", self.logged())
        self.assertIn("abcd-efgh", done.stderr)
        self.assertIn("http://192.168.1.20:8440", done.stderr)

    def test_each_media_folder_is_served_and_one_named_again_later_is_added_once(self):
        films, shows, music = self.root / "films", self.root / "shows", self.root / "music"
        for folder in (films, shows, music):
            folder.mkdir()
        done = self.install("--media", str(films), "--media", str(shows), "--dir", str(self.dir))
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertIn(f'roots = ["{films}", "{shows}"]', (self.dir / "values.toml").read_text())
        done = self.install("--media", str(shows), "--media", str(music), "--dir", str(self.dir))
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertIn(f'roots = ["{films}", "{shows}", "{music}"]', (self.dir / "values.toml").read_text())

    def test_a_machine_with_no_address_at_home_is_told_where_to_open_in_words(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_NO_HOME="1")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertIn("http://<this machine's address>:8440", done.stderr)
        self.assertIn('address = ""', (self.dir / "values.toml").read_text())

    def test_it_refuses_with_no_media_or_a_folder_that_is_not_there(self):
        done = self.install("--dir", str(self.dir))
        self.assertEqual(done.returncode, 2)
        self.assertIn("--media", done.stderr)
        done = self.install("--media", str(self.root / "nothing here"), "--dir", str(self.root / "o"))
        self.assertEqual(done.returncode, 1)
        self.assertIn("is not a folder", done.stderr)

    def test_it_says_whether_the_machine_has_a_public_ipv6_address(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir))
        self.assertIn("a public IPv6 address: yes", done.stderr)
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_NO_IPV6="1")
        self.assertIn("a public IPv6 address: no", done.stderr)

    def test_a_deployment_that_cannot_begin_stops_and_says_why(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_BEGIN_FAILS="1")
        self.assertEqual(done.returncode, 1)
        self.assertIn("permission denied", done.stderr)
        self.assertNotIn("compose up", self.logged())

    def test_an_image_built_here_is_not_pulled(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir), "--no-pull", "--image", "orpheum:v1")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertNotIn("docker pull", self.logged())


if __name__ == "__main__":
    unittest.main()
