#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Resolves the base the branch gates (`make check-pr`) compare against: the BASE given, or
# ABOUTME: the newest of what check-pr last passed through, what the owner last took, and
# ABOUTME: origin/main. It prints the base and refuses an implicit base too far back to mean anything.
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

A passing check-pr records the commit it checked as `refs/orpheum/checked`, so the next run starts
where the last one ended, and a batch of work is gated once without anyone marking anything: the
owner, outside the sandbox, could not run `make mark-applied` easily (2026-10-01). The implicit
base is the newest of the two marks and origin/main that HEAD descends from. Every pass moves the
checked mark, from whatever base it was given: a mark held back for a run named to start later
went stale, 189 commits behind, and the runs that named their base instead re-checked forty
commits each time, twelve minutes where three would do (the owner's choice, 2026-10-06). A run
named to start later does mark the commits before its base as checked; each batch is gated
before the owner takes it.

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
CHECKED_REF = "refs/orpheum/checked"
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


def is_ancestor(older: str, newer: str) -> bool:
    return _git("merge-base", "--is-ancestor", older, newer).returncode == 0


def implicit(head: str = "HEAD", refs: tuple[str, ...] = (CHECKED_REF, APPLIED_REF, FALLBACK_REF),
             ) -> tuple[str, str, int] | None:
    """(the ref, its commit, the commits from it to head) of the newest of refs that head descends
    from, or None."""
    found = []
    for ref in refs:
        sha = commit_of(ref)
        if sha is not None and is_ancestor(sha, head):
            found.append((ref, sha, commits_between(sha, head)))
    return min(found, key=lambda candidate: candidate[2]) if found else None


def resolve(explicit: str | None, head: str = "HEAD",
            limit: int = MAX_IMPLICIT_COMMITS) -> tuple[str, str, int]:
    """(the ref used, its commit, the commits from it to head). An explicit ref is used as
    given; otherwise the newest mark (implicit), and one past `limit` commits is refused."""
    if explicit:
        sha = commit_of(explicit)
        if sha is None:
            raise BaseError(f"BASE={explicit} is not a commit this repository can resolve")
        return explicit, sha, commits_between(sha, head)
    newest = implicit(head)
    if newest is None:
        raise BaseError(f"none of {CHECKED_REF}, {APPLIED_REF} or {FALLBACK_REF} is behind {head}: name "
                        "the base, `make check-pr BASE=<commit>`, or mark it with `make mark-applied`")
    ref, sha, count = newest
    if count > limit:
        raise BaseError(
            f"the implicit base {ref} ({sha[:12]}) is {count} commits behind {head}, more than "
            f"{limit}.\nThe gates would report work already checked or taken as this branch's, "
            "mostly as added files, which they cannot check.\nName the base: "
            "`make check-pr BASE=<commit>`; a pass then marks where it ended.")
    return ref, sha, count


def mark_checked(head: str) -> str:
    """Records head as checked after check-pr passed, from whatever base, and returns its commit."""
    head_sha = commit_of(head)
    if head_sha is None:
        raise BaseError(f"{head} is not a commit this repository can resolve")
    done = _git("update-ref", "-m", "orpheum: check-pr passed up to here", CHECKED_REF, head_sha)
    if done.returncode != 0:
        raise BaseError(f"could not set {CHECKED_REF}: {done.stderr.strip()}")
    return head_sha


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
    ap.add_argument("--base", default="", help="the base to use as given (default: the newest of the checked mark, the owner's mark and origin/main)")
    ap.add_argument("--head", default="HEAD")
    ap.add_argument("--mark", metavar="REV", help=f"set {APPLIED_REF} to REV and exit")
    ap.add_argument("--checked", metavar="REV", help=f"after a pass, set {CHECKED_REF} to REV")
    args = ap.parse_args()
    try:
        if args.checked:
            sha = mark_checked(args.checked)
            print(f"branch base: {CHECKED_REF} is now {sha[:12]}; the next check-pr starts there",
                  file=sys.stderr)
            return 0
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
