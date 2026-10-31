# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for the token ledger: a call counted once however many lines Claude Code writes for it,
# ABOUTME: the weights applied, and agents told apart from the lead.
import contextlib
import io
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import token_usage  # noqa: E402


def call(message_id: str, **usage: int) -> str:
    return json.dumps({"type": "assistant", "message": {"id": message_id, "model": "m", "usage": usage,
                                                         "content": [{"type": "tool_use"}]}})


class TokenUsage(unittest.TestCase):
    def test_a_call_counts_once_and_agents_are_told_apart(self):
        directory = Path(tempfile.mkdtemp())
        (directory / "lead.jsonl").write_text("\n".join([
            json.dumps({"type": "user", "message": {"content": "look into the token use"}}),
            call("m1", cache_read_input_tokens=100_000, output_tokens=100),
            call("m1", cache_read_input_tokens=100_000, output_tokens=100),  # the same call, streamed twice
        ]) + "\n")
        (directory / "subagents").mkdir()
        (directory / "subagents" / "agent-a.jsonl").write_text(
            call("m2", cache_creation_input_tokens=40_000, input_tokens=5) + "\n")
        lead = token_usage.read(directory / "lead.jsonl")
        self.assertEqual((lead.calls, lead.tools, lead.peak_context, lead.is_agent), (1, 2, 100_000, False))
        self.assertEqual(lead.weighted, 0.1 * 100_000 + 5 * 100)
        self.assertEqual(lead.task, "look into the token use")
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            self.assertEqual(token_usage.main(["--dir", str(directory)]), 0)
        self.assertIn("2 sessions, 2 model calls", out.getvalue())
        self.assertIn("agents 83%", out.getvalue())  # 50,005 of 60,505


if __name__ == "__main__":
    unittest.main()
