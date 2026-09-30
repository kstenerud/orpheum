#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Dependency gate run by `make check`: every Go module in go.mod and every
# ABOUTME: package in a web/ package.json must be on the owner's allowlist, with a reason; the
# ABOUTME: pinned Playwright is the lockfile's; eslint names only installed packages as the fix.
"""Check that every dependency is one the owner chose.

Adding a dependency is an owner action. Three failures motivate it:

* **Invented packages.** Coding models still suggest package names that do not exist,
  and squatters register the popular inventions.
* **A second library for one job.** Each new idiom makes the codebase look less like
  itself (principles §4).
* **Silent sprawl.** Nobody decided the dependency, so nobody owns its upgrades.

The allowlists are `deps/go.allow` and `deps/npm.allow`, one entry per line:
`<name>  # <reason> (<decision or standard>)`. An entry without a reason fails, because
an allowlist of bare names records that something was added but not why, and "why" is
what the next person needs. An allowlisted name that nothing uses is reported (not
failed), so the list does not quietly outlive its reasons.

Two checks keep what the tooling says about dependencies true:

* **The Makefile's Playwright pin is the lockfile's.** `make browsers` installs the browsers for
  `PLAYWRIGHT_VERSION`; Playwright fetches the browsers of its own version, so a pin that
  drifted from `@playwright/test` would refuse every browser-tier run.
* **eslint's "use X instead" names an installed package.** Each banned library's message is the
  fix, and a fix naming a scoped package nothing installs (it named `@tanstack/react-query` when
  the app's `useResource` does the job) sends the reader to add a dependency.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

GO_REQUIRE_LINE = re.compile(r"^\s*([^\s()]+)\s+v[^\s]+")
PLAYWRIGHT_PIN = re.compile(r"^PLAYWRIGHT_VERSION\s*:?=\s*(\S+)", re.M)
# One entry of eslint.config.js's alternativeLibraries: ["banned", "what to use instead"].
LINT_ALTERNATIVE = re.compile(r'^\s*\["([^"]+)",\s*"([^"]+)"\],?\s*$', re.M)
SCOPED_PACKAGE = re.compile(r"(?<![\w@/.-])@[a-z0-9][\w.-]*/[a-z0-9][\w.-]*")


@dataclass(frozen=True)
class Problem:
    where: str
    message: str


def parse_allowlist(text: str) -> tuple[dict[str, str], list[str]]:
    """Return ({name: reason}, [names listed without a reason])."""
    allowed: dict[str, str] = {}
    bare: list[str] = []
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        name, _, reason = line.partition("#")
        name, reason = name.strip(), reason.strip()
        allowed[name] = reason
        if not reason:
            bare.append(name)
    return allowed, bare


def go_modules(go_mod: str) -> list[str]:
    """Every module named in a `require` directive, direct or indirect."""
    modules: list[str] = []
    in_block = False
    for raw in go_mod.splitlines():
        line = raw.split("//", 1)[0].rstrip()
        stripped = line.strip()
        if stripped.startswith("require ("):
            in_block = True
            continue
        if in_block and stripped == ")":
            in_block = False
            continue
        if in_block:
            m = GO_REQUIRE_LINE.match(stripped)
            if m:
                modules.append(m.group(1))
        elif stripped.startswith("require "):
            m = GO_REQUIRE_LINE.match(stripped[len("require "):])
            if m:
                modules.append(m.group(1))
    return modules


def npm_packages(manifest: dict[str, object]) -> list[str]:
    names: list[str] = []
    for field in ("dependencies", "devDependencies", "peerDependencies", "optionalDependencies"):
        section = manifest.get(field)
        if isinstance(section, dict):
            names.extend(str(k) for k in section)
    return names


def workspace_manifests(root: Path) -> list[Path]:
    """package.json files under web/ that git would commit (never node_modules)."""
    out = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "web"],
        cwd=root, capture_output=True, text=True, check=True,
    ).stdout
    return [root / p for p in out.splitlines() if p.endswith("package.json")]


def workspace_names(manifests: list[Path]) -> set[str]:
    """Our own workspace packages, which depend on each other without being dependencies."""
    names: set[str] = set()
    for m in manifests:
        name = json.loads(m.read_text()).get("name")
        if isinstance(name, str):
            names.add(name)
    return names


def check_set(
    kind: str, used: dict[str, str], allowed: dict[str, str], bare: list[str], allow_path: str
) -> tuple[list[Problem], list[str]]:
    problems = [
        Problem(where, f"{kind} dependency {name!r} is not in {allow_path}. Adding a dependency "
                       "is an owner action: propose it with a reason.")
        for name, where in sorted(used.items())
        if name not in allowed
    ]
    problems += [Problem(allow_path, f"{name!r} is listed without a reason") for name in bare]
    unused = [f"{allow_path}: {name!r} is allowed but nothing uses it" for name in sorted(allowed)
              if name not in used]
    return problems, unused


def playwright_pin_problems(makefile: str, lockfile: dict[str, object]) -> list[Problem]:
    """The Makefile's PLAYWRIGHT_VERSION differs from the @playwright/test the lockfile installs."""
    pin = PLAYWRIGHT_PIN.search(makefile)
    packages = lockfile.get("packages")
    entry = packages.get("node_modules/@playwright/test") if isinstance(packages, dict) else None
    locked = entry.get("version") if isinstance(entry, dict) else None
    if pin is None:
        return [Problem("Makefile", "sets no PLAYWRIGHT_VERSION, so `make browsers` pins nothing")]
    if pin.group(1) != locked:
        return [Problem("Makefile", f"PLAYWRIGHT_VERSION is {pin.group(1)}, but web/package-lock.json "
                                    f"installs @playwright/test {locked}: the browsers are Playwright's own "
                                    "version's, so pin the one installed")]
    return []


