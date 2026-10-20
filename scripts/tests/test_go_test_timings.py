# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for the Go timings filter: it prints go test's own non-verbose text from -json, a
# ABOUTME: failure nested as go test nests it, and keeps each test's and package's time.
import datetime
import io
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import go_test_timings  # noqa: E402


def events(*rows: dict) -> io.StringIO:
    return io.StringIO("".join(json.dumps(row) + "\n" for row in rows))


def out(test: str, text: str) -> dict:
    return {"Action": "output", "Package": "probe", "Test": test, "Output": text}


# A run of `go test -json` with one passing test and a failing one whose subtest failed, as test2json
# writes it (captured from Go 1.26, 2026-10-20).
RUN = events(
    out("TestPasses", "=== RUN   TestPasses\n"),
    out("TestPasses", "    p_test.go:5: hidden when passing\n"),
    out("TestPasses", "--- PASS: TestPasses (0.00s)\n"),
    {"Action": "pass", "Package": "probe", "Test": "TestPasses", "Elapsed": 0.01},
    out("TestFails", "=== RUN   TestFails\n"),
    out("TestFails", "    p_test.go:8: shown when failing\n"),
    out("TestFails/sub", "=== RUN   TestFails/sub\n"),
    out("TestFails/sub", "    p_test.go:9: the sub broke\n"),
    out("TestFails/sub", "--- FAIL: TestFails/sub (0.00s)\n"),
    {"Action": "fail", "Package": "probe", "Test": "TestFails/sub", "Elapsed": 0},
    out("TestFails/fine", "=== RUN   TestFails/fine\n"),
    out("TestFails/fine", "--- PASS: TestFails/fine (0.00s)\n"),
    {"Action": "pass", "Package": "probe", "Test": "TestFails/fine", "Elapsed": 0},
    out("TestFails", "--- FAIL: TestFails (0.00s)\n"),
    {"Action": "fail", "Package": "probe", "Test": "TestFails", "Elapsed": 1.5},
    {"Action": "output", "Package": "probe", "Output": "FAIL\n"},
    {"Action": "output", "Package": "probe", "Output": "FAIL\tprobe\t1.512s\n"},
    {"Action": "fail", "Package": "probe", "Elapsed": 1.512},
)

# go test's own text for the same run, without -json.
PLAIN = """--- FAIL: TestFails (0.00s)
    p_test.go:8: shown when failing
    --- FAIL: TestFails/sub (0.00s)
        p_test.go:9: the sub broke
FAIL
FAIL\tprobe\t1.512s
"""


class Filter(unittest.TestCase):
    def test_it_prints_go_tests_own_text_and_keeps_each_tests_and_packages_time(self):
        echo = io.StringIO()
        started = datetime.datetime(2026, 10, 20, 21, 0, tzinfo=datetime.timezone.utc)
        record = go_test_timings.record_of(RUN, echo, "go", started)
        self.assertEqual(echo.getvalue(), PLAIN)
        self.assertEqual(record["status"], "failed")
        self.assertEqual([(t["test"], t["status"], t["ms"]) for t in record["tests"]],
                         [("TestPasses", "pass", 10), ("TestFails/sub", "fail", 0),
                          ("TestFails/fine", "pass", 0), ("TestFails", "fail", 1500)])
        self.assertEqual(record["files"], [{"file": "probe", "project": "go", "ms": 1512}])

    def test_a_test_still_running_when_its_package_fails_is_printed(self):
        echo = io.StringIO()
        started = datetime.datetime(2026, 10, 20, 21, 0, tzinfo=datetime.timezone.utc)
        go_test_timings.record_of(events(
            out("TestHangs", "=== RUN   TestHangs\n"),
            out("TestHangs", "panic: test timed out after 30m0s\n"),
            {"Action": "output", "Package": "probe", "Output": "FAIL\tprobe\t1800.0s\n"},
            {"Action": "fail", "Package": "probe", "Elapsed": 1800},
        ), echo, "go", started)
        self.assertEqual(echo.getvalue(), "panic: test timed out after 30m0s\nFAIL\tprobe\t1800.0s\n")

    def test_a_line_that_is_no_event_is_printed_as_it_is_and_only_the_newest_passes_are_kept(self):
        echo = io.StringIO()
        started = datetime.datetime(2026, 10, 20, 21, 0, tzinfo=datetime.timezone.utc)
        go_test_timings.record_of(io.StringIO("# probe\nvet: p.go:3: unreachable code\n"), echo, "go", started)
        self.assertEqual(echo.getvalue(), "# probe\nvet: p.go:3: unreachable code\n")
        root = Path(tempfile.mkdtemp())
        for minute in range(go_test_timings.KEPT_PASSES + 2):
            go_test_timings.write({"suite": "go", "pass": "run", "startedAt": f"2026-10-20T21:{minute:02d}:00Z"},
                                  root)
        kept = sorted(path.name for path in (root / "go").glob("*.json"))
        self.assertEqual(len(kept), go_test_timings.KEPT_PASSES)
        self.assertEqual(kept[0], "2026-10-20T21-02-00Z-run.json")


if __name__ == "__main__":
    unittest.main()
