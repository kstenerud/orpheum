#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Documentation integrity gate: links and anchors resolve, every decision is defined
# ABOUTME: once, a decision link lands on its decision, and live docs never cite an archived one;
# ABOUTME: a decision is never recorded with a Status that says it is already built, and no
# ABOUTME: document cites a commit of this repository by its hash.
"""Check the Markdown tree's links and decision citations. Exit 1 on any failure.

Run from anywhere: ``python3 scripts/check_docs.py``; ``--write-index`` first regenerates
``decisions/index.md``, which the check otherwise fails on when it is stale. Tests: ``python3 -m unittest
discover scripts/tests``.

Six checks, each for a failure that has already happened here or in yoloAI, whose
repo-hygiene gates this follows:

1. **Links resolve.** Every relative link names a file that exists.
2. **Anchors resolve.** A ``#fragment`` names a heading the target defines, slugged the way
   GitHub slugs it. Hand-slugging produced wrong anchors in this tree more than once.
3. **Decisions are defined once, with no gaps.** ``## Dn — `` headings across the live log and
   its archive cover 1..max exactly once. Numbers are never reused, and a gap means an entry
   was lost in a move.
4. **A decision link lands on its decision.** ``[D12](...)`` must point at the file that
   defines D12, and at D12's anchor if it has one. This is what keeps citations right when an
   entry moves to the archive, and it catches ``[D12](#d15--...)`` copy-paste slips.
5. **Live docs do not cite archived decisions.** Outside ``decisions/`` and
   ``design/research/`` (dated records of what was known then), any mention of an archived
   decision, linked or bare, fails. An archived decision no longer describes the design, so a
   plan citing one is probably asserting something that is no longer true. The failure message
   names the successor from the entry's archive note.
6. **A decision is not built when it is recorded.** A decision added to the live log may not
   have a Status saying it is built ("Decided; built in M3"): no code can have been checked
   against it yet. D226, D227 and D228 each said "built" in the commit that recorded them, and
   D227's clauses added later were never built by that word (audit-2026-09-29 §6, P31). The
   Status is changed to built by a later commit, once its clauses are read back against the
   code. `make check` compares the working tree's log with HEAD's; with ``--base <ref>``
   (`make check-decision-status`, part of `make check-pr`) each commit on the branch is
   compared with its parent, and nothing else is checked.
7. **A commit is cited by its subject, not its hash.** A hash that names a commit of this
   repository changes when `yoloai apply` replays the commit and again in any rewrite, so it is
   wrong everywhere but where it was written (agent-failures A64; ``commit_hashes.py``).

Deliberately not checked: external URLs (the network is not a gate), and bare ``Dn``
mentions *resolving* (they are checked only against the archive list and the maximum).
"""

from __future__ import annotations

import re
import subprocess
import sys
import unicodedata
from pathlib import Path

from commit_hashes import candidates, cited_commits, subject_of

ROOT = Path(__file__).resolve().parent.parent
DECISIONS = "docs/contributors/decisions/"
LIVE_LOG = DECISIONS + "working-notes.md"
ARCHIVE_LOG = DECISIONS + "working-notes-archive.md"
INDEX = DECISIONS + "index.md"
# Dated records: allowed to cite what was current when they were written.
HISTORY_PREFIXES = (DECISIONS, "docs/contributors/design/research/")

LINK_RE = re.compile(r"(?<!!)\[((?:[^\[\]]|\[[^\]]*\])*)\]\(([^()\s]*(?:\([^()\s]*\)[^()\s]*)*)\)")
IMAGE_RE = re.compile(r"!\[[^\]]*\]\(([^)\s]+)\)")
CODE_SPAN_RE = re.compile(r"(`+)(?:(?!\1).)+?\1")
DECISION_HEADING_RE = re.compile(r"^## D(\d+) — ", re.M)
DECISION_TEXT_RE = re.compile(r"^D(\d+)$")
DECISION_ANCHOR_RE = re.compile(r"^d(\d+)--")
BARE_DECISION_RE = re.compile(r"(?<![\w/#-])D(\d{1,4})(?![\w-])")
EXPLICIT_ANCHOR_RE = re.compile(r"""<a\s+[^>]*(?:name|id)=["']([^"']+)["']""")
ARCHIVE_NOTE_RE = re.compile(r"^> \*\*Archived [^*]*\*\*(.*)$", re.M)
STATUS_RE = re.compile(r"(?ms)^\*\*Status:?\*\*(.*?)(?=\n\n|\Z)")
BUILT_RE = re.compile(r"\bbuilt\b", re.I)
# Words before "built" that say it is not built yet: "not built", "to be built", "being built".
NOT_YET_BUILT = {"not", "be", "being", "nothing", "never", "unbuilt", "yet"}


