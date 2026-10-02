# SPDX-License-Identifier: GPL-3.0-or-later
"""The dev loop runs the core and the node as two processes, as deployment does (D235 §4)."""

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import dev  # noqa: E402


class RoleCommands(unittest.TestCase):
    def test_the_loop_runs_the_core_and_the_node_each_on_the_dev_config(self):
        commands = dev.role_commands(Path("/b/orpheum"), Path("/c/orpheum.toml"))
        self.assertEqual(commands, {
            "core": ["/b/orpheum", "core", "--config", "/c/orpheum.toml"],
            "node": ["/b/orpheum", "node", "--config", "/c/orpheum.toml"],
        })


if __name__ == "__main__":
    unittest.main()
