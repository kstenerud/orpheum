# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for the development image (D235 §2): its Go and Node follow the repository's pins,
# ABOUTME: every download is pinned by hash, and the fetch scripts use the tools baked into it.
import os
import re
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / "scripts"))
import fetch_ffmpeg  # noqa: E402
import fetch_mkvtoolnix  # noqa: E402
import run_gate  # noqa: E402

CONTAINERFILE = REPO / "deploy" / "dev.Containerfile"


class Pins(unittest.TestCase):
    """The image's Go and Node follow the repository's pins; nothing else says which to use."""

    def containerfile_arg(self, name: str) -> str:
        match = re.search(rf'^ARG {name}=(?:"([^"]+)"|(\S+))$', CONTAINERFILE.read_text(), re.M)
        self.assertIsNotNone(match, name)
        return match.group(1) or match.group(2)

    def toolchain(self) -> str:
        match = re.search(r"^toolchain go(\d+\.\d+\.\d+)$", (REPO / "go.mod").read_text(), re.M)
        self.assertIsNotNone(match, "go.mod pins Go exactly with a toolchain line")
        return match.group(1)

    def test_go_is_go_mods_version(self):
        """The image reads Go's version from go.mod's toolchain line; no second copy is kept."""
        text = CONTAINERFILE.read_text()
        self.assertNotRegex(text, r"(?m)^ARG GO_VERSION")
        self.assertRegex(text, r"(?m)^COPY go\.mod ")
        self.assertIn("sed -n 's/^toolchain go//p'", text)
        go_line = re.search(r"^go (\S+)$", (REPO / "go.mod").read_text(), re.M).group(1)
        self.assertEqual(self.toolchain().split(".")[:2], go_line.split(".")[:2])

    def test_each_go_checksum_is_for_go_mods_toolchain(self):
        for arch in ("amd64", "arm64"):
            self.assertRegex(self.containerfile_arg(f"GO_SHA256_{arch.upper()}"),
                             rf"^[0-9a-f]{{64}}  go{re.escape(self.toolchain())}\.linux-{arch}\.tar\.gz$")

    def test_node_is_the_makefiles_major(self):
        major = re.search(r"^NODE_MAJOR := (\d+)$", (REPO / "Makefile").read_text(), re.M).group(1)
        self.assertEqual(self.containerfile_arg("NODE_VERSION").split(".")[0], major)

    def test_every_download_has_a_sha256_per_architecture(self):
        for tool in ("GO", "NODE"):
            for arch in ("AMD64", "ARM64"):
                self.assertRegex(self.containerfile_arg(f"{tool}_SHA256_{arch}"), r"^[0-9a-f]{64}( |$)")

    def test_the_base_image_is_pinned_by_digest(self):
        self.assertRegex(self.containerfile_arg("DEBIAN_IMAGE"), r"^debian:trixie-slim@sha256:[0-9a-f]{64}$")

    def test_the_image_says_it_is_the_dev_image_so_the_hooks_run_the_gate_directly(self):
        self.assertRegex(CONTAINERFILE.read_text(), rf"(?m)^ENV .*\b{run_gate.IN_DEV_IMAGE}=1\b")


class ToolsDir(unittest.TestCase):
    """A tool baked into the image is found there, not fetched into the clone again."""

    def install_with_tools_in(self, tools: Path, install):
        with mock.patch.dict(os.environ, {"ORPHEUM_TOOLS_DIR": str(tools)}), \
                mock.patch("urllib.request.urlopen", side_effect=AssertionError("fetched")):
            return install("linux/amd64", Path(tempfile.mkdtemp()))

    def test_ffmpeg_in_the_named_directory_is_used(self):
        tools = Path(tempfile.mkdtemp())
        dest = tools / "ffmpeg" / f"{fetch_ffmpeg.VERSION}-linux-amd64"
        dest.mkdir(parents=True)
        for name in ("ffmpeg", "ffprobe"):
            (dest / name).write_text("")
        self.assertEqual(self.install_with_tools_in(tools, fetch_ffmpeg.install), dest)

    def test_mkvtoolnix_in_the_named_directory_is_used(self):
        tools = Path(tempfile.mkdtemp())
        dest = tools / "mkvtoolnix" / f"{fetch_mkvtoolnix.VERSION}-linux-amd64"
        (dest / "usr" / "bin").mkdir(parents=True)
        for name in fetch_mkvtoolnix.TOOLS:
            (dest / "usr" / "bin" / name).write_text("")
        self.assertEqual(self.install_with_tools_in(tools, fetch_mkvtoolnix.install), dest)


if __name__ == "__main__":
    unittest.main()
