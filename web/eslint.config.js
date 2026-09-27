// SPDX-License-Identifier: MPL-2.0

// The lint gate for web/. Every rule here is named, with its reason, in
// docs/contributors/standards/typescript.md. `npm run lint` allows zero warnings, so a rule is
// either an error or off; nothing is advisory.

import eslintComments from "@eslint-community/eslint-plugin-eslint-comments/configs";
import eslintReact from "@eslint-react/eslint-plugin";
import js from "@eslint/js";
import jsxA11y from "eslint-plugin-jsx-a11y-x";
import reactHooks from "eslint-plugin-react-hooks";
import unicorn from "eslint-plugin-unicorn";
import globals from "globals";
import tseslint from "typescript-eslint";

const testFiles = ["**/*.test.{ts,tsx}"];
const toolingFiles = ["*.{js,cjs,ts}", "*/*/vite.config.ts"];
const sdkFiles = ["packages/sdk/**"];

// One library per job (standards/typescript.md, "Dependencies"). Each entry names what to use
// instead, so the error message is the fix.
const alternativeLibraries = [
  ["axios", "the generated SDK client in @orpheum/sdk (openapi-fetch)"],
  ["ky", "the generated SDK client in @orpheum/sdk (openapi-fetch)"],
  ["got", "the generated SDK client in @orpheum/sdk (openapi-fetch)"],
  ["superagent", "the generated SDK client in @orpheum/sdk (openapi-fetch)"],
  ["node-fetch", "the generated SDK client in @orpheum/sdk (openapi-fetch)"],
  ["cross-fetch", "the generated SDK client in @orpheum/sdk (openapi-fetch)"],
  ["swr", "@tanstack/react-query"],
  ["react-query", "@tanstack/react-query"],
  ["lodash", "the standard library (Array, Object, structuredClone)"],
  ["lodash-es", "the standard library (Array, Object, structuredClone)"],
  ["underscore", "the standard library (Array, Object, structuredClone)"],
  ["ramda", "the standard library (Array, Object, structuredClone)"],
  ["moment", "Intl.DateTimeFormat and Date; server-formatted values where possible"],
  ["dayjs", "Intl.DateTimeFormat and Date; server-formatted values where possible"],
  ["date-fns", "Intl.DateTimeFormat and Date; server-formatted values where possible"],
  ["luxon", "Intl.DateTimeFormat and Date; server-formatted values where possible"],
  ["uuid", "crypto.randomUUID()"],
  ["classnames", "a template literal or array join"],
  ["clsx", "a template literal or array join"],
  ["jquery", "React and the DOM"],
  ["redux", "React state, context, and @tanstack/react-query for server state"],
  ["@reduxjs/toolkit", "React state, context, and @tanstack/react-query for server state"],
  ["zustand", "React state, context, and @tanstack/react-query for server state"],
  ["mobx", "React state, context, and @tanstack/react-query for server state"],
  ["jotai", "React state, context, and @tanstack/react-query for server state"],
  ["recoil", "React state, context, and @tanstack/react-query for server state"],
  ["styled-components", "a plain CSS file with custom properties"],
  ["@emotion/react", "a plain CSS file with custom properties"],
  ["@emotion/styled", "a plain CSS file with custom properties"],
  ["tailwindcss", "a plain CSS file with custom properties"],
  ["react-intl", "react-i18next (D51)"],
  ["@lingui/react", "react-i18next (D51)"],
  ["video.js", "hls.js through @orpheum/player"],
  ["shaka-player", "hls.js through @orpheum/player"],
  ["plyr", "hls.js through @orpheum/player"],
  ["libass-wasm", "jassub through @orpheum/player"],
  ["jest", "vitest"],
  ["mocha", "vitest"],
  ["chai", "vitest's expect"],
  ["sinon", "a hand-written fake, or vitest's vi.fn at a boundary"],
  ["enzyme", "@testing-library/react"],
].map(([name, instead]) => ({ name, message: `Use ${instead} instead.` }));

