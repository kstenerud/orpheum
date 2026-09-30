#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Fetches the pinned jellyfin-ffmpeg portable build for this machine into .tools/,
# ABOUTME: verifies its SHA-256 and prints the directory holding ffmpeg and ffprobe.
"""Download the one FFmpeg this project builds and tests against (D193 §6).

The server never downloads FFmpeg at runtime: a release bundles it. This script is for
development and CI. `make ffmpeg` runs it, and the dev loop points the node at the result.

The pin is a version plus one SHA-256 per platform, taken from the release's own asset
digests (GitHub API, 2026-09-23). A download whose hash differs is deleted and refused. It is
never "probably fine": a swapped binary is exactly what a hash exists to catch. Changing the
pin is an owner action, like any gate.

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
from pathlib import Path

VERSION = "8.1.2-5"
BASE_URL = f"https://github.com/jellyfin/jellyfin-ffmpeg/releases/download/v{VERSION}"
PINS = {
    "linux/amd64": ("linux64", "1fd859927053c44a4f2dbf67ae8b9ba8d29fb3b8930df0dd57d91aa60589363d"),
    "linux/arm64": ("linuxarm64", "1bd4fafaf4c309cad896d3479152c31d6b7604d4be8b8526882ea16cc6d5f589"),
    "darwin/arm64": ("macarm64", "b2ac80bb184e9a2f3f7c236876b2f56a5639596a95b42f41ced34fab66ad720d"),
}

# The environment variable naming a directory that holds fetched tools in place of .tools/.
TOOLS_DIR_ENV = "ORPHEUM_TOOLS_DIR"


def this_platform() -> str:
    system = platform.system().lower()
    machine = platform.machine().lower()
    arch = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(machine, machine)
    return f"{system}/{arch}"


def tools_dir(root: Path) -> Path:
    """Where fetched tools are kept: ORPHEUM_TOOLS_DIR when it is set, else root/.tools.

    The development image sets it to the tools it was built with (deploy/dev.Containerfile,
    D235), so a gate run inside it finds them there instead of fetching into the repository.
    """
    override = os.environ.get(TOOLS_DIR_ENV)
    return Path(override) if override else root / ".tools"


def asset_name(target: str) -> str:
    return f"jellyfin-ffmpeg_{VERSION}_portable_{PINS[target][0]}-gpl.tar.xz"


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
    dest = tools_dir(root) / "ffmpeg" / f"{VERSION}-{target.replace('/', '-')}"
    if (dest / "ffmpeg").exists() and (dest / "ffprobe").exists():
        return dest
    if target not in PINS:
        raise SystemExit(f"no pinned FFmpeg for {target}; pinned: {', '.join(sorted(PINS))}")
    url = f"{BASE_URL}/{asset_name(target)}"
    with tempfile.TemporaryDirectory(prefix="ffmpeg-") as tmp:
        archive = Path(tmp) / asset_name(target)
        print(f"fetching {url}", file=sys.stderr)
        with urllib.request.urlopen(url, timeout=120) as response, archive.open("wb") as out:
            shutil.copyfileobj(response, out)
        verify(archive, PINS[target][1])
        with tarfile.open(archive) as tar:
            tar.extractall(Path(tmp) / "x", filter="data")
        found = {p.name: p for p in (Path(tmp) / "x").rglob("*") if p.name in ("ffmpeg", "ffprobe")}
        if set(found) != {"ffmpeg", "ffprobe"}:
            raise SystemExit(f"{asset_name(target)} did not contain ffmpeg and ffprobe")
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
