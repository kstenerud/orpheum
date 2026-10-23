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
# directory that has none, failing then as the real one does to say what to fill in, answers
# setup-code with a code, and hands over NVIDIA's GPUs as FAKE_NVIDIA says. Its images are
# FAKE_IMAGES' lines, "<id> <repository>:<tag>"; removing one named in FAKE_IN_USE fails.
FAKE_DOCKER = """#!/bin/sh
echo "docker $*" >> "$FAKE_LOG"
case "$1 $2" in
"image inspect")
    eval "name=\\${$#}"
    printf '%s\\n' "$FAKE_IMAGES" | awk -v n="$name" '$2 == n { print $1; found = 1 } END { exit !found }'
    exit ;;
"image ls")
    eval "repository=\\${$#}"
    printf '%s\\n' "$FAKE_IMAGES" | awk -v r="$repository" 'index($2, r ":") == 1'
    exit 0 ;;
"image rm")
    case " $FAKE_IN_USE " in *" $3 "*) exit 1 ;; esac
    exit 0 ;;
esac
case "$*" in
# Docker's security options, rootless among them with FAKE_ROOTLESS=1.
info)
    printf ' Security Options:\n  seccomp\n   Profile: builtin\n'
    [ "$FAKE_ROOTLESS" = 1 ] && printf '  rootless\n  cgroupns\n'
    printf ' Kernel Version: 6.14.0\n'
    exit 0 ;;
# A trial of handing over NVIDIA's GPUs: FAKE_NVIDIA names the one way that works, if any.
*"--entrypoint true --device nvidia.com/gpu=all"*) [ "$FAKE_NVIDIA" = cdi ]; exit ;;
*"--entrypoint true --gpus driver=nvidia,count=all"*) [ "$FAKE_NVIDIA" = driver ]; exit ;;
esac
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
"compose exec")
    # The admin made and the core paired at the terminal: what each was given on standard input.
    case "$*" in
    *" reset-admin "*) cat > "$FAKE_LOG.reset-admin"; echo "$*" >> "$FAKE_LOG.reset-admin"
        [ "$FAKE_PASSWORD_REFUSED" = 1 ] && { echo "the password is too short" >&2; exit 1; }
        exit 0 ;;
    *" pair "*) cat > "$FAKE_LOG.pair"; echo "paired: realm the-smiths"; exit 0 ;;
    esac
    if [ "$FAKE_SET_UP" = 1 ]; then
        echo "setup is complete: an admin exists; sign in instead"
        exit 1
    fi
    echo "abcd-efgh" ;;
esac
exit 0
"""

