#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Resolves the base the branch gates (`make check-pr`) compare against: the BASE given,
# ABOUTME: or the commit the owner last took (refs/orpheum/applied), or origin/main. It prints the
# ABOUTME: base and how far back it is, and refuses an implicit base too far back to mean anything.
"""Which commit the branch gates compare the work against.

The branch gates ask what the work since a base changed: revert-red reverts each changed file,
test-weakening compares each test file with the base's. A base far older than the work makes
both blind without saying so. Against a base 323 commits back, revert-red saw 285 of 314 files
as added, and reverting an added file breaks the build rather than a test; test-weakening skips
added test files entirely (audit-2026-09-29 §6).

In a yoloAI sandbox origin/main is where the sandbox started, not what the owner has taken: the
owner takes commits out with `yoloai apply`, which moves nothing in the sandbox. So the owner
marks what was taken, `make mark-applied` (or `REV=<commit>` for an earlier one), which sets
`refs/orpheum/applied`, and the gates compare against that. Without the ref they fall back to
origin/main.

An implicit base more than MAX_IMPLICIT_COMMITS behind HEAD is refused: the mark was probably not
moved after an apply. 150 is a little more than the busiest day's commits in this repository so
far (134 on 2026-09-29), so a base older than that is a day or more of work the owner took and
never marked, which the gates would then report again as the branch's, and mostly as added
files. A BASE given explicitly is always used: CI names the pull request's base, and a person
rechecking a range names it on purpose.
"""

from __future__ import annotations

import argparse
import subprocess
import sys

APPLIED_REF = "refs/orpheum/applied"
FALLBACK_REF = "origin/main"
MAX_IMPLICIT_COMMITS = 150


class BaseError(Exception):
    """Why no base could be used."""


def _git(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], capture_output=True, text=True, check=False)


def commit_of(ref: str) -> str | None:
    """The commit `ref` names, or None when it names none."""
    done = _git("rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}")
    return done.stdout.strip() if done.returncode == 0 else None


def commits_between(base: str, head: str) -> int:
    return int(_git("rev-list", "--count", f"{base}..{head}").stdout.strip() or "0")


def resolve(explicit: str | None, head: str = "HEAD",
            limit: int = MAX_IMPLICIT_COMMITS) -> tuple[str, str, int]:
    """(the ref used, its commit, the commits from it to head). An explicit ref is used as
    given; otherwise the owner's mark, then origin/main, and an implicit one past `limit`
    commits is refused."""
    if explicit:
        sha = commit_of(explicit)
        if sha is None:
            raise BaseError(f"BASE={explicit} is not a commit this repository can resolve")
        return explicit, sha, commits_between(sha, head)
    for ref in (APPLIED_REF, FALLBACK_REF):
        sha = commit_of(ref)
        if sha is None:
            continue
        count = commits_between(sha, head)
        if count > limit:
            raise BaseError(
                f"the implicit base {ref} ({sha[:12]}) is {count} commits behind {head}, more than "
                f"{limit}.\nThe gates would report work the owner already took as this branch's, "
                "mostly as added files, which they cannot check.\nAfter `yoloai apply`, mark what "
                "was taken with `make mark-applied` (REV=<commit> for an earlier one), or name "
                "the base: `make check-pr BASE=<commit>`.")
        return ref, sha, count
    raise BaseError(f"neither {APPLIED_REF} nor {FALLBACK_REF} exists: name the base, "
                    "`make check-pr BASE=<commit>`, or mark it with `make mark-applied`")


def mark(rev: str) -> str:
    """Sets the owner's mark to `rev`, and returns its commit."""
    sha = commit_of(rev)
    if sha is None:
        raise BaseError(f"{rev} is not a commit this repository can resolve")
    done = _git("update-ref", "-m", "orpheum: the owner took the commits up to here", APPLIED_REF, sha)
    if done.returncode != 0:
        raise BaseError(f"could not set {APPLIED_REF}: {done.stderr.strip()}")
    return sha


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base", default="", help="the base to use as given (default: the owner's mark, then origin/main)")
    ap.add_argument("--head", default="HEAD")
    ap.add_argument("--mark", metavar="REV", help=f"set {APPLIED_REF} to REV and exit")
    args = ap.parse_args()
    try:
        if args.mark:
            sha = mark(args.mark)
            print(f"mark-applied: {APPLIED_REF} is now {sha[:12]}; the branch gates compare against it")
            return 0
        ref, sha, count = resolve(args.base or None, args.head)
    except BaseError as problem:
        print(f"branch base: {problem}", file=sys.stderr)
        return 1
    how = "given" if args.base else "implicit"
    print(f"branch base: {ref} ({sha[:12]}, {how}), {count} commit(s) before {args.head}", file=sys.stderr)
    print(sha)
    return 0


if __name__ == "__main__":
    sys.exit(main())
