// SPDX-License-Identifier: AGPL-3.0-or-later
// ABOUTME: The web files a change since a base can alter the running of: each changed file, and what
// ABOUTME: uses its exports, followed by name through re-exports, or loads it when its loading changed.
"use strict";
// Run in web/ by web_tests.py. Reads {typescript, base, changed, universe, roots} from stdin (the
// web_reach_common.cjs input, and the files every test loads first, its setup files); writes
// {reached: [...]}, the files whose running may differ, tests among them, or {wide: why}.
//
// vitest related follows files: every test importing @orpheum/sdk imports its index.ts, which
// re-exports every module, so a change to almost any module ran 400 of 540 tests (2026-11-21). This
// follows top-level names instead. A test sees a module in two ways, and each is followed apart:
//
// - By what it uses. In a changed file, the declarations whose text changed, and the imports bound
//   differently, are tainted; so is every declaration naming a tainted one, and every mutable one (not
//   a const of a literal or a function) a tainted one names, since it may write it. An export of a
//   tainted declaration changes. A file using it (by name, by namespace, or by an `import()` or a
//   vitest mock or actual import, which take the module whole) is reached; the names it binds it to
//   are tainted in turn, unless it only re-exports (a barrel: export ... from, and types): then the
//   names it passes on change.
// - By loading it. A module's top level runs when anything imports it, a barrel's importers too. When
//   it runs differently (a changed file's code outside its declarations' bodies changed, or it loads a
//   module it did not that runs code as it loads), or it calls tainted code as it loads, or hands a
//   tainted value to a call outside a name's initializer (registering it elsewhere, say), every file
//   that loads it, directly or not, is reached. What a name's initializer makes of a tainted value is
//   that name's, followed by name.
//
// Left unfollowed: a changed function writing into another module's state (an imported object's
// fields), and what a getter or an iterator runs as a module loads. Type-only imports load nothing
// (verbatimModuleSyntax: one that is not marked type is kept). Every doubt is wide: a changed file that
// a setup file loads.

const { input, ts, universe, headText, baseTexts, resolve, parse, stringOf, sourceExports } =
  require("./web_reach_common.cjs").setUp();
const changed = input.changed.filter((rel) => universe.has(rel));
const roots = (input.roots ?? []).filter((rel) => universe.has(rel));

// vitest's calls that load a module whole: its mocks (a factory's importOriginal included) and its
// actual imports.
const VI_LOADS = new Set(["mock", "doMock", "unmock", "doUnmock", "importActual", "importMock"]);

/** The spec an `import()` or a vitest call that loads a module whole (VI_LOADS) names, else null. */
const wholeLoad = (node) => {
  if (!ts.isCallExpression(node)) return null;
  if (node.expression.kind === ts.SyntaxKind.ImportKeyword) return stringOf(node.arguments[0]);
  const callee = node.expression;
  if (ts.isPropertyAccessExpression(callee) && ts.isIdentifier(callee.expression) && callee.expression.text === "vi" &&
      VI_LOADS.has(callee.name.text)) {
    return stringOf(node.arguments[0]);
  }
  return null;
};

/** What rel loads and uses of each web source: [{target, loads, names, all, star, reexports}], and
 * whether it only re-exports. */
