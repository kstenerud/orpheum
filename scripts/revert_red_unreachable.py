# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Whether a Go file's change only removed declarations nothing could reach, so reverting
# ABOUTME: it restores dead code no test could notice (check_revert_red.py reports it UNREACHABLE).
"""Removed code nothing reached, which revert-red reports rather than probes.

Deleting a function the last caller stopped using is the tidy end of most refactors. Reverting it
restores code nothing calls, so no test can go red, and the gate listed the deletion as a behaviour
change nothing noticed (43b64f3's `withoutLocalKinds`, and sqlc dropping a model whose table went,
2026-09-30). The owner's answer was that the gate should know this case, not be argued past.

The question is exact within its guard, and declines whenever it cannot be sure; declining means
the file is probed as before. The change qualifies when:

* both versions are gofmt'd Go without block comments, split into top-level declarations (each
  with its doc comment) and the lines between them;
* the lines between declarations are unchanged apart from blank and comment lines, the new version
  keeps every declaration it had, unchanged, and only drops some;
* every dropped declaration is a `func` (not `init`), a method, a `type` or a `const` — never a
  `var`, whose initialiser may run code, and none carries a `//go:` directive;
* an import it dropped is not a blank or dot import, whose loss can stop an `init` running;
* a dropped method's type was dropped with it, or the method is unexported and its name is not
  a word anywhere in its package at the new revision.

Why that is enough: the new revision compiles, so nothing it has refers to a dropped name, and Go
has no way to reach a package-level func, type or const by anything but its name. A method is the
exception: re-adding one grows its type's method set, which can change what an interface
conversion or type assertion finds. An unexported method can only satisfy an interface of its own
package, so a package that never says its name cannot be asking for it; an exported one may be
wanted by an interface anywhere, the standard library's included, so it declines.
"""

from __future__ import annotations

import re
import subprocess

GO_SOURCE = re.compile(r"\.go$")
DECL_START = re.compile(r"^(func|type|const|var|import)\b")
FUNC_NAME = re.compile(r"^func\s+(?:\(\s*(?:\w+\s+)?\*?\s*([A-Za-z_]\w*)(?:\[[^\]]*\])?\s*\)\s*)?([A-Za-z_]\w*)")
SINGLE_NAME = re.compile(r"^(?:type|const)\s+([A-Za-z_]\w*)")
GROUP_MEMBER = re.compile(r"^\t([A-Za-z_]\w*)\b")
IMPORT_LINE = re.compile(r'^\s*(?:import\s+)?(?:([\w.]+)\s+)?"([^"]+)"\s*$')


def _git(*args: str, cwd: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], capture_output=True, text=True, cwd=cwd, check=False)


def split_declarations(text: str) -> tuple[list[str], list[str]] | None:
    """A gofmt'd file's top-level declarations, each with the comment lines just above it, and the
    other non-blank, non-comment lines; None when the file is not in the shape this can read."""
    lines = text.splitlines()
    decls: list[str] = []
    others: list[str] = []
    comment: list[str] = []
    i = 0
    while i < len(lines):
        line = lines[i]
        if line.startswith("//"):
            comment.append(line)
            i += 1
            continue
        if DECL_START.match(line):
            block = [*comment, line]
            comment = []
            opener = line.rstrip()[-1:]
            if opener in "{(" and not line.rstrip().endswith(("{}", "()")):
                closer = "}" if opener == "{" else ")"
                i += 1
                while i < len(lines) and not lines[i].startswith(closer):
                    if lines[i] and not lines[i][0].isspace():
                        return None  # a column-0 line inside a block: not gofmt's shape
                    block.append(lines[i])
                    i += 1
                if i == len(lines):
                    return None
                block.append(lines[i])
            decls.append("\n".join(block))
            i += 1
            continue
        comment = []
        if line.strip():
            others.append(line)
        i += 1
    return decls, others


