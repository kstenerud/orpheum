# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for check_media_names.py: a film, series or media file the registry does not list
# ABOUTME: fails, in a file and in a commit message; a listed one, and code that only looks like one, passes.
#
# Every unlisted name here is invented ("Blue Gannet", "Quarry Lamps"): a test of this gate never
# names a real film, which would put it in the repository the gate keeps such names out of.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_media_names as cmn  # noqa: E402

REGISTRY = """
# a comment
[names]
velvet static | invented for the tests
velvet static: chapter 3 | invented, a title with a colon
der blaue kanal | invented, a German title
film | a stand-in
tvshow | Kodi's sidecar

[words]
fire tv | a device

[episode-files]
internal/core/identify/* | the parser
"""


def registry() -> cmn.Registry:
    return cmn.read_registry(REGISTRY)


def rules(text: str, path: str | None = "docs/x.md") -> list[tuple[str, str]]:
    return [(f.rule, f.text) for f in cmn.find(text, registry(), path)]


class Registry(unittest.TestCase):
    def test_an_entry_without_its_reason_is_refused(self):
        with self.assertRaises(ValueError):
            cmn.read_registry("[names]\nvelvet static\n")
        with self.assertRaises(ValueError):
            cmn.read_registry("[names]\nvelvet static |   \n")

    def test_an_unknown_section_is_refused(self):
        with self.assertRaises(ValueError):
            cmn.read_registry("[films]\nvelvet static | invented\n")

    def test_names_are_read_as_their_normal_form(self):
        self.assertIn("velvet static", cmn.read_registry("[names]\nVelvet.Static | x\n").names)


class MediaFiles(unittest.TestCase):
    def test_an_unlisted_film_file_fails(self):
        self.assertEqual(
            rules('{"Blue Gannet (1987).mkv"}'),
            [("media-file", "Blue Gannet (1987).mkv"), ("title-and-year", "Blue Gannet (1987)")],
        )

    def test_a_listed_film_file_passes_whatever_its_tags(self):
        for name in (
            "Velvet Static (2016).mkv",
            "Velvet.Static.2016.1080p.BluRay.x264-GROUP.mkv",
            "media/Velvet Static/Velvet Static - 720p.en.forced.srt",
            "Velvet Static-trailer.mkv",
            "[Group] Velvet Static - 05v2 [1080p][ABCD1234].mkv",
            "Velvet.Static.S01E02.mkv",
            "VELVET STATIC.MKV",
        ):
            with self.subTest(name):
                self.assertEqual(rules(f'"{name}"'), [])

    def test_a_file_named_in_prose_is_its_last_words(self):
        self.assertEqual(rules("the scan plays film.mkv first"), [])
        self.assertEqual(rules("each series keeps its tvshow.nfo"), [])
        self.assertEqual(rules("then Blue Film.mkv"), [("media-file", "then Blue Film.mkv")])

    def test_two_files_on_a_line_are_two_names(self):
        self.assertEqual(
            rules("film.mkv, Quarry Lamps.mkv"), [("media-file", "Quarry Lamps.mkv")]
        )

    def test_a_name_of_no_letters_names_nothing(self):
        self.assertEqual(rules("00001.m4s and 01.mkv and .mkv"), [])

    def test_code_is_not_a_file(self):
        self.assertEqual(rules("age := now.Sub(file.ModifiedAt)"), [])
        self.assertEqual(rules("got := string(text.VTT)"), [])
        self.assertEqual(rules("sound (.ogg), or .vtt"), [])

    def test_an_accent_written_as_a_combining_mark_is_the_same_name(self):
        self.assertEqual(rules('"Velvet Sta\u0301tic.mkv"', ), [("media-file", "Velvet Sta\u0301tic.mkv")])
        self.assertEqual(cmn.read_registry("[names]\nvelvet st\u00e1tic | x\n").knows_file("Velvet Sta\u0301tic"), True)

    def test_an_escaped_accent_in_code_is_the_same_name(self):
        accented = cmn.read_registry("[names]\nvelvet st\u00e1tic | x\n")
        self.assertEqual(cmn.find('"Velvet Sta\\u0301tic.mkv"', accented, "x.go"), [])
        self.assertEqual(rules('"Quarry La\\u0301mps.mkv"'), [("media-file", "Quarry La\u0301mps.mkv")])
        self.assertEqual(rules('"\\u003e.vtt"'), [])

    def test_a_key_before_a_listed_title_and_year_names_nothing_more(self):
        self.assertEqual(rules('"10:1:Velvet Static (2016).mkv"'), [])

    def test_an_extra_named_after_a_space_is_its_film(self):
        self.assertEqual(rules('"Velvet Static teaser.mkv", "Velvet Static trailer.mkv"'), [])
        self.assertEqual(rules('"Quarry Lamps trailer.mkv"'), [("media-file", "Quarry Lamps trailer.mkv")])

    def test_a_fansubs_version_after_its_episode_is_no_part_of_the_title(self):
        self.assertEqual(cmn.media_title("[Group]_Quarry_Lamps_-_03_v2_[720p-10bit] [C66E9481]"), "quarry lamps")

    def test_an_episode_file_marks_no_episode(self):
        self.assertEqual(rules('"Velvet Static/Season 1/Velvet Static - S01E02.mkv"'), [])


