# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for orpheum-tunnel, the core's end of the WireGuard tunnel (deploy/staging/tunnel.sh):
# ABOUTME: run against fake ip, wg and sleep, it drops packets on wg0 whose source is home or private.
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SCRIPT = REPO / "deploy" / "staging" / "tunnel.sh"

# An ip that logs each call and has no rule at any preference until one is added.
FAKE_IP = """#!/bin/sh
echo "ip $*" >> "$FAKE_LOG"
case "$*" in
*"rule show pref"*) grep -F -- "ip $1 rule add pref $5 " "$FAKE_LOG" | head -1 ;;
*"rule del pref"*) exit 1 ;;
esac
exit 0
"""
FAKE_WG = """#!/bin/sh
[ "$1" = show ] && printf 'peer\\t203.0.113.1:51820\\n'
exit 0
"""
# The script waits on a sleep forever; this one returns at once, so the script ends.
FAKE_SLEEP = "#!/bin/sh\nexit 0\n"

FORGED_IPV4 = ["0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
               "172.16.0.0/12", "192.168.0.0/16"]
FORGED_IPV6 = ["::1/128", "fc00::/7", "fe80::/10"]


def run_tunnel() -> list[str]:
    """The ip calls one start of the tunnel makes, in order."""
    with tempfile.TemporaryDirectory() as scratch:
        bin_dir = Path(scratch) / "bin"
        bin_dir.mkdir()
        for name, text in (("ip", FAKE_IP), ("wg", FAKE_WG), ("sleep", FAKE_SLEEP)):
            (bin_dir / name).write_text(text)
            (bin_dir / name).chmod(0o755)
        log = Path(scratch) / "log"
        log.touch()
        env = {"PATH": f"{bin_dir}:/usr/bin:/bin", "FAKE_LOG": str(log),
               "ORPHEUM_TUNNEL_IPV4": "10.77.0.3", "ORPHEUM_TUNNEL_IPV6": "fd77::3"}
        subprocess.run(["sh", str(SCRIPT)], env=env, check=True, capture_output=True, text=True, timeout=30)
        return log.read_text().splitlines()


class TunnelScriptTest(unittest.TestCase):
    def test_drops_home_and_private_sources_arriving_on_wg0(self):
        # The VPS keeps each packet's source, so one from home or a private network through the
        # tunnel is forged to pass as the house (the Plex-chain review's L2).
        calls = run_tunnel()
        for family, sources in (("-4", FORGED_IPV4), ("-6", FORGED_IPV6)):
            for source in sources:
                self.assertIn(f"ip {family} rule add pref 20 iif wg0 from {source} blackhole", calls)

    def test_lets_the_tunnels_own_peers_through(self):
        calls = run_tunnel()
        self.assertIn("ip -4 rule add pref 10 iif wg0 from 10.77.0.0/24 lookup local", calls)
        self.assertIn("ip -6 rule add pref 10 iif wg0 from fd77::/64 lookup local", calls)

    def test_moves_the_local_table_behind_the_drops_before_taking_it_from_the_head(self):
        # The kernel looks in the local table first for a packet to this machine, so a drop after it
        # would never be reached; and with no local rule at all, nothing is delivered.
        calls = run_tunnel()
        for family in ("-4", "-6"):
            moved = calls.index(f"ip {family} rule add pref 100 lookup local")
            taken = calls.index(f"ip {family} rule del pref 0 lookup local")
            first_drop = next(i for i, call in enumerate(calls) if call.startswith(f"ip {family} rule add pref 20 "))
            self.assertLess(moved, taken)
            self.assertLess(taken, first_drop)


if __name__ == "__main__":
    unittest.main()