def strip_code(body: str, keep_spans: bool = False) -> list[tuple[int, str]]:
    """Return (line number, text) for lines outside fenced blocks, with code spans blanked
    unless keep_spans (headings keep them: GitHub slugs a code span's text).

    Markdown shown as an example is prose about Markdown, not a claim the document makes.
    """
    out, fence = [], None
    for i, line in enumerate(body.split("\n"), 1):
        m = re.match(r"^\s*(`{3,}|~{3,})", line)
        if m:
            marker = m.group(1)[0] * 3
            if fence is None:
                fence = marker
            elif marker == fence:
                fence = None
            continue
        if fence is None:
            out.append((i, line if keep_spans else CODE_SPAN_RE.sub(lambda s: " " * len(s.group(0)), line)))
    return out


def slug(heading: str) -> str:
    """GitHub's heading anchor: link syntax to its text, lowercase, keep letters, digits,
    spaces, hyphens and underscores, then spaces to hyphens. Punctuation is dropped, not
    hyphenated, which is why ' — ' becomes '--'."""
    text = LINK_RE.sub(lambda m: m.group(1), heading)
    text = re.sub(r"<[^>]+>", "", text)
    out = []
    for ch in text.strip().lower():
        if ch == " ":
            out.append("-")
        elif ch in "-_" or ch.isalnum() or unicodedata.category(ch).startswith("M"):
            out.append(ch)
    return "".join(out)


def anchors(body: str) -> set[str]:
    found: set[str] = set()
    seen: dict[str, int] = {}
    for _, line in strip_code(body, keep_spans=True):
        for m in EXPLICIT_ANCHOR_RE.finditer(line):
            found.add(m.group(1))
        m = re.match(r"^ {0,3}(#{1,6})\s+(.*?)\s*#*\s*$", line)
        if not m:
            continue
        s = slug(m.group(2))
        n = seen.get(s, 0)
        found.add(s if n == 0 else f"{s}-{n}")
        seen[s] = n + 1
    return found


def links(body: str) -> list[tuple[int, str, str]]:
    """(line, text, target) for every inline link and image outside code."""
    out = []
    for no, line in strip_code(body):
        for m in LINK_RE.finditer(line):
            out.append((no, m.group(1), m.group(2)))
        for m in IMAGE_RE.finditer(line):
            out.append((no, "", m.group(1)))
    return out


def is_external(target: str) -> bool:
    return bool(re.match(r"^[a-z][a-z0-9+.-]*:", target, re.I))


def tracked_markdown() -> list[str]:
    out = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "*.md"],
        cwd=ROOT, check=True, capture_output=True, text=True,
    ).stdout.split()
    return sorted(p for p in out if (ROOT / p).exists())


def decision_sites(texts: dict[str, str]) -> dict[int, list[str]]:
    sites: dict[int, list[str]] = {}
    for rel in (LIVE_LOG, ARCHIVE_LOG):
        for m in DECISION_HEADING_RE.finditer(texts.get(rel, "")):
            sites.setdefault(int(m.group(1)), []).append(rel)
    return sites


def archive_notes(archive: str) -> dict[int, str]:
    notes = {}
    for chunk in re.split(r"(?m)^(?=## D\d+ — )", archive)[1:]:
        n = int(re.match(r"## D(\d+)", chunk).group(1))
        m = ARCHIVE_NOTE_RE.search(chunk)
        succ = re.findall(r"\[(D\d+)\]", m.group(1)) if m else []
        notes[n] = ", ".join(dict.fromkeys(succ)) or "(no successor named)"
    return notes