function edgesOf(rel) {
  const edges = new Map();
  const edge = (spec) => {
    const target = spec === null ? null : resolve(spec, rel);
    if (target === null) return null;
    if (!edges.has(target)) {
      edges.set(target, { target, loads: false, names: new Set(), all: false, star: false, reexports: [] });
    }
    const e = edges.get(target);
    e.loads = true;
    return e;
  };
  const file = parse(rel, headText(rel));
  const visit = (node) => {
    if (ts.isImportDeclaration(node) && !node.importClause?.isTypeOnly) {
      const e = edge(stringOf(node.moduleSpecifier));
      const clause = node.importClause;
      if (e && clause) {
        if (clause.name) e.names.add("default");
        const bindings = clause.namedBindings;
        if (bindings && ts.isNamespaceImport(bindings)) e.all = true;
        if (bindings && ts.isNamedImports(bindings)) {
          for (const s of bindings.elements) if (!s.isTypeOnly) e.names.add((s.propertyName ?? s.name).text);
        }
      }
    } else if (ts.isExportDeclaration(node) && node.moduleSpecifier && !node.isTypeOnly) {
      const e = edge(stringOf(node.moduleSpecifier));
      if (e && !node.exportClause) e.star = true;
      else if (e && ts.isNamespaceExport(node.exportClause)) {
        e.all = true;
        e.reexports.push({ imported: "*", exported: node.exportClause.name.text });
      } else if (e) {
        for (const s of node.exportClause.elements) {
          if (s.isTypeOnly) continue;
          const imported = (s.propertyName ?? s.name).text;
          e.names.add(imported);
          e.reexports.push({ imported, exported: s.name.text });
        }
      }
    } else if (wholeLoad(node) !== null) {
      const e = edge(wholeLoad(node));
      if (e) e.all = true;
    }
    ts.forEachChild(node, visit);
  };
  visit(file);
  const typeOnly = (s) => ts.isInterfaceDeclaration(s) || ts.isTypeAliasDeclaration(s) ||
    (ts.isImportDeclaration(s) && s.importClause?.isTypeOnly) || (ts.isExportDeclaration(s) && s.isTypeOnly);
  const barrel = file.statements.every((s) => typeOnly(s) || (ts.isExportDeclaration(s) && s.moduleSpecifier));
  return { edges: [...edges.values()], barrel };
}

// Calls that run nothing they are given: React's wrappers of a component and its context's making, and
// the empty collections.
const PURE_CALLS = new Map([["react", new Set(["memo", "forwardRef", "lazy", "createContext"])]]);
const PURE_CONSTRUCTORS = new Set(["Map", "Set", "WeakMap", "WeakSet"]);

/** Whether evaluating node runs nothing: a function, a literal, a name, an object or array of them, or
 * a pure call (PURE_CALLS, whose callee packages names local → {spec, imported}) of them. */
function inert(node, packages) {
  const of = (n) => inert(n, packages);
  if (ts.isParenthesizedExpression(node) || ts.isAsExpression(node) || ts.isSatisfiesExpression(node) ||
      ts.isTypeAssertionExpression(node) || ts.isNonNullExpression(node)) {
    return of(node.expression);
  }
  if (ts.isArrowFunction(node) || ts.isFunctionExpression(node) || ts.isIdentifier(node) ||
      ts.isStringLiteral(node) || ts.isNumericLiteral(node) || ts.isBigIntLiteral(node) ||
      ts.isNoSubstitutionTemplateLiteral(node) || ts.isRegularExpressionLiteral(node)) {
    return true;
  }
  if ([ts.SyntaxKind.TrueKeyword, ts.SyntaxKind.FalseKeyword, ts.SyntaxKind.NullKeyword].includes(node.kind)) return true;
  if (ts.isPrefixUnaryExpression(node) && ts.isNumericLiteral(node.operand)) return true;
  if (ts.isArrayLiteralExpression(node)) return node.elements.every(of);
  if (ts.isObjectLiteralExpression(node)) {
    return node.properties.every((p) => (ts.isPropertyAssignment(p) && !ts.isComputedPropertyName(p.name) && of(p.initializer)) ||
                                       ts.isShorthandPropertyAssignment(p) || ts.isMethodDeclaration(p));
  }
  if (ts.isNewExpression(node) && ts.isIdentifier(node.expression) && PURE_CONSTRUCTORS.has(node.expression.text)) {
    return (node.arguments ?? []).length === 0;
  }
  if (ts.isCallExpression(node) && ts.isIdentifier(node.expression)) {
    const imported = packages.get(node.expression.text);
    return imported !== undefined && PURE_CALLS.get(imported.spec)?.has(imported.imported) === true &&
      node.arguments.every(of);
  }
  return false;
}

