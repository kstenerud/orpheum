# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for emitted_js.py: a change of types or comments alone compiles to the same JavaScript,
# ABOUTME: and anything else, or anything it cannot compile, does not.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import emitted_js  # noqa: E402

BEFORE = "export type Body = { a: number };\nexport const make = (a: number): Body => ({ a });\n"


class EmitsTheSame(unittest.TestCase):
    def same(self, after: str, path: str = "a.ts") -> bool:
        return emitted_js.emits_the_same(path, BEFORE, after)

    def test_types_and_comments_alone_compile_to_the_same_javascript(self):
        self.assertTrue(self.same("export type Body = { a: number; b?: string };\n"
                                  "export const make = (a: number): Body => ({ a });\n"))
        self.assertTrue(self.same("// Made here.\nexport type Body = { a: number };\n"
                                  "export const make = (a: number): Body => ({ a }); // and kept\n"))

    def test_a_changed_value_an_import_or_jsx_does_not(self):
        self.assertFalse(self.same("export type Body = { a: number };\nexport const make = (a: number): Body => ({ a: a + 1 });\n"))
        self.assertFalse(self.same('import "./b.ts";\n' + BEFORE))
        self.assertFalse(emitted_js.emits_the_same("a.tsx", "export const A = () => <b />;\n",
                                                   "export const A = () => <i />;\n"))

    def test_what_does_not_parse_or_is_not_typescript_is_never_the_same(self):
        self.assertFalse(self.same(BEFORE + "export const = ;\n"))
        self.assertFalse(emitted_js.emits_the_same("a.css", "a {}\n", "a {}\n"))


if __name__ == "__main__":
    unittest.main()
