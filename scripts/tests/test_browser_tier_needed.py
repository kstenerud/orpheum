# SPDX-License-Identifier: AGPL-3.0-or-later
"""browser_tier_needed: which changes the browser tier, and M4's acceptance under home, check."""

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import browser_tier_needed as needed  # noqa: E402


class Acceptance(unittest.TestCase):
    def test_runs_for_what_keeps_two_browsers_in_step_and_talking_and_for_nothing_else(self):
        for path in ("internal/core/together/hub.go", "internal/core/rtcapi/rtcapi.go",
                     "web/packages/player/src/abr.ts", "web/packages/sdk/src/together-voice.ts",
                     "web/packages/sdk/src/rtc-transport.ts", "web/apps/web/src/together-join.tsx",
                     "web/apps/web/src/film-player.tsx", "web/e2e/acceptance.spec.ts"):
            self.assertTrue(needed.is_in_the_acceptance(path), path)
        for path in ("internal/core/identify/match.go", "web/apps/web/src/title.tsx", "docs/x.md",
                     "web/packages/sdk/src/together-voice.test.ts", "internal/core/together/hub_test.go",
                     "scripts/browser_tier_needed.py"):
            self.assertFalse(needed.is_in_the_acceptance(path), path)


if __name__ == "__main__":
    unittest.main()
