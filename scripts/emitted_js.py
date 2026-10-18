# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Whether two versions of a web source compile to the same JavaScript: a change of types or
# ABOUTME: comments alone, which no browser can see, so the browser tier is not run for it.
"""Whether a change to a TypeScript source leaves the JavaScript a browser runs the same.

The web code compiles each file alone (`isolatedModules`, `verbatimModuleSyntax`, tsconfig.base.json):
a file's JavaScript depends on its own text and nothing else, so two versions that compile to the
same JavaScript build the same client. A change of types alone (the SDK's generated `gen/api.ts`) is
then the type check's to notice, never a browser's: its revert ran every browser test, 507 s of
revert-red, and check-pr's own browser step whole, since no chunk the browser map records holds a file
of types (2026-10-18).

TypeScript's own `transpileModule` compiles each version, comments removed. It keeps some of a
file's line breaks (an object literal's), so a reformat may read as different: a browser run, as before. Every doubt answers
"not the same", so the browser tier runs as before: a version that does not parse, a missing node or
typescript, a file that is not TypeScript.
"""

from __future__ import annotations

import json
import re
import subprocess
from pathlib import Path

TS_FILE = re.compile(r"\.tsx?$")
# The web/ beside these scripts, whose node_modules holds the pinned typescript.
WEB = Path(__file__).resolve().parents[1] / "web"

# Reads {"fileName", "before", "after"} from stdin; prints "same" or "different".
_COMPARE = r"""
const ts = require("typescript");
const input = JSON.parse(require("node:fs").readFileSync(0, "utf8"));
const options = {
  target: ts.ScriptTarget.ES2023, module: ts.ModuleKind.ESNext, jsx: ts.JsxEmit.ReactJSX,
  verbatimModuleSyntax: true, isolatedModules: true, removeComments: true,
};
const emit = (text) => {
  const out = ts.transpileModule(text, { compilerOptions: options, fileName: input.fileName, reportDiagnostics: true });
  return out.diagnostics && out.diagnostics.length > 0 ? null : out.outputText;
};
const before = emit(input.before), after = emit(input.after);
process.stdout.write(before !== null && before === after ? "same" : "different");
"""


def emits_the_same(path: str, before: str, after: str) -> bool:
    """Whether `before` and `after`, two versions of the web source `path`, compile to the same
    JavaScript."""
    if not TS_FILE.search(path):
        return False
    try:
        done = subprocess.run(["node", "-e", _COMPARE], cwd=WEB, capture_output=True, text=True, check=False,
                              input=json.dumps({"fileName": Path(path).name, "before": before, "after": after}),
                              timeout=60)
    except (OSError, subprocess.TimeoutExpired):
        return False
    return done.returncode == 0 and done.stdout == "same"


def emits_the_same_between(tree: str, base: str, head: str, path: str) -> bool:
    """Whether `path` compiles to the same JavaScript at `base` as at `head`, in the git tree `tree`.
    A file new or gone on either side is not the same."""
    versions = [subprocess.run(["git", "show", f"{rev}:{path}"], cwd=tree, capture_output=True, text=True,
                               check=False) for rev in (base, head)]
    if any(v.returncode != 0 for v in versions):
        return False
    return emits_the_same(path, versions[0].stdout, versions[1].stdout)
