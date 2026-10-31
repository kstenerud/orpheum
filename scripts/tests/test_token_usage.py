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

    def test_since_counts_the_calls_made_since_not_every_call_of_a_file_written_since(self):
        directory = Path(tempfile.mkdtemp())
        stamped = [json.loads(call(f"m{n}", input_tokens=1000)) | {"timestamp": at}
                   for n, at in enumerate(["2026-10-30T23:00:00.000Z", "2026-10-31T09:00:00.000Z"])]
        (directory / "lead.jsonl").write_text("\n".join(json.dumps(e) for e in stamped) + "\n")
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            self.assertEqual(token_usage.main(["--dir", str(directory), "--since", "2026-10-31T00:00+00:00"]), 0)
        self.assertIn("1 sessions, 1 model calls", out.getvalue())
        self.assertEqual(token_usage.read(directory / "lead.jsonl").calls, 2)

    def test_cost_is_at_each_models_price(self):
        directory = Path(tempfile.mkdtemp())
        def priced(model: str, **usage: int) -> str:
            entry = json.loads(call("x" + model, **usage))
            entry["message"]["model"] = model
            return json.dumps(entry)
        (directory / "lead.jsonl").write_text(priced("claude-opus-5-5", cache_read_input_tokens=1_000_000,
                                                     output_tokens=1_000_000) + "\n")
        (directory / "subagents").mkdir()
        (directory / "subagents" / "a.jsonl").write_text(priced("claude-sonnet-5-5", input_tokens=1_000_000,
                                                                cache_creation_input_tokens=1_000_000) + "\n")
        self.assertAlmostEqual(token_usage.read(directory / "lead.jsonl").dollars, 0.20 + 20.0)
        self.assertAlmostEqual(token_usage.read(directory / "subagents" / "a.jsonl").dollars, 2.0 + 4.0)
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            token_usage.main(["--dir", str(directory)])
        self.assertIn("$26.20 at API prices", out.getvalue())
        self.assertIn("agents 23%", out.getvalue())
        self.assertNotIn("no price", out.getvalue())


if __name__ == "__main__":
    unittest.main()
