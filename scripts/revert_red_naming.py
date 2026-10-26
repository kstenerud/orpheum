# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: For a probe of a Go file, the tests whose own code names what its change touched, read from the
# ABOUTME: head's sources; run before the package's whole run, so a red is found without waiting for it.
"""The Go tests that name a change (check_revert_red.attempts_for's first run for a Go file).

Revert-red ran a Go file's whole package and then its dependents' with no `-run`: internal/core/api's
310 tests took 32 to 81 s a probe, and Go's probes 1319 s of one check-pr's 3044 s of probing
(test-architecture-2026-10-26.md, finding 1). The test that goes red is nearly always one that calls
what the change touched; a red then also pays for a clean run of the same command
(revert_red_clean), which for the whole package was most of the probe (api's together.go: a 19 s red,
137 s in all, 2026-10-26).

**The tests that name it.** The changed names are the top-level declarations whose code differs
between the file's two versions, comments and layout aside; then, level by level, those of its own
package's code that name one (an unexported helper is reached through the function the tests call),
and the nearest level some test names is taken. A test names one when its function, or a declaration
of its package's test files it uses, directly or not, names it: in its own package by its bare name,
from another (the external test package, a dependent) as `pkg.Name` through an import of it, and a
method by `.Name` on anything. Its own package's such tests run first, its dependents'
(revert_red_dependents.go_dependents) after its own package's whole run, apart, so a red in its own
package waits for no dependent's build; never those that read the sources, which only their own runs
can show. A package where they are more than half its tests is left to its whole run.

**Why not a reach map or recorded coverage.** A transitive map of what each declaration names, types
reaching their methods, reached 295 of the api package's 311 tests for every change in it: every test
builds the server through one harness, and the server registers every handler. Go 1.26 records
coverage per process (`-coverprofile` per package, GOCOVERDIR per process), so a per-test map needs a
process per test: about 2000 runs, each paying its package's set-up, recorded apart from any run the
gate makes and stale by the time a batch is probed. Both are weighed in
docs/contributors/test-weight.md §30.

**It decides nothing.** The tests it names run before the probe's usual runs, and a red there settles
the probe; a green goes on to every run the probe made before, so no verdict rests on it, as
browser_map.tests_loading orders the browser tier's. A name it misses (a value handed through an
interface, a route reached by its path, state one test leaves another) costs a run, never a verdict.
"""

from __future__ import annotations

import re
import subprocess
from dataclasses import dataclass

from revert_red_attempts import Attempt
from revert_red_commands import go_test
from revert_red_dependents import go_module, go_source_readers

# The test functions `go test` runs: tests, examples, fuzz targets (whose seed corpus runs as tests).
TEST_FUNCTION = re.compile(r"^(Test|Example|Fuzz)\w*$")
# A name used alone, a name after a `.` (a field, a method, or a package's declaration), and the two
# names of `a.b`, the first of which may be an import's.
BARE = re.compile(r"(?<![.\w])([A-Za-z_]\w*)")
SELECTED = re.compile(r"\.\s*([A-Za-z_]\w*)")
QUALIFIED = re.compile(r"(?<![.\w])([A-Za-z_]\w*)\s*\.\s*([A-Za-z_]\w*)")
DECLARATION_START = re.compile(r"^(func|type|var|const|import|package)\b")
RECEIVER = re.compile(r"^func\s*\(\s*(?:\w+\s+)?\*?\s*(\w+)")
FUNCTION_NAME = re.compile(r"^func\s*(?:\([^)]*\)\s*)?(\w+)")
SPEC_NAMES = re.compile(r"^\t?(\w+(?:\s*,\s*\w+)*)")
IMPORT_SPEC = re.compile(r'^\s*(?:import\s+)?(?:\(\s*)?(?:([\w.]+)\s+)?"([^"]+)"')
MAJOR_VERSION = re.compile(r"^v\d+$")
# A narrowing to more than this share of a package's tests costs more than it saves: a green probe
# pays it and then the whole run (browser_map.tests_loading's reason).
MOST = 0.5


