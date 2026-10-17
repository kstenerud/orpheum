# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for browser_map.py's script reach: a source file's revert first runs only the browser
# ABOUTME: tests that loaded a chunk holding it, and anything the map cannot narrow runs every test.
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import browser_map  # noqa: E402
import check_revert_red as rr  # noqa: E402

LOADER = "web/apps/web/src/loader-main.tsx"
SHARED = "web/packages/sdk/src/discovery.ts"
PLAYER = "web/apps/web/src/player-page.tsx"
WORKER = "web/apps/web/src/sw.ts"

CLIENT_BUILD = {"assets/index-a.js": [SHARED], "assets/loader-main-b.js": [LOADER],
                "assets/client-main-c.js": [PLAYER]}
SW_BUILD = {"sw.js": [WORKER, SHARED]}

THROUGH_DISCOVERY = "chromium through-discovery.spec.ts signs in"
PLAYS = "firefox play-timing.spec.ts plays"
UNVOUCHED = "chromium shell.spec.ts reads the sheets"


class TestsLoading(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        (self.root / browser_map.COMPLETE).write_text("an older commit\n")
        (self.root / "modules-client.json").write_text(json.dumps(CLIENT_BUILD))
        (self.root / browser_map.SERVICE_WORKER_MODULES).write_text(json.dumps(SW_BUILD))
        self.record(THROUGH_DISCOVERY, ["assets/index-a.js", "assets/loader-main-b.js", "assets/client-main-c.js"])
        self.record(PLAYS, ["assets/index-a.js", "assets/client-main-c.js"])
        self.record("chromium shell.spec.ts the logo", ["assets/index-a.js"])

    def record(self, test: str, scripts: list[str] | None) -> None:
        directory = self.root / test.split()[0]
        directory.mkdir(exist_ok=True)
        name = f"{len(list(directory.iterdir()))}.json"
        (directory / name).write_text(json.dumps({"test": test, "tokens": [], "scripts": scripts}))

    def test_a_file_reaches_only_the_tests_that_loaded_a_chunk_holding_it(self):
        self.assertEqual(browser_map.tests_loading(LOADER, self.root), [THROUGH_DISCOVERY])
        self.assertCountEqual(browser_map.tests_loading(PLAYER, self.root) or [], [THROUGH_DISCOVERY, PLAYS])

    def test_a_test_whose_scripts_were_not_vouched_for_is_reached_by_every_file(self):
        self.record(UNVOUCHED, None)
        self.assertCountEqual(browser_map.tests_loading(LOADER, self.root) or [], [THROUGH_DISCOVERY, UNVOUCHED])

    def test_what_cannot_be_narrowed_runs_every_test(self):
        self.assertIsNone(browser_map.tests_loading(SHARED, self.root), "every test loaded its chunk")
        self.assertIsNone(browser_map.tests_loading(WORKER, self.root), "the service worker holds it")
        self.assertIsNone(browser_map.tests_loading("web/apps/web/src/nowhere.ts", self.root), "no chunk holds it")
        (self.root / browser_map.COMPLETE).unlink()
        self.assertIsNone(browser_map.tests_loading(LOADER, self.root), "the last run did not pass")

    def test_a_map_recorded_before_scripts_were_narrows_nothing(self):
        for record in self.root.glob("*/*.json"):
            record.write_text(json.dumps({"test": "old", "tokens": []}))
        self.assertIsNone(browser_map.tests_loading(LOADER, self.root))


class BrowserAttempts(unittest.TestCase):
    def setUp(self):
        rr._installed_ffmpeg.cache_clear()
        self.original = rr._installed_ffmpeg, browser_map.tests_loading
        rr._installed_ffmpeg = lambda: "/ffmpeg"  # type: ignore[assignment]

    def tearDown(self):
        rr._installed_ffmpeg, browser_map.tests_loading = self.original

    def test_the_tests_that_load_it_run_first_and_every_test_after_before_it_is_green(self):
        browser_map.tests_loading = lambda path: [THROUGH_DISCOVERY]  # type: ignore[assignment]
        narrowed, whole = rr.browser_tier(LOADER)
        self.assertEqual((narrowed.env or {}).get(browser_map.ONLY_THESE_TESTS), json.dumps([THROUGH_DISCOVERY]))
        self.assertIsNone(whole.env)
        self.assertEqual(narrowed.command, whole.command)

    def test_a_file_the_map_cannot_narrow_runs_every_test_once(self):
        for reach in (None, []):
            browser_map.tests_loading = lambda path, reach=reach: reach  # type: ignore[assignment]
            (only,) = rr.browser_tier(LOADER)
            self.assertIsNone(only.env)


if __name__ == "__main__":
    unittest.main()