def lint_alternative_problems(eslint_config: str, installed: set[str]) -> list[Problem]:
    """eslint messages that tell the reader to use a scoped package nothing installs."""
    return [
        Problem("web/eslint.config.js", f"the message for {banned!r} names {name}, which no web "
                                        "package installs: name what the code uses instead")
        for banned, instead in LINT_ALTERNATIVE.findall(eslint_config)
        for name in SCOPED_PACKAGE.findall(instead)
        if name not in installed
    ]


def check(root: Path) -> tuple[list[Problem], list[str]]:
    problems: list[Problem] = []
    notes: list[str] = []

    go_allowed, go_bare = parse_allowlist((root / "deps/go.allow").read_text())
    go_used = {m: "go.mod" for m in go_modules((root / "go.mod").read_text())}
    p, n = check_set("Go", go_used, go_allowed, go_bare, "deps/go.allow")
    problems += p
    notes += n

    npm_allowed, npm_bare = parse_allowlist((root / "deps/npm.allow").read_text())
    manifests = workspace_manifests(root)
    ours = workspace_names(manifests)
    npm_used: dict[str, str] = {}
    for m in manifests:
        for name in npm_packages(json.loads(m.read_text())):
            if name not in ours:
                npm_used.setdefault(name, str(m.relative_to(root)))
    p, n = check_set("npm", npm_used, npm_allowed, npm_bare, "deps/npm.allow")
    problems += p
    notes += n

    # A tree without a web client or a Makefile (a gate test's) has nothing of these to check.
    lockfile, makefile, eslint = root / "web/package-lock.json", root / "Makefile", root / "web/eslint.config.js"
    if lockfile.exists() and makefile.exists():
        problems += playwright_pin_problems(makefile.read_text(), json.loads(lockfile.read_text()))
    if eslint.exists():
        problems += lint_alternative_problems(eslint.read_text(), set(npm_used) | ours)
    return problems, notes


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--root", default=".")
    args = ap.parse_args()
    problems, notes = check(Path(args.root).resolve())
    for note in notes:
        print(f"note: {note}")
    for p in problems:
        print(f"{p.where}: {p.message}")
    if problems:
        print(f"check_deps: {len(problems)} problem(s)")
        return 1
    print("check_deps: every dependency is allowlisted with a reason")
    return 0


if __name__ == "__main__":
    sys.exit(main())
