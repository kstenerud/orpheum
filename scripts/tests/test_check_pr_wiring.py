# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests of check-pr's wiring in the Makefile: its tiers' order, revert-red after the timing
# ABOUTME: tiers, a failed quick tier stopping it before the long ones (D424), and one way to its base.
import re
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent.parent

def expand_variables(makefile: str, text: str) -> str:
    """`text` with the Makefile's simple variables expanded, `$(NAME)` and `$(NAME:%=prefix%)`."""
    values = dict(re.findall(r"(?m)^([A-Z_]+) := (.*)$", makefile))

    def expand(match: re.Match[str]) -> str:
        words = values.get(match.group(1))
        if words is None:
            return match.group(0)
        prefix = match.group(2) or ""
        return " ".join(prefix + word for word in words.split())
    return re.sub(r"\$\(([A-Z_]+)(?::%=([^%)]*)%)?\)", expand, text)


def revert_red_waits_for_the_tiers(makefile: str) -> bool:
    """Whether check-pr runs check-revert-red in a make of its own after the parallel one. The
    steps are check-pr-verbose's; check-pr runs it quietly (scripts/quiet_gate.py)."""
    recipe = re.search(r"^check-pr-verbose:.*\n((?:\t.*\n)+)", makefile, re.M)
    if recipe is None:
        return False
    body = expand_variables(makefile, recipe.group(1).replace("\\\n", " "))
    parallel = body.find("-j")
    tiers = body.find("test-integration")
    revert = body.find("check-revert-red")
    return 0 <= parallel < tiers < revert and "$(MAKE)" in body[tiers:revert]


def run_the_tiers(makefile: str, failing: str) -> tuple[list[str], str]:
    """check-pr-verbose's tiers run by bash with a make that records each step it is asked for and
    fails the one named `failing`: the steps asked for, in order, and the run's last line."""
    recipe = re.search(r"^check-pr-verbose:.*\n((?:\t.*\n)+)", makefile, re.M)
    assert recipe is not None, "no check-pr-verbose recipe"
    body = recipe.group(1).replace("\\\n", " ")
    block = re.search(r"\{ failed=0;.*?exit 1; \};", body, re.S)
    assert block is not None, "no tier block in check-pr-verbose"
    script = expand_variables(makefile, block.group(0)).replace("$$", "$")
    script = script.replace("$(MAKE)", "fake_make").replace("$(CHECK_JOBS)", "4").replace("python3 ", "true ")
    with tempfile.TemporaryDirectory() as tmp:
        asked = Path(tmp) / "asked"
        stub = ('fake_make() { for a in "$@"; do case "$a" in pr-step-*) echo "${a#pr-step-}" >> "$ASKED";'
                ' [ "${a#pr-step-}" = "$FAILING" ] && bad=1;; esac; done; [ -z "${bad:-}" ]; }\n')
        result = subprocess.run(["bash", "-c", stub + "base=b; " + script],
                                env={"ASKED": str(asked), "FAILING": failing, "PATH": "/usr/bin:/bin"},
                                capture_output=True, text=True, check=False)
        steps = asked.read_text().split() if asked.exists() else []
    lines = result.stdout.strip().splitlines()
    return steps, lines[-1] if lines else ""


class CheckPrWiring(unittest.TestCase):
    MAKEFILE = (REPO / "Makefile").read_text()

    def test_revert_red_runs_after_the_timing_tiers_not_beside_them(self):
        self.assertTrue(revert_red_waits_for_the_tiers(self.MAKEFILE))
        beside = ("check-pr-verbose:\n\t@$(MAKE) -j4 test-integration test-browser check-revert-red lint-commits\n")
        self.assertFalse(revert_red_waits_for_the_tiers(beside))

    def test_a_failed_quick_tier_stops_check_pr_before_the_long_ones_and_a_long_one_does_not(self):
        # D424: a lint error found in a minute does not wait behind half an hour of revert-red.
        steps, last = run_the_tiers(self.MAKEFILE, "web-lint-full")
        self.assertIn("web-lint-full", steps)
        for long_step in ("check-revert-red-code", "test-acceptance-home", "test-browser", "check-revert-red-browser"):
            self.assertNotIn(long_step, steps)
        self.assertIn("a quick tier failed", last)
        # A failed long tier still lets every later one run (the owner, 2026-10-17).
        steps, last = run_the_tiers(self.MAKEFILE, "check-revert-red-code")
        self.assertEqual(steps[-1], "check-revert-red-browser")
        self.assertIn("test-browser", steps)
        self.assertIn("every tier still ran", last)

    def test_every_branch_gate_resolves_its_base_the_one_way(self):
        self.assertRegex(self.MAKEFILE, r"(?m)^check-pr-locked:.*\n\t.*quiet_gate\.py .*check-pr-verbose")
        for target in ("check-pr-verbose", "check-pr-failed", "check-revert-red", "check-revert-red-failed",
                       "check-test-weakening", "check-decision-status", "lint-commits"):
            recipe = re.search(rf"^{target}:.*\n\t(.*)", self.MAKEFILE, re.M)
            self.assertIsNotNone(recipe, target)
            self.assertIn("$(RESOLVE_BASE)", recipe.group(1), target)
        self.assertRegex(self.MAKEFILE, r"(?m)^RESOLVE_BASE = .*scripts/branch_base\.py")
        self.assertRegex(self.MAKEFILE, r"(?m)^mark-applied:.*\n\t.*branch_base\.py --mark")


if __name__ == "__main__":
    unittest.main()
