# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for browser_map.py: a stylesheet change reaches only the browser tests whose pages held
# ABOUTME: what its changed rules select, and anything it cannot narrow reaches every test.
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import browser_map  # noqa: E402

SHEET = """/* The panel. */
.panel { gap: 1rem; }
.panel__title { font-weight: 600; }
@media (width >= 600px) {
  .panel__title:hover { color: red; }
}
button.panel__close[aria-pressed="true"]::before { content: "x"; }
@keyframes panel-in { from { opacity: 0; } }
"""

PLAY = browser_map.Recorded("chromium play.spec.ts plays", frozenset({"tag:div", "class:play", "class:panel"}))
TOGETHER = browser_map.Recorded("firefox together.spec.ts joins", frozenset(
    {"tag:div", "tag:button", "class:panel", "class:panel__title", "class:panel__close", "attr:aria-pressed"}))
SHELL = browser_map.Recorded("chromium shell.spec.ts logo", frozenset({"tag:div", "class:app-shell"}))
MAP = [PLAY, TOGETHER, SHELL]


class Reached(unittest.TestCase):
    def reached(self, head: str, base: str = SHEET) -> list[str] | None:
        return browser_map.reached(MAP, [(base, head)])

    def test_a_changed_declaration_reaches_only_the_tests_whose_pages_held_its_selector(self):
        self.assertEqual(self.reached(SHEET.replace("gap: 1rem", "gap: 2rem")), [PLAY.test, TOGETHER.test])
        self.assertEqual(self.reached(SHEET.replace("font-weight: 600", "font-weight: 700")), [TOGETHER.test])

    def test_a_rule_whose_selector_no_page_held_reaches_no_test(self):
        self.assertEqual(self.reached(SHEET + ".settings__row { margin: 0; }\n"), [])
        self.assertEqual(self.reached(SHEET.replace(".panel {", ".panel, .settings {")), [PLAY.test, TOGETHER.test],
                         "a selector list's changed rule needs any one of its selectors held")

    def test_a_removed_or_added_rule_counts_from_either_side(self):
        self.assertEqual(self.reached(SHEET.replace(".panel { gap: 1rem; }\n", "")), [PLAY.test, TOGETHER.test])
        self.assertEqual(self.reached(SHEET, SHEET.replace(".panel { gap: 1rem; }\n", "")), [PLAY.test, TOGETHER.test])

    def test_a_selector_needs_every_token_of_every_compound(self):
        self.assertEqual(self.reached(SHEET.replace('content: "x"', 'content: "y"')), [TOGETHER.test])
        held_without_attribute = browser_map.Recorded("t", TOGETHER.tokens - {"attr:aria-pressed"})
        self.assertEqual(browser_map.reached([held_without_attribute], [(SHEET, SHEET.replace('"x"', '"y"'))]), [])

    def test_a_changed_media_condition_changes_the_rules_inside_it(self):
        self.assertEqual(self.reached(SHEET.replace("width >= 600px", "width >= 700px")), [TOGETHER.test])

    def test_reordered_rules_count_as_changed_since_order_decides_the_cascade(self):
        swapped = SHEET.replace(".panel { gap: 1rem; }\n.panel__title { font-weight: 600; }",
                                ".panel__title { font-weight: 600; }\n.panel { gap: 1rem; }")
        self.assertNotEqual(self.reached(swapped), [], "a reorder that no test sees is still a change")

    def test_what_may_style_anything_reaches_every_test(self):
        for name, head in [
            ("a rule on :root", SHEET + ":root { --gap: 1rem; }\n"),
            ("a rule on everything", SHEET + "* { margin: 0; }\n"),
            ("a changed @keyframes", SHEET.replace("opacity: 0", "opacity: 0.5")),
            ("an added @font-face", SHEET + "@font-face { font-family: x; src: url(x.woff2); }\n"),
            ("an @import", '@import "other.css";\n' + SHEET),
            ("an escaped class", SHEET + ".a\\:b { margin: 0; }\n"),
            ("a nested rule", SHEET + ".panel { & .x { margin: 0; } }\n"),
            ("a pseudo-class alone", SHEET + ":is(.a, .b) { margin: 0; }\n"),
            ("an unclosed block", SHEET + ".panel { gap: 0;\n"),
        ]:
            with self.subTest(name):
                self.assertIsNone(self.reached(head))

    def test_a_test_whose_record_was_not_vouched_for_is_reached_by_every_change(self):
        unvouched = browser_map.Recorded("chromium x.spec.ts opened a context unwatched", None)
        self.assertEqual(browser_map.reached([*MAP, unvouched], [(SHEET, SHEET.replace("gap: 1rem", "gap: 2rem"))]),
                         [PLAY.test, TOGETHER.test, unvouched.test])

    def test_an_unchanged_sheet_reaches_no_test_whatever_its_comments_and_spacing(self):
        self.assertEqual(self.reached(SHEET.replace("/* The panel. */", "/* Changed. */").replace("{ gap", "{  gap")), [])


class Tokens(unittest.TestCase):
    def test_pseudo_classes_and_elements_narrow_but_require_nothing(self):
        self.assertEqual(browser_map.selector_tokens("button.irreversible:hover:not(:disabled)"),
                         frozenset({"tag:button", "class:irreversible"}))
        self.assertEqual(browser_map.selector_tokens(".menu > summary::-webkit-details-marker"),
                         frozenset({"class:menu", "tag:summary"}))
        self.assertEqual(browser_map.selector_tokens("input:not([type=checkbox], [type=radio])"),
                         frozenset({"tag:input"}))

    def test_attribute_names_whatever_their_operator_and_case(self):
        self.assertEqual(browser_map.selector_tokens("[aria-pressed=true]"), frozenset({"attr:aria-pressed"}))
        self.assertEqual(browser_map.selector_tokens('a[Lang|="en"]'), frozenset({"tag:a", "attr:lang"}))

    def test_every_compound_counts_across_every_combinator(self):
        for selector in (".a .b", ".a>.b", ".a + .b", ".a ~ .b", ".a:has(.c) .b"):
            with self.subTest(selector):
                self.assertEqual(browser_map.selector_tokens(selector), frozenset({"class:a", "class:b"}))

    def test_a_compound_naming_nothing_is_narrowed_by_the_others(self):
        self.assertEqual(browser_map.selector_tokens(".sound__answer > :not(:first-child)"),
                         frozenset({"class:sound__answer"}))

    def test_a_namespace_reads_as_anything(self):
        self.assertIsNone(browser_map.selector_tokens("svg|rect"))


class Load(unittest.TestCase):
    def test_a_map_counts_only_when_complete_and_recorded_at_the_commit_checked(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "chromium").mkdir()
            (root / "chromium" / "1.json").write_text(json.dumps({"test": PLAY.test, "tokens": sorted(PLAY.tokens)}))
            self.assertIsNone(browser_map.load(root, "abc"), "no completion stamp: the run did not pass")
            (root / browser_map.COMPLETE).write_text("abc\n")
            self.assertEqual(browser_map.load(root, "abc"), [PLAY])
            self.assertIsNone(browser_map.load(root, "def"), "recorded at another commit")
            (root / "chromium" / "2.json").write_text(json.dumps({"test": "unvouched", "tokens": None}))
            self.assertIn(browser_map.Recorded("unvouched", None), browser_map.load(root, "abc") or [])


if __name__ == "__main__":
    unittest.main()
