#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Media-names gate run by `make check`, the spot check and the commit lint: every film,
# ABOUTME: series and media file the repository names is one scripts/media_names.allow lists.
"""Hold every film, series and media file the repository names to a registry.

What the owner watches is personal, and the repository is public (the owner, 2026-11-11: "Don't
record video names in the repo"; agent-failures A70). A test of metadata needs filenames, and some
need a real film's name: those are old ones, out of copyright where possible, or invented, and
never a line copied from a file listing. `scripts/media_names.allow` lists each name once with why
it is needed; adding one is a gate change.

What is held to it, in the tree and in commit messages:

* **A media file's name** (`.mkv`, `.srt`, `.nfo`, ...): its title, once the year, the episode
  marker, the quality tags, a language code and the extension are taken off, is a registered name.
  A name with no letters left (`.mkv`, `00001.m4s`) names nothing.
* **"Title (Year)"**: the title before a year in brackets is a registered name.
* **An episode marker** (S02E05, s2e5, 2x05): inside a media file's name, or in a file the registry
  lists under [episode-files] (the episode parser and its tests).
* **The phrases a title is written in**: "the owner's <Capitalised words>", and a capitalised
  phrase after film, episode, series, watching, playing, cast and their kin, unless it is capitals
  and figures alone (an acronym, an item's id). The phrase, or its
  first words, is a registered name or one of the registry's [words], which are no titles: a
  device, a format, a product.
* **A title of the owner's library, where this machine has it** (`/tank/media/video`, or the folders
  ORPHEUM_MEDIA_LIBRARY names): a folder's or file's title of two words or more, written as a title is
  (its words capitalised but for the small ones), across a line break too (the owner, 2026-11-15).
  The titles are read where the gate runs and written nowhere; on a machine without the library, CI's,
  none is compared. A one-word title, and a folder name media servers give (Season 1, Behind the
  Scenes), is not compared: the words are ordinary ones. A bare title elsewhere has no shape to find;
  AGENTS.md rule 12 and the commit checklist guard it.
"""

from __future__ import annotations

import argparse
import bisect
import fnmatch
import functools
import os
import re
import subprocess
import sys
import unicodedata
from collections import Counter
from dataclasses import dataclass
from pathlib import Path

REGISTRY = Path("scripts/media_names.allow")