REVISE_VERB = (
    r"(?:supersed\w*|drops?|amend\w*|refine\w*|correct\w*|closes?|closed|replac\w*|reshap\w*"
    r"|overturn\w*|retire\w*|withdr\w*|extend\w*|defers?|revis\w*|revers\w*|narrow\w*"
    r"|simplif\w*|layered)"
)


def entries(log: str) -> dict[int, str]:
    return {
        int(re.match(r"## D(\d+)", c).group(1)): c
        for c in re.split(r"(?m)^(?=## D\d+ — )", log)[1:]
    }


def revisits(ents: dict[int, str]) -> dict[int, set[int]]:
    """Later decisions that appear to revisit each one. A heuristic, and labelled as one: an
    entry's own Status line, blockquotes and strike marks naming a later number, plus a later
    entry saying it supersedes, amends, refines (and so on) an earlier one. It over-reports
    rather than under-reports, because a missed amendment is the expensive error."""
    rev: dict[int, set[int]] = {n: set() for n in ents}
    for n, chunk in ents.items():
        lines = chunk.split("\n")
        m = re.search(r"(?ms)^\*\*Status:?\*\*(.*?)(?=\n\n|\Z)", chunk)
        own = [m.group(1) if m else ""]
        own += [" ".join(lines[i:i + 3]) for i, l in enumerate(lines) if l.startswith(">") or "~~" in l]
        rev[n] |= {int(x) for x in re.findall(r"\bD(\d+)\b", " ".join(own)) if int(x) > n}
        flat = " ".join(chunk.split())
        for pat in (
            REVISE_VERB + r"[^.;]{0,60}?\[?D(\d+)\b",
            r"\[?D(\d+)\b(?:\]\([^)]*\))?(?:'s)?[^.;]{0,40}?\b(?:is|was|are|now)\s+(?:now\s+)?" + REVISE_VERB,
            r"\bD(\d+)\W{0,3}" + REVISE_VERB,
        ):
            for t in re.findall(pat, flat, re.I):
                if int(t) < n and int(t) in rev:
                    rev[int(t)].add(n)
    return rev


def render_index(live: str, archive: str) -> str:
    live_ents, arch_ents = entries(live), entries(archive)
    rev = revisits(live_ents)
    succ = archive_notes(archive)
    out = [
        "> **ABOUTME:** Generated index of the decision log — one line per decision, with the later",
        "> entries that appear to revisit it. Rebuild with `python3 scripts/check_docs.py --write-index`.",
        "",
        "# Decision index",
        "",
        "Generated from [working-notes.md](working-notes.md) and",
        "[working-notes-archive.md](working-notes-archive.md); do not edit by hand. **Revisited by** is",
        "found by pattern, not judgement: it over-reports, so read the entries it names before relying",
        "on a decision, and treat an empty one as *probably* untouched.",
        "",
        "## Live",
        "",
    ]
    for n in sorted(live_ents):
        heading = live_ents[n].split("\n", 1)[0][3:]
        title = heading.split(" — ", 1)[1]
        line = f"- [D{n}](working-notes.md#{slug(heading)}) {title}"
        if rev[n]:
            line += " — *revisited by " + ", ".join(f"D{x}" for x in sorted(rev[n])) + "*"
        out.append(line)
    out += ["", "## Archived", ""]
    for n in sorted(arch_ents):
        heading = arch_ents[n].split("\n", 1)[0][3:]
        title = heading.split(" — ", 1)[1]
        out.append(f"- [D{n}](working-notes-archive.md#{slug(heading)}) {title} — *see {succ.get(n)}*")
    return "\n".join(out) + "\n"


