# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for `./staging` (D249 §3): the owner's values file is read and checked, the configs
# ABOUTME: are rendered from the templates with nothing left over, and its ports never clash with dev's.
import importlib.machinery
import importlib.util
import json
import sys
import shutil
import socket
import subprocess
import tempfile
import tomllib
import unittest
from datetime import datetime, timezone
from pathlib import Path
from unittest import mock

REPO = Path(__file__).resolve().parents[2]
TEMPLATES = REPO / "deploy" / "staging"


def load_staging():
    """`./staging` as a module. It has no .py suffix, so it is loaded by path."""
    loader = importlib.machinery.SourceFileLoader("staging_launcher", str(REPO / "staging"))
    spec = importlib.util.spec_from_loader("staging_launcher", loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules["staging_launcher"] = module  # its dataclass looks itself up there
    loader.exec_module(module)
    return module


staging = load_staging()
VPS_KEY = "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE="  # 32 bytes


def filled() -> "staging.Values":
    return staging.Values(domain="media.orpheum-app.com", contact="owner@example.org",
                          vps_endpoint="[2001:db8::7]:51820", vps_public_ipv4="203.0.113.7",
                          vps_public_key=VPS_KEY, media_roots=["/mnt/tank/media/video/movies"])


class ValuesFile(unittest.TestCase):
    def test_the_template_as_copied_says_what_to_fill_in_and_nothing_else(self):
        values = staging.parse_values((TEMPLATES / "staging.toml").read_text())
        problems = staging.value_problems(values)
        self.assertEqual([problem.split(":")[0].split(" ")[0] for problem in problems],
                         ["contact", "vps.endpoint", "vps.public_ipv4", "vps.public_key", "media.roots"])

    def test_filled_values_are_read_back_and_pass(self):
        text = """domain = "media.orpheum-app.com"
contact = "owner@example.org"
[vps]
endpoint = "[2001:db8::7]:51820"
public_ipv4 = "203.0.113.7"
public_key = "%s"
mtu = 1380
[media]
roots = ["/mnt/tank/media/video/movies"]
[http]
port = 8441
""" % VPS_KEY
        values = staging.parse_values(text)
        self.assertEqual(staging.value_problems(values), [])
        self.assertEqual((values.mtu, values.http_port, values.http_publish), (1380, 8441, "0.0.0.0"))
        self.assertEqual(staging.tunnel_mtu(values), 1380)

    def test_an_endpoint_is_an_ip_address_and_a_port_with_ipv6_in_brackets(self):
        for good in ("[2001:db8::7]:51820", "203.0.113.7:51820"):
            self.assertIsNone(staging.endpoint_problem(good), good)
        for bad in ("2001:db8::7:51820", "vps.example.org:51820", "[2001:db8::7]", "203.0.113.7:0"):
            self.assertIsNotNone(staging.endpoint_problem(bad), bad)

    def test_the_mtu_is_chosen_from_the_endpoint_s_family_unless_the_owner_sets_one(self):
        """The coordinator, 2026-10-06: an IPv4-only machine, or DS-Lite's IPv4, dials the VPS's IPv4."""
        values = filled()
        self.assertEqual(staging.tunnel_mtu(values), 1420)  # [IPv6]:port
        values.vps_endpoint = "203.0.113.7:51820"
        self.assertEqual(staging.tunnel_mtu(values), 1380)
        self.assertEqual(staging.compose_environment(values, 1, 1, {})["ORPHEUM_TUNNEL_MTU"], "1380")
        values.mtu = 1300
        self.assertEqual(staging.tunnel_mtu(values), 1300)
        values.mtu = 9000
        self.assertTrue(any(p.startswith("vps.mtu") for p in staging.value_problems(values)))
        self.assertIsNone(staging.parse_values((TEMPLATES / "staging.toml").read_text()).mtu)

    def test_a_relative_media_root_a_short_key_and_dev_s_ports_are_refused(self):
        values = filled()
        values.media_roots = ["movies"]
        values.vps_public_key = "short"
        values.http_port = 8432
        keys = [problem.split(" ")[0].rstrip(":") for problem in staging.value_problems(values)]
        self.assertEqual(keys, ["vps.public_key", "media.roots", "http.port"])
        values.http_port = 5173
        self.assertTrue(any(p.startswith("http.port") for p in staging.value_problems(values)))


class Rendering(unittest.TestCase):
    def test_the_core_config_serves_the_name_and_relays_through_the_vps_and_starts_over(self):
        config = tomllib.loads(staging.core_config((TEMPLATES / "core.toml").read_text(), filled()))
        self.assertEqual(config["https"]["domain"], "media.orpheum-app.com")
        self.assertEqual(config["https"]["contact"], "owner@example.org")
        self.assertEqual(config["turn"]["public_address"], "203.0.113.7")
        self.assertEqual(config["turn"]["listen"], "10.77.0.2:3478")
        self.assertEqual(config["server"]["listen"], "0.0.0.0:8440")
        self.assertEqual(config["database"]["when_unmigratable"], "start_over")  # D249 §4

    def test_the_node_is_named_by_the_values_file_or_the_machine(self):
        """Staging's node was always "devnas", wherever it ran (2026-10-06)."""
        named = staging.parse_values('[node]\nname = "sandbox"\n')
        self.assertEqual(named.node_name, "sandbox")
        self.assertEqual(staging.parse_values("").node_name, socket.gethostname())
        config = tomllib.loads(staging.node_config((TEMPLATES / "node.toml").read_text(), named))
        self.assertEqual(config["node"]["name"], "sandbox")

    def test_the_node_config_names_the_media_and_requires_confinement(self):
        config = tomllib.loads(staging.node_config((TEMPLATES / "node.toml").read_text(), filled()))
        self.assertEqual(config["node"]["root_paths"], ["/mnt/tank/media/video/movies"])
        self.assertEqual(config["node"]["confinement_policy"], "require")
        self.assertEqual(config["node"]["core_socket"], "/run/orpheum/core/core.sock")

    def test_the_tunnel_config_names_both_keys_and_the_vps(self):
        text = staging.tunnel_config((TEMPLATES / "wg0.conf").read_text(), filled(), "PRIVATE")
        self.assertIn("PrivateKey = PRIVATE", text)
        self.assertIn(f"PublicKey = {VPS_KEY}", text)
        self.assertIn("Endpoint = [2001:db8::7]:51820", text)
        self.assertNotIn("Address", text)  # wg setconf's format; the addresses are tunnel.sh's

    def test_a_placeholder_with_no_value_is_an_error_not_a_config(self):
        with self.assertRaises(ValueError):
            staging.render('domain = "{domain}"\nother = "{other}"\n', {"domain": "x"})

    def test_the_media_override_mounts_each_root_read_only_and_the_gpu_when_there_is_one(self):
        override = json.loads(staging.media_override(["/mnt/a b", "/srv/c"], 105))["services"]["orpheum-node"]
        self.assertEqual(override["volumes"], ["/mnt/a b:/mnt/a b:ro", "/srv/c:/srv/c:ro"])
        self.assertEqual((override["devices"], override["group_add"]), (["/dev/dri:/dev/dri"], ["105"]))
        without = json.loads(staging.media_override(["/srv/c"], None))["services"]["orpheum-node"]
        self.assertNotIn("devices", without)

    def test_compose_reads_the_image_the_directory_the_owner_and_the_port(self):
        env = staging.compose_environment(filled(), 1000, 1001, {"PATH": "/bin"})
        self.assertEqual(env["ORPHEUM_IMAGE"], staging.IMAGE)
        self.assertEqual((env["ORPHEUM_UID"], env["ORPHEUM_GID"]), ("1000", "1001"))
        self.assertEqual((env["ORPHEUM_HTTP_PORT"], env["ORPHEUM_TUNNEL_MTU"]), ("8440", "1420"))
        self.assertEqual(staging.compose_environment(filled(), 1, 1, {}, "sha256:abc")["ORPHEUM_IMAGE"],
                         "sha256:abc")
        self.assertEqual(env["PATH"], "/bin")
        compose = (REPO / "deploy" / "compose.yaml").read_text()
        for name in ("ORPHEUM_IMAGE", "ORPHEUM_DIR", "ORPHEUM_UID", "ORPHEUM_GID", "ORPHEUM_HTTP_PUBLISH",
                     "ORPHEUM_HTTP_PORT", "ORPHEUM_TUNNEL_MTU"):
            self.assertIn("${" + name, compose, name)


class SavedLogs(unittest.TestCase):
    def run_logs(self, output: str) -> tuple["Path | None", Path]:
        """save_logs into a fresh directory, with compose's logs answering output."""
        directory = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, directory)
        answered = subprocess.CompletedProcess([], 0, stdout=output)
        with mock.patch.object(staging, "LOGS", directory / "logs"), \
                mock.patch.object(staging.subprocess, "run", return_value=answered) as run:
            saved = staging.save_logs({}, datetime(2026, 10, 6, 13, 5, 9, tzinfo=timezone.utc))
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
