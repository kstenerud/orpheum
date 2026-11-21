// SPDX-License-Identifier: AGPL-3.0-or-later
// ABOUTME: The web files whose lint a change since a base can alter: each changed file, and each file
// ABOUTME: using an export whose declared type changed, followed by name through imports (impact.py).
"use strict";
// Run in web/ by impact.py. Reads {typescript, base, changed, universe} from stdin: the typescript
// package's directory, the base revision, the changed files and every web source (both relative to
// web/, the working tree's); writes {lint: [...]} or {wide: why}, and fails on what it cannot read.
//
// Lint reads other files only through their types (and the @deprecated of their JSDoc). Under
// isolatedDeclarations (tsconfig.base.json) a file's declaration is its own text's alone, so how a
// change reaches another file's lint can be followed by name:
//
// - A changed file's declaration, compared with the base's statement by statement, names its
//   changed locals; a local is changed too when its declared text names a changed one.
// - An export of a changed local, a new export and a gone one are changed exports.
// - A file importing a changed export is linted, and the export's name taints its import there:
//   an export of that file whose declaration names the import is changed in its turn. A re-export
//   (`export { a } from`, `export * from`) passes the change on under the name it exports.
// - A namespace import, `import()` and `import("…")` as a type use every export.
//
// Every doubt is wide: a file whose declaration TypeScript cannot write from it alone (or whose
// declaration this cannot read) changes every export it has, and a changed file declaring a global
// or augmenting a module lints everything.

const path = require("node:path");
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");

const input = JSON.parse(fs.readFileSync(0, "utf8"));
const ts = require(input.typescript);
const WEB = process.cwd();
const universe = new Set(input.universe);
const changed = input.changed;

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

/** How rel's source uses each web source it names: {target, names, all, star, reexports}, and
 * whether it declares a global or augments a module. */
function usesOf(rel) {
  const edges = new Map();
  let declaresGlobally = false;
  const edge = (spec) => {
    const target = spec === null ? null : resolve(spec, rel);
    if (target === null) return null;
    if (!edges.has(target)) edges.set(target, { target, names: new Set(), all: false, star: false, reexports: [] });
    return edges.get(target);
  };
  const visit = (node) => {
    if (ts.isImportDeclaration(node)) {
      const e = edge(stringOf(node.moduleSpecifier));
      const clause = node.importClause;
      if (e && clause) {
        if (clause.name) e.names.add("default");
        const bindings = clause.namedBindings;
        if (bindings && ts.isNamespaceImport(bindings)) e.all = true;
        if (bindings && ts.isNamedImports(bindings)) {
          for (const s of bindings.elements) e.names.add((s.propertyName ?? s.name).text);
        }
      }
    } else if (ts.isExportDeclaration(node) && node.moduleSpecifier) {
      const e = edge(stringOf(node.moduleSpecifier));
      if (e && !node.exportClause) e.star = true;
      else if (e && ts.isNamespaceExport(node.exportClause)) {
        e.all = true;
        e.reexports.push({ imported: "*", exported: node.exportClause.name.text });
      } else if (e) {
        for (const s of node.exportClause.elements) {
          const imported = (s.propertyName ?? s.name).text;
          e.names.add(imported);
          e.reexports.push({ imported, exported: s.name.text });
        }
      }
    } else if (ts.isImportTypeNode(node) && ts.isLiteralTypeNode(node.argument)) {
      const e = edge(stringOf(node.argument.literal));
      if (e) e.all = true;
    } else if (ts.isCallExpression(node) && node.expression.kind === ts.SyntaxKind.ImportKeyword) {
      const e = edge(stringOf(node.arguments[0]));
      if (e) e.all = true;
    } else if (ts.isModuleDeclaration(node) && (ts.isStringLiteral(node.name) || node.name.text === "global")) {
      declaresGlobally = true;
    }
    ts.forEachChild(node, visit);
  };
  visit(parse(rel, headText(rel)));
  return { edges: [...edges.values()], declaresGlobally };
}

/** rel's declaration, as text: {locals: name → {text, refs}, exports: exported → local name or
 * {from, imported}, stars: [target], imports: local → {target, imported}}, or null when TypeScript
 * cannot write it from the file alone or this cannot read it. */