/** The names node's code names, its types' aside (they run nothing): what a class extends is code. */
const identifiers = (node, into = new Set()) => {
  const walk = (n) => {
    if (ts.isExpressionWithTypeArguments(n)) return walk(n.expression);
    if (ts.isHeritageClause(n) && n.token === ts.SyntaxKind.ImplementsKeyword) return;
    if (ts.isTypeNode(n)) return;
    if (ts.isIdentifier(n)) into.add(n.text);
    ts.forEachChild(n, walk);
  };
  walk(node);
  return into;
};
const bindingNames = (name, into = []) => {
  if (ts.isIdentifier(name)) into.push(name.text);
  else for (const element of name.elements) if (!ts.isOmittedExpression(element)) bindingNames(element.name, into);
  return into;
};
const declared = (statement) => ts.canHaveModifiers(statement) &&
  (ts.getCombinedModifierFlags(statement) & ts.ModifierFlags.Ambient) !== 0;

/** Whether a top-level variable's value can change after load: anything but a const of a primitive
 * literal or a function. */
const mutable = (declaration, list) => {
  if ((list.flags & ts.NodeFlags.Const) === 0 || !declaration.initializer) return true;
  let init = declaration.initializer;
  while (ts.isParenthesizedExpression(init) || ts.isAsExpression(init) || ts.isSatisfiesExpression(init)) init = init.expression;
  return !(ts.isArrowFunction(init) || ts.isFunctionExpression(init) || ts.isStringLiteral(init) ||
           ts.isNumericLiteral(init) || ts.isNoSubstitutionTemplateLiteral(init) || ts.isBigIntLiteral(init) ||
           [ts.SyntaxKind.TrueKeyword, ts.SyntaxKind.FalseKeyword, ts.SyntaxKind.NullKeyword].includes(init.kind));
};
// whole's mark for what takes a module whole at load.
const LOAD = "\0load";
const exported = (statement) => ts.canHaveModifiers(statement) &&
  (ts.getCombinedModifierFlags(statement) & ts.ModifierFlags.Export) !== 0;
const isDefault = (statement) => ts.canHaveModifiers(statement) &&
  (ts.getCombinedModifierFlags(statement) & ts.ModifierFlags.Default) !== 0;

/** What rel's top level declares, imports, exports and runs at load: {decls: name → {text, refs,
 * mutable}, imports: local → {target, imported, spec}, packages: local → {spec, imported} for a
 * package's, loads: the web sources it imports, exports: exported → local, from: exported → where it is
 * re-exported from, stars: its `export *` targets, whole: target → the declarations (LOAD: what runs at
 * load) that take it whole, runNodes: [{node, bound}] what runs at load, bound when its value only
 * initializes a name, shape: their text}. */
