#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Fetches the pinned golangci-lint and sqlc release binaries into .tools/, verifies each
# ABOUTME: one's SHA-256, and links them into .tools/bin, which the Makefile puts first on PATH.
"""Install the gate's own tools: golangci-lint and sqlc, at the versions this project pins.

Each is the project's own release binary, pinned by version and by one SHA-256 per platform
(D235 §2), as scripts/fetch_ffmpeg.py pins FFmpeg. Building them from source instead took ten
minutes of a development image's build (2026-10-01), and the checksum checks the same thing
`go install` checked: that the binary is the one the pin names. A download whose hash differs is
deleted and refused. Changing a pin is an owner action, like any gate, because a linter upgrade
changes what the gate means.

Each tool lands in <tools>/<tool>/<version>-<platform>/ and is linked from <tools>/bin, where
<tools> is ORPHEUM_TOOLS_DIR or the clone's .tools/. A pin change fetches the new version and
moves the link; an unchanged pin fetches nothing. `make check` runs this first, so a fresh clone
or worktree gets its tools without a separate step.

The pins: golangci-lint's from its release's checksums file, sqlc's from its release's asset
digests (GitHub API), both read on 2026-10-01.
"""

from __future__ import annotations

import argparse
import shutil
import sys
import tarfile
import tempfile
from dataclasses import dataclass
from pathlib import Path

from fetch_ffmpeg import download, this_platform, tools_dir, verify


@dataclass(frozen=True)
class Tool:
    name: str
    version: str
    # The release asset's name for a platform's (os, arch), e.g. ("linux", "amd64").
    asset: str
    url: str
    # SHA-256 of the asset, by platform ("linux/amd64").
    sha256: dict[str, str]

    def asset_for(self, target: str) -> str:
        os_name, arch = target.split("/")
        return self.asset.format(version=self.version, os=os_name, arch=arch)


TOOLS = (
    Tool(
        name="golangci-lint",
        version="2.13.2",
        asset="golangci-lint-{version}-{os}-{arch}.tar.gz",
        url="https://github.com/golangci/golangci-lint/releases/download/v{version}/{asset}",
        sha256={
            "linux/amd64": "2277d43b98ec0054280f2ac26b53268bae97682444678a59a657dd565da021d6",
            "linux/arm64": "a2a4e0065aa41be71f7c5ac90f271b61751331e5d04314e62afe4027855f0893",
            "darwin/arm64": "f4bf83f0b64f055c42b28fc9a38861839f69c096e61c788e72dfaae412011789",
        },
    ),
    Tool(
        name="sqlc",
        version="1.31.1",
        asset="sqlc_{version}_{os}_{arch}.tar.gz",
        url="https://github.com/sqlc-dev/sqlc/releases/download/v{version}/{asset}",
        sha256={
            "linux/amd64": "497ae4fcdfa64c5b0c311ffe4c2bd991e43991e82e5367792ed78bc2dca27354",
            "linux/arm64": "b7cae247740d0c51a1e657479e5b2d21e6fef428f596682a01bc55bf4ab8a23d",
            "darwin/arm64": "21602158c99eb1f2bae197a66abfb1941d1e9e50b23125bb193349c6b1acc71e",
        },
    ),
)


def install_dir(tool: Tool, target: str, root: Path) -> Path:
    return tools_dir(root) / tool.name / f"{tool.version}-{target.replace('/', '-')}"


def bin_dir(root: Path) -> Path:
    """The directory the Makefile puts first on PATH."""
    return tools_dir(root) / "bin"


def install(tool: Tool, target: str, root: Path) -> Path:
    """The pinned binary for target, fetched and verified if it is not already there."""
    if target not in tool.sha256:
        raise SystemExit(f"no pinned {tool.name} for {target}; pinned: {', '.join(sorted(tool.sha256))}")
    dest = install_dir(tool, target, root)
    binary = dest / tool.name
    if binary.exists():
        return binary
    asset = tool.asset_for(target)
    url = tool.url.format(version=tool.version, asset=asset)
    with tempfile.TemporaryDirectory(prefix=f"{tool.name}-") as tmp:
        archive = Path(tmp) / asset
        print(f"fetching {url}", file=sys.stderr)
        download(url, archive, timeout=120)
        verify(archive, tool.sha256[target])
        with tarfile.open(archive) as tar:
            tar.extractall(Path(tmp) / "x", filter="data")
        found = [p for p in (Path(tmp) / "x").rglob(tool.name) if p.is_file()]
        if len(found) != 1:
            raise SystemExit(f"{asset} did not contain one {tool.name}")
        dest.mkdir(parents=True, exist_ok=True)
        shutil.copy2(found[0], binary)
        binary.chmod(0o755)
    return binary


def link(binary: Path, root: Path) -> None:
    """Point <tools>/bin/<name> at binary, replacing a link to another version."""
    bin_path = bin_dir(root)
    bin_path.mkdir(parents=True, exist_ok=True)
    link_path = bin_path / binary.name
    target = Path("..") / binary.relative_to(tools_dir(root))
    if link_path.is_symlink() and Path(link_path.readlink()) == target:
        return
    link_path.unlink(missing_ok=True)
    link_path.symlink_to(target)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--root", default=".")
    ap.add_argument("--platform", default=this_platform(), help="e.g. linux/amd64")
    args = ap.parse_args()
    root = Path(args.root).resolve()
    for tool in TOOLS:
        link(install(tool, args.platform, root), root)
    return 0


if __name__ == "__main__":
    sys.exit(main())
