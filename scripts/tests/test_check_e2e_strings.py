# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for check_e2e_strings.py: an e2e spec's words the web client no longer has fail make check.
from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_e2e_strings as ces  # noqa: E402

WORDS = '''export const enCast = {
  // a comment's "Words in a comment" are not words
  playingOn: "Playing on {{name}}",
  use: "Use",
  question: `{{name}} wants this device`,
  apostrophe: "Your server's link",
};
'''


class Strings(unittest.TestCase):
    """The owner's go, 2026-11-13: a UI string an e2e spec waits for, gone from the locales, fails."""

    def check(self, spec: str, spec_name: str = "cast.spec.ts") -> list[str]:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "web/apps/web/src/locales").mkdir(parents=True)
            (root / "web/e2e").mkdir(parents=True)
            (root / "web/apps/web/src/locales/en-cast.ts").write_text(WORDS)
            (root / "web/apps/web/src/system-words.ts").write_text('export const w = { a: "Hardware encoding" };\n')
            (root / "web/e2e" / spec_name).write_text(spec)
            return ces.problems(root)

    def test_words_the_client_has_pass_whole_in_part_folded_or_through_a_placeholder(self):
        spec = ('page.getByRole("button", { name: "Use" });\npage.getByText("playing on");\n'
                'page.getByText("Playing on the TV");\npage.getByRole("heading", { name: "Your server\'s link" });\n'
                'page.getByRole("heading", { name: "Hardware encoding" });\npage.getByText(/^Playing on /u);\n')
        self.assertEqual(self.check(spec), [])

    def test_words_taken_from_the_client_fail_naming_the_line(self):
        spec = 'await page.getByRole("button", { name: "Use" });\nawait page.getByRole("button", { name: "Asking" });\n'
        self.assertEqual(self.check(spec), ["web/e2e/cast.spec.ts:2: waits for 'Asking', which the web client's "
                                            "words no longer have"])

    def test_a_pattern_no_catalogue_string_matches_fails(self):
        self.assertEqual(len(self.check("page.getByText(/^Words in a comment/u);\n")), 1)

    def test_a_locator_expected_to_find_nothing_may_name_words_gone_on_purpose(self):
        spec = ('await expect(page.getByRole("dialog", { name: /wants to use this device/u })).toHaveCount(\n  0,\n);\n'
                'await expect(page.getByText("Asking")).not.toBeVisible();\n'
                'await expect(page.getByText("Asking")).toBeHidden();\n')
        self.assertEqual(self.check(spec), [])

    def test_a_string_in_a_variable_and_the_e2e_machinerys_own_files_are_not_read(self):
        self.assertEqual(self.check("page.getByRole('button', { name: filmTitle });\n"), [])
        self.assertEqual(self.check('projects: [{ name: "chromium" }]\n', "playwright.config.ts"), [])
        self.assertEqual(self.check('x.getByText("Gone");\n', "timings.test.ts"), [])

    def test_the_tree_passes(self):
        self.assertEqual(ces.problems(Path(__file__).resolve().parents[2]), [])


if __name__ == "__main__":
    unittest.main()
