# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The browser map: which browser tests could notice a stylesheet change, from what each test's
# ABOUTME: pages were seen to hold (web/e2e/browser-map.ts) and what the changed rules select.
"""Which browser tests a stylesheet change can reach.

check-pr's browser run records, for every test in both browsers, every tag, class, id and
attribute name its pages ever held (web/e2e/browser-map.ts). A CSS rule can only have styled a
page that held every token each of its selector's compounds names (`.a .b:hover > .c[x]` needs an
element with class `c` and attribute `x`, and something holding class `a` and `b`), so a test
whose pages never held them all cannot have been styled by it, in either version of the sheet. A change whose changed rules no test's pages
could match cannot change any browser test's outcome, and needs no browser run to say so.

What any page might hold is assumed reached: a rule whose selector names nothing (`:root`,
`*`, `::backdrop` alone), an at-rule that is not a block of ordinary rules (`@keyframes`,
`@font-face`, `@property`, `@import`), a selector this reader does not take apart (an escape, a
namespace), and a stylesheet it cannot read. The map is used only when it was recorded at the
commit being checked, by a browser run of every test that passed (`complete`); otherwise every test
is reached.
"""

from __future__ import annotations

import difflib
import json
import subprocess
from dataclasses import dataclass
from pathlib import Path

# Where check-pr's browser run records the map (Makefile, BROWSER_MAP_DIR).
BROWSER_MAP = Path(__file__).resolve().parent.parent / ".dev" / "browser-map"
# What a browser run in only some tests is told to run (web/e2e/playwright.config.ts).
ONLY_THESE_TESTS = "ORPHEUM_E2E_ONLY"
NO_TEST_REACHES = ("no browser test's pages held what its changed rules select (the browser map), so no "
                   "test can notice its revert")
# Written by the recording run's last step, only when every test passed (Makefile, test-browser).
COMPLETE = "complete"
# The at-rules whose blocks hold ordinary rules: a change of their condition changes the rules inside.
GROUPING_AT_RULES = ("@media", "@supports", "@container", "@layer", "@scope", "@starting-style")


@dataclass(frozen=True)
class Rule:
    """A style rule: the grouping at-rules around it, its selector list and its declarations."""

    context: tuple[str, ...]
    selector: str
    body: str


class Unreadable(Exception):
    """A stylesheet this reader cannot take apart; every test is then reached."""


def strip_comments(text: str) -> str:
    """The text without /* */ comments, strings kept whole."""
    out: list[str] = []
    i, quote = 0, ""
    while i < len(text):
        c = text[i]
        if quote:
            out.append(c)
            if c == "\\" and i + 1 < len(text):
                out.append(text[i + 1])
                i += 1
            elif c == quote:
                quote = ""
        elif c in "\"'":
            quote = c
            out.append(c)
        elif text.startswith("/*", i):
            end = text.find("*/", i + 2)
            if end < 0:
                raise Unreadable("an unclosed comment")
            i = end + 2
            continue
        else:
            out.append(c)
        i += 1
    if quote:
        raise Unreadable("an unclosed string")
    return "".join(out)


def _block_end(text: str, start: int) -> int:
    """The index of the `}` closing the block whose `{` is at start."""
    depth, quote, i = 0, "", start
    while i < len(text):
        c = text[i]
        if quote:
            if c == "\\":
                i += 1
            elif c == quote:
                quote = ""
        elif c in "\"'":
            quote = c
        elif c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return i
        i += 1
    raise Unreadable("an unclosed block")


def _normal(text: str) -> str:
    return " ".join(text.split())


def rules(text: str) -> list[Rule]:
    """The sheet's style rules, in order."""
    found: list[Rule] = []
    _collect(strip_comments(text), (), found)
    return found