function declarationOf(rel, text) {
  const out = ts.transpileDeclaration(text, { compilerOptions: options, fileName: path.basename(rel), reportDiagnostics: true });
  if (out.diagnostics && out.diagnostics.length > 0) return null;
  const file = ts.createSourceFile("declaration.d.ts", out.outputText, ts.ScriptTarget.ES2023, true, ts.ScriptKind.TS);
  const locals = new Map();
  const exports = new Map();
  const stars = [];
  const imports = new Map();
  const target = (spec) => resolve(spec, rel) ?? `package:${spec}`;
  const refsOf = (node) => {
    const refs = new Set();
    const walk = (n) => {
      if (ts.isIdentifier(n)) refs.add(n.text);
      if (ts.isImportTypeNode(n) && ts.isLiteralTypeNode(n.argument) && stringOf(n.argument.literal) !== null) {
        refs.add(`import(${target(n.argument.literal.text)})`);
      }
      ts.forEachChild(n, walk);
    };
    walk(node);
    return refs;
  };
  // A local's text holds its leading comments: a JSDoc's @deprecated is lint another file reads.
  const declare = (name, node) => {
    const local = locals.get(name) ?? { text: "", refs: new Set() };
    local.text += node.getFullText();
    for (const ref of refsOf(node)) local.refs.add(ref);
    locals.set(name, local);
  };
  const flags = (node) => ts.getCombinedModifierFlags(node);
  for (const statement of file.statements) {
    if (ts.isImportDeclaration(statement)) {
      const from = target(statement.moduleSpecifier.text);
      const clause = statement.importClause;
      if (clause?.name) imports.set(clause.name.text, { target: from, imported: "default" });
      const bindings = clause?.namedBindings;
      if (bindings && ts.isNamespaceImport(bindings)) imports.set(bindings.name.text, { target: from, imported: "*" });
      if (bindings && ts.isNamedImports(bindings)) {
        for (const s of bindings.elements) imports.set(s.name.text, { target: from, imported: (s.propertyName ?? s.name).text });
      }
    } else if (ts.isExportDeclaration(statement)) {
      const from = statement.moduleSpecifier ? target(statement.moduleSpecifier.text) : null;
      if (!statement.exportClause) stars.push(from);
      else if (ts.isNamespaceExport(statement.exportClause)) {
        exports.set(statement.exportClause.name.text, { from, imported: "*" });
      } else {
        for (const s of statement.exportClause.elements) {
          const local = (s.propertyName ?? s.name).text;
          exports.set(s.name.text, from === null ? local : { from, imported: local });
        }
      }
    } else if (ts.isExportAssignment(statement)) {
      declare("export default", statement);
      exports.set("default", "export default");
    } else if (ts.isVariableStatement(statement)) {
      for (const declaration of statement.declarationList.declarations) {
        if (!ts.isIdentifier(declaration.name)) return null;
        declare(declaration.name.text, statement);
        if (flags(statement) & ts.ModifierFlags.Export) exports.set(declaration.name.text, declaration.name.text);
      }
    } else if (ts.isFunctionDeclaration(statement) || ts.isClassDeclaration(statement) ||
               ts.isInterfaceDeclaration(statement) || ts.isTypeAliasDeclaration(statement) ||
               ts.isEnumDeclaration(statement) || (ts.isModuleDeclaration(statement) && ts.isIdentifier(statement.name))) {
      const name = statement.name?.text ?? "export default";
      declare(name, statement);
      if (flags(statement) & ts.ModifierFlags.Export) {
        exports.set(flags(statement) & ts.ModifierFlags.Default ? "default" : name, name);
      }
    } else if (statement.kind !== ts.SyntaxKind.EmptyStatement) {
      return null;
    }
  }
  return { locals, exports, stars, imports };
}

const memo = (f) => {
  const kept = new Map();
  return (rel) => {
    if (!kept.has(rel)) kept.set(rel, f(rel));
    return kept.get(rel);
  };
};
const uses = memo(usesOf);
const declaration = memo((rel) => declarationOf(rel, headText(rel)));