def check(files: list[str], texts: dict[str, str]) -> list[str]:
    errors: list[str] = []
    anchor_cache: dict[str, set[str]] = {}
    if INDEX in texts and texts[INDEX] != render_index(texts.get(LIVE_LOG, ""), texts.get(ARCHIVE_LOG, "")):
        errors.append(f"{INDEX} is out of date with the log; run: python3 scripts/check_docs.py --write-index")

    def anchors_of(rel: str) -> set[str] | None:
        if rel not in anchor_cache:
            p = ROOT / rel
            if not p.is_file():
                return None
            anchor_cache[rel] = anchors(texts.get(rel) or p.read_text(encoding="utf-8"))
        return anchor_cache[rel]

    sites = decision_sites(texts)
    # Check 3: once each, no gaps.
    for n, where in sorted(sites.items()):
        if len(where) > 1:
            errors.append(f"D{n} is defined {len(where)} times ({', '.join(where)}); numbers are never reused")
    top = max(sites, default=0)
    for n in range(1, top + 1):
        if n not in sites:
            errors.append(f"D{n} is defined nowhere; entries D1..D{top} must all exist in the live log or the archive")
    archived = {n for n, w in sites.items() if w == [ARCHIVE_LOG]}
    successors = archive_notes(texts.get(ARCHIVE_LOG, ""))
    for n in archived:
        if successors.get(n, "(no successor named)") == "(no successor named)":
            errors.append(f"{ARCHIVE_LOG}: D{n} has no '> **Archived <date>.** ...' note naming its successor")
    decision_anchor: dict[int, str] = {}
    for rel in (LIVE_LOG, ARCHIVE_LOG):
        for line in texts.get(rel, "").split("\n"):
            m = re.match(r"^## (D(\d+) — .*)$", line)
            if m:
                decision_anchor[int(m.group(2))] = slug(m.group(1))

    for rel in files:
        body = texts[rel]
        base = (ROOT / rel).parent
        history = rel.startswith(HISTORY_PREFIXES)
        for no, text, target in links(body):
            if is_external(target):
                continue
            path, _, frag = target.partition("#")
            dest = (base / path).resolve() if path else (ROOT / rel).resolve()
            try:
                dest_rel = dest.relative_to(ROOT).as_posix()
            except ValueError:
                errors.append(f"{rel}:{no}: link {target!r} leaves the repository")
                continue
            # Check 1.
            if not dest.exists():
                errors.append(f"{rel}:{no}: link {target!r} names {dest_rel}, which does not exist")
                continue
            # Check 2.
            if frag and dest_rel.endswith(".md"):
                have = anchors_of(dest_rel)
                if have is not None and frag not in have:
                    errors.append(f"{rel}:{no}: {dest_rel} has no heading with anchor #{frag}")
            # Check 4.
            dm = DECISION_TEXT_RE.match(text.strip())
            am = DECISION_ANCHOR_RE.match(frag)
            if dest_rel in (LIVE_LOG, ARCHIVE_LOG) and (dm or am):
                n = int(dm.group(1)) if dm else int(am.group(1))
                where = sites.get(n)
                if not where:
                    errors.append(f"{rel}:{no}: [{text}] cites D{n}, which does not exist")
                elif dest_rel not in where:
                    errors.append(f"{rel}:{no}: [{text}]({target}) points at {dest_rel}, but D{n} is in {where[0]}")
                elif dm and am and int(am.group(1)) != n:
                    errors.append(f"{rel}:{no}: [{text}] links to D{am.group(1)}'s anchor, not D{n}'s")
                elif dm and frag and frag != decision_anchor.get(n):
                    errors.append(f"{rel}:{no}: [{text}] has anchor #{frag}, but D{n}'s is #{decision_anchor.get(n)}")
        # Check 5, plus bare numbers beyond the log.
        for no, line in strip_code(body):
            for m in BARE_DECISION_RE.finditer(line):
                n = int(m.group(1))
                if n > top and top:
                    errors.append(f"{rel}:{no}: D{n} is beyond the last decision (D{top})")
                elif n in archived and not history:
                    errors.append(
                        f"{rel}:{no}: cites archived D{n}; cite its successor ({successors.get(n)}) "
                        f"and check the sentence still holds under it"
                    )
            for _, text, target in links(line):
                am = DECISION_ANCHOR_RE.match(target.partition("#")[2])
                if am and int(am.group(1)) in archived and not history and not DECISION_TEXT_RE.match(text.strip()):
                    errors.append(
                        f"{rel}:{no}: links archived D{am.group(1)} as [{text}]; cite its successor "
                        f"({successors.get(int(am.group(1)))}) instead"
                    )
    errors += cited_hash_errors(files, texts)
    return errors


