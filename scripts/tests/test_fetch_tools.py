# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for scripts/fetch_tools.py: every gate tool is pinned by hash on each platform
# ABOUTME: FFmpeg is, a fetched one is verified, and .tools/bin follows the pin.
import hashlib
import io
import os
import sys
import tarfile
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path
from unittest import mock

REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / "scripts"))
import fetch_ffmpeg  # noqa: E402
import fetch_tools  # noqa: E402


def archive_holding(name: str, content: bytes) -> bytes:
    """A .tar.gz with content at <dir>/<name>, the layout golangci-lint's releases use."""
    out = io.BytesIO()
    with tarfile.open(fileobj=out, mode="w:gz") as tar:
        info = tarfile.TarInfo(f"release/{name}")
        info.size = len(content)
        tar.addfile(info, io.BytesIO(content))
    return out.getvalue()


def serving(data: bytes):
    """urlopen, answering every request with data."""
    return mock.patch("urllib.request.urlopen", side_effect=lambda *a, **k: io.BytesIO(data))


class Pins(unittest.TestCase):
    def test_each_tool_is_pinned_by_sha256_on_every_platform_ffmpeg_is(self):
        for tool in fetch_tools.TOOLS:
            self.assertEqual(set(tool.sha256), set(fetch_ffmpeg.PINS), tool.name)
            for digest in tool.sha256.values():
                self.assertRegex(digest, r"^[0-9a-f]{64}$", tool.name)

    def test_the_makefile_keeps_no_second_copy_of_a_pin(self):
        makefile = (REPO / "Makefile").read_text()
        for tool in fetch_tools.TOOLS:
            self.assertNotIn(tool.version, makefile, tool.name)


class Install(unittest.TestCase):
    def setUp(self):
        # Inside the development image ORPHEUM_TOOLS_DIR names the baked-in tools; these tests
        # install into their own root's .tools/ instead.
        env = mock.patch.dict(os.environ)
        env.start()
        self.addCleanup(env.stop)
        os.environ.pop(fetch_ffmpeg.TOOLS_DIR_ENV, None)
        self.root = Path(tempfile.mkdtemp())
        self.data = archive_holding("sqlc", b"binary")
        self.tool = replace(fetch_tools.TOOLS[1], sha256={"linux/amd64": hashlib.sha256(self.data).hexdigest()})

    def test_a_download_matching_its_pin_is_installed_and_linked(self):
        with serving(self.data), mock.patch("sys.stderr", io.StringIO()):
            binary = fetch_tools.install(self.tool, "linux/amd64", self.root)
        fetch_tools.link(binary, self.root)
        self.assertEqual((fetch_tools.bin_dir(self.root) / "sqlc").read_bytes(), b"binary")

    def test_a_download_with_another_hash_is_refused_and_nothing_is_installed(self):
        with serving(archive_holding("sqlc", b"swapped")), mock.patch("sys.stderr", io.StringIO()), \
                self.assertRaises(SystemExit):
            fetch_tools.install(self.tool, "linux/amd64", self.root)
        self.assertFalse(fetch_tools.install_dir(self.tool, "linux/amd64", self.root).exists())

    def test_a_changed_pin_moves_the_link_to_the_new_version(self):
        for version in ("1.0.0", "2.0.0"):
            tool = replace(self.tool, version=version)
            dest = fetch_tools.install_dir(tool, "linux/amd64", self.root)
            dest.mkdir(parents=True)
            (dest / "sqlc").write_text(version)
            fetch_tools.link(fetch_tools.install(tool, "linux/amd64", self.root), self.root)
            self.assertEqual((fetch_tools.bin_dir(self.root) / "sqlc").read_text(), version)


if __name__ == "__main__":
    unittest.main()
