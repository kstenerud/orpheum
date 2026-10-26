# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: For a probe of a locale catalogue file, the catalogue's own test and the tests that render
# ABOUTME: its changed keys, read from the source; run before the probe's usual runs, never instead.
"""The web tests that name a locale change (check_revert_red.attempts_for's first runs for one).

103 of 167 app test files import `i18n.ts`, so `vitest related` on a locale file ran nearly the whole
suite: `en-settings.ts` probes took 85 s and 52 s (test-architecture-2026-10-26.md, finding 4). The
catalogue's own test (`catalogue.test.ts`) is cheap and catches a missing or mistyped key outright; a
changed key's wording is caught by whichever screen renders it, found the same way
`locales/catalogue.test.ts` itself finds a key a source file asks for: a literal `t("a.b")`.

**The tests it finds.** A changed leaf's dotted path (`settings.sections.general`) is matched against
every non-test source file under `web/apps/web/src` that asks for it by that exact literal string; each
such file's own rendering tests are `vitest related`'s, over the small set of matched source files
rather than the locale file itself, which is the width that matters: a component a handful of tests
import, not the whole app's `i18n.ts`.

**Reading the catalogue itself.** `en.ts` and `en-settings.ts`'s `details` and `secretDescriptions`
are object literals spread together (`...enTogether`, `...enSettingsCatalogue`): a changed leaf's path
is the same whichever file holds the spread, so a spread is followed to the file it names (by the
spreading file's own `import { x } from "./y.ts"`) and read at the same revision, recursively.

**What it does not vouch for.** A key asked for only through a built string
(`` t(`settings.sections.${camelKey(section)}`) ``) is not matched: the literal text before the `${` is
not the key, and trusting it would be guessing which of several keys sharing that prefix the change
actually touched. A key no source file asks for by a literal at all is the same: nothing is added for
it. Neither stops the probe — the attempts this module returns go *before* the probe's usual ones
(`check_revert_red.attempts_for`'s `scope_for`, today's `vitest related` over the whole locale file),
never instead of them, so a change only reached through a built key is still found, only not sooner.
A file this cannot parse (not the plain shape above) is the same: nothing is added, and the probe's
usual runs are exactly as they were before this module existed.

**It decides nothing.** As `revert_red_naming`'s Go tests that name a change: a red here settles the
probe; a green goes on to every run the probe made before, so no verdict rests on the scan.
"""

from __future__ import annotations

import functools
import os
import re
import subprocess
from pathlib import Path

from revert_red_attempts import Attempt
from revert_red_commands import vitest

# A locale data file: the catalogue and its parts, never the catalogue's own test.
LOCALE_SOURCE = re.compile(r"^web/apps/web/src/locales/en[\w-]*\.ts$")

CATALOGUE_TEST = "apps/web/src/locales/catalogue.test.ts"

# The literal a source file asks the translation layer for by name, as catalogue.test.ts's own
# keysAskedFor reads it: `t("a.b")`. A built key (a template literal) is not this: see the module
# docstring on what it does not vouch for.
STATIC_KEY = re.compile(r'\bt\(\s*"([^"]+)"')

TEST_FILE = re.compile(r"\.test\.tsx?$")
WEB_APP_SRC = "web/apps/web/src"

IMPORT = re.compile(r'import\s*\{\s*([^}]+?)\s*\}\s*from\s*"(\.[^"]+)"')
IDENT = re.compile(r"[A-Za-z_$][A-Za-z0-9_$]*")


class Unparseable(Exception):
    """The file is not the plain `export const x = {...}` of string, object and spread this reads."""


def _skip_ws(text: str, i: int) -> int:
    n = len(text)
    while i < n:
        if text[i].isspace():
            i += 1
        elif text.startswith("//", i):
            end = text.find("\n", i)
            i = n if end < 0 else end
        elif text.startswith("/*", i):
            end = text.find("*/", i + 2)
            i = n if end < 0 else end + 2
        else:
            break
    return i


def _parse_string(text: str, i: int) -> tuple[str, int]:
    quote = text[i]
    j, n = i + 1, len(text)
    while j < n and text[j] != quote:
        j += 2 if text[j] == "\\" else 1
    if j >= n:
        raise Unparseable("unterminated string")
    return text[i : j + 1], j + 1


def _parse_key(text: str, i: int) -> tuple[str, int]:
    if text[i] in "\"'":
        raw, j = _parse_string(text, i)
        return raw[1:-1], j
    found = IDENT.match(text, i)
    if not found:
        raise Unparseable(f"expected a key at {i}")
    return found.group(0), found.end()


EXPORT = re.compile(r"export\s+const\s+[A-Za-z_$][A-Za-z0-9_$]*\s*=\s*\{")


