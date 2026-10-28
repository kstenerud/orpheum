# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for disk_guard.py: a cache over its cap loses its oldest files first, only the pinned
# ABOUTME: browsers stay, the trim waits an hour, and a nearly full disk stops the gate saying so.
import io
import json
import os
import shutil
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
from unittest import mock

SCRIPTS = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(SCRIPTS))
import disk_guard  # noqa: E402


class Guard(unittest.TestCase):
    def setUp(self):
        self.temp = Path(tempfile.mkdtemp(prefix="disk-guard-"))
        self.addCleanup(shutil.rmtree, self.temp, ignore_errors=True)

    def file(self, relative: str, size: int, age_seconds: int) -> Path:
        path = self.temp / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(b"x" * size)
        when = 1_000_000_000 - age_seconds
        os.utime(path, (when, when))
        return path

    def test_a_cache_over_its_cap_loses_its_oldest_files_first(self):
        oldest = self.file("go/aa/oldest-a", 400, 300)
        older = self.file("go/bb/older-d", 400, 200)
        newer = self.file("go/cc/newer-a", 400, 100)
        readme = self.file("go/README", 10, 999)
        freed = disk_guard.trim(disk_guard.Cap("Go", self.temp / "go", cap_bytes=1000))
        # Over 1000, trimmed to 800: the two oldest go, the newest and Go's own README stay.
        self.assertEqual(freed, 800)
        self.assertFalse(oldest.exists())
        self.assertFalse(older.exists())
        self.assertTrue(newer.exists())
        self.assertTrue(readme.exists())

    def test_a_cache_under_its_cap_is_left_alone(self):
        kept = self.file("go/aa/entry", 100, 999)
        self.assertEqual(disk_guard.trim(disk_guard.Cap("Go", self.temp / "go", cap_bytes=1000)), 0)
        self.assertTrue(kept.exists())

    def test_only_the_pinned_playwrights_browsers_stay(self):
        lock = self.temp / "browsers.json"
        lock.write_text(json.dumps({"browsers": [
            {"name": "chromium", "revision": "1243"},
            {"name": "chromium-headless-shell", "revision": "1243"},
            {"name": "firefox", "revision": "1543"},
        ]}))
        store = self.temp / "ms-playwright"
        for name in ("chromium-1243", "chromium_headless_shell-1243", "firefox-1543", "firefox-1490",
                     "chromium-1200", ".links"):
            (store / name).mkdir(parents=True)
        gone = disk_guard.trim_browsers(store, disk_guard.pinned_browsers(lock))
        self.assertEqual(gone, ["chromium-1200", "firefox-1490"])
        self.assertEqual(sorted(entry.name for entry in store.iterdir()),
                         [".links", "chromium-1243", "chromium_headless_shell-1243", "firefox-1543"])

    def test_no_browser_goes_when_the_pin_cannot_be_read(self):
        store = self.temp / "ms-playwright"
        (store / "chromium-1200").mkdir(parents=True)
        self.assertEqual(disk_guard.trim_browsers(store, disk_guard.pinned_browsers(self.temp / "none")), [])
        self.assertTrue((store / "chromium-1200").is_dir())

    def test_tests_leftovers_two_hours_old_are_swept_and_nothing_else(self):
        now = 1_000_000_000
        day = disk_guard.LEFTOVER_AGE_SECONDS
        for name, age in (("TestServe_Something123456", day), ("go-build42", day),
                          ("playwright_chromiumdev_profile-abc", day), ("branch-gate-test-x1y2", day),
                          ("TestServe_Fresh99", day - 60), ("claude-1000", day * 9), ("notes.txt", day * 9)):
            self.file(f"tmp/{name}/inner", 1, age)
            os.utime(self.temp / "tmp" / name, (now - age, now - age))
        self.assertEqual(disk_guard.sweep_leftovers(self.temp / "tmp", now), 4)
        self.assertEqual(sorted(entry.name for entry in (self.temp / "tmp").iterdir()),
                         ["TestServe_Fresh99", "claude-1000", "notes.txt"])

    def test_the_trim_waits_an_hour_after_the_last(self):
        stamp = self.temp / "stamp"
        self.assertTrue(disk_guard.is_due(stamp, 1_000_000_000), "never trimmed")
        stamp.touch()
        os.utime(stamp, (1_000_000_000, 1_000_000_000))
        self.assertFalse(disk_guard.is_due(stamp, 1_000_000_000 + disk_guard.TRIM_EVERY_SECONDS - 1))
        self.assertTrue(disk_guard.is_due(stamp, 1_000_000_000 + disk_guard.TRIM_EVERY_SECONDS))

    def test_a_nearly_full_disk_stops_the_gate_saying_so(self):
        low = disk_guard.MIN_FREE_BYTES - 1
        with mock.patch.object(disk_guard, "free_bytes", return_value=low), \
                mock.patch.object(disk_guard, "trim_all") as trimmed, \
                mock.patch.object(disk_guard, "is_due", return_value=False), \
                mock.patch.object(disk_guard, "caps", return_value=[]):
            said = io.StringIO()
            with redirect_stderr(said):
                self.assertEqual(disk_guard.main([]), 1)
        trimmed.assert_called_once()  # it tried trimming before giving up
        self.assertIn("stopping now rather than failing part-way", said.getvalue())

    def test_room_enough_passes_without_trimming_when_not_due(self):
        with mock.patch.object(disk_guard, "free_bytes", return_value=disk_guard.MIN_FREE_BYTES), \
                mock.patch.object(disk_guard, "trim_all") as trimmed, \
                mock.patch.object(disk_guard, "is_due", return_value=False), redirect_stdout(io.StringIO()):
            self.assertEqual(disk_guard.main([]), 0)
        trimmed.assert_not_called()


class Wiring(unittest.TestCase):
    def test_make_check_check_pr_and_staging_call_it(self):
        makefile = (SCRIPTS.parent / "Makefile").read_text()
        check = makefile.split("\ncheck-verbose: ", 1)[1].split("\n\n", 1)[0]
        self.assertIn("scripts/disk_guard.py", check)
        check_pr = makefile.split("\ncheck-pr-verbose: ", 1)[1].split("\n\n", 1)[0]
        self.assertIn("scripts/disk_guard.py", check_pr)
        staging = (SCRIPTS.parent / "staging").read_text()
        self.assertIn('"disk_guard.py"', staging)
        self.assertIn('"builder", "prune"', staging)

    def test_node_and_playwright_keep_their_caches_out_of_a_runs_tmpdir(self):
        makefile = (SCRIPTS.parent / "Makefile").read_text()
        self.assertIn("export NODE_COMPILE_CACHE", makefile)
        self.assertIn("export PWTEST_CACHE_DIR", makefile)


if __name__ == "__main__":
    unittest.main()
