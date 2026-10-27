# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for handoff.py: the handoff quotes the records, a decision not built by its own
# ABOUTME: status line, an open VQ item, a code-debt row, so no claim in it is the session's memory.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import handoff  # noqa: E402

DECISIONS = """## D1 — Built thing

**Status:** Decided, and built (2026-10-01).

## D2 — Unbuilt thing

1. **Point.** x

**Status:** Decided; not built.

## D3 — Reserved: Something later

**Reserved** (2026-10-26).
"""


class Handoff(unittest.TestCase):
    def test_a_decision_not_built_is_quoted_by_its_own_status(self):
        found = handoff.unbuilt_decisions(DECISIONS)
        self.assertEqual(len(found), 2, found)
        self.assertIn("D2", found[0])
        self.assertIn("Status: Decided; not built.", found[0])
        self.assertIn("D3", found[1])

    def test_open_checks_and_debts_are_listed(self):
        queue = "- [x] **VQ1** — done\n- [ ] **VQ2** — a TV plays it\n"
        self.assertEqual(handoff.open_checks(queue), ["- VQ2 — a TV plays it"])
        table = "| Debt | The tax | The fix | Found |\n| --- | --- | --- | --- |\n| A raw insert | x | y | z |\n"
        self.assertEqual(handoff.debts(table), ["- A raw insert"])


if __name__ == "__main__":
    unittest.main()
