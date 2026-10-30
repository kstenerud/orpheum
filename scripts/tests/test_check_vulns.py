# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for check_vulns.py: govulncheck's reachable findings and npm audit's high advisories
# ABOUTME: are reported, and nothing, a finding or a tool that cannot run, ever fails the run.
import json
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_vulns  # noqa: E402

REACHED = {"finding": {"osv": "GO-2026-6163", "fixed_version": "v3.1.5", "trace": [
    {"module": "github.com/pion/stun/v3", "version": "v3.0.1", "package": "github.com/pion/stun/v3",
     "function": "GetFromAs"}, {"module": "github.com/kstenerud/orpheum", "function": "Start"}]}}
# Imported, but no function of ours reaches it: govulncheck's trace then names no function.
IMPORTED_ONLY = {"finding": {"osv": "GO-2026-0001", "fixed_version": "v0.62.0", "trace": [
    {"module": "golang.org/x/net", "version": "v0.61.0", "package": "golang.org/x/net/html"}]}}
OSV = {"osv": {"id": "GO-2026-6163", "summary": "Panic on malformed XOR-MAPPED-ADDRESS"}}


class GoFindings(unittest.TestCase):
    def test_only_what_our_code_reaches_is_reported_with_its_fix(self):
        stream = "\n".join(json.dumps(m, indent=2) for m in (OSV, REACHED, IMPORTED_ONLY))
        self.assertEqual(check_vulns.go_findings(stream), [
            "GO-2026-6163: github.com/pion/stun/v3 v3.0.1, fixed in v3.1.5 — Panic on malformed XOR-MAPPED-ADDRESS"])

    def test_nothing_reached_is_no_finding(self):
        self.assertEqual(check_vulns.go_findings(json.dumps(IMPORTED_ONLY)), [])


class NpmFindings(unittest.TestCase):
    def test_high_and_critical_are_reported_and_lower_left_out(self):
        report = json.dumps({"vulnerabilities": {
            "vite": {"severity": "high", "fixAvailable": True},
            "zod": {"severity": "critical", "fixAvailable": False},
            "debug": {"severity": "moderate", "fixAvailable": True}}})
        self.assertEqual(check_vulns.npm_findings(report),
                         ["vite: high, a fix is available", "zod: critical, no fix yet"])


class NeverFails(unittest.TestCase):
    def test_findings_and_tools_that_cannot_run_both_exit_zero(self):
        found = mock.Mock(returncode=3, stdout=json.dumps(REACHED))
        with mock.patch.object(check_vulns, "run", return_value=found), mock.patch("builtins.print"):
            self.assertEqual(check_vulns.main(), 0)
        with mock.patch.object(check_vulns, "run", return_value=None), mock.patch("builtins.print") as said:
            self.assertEqual(check_vulns.main(), 0)
        printed = " ".join(str(call.args[0]) for call in said.call_args_list)
        self.assertIn("Go: could not check", printed)
        self.assertIn("Web: could not check", printed)


if __name__ == "__main__":
    unittest.main()