def _collect(text: str, context: tuple[str, ...], found: list[Rule]) -> None:
    i = 0
    while i < len(text):
        brace = text.find("{", i)
        semicolon = text.find(";", i)
        if brace < 0:
            return
        if 0 <= semicolon < brace and text[i:semicolon].strip().startswith("@"):
            i = semicolon + 1
            continue
        head = _normal(text[i:brace])
        end = _block_end(text, brace)
        if head.startswith("@"):
            if _is_grouping(head):
                _collect(text[brace + 1:end], (*context, head), found)
        elif head:
            body = text[brace + 1:end]
            if "{" in body:
                # Nesting: the rules inside select relative to this one, which this reader does not do.
                raise Unreadable(f"a nested rule in {head}")
            found.append(Rule(context, head, _normal(body)))
        i = end + 1


def _is_grouping(head: str) -> bool:
    return head.split(" ", 1)[0].split("(", 1)[0].lower() in GROUPING_AT_RULES


def changed_rules(base: str, head: str) -> tuple[list[Rule], bool]:
    """The rules one version has where the other does not, and whether the change reaches every
    test by touching what is not an ordinary rule (an at-rule like @keyframes, @font-face, @import).

    The rules are compared as sequences, since order decides the cascade: of two rules whose order
    changed, at least one is outside what both versions share in order, so it counts as changed, and
    either rule alone can only style what it matches."""
    base_rules, head_rules = rules(base), rules(head)
    matcher = difflib.SequenceMatcher(None, base_rules, head_rules, autojunk=False)
    changed: list[Rule] = []
    for operation, base_start, base_end, head_start, head_end in matcher.get_opcodes():
        if operation != "equal":
            changed.extend(base_rules[base_start:base_end])
            changed.extend(head_rules[head_start:head_end])
    return changed, _non_rule_parts(base) != _non_rule_parts(head)


def _non_rule_parts(text: str) -> list[str]:
    """What of a sheet is not an ordinary rule, in order: whether a change touched any of it."""
    parts: list[str] = []
    _non_rules(strip_comments(text), parts)
    return parts


def _non_rules(text: str, parts: list[str]) -> None:
    i = 0
    while i < len(text):
        brace = text.find("{", i)
        semicolon = text.find(";", i)
        if brace < 0:
            rest = _normal(text[i:])
            if rest.startswith("@"):
                parts.append(rest)
            return
        if 0 <= semicolon < brace and text[i:semicolon].strip().startswith("@"):
            parts.append(_normal(text[i:semicolon]))
            i = semicolon + 1
            continue
        head = _normal(text[i:brace])
        end = _block_end(text, brace)
        if head.startswith("@"):
            if _is_grouping(head):
                _non_rules(text[brace + 1:end], parts)
            else:
                parts.append(head + "{" + _normal(text[brace + 1:end]) + "}")
        i = end + 1


def split_top(text: str, separators: str) -> list[str]:
    """text split at the characters in separators that are outside (), [] and strings."""
    parts: list[str] = []
    depth, quote, start = 0, "", 0
    for i, c in enumerate(text):
        if quote:
            if c == quote:
                quote = ""
        elif c in "\"'":
            quote = c
        elif c in "([":
            depth += 1
        elif c in ")]":
            depth -= 1
        elif depth == 0 and c in separators:
            parts.append(text[start:i])
            start = i + 1
    parts.append(text[start:])
    return parts


def selector_tokens(selector: str) -> frozenset[str] | None:
    """The tokens a page must hold for one complex selector to match anything in it: every
    compound's tag, classes, ids and attribute names, since each compound matches some element
    (`.a > :first-child` needs class `a` held, by the parent). None when it names nothing, or when it
    is one this reader does not take apart: it may then style anything."""
    if "\\" in selector or "|" in selector.replace("|=", ""):
        return None
    tokens: set[str] = set()
    for compound in split_top(selector.strip(), " >+~"):
        if compound:
            named = compound_tokens(compound)
            if named is None:
                return None
            tokens |= named
    return frozenset(tokens) or None


