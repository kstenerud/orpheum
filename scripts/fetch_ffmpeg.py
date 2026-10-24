#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Fetches the pinned jellyfin-ffmpeg portable build for this machine into .tools/,
# ABOUTME: verifies its SHA-256 and prints the directory holding ffmpeg and ffprobe.
"""Download the one FFmpeg this project builds and tests against (D193 §6).

The server never downloads FFmpeg at runtime: a release bundles it. This script is for
development and CI. `make ffmpeg` runs it, and the dev loop points the node at the result.

On Linux it is jellyfin-ffmpeg with the fdproxy protocol added (ffmpeg-fdproxy, formerly ffmpeg-ffstreamer, D330), so
the node serves each film to the run that reads it; on macOS, for development alone, jellyfin's
own build, whose runs read each film by its path. The pin is a release plus one SHA-256 per
platform, taken from the release's own asset digests. A download whose hash differs is deleted and refused. It is
never "probably fine": a swapped binary is exactly what a hash exists to catch. Changing the
pin is an owner action, like any gate.

ORPHEUM_FFMPEG_LOCAL, when set, names a directory holding an `ffmpeg` and an `ffprobe` to use in
place of the pin: ffmpeg-fdproxy's own build of fixes not yet released (`make remote-build` there
fetches one into ../ffmpeg-fdproxy/dist/bin-amd64). It is used as it is, with no hash, and said so
on stderr each time, so a local build is never taken for the pinned one. ./dev mounts it and ./staging
builds it into the runtime image.

Prints the path of the directory that contains `ffmpeg` and `ffprobe`, and nothing else on
stdout, so a Makefile can capture it.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import platform
import shutil
import sys
import tarfile
import tempfile
import urllib.request
from dataclasses import dataclass
from pathlib import Path

# ffmpeg-fdproxy's release, made when it was ffmpeg-ffstreamer, under that name: jellyfin-ffmpeg
# v8.1.2-5 and its first patches (GitHub API, 2026-10-23). GitHub sends the old repository's URLs to
# the renamed one; the next pin names ffmpeg-fdproxy.
FFSTREAMER_VERSION = "8.1.2-5-1"
FFSTREAMER_URL = f"https://github.com/kstenerud/ffmpeg-ffstreamer/releases/download/v{FFSTREAMER_VERSION}"
# jellyfin-ffmpeg's own release, which ffmpeg-fdproxy builds no macOS binary of (GitHub API, 2026-09-23).
JELLYFIN_VERSION = "8.1.2-5"
JELLYFIN_URL = f"https://github.com/jellyfin/jellyfin-ffmpeg/releases/download/v{JELLYFIN_VERSION}"


@dataclass(frozen=True)
class Pin:
    """One platform's FFmpeg: the archive's URL and its SHA-256."""

    url: str
    sha256: str

    @property
    def asset(self) -> str:
        return self.url.rsplit("/", 1)[1]


PINS = {
    "linux/amd64": Pin(
        f"{FFSTREAMER_URL}/ffmpeg-ffstreamer_{FFSTREAMER_VERSION}_linux64-gpl.tar.xz",
        "4d606cac0269a43ebb10ba0bc3fcc50cfee4d1c524c4cadbb9b8fa2ea0c970ff",
    ),
    "linux/arm64": Pin(
        f"{FFSTREAMER_URL}/ffmpeg-ffstreamer_{FFSTREAMER_VERSION}_linuxarm64-gpl.tar.xz",
        "20a4c22fb0356bd59ab88de46b034b8c699851d7535df1fc9244054a8b3f8324",
    ),
    "darwin/arm64": Pin(
        f"{JELLYFIN_URL}/jellyfin-ffmpeg_{JELLYFIN_VERSION}_portable_macarm64-gpl.tar.xz",
        "b2ac80bb184e9a2f3f7c236876b2f56a5639596a95b42f41ced34fab66ad720d",
    ),
}

# The environment variable naming a directory that holds fetched tools in place of .tools/.
TOOLS_DIR_ENV = "ORPHEUM_TOOLS_DIR"
# The environment variable naming a local build of FFmpeg to use in place of the pin.
LOCAL_ENV = "ORPHEUM_FFMPEG_LOCAL"


def local_build() -> Path | None:
    """The local build ORPHEUM_FFMPEG_LOCAL names, said so on stderr; None when it names none.

    A name that holds no runnable ffmpeg and ffprobe is refused, never fallen back from: the caller
    asked for that build, and the pin in its place would test something else unnoticed.
    """
    named = os.environ.get(LOCAL_ENV)
    if not named:
        return None
    local = Path(named).resolve()
    missing = [name for name in ("ffmpeg", "ffprobe") if not os.access(local / name, os.X_OK)]
    if missing:
        raise SystemExit(f"{LOCAL_ENV}={named} holds no runnable {' or '.join(missing)}")
    commit = local / "COMMIT"
    built = f" (ffmpeg-fdproxy {commit.read_text().strip()})" if commit.is_file() else ""
    print(f"FFmpeg: the local build in {local}{built}, not the pin ({LOCAL_ENV})", file=sys.stderr)
    return local


def this_platform() -> str:
    system = platform.system().lower()
    machine = platform.machine().lower()
    arch = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(machine, machine)
    return f"{system}/{arch}"


def tools_dir(root: Path) -> Path:
    """Where fetched tools are kept: ORPHEUM_TOOLS_DIR when it is set, else root/.tools.

    The development image sets it to the tools it was built with (deploy/Containerfile,
    D235), so a gate run inside it finds them there instead of fetching into the repository.
    """
    override = os.environ.get(TOOLS_DIR_ENV)
    return Path(override) if override else root / ".tools"


def sha256_of(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def verify(path: Path, expected: str) -> None:
    got = sha256_of(path)
    if got != expected:
        path.unlink(missing_ok=True)
        raise SystemExit(
            f"{path.name}: SHA-256 is {got}, the pin says {expected}. Refusing it and deleting "
            "the download. If the pin is wrong, the owner changes it in scripts/fetch_ffmpeg.py."
        )


def install(target: str, root: Path) -> Path:
    local = local_build()
    if local is not None:
        return local
    if target not in PINS:
        raise SystemExit(f"no pinned FFmpeg for {target}; pinned: {', '.join(sorted(PINS))}")
    pin = PINS[target]
    # The directory names the archive, so a new pin is fetched rather than taken for the old one.
    dest = tools_dir(root) / "ffmpeg" / f"{pin.asset.removesuffix('.tar.xz')}-{target.replace('/', '-')}"
    if (dest / "ffmpeg").exists() and (dest / "ffprobe").exists():
        return dest
    with tempfile.TemporaryDirectory(prefix="ffmpeg-") as tmp:
        archive = Path(tmp) / pin.asset
        print(f"fetching {pin.url}", file=sys.stderr)
        with urllib.request.urlopen(pin.url, timeout=120) as response, archive.open("wb") as out:
            shutil.copyfileobj(response, out)
        verify(archive, pin.sha256)
        with tarfile.open(archive) as tar:
            tar.extractall(Path(tmp) / "x", filter="data")
        found = {p.name: p for p in (Path(tmp) / "x").rglob("*") if p.name in ("ffmpeg", "ffprobe")}
        if set(found) != {"ffmpeg", "ffprobe"}:
            raise SystemExit(f"{pin.asset} did not contain ffmpeg and ffprobe")
        dest.mkdir(parents=True, exist_ok=True)
        for name, src in found.items():
            shutil.copy2(src, dest / name)
            (dest / name).chmod(0o755)
    return dest


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--root", default=".")
    ap.add_argument("--platform", default=this_platform(), help="e.g. linux/amd64")
    args = ap.parse_args()
    print(install(args.platform, Path(args.root).resolve()))
    return 0


if __name__ == "__main__":
    sys.exit(main())
