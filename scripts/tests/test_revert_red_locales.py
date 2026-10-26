# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_locales.py: a locale file's changed keys are read from its two
# ABOUTME: versions, spreads followed, and the tests rendering them run before the probe's usual ones.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_locales as loc  # noqa: E402
from gitrepo import Repo  # noqa: E402

SRC = "web/apps/web/src"
LOCALE = f"{SRC}/locales/en-x.ts"
PART = f"{SRC}/locales/en-part.ts"

CATALOGUE = '''import { enXPart } from "./en-part.ts";

export const enX = {
  greeting: "Hello",
  farewell: "Bye",
  settings: {
    ...enXPart,
    general: "General",
  },
};
'''
PART_SOURCE = '''export const enXPart = {
  advanced: "Advanced",
};
'''

FILES = {
    "web/apps/web/src/locales/en-x.ts": CATALOGUE,
    "web/apps/web/src/locales/en-part.ts": PART_SOURCE,
    f"{SRC}/greeter.tsx": 'import "./i18n.ts";\nexport function Greeter(t) { return t("greeting"); }\n',
    f"{SRC}/greeter.test.tsx": 'import { Greeter } from "./greeter.tsx";\nGreeter;\n',
    f"{SRC}/built.tsx": 'export function Built(t, section) { return t(`settings.${section}`); }\n',
}


class ParsingAndDiffing(unittest.TestCase):
    def setUp(self):
        self.repo = Repo(FILES)
        self.base = self.repo.commit("base")

    def changed(self, files: dict[str, str], path: str = LOCALE) -> set[str] | None:
        head = self.repo.commit("change", files)
        return loc.changed_locale_keys(str(self.repo.root), self.base, head, path)

    def test_a_changed_leaf_is_found_and_an_unchanged_one_is_not(self):
        self.assertEqual(self.changed({LOCALE: CATALOGUE.replace('"Hello"', '"Hi"')}),
                         {"greeting"})

    def test_an_added_and_a_removed_leaf_are_both_found(self):
        after = CATALOGUE.replace('farewell: "Bye",', 'parting: "See you",')
        self.assertEqual(self.changed({LOCALE: after}), {"farewell", "parting"})

    def test_a_spread_is_followed_to_the_file_it_imports_at_the_same_revision(self):
        # The spread's own file changed, not the spreading file; still found through en-x.ts's import.
        self.assertEqual(self.changed({PART: PART_SOURCE.replace('"Advanced"', '"More"')}),
                         {"settings.advanced"})

    def test_a_file_new_at_head_has_every_leaf_changed_from_nothing(self):
        repo = Repo({path: text for path, text in FILES.items() if path != LOCALE})
        base = repo.commit("base, without the locale file yet")
        head = repo.commit("feat(web): add it", {LOCALE: CATALOGUE})
        found = loc.changed_locale_keys(str(repo.root), base, head, LOCALE)
        self.assertEqual(found, {"greeting", "farewell", "settings.general", "settings.advanced"})

    def test_a_value_this_cannot_parse_leaves_the_file_to_the_usual_runs(self):
        for after in (CATALOGUE.replace('"Hello"', "1"),
                      CATALOGUE.replace('"Hello"', "[1, 2]"),
                      CATALOGUE.replace("export const enX = {", "export const enX = someFunction({")):
            self.assertIsNone(self.changed({LOCALE: after}), after)

    def test_an_unresolved_spread_leaves_the_file_to_the_usual_runs(self):
        after = CATALOGUE.replace("...enXPart,", "...enMissing,")
        self.assertIsNone(self.changed({LOCALE: after}))


class KeyUsage(unittest.TestCase):
    def test_a_static_literal_is_matched_and_a_built_key_is_not(self):
        repo = Repo(FILES)
        root = str(repo.root)
        self.assertEqual(loc.matching_source_files(root, {"greeting"}), ["apps/web/src/greeter.tsx"])
        # "settings.general" is only ever asked for through a built string in built.tsx; the scan
        # does not trust that it is being asked for, so nothing matches it.
        self.assertEqual(loc.matching_source_files(root, {"settings.general"}), [])

    def test_a_test_file_and_the_locales_directory_itself_are_not_scanned(self):
        repo = Repo({**FILES, f"{SRC}/stray.test.tsx": 't("greeting");\n'})
        root = str(repo.root)
        self.assertEqual(loc.matching_source_files(root, {"greeting"}), ["apps/web/src/greeter.tsx"])


class LocaleRuns(unittest.TestCase):
    def setUp(self):
        self.repo = Repo(FILES)
        self.base = self.repo.commit("base")

    def test_the_catalogues_own_test_always_comes_first_then_the_tests_rendering_the_changed_key(self):
        head = self.repo.commit("change", {LOCALE: CATALOGUE.replace('"Hello"', '"Hi"')})
        attempts = loc.locale_runs(str(self.repo.root), self.base, head, LOCALE)
        self.assertEqual([a.command for a in attempts], [
            loc.vitest("run", loc.CATALOGUE_TEST),
            loc.vitest("related", "apps/web/src/greeter.tsx"),
        ])

    def test_a_change_the_scan_cannot_place_still_gets_the_catalogues_own_test_and_nothing_else(self):
        head = self.repo.commit("change", {LOCALE: CATALOGUE.replace('"General"', '"Overall"')})
        attempts = loc.locale_runs(str(self.repo.root), self.base, head, LOCALE)
        self.assertEqual([a.command for a in attempts], [loc.vitest("run", loc.CATALOGUE_TEST)])


class Attempts(unittest.TestCase):
    """As revert_red_naming's Go tests that name a change: these come first and decide only a red;
    every run the probe made before still follows, so a green verdict rests on the same runs as ever."""

    def setUp(self):
        self.repo = Repo(FILES)
        self.base = self.repo.commit("base")
        self.head = self.repo.commit("change", {LOCALE: CATALOGUE.replace('"Hello"', '"Hi"')})

    def attempts(self) -> list[list[str]]:
        with self.repo.cwd():
            return [attempt.command for attempt in rr.attempts_for(
                rr.Change(LOCALE, "M"), rr.scope_for(LOCALE), self.base, self.head)]

    def test_the_catalogue_test_and_the_rendering_tests_come_before_the_usual_whole_file_run(self):
        runs = self.attempts()
        self.assertEqual(runs[:3], [
            loc.vitest("run", loc.CATALOGUE_TEST),
            loc.vitest("related", "apps/web/src/greeter.tsx"),
            rr.scope_for(LOCALE),
        ])

    def test_every_run_the_probe_made_without_them_still_follows(self):
        saved = rr.locale_runs
        rr.locale_runs = lambda *_args: []
        try:
            without = self.attempts()
        finally:
            rr.locale_runs = saved
        with_them = self.attempts()
        self.assertEqual(with_them[len(with_them) - len(without):], without)


if __name__ == "__main__":
    unittest.main()