def strip(source: str) -> str:
    """The Go source with its comments and string, rune and raw-string literals blanked, newlines kept,
    so what is left is code: the names in it are the names the code uses."""
    out: list[str] = []
    i, n = 0, len(source)
    while i < n:
        c = source[i]
        if source.startswith("//", i):
            end = source.find("\n", i)
            i = n if end < 0 else end
        elif source.startswith("/*", i):
            end = source.find("*/", i + 2)
            end = n if end < 0 else end + 2
            out.append("\n" * source.count("\n", i, end) or " ")
            i = end
        elif c == "`":
            end = source.find("`", i + 1)
            end = n if end < 0 else end + 1
            out.append('""' + "\n" * source.count("\n", i, end))
            i = end
        elif c in "\"'":
            j = i + 1
            while j < n and source[j] != c and source[j] != "\n":
                j += 2 if source[j] == "\\" else 1
            out.append('""')
            i = j + 1
        else:
            out.append(c)
            i += 1
    return "".join(out)


@dataclass(frozen=True)
class Declaration:
    """One top-level declaration: its names (a group has several; a method's is `Type.Method`), its
    code with comments and literals blanked, and its kind: func, method, init, type, var or const."""

    kind: str
    names: tuple[str, ...]
    code: str


@dataclass(frozen=True)
class GoFile:
    """A Go file's package clause, its imports by the name the code uses each by (a dot import as
    `.path`), its blank and dot imports, and its top-level declarations."""

    package: str
    imports: dict[str, str]
    blank_imports: frozenset[str]
    declarations: tuple[Declaration, ...]


def import_name(path: str, clauses: dict[str, str]) -> str:
    """The name an import without an alias is used by: its package clause when known (the module's
    own), else the last element of its path, a version suffix dropped."""
    if path in clauses:
        return clauses[path]
    parts = path.split("/")
    last = parts[-2] if len(parts) > 1 and MAJOR_VERSION.match(parts[-1]) else parts[-1]
    return last.removeprefix("go-").replace("-", "_").replace(".", "_")


def parse(source: str, clauses: dict[str, str] | None = None) -> GoFile:
    """A gofmt'd Go file's package, imports and top-level declarations, each of which starts a line."""
    raw_lines, lines = source.splitlines(), strip(source).splitlines()
    package = ""
    imports: dict[str, str] = {}
    blank: set[str] = set()
    declarations: list[Declaration] = []
    starts = [index for index, line in enumerate(lines) if DECLARATION_START.match(line)]
    for at, start in enumerate(starts):
        end = starts[at + 1] if at + 1 < len(starts) else len(lines)
        text = "\n".join(lines[start:end])
        keyword = lines[start].split("(", 1)[0].split()[0]
        if keyword == "package":
            package = lines[start].split()[1] if len(lines[start].split()) > 1 else ""
        elif keyword == "import":
            for raw in raw_lines[start:end]:
                spec = IMPORT_SPEC.match(raw.split("//", 1)[0])
                if not spec:
                    continue
                alias, path = spec.group(1), spec.group(2)
                if alias in ("_", "."):
                    blank.add(f"{alias} {path}")
                if alias == ".":
                    imports[f".{path}"] = path
                elif alias != "_":
                    imports[alias or import_name(path, clauses or {})] = path
        elif keyword == "func":
            declarations.append(_function(lines[start], text))
        else:
            declarations.append(Declaration(keyword, _spec_names(lines[start:end], keyword), text))
    return GoFile(package, imports, frozenset(blank), tuple(declarations))


def _function(first: str, text: str) -> Declaration:
    name_match = FUNCTION_NAME.match(first)
    name = name_match.group(1) if name_match else ""
    receiver = RECEIVER.match(first) if re.match(r"^func\s*\(", first) else None
    if receiver:
        return Declaration("method", (f"{receiver.group(1)}.{name}",), text)
    return Declaration("init" if name == "init" else "func", (name,), text)


def _spec_names(lines: list[str], keyword: str) -> tuple[str, ...]:
    """The names a type, var or const declaration declares, one spec or a parenthesised group."""
    first = lines[0][len(keyword):].strip()
    if not first.startswith("("):
        found = SPEC_NAMES.match(first)
        return tuple(name.strip() for name in found.group(1).split(",")) if found else ()
    names: list[str] = []
    for line in lines[1:]:
        if line.startswith("\t") and not line.startswith("\t\t"):
            found = SPEC_NAMES.match(line)
            if found:
                names += [name.strip() for name in found.group(1).split(",")]
    return tuple(names)


def _normal(code: str) -> str:
    return " ".join(code.split())


