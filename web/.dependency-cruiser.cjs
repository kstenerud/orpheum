// SPDX-License-Identifier: MPL-2.0

// Module and package boundaries for web/, checked by `npm run deps:check`. Each rule's reason is
// in docs/contributors/standards/typescript.md, "Module and package boundaries".

const testFile = String.raw`\.test\.tsx?$`;
const configFile = String.raw`(^|/)(vite|vitest)\.config\.ts$`;

/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: "no-circular",
      severity: "error",
      comment: "A cycle means neither module can be understood without the other.",
      from: {},
      to: { circular: true },
    },
    {
      name: "no-orphans",
      severity: "error",
      comment: "A module nothing imports is dead code, or an entry point that belongs in pathNot.",
      from: {
        orphan: true,
        pathNot: [
          testFile,
          configFile,
          String.raw`\.d\.ts$`,
          String.raw`^packages/[^/]+/src/index\.ts$`,
          String.raw`^apps/[^/]+/src/main\.tsx?$`,
        ],
      },
      to: {},
    },
    {
      name: "app-not-to-other-app",
      severity: "error",
      comment: "Apps share code only through packages/; an app never imports another app.",
      from: { path: "^apps/([^/]+)/" },
      to: { path: "^apps/", pathNot: "^apps/$1/" },
    },
    {
      name: "package-not-to-app",
      severity: "error",
      comment: "Packages are the lower layer; they never import an app.",
      from: { path: "^packages/" },
      to: { path: "^apps/" },
    },
    {
      name: "sdk-not-to-react-or-player",
      severity: "error",
      comment: "The SDK is React-free and usable from a plain page (D28, web-client.md).",
      from: { path: "^packages/sdk/" },
      to: {
        path: [
          String.raw`(^|/)node_modules/(react|react-dom|@types/react|@types/react-dom)/`,
          "^packages/player/",
        ],
      },
    },
    {
      name: "not-into-another-package-by-path",
      severity: "error",
      comment:
        "A workspace package is imported by its name (@orpheum/sdk), which resolves through its " +
        "exports field; a relative path into another package's src/ bypasses its public entry. " +
        "An import by name is also typed local once resolved, so it is told apart as aliased-workspace.",
      from: { path: "^(packages|apps)/([^/]+)/" },
      to: {
        path: "^(packages|apps)/",
        pathNot: "^$1/$2/",
        dependencyTypes: ["local"],
        dependencyTypesNot: ["aliased-workspace"],
      },
    },
    {
      name: "no-utility-modules",
      severity: "error",
      comment: "No utils/helpers/common modules: a module is named for what it holds.",
      from: {},
      to: { path: String.raw`(^|/)(utils?|helpers?|common|misc|shared)(/|\.[jt]sx?$)` },
    },
    {
      name: "no-barrel-files",
      severity: "error",
      comment:
        "Only a package's src/index.ts is an index; inside apps and packages, import the module " +
        "that defines the thing, so the import names where it lives.",
      from: {},
      to: {
        path: [String.raw`^apps/.+/index\.tsx?$`, String.raw`^packages/[^/]+/src/.+/index\.tsx?$`],
      },
    },
    {
      name: "not-to-unresolvable",
      severity: "error",
      comment: "An import that does not resolve is a typo or a missing dependency.",
      from: {},
      to: { couldNotResolve: true },
    },
    {
      name: "no-non-package-json",
      severity: "error",
      comment: "Every npm import is declared in a package.json, so the allowlist sees it.",
      from: {},
      to: { dependencyTypes: ["npm-no-pkg", "npm-unknown"] },
    },
    {
      name: "not-to-dev-dep",
      severity: "error",
      comment: "Production code may not import a devDependency; it would be missing from a build.",
      from: { path: "^(packages|apps)/", pathNot: [testFile, configFile] },
      to: {
        dependencyTypes: ["npm-dev"],
        dependencyTypesNot: ["type-only"],
        pathNot: String.raw`node_modules/@types/`,
      },
    },
    {
      name: "not-to-test",
      severity: "error",
      comment: "Production code never imports a test file.",
      from: { pathNot: testFile },
      to: { path: testFile },
    },
  ],
  options: {
    doNotFollow: { path: "node_modules" },
    exclude: { path: String.raw`^(apps|packages)/[^/]+/(\.tsbuild|dist|coverage)/` },
    tsPreCompilationDeps: true,
    combinedDependencies: true,
    tsConfig: { fileName: "tsconfig.json" },
    enhancedResolveOptions: {
      exportsFields: ["exports"],
      conditionNames: ["import", "require", "node", "default"],
      mainFields: ["module", "main"],
      extensions: [".ts", ".tsx", ".js", ".mjs", ".cjs", ".json", ".css"],
    },
    reporterOptions: { text: { highlightFocused: true } },
  },
};
