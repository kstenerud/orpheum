#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Fails make check when an e2e spec waits for words the web client no longer has: each name,
# ABOUTME: text or label a Playwright locator in web/e2e/ looks for must be in the English catalogue.
"""Every UI string an e2e spec waits for is still in the web client's words.

    check_e2e_strings.py [ROOT]

An e2e spec finds what it presses by its words: `getByRole("button", { name: "Use" })`. When a
change takes those words from the client, the spec waits for them until its timeout, and only
check-pr's browser tier, hours later, shows it: D468 took the receiver's question away, and
`cast.spec.ts` failed three tests there (the 11-24 batch's check-pr; the owner's go, 2026-11-13).
This finds it in make check instead, in under a second.

Read from the specs and their page helpers (`web/e2e/*.ts` but the tests of the e2e machinery and
the Playwright configurations): each quoted string, and each regular expression, a locator looks
for: `getByRole`'s `name`, and `getByText`, `getByLabel`, `getByPlaceholder`, `getByTitle`,
`toHaveText` and `toContainText`. Read from the words (`web/apps/web/src/locales/*.ts` and the
screens' own `*-words.ts`): every quoted string, a `{{placeholder}}` in one matching any words. As
Playwright matches a name, a string passes when some catalogue string contains it, case folded;
a regular expression, when it matches some catalogue string. A string held in a variable is not
read: it names test data (a film's title), not the client's words. Nor is a locator a statement
expects to find nothing (`toHaveCount(0)`, `.not.`, `toBeHidden`): words gone on purpose, as
D468's question, are what it proves.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

E2E = Path("web") / "e2e"
WORDS = [Path("web") / "apps" / "web" / "src" / "locales", Path("web") / "apps" / "web" / "src"]

# A double-quoted or backtick string without ${...}, or a regular expression literal. Prettier
# writes TypeScript's strings in double quotes, so a single quote is an apostrophe in words.
QUOTED = r'"((?:[^"\\\n]|\\.)*)"|`((?:[^`\\$]|\\.)*)`'
REGEX = r"/((?:[^/\\\n]|\\.)+)/([a-z]*)"
# Where a locator takes the words it looks for, the words being the next argument.
LOOKS_FOR = re.compile(
    r"(?:\bname:\s*|\.getBy(?:Text|Label|Placeholder|Title)\(\s*|\.(?:toHaveText|toContainText)\(\s*)"
    rf"(?:{QUOTED}|{REGEX})"
)
# A statement expecting its locator to find nothing: the words' absence is what it asserts.
EXPECTS_NONE = re.compile(r"toHaveCount\(\s*0\s*[,)]|\.not\.|\.toBeHidden\(")
COMMENT = re.compile(r"//[^\n]*|/\*.*?\*/", re.S)
PLACEHOLDER = re.compile(r"\{\{\w+\}\}")


def without_comments(source: str) -> str:
    """source with its comments blanked to their newlines, so line numbers hold. A `//` inside a
    string is rare in these files and would only shorten what is read of that line."""
    return COMMENT.sub(lambda match: "\n" * match.group(0).count("\n"), source)


def unescape(text: str) -> str:
    return re.sub(r"\\(.)", r"\1", text)


def catalogue(root: Path) -> list[str]:
    """Every string of the web client's words."""
    files = sorted((root / WORDS[0]).glob("*.ts")) + sorted((root / WORDS[1]).glob("*-words.ts"))
    strings: list[str] = []
    for path in files:
        if path.name.endswith(".test.ts"):
            continue
        for match in re.finditer(QUOTED, without_comments(path.read_text(encoding="utf-8"))):
            strings.append(unescape(match.group(1) if match.group(1) is not None else match.group(2)))
    return strings


def spec_files(root: Path) -> list[Path]:
    """The specs and their page helpers: not the tests of the e2e machinery, nor its configurations."""
    return [path for path in sorted((root / E2E).glob("*.ts"))
            if not path.name.endswith((".test.ts", ".config.ts"))]


def looked_for(path: Path) -> list[tuple[int, str, str | None]]:
    """What path's locators look for: (line, the string or the pattern, the pattern's flags or None
    for a string)."""
    source = without_comments(path.read_text(encoding="utf-8"))
    found = []
    for match in LOOKS_FOR.finditer(source):
        statement_end = source.find(";", match.end())
        if EXPECTS_NONE.search(source, match.end(), statement_end if statement_end >= 0 else len(source)):
            continue
        line = source.count("\n", 0, match.start()) + 1
        if match.group(3) is not None:
            found.append((line, match.group(3), match.group(4)))
        else:
            found.append((line, unescape(match.group(1) if match.group(1) is not None else match.group(2)), None))
    return found


def is_in(words: str, flags: str | None, strings: list[str]) -> bool:
    """Whether a locator looking for words would find some catalogue string."""
    if flags is None:
        folded = words.casefold()
        if any(folded in string.casefold() for string in strings):
            return True
        # Words a {{placeholder}} took the place of: "Playing on the TV" is "Playing on {{name}}".
        return any(re.search(".+".join(re.escape(part) for part in PLACEHOLDER.split(string)), words, re.I | re.S)
                   for string in strings if PLACEHOLDER.search(string))
    try:
        pattern = re.compile(words, (re.I if "i" in flags else 0) | (re.S if "s" in flags else 0))
    except re.error:
        return True  # JavaScript's syntax Python does not read: not this check's to judge
    return any(pattern.search(PLACEHOLDER.sub("Name", string)) for string in strings)


def problems(root: Path) -> list[str]:
    strings = catalogue(root)
    found = []
    for path in spec_files(root):
        for line, words, flags in looked_for(path):
            if not is_in(words, flags, strings):
                shown = f"/{words}/{flags}" if flags is not None else repr(words)
                found.append(f"{path.relative_to(root)}:{line}: waits for {shown}, which the web client's words "
                             "no longer have")
    return found


def main(argv: list[str]) -> int:
    root = Path(argv[0]) if argv else Path(".")
    found = problems(root)
    for problem in found:
        print(problem)
    if found:
        print("check_e2e_strings: an e2e spec would wait for these until its timeout. Change the spec to the "
              "words the client now has, or the words back if they were taken by mistake.")
        return 1
    print("check_e2e_strings: every string the e2e specs wait for is in the web client's words")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
