# SPDX-License-Identifier: AGPL-3.0-or-later
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
import fetch_tools  # noqa: E402
import run_gate  # noqa: E402

CONTAINERFILE = REPO / "deploy" / "Containerfile"


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

    def test_no_tool_is_built_from_source(self):
        """`go install` of golangci-lint, sqlc and delve took 613 s of the image's build on devnas
        (2026-09-30); the gate tools are fetched release binaries, and delve is installed on use."""
        self.assertNotIn("go install", CONTAINERFILE.read_text())
        self.assertRegex(CONTAINERFILE.read_text(), r"(?m)^RUN python3 /opt/orpheum/scripts/fetch_tools\.py$")

    def test_the_gate_tools_are_on_every_shells_path(self):
        """`golangci-lint` in a ./dev shell found nothing once the tools left /usr/local/bin (2026-10-01)."""
        self.assertRegex(CONTAINERFILE.read_text(), r"(?m)^ENV PATH=/opt/orpheum/tools/bin:\$PATH$")

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
        asset = fetch_ffmpeg.PINS["linux/amd64"].asset.removesuffix(".tar.xz")
        dest = tools / "ffmpeg" / f"{asset}-linux-amd64"
        dest.mkdir(parents=True)
        for name in ("ffmpeg", "ffprobe"):
            (dest / name).write_text("")
        self.assertEqual(self.install_with_tools_in(tools, fetch_ffmpeg.install), dest)

    def test_a_local_build_is_used_in_place_of_the_pin_and_said_so(self):
        local = Path(tempfile.mkdtemp())
        for name in ("ffmpeg", "ffprobe"):
            (local / name).write_text("")
            (local / name).chmod(0o755)
        (local / "COMMIT").write_text("abc123\n")
        with mock.patch.dict(os.environ, {fetch_ffmpeg.LOCAL_ENV: str(local)}), \
                mock.patch("urllib.request.urlopen", side_effect=AssertionError("fetched")), \
                mock.patch("sys.stderr") as stderr:
            self.assertEqual(fetch_ffmpeg.install("linux/amd64", Path(tempfile.mkdtemp())), local.resolve())
        said = "".join(call.args[0] for call in stderr.write.call_args_list)
        self.assertIn("not the pin", said)
        self.assertIn("abc123", said)

    def test_a_local_build_without_ffprobe_is_refused_not_fallen_back_from(self):
        local = Path(tempfile.mkdtemp())
        (local / "ffmpeg").write_text("")
        (local / "ffmpeg").chmod(0o755)
        with mock.patch.dict(os.environ, {fetch_ffmpeg.LOCAL_ENV: str(local)}), \
                mock.patch("urllib.request.urlopen", side_effect=AssertionError("fetched")), \
                self.assertRaisesRegex(SystemExit, "ffprobe"):
            fetch_ffmpeg.install("linux/amd64", Path(tempfile.mkdtemp()))

    def test_mkvtoolnix_in_the_named_directory_is_used(self):
        tools = Path(tempfile.mkdtemp())
        dest = tools / "mkvtoolnix" / f"{fetch_mkvtoolnix.VERSION}-linux-amd64"
        (dest / "usr" / "bin").mkdir(parents=True)
        for name in fetch_mkvtoolnix.TOOLS:
            (dest / "usr" / "bin" / name).write_text("")
        self.assertEqual(self.install_with_tools_in(tools, fetch_mkvtoolnix.install), dest)

    def test_a_gate_tool_in_the_named_directory_is_used(self):
        tools = Path(tempfile.mkdtemp())
        tool = fetch_tools.TOOLS[0]
        dest = tools / tool.name / f"{tool.version}-linux-amd64"
        dest.mkdir(parents=True)
        (dest / tool.name).write_text("")
        self.assertEqual(self.install_with_tools_in(tools, lambda target, root: fetch_tools.install(tool, target, root)),
                         dest / tool.name)


class Readable(unittest.TestCase):
    def test_an_extracted_mkvtoolnix_is_readable_and_runnable_by_every_user(self):
        tree = Path(tempfile.mkdtemp())
        (tree / "usr" / "bin").mkdir(parents=True)
        tool, lib = tree / "usr" / "bin" / "mkvmerge", tree / "usr" / "lib.so"
        tool.write_text("")
        lib.write_text("")
        for path, mode in ((tool, 0o700), (lib, 0o600), (tree / "usr" / "bin", 0o700), (tree / "usr", 0o700), (tree, 0o700)):
            path.chmod(mode)
        fetch_mkvtoolnix.make_readable(tree)
        self.assertEqual({p.relative_to(tree).as_posix(): p.stat().st_mode & 0o777 for p in [tree, *tree.rglob("*")]},
                         {".": 0o755, "usr": 0o755, "usr/bin": 0o755, "usr/bin/mkvmerge": 0o755, "usr/lib.so": 0o644})


class Pruned(unittest.TestCase):
    def test_only_the_tools_and_the_bundled_libraries_they_load_are_kept(self):
        tree = Path(tempfile.mkdtemp())
        lib = tree / "usr" / "lib"
        for rel in ("usr/bin/mkvmerge", "usr/bin/mkvpropedit", "usr/bin/mkvtoolnix-gui", "usr/lib/libQt6Core.so.6",
                    "usr/lib/libogg.so.0.8", "usr/lib/libQt6Gui.so.6", "usr/plugins/platforms/libqxcb.so",
                    "usr/share/icons/mkv.png", "AppRun"):
            (tree / rel).parent.mkdir(parents=True, exist_ok=True)
            (tree / rel).write_text("")
        (lib / "libogg.so.0").symlink_to("libogg.so.0.8")
        ldd = (f"\tlinux-vdso.so.1 (0x1)\n\tlibQt6Core.so.6 => {lib.resolve()}/libQt6Core.so.6 (0x2)\n"
               f"\tlibogg.so.0 => {lib.resolve()}/libogg.so.0 (0x3)\n\tlibc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x4)\n")
        with mock.patch("subprocess.run", return_value=mock.Mock(stdout=ldd)):
            fetch_mkvtoolnix.keep_only_the_tools(tree)
        self.assertEqual(sorted(p.relative_to(tree).as_posix() for p in tree.rglob("*") if not p.is_dir()),
                         ["usr/bin/mkvmerge", "usr/bin/mkvpropedit", "usr/lib/libQt6Core.so.6", "usr/lib/libogg.so.0",
                          "usr/lib/libogg.so.0.8"])


if __name__ == "__main__":
    unittest.main()