def changed_names(before: str | None, after: str) -> set[str] | None:
    """The names whose declarations differ between a file's two versions, comments and layout aside
    (a method's as `Type.Method`); None when what changed is not a named declaration: its package
    clause, its blank or dot imports (which run or expose a package's code), an `init` or a `_` var."""
    old, new = parse(before or ""), parse(after)
    if before is not None and (old.blank_imports != new.blank_imports or old.package != new.package):
        return None

    def by_name(gofile: GoFile) -> tuple[dict[str, str], list[str]]:
        named: dict[str, str] = {}
        anonymous: list[str] = []
        for declaration in gofile.declarations:
            if declaration.kind == "init" or declaration.names == ("_",):
                anonymous.append(_normal(declaration.code))
                continue
            for name in declaration.names:
                named[name] = named.get(name, "") + _normal(declaration.code)
        return named, anonymous

    (old_named, old_anonymous), (new_named, new_anonymous) = by_name(old), by_name(new)
    if old_anonymous != new_anonymous:
        return None
    return {name for name in old_named.keys() | new_named.keys() if old_named.get(name) != new_named.get(name)}


@dataclass(frozen=True)
class Names:
    """What some code names: bare names, `(import path, name)` through an import, and selectors."""

    bare: frozenset[str]
    qualified: frozenset[tuple[str, str]]
    selected: frozenset[str]


def names_in(code: str, imports: dict[str, str]) -> Names:
    """The names code uses, given its file's imports by the name each is used by."""
    bare = set(BARE.findall(code))
    selected = set(SELECTED.findall(code))
    qualified = {(imports[first], second) for first, second in QUALIFIED.findall(code) if first in imports}
    for key, path in imports.items():
        if key.startswith("."):
            qualified.update((path, name) for name in bare)
    return Names(frozenset(bare), frozenset(qualified), frozenset(selected))


def callers_by_level(files: list[GoFile], changed: set[str]) -> list[set[str]]:
    """changed, then with the package's declarations whose code names one of those, and so on, each
    level holding the one before: a function the tests call is found for the helper it calls. A name
    is named by its bare name, a method by `.Name`. files are the package's code, its tests' excluded."""
    named = [(declaration, names_in(declaration.code, {})) for gofile in files for declaration in gofile.declarations
             if declaration.kind != "init"]
    levels = [set(changed)]
    while True:
        reached = levels[-1]
        bare = {name for name in reached if "." not in name}
        methods = {name.split(".", 1)[1] for name in reached if "." in name}
        grown = set(reached)
        for declaration, uses in named:
            if not set(declaration.names) <= reached and (uses.bare & bare or uses.selected & methods):
                grown.update(declaration.names)
        if grown == reached:
            return levels
        levels.append(grown)


@dataclass(frozen=True)
class TestCode:
    """A test, whether it is an internal test of the package whose change is probed (and so names its
    declarations bare), and what its code and the test files' declarations it uses name."""

    name: str
    internal: bool
    uses: tuple[Names, ...]


def test_code(test_files: list[GoFile], own: bool) -> list[TestCode]:
    """The tests of one package's test files, own when it is the probed file's package, each with
    what its function and the test files' declarations it uses, directly or not, name."""
    found: list[TestCode] = []
    for clause in sorted({gofile.package for gofile in test_files}):
        in_clause = [gofile for gofile in test_files if gofile.package == clause]
        declared: dict[str, Names] = {}
        for gofile in in_clause:
            for declaration in gofile.declarations:
                for name in declaration.names:
                    declared.setdefault(name, names_in(declaration.code, gofile.imports))
        for gofile in in_clause:
            for declaration in gofile.declarations:
                name = declaration.names[0] if declaration.names else ""
                if declaration.kind != "func" or not TEST_FUNCTION.match(name):
                    continue
                seen, todo, uses = {name}, [name], []
                while todo:
                    names = declared[todo.pop()]
                    uses.append(names)
                    for helper in names.bare - seen:
                        if helper in declared:
                            seen.add(helper)
                            todo.append(helper)
                found.append(TestCode(name, own and not clause.endswith("_test"), tuple(uses)))
    return found


def tests_naming(tests: list[TestCode], package_path: str, changed: set[str]) -> list[str]:
    """The tests whose code names one of changed, declarations of the package at package_path: by its
    bare name in an internal test, `pkg.Name` through an import elsewhere, a method by `.Name`."""
    bare = {name for name in changed if "." not in name}
    exported = {(package_path, name) for name in bare if name[:1].isupper()}
    methods = {name.split(".", 1)[1] for name in changed if "." in name}
    return [test.name for test in tests
            if any(names.selected & methods or (test.internal and names.bare & bare) or names.qualified & exported
                   for names in test.uses)]