def compound_tokens(compound: str) -> set[str] | None:
    """The tag, classes, ids and attribute names one compound selector names; None when it holds
    something this reader does not know."""
    tokens: set[str] = set()
    i = 0
    while i < len(compound):
        c = compound[i]
        if c in ".#":
            j = i + 1
            while j < len(compound) and (compound[j].isalnum() or compound[j] in "-_"):
                j += 1
            if j == i + 1:
                return None
            tokens.add(("class:" if c == "." else "id:") + compound[i + 1:j])
            i = j
        elif c == "[":
            end = _matching(compound, i, "[", "]")
            name = compound[i + 1:end].strip()
            for operator in ("~=", "|=", "^=", "$=", "*=", "="):
                name = name.split(operator, 1)[0]
            tokens.add("attr:" + name.strip().lower())
            i = end + 1
        elif c == ":":
            # A pseudo-class or pseudo-element, with whatever it holds in parentheses: it narrows what
            # matches but requires no token this reader relies on.
            j = i + 1
            while j < len(compound) and (compound[j] == ":" or compound[j].isalnum() or compound[j] == "-"):
                j += 1
            if j < len(compound) and compound[j] == "(":
                j = _matching(compound, j, "(", ")") + 1
            i = j
        elif c == "*":
            i += 1
        elif c.isalpha():
            j = i
            while j < len(compound) and (compound[j].isalnum() or compound[j] == "-"):
                j += 1
            tokens.add("tag:" + compound[i:j].lower())
            i = j
        else:
            return None
    return tokens


def _matching(text: str, start: int, opening: str, closing: str) -> int:
    depth = 0
    for i in range(start, len(text)):
        if text[i] == opening:
            depth += 1
        elif text[i] == closing:
            depth -= 1
            if depth == 0:
                return i
    raise Unreadable(f"an unclosed {opening}")


def requirements(base: str, head: str) -> list[frozenset[str]] | None:
    """What an element must hold for some changed rule to style it, one set per selector of each
    changed rule; None when the change may reach any test."""
    try:
        changed, global_ = changed_rules(base, head)
    except Unreadable:
        return None
    if global_:
        return None
    needed: list[frozenset[str]] = []
    for rule in changed:
        for selector in split_top(rule.selector, ","):
            try:
                tokens = selector_tokens(selector)
            except Unreadable:
                return None
            if tokens is None:
                return None
            needed.append(tokens)
    return needed


@dataclass(frozen=True)
class Recorded:
    """One test's record: the string Playwright's grep matches it by, and every token its pages held,
    or None when the recording could not vouch for them (a context it did not watch): it is then
    reached by every change."""

    test: str
    tokens: frozenset[str] | None


def load(map_dir: Path, commit: str) -> list[Recorded] | None:
    """The map recorded at commit, or None when there is none, or it is incomplete or another's."""
    stamp = map_dir / COMPLETE
    if not stamp.is_file() or stamp.read_text().strip() != commit:
        return None
    recorded: list[Recorded] = []
    for path in sorted(map_dir.glob("*/*.json")):
        entry = json.loads(path.read_text())
        tokens = entry["tokens"]
        recorded.append(Recorded(entry["test"], None if tokens is None else frozenset(tokens)))
    return recorded or None


def reached(recorded: list[Recorded], changes: list[tuple[str, str]]) -> list[str] | None:
    """The tests that could notice the changes, each a (base, head) pair of one sheet's text; None
    when they may reach any test."""
    needed: list[frozenset[str]] = []
    for base, head in changes:
        sheet = requirements(base, head)
        if sheet is None:
            return None
        needed.extend(sheet)
    return [record.test for record in recorded
            if record.tokens is None or any(tokens <= record.tokens for tokens in needed)]


def _show(tree: str, revision: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", "show", revision], cwd=tree, capture_output=True, text=True, check=False)


def tests_reaching(tree: str, base: str, head: str, sheets: list[str]) -> list[str] | None:
    """The browser tests that could notice the sheets' revert in tree, by the map recorded at head;
    None when there is no such map, or the change may reach any test: then every test runs."""
    commit = subprocess.run(["git", "rev-parse", head], cwd=tree, capture_output=True, text=True,
                            check=False).stdout.strip()
    recorded = load(BROWSER_MAP, commit)
    if recorded is None:
        return None
    changes: list[tuple[str, str]] = []
    for sheet in sheets:
        before, after = _show(tree, f"{base}:{sheet}"), _show(tree, f"{head}:{sheet}")
        if before.returncode != 0 or after.returncode != 0:
            return None
        changes.append((before.stdout, after.stdout))
    return reached(recorded, changes)