# The routes and addresses the install asks for: IPv4's route answered with an address at home, or
# with FAKE_NO_HOME=1 with none; IPv6's route with a public address, or with FAKE_NO_IPV6=1 with none,
# and then eth0's public address only with FAKE_IPV6_ADDRESS=1.
FAKE_IP = """#!/bin/sh
case "$*" in
"-6 route get "*)
    [ "$FAKE_NO_IPV6" = 1 ] && exit 2
    echo "2606:4700:4700::1111 from :: via fe80::1 dev eth0 src 2a0f:2000:4753:8afc::20 metric 100" ;;
"-6 addr show dev eth0 scope global")
    [ "$FAKE_IPV6_ADDRESS" = 1 ] && echo "    inet6 2a0f:2000:4753:8afc::47f/128 scope global dynamic" ;;
"-4 route show default") echo "default via 192.168.1.1 dev eth0 proto static" ;;
*)
    [ "$FAKE_NO_HOME" = 1 ] && exit 0
    echo "1.1.1.1 via 192.168.1.1 dev eth0 src 192.168.1.20 uid 1000" ;;
esac
exit 0
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
        # The kernel's settings the script reads, the test's own: forwarding on, as Docker makes it.
        self.proc_sys = self.root / "proc-sys"
        for setting, value in (("all/forwarding", "1"), ("eth0/accept_ra", "1")):
            (self.proc_sys / "net/ipv6/conf" / setting).parent.mkdir(parents=True, exist_ok=True)
            (self.proc_sys / "net/ipv6/conf" / setting).write_text(value + "\n")
        self.script = (TEMPLATE.read_text().replace("{{.Image}}", "ghcr.io/example/orpheum:v1.2.3")
                       .replace("procSys=/proc/sys", f"procSys={self.proc_sys}")
                       .replace("lsmList=/sys/kernel/security/lsm", f"lsmList={self.root / 'lsm'}")
                       .replace("nvidiaControl=/dev/nvidiactl", f"nvidiaControl={self.root / 'nvidiactl'}")
                       .replace("ttyIn=/dev/tty", f"ttyIn={self.root / 'terminal-in'}")
                       .replace("ttyOut=/dev/tty", f"ttyOut={self.root / 'terminal-out'}"))
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

    def test_an_ipv6_address_with_no_route_out_while_forwarding_is_named_with_its_fix(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_NO_IPV6="1", FAKE_IPV6_ADDRESS="1")
        self.assertIn("a public IPv6 address: yes, but no route out by it", done.stderr)
        self.assertIn("net.ipv6.conf.eth0.accept_ra = 2", done.stderr)
        (self.proc_sys / "net/ipv6/conf/eth0/accept_ra").write_text("2\n")
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_NO_IPV6="1", FAKE_IPV6_ADDRESS="1")
        self.assertIn("a public IPv6 address: no", done.stderr)

    def test_landlock_is_yes_or_no_from_the_kernels_list_and_unknown_where_it_cannot_be_read(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir))
        self.assertIn("Landlock: cannot be seen from here", done.stderr)
        (self.root / "lsm").write_text("lockdown,capability,landlock,yama,apparmor\n")
        self.assertIn("Landlock: yes", self.install("--dir", str(self.dir)).stderr)
        (self.root / "lsm").write_text("lockdown,capability,yama,apparmor\n")
        self.assertIn("Landlock: no", self.install("--dir", str(self.dir)).stderr)

    def at_the_terminal(self, *answers: str) -> None:
        """The person at the terminal, who answers each question in turn."""
        (self.root / "terminal-in").write_text("".join(answer + "\n" for answer in answers))
        (self.root / "terminal-out").write_text("")

    # D334 §2, §3: the first admin is made at the terminal, and the core paired by the account's key
    # with the password just given, each only on standard input; then both addresses are named.
    def test_at_a_terminal_it_makes_the_admin_pairs_the_core_and_names_both_addresses(self):
        films = self.root / "films"
        films.mkdir()
        self.at_the_terminal("karl", "a-long-password", "a-long-password", "abcd-efgh-jkmn-pqrs")
        done = self.install("--media", str(films), "--dir", str(self.dir))
        self.assertEqual(done.returncode, 0, done.stderr)
        made = Path(f"{self.log}.reset-admin").read_text()
        self.assertTrue(made.startswith("a-long-password\n"), made)
        self.assertIn("reset-admin --name karl", made)
        self.assertEqual(Path(f"{self.log}.pair").read_text(), "a-long-password\nabcd-efgh-jkmn-pqrs\n")
        self.assertNotIn("a-long-password", self.logged())
        self.assertNotIn("abcd-efgh-jkmn-pqrs", self.logged())
        self.assertIn("http://192.168.1.20:8440", done.stderr)
        self.assertIn("http://localhost:8441", done.stderr)
        self.assertIn("realm the-smiths, as karl", done.stderr)
        self.assertNotIn("abcd-efgh\n", done.stderr.replace("abcd-efgh-jkmn", ""))
        self.assertNotIn("one-time code", done.stderr)

    def test_at_a_terminal_a_skipped_key_pairs_nothing_and_a_mismatch_asks_again(self):
        films = self.root / "films"
        films.mkdir()
        self.at_the_terminal("karl", "one-password", "another", "karl", "a-long-password", "a-long-password", "")
        done = self.install("--media", str(films), "--dir", str(self.dir))
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertIn("differ", done.stderr)
        self.assertTrue(Path(f"{self.log}.reset-admin").read_text().startswith("a-long-password\n"))
        self.assertFalse(Path(f"{self.log}.pair").exists())
        self.assertNotIn("realm", done.stderr)

    def test_with_no_terminal_it_prints_the_setup_code(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir))
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertIn("one-time code", done.stderr)
        self.assertIn("abcd-efgh", done.stderr)
        self.assertFalse(Path(f"{self.log}.reset-admin").exists())

    def test_a_server_with_its_admin_already_is_not_waited_on_for_a_code(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_SET_UP="1")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertIn("it has its first admin already", done.stderr)
        self.assertNotIn("setup is complete", done.stderr)
        self.assertEqual(self.logged().count("setup-code"), 1)

    def test_a_deployment_that_cannot_begin_stops_and_says_why(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_BEGIN_FAILS="1")
        self.assertEqual(done.returncode, 1)
        self.assertIn("permission denied", done.stderr)
        self.assertNotIn("compose up", self.logged())

    def test_an_update_keeps_the_image_it_replaced_and_removes_the_older_ones(self):
        films = self.root / "films"
        films.mkdir()
        self.assertEqual(self.install("--media", str(films), "--dir", str(self.dir)).returncode, 0)
        (self.dir / ".env").write_text("ORPHEUM_IMAGE=ghcr.io/example/orpheum:v1.2.2\n")
        images = "\n".join([
            "sha256:new ghcr.io/example/orpheum:v1.2.3", "sha256:previous ghcr.io/example/orpheum:v1.2.2",
            "sha256:older ghcr.io/example/orpheum:v1.2.1", "sha256:moved ghcr.io/example/orpheum:<none>",
            "sha256:used ghcr.io/example/orpheum:v1.0.0", "sha256:other ghcr.io/example/other:v1.2.1",
        ])
        self.log.unlink()
        done = self.install("--dir", str(self.dir), FAKE_IMAGES=images, FAKE_IN_USE="ghcr.io/example/orpheum:v1.0.0")
        self.assertEqual(done.returncode, 0, done.stderr)
        removed = [line.removeprefix("docker image rm ") for line in self.logged().splitlines()
                   if line.startswith("docker image rm ")]
        self.assertEqual(sorted(removed), ["ghcr.io/example/orpheum:v1.0.0", "ghcr.io/example/orpheum:v1.2.1",
                                           "sha256:moved"])
        self.assertIn("removed the old image ghcr.io/example/orpheum:v1.2.1", done.stderr)
        self.assertNotIn("v1.0.0", done.stderr)
        # The previous image was found before the pull, which can move its tag.
        log = self.logged()
        self.assertLess(log.index("image inspect --format {{.Id}} ghcr.io/example/orpheum:v1.2.2"), log.index("pull"))

    def test_a_first_install_removes_no_image(self):
        films = self.root / "films"
        films.mkdir()
        images = "sha256:new ghcr.io/example/orpheum:v1.2.3\nsha256:older ghcr.io/example/orpheum:v1.2.1"
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_IMAGES=images)
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertNotIn("image rm", self.logged())

    def test_keep_images_removes_none(self):
        films = self.root / "films"
        films.mkdir()
        self.assertEqual(self.install("--media", str(films), "--dir", str(self.dir)).returncode, 0)
        (self.dir / ".env").write_text("ORPHEUM_IMAGE=ghcr.io/example/orpheum:v1.2.2\n")
        images = "\n".join(["sha256:new ghcr.io/example/orpheum:v1.2.3", "sha256:previous ghcr.io/example/orpheum:v1.2.2",
                            "sha256:older ghcr.io/example/orpheum:v1.2.1"])
        self.log.unlink()
        done = self.install("--dir", str(self.dir), "--keep-images", FAKE_IMAGES=images)
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertNotIn("image rm", self.logged())

    def test_an_nvidia_gpu_is_handed_over_the_way_docker_can_and_named_to_write_deployment(self):
        films = self.root / "films"
        films.mkdir()
        (self.root / "nvidiactl").touch()
        for way in ("cdi", "driver"):
            with self.subTest(way=way):
                self.log.unlink(missing_ok=True)
                done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_NVIDIA=way)
                self.assertEqual(done.returncode, 0, done.stderr)
                writes = [line for line in self.logged().splitlines() if "write-deployment" in line]
                self.assertTrue(writes and all(line.endswith(f"--nvidia {way}") for line in writes), writes)
                self.assertIn("an NVIDIA GPU: yes, for hardware encoding", done.stderr)

    def test_an_nvidia_gpu_docker_cannot_hand_over_is_named_with_the_toolkit_to_install(self):
        films = self.root / "films"
        films.mkdir()
        (self.root / "nvidiactl").touch()
        done = self.install("--media", str(films), "--dir", str(self.dir))
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertNotIn("--nvidia", self.logged())
        self.assertIn("install the NVIDIA Container Toolkit", done.stderr)

    def test_a_machine_with_no_nvidia_driver_tries_nothing_and_says_nothing_of_nvidia(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_NVIDIA="cdi")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertNotIn("--entrypoint", self.logged())
        self.assertNotIn("--nvidia", self.logged())
        self.assertNotIn("NVIDIA", done.stderr)

    def test_the_deployment_is_written_as_this_user_or_under_rootless_docker_as_the_containers_root(self):
        films = self.root / "films"
        films.mkdir()
        for rootless, user in (("0", f"{os.getuid()}:{os.getgid()}"), ("1", "0:0")):
            with self.subTest(rootless=rootless):
                self.log.unlink(missing_ok=True)
                done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_ROOTLESS=rootless)
                self.assertEqual(done.returncode, 0, done.stderr)
                writes = [line for line in self.logged().splitlines() if "write-deployment" in line]
                self.assertTrue(writes and all(f" --user {user} " in line for line in writes), writes)

    def test_rootless_docker_whose_proxy_hides_addresses_is_named_with_its_fix_in_order(self):
        films = self.root / "films"
        films.mkdir()
        config = self.root / "config"
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_ROOTLESS="1",
                            XDG_CONFIG_HOME=str(config))
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertIn("rootless Docker: its userland proxy hides each device's address", done.stderr)
        module = done.stderr.index("modprobe br_netfilter")
        proxy = done.stderr.index(f'"userland-proxy": false to {config}/docker/daemon.json')
        self.assertLess(module, proxy, "the module must be loaded before the proxy is turned off")
        # With the module loaded already, only the proxy is left to turn off.
        (self.proc_sys / "net/bridge").mkdir(parents=True)
        (self.proc_sys / "net/bridge/bridge-nf-call-iptables").write_text("1\n")
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_ROOTLESS="1",
                            XDG_CONFIG_HOME=str(config))
        self.assertNotIn("br_netfilter", done.stderr)
        self.assertIn('add "userland-proxy": false', done.stderr)

    def test_rootless_docker_with_its_proxy_off_is_said_to_keep_addresses_and_rootful_says_nothing(self):
        films = self.root / "films"
        films.mkdir()
        config = self.root / "config"
        (config / "docker").mkdir(parents=True)
        (config / "docker/daemon.json").write_text('{"data-root": "/big", "userland-proxy" : false}\n')
        done = self.install("--media", str(films), "--dir", str(self.dir), FAKE_ROOTLESS="1",
                            XDG_CONFIG_HOME=str(config))
        self.assertIn("its userland proxy is off, so the server sees each device's own address", done.stderr)
        done = self.install("--media", str(films), "--dir", str(self.dir), XDG_CONFIG_HOME=str(config))
        self.assertNotIn("rootless", done.stderr)

    def test_an_image_built_here_is_not_pulled(self):
        films = self.root / "films"
        films.mkdir()
        done = self.install("--media", str(films), "--dir", str(self.dir), "--no-pull", "--image", "orpheum:v1")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertNotIn("docker pull", self.logged())


if __name__ == "__main__":
    unittest.main()