# Files never read: the registry, this gate and its tests (which hold the unlisted names the rules
# are tried on, all invented), and what no person writes.
SKIPPED = frozenset({
    str(REGISTRY),
    "scripts/check_media_names.py",
    "scripts/tests/test_check_media_names.py",
    "web/package-lock.json",
    "go.sum",
})
SKIPPED_SUFFIXES = (".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".woff", ".woff2", ".ttf")

MEDIA_EXTENSIONS = (
    "mkv|mp4|m4v|avi|mov|webm|m2ts|mts|wmv|mpg|mpeg|vob|iso|flv|"
    "srt|ass|ssa|sub|idx|sup|vtt|nfo|flac|mp3|m4a|aac|ogg|opus"
)
# A media file's name: its extension, in one case and with no call after it (`now.Sub(at)` is Go,
# not a subtitle), and the run of name characters before it, a path's last part. The run is found
# back from the extension, so the pattern is tried only where an extension is.
MEDIA_EXTENSION_RE = re.compile(
    r"\.(?:" + MEDIA_EXTENSIONS + "|" + MEDIA_EXTENSIONS.upper() + r")(?![\w(])"
)
# A combining mark (U+0300–U+036F) is part of a word: "Fie\u0300vre" is one name, not "vre".
NAME_RUN_RE = re.compile(r"[\w\u0300-\u036f' .,()\[\]&!+-]*\Z")
# A name as code writes it may hold an escape ("Fie\u0300vre.mkv"): the run takes it in, and the name
# is read from the run with its escapes decoded ("\u003e.vtt" names nothing: ">" is no name's).
ESCAPED_NAME_RUN_RE = re.compile(r"(?:[\w\u0300-\u036f' .,()\[\]&!+-]|\\u[0-9a-fA-F]{4})*\Z")
# An episode's marker: S02E05, s2e5, or 2x05 (one figure before the x: "64x64" is a picture's size).
EPISODE_RE = re.compile(r"(?<![\w])(?:[sS]\d{1,2}[eE]\d{1,3}|[1-9]x\d{2})(?![\w])")
# A title before its year: the year is found first, and the run of capitalised words before it after.
YEAR_RE = re.compile(r" \((?:19|20)\d\d\)")
# Lowercase words may stand inside it before a capitalised one ("Der letzte Mann"): every tail of the
# run is tried against the registry, so a longer run only adds names it may be.
TITLE_WORD = r"[A-Z0-9][\w'’&!:,.-]*"
SMALL_WORD = r"(?:of|the|a|an|and|in|on|to|for|at|de|la|le|du)"
YEAR_TITLE_RE = re.compile(
    r"(?<![\w'’&!:,.-])(" + TITLE_WORD + r"(?:(?: [^\W\d_A-Z][\w'’-]*)+ [A-Z][\w'’&!:,.-]*"
    r"| (?:" + TITLE_WORD + "|" + SMALL_WORD + r"))*)\Z"
)
# What a finding quotes: the capitalised words before the year, as the title is usually written.
YEAR_TITLE_QUOTED_RE = re.compile(
    r"(?<![\w'’&!:,.-])(?:" + TITLE_WORD + r"(?: (?:" + TITLE_WORD + "|" + SMALL_WORD + r"))*)\Z"
)
CAPITALISED = r"([A-Z][\w'’.-]*(?: [A-Z0-9][\w'’.-]*)*)"
PHRASE_RES = (
    re.compile(r"\bthe owner's " + CAPITALISED),
    re.compile(
        r"\b(?:films?|episodes?|series|watching|watched|watches|cast|casting|casts|"
        r"playing|played|plays)(?: of)? [\"“']?" + CAPITALISED
    ),
)
# What follows a title in a file's name and is no part of it.
QUALITY_RE = re.compile(
    r"[ ._\[(-](?:(?:19|20)\d\d|[sS]\d{1,2}[eE]\d{1,3}|[sS]\d{1,2}|[1-9]\d?x\d{1,3}|e\d{2,3}|"
    r"(?:- ?)?\d{1,4}(?:v\d)?(?= -|$)|season|saison|staffel|temporada|se\d|ep\d|episode|"
    r"cd\d|disc\s?\d|part\s?\d|\d{3,4}[pi]|4k|uhd|hdr\w*|"
    r"blu-?ray|bdrip|brrip|web-?(?:dl|rip)?|hdtv|dvd\w*|remux|x26[45]|h\.?26[45]|hevc|avc|"
    r"xvid|aac\w*|ac3|dts\w*|ddp?\d?\.?\d?|truehd|atmos|10bit|8bit|proper|repack|extended|"
    r"director'?s|theatrical|remastered|unrated|complete)(?![a-z]).*$",
    re.IGNORECASE,
)
# An extra's kind after its film's name (Jellyfin's and Plex's "Name-trailer.mkv").
EXTRA_RE = re.compile(
    r"(?:\s+(?:trailer|teaser)|\s*-\s*(?:trailers?|teaser|deleted(?: scenes?)?|featurette|interview|scene|short|other|clip|"
    r"theme|behind ?the ?scenes|sample|final))\s*$",
    re.IGNORECASE,
)
LANGUAGE_RE = re.compile(
    r"\.(?:[a-z]{2,3}(?:-[A-Za-z]{2,4})?|forced|sdh|default|cc|hi|commentary)$", re.IGNORECASE
)


def normalise(name: str) -> str:
    """A name as the registry is read: case, separators, edge punctuation and how an accent is
    encoded (a letter and its combining mark, or the one character) aside."""
    name = unicodedata.normalize("NFC", name)
    name = re.sub(r"[._]+", " ", name)
    name = re.sub(r"\s+", " ", name)
    return name.strip(" -–—:,'\"“”[]()").casefold()


@dataclass(frozen=True)
class Registry:
    names: frozenset[str]
    words: frozenset[str]
    episode_files: tuple[str, ...]

    def knows(self, phrase: str) -> bool:
        """Whether the phrase, or its first words, is a registered name or word."""
        words = normalise(re.sub(r"['’]s\b", "", phrase)).split(" ")
        return any(
            " ".join(words[:n]) in self.names or " ".join(words[:n]) in self.words
            for n in range(1, len(words) + 1)
        )

    def ends_with_known(self, phrase: str) -> bool:
        """Whether the phrase, or its last words, is a registered name or word: a title before its
        year is the end of a run of capitalised words, which may start the sentence."""
        return any(self._has_tail(name) for name in (phrase, QUALITY_RE.sub("", " " + phrase)))

    def _has_tail(self, phrase: str) -> bool:
        """Whether the phrase's last words are a registered name or word; a title's tags ("Title
        Unrated") are tried without them too."""
        words = re.split(r" |:(?! )", normalise(phrase))  # "10:1:Title": a key before it
        return any(
            " ".join(words[n:]) in self.names or " ".join(words[n:]) in self.words
            for n in range(len(words))
        )

    def knows_file(self, name: str) -> bool:
        """Whether a media file's name names nothing or a registered name. The run before the
        extension may take in the prose or code before it ("plays heat.mkv", "WHERE (kept.ass"), so
        its last words are a name too, unless the word before them is a capitalised word, which
        may start the title ("Blue Film.mkv" is not "film.mkv")."""
        words = re.split(r"[/\\]", name)[-1].split(" ")
        for n in range(len(words)):
            if n and re.match(r"[^\w]*[A-Z][a-z]", words[n - 1]):
                continue
            title = media_title(" ".join(words[n:]))
            if not re.search(r"[^\W\d_]", title) or title in self.names:
                return True
        return False

    def allows_episodes_in(self, path: str) -> bool:
        return any(fnmatch.fnmatch(path, pattern) for pattern in self.episode_files)


def read_registry(text: str) -> Registry:
    """The registry's sections: [names], [words] and [episode-files], an entry a line, its reason
    after a `|`. An entry with no reason is refused: the reason is the point."""
    sections: dict[str, set[str]] = {"names": set(), "words": set(), "episode-files": set()}
    section = ""
    for number, raw in enumerate(text.splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("[") and line.endswith("]"):
            section = line[1:-1]
            if section not in sections:
                raise ValueError(f"{REGISTRY}:{number}: no section [{section}]")
            continue
        entry, bar, why = line.partition("|")
        if not section or not bar or not why.strip() or not entry.strip():
            raise ValueError(f"{REGISTRY}:{number}: an entry is `<name> | <why it is needed>`")
        sections[section].add(entry.strip() if section == "episode-files" else normalise(entry))
    return Registry(
        frozenset(sections["names"]),
        frozenset(sections["words"]),
        tuple(sorted(sections["episode-files"])),
    )


def media_title(stem: str) -> str:
    """The title a media file's name carries: before its year, episode marker or quality tags."""
    stem = re.split(r"[/\\]", stem)[-1]
    while LANGUAGE_RE.search(stem):
        stem = LANGUAGE_RE.sub("", stem)
    stem = re.sub(r"^\s*(?:\[[^\]]*\]\s*)+", "", stem)  # a release group's tag before it
    stem = QUALITY_RE.sub("", " " + stem)
    stem = re.sub(r"[\[(][^\])]*(?:[\])]|$)", " ", stem)  # a bracketed tag: a site, a version
    stem = re.sub(r"[ ._]+-?[ ._]*\d{1,4}(?:[ ._]?v\d)?[\s._]*$", "", stem)  # an anime's absolute episode
    stem = EXTRA_RE.sub("", stem)
    return normalise(stem)


def names_a_file(name: str, extension: str) -> bool:
    """Whether a name before an extension is a file's: not an extension alone ("or .vtt", "(.ogg"),
    and, before an extension in capitals, not code's field (`text.VTT`) but a name in capitals too."""
    if not name or name[-1] in " \t([{,;:":
        return False
    return not extension[1:].isupper() or not re.search(r"[a-z]", name)


def is_acronyms(phrase: str) -> bool:
    """Whether a phrase is capitals and figures alone ("TCL TV", "VQ105", "H.264"): a device, a
    format or an item, not the way a title is written."""
    return not re.search(r"[a-z]", phrase)


# The owner's video library, where the machine the gate runs on mounts it (the owner's and the
# sandboxes'): ORPHEUM_MEDIA_LIBRARY names other folders instead, split by os.pathsep, or none when
# empty, as the gate's own tests have it (py_tests.py), which never read the real library (rule 16).
DEFAULT_LIBRARY = "/tank/media/video"
LIBRARY_ENV = "ORPHEUM_MEDIA_LIBRARY"
# A word as prose writes it, an apostrophe inside it ("Three's"): a title's words are compared.
WORD_RE = re.compile(r"[^\W_]+(?:['’][^\W_]+)*")
# A title's words a title leaves in lower case ("Back to the Lantern").
TITLE_SMALL_WORDS = frozenset(
    "a an and as at but by de des du for from in into la le les nor of on or the to vs with".split()
)
# The words of the folders media servers and discs name (Season 1, Behind the Scenes, VIDEO_TS): a
# library name of these and figures alone is no title.
FOLDER_WORDS = frozenset(
    "a and audio backup bdmv behind bonus cd certificate christmas clip clips deleted disc disk dvd extra extras "
    "featurette featurettes feature features interview interviews making meta of other part playlist "
    "sample samples scene scenes season series short shorts special specials stream sub subs subtitles "
    "the theme trailer trailers ts video vol volume".split()
)
SUBTITLE_SUFFIXES = re.compile(r"\.(?:srt|ass|ssa|sub|idx|sup|vtt)$", re.IGNORECASE)
LIBRARY_SUFFIXES = re.compile(r"\.(?:" + MEDIA_EXTENSIONS + r"|jpe?g|png|webp|txt|ifo|bup|m2ts|bdmv|mpls|clpi)$", re.IGNORECASE)


@dataclass(frozen=True)
class Library:
    """The titles of the owner's library this machine has: each title's words, by its first two."""

    roots: tuple[str, ...]
    by_start: dict[tuple[str, str], tuple[tuple[str, ...], ...]]
    count: int


def library_roots() -> tuple[str, ...]:
    """The library's folders this machine has: ORPHEUM_MEDIA_LIBRARY's, else the owner's."""
    named = os.environ.get(LIBRARY_ENV)
    roots = (named.split(os.pathsep) if named else []) if named is not None else [DEFAULT_LIBRARY]
    return tuple(root for root in roots if root and readable_folder(root))


def readable_folder(root: str) -> bool:
    """Whether root is a folder this machine can read. A network mount whose server is down answers
    an I/O error rather than "no such folder" (nas.lan's crash, 2026-11-21: every commit's message
    lint failed with OSError), and an unreadable library is no library here."""
    try:
        return Path(root).is_dir()
    except OSError:
        return False


def library_title(name: str) -> tuple[str, ...]:
    """The words of the title a library folder's or file's name carries, as the registry reads it."""
    return tuple(WORD_RE.findall(media_title(LIBRARY_SUFFIXES.sub("", name))))


def read_library(roots: tuple[str, ...], registry: Registry) -> Library:
    """Every title of two words or more under roots, but a registered one and a folder's kind."""
    titles: set[tuple[str, ...]] = set()
    for root in roots:
        for _, folders, files in os.walk(root):
            # A subtitle's file is named for its language or its video ("5_English.srt"), and a
            # hidden one (.DS_Store) for no film.
            kept = [f for f in files if not SUBTITLE_SUFFIXES.search(f)]
            for name in folders + kept:
                if name.startswith("."):
                    continue
                words = library_title(name)
                # A figure and one word is a track's or a disc's ("5 English"), not a title.
                named = words[1:] if words and words[0].isdigit() else words
                if len(named) < 2 or all(w in FOLDER_WORDS or w.isdigit() for w in words):
                    continue
                if " ".join(words) in registry.names or " ".join(words) in registry.words:
                    continue
                titles.add(words)
    by_start: dict[tuple[str, str], list[tuple[str, ...]]] = {}
    for words in sorted(titles, key=len, reverse=True):  # the longest first: it is the one quoted
        by_start.setdefault((words[0], words[1]), []).append(words)
    return Library(roots, {start: tuple(found) for start, found in by_start.items()}, len(titles))


@functools.cache
def machine_library(registry: Registry) -> Library | None:
    """The library this machine has, read once a run; None where it has none."""
    roots = library_roots()
    return read_library(roots, registry) if roots else None


def written_as_a_title(words: list[str]) -> bool:
    """Whether words are written as a title is: the first and each after capitalised or a figure,
    a small word after the first aside, and one at least capitalised. A title's lower-case start
    (\"Who Stole the Lighthouse\") is inside another's."""
    def big(word: str) -> bool:
        return word[0].isupper() or word[0].isdigit()

    return any(w[0].isupper() for w in words) and big(words[0]) and all(
        big(w) or w.casefold() in TITLE_SMALL_WORDS for w in words[1:]
    )


@dataclass(frozen=True)
class Finding:
    where: str
    rule: str
    text: str


def find(text: str, registry: Registry, path: str | None, library: Library | None = None) -> list[Finding]:
    """What in text names a film, series or media file the registry does not list, or a title of
    library. Each rule runs over the whole text, none of its patterns but a library title's crossing
    a line, and a match's line is found after."""
    starts = [0] + [m.end() for m in re.finditer("\n", text)]

    def where(offset: int) -> str:
        number = bisect.bisect_right(starts, offset)
        return f"{path}:{number}" if path else f"line {number}"

    found: list[tuple[int, Finding]] = []

    def add(offset: int, rule: str, matched: str) -> None:
        found.append((offset, Finding(where(offset), rule, matched)))

    in_files: list[tuple[int, int]] = []
    for ext in MEDIA_EXTENSION_RE.finditer(text):
        # A name starts on its line and after the file named before it ("a.mkv, b.mkv").
        line_start = max(text.rfind("\n", 0, ext.start()) + 1, in_files[-1][1] if in_files else 0)
        run = ESCAPED_NAME_RUN_RE.search(text, line_start, ext.start())
        start = run.start() if run else ext.start()
        in_files.append((start, ext.end()))
        decoded = re.sub(r"\\u([0-9a-fA-F]{4})", lambda m: chr(int(m[1], 16)), text[start : ext.start()])
        name = decoded[NAME_RUN_RE.search(decoded).start() :].lstrip(" ,;:()&!+'-")
        if not names_a_file(name, ext.group(0)) or registry.knows_file(name):
            continue
        add(start, "media-file", name + ext.group(0))
    for year in YEAR_RE.finditer(text):
        line_start = text.rfind("\n", 0, year.start()) + 1
        title = YEAR_TITLE_RE.search(text, line_start, year.start())
        # The words after a sentence's end are the title; in capitals and figures alone they are an
        # id ("CVE-2024-10441 (2024)"), not how a title is written.
        last = re.split(r"\. ", title.group(1))[-1] if title else ""
        if title and re.search(r"[^\W\d_]", last) and not is_acronyms(last) and not registry.ends_with_known(
            title.group(1)
        ):
            quoted = YEAR_TITLE_QUOTED_RE.search(text, title.start(), year.start())
            start = quoted.start() if quoted else title.start()
            add(start, "title-and-year", text[start : year.end()])
    if path is None or not registry.allows_episodes_in(path):
        for m in EPISODE_RE.finditer(text):
            if not any(a <= m.start() < b for a, b in in_files):
                add(m.start(), "episode-marker", m.group(0))
    for phrase_re in PHRASE_RES:
        for m in phrase_re.finditer(text):
            phrase = re.split(r"\.(?= )", m.group(1))[0]  # a sentence ends it
            if not is_acronyms(phrase) and not registry.knows(phrase):
                add(m.start(), "named-in-prose", m.group(0))
    if library is not None:
        for start, quoted in library_titles_in(text, library):
            add(start, "library-title", quoted)
    return [f for _, f in sorted(found, key=lambda pair: pair[0])]


def library_titles_in(text: str, library: Library) -> list[tuple[int, str]]:
    """Each title of library text writes as a title, with where it starts."""
    words = list(WORD_RE.finditer(unicodedata.normalize("NFC", text)))
    folded = [w.group(0).casefold() for w in words]
    found: list[tuple[int, str]] = []
    i = 0
    while i < len(words) - 1:
        for title in library.by_start.get((folded[i], folded[i + 1]), ()):
            end = i + len(title)
            if tuple(folded[i:end]) == title and written_as_a_title([w.group(0) for w in words[i:end]]):
                found.append((words[i].start(), " ".join(w.group(0) for w in words[i:end])))
                i = end - 1
                break
        i += 1
    return found


def tracked_files() -> list[str]:
    out = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard"],
        capture_output=True, text=True, check=True,
    ).stdout
    return [p for p in out.splitlines() if p not in SKIPPED and not p.endswith(SKIPPED_SUFFIXES)]


def find_in_files(paths: list[str], registry: Registry, library: Library | None = None) -> list[Finding]:
    out: list[Finding] = []
    for path in paths:
        if path in SKIPPED or path.endswith(SKIPPED_SUFFIXES):  # a path spot names, as make check skips it
            continue
        try:
            data = Path(path).read_bytes()
        except (FileNotFoundError, IsADirectoryError):
            continue
        if b"\0" in data[:8192]:
            continue
        out.extend(find(data.decode("utf-8", errors="replace"), registry, path, library))
    return out


HINT = (
    f"Name a film by its id or as \"the owner's film\". A test that needs a name takes one "
    f"{REGISTRY} lists; a new one, an old film or an invented one and never a line from a file "
    f"listing, is added there with why, a gate change (the owner's Gate-Changed: trailer)."
)


def compared(library: Library | None) -> str:
    """What the library rule compared, for the gate's last line."""
    if library is None:
        return f"no library here to compare titles with ({DEFAULT_LIBRARY} or {LIBRARY_ENV})"
    return f"none of the {library.count} titles of the library at {', '.join(library.roots)} is written"


def unlisted(f: Finding) -> str:
    """Why a finding fails."""
    if f.rule == "library-title":
        return "is a title of the owner's library, written as a title"
    return f"is not in {REGISTRY}"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("paths", nargs="*", help="files to check (default: every tracked file)")
    ap.add_argument("--message-file", help="check a commit message instead")
    ap.add_argument("--report", action="store_true", help="count the unlisted names, most first")
    args = ap.parse_args()
    registry = read_registry(REGISTRY.read_text(encoding="utf-8"))
    library = machine_library(registry)
    if args.message_file:
        found = find(Path(args.message_file).read_text(encoding="utf-8"), registry, None, library)
    else:
        found = find_in_files(args.paths or tracked_files(), registry, library)
    if args.report:
        for (rule, text), count in Counter((f.rule, f.text) for f in found).most_common():
            print(f"{count:5d} [{rule}] {text}")
        return 0
    if not found:
        print(f"check_media_names: every film, series and media file named is listed; {compared(library)}")
        return 0
    for f in found:
        print(f"{f.where}: [{f.rule}] {f.text!r} {unlisted(f)}", file=sys.stderr)
    print(f"\ncheck_media_names: {len(found)} name(s) unlisted. {HINT}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