function loadingOf(rel, text) {
  const file = parse(rel, text);
  const decls = new Map();
  const imports = new Map();
  const packages = new Map();
  const loads = new Set();
  const exports = new Map();
  const from = new Map();
  const stars = [];
  const whole = new Map();
  const runNodes = [];
  const shape = [];
  const takesWhole = (node, by) => {
    const visit = (n) => {
      const target = wholeLoad(n) === null ? null : resolve(wholeLoad(n), rel);
      if (target !== null) {
        if (!whole.has(target)) whole.set(target, new Set());
        whole.get(target).add(by);
      }
      ts.forEachChild(n, visit);
    };
    visit(node);
  };
  const declare = (name, node, isMutable = false) => {
    const d = decls.get(name) ?? { text: "", refs: new Set(), mutable: false };
    d.text += node.getText();
    identifiers(node, d.refs);
    d.mutable ||= isMutable;
    decls.set(name, d);
    takesWhole(node, name);
  };
  const run = (node, bound = false) => {
    runNodes.push({ node, bound });
    shape.push(node.getText());
    takesWhole(node, LOAD);
  };
  for (const statement of file.statements) {
    if (ts.isInterfaceDeclaration(statement) || ts.isTypeAliasDeclaration(statement) || declared(statement)) continue;
    if (ts.isImportDeclaration(statement)) {
      const clause = statement.importClause;
      if (clause?.isTypeOnly) continue;
      const spec = stringOf(statement.moduleSpecifier);
      const target = resolve(spec, rel);
      if (target !== null) loads.add(target);
      if (!clause) {
        shape.push(statement.getText()); // imported for what its loading does
        continue;
      }
      const bind = (local, imported) => {
        imports.set(local, { target, imported, spec });
        if (target === null) packages.set(local, { spec, imported });
      };
      if (clause.name) bind(clause.name.text, "default");
      const bindings = clause.namedBindings;
      if (bindings && ts.isNamespaceImport(bindings)) bind(bindings.name.text, "*");
      if (bindings && ts.isNamedImports(bindings)) {
        for (const s of bindings.elements) if (!s.isTypeOnly) bind(s.name.text, (s.propertyName ?? s.name).text);
      }
    } else if (ts.isExportDeclaration(statement)) {
      if (statement.isTypeOnly) continue;
      const target = statement.moduleSpecifier ? resolve(stringOf(statement.moduleSpecifier), rel) : null;
      const where = statement.moduleSpecifier ? (target ?? stringOf(statement.moduleSpecifier)) : null;
      if (target !== null) loads.add(target);
      if (!statement.exportClause) stars.push(where);
      else if (ts.isNamespaceExport(statement.exportClause)) from.set(statement.exportClause.name.text, `${where}:*`);
      else {
        for (const s of statement.exportClause.elements) {
          if (s.isTypeOnly) continue;
          const local = (s.propertyName ?? s.name).text;
          if (where === null) exports.set(s.name.text, local);
          else from.set(s.name.text, `${where}:${local}`);
        }
      }
    } else if (ts.isFunctionDeclaration(statement) || ts.isClassDeclaration(statement)) {
      const name = statement.name?.text ?? "default";
      declare(name, statement);
      if (exported(statement)) exports.set(isDefault(statement) ? "default" : name, name);
      if (ts.isClassDeclaration(statement)) {
        // What a class runs as it is declared: what it extends, its decorators, its computed names and
        // its static members.
        for (const clause of statement.heritageClauses ?? []) if (clause.token === ts.SyntaxKind.ExtendsKeyword) run(clause);
        for (const modifier of statement.modifiers ?? []) if (ts.isDecorator(modifier)) run(modifier);
        for (const member of statement.members) {
          if (member.name && ts.isComputedPropertyName(member.name)) run(member.name);
          const isStatic = ts.canHaveModifiers(member) && (ts.getCombinedModifierFlags(member) & ts.ModifierFlags.Static);
          if (ts.isClassStaticBlockDeclaration(member) || (isStatic && ts.isPropertyDeclaration(member))) run(member);
          for (const modifier of (ts.canHaveModifiers(member) && member.modifiers) || []) if (ts.isDecorator(modifier)) run(modifier);
        }
      }
    } else if (ts.isVariableStatement(statement)) {
      const list = statement.declarationList;
      for (const declaration of list.declarations) {
        for (const name of bindingNames(declaration.name)) {
          declare(name, declaration, mutable(declaration, list));
          if (exported(statement)) exports.set(name, name);
        }
        if (!ts.isIdentifier(declaration.name) || (declaration.initializer && !inert(declaration.initializer, packages))) {
          run(declaration, ts.isIdentifier(declaration.name));
        }
      }
    } else if (ts.isExportAssignment(statement)) {
      declare("default", statement);
      exports.set("default", "default");
      if (!inert(statement.expression, packages)) run(statement);
    } else if (ts.isEnumDeclaration(statement) || ts.isModuleDeclaration(statement)) {
      declare(statement.name.text, statement, true);
      if (exported(statement)) exports.set(statement.name.text, statement.name.text);
      run(statement);
    } else if (statement.kind !== ts.SyntaxKind.EmptyStatement) {
      run(statement);
    }
  }
  return { decls, imports, packages, loads, exports, from, stars, whole, runNodes, shape: shape.join("\n") };
}

