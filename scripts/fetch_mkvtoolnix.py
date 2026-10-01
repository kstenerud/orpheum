#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Fetches the pinned MKVToolNix AppImage into .tools/, verifies its SHA-256, extracts
# ABOUTME: it, and prints the directory holding usr/bin/mkvmerge and usr/bin/mkvpropedit.
"""Download the one MKVToolNix the integration tier edits Matroska files with (D203, M2-3).

The node never runs MKVToolNix: it only reads media. The tests do, to show that an in-place tag
edit keeps a file's identity and a remux does not (D179). D179's measurements used 102.0, which
is the pin. MKVToolNix publishes a portable build only as an x86_64 Linux AppImage, so that is
the only platform pinned; elsewhere this prints nothing, and a test that needs MKVToolNix fails
unless its job lists "mkvtoolnix" in ORPHEUM_TEST_UNCONTROLLED (standards/testing.md §3).

The AppImage is extracted with its own --appimage-extract, which needs no FUSE, after its hash
is verified. Its tools find their bundled libraries through LD_LIBRARY_PATH=<dir>/usr/lib.

Prints the path of the extracted directory and nothing else on stdout, so a Makefile can
capture it. Changing the pin is an owner action, like any gate.
"""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
import tempfile
import urllib.request
from pathlib import Path

from fetch_ffmpeg import this_platform, tools_dir, verify

VERSION = "102.0"
ASSET = f"MKVToolNix_GUI-{VERSION}-x86_64.AppImage"
URL = f"https://mkvtoolnix.download/appimage/{ASSET}"
PINS = {
    "linux/amd64": "c66345b30d6d5fd640ea982ab5e202a99b9f541a20e42aa19edd90f3ddd5dc9b",
}
TOOLS = ("mkvmerge", "mkvpropedit")


def install(target: str, root: Path) -> Path | None:
    """The extracted directory for target, fetching it if needed; None where nothing is pinned."""
    if target not in PINS:
        print(f"no pinned MKVToolNix for {target}; pinned: {', '.join(sorted(PINS))}", file=sys.stderr)
        return None
    dest = tools_dir(root) / "mkvtoolnix" / f"{VERSION}-{target.replace('/', '-')}"
    if all((dest / "usr" / "bin" / tool).exists() for tool in TOOLS):
        return dest
    with tempfile.TemporaryDirectory(prefix="mkvtoolnix-") as tmp:
        appimage = Path(tmp) / ASSET
        print(f"fetching {URL}", file=sys.stderr)
        with urllib.request.urlopen(URL, timeout=300) as response, appimage.open("wb") as out:
            shutil.copyfileobj(response, out)
        verify(appimage, PINS[target])
        appimage.chmod(0o755)
        subprocess.run([str(appimage), "--appimage-extract"], cwd=tmp, check=True, stdout=subprocess.DEVNULL)
        extracted = Path(tmp) / "squashfs-root"
        missing = [tool for tool in TOOLS if not (extracted / "usr" / "bin" / tool).exists()]
        if missing:
            raise SystemExit(f"{ASSET} did not contain {', '.join(missing)}")
        make_readable(extracted)
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.rmtree(dest, ignore_errors=True)
        shutil.move(str(extracted), dest)
    return dest


def make_readable(tree: Path) -> None:
    """Let every user read tree and run what its owner can run.

    --appimage-extract leaves its directories mode 700, so in the development image, where root
    fetches and the developer runs the tests, the tools were there and unreachable (2026-10-01).
    """
    for path in [tree, *tree.rglob("*")]:
        if path.is_symlink():
            continue
        mode = path.stat().st_mode
        extra = 0o055 if path.is_dir() or mode & 0o100 else 0o044
        os.chmod(path, mode | extra)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--root", default=".")
    ap.add_argument("--platform", default=this_platform(), help="e.g. linux/amd64")
    args = ap.parse_args()
    found = install(args.platform, Path(args.root).resolve())
    if found is not None:
        print(found)
    return 0


if __name__ == "__main__":
    sys.exit(main())
