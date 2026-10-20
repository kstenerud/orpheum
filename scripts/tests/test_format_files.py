# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for format_files.py: the gate's formatters run on what was edited, or on what is staged
# ABOUTME: and staged again, never on generated files nor on a file whose unstaged changes it would sweep in.
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import format_files  # noqa: E402
from gitrepo import Repo  # noqa: E402

# Formatters that mark each file they are given, so a test sees which ones ran on what.
FAKE_GOLANGCI = """#!/bin/sh
[ "$1" = fmt ] || exit 1
shift
for file in "$@"; do echo "// formatted" >> "$file"; done
"""
FAKE_PRETTIER = """#!/bin/sh
for file in "$@"; do
    case $file in --*) ;; *) echo "/* formatted */" >> "$file" ;; esac
done
"""


def with_fake_formatters(repo: Repo) -> mock._patch:
    """golangci-lint on PATH, and the web's Prettier in its node_modules, both fakes."""
    bin_dir = Path(tempfile.mkdtemp(prefix="format-files-bin-"))
    (bin_dir / "golangci-lint").write_text(FAKE_GOLANGCI)
    (bin_dir / "golangci-lint").chmod(0o755)
    prettier = repo.root / "web" / "node_modules" / ".bin" / "prettier"
    prettier.parent.mkdir(parents=True, exist_ok=True)
    prettier.write_text(FAKE_PRETTIER)
    prettier.chmod(0o755)
    return mock.patch.dict(os.environ, {"PATH": f"{bin_dir}:{os.environ['PATH']}"})


def staged_text(repo: Repo, rel: str) -> str:
    return subprocess.run(["git", "show", f":{rel}"], cwd=repo.root, capture_output=True, text=True,
                          check=True).stdout


class FormatFilesTest(unittest.TestCase):
    def test_formats_the_go_and_web_files_it_is_given(self):
        repo = Repo({"cmd/a.go": "package a\n", "web/apps/web/src/b.ts": "export {};\n", "README.md": "x\n"})
        with with_fake_formatters(repo):
            format_files.format_files(["cmd/a.go", "web/apps/web/src/b.ts", "README.md"], repo.root)
        self.assertIn("// formatted", (repo.root / "cmd/a.go").read_text())
        self.assertIn("/* formatted */", (repo.root / "web/apps/web/src/b.ts").read_text())
        self.assertEqual((repo.root / "README.md").read_text(), "x\n")

    def test_never_formats_generated_files(self):
        # Their source makes them; formatting one by hand is editing it (AGENTS rule 7).
        repo = Repo({"internal/store/sqlcgen/q.go": "package sqlcgen\n",
                     "web/packages/sdk/src/gen/api.ts": "export {};\n"})
        with with_fake_formatters(repo):
            format_files.format_files(["internal/store/sqlcgen/q.go", "web/packages/sdk/src/gen/api.ts"], repo.root)
        self.assertEqual((repo.root / "internal/store/sqlcgen/q.go").read_text(), "package sqlcgen\n")
        self.assertEqual((repo.root / "web/packages/sdk/src/gen/api.ts").read_text(), "export {};\n")

    def test_formats_what_is_staged_and_stages_it_again(self):
        repo = Repo({"cmd/a.go": "package a\n", "web/apps/web/src/b.ts": "export {};\n"})
        subprocess.run(["git", "add", "-A"], cwd=repo.root, check=True)
        with with_fake_formatters(repo):
            left = format_files.format_staged(repo.root)
        self.assertEqual(left, [])
        self.assertIn("// formatted", staged_text(repo, "cmd/a.go"))
        self.assertIn("/* formatted */", staged_text(repo, "web/apps/web/src/b.ts"))

    def test_leaves_a_staged_file_with_unstaged_changes_as_staged(self):
        # Formatting it and staging it again would commit what its author had not staged.
        repo = Repo({"cmd/a.go": "package a\n"})
        subprocess.run(["git", "add", "-A"], cwd=repo.root, check=True)
        repo.write({"cmd/a.go": "package a\n\nvar notStaged = 1\n"})
        with with_fake_formatters(repo):
            left = format_files.format_staged(repo.root)
        self.assertEqual(left, ["cmd/a.go"])
        self.assertEqual(staged_text(repo, "cmd/a.go"), "package a\n")
        self.assertNotIn("// formatted", (repo.root / "cmd/a.go").read_text())


if __name__ == "__main__":
    unittest.main()