/** Grows taint, a set of a file's top-level names whose behaviour may have changed: a declaration
 * naming one changes, and so does a mutable one that a changed declaration names (it may write it). */
function growTaint(loading, taint) {
  for (let grew = true; grew;) {
    grew = false;
    for (const [name, d] of loading.decls) {
      const refs = [...d.refs];
      if (!taint.has(name) && refs.some((ref) => taint.has(ref))) {
        taint.add(name);
        grew = true;
      }
      if (taint.has(name)) {
        for (const ref of refs) {
          if (!taint.has(ref) && loading.decls.get(ref)?.mutable) {
            taint.add(ref);
            grew = true;
          }
        }
      }
    }
  }
}

// Calls of what runs nothing it is given when called: the empty or filled collections and a pattern.
const PURE_GLOBALS = new Set(["Map", "Set", "WeakMap", "WeakSet", "RegExp"]);

/** Whether what loading runs at load can run tainted code or store it beyond its own names: a call (a
 * construction, a tagged template, a decorator) of a tainted callee; an assignment of a tainted value;
 * or, outside a name's initializer and unless the callee is pure, a call with a tainted argument. What
 * a name's initializer makes of a tainted argument is that name's, which is followed by name. A
 * function's body runs only when called. */
function loadsTainted(loading, taint) {
  const hits = (n) => n !== undefined && [...identifiers(n)].some((name) => taint.has(name));
  const pure = (callee) => {
    if (!ts.isIdentifier(callee)) return false;
    const imported = loading.packages.get(callee.text);
    if (imported) return PURE_CALLS.get(imported.spec)?.has(imported.imported) === true;
    return PURE_GLOBALS.has(callee.text) && !loading.decls.has(callee.text) && !loading.imports.has(callee.text);
  };
  const walk = (n, bound) => {
    if (ts.isFunctionLike(n) && !ts.isClassStaticBlockDeclaration(n)) return false;
    if (ts.isCallExpression(n) || ts.isNewExpression(n) || ts.isTaggedTemplateExpression(n) || ts.isDecorator(n)) {
      const callee = ts.isTaggedTemplateExpression(n) ? n.tag : n.expression;
      if (hits(callee)) return true;
      const args = ts.isTaggedTemplateExpression(n) ? [n.template] : ts.isDecorator(n) ? [] : (n.arguments ?? []);
      if (!bound && !pure(callee) && args.some(hits)) return true;
    }
    if (ts.isBinaryExpression(n) && n.operatorToken.kind >= ts.SyntaxKind.FirstAssignment &&
        n.operatorToken.kind <= ts.SyntaxKind.LastAssignment && hits(n.right)) {
      return true;
    }
    return ts.forEachChild(n, (child) => walk(child, bound)) === true;
  };
  return loading.runNodes.some(({ node, bound }) => walk(node, bound));
}

const memo = (f) => {
  const kept = new Map();
  return (rel) => {
    if (!kept.has(rel)) kept.set(rel, f(rel));
    return kept.get(rel);
  };
};
const edges = memo(edgesOf);
const loading = memo((rel) => loadingOf(rel, headText(rel)));

/** Every name rel exports, its star re-exports' included. */
const exportNames = memo((rel) => namesOf(rel, new Set()));
function namesOf(rel, seen) {
  seen.add(rel);
  const own = sourceExports(rel);
  const names = new Set(own.names);
  for (const target of own.stars) {
    if (!seen.has(target)) for (const name of namesOf(target, seen)) if (name !== "default") names.add(name);
  }
  return names;
}