@dataclass(frozen=True)
class Naming:
    """The tests that name a change: its own package's, and its dependents' by package "./x/y"."""

    own: list[str]
    dependents: dict[str, list[str]]


def _read(root: str, rev: str, paths: list[str]) -> dict[str, str]:
    """The text of each path at rev, read in one `git cat-file --batch`; a path rev has not is left out."""
    if not paths:
        return {}
    request = "".join(f"{rev}:{path}\n" for path in paths).encode()
    out = subprocess.run(["git", "cat-file", "--batch"], cwd=root, input=request, capture_output=True,
                         check=False).stdout
    files: dict[str, str] = {}
    at = 0
    for path in paths:
        header_end = out.index(b"\n", at)
        header = out[at:header_end].split()
        if len(header) < 3 or header[1] != b"blob":
            at = header_end + 1
            continue
        size = int(header[2])
        files[path] = out[header_end + 1: header_end + 1 + size].decode("utf-8", errors="replace")
        at = header_end + 1 + size + 1
    return files


def _go_files(root: str, rev: str, directory: str) -> list[str]:
    """The Go files directly in directory ("x/y") at rev."""
    listed = subprocess.run(["git", "ls-tree", "--name-only", rev, "--", f"{directory}/"], cwd=root,
                            capture_output=True, text=True, check=False).stdout.split("\n")
    return [path for path in listed if path.endswith(".go")]


def go_tests_naming(root: str, base: str, head: str, path: str, dependents: list[str]) -> Naming | None:
    """The tests of a Go file's own package, and of its dependents, whose code names what its change
    between base and head touched, at head; each the nearest: those naming the changed declarations,
    else those naming their callers in the package, and so on. A package where the nearest are more
    than half its tests has none: its whole run is as quick. None when no test is left, or the change
    is not to named declarations. Tests that read the sources are left to their own runs."""
    directory = path.rsplit("/", 1)[0] if "/" in path else "."
    module = go_module(root)
    after = _read(root, head, [path]).get(path)
    if after is None or not module:
        return None
    changed = changed_names(_read(root, base, [path]).get(path), after)
    if not changed:
        return None
    own_files = _read(root, head, _go_files(root, head, directory))
    sources = [parse(text) for name, text in own_files.items() if not name.endswith("_test.go")]
    levels = callers_by_level(sources, changed)
    package_path = f"{module}/{directory}" if directory != "." else module
    clauses = {package_path: next((gofile.package for gofile in sources if gofile.package), "")}
    readers = set(go_source_readers(root))

    def tests_of(package_dir: str) -> list[TestCode]:
        test_paths = [name for name in _go_files(root, head, package_dir) if name.endswith("_test.go")]
        return test_code([parse(text, clauses) for text in _read(root, head, test_paths).values()],
                         package_dir == directory)

    own = nearest({"./" + directory: tests_of(directory)}, package_path, levels).get("./" + directory, [])
    others = {package: tests_of(package.removeprefix("./")) for package in dependents
              if package not in readers and package != "./" + directory}
    naming = Naming(own, nearest(others, package_path, levels))
    return naming if naming.own or naming.dependents else None


def nearest(packages: dict[str, list[TestCode]], package_path: str, levels: list[set[str]]) -> dict[str, list[str]]:
    """The tests, by package, naming the first level of changed names any of them names; a package
    whose such tests are more than half of its tests is left out."""
    for changed in levels:
        named = {package: tests_naming(tests, package_path, changed) for package, tests in packages.items()}
        if any(named.values()):
            return {package: found for package, found in named.items()
                    if found and len(found) <= MOST * len(packages[package])}
    return {}


def naming_runs(root: str, base: str, head: str, path: str, dependents: list[str]) -> tuple[list[Attempt], list[Attempt]]:
    """A Go file's probe's runs of the tests that name its change: its own package's, to go before its
    whole run, and its dependents', to go after it and before every dependent's."""
    naming = go_tests_naming(root, base, head, path, dependents)
    if naming is None:
        return [], []
    own = "./" + (path.rsplit("/", 1)[0] if "/" in path else ".")
    first = [Attempt(go_test(own, tests=naming.own), ", found by the tests that name what it changed")] if naming.own else []
    tests = [test for found in naming.dependents.values() for test in found]
    after = [Attempt(go_test(*naming.dependents, tests=tests), ", found by its dependents' tests that name what it "
                     "changed")] if tests else []
    return first, after