def declared_names(decl: str) -> list[tuple[str, str]] | None:
    """The (name, receiver type) pairs a dropped declaration declared, the receiver '' for all but
    a method; None when dropping it may change behaviour."""
    body = [line for line in decl.splitlines() if not line.startswith("//")]
    if any(line.startswith("//go:") for line in decl.splitlines()):
        return None
    head = body[0]
    if head.startswith("func"):
        name = FUNC_NAME.match(head)
        if not name or (not name.group(1) and name.group(2) == "init"):
            return None
        return [(name.group(2), name.group(1) or "")]
    if head.startswith(("type", "const")):
        if head.rstrip().endswith("("):
            names = [(m.group(1), "") for line in body[1:-1] if (m := GROUP_MEMBER.match(line))]
            return names or None
        name = SINGLE_NAME.match(head)
        return [(name.group(1), "")] if name else None
    return None  # var or import: not a plain dropped declaration


def imports_of(decl: str) -> list[tuple[str, str]] | None:
    """An import declaration's (alias, path) pairs, or None for anything else."""
    body = [line for line in decl.splitlines() if not line.lstrip().startswith("//")]
    if not body[0].startswith("import"):
        return None
    lines = body[1:-1] if body[0].rstrip().endswith("(") else body
    pairs = []
    for line in lines:
        if not line.strip():
            continue
        match = IMPORT_LINE.match(line)
        if not match:
            return None
        pairs.append((match.group(1) or "", match.group(2)))
    return pairs


def dropped_names(before: str, after: str) -> list[tuple[str, str]] | None:
    """The names a change dropped, when all it did was drop declarations that may be dropped;
    otherwise None."""
    if "/*" in before or "/*" in after:
        return None
    split_before, split_after = split_declarations(before), split_declarations(after)
    if split_before is None or split_after is None:
        return None
    (decls_before, others_before), (decls_after, others_after) = split_before, split_after
    if others_before != others_after:
        return None
    imports_before = [pair for d in decls_before if (p := imports_of(d)) is not None for pair in p]
    imports_after = [pair for d in decls_after if (p := imports_of(d)) is not None for pair in p]
    if not set(imports_after) <= set(imports_before):
        return None
    if any(alias in ("_", ".") for alias, _ in set(imports_before) - set(imports_after)):
        return None
    rest_before = [d for d in decls_before if imports_of(d) is None]
    rest_after = [d for d in decls_after if imports_of(d) is None]
    remaining = iter(_code_of(d) for d in rest_before)
    if not all(any(code == _code_of(kept) for code in remaining) for kept in rest_after):
        return None  # a kept declaration changed, or the order did
    kept_codes = {_code_of(d) for d in rest_after}
    names: list[tuple[str, str]] = []
    for decl in rest_before:
        if _code_of(decl) in kept_codes:
            continue
        declared = declared_names(decl)
        if declared is None:
            return None
        names.extend(declared)
    return names or None


def _code_of(decl: str) -> str:
    """A declaration without its comment lines, so rewording a doc comment keeps it the same."""
    return "\n".join(line for line in decl.splitlines() if not line.lstrip().startswith("//"))


def is_unreachable_removal(tree: str, base: str, head: str, path: str) -> list[str]:
    """The dropped names when the change to `path` only removed code nothing reaches, else []."""
    if not GO_SOURCE.search(path):
        return []
    before = _git("show", f"{base}:{path}", cwd=tree).stdout
    after_show = _git("show", f"{head}:{path}", cwd=tree)
    after = after_show.stdout if after_show.returncode == 0 else ""
    if not after:  # the whole file went: all of it must be droppable, its package clause aside
        before = "\n".join(line for line in before.splitlines() if not line.startswith("package "))
    names = dropped_names(before, after)
    if not names:
        return []
    dropped_types = {name for name, receiver in names if not receiver}
    package = path.rsplit("/", 1)[0] + "/" if "/" in path else ""
    for name, receiver in names:
        if not receiver or receiver in dropped_types:
            continue
        if name[0].isupper():
            return []  # an interface anywhere may want it
        used = _git("grep", "-q", "-w", "-F", "-e", name, head, "--", f"{package}*.go", cwd=tree)
        if used.returncode != 1:  # 0: its package says the name; anything else: could not tell
            return []
    return [name for name, _ in names]