class _Parser:
    """One file's object literal, read at one revision, spreads followed to the files that import
    them name (`...enTogether`, `import { enTogether } from "./en-together.ts"`)."""

    def __init__(self, root: str, rev: str, path: str, text: str):
        self.root = root
        self.rev = rev
        self.text = text
        directory = str(Path(path).parent)
        self.imports: dict[str, str] = {}
        for names, specifier in IMPORT.findall(text):
            resolved = os.path.normpath(os.path.join(directory, specifier))
            for name in names.split(","):
                name = name.strip()
                if name:
                    self.imports[name] = resolved

    def parse_export(self) -> dict[str, object]:
        found = EXPORT.search(self.text)
        if not found:
            raise Unparseable("no 'export const x = {' found")
        value, _ = self.parse_value(found.end() - 1)
        if not isinstance(value, dict):
            raise Unparseable("the export is not an object")
        return value

    def parse_value(self, i: int) -> tuple[object, int]:
        """A value: a nested object (a dict), a string (its raw text, quotes included, for
        comparison only — not unescaped), or Unparseable for anything else (a number, an array, a
        template literal): this file is then left to the probe's usual runs, as a parse failure
        never narrows anything."""
        i = _skip_ws(self.text, i)
        if i >= len(self.text):
            raise Unparseable("end of file in a value")
        if self.text[i] == "{":
            return self.parse_object(i)
        if self.text[i] in "\"'":
            return _parse_string(self.text, i)
        raise Unparseable(f"not a string or object at {i}: {self.text[i:i + 20]!r}")

    def parse_object(self, i: int) -> tuple[dict[str, object], int]:
        assert self.text[i] == "{"
        i += 1
        out: dict[str, object] = {}
        while True:
            i = _skip_ws(self.text, i)
            if i >= len(self.text):
                raise Unparseable("end of file inside an object")
            if self.text[i] == "}":
                return out, i + 1
            if self.text.startswith("...", i):
                found = IDENT.match(self.text, i + 3)
                if not found:
                    raise Unparseable(f"expected a name to spread at {i + 3}")
                out.update(self._spread(found.group(0)))
                i = found.end()
            else:
                key, i = _parse_key(self.text, i)
                i = _skip_ws(self.text, i)
                if i >= len(self.text) or self.text[i] != ":":
                    raise Unparseable(f"expected ':' at {i}")
                value, i = self.parse_value(i + 1)
                out[key] = value
            i = _skip_ws(self.text, i)
            if i < len(self.text) and self.text[i] == ",":
                i = _skip_ws(self.text, i + 1)

    def _spread(self, name: str) -> dict[str, object]:
        path = self.imports.get(name)
        if path is None:
            raise Unparseable(f"no import of a spread name {name!r}")
        return _parsed_tree(self.root, self.rev, path)


def _read_at(root: str, rev: str, path: str) -> str | None:
    """path's text at rev, or None when rev has no such path (an added file's base)."""
    done = subprocess.run(["git", "show", f"{rev}:{path}"], cwd=root, capture_output=True, text=True,
                          check=False)
    return done.stdout if done.returncode == 0 else None


@functools.lru_cache(maxsize=None)
def _parsed_tree(root: str, rev: str, path: str) -> dict[str, object]:
    """path's exported object at rev, spreads followed, as a nested dict of dotted keys to leaf
    string text; raises Unparseable when it, or a file it spreads, is not that shape."""
    text = _read_at(root, rev, path)
    if text is None:
        raise Unparseable(f"{path} has no text at {rev}")
    return _Parser(root, rev, path, text).parse_export()


def _flatten(node: object, prefix: str = "") -> dict[str, str]:
    if isinstance(node, str):
        return {prefix: node}
    out: dict[str, str] = {}
    for key, value in node.items():  # type: ignore[union-attr]
        out.update(_flatten(value, f"{prefix}.{key}" if prefix else key))
    return out


def changed_locale_keys(root: str, base: str, head: str, path: str) -> set[str] | None:
    """The dotted paths whose text differs between base and head, or is new or removed; None when
    either version is not the plain object literal this parses (the file is left to the usual runs)."""
    try:
        new_keys = _flatten(_parsed_tree(root, head, path))
        old_keys = {} if _read_at(root, base, path) is None else _flatten(_parsed_tree(root, base, path))
    except Unparseable:
        return None
    return {key for key in old_keys.keys() | new_keys.keys() if old_keys.get(key) != new_keys.get(key)}


@functools.cache
def _key_usage(root: str) -> dict[str, frozenset[str]]:
    """Every non-test source file under web/apps/web/src (its path relative to web/) to the keys it
    asks for by a literal `t("a.b")`, for files that ask for at least one."""
    usage: dict[str, frozenset[str]] = {}
    src = Path(root) / WEB_APP_SRC
    for pattern in ("**/*.ts", "**/*.tsx"):
        for file in sorted(src.glob(pattern)):
            rel = file.relative_to(Path(root) / "web")
            if TEST_FILE.search(file.name) or "locales" in rel.parts:
                continue
            try:
                text = file.read_text(encoding="utf-8")
            except OSError:
                continue
            keys = frozenset(STATIC_KEY.findall(text))
            if keys:
                usage[str(rel)] = keys
    return usage


def matching_source_files(root: str, changed: set[str]) -> list[str]:
    """The source files (relative to web/) that ask for one of changed by its literal key, sorted."""
    return sorted(path for path, keys in _key_usage(root).items() if keys & changed)


def locale_runs(root: str, base: str, head: str, path: str) -> list[Attempt]:
    """A locale file's probe's early runs: the catalogue's own test alone, always, since it is cheap
    and catches a key gone missing or mistyped outright; then, if the scan found source files asking
    for one of the changed keys by a literal, the tests that render them (`vitest related` over just
    those files, never the locale file's own importers, which is most of the app)."""
    attempts = [Attempt(vitest("run", CATALOGUE_TEST), ", found by the English catalogue's own test")]
    changed = changed_locale_keys(root, base, head, path)
    if changed:
        matches = matching_source_files(root, changed)
        if matches:
            attempts.append(Attempt(vitest("related", *matches), ", found by the tests that render its "
                                    "changed keys"))
    return attempts