const bannedImportPatterns = [
  {
    group: ["**/packages/*/src/**", "@orpheum/*/src/**", "**/apps/*/src/**"],
    message: "Import a workspace package by its name (@orpheum/sdk), never by its src/ path.",
  },
  {
    regex: String.raw`(^|/)(utils?|helpers?|common|misc|shared)(\.[jt]sx?)?$`,
    message: "No utils/helpers/common modules: name the module for what it holds.",
  },
];

// Server calls go through @orpheum/sdk only (audit §7 item 5).
const networkGlobals = ["fetch", "XMLHttpRequest", "WebSocket", "EventSource"].map((name) => ({
  name,
  message: `Talk to the server through @orpheum/sdk; ${name} is allowed only in packages/sdk.`,
}));

const noDefaultExport = {
  selector: "ExportDefaultDeclaration",
  message: "Use a named export: the name at the import site then matches the definition.",
};
const noEnum = {
  selector: "TSEnumDeclaration",
  message:
    "Use a union of string literals; enums emit runtime code and number enums accept any number.",
};
// D204: the built client reads no environment; what it needs comes from the server.
const noImportMetaEnv = {
  selector: 'MemberExpression[object.type="MetaProperty"][property.name="env"]',
  message:
    "The web client reads no environment (D204); take the value from the server through @orpheum/sdk.",
};
// Until react-i18next lands (D51), user-visible text may not be written inline in JSX.
const noInlineUserText = {
  selector: String.raw`JSXText[value=/\p{L}/u]`,
  message: "User-visible text goes through the translation layer (D51), never inline in JSX.",
};
const noInlineAccessibleText = {
  selector:
    "JSXAttribute[name.name=/^(aria-label|aria-description|title|alt|placeholder)$/] > Literal",
  message: "User-visible text goes through the translation layer (D51), never inline in JSX.",
};
// cut-a §5.2: the server's message is an English note for developers; a person is told what went
// wrong by its code, through problemText, in their language.
const noServerMessageShown = {
  selector:
    "MemberExpression[object.type='MemberExpression'][object.property.name='error'][property.name='message']",
  message:
    "The server's message is a note for developers (cut-a §5.2): throw ApiProblem and show problemText's words.",
};
// D51: text in a JSX expression is as visible as text in JSX.
const noInlineExpressionText = {
  selector: String.raw`:matches(JSXElement, JSXFragment) > JSXExpressionContainer > Literal[value=/\p{L}/u], :matches(JSXElement, JSXFragment) > JSXExpressionContainer > TemplateLiteral > TemplateElement[value.raw=/\p{L}/u]`,
  message: "User-visible text goes through the translation layer (D51), never inline in JSX.",
};

