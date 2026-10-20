# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for the browser timings reader: the latest pass's slowest tests, against their median
# ABOUTME: over the passes kept before it, each with its longest steps.
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import browser_timings  # noqa: E402


def a_pass(started: str, guest_ms: int) -> dict:
    return {"startedAt": started, "pass": "side-by-side", "ms": guest_ms + 1000, "tests": [
        {"test": "a guest joins", "project": "firefox", "file": "together.spec.ts", "line": 70,
         "status": "passed", "ms": guest_ms, "worker": 0,
         "steps": [{"title": "Expect \"poll toBe\"", "category": "expect", "ms": guest_ms - 500, "depth": 0},
                   {"title": "Click", "category": "pw:api", "ms": 300, "depth": 1}]},
        {"test": "a page loads", "project": "chromium", "file": "shell.spec.ts", "line": 18,
         "status": "passed", "ms": 900, "worker": 1, "steps": []},
    ]}


class Report(unittest.TestCase):
    def test_the_latest_slowest_first_against_their_earlier_median_with_their_longest_steps(self):
        directory = Path(tempfile.mkdtemp())
        for started, guest in (("2026-10-18T10-00-00.000Z", 4000), ("2026-10-19T10-00-00.000Z", 6000),
                               ("2026-10-20T10-00-00.000Z", 15000)):
            (directory / f"{started}-side-by-side.json").write_text(json.dumps(a_pass(started, guest)))
        (directory / "2026-10-20T11-00-00.000Z-timed.json").write_text(json.dumps(a_pass("t", 99000)))
        lines = browser_timings.report(browser_timings.passes(directory, "side-by-side"), top=1, steps=1)
        self.assertIn("medians over the 2 passes before it", lines[0])
        self.assertEqual(lines[1], "  15.0 s  (median   5.0 s)  [firefox] together.spec.ts:70 a guest joins")
        self.assertEqual(lines[2], "             14.5 s  expect: Expect \"poll toBe\"")
        self.assertEqual(len(lines), 3)

    def test_with_nothing_kept_it_says_where_timings_come_from(self):
        lines = browser_timings.report([], top=10, steps=5)
        self.assertIn("web/e2e/timings.ts", lines[0])


if __name__ == "__main__":
    unittest.main()