class TitlesAndYears(unittest.TestCase):
    def test_an_unlisted_title_before_a_year_fails(self):
        self.assertEqual(
            rules("We watched Quarry Lamps (1987)."),
            [("named-in-prose", "watched Quarry Lamps"), ("title-and-year", "Quarry Lamps (1987)")],
        )

    def test_a_listed_title_starting_a_sentence_passes(self):
        self.assertEqual(rules("Then Velvet Static (2016) played."), [])

    def test_a_year_after_figures_alone_names_nothing(self):
        self.assertEqual(rules("iOS 18.1 (2024)"), [])
        self.assertEqual(rules("WebKit bug 117855 (2013)"), [])

    def test_an_id_in_capitals_before_a_year_names_nothing(self):
        self.assertEqual(rules("fixed on 2026-09-18. CVE-2024-10441 (2024)"), [])

    def test_a_listed_title_with_lowercase_words_inside_passes(self):
        self.assertEqual(rules("Der blaue Kanal (1924) played"), [])
        self.assertEqual(rules("Der graue Kanal (1924)"), [("title-and-year", "Kanal (1924)")])

    def test_a_listed_title_with_its_tags_or_a_colon_passes(self):
        self.assertEqual(rules("Velvet Static Unrated (2016), Velvet Static Remastered (2016)"), [])
        self.assertEqual(rules("Velvet Static: Chapter 3 (2021)"), [])
        self.assertEqual(rules("Quarry Lamps Unrated (2016)"), [("title-and-year", "Quarry Lamps Unrated (2016)")])


class EpisodeMarkers(unittest.TestCase):
    def test_a_marker_in_prose_fails(self):
        self.assertEqual(rules("it stalled in s2e5 at 3:30"), [("episode-marker", "s2e5")])
        self.assertEqual(rules("the 2x05 one"), [("episode-marker", "2x05")])

    def test_a_marker_in_a_listed_file_passes(self):
        self.assertEqual(rules("S02E05", path="internal/core/identify/filename.go"), [])

    def test_a_picture_size_is_no_marker(self):
        self.assertEqual(rules("a 64x64 picture"), [])

    def test_a_commit_message_has_no_listed_file(self):
        self.assertEqual(rules("S02E05", path=None), [("episode-marker", "S02E05")])


class Prose(unittest.TestCase):
    def test_a_title_in_the_owners_mouth_fails(self):
        self.assertEqual(
            rules("it stood still on the owner's Quarry Lamps"),
            [("named-in-prose", "the owner's Quarry Lamps")],
        )

    def test_a_title_after_playing_fails(self):
        self.assertEqual(
            rules('while playing "Quarry Lamps"'), [("named-in-prose", 'playing "Quarry Lamps')]
        )

    def test_a_listed_word_passes(self):
        self.assertEqual(rules("on the owner's Fire TV Stick"), [])

    def test_acronyms_and_ids_pass(self):
        self.assertEqual(rules("the owner's VQ105, the owner's TCL TV, plays HEVC"), [])

    def test_a_sentence_ends_the_phrase(self):
        self.assertEqual(rules("the owner's VPS. It"), [])


class MediaTitle(unittest.TestCase):
    def test_the_title_is_what_comes_before_the_tags(self):
        self.assertEqual(cmn.media_title("Quarry.Lamps.1987.2160p.WEB-DL.DDP5.1"), "quarry lamps")
        self.assertEqual(cmn.media_title("Quarry Lamps (1987).en.forced"), "quarry lamps")
        self.assertEqual(cmn.media_title("Quarry Lamps - 2x05 - The Kiln"), "quarry lamps")
        self.assertEqual(cmn.media_title("Quarry Lamps Director's Cut"), "quarry lamps")


if __name__ == "__main__":
    unittest.main()