def cited_hash_errors(files: list[str], texts: dict[str, str]) -> list[str]:
    """Check 7: a document cites a commit by its subject, not its hash, which changes when the
    commit leaves the sandbox (agent-failures A64)."""
    found = set(cited_commits("\n".join(texts[rel] for rel in files)))
    errors: list[str] = []
    if not found:
        return errors
    for rel in files:
        for no, line in enumerate(texts[rel].split("\n"), 1):
            for token in candidates(line):
                if token in found:
                    errors.append(
                        f"{rel}:{no}: cites commit {token} by its hash, which `yoloai apply` and any "
                        f'rewrite change; cite its subject instead: "{subject_of(token)}"'
                    )
    return errors


def claims_built(status: str) -> bool:
    """Whether a Status paragraph says the decision is built, in whole or in part."""
    for m in BUILT_RE.finditer(status):
        before = re.findall(r"[a-z']+", status[:m.start()].lower())[-2:]
        if not NOT_YET_BUILT & set(before):
            return True
    return False


def built_on_arrival(before_log: str, after_log: str) -> list[int]:
    """Decisions the after log adds whose Status already says they are built."""
    before, after = entries(before_log), entries(after_log)
    found = []
    for n in sorted(set(after) - set(before)):
        m = STATUS_RE.search(after[n])
        if m and claims_built(m.group(1)):
            found.append(n)
    return found


def built_on_arrival_message(n: int, where: str) -> str:
    return (f"{where}: D{n} is recorded with a Status saying it is built. Record it as decided; "
            "a later commit says built, once its clauses are read back against the code (P31)")


def _git(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], cwd=ROOT, capture_output=True, text=True, check=False)


def check_branch(base: str) -> int:
    """Check 6 over every commit in base..HEAD that changed the live log."""
    listed = _git("rev-list", "--reverse", f"{base}..HEAD", "--", LIVE_LOG)
    if listed.returncode != 0:
        print(f"check_docs: {base}..HEAD is not a range git can list: {listed.stderr.strip()}")
        return 1
    errors = []
    for commit in listed.stdout.split():
        after = _git("show", f"{commit}:{LIVE_LOG}").stdout
        parent = _git("show", f"{commit}^:{LIVE_LOG}")
        before = parent.stdout if parent.returncode == 0 else ""
        errors += [built_on_arrival_message(n, f"{commit[:12]} {LIVE_LOG}") for n in built_on_arrival(before, after)]
    for e in errors:
        print(e)
    print(f"check_docs: no decision recorded as built in {base}..HEAD" if not errors
          else f"check_docs: {len(errors)} decision(s) recorded as built", file=sys.stderr)
    return 1 if errors else 0


def main() -> int:
    if "--base" in sys.argv[1:]:
        return check_branch(sys.argv[sys.argv.index("--base") + 1])
    if "--write-index" in sys.argv[1:]:
        live = (ROOT / LIVE_LOG).read_text(encoding="utf-8")
        arch = (ROOT / ARCHIVE_LOG).read_text(encoding="utf-8") if (ROOT / ARCHIVE_LOG).exists() else ""
        (ROOT / INDEX).write_text(render_index(live, arch), encoding="utf-8")
    files = tracked_markdown()
    if not files:
        print("check_docs: no Markdown files found — the gate is checking nothing", file=sys.stderr)
        return 1
    texts = {rel: (ROOT / rel).read_text(encoding="utf-8") for rel in files}
    errors = check(files, texts)
    committed = _git("show", f"HEAD:{LIVE_LOG}")
    if committed.returncode == 0 and LIVE_LOG in texts:
        errors += [built_on_arrival_message(n, LIVE_LOG) for n in built_on_arrival(committed.stdout, texts[LIVE_LOG])]
    nlinks = sum(len(links(t)) for t in texts.values())
    for e in errors:
        print(e)
    print(f"check_docs: {len(files)} files, {nlinks} links, {len(errors)} problems", file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
