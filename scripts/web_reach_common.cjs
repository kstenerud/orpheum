// SPDX-License-Identifier: AGPL-3.0-or-later
// ABOUTME: What the web reach scripts share: the input they read, the web sources' texts at the base
// ABOUTME: and now, resolving an import to a web source, parsing, and the names a source exports.
"use strict";
// web_lint_reach.cjs follows a change to the lint of other files, web_test_reach.cjs to the tests that
// can see it; both read {typescript, base, changed, universe} from stdin, run in web/.

const path = require("node:path");
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");

/** The input on stdin, and the helpers bound to it. */
function setUp() {
  const input = JSON.parse(fs.readFileSync(0, "utf8"));
  const ts = require(input.typescript);
  const WEB = process.cwd();
  const universe = new Set(input.universe);

  const headText = (rel) => fs.readFileSync(path.join(WEB, rel), "utf8");

  /** The base's text of each of rels that it has. */
  function baseTexts(rels) {
    if (rels.length === 0) return new Map();
    const names = rels.map((rel) => `${input.base}:./${rel}`).join("\n") + "\n";
    const out = execFileSync("git", ["cat-file", "--batch"], { input: names, maxBuffer: 1 << 30 });
    const texts = new Map();
    let at = 0;
    for (const rel of rels) {
      const end = out.indexOf(10, at);
      const header = out.subarray(at, end).toString();
      at = end + 1;
      if (header.endsWith(" missing")) continue;
      const size = Number(header.split(" ")[2]);
      texts.set(rel, out.subarray(at, at + size).toString("utf8"));
      at += size + 1;
    }
    return texts;
  }

  const options = {
    target: ts.ScriptTarget.ES2023, module: ts.ModuleKind.ESNext, moduleResolution: ts.ModuleResolutionKind.Bundler,
    jsx: ts.JsxEmit.ReactJSX, allowImportingTsExtensions: true, verbatimModuleSyntax: true, isolatedModules: true,
    isolatedDeclarations: true, declaration: true, emitDeclarationOnly: true, resolveJsonModule: true, strict: true,
  };
  const resolveHost = {
    fileExists: ts.sys.fileExists, readFile: ts.sys.readFile, realpath: ts.sys.realpath,
    directoryExists: ts.sys.directoryExists, getCurrentDirectory: () => WEB,
  };
  const resolved = new Map();
  /** The web source spec names from rel, or null for one outside them (a package's, a stylesheet). */
  function resolve(spec, rel) {
    const key = `${path.dirname(rel)}\0${spec}`;
    if (!resolved.has(key)) {
      const found = ts.resolveModuleName(spec, path.join(WEB, rel), options, resolveHost).resolvedModule;
      const target = found ? path.relative(WEB, ts.sys.realpath(found.resolvedFileName)) : null;
      resolved.set(key, target !== null && universe.has(target) ? target : null);
    }
    return resolved.get(key);
  }

  const scriptKind = (rel) => (rel.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const parse = (rel, text) => ts.createSourceFile(rel, text, ts.ScriptTarget.ES2023, true, scriptKind(rel));
  const stringOf = (node) => (node && ts.isStringLiteralLike(node) ? node.text : null);

  /** The names rel's own statements export (its `export *` targets' aside), and those targets. */
  function sourceExports(rel) {
    const names = new Set();
    const stars = [];
    for (const statement of parse(rel, headText(rel)).statements) {
      const f = ts.canHaveModifiers(statement) ? ts.getCombinedModifierFlags(statement) : 0;
      if (ts.isExportDeclaration(statement) && !statement.exportClause) {
        const target = statement.moduleSpecifier ? resolve(stringOf(statement.moduleSpecifier), rel) : null;
        if (target !== null) stars.push(target);
      } else if (ts.isExportDeclaration(statement) && ts.isNamedExports(statement.exportClause)) {
        for (const s of statement.exportClause.elements) names.add(s.name.text);
      } else if (ts.isExportDeclaration(statement)) {
        names.add(statement.exportClause.name.text);
      } else if (ts.isExportAssignment(statement) || (f & ts.ModifierFlags.Default)) {
        names.add("default");
      } else if ((f & ts.ModifierFlags.Export) && ts.isVariableStatement(statement)) {
        for (const d of statement.declarationList.declarations) {
          if (ts.isIdentifier(d.name)) names.add(d.name.text);
          else throw new Error(`${rel} exports a destructured name, which this does not read`);
        }
      } else if ((f & ts.ModifierFlags.Export) && statement.name) {
        names.add(statement.name.text);
      }
    }
    return { names, stars };
  }

  return { input, ts, WEB, universe, headText, baseTexts, options, resolve, parse, stringOf, sourceExports };
}

module.exports = { setUp };