function main() {
  const users = new Map(); // target → [{user, edge}]
  for (const rel of universe) {
    for (const edge of edges(rel).edges) {
      if (!users.has(edge.target)) users.set(edge.target, []);
      users.get(edge.target).push({ user: rel, edge });
    }
  }

  // What the setup files load runs before every test.
  const setUpLoads = new Set();
  for (const pending = [...roots]; pending.length > 0;) {
    const rel = pending.pop();
    if (setUpLoads.has(rel)) continue;
    setUpLoads.add(rel);
    for (const edge of edges(rel).edges) pending.push(edge.target);
  }
  for (const rel of changed) if (setUpLoads.has(rel)) return { wide: `${rel} is loaded before every test` };

  const reached = new Set();
  const loadChanged = new Set();
  const loadWork = [];
  const loadChange = (rel) => {
    if (loadChanged.has(rel)) return;
    loadChanged.add(rel);
    loadWork.push(rel);
  };
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
  const tainted = new Map(); // file → its top-level names whose behaviour may have changed
  /** Taints locals in file, and passes on what that changes: its exports, and its loading. */
  const taintIn = (file, locals) => {
    if (!tainted.has(file)) tainted.set(file, new Set());
    const taint = tainted.get(file);
    const size = taint.size;
    for (const local of locals) taint.add(local);
    if (taint.size === size) return;
    const l = loading(file);
    growTaint(l, taint);
    for (const [name, local] of l.exports) if (taint.has(local)) change(file, name);
    if (loadsTainted(l, taint)) loadChange(file);
  };

  const before = baseTexts(changed);
  for (const rel of changed) {
    reached.add(rel);
    const is = loading(rel);
    if (!before.has(rel)) {
      changeAll(rel);
      if (is.runNodes.length > 0) loadChange(rel);
      continue;
    }
    const was = loadingOf(rel, before.get(rel));
    const differs = (a, b) => [...new Set([...a.keys(), ...b.keys()])].filter((k) => JSON.stringify(a.get(k)) !== JSON.stringify(b.get(k)));
    const texts = (decls) => new Map([...decls].map(([name, d]) => [name, d.text]));
    const locals = [...differs(texts(was.decls), texts(is.decls)), ...differs(was.imports, is.imports)];
    for (const name of [...differs(was.exports, is.exports), ...differs(was.from, is.from)]) change(rel, name);
    if (JSON.stringify(was.stars) !== JSON.stringify(is.stars)) changeAll(rel);
    // What it newly loads runs as it loads.
    const newlyLoaded = [...is.loads].some((target) => !was.loads.has(target) && loading(target).runNodes.length > 0);
    if (was.shape !== is.shape || newlyLoaded) loadChange(rel);
    taintIn(rel, locals);
  }

  while (work.length > 0) {
    const [file, name] = work.pop();
    for (const { user, edge } of users.get(file) ?? []) {
      const starred = edge.star && name !== "default";
      if (!edge.all && !edge.names.has(name) && !starred) continue;
      reached.add(user);
      if (starred) change(user, name);
      for (const r of edge.reexports) if (r.imported === name || r.imported === "*") change(user, r.exported);
      if (edges(user).barrel) continue;
      const l = loading(user);
      const locals = [...l.imports].filter(([, i]) => i.target === file && (i.imported === name || i.imported === "*"))
        .map(([local]) => local);
      // An `import()` or a vitest mock takes the module whole: the declarations holding one change, and
      // what loads the user, when its top level holds one.
      const holders = l.whole.get(file) ?? new Set();
      if (holders.has(LOAD)) loadChange(user);
      taintIn(user, [...locals, ...[...holders].filter((name) => name !== LOAD)]);
    }
  }
  while (loadWork.length > 0) {
    const file = loadWork.pop();
    reached.add(file);
    for (const { user, edge } of users.get(file) ?? []) if (edge.loads) loadChange(user);
  }
  return { reached: [...reached].sort() };
}

process.stdout.write(JSON.stringify(main()));
