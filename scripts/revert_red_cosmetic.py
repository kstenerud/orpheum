# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Whether a file's change cannot affect behaviour (comments, formatting, a struct tag's
# ABOUTME: doc text), so revert-red reports it COSMETIC rather than probing it (check_revert_red.py).
"""Changes no test could notice, which revert-red reports rather than probes."""

from __future__ import annotations

import ast
import re
import subprocess

GO_SOURCE = re.compile(r"\.go$")
PY_SOURCE = re.compile(r"\.py$")

# A Go struct field with a tag: its name(s) and type (or an embedded type), the tag, and at most
# a line comment after it. `return` is excluded: `return `...`` is the one statement of that shape.
GO_TAGGED_FIELD = re.compile(r"^\s*(?!return\b)([\w.*]+(?:\s*,\s*\w+)*(?:\s+[^`=:]*?)?)\s*`([^`]*)`\s*(?://.*)?$")
# A whole struct tag, in the form reflect.StructTag reads: key:"quoted value" pairs.
GO_TAG = re.compile(r'^(?:[A-Za-z_][\w-]*:"(?:[^"\\]|\\.)*"(?: +|$))+$')
# huma's `doc:"..."`: the field's description in the API document, which no code reads.
GO_DOC_VALUE = re.compile(r'(^| )doc:"(?:[^"\\]|\\.)*"')


def _git(*args: str, cwd: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], capture_output=True, text=True, cwd=cwd, check=False)


def changed_lines(base: str, head: str, path: str, cwd: str | None = None) -> list[str]:
    """The added and removed content lines for one path, without their +/- marker.

    `-U0` so context lines are not mistaken for changes, and `--ignore-all-space`
    so a reindent is not either.
    """
    out = _git(
        "diff", "-U0", "--ignore-all-space", f"{base}...{head}", "--", path, cwd=cwd
    ).stdout
    lines = []
    for line in out.splitlines():
        if line.startswith(("+++", "---", "@@", "diff ", "index ", "new file", "deleted file")):
            continue
        if line.startswith(("+", "-")):
            lines.append(line[1:])
    return lines


def is_cosmetic(tree: str, base: str, head: str, path: str) -> bool:
    """Whether the change to `path` cannot affect behaviour, so no test could notice.

    A `feat` commit that adjusts a doc comment alongside its real work leaves this
    gate reporting the comment's file as an uncovered behaviour change, because
    reverting a comment of course breaks nothing. That is a false positive, and this
    gate's whole claim on a contributor's attention is that it does not produce them
    — yoloAI's specimen was a two-line doc-comment edit on `applyBrokerOption`,
    reported identically to a real missing test.

    Both tests below are *exact within their guard* and decline to classify whenever
    they cannot be sure. Declining means the file is probed as before, so the worst
    case is the behaviour we already had.

    * **Go**: every changed line is a `//` comment or blank, and neither version of
      the file contains `/*` anywhere. Without that second condition a changed line
      inside a block comment is indistinguishable from code by inspection, and a
      guard that reads `*p = 0` as a comment would suppress a real finding. Or, under
      the same guard, the two versions differ only in comments and in the text of struct
      tags' `doc:"..."` values (`same_but_doc_text`).
    * **Python**: the two versions parse to the same AST once function and class
      docstrings are left out (`_code_of`). That covers `#` comments, formatting and
      those docstrings together. A module's docstring stays in: the gate scripts hand
      it to argparse as their `--help` (`description=__doc__`), so rewording it changes
      what a person is shown. A docstring of a function or class is read by nothing in
      them; one rewording a renamed path in fetch_ffmpeg.py's `tools_dir` failed a
      check-pr as a change no test covered (2026-10-06).
    """
    if GO_SOURCE.search(path):
        before = _git("show", f"{base}:{path}", cwd=tree).stdout
        after = _git("show", f"{head}:{path}", cwd=tree).stdout
        if "/*" in before or "/*" in after:
            return False  # cannot tell a block-comment line from code
        changed = changed_lines(base, head, path, cwd=tree)
        return bool(changed) and (all(
            not line.strip() or line.strip().startswith("//") for line in changed
        ) or same_but_doc_text(before, after))

    if PY_SOURCE.search(path):
        try:
            before = _code_of(_git("show", f"{base}:{path}", cwd=tree).stdout)
            after = _code_of(_git("show", f"{head}:{path}", cwd=tree).stdout)
        except SyntaxError:
            return False  # unparseable on either side; let the probe speak
        return before == after

    return False


def _code_of(source: str) -> str:
    """A Python file's syntax tree without its functions' and classes' docstrings, as text to compare.

    A docstring is the first statement of a body when it is a string constant. One that is a
    body's only statement is kept, as `pass` would have to take its place.
    """
    tree = ast.parse(source)
    for node in ast.walk(tree):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)) and len(node.body) > 1:
            first = node.body[0]
            if isinstance(first, ast.Expr) and isinstance(first.value, ast.Constant) and isinstance(
                first.value.value, str
            ):
                node.body = node.body[1:]
    return ast.dump(tree)


def same_but_doc_text(before: str, after: str) -> bool:
    """Whether two versions of a Go file differ only in comment lines, in the line comment after a
    struct tag, and in the text of a struct tag's `doc:"..."` value.

    huma reads `doc` for the field's description in the API document and nothing else, so
    rewording it changes api/openapi.json's prose and no behaviour. Everything else in the tag
    (the json name, pattern, enum, required, a second doc key's position) must match, and a line
    this cannot read as a whole tag is compared as it stands, so the answer declines to "no".
    """
    return _without_doc_text(before) == _without_doc_text(after)


def _without_doc_text(text: str) -> list[str]:
    lines = []
    for line in text.splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("//"):
            continue
        field = GO_TAGGED_FIELD.match(line)
        if field and GO_TAG.match(field.group(2)):
            lines.append(" ".join(field.group(1).split()) + " `" + GO_DOC_VALUE.sub(r'\1doc:""', field.group(2)) + "`")
        else:
            lines.append(" ".join(stripped.split()))
    return lines