/** Every name rel exports, its star re-exports' included. */
const exportNames = memo((rel) => namesOf(rel, new Set()));
function namesOf(rel, seen) {
  seen.add(rel);
  const names = new Set();
  const fromStar = (target) => {
    if (target !== null && universe.has(target) && !seen.has(target)) {
      for (const name of namesOf(target, seen)) if (name !== "default") names.add(name);
    }
  };
  const d = declaration(rel);
  if (d !== null) {
    for (const name of d.exports.keys()) names.add(name);
    d.stars.forEach(fromStar);
    return names;
  }
  // No declaration: the names its source exports, read from its statements.
  for (const e of uses(rel).edges) if (e.star) fromStar(e.target);
  for (const statement of parse(rel, headText(rel)).statements) {
    const f = ts.canHaveModifiers(statement) ? ts.getCombinedModifierFlags(statement) : 0;
    if (ts.isExportDeclaration(statement) && statement.exportClause && ts.isNamedExports(statement.exportClause)) {
      for (const s of statement.exportClause.elements) names.add(s.name.text);
    } else if (ts.isExportDeclaration(statement) && statement.exportClause) {
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
  return names;
}

function main() {
  const users = new Map(); // target → [{user, edge}]
  for (const rel of universe) {
    const { edges, declaresGlobally } = uses(rel);
    if (declaresGlobally && changed.includes(rel)) return { wide: `${rel} declares a global or augments a module` };
    for (const edge of edges) {
      if (!users.has(edge.target)) users.set(edge.target, []);
      users.get(edge.target).push({ user: rel, edge });
    }
  }

  const lint = new Set();
  const propagated = new Map(); // file → the exports whose change has been passed on
  const work = [];
  const change = (file, name) => {
    if (!propagated.has(file)) propagated.set(file, new Set());
    if (propagated.get(file).has(name)) return;
    propagated.get(file).add(name);
    work.push([file, name]);
  };
  const changeAll = (file) => {
    for (const name of exportNames(file)) change(file, name);
  };
  const tainted = new Map(); // file → its declaration's changed locals and imports
  // Taints the locals of file's declaration that name a tainted one, and changes its exports of them.
  const spread = (file) => {
    const d = declaration(file);
    if (d === null) return changeAll(file);
    const taint = tainted.get(file);
    for (let grew = true; grew;) {
      grew = false;
      for (const [name, local] of d.locals) {
        if (!taint.has(name) && [...local.refs].some((ref) => taint.has(ref))) {
          taint.add(name);
          grew = true;
        }
      }
    }
    for (const [exported, local] of d.exports) if (typeof local === "string" && taint.has(local)) change(file, exported);
  };

  const before = baseTexts(changed.filter((rel) => universe.has(rel)));
  for (const rel of changed) {
    if (!universe.has(rel)) continue; // gone: what imported it changed with it
    lint.add(rel);
    const was = before.has(rel) ? declarationOf(rel, before.get(rel)) : null;
    const is = declaration(rel);
    if (was === null || is === null) {
      changeAll(rel);
      continue;
    }
    const taint = new Set();
    for (const [name, local] of is.locals) if (was.locals.get(name)?.text !== local.text) taint.add(name);
    for (const name of was.locals.keys()) if (!is.locals.has(name)) taint.add(name);
    for (const [name, i] of is.imports) {
      const j = was.imports.get(name);
      if (j?.target !== i.target || j?.imported !== i.imported) taint.add(name);
    }
    const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);
    for (const [exported, local] of is.exports) if (!same(was.exports.get(exported), local)) change(rel, exported);
    for (const exported of was.exports.keys()) if (!is.exports.has(exported)) change(rel, exported);
    if (!same(was.stars, is.stars)) changeAll(rel);
    tainted.set(rel, taint);
    spread(rel);
  }

  while (work.length > 0) {
    const [file, name] = work.pop();
    for (const { user, edge } of users.get(file) ?? []) {
      const starred = edge.star && name !== "default";
      if (!edge.all && !edge.names.has(name) && !starred) continue;
      lint.add(user);
      if (starred) change(user, name);
      for (const r of edge.reexports) if (r.imported === name || r.imported === "*") change(user, r.exported);
      const d = declaration(user);
      if (d === null) {
        changeAll(user);
        continue;
      }
      if (!tainted.has(user)) tainted.set(user, new Set());
      const taint = tainted.get(user);
      for (const [local, i] of d.imports) if (i.target === file && (i.imported === name || i.imported === "*")) taint.add(local);
      taint.add(`import(${file})`);
      spread(user);
    }
  }
  return { lint: [...lint].sort() };
}

process.stdout.write(JSON.stringify(main()));