export default tseslint.config(
  {
    // Generated code (the OpenAPI client, later) is regenerated, never edited, so never linted.
    ignores: [
      "**/node_modules/",
      "**/dist/",
      "**/.tsbuild/",
      "**/coverage/",
      "packages/sdk/src/gen/",
    ],
  },
  { linterOptions: { reportUnusedDisableDirectives: "error", reportUnusedInlineConfigs: "error" } },

  js.configs.recommended,
  ...tseslint.configs.strictTypeChecked,
  ...tseslint.configs.stylisticTypeChecked,
  eslintComments.recommended,
  {
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
      globals: { ...globals.browser },
    },
    plugins: { unicorn },
    rules: {
      // Suppressions must say why; a blanket disable is never allowed.
      "@eslint-community/eslint-comments/require-description": ["error", { ignore: [] }],
      "@eslint-community/eslint-comments/no-unlimited-disable": "error",
      "@eslint-community/eslint-comments/disable-enable-pair": ["error", { allowWholeFile: false }],

      // Types are the first gate: no escape hatches.
      "@typescript-eslint/no-explicit-any": "error",
      "@typescript-eslint/consistent-type-assertions": ["error", { assertionStyle: "never" }],
      "@typescript-eslint/no-non-null-assertion": "error",
      "@typescript-eslint/ban-ts-comment": [
        "error",
        {
          "ts-expect-error": "allow-with-description",
          "ts-ignore": true,
          "ts-nocheck": true,
          "ts-check": false,
          minimumDescriptionLength: 10,
        },
      ],
      "@typescript-eslint/strict-boolean-expressions": [
        "error",
        { allowString: false, allowNumber: false, allowNullableObject: true },
      ],
      "@typescript-eslint/switch-exhaustiveness-check": [
        "error",
        { considerDefaultExhaustiveForUnions: false, requireDefaultForNonUnion: true },
      ],
      "@typescript-eslint/explicit-module-boundary-types": "error",
      "@typescript-eslint/consistent-type-imports": "error",
      "@typescript-eslint/no-import-type-side-effects": "error",
      "@typescript-eslint/prefer-readonly": "error",

      // Promises: nothing floats, nothing is passed where a void callback is expected.
      "@typescript-eslint/no-floating-promises": "error",
      "@typescript-eslint/no-misused-promises": "error",
      "@typescript-eslint/only-throw-error": "error",
      "@typescript-eslint/prefer-promise-reject-errors": "error",
      "@typescript-eslint/return-await": ["error", "always"],

      // Names say what a thing is.
      "@typescript-eslint/naming-convention": [
        "error",
        {
          selector: "default",
          format: ["camelCase"],
          leadingUnderscore: "forbid",
          trailingUnderscore: "forbid",
        },
        { selector: "import", format: ["camelCase", "PascalCase"] },
        { selector: "variable", format: ["camelCase", "PascalCase", "UPPER_CASE"] },
        { selector: "function", format: ["camelCase", "PascalCase"] },
        { selector: "parameter", format: ["camelCase"], leadingUnderscore: "allow" },
        { selector: "typeLike", format: ["PascalCase"] },
        { selector: "enumMember", format: ["PascalCase"] },
        {
          selector: ["objectLiteralProperty", "typeProperty"],
          format: null,
          modifiers: ["requiresQuotes"],
        },
        {
          selector: [
            "variable",
            "parameter",
            "classProperty",
            "typeProperty",
            "parameterProperty",
            "accessor",
          ],
          types: ["boolean"],
          format: ["PascalCase"],
          prefix: ["is", "has", "can", "should", "was", "did", "will", "needs", "allows", "shows"],
        },
        { selector: "variable", modifiers: ["destructured"], format: null },
      ],
      "unicorn/name-replacements": [
        "error",
        {
          checkFilenames: true,
          replacements: {
            props: false,
            ref: false,
            refs: false,
            env: false,
            dev: false,
            db: false,
          },
        },
      ],
      "unicorn/filename-case": ["error", { case: "kebabCase" }],

      // Low complexity, enforced.
      complexity: ["error", 10],
      "max-depth": ["error", 3],
      "max-lines": ["error", { max: 800, skipBlankLines: false, skipComments: false }],
      "max-lines-per-function": ["error", { max: 60, skipBlankLines: true, skipComments: true }],
      "max-params": ["error", 3],
      "max-nested-callbacks": ["error", 3],
      "no-nested-ternary": "error",
      "no-else-return": ["error", { allowElseIf: false }],

      // One way to do each thing.
      "no-restricted-imports": [
        "error",
        { paths: alternativeLibraries, patterns: bannedImportPatterns },
      ],
      "no-restricted-globals": ["error", ...networkGlobals],
      "no-restricted-properties": [
        "error",
        ...["window", "globalThis", "self"].map((object) => ({
          object,
          property: "fetch",
          message:
            "Talk to the server through @orpheum/sdk; fetch is allowed only in packages/sdk.",
        })),
      ],
      "no-restricted-syntax": ["error", noDefaultExport, noEnum, noImportMetaEnv],
      "no-console": "error",
      "no-alert": "error",
      eqeqeq: ["error", "always"],
      curly: ["error", "all"],
      "no-param-reassign": ["error", { props: true }],
      "prefer-const": "error",
      "no-var": "error",
      "object-shorthand": "error",
    },
  },

  // React: apps only. Packages stay framework-free by dependency-cruiser; these rules only apply
  // where JSX can appear.
  { ...eslintReact.configs["strict-type-checked"], files: ["apps/**/*.tsx"] },
  eslintReact.configs["disable-conflict-eslint-plugin-react-hooks"],
  {
    files: ["apps/**/*.tsx"],
    plugins: { "react-hooks": reactHooks },
    rules: {
      ...reactHooks.configs.flat.recommended.rules,
      "react-hooks/exhaustive-deps": "error",
      "react-hooks/incompatible-library": "error",
      "react-hooks/unsupported-syntax": "error",
      "no-restricted-syntax": [
        "error",
        noDefaultExport,
        noEnum,
        noImportMetaEnv,
        noInlineUserText,
        noInlineAccessibleText,
        noInlineExpressionText,
        noServerMessageShown,
      ],
    },
  },
  { ...jsxA11y.configs.strict, files: ["apps/**/*.tsx"] },
  {
    // D188: every focusable element has an accessible name. The strict preset leaves these off.
    files: ["apps/**/*.tsx"],
    rules: {
      "jsx-a11y-x/control-has-associated-label": [
        "error",
        {
          depth: 3,
          ignoreElements: ["audio", "canvas", "embed", "input", "textarea", "tr", "video"],
          ignoreRoles: ["grid", "listbox", "menu", "menubar", "radiogroup", "row", "tablist"],
          includeRoles: ["alert", "dialog"],
        },
      ],
      "jsx-a11y-x/no-aria-hidden-on-focusable": "error",
      "jsx-a11y-x/prefer-tag-over-role": "error",
      "jsx-a11y-x/anchor-ambiguous-text": "error",
    },
  },

  // The SDK is the one place that may speak to the network, and it never imports React.
  {
    files: sdkFiles,
    rules: {
      "no-restricted-globals": "off",
      "no-restricted-properties": "off",
      "no-restricted-imports": [
        "error",
        {
          paths: [
            ...alternativeLibraries,
            { name: "react", message: "The SDK is React-free (D28, web-client.md)." },
            { name: "react-dom", message: "The SDK is React-free (D28, web-client.md)." },
            { name: "@orpheum/player", message: "The SDK may not depend on the player." },
          ],
          patterns: bannedImportPatterns,
        },
      ],
    },
  },

  // Tests: long describe blocks are fine; assertions on test doubles are allowed here only.
  {
    files: testFiles,
    rules: {
      // A skipped test is a weaker test. The only exception is the uncontrolled list in
      // standards/testing.md, cited in the disable comment's description.
      "no-restricted-properties": [
        "error",
        ...["describe", "it", "test"].flatMap((object) =>
          ["only", "skip", "skipIf", "runIf", "todo", "fixme"].map((property) => ({
            object,
            property,
            message: "No focused, skipped or todo tests (standards/testing.md).",
          })),
        ),
      ],
      // Fakes over mocks (standards/testing.md §5): our own modules are never vi.mock'ed; the
      // code under test takes its dependency as a parameter instead.
      "no-restricted-syntax": [
        "error",
        noDefaultExport,
        noEnum,
        noImportMetaEnv,
        {
          selector: String.raw`CallExpression[callee.object.name="vi"][callee.property.name=/^(mock|doMock)$/] > Literal.arguments:first-child[value=/^(\.|@orpheum\/)/]`,
          message: "Pass a hand-written fake as a parameter; never vi.mock our own modules.",
        },
      ],
      "max-lines-per-function": "off",
      "max-nested-callbacks": ["error", 4],
      "@typescript-eslint/consistent-type-assertions": [
        "error",
        { assertionStyle: "as", objectLiteralTypeAssertions: "allow-as-parameter" },
      ],
      "@typescript-eslint/no-non-null-assertion": "off",
    },
  },

  // Tool configuration files must default-export, and are not in a browser.
  {
    files: toolingFiles,
    languageOptions: { globals: { ...globals.node } },
    rules: {
      "no-restricted-syntax": ["error", noEnum],
      "unicorn/name-replacements": "off",
    },
  },
  { ...tseslint.configs.disableTypeChecked, files: ["**/*.{js,cjs}"] },
  {
    files: ["**/*.cjs"],
    languageOptions: { sourceType: "commonjs", globals: { ...globals.node } },
    rules: { "@typescript-eslint/no-require-imports": "off" },
  },
);
