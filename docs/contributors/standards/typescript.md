> **ABOUTME:** The TypeScript, React, CSS and test standard for `web/` — what the code looks like
> and which mechanism holds each rule, so an agent's mistake fails `make check` rather than
> waiting for a reader. Every rule names its enforcer; "review-only" marks the few that nothing
> checks yet.

# TypeScript Standard

Covers everything under `web/`: `packages/sdk`, `packages/player`, `apps/web`,
`apps/cast-receiver` ([cut-a.md](../design/plans/cut-a.md) §3). The stack is plain React +
TypeScript + Vite in an npm workspace ([D193](../decisions/working-notes.md) §6).

**Four aims, in this order of cost to lose:**
1. **Readable by a person** who opens one file cold.
2. **Low complexity, held by tooling** — limits are errors, not advice.
3. **Understandable without chasing definitions** — a name says what the thing is.
4. **Agent errors caught early** — the leftmost mechanism that can hold a rule holds it: type
   system → lint → dependency rules → test → review
   ([audit-2026-09-23.md](../design/research/audit-2026-09-23.md) §7).

See also: [testing.md](testing.md) (what a test is for, the uncontrolled list),
[markdown.md](markdown.md) (the package READMEs), `deps/npm.allow`.

## The gate

`make check-web` runs these at once, each one of `web/package.json`'s scripts:

| Step | Command | Config |
| --- | --- | --- |
| Types | `tsc -b` | `web/tsconfig.base.json` + one `tsconfig.json` per package |
| Lint | `eslint --max-warnings 0 .` | `web/eslint.config.js` |
| Format | `prettier --check .` | `web/.prettierrc.json` |
| Boundaries | `depcruise packages apps` | `web/.dependency-cruiser.cjs` |
| Tests | `vitest run` | `web/vitest.config.ts` |

**Zero warnings.** A rule is `error` or `off`; there is no advisory tier, because an agent
reads a passing run as permission. `npm run format` rewrites in place; `npm run build` builds
both apps. The config files above are gates (`scripts/gate_paths.py`): loosening one is the
owner's act.

Versions are exact (`save-exact` in `web/.npmrc`) and `web/package-lock.json` is committed.
No package's install scripts run (`ignore-scripts` there, held by `apps/web/src/npm-config.test.ts`):
each package is its tarball, and one that needs its script to work is the owner's call (rule 3).

## Compiler settings

`web/tsconfig.base.json`; every package extends it and none may relax it.

| Flag | Why |
| --- | --- |
| `strict` | The baseline: null checks, no implicit `any`, strict function types. |
| `noUncheckedIndexedAccess` | `list[0]` is `T \| undefined`. The commonest crash an agent writes is indexing an empty array. |
| `exactOptionalPropertyTypes` | `{ title?: string }` does not accept `title: undefined`, so "absent" and "explicitly cleared" stay different — the API's PATCH semantics depend on it. |
| `noImplicitOverride` | A method that shadows a base method says `override`; renaming the base breaks the build instead of silently orphaning the override. |
| `noPropertyAccessFromIndexSignature` | `record["key"]` for a lookup, `object.key` for a known field: the syntax tells the reader which one could be missing. |
| `noFallthroughCasesInSwitch` | A `case` that falls through is almost always a missing `return`. |
| `noImplicitReturns` | Every path returns, or none does. |
| `noUnusedLocals`, `noUnusedParameters` | Dead code is read as live code. An unavoidable unused parameter is prefixed `_`. |
| `allowUnreachableCode: false`, `allowUnusedLabels: false` | Same. |
| `useUnknownInCatchVariables` | A caught value is `unknown` until narrowed. |
| `verbatimModuleSyntax`, `isolatedModules` | Type-only imports are written `import type`, so what is erased is visible and every file compiles alone (Vite compiles file by file). |
| `noUncheckedSideEffectImports` | `import "./missing.css"` fails. |
| `forceConsistentCasingInFileNames` | macOS forgives a wrong case; Linux CI does not. |
| `moduleResolution: bundler`, `allowImportingTsExtensions` | Relative imports name the file with its extension (`./app-shell.tsx`), so the import says what it points at. |
| `target: ES2023`, `lib` per package | Evergreen browsers and the Cast receiver's Chrome. Packages and apps add `DOM`; nothing gets Node's types except tool configs. |
| `composite`, `emitDeclarationOnly` | Project references let `tsc -b` check packages before the apps that use them. Output goes to git-ignored `.tsbuild/`; Vite does the real build. |

**TypeScript is held at 6.0.** TypeScript 7 (the native compiler) is out, but typescript-eslint
supports `<6.1`. Move when typescript-eslint does.

## Lint

Flat config: `@eslint/js` recommended, typescript-eslint **`strictTypeChecked`** and
**`stylisticTypeChecked`** with the project service, then the rules below.

### No escape from the type system

| Rule | Why |
| --- | --- |
| `no-explicit-any`, the `no-unsafe-*` family | `any` switches the checker off for everything it touches. Use `unknown` and narrow. |
| `consistent-type-assertions: never` | `x as T` is a claim the compiler cannot check. Narrow with a type guard or parse at the boundary. `as const` is allowed. Test files may assert (`as`) on test doubles. |
| `no-non-null-assertion` | `x!` is the same claim. Handle `undefined`. Allowed in tests. |
| `ban-ts-comment` | `@ts-ignore` and `@ts-nocheck` are banned; `@ts-expect-error` needs a description of at least 10 characters. |
| `strict-boolean-expressions` (no strings, no numbers) | `if (count)` hides the `0` case; write `count > 0`. Nullable objects may be tested directly. |
| `switch-exhaustiveness-check`, `considerDefaultExhaustiveForUnions: false` | Adding a member to a union breaks every `switch` that does not name it. A `default` does not count, so it cannot hide the new case (the Go side's `exhaustive` rule, same reason). |
| `explicit-module-boundary-types` | An exported function states its return type, so a reader need not infer it. |
| `consistent-type-imports`, `no-import-type-side-effects` | Pairs with `verbatimModuleSyntax`. |
| `no-restricted-syntax: TSEnumDeclaration` | A string-literal union instead: no runtime object, and number enums accept any number. |

### Promises

| Rule | Why |
| --- | --- |
| `no-floating-promises` | An unawaited promise loses its rejection. Await it, return it, or mark a deliberate fire-and-forget with `void` **and** a `.catch` that reports. |
| `no-misused-promises` | An `async` function passed as `onClick` returns a promise React ignores. Wrap it: `onClick={() => { void save().catch(report); }}`. |
| `only-throw-error`, `prefer-promise-reject-errors` | Only `Error` objects are thrown or rejected, so every failure has a stack. |
| `return-await: always` | The stack includes the awaiting frame, and a `try` around it actually catches. |

### Suppressions

| Rule | Why |
| --- | --- |
| `eslint-comments/require-description` | `// eslint-disable-next-line rule -- why`. No reason, no suppression. |
| `eslint-comments/no-unlimited-disable` | A disable names its rule. |
| `eslint-comments/disable-enable-pair` | No whole-file disable. |
| `linterOptions.reportUnusedDisableDirectives: "error"` | A suppression that no longer suppresses anything is deleted. |

### Complexity

| Rule | Limit | Why |
| --- | --- | --- |
| `complexity` | 10 | A function that needs more branches than this is two functions. |
| `max-depth` | 3 | Early returns, not nesting. |
| `max-lines-per-function` | 60 (code lines) | A component or function that fits on a screen gets read whole. Off in tests. |
| `max-params` | 3 | More than three is an options object with named fields. |
| `max-nested-callbacks` | 3 (4 in tests) | |
| `max-lines` | 800 per file | A file that fits in one read gets read whole. Split at 400 — review-only, since zero-warnings has no "warn" tier; `scripts/check_source.py` warns at 400. |
| `no-nested-ternary`, `no-else-return`, `curly` | | Each branch is visible. |

### One way to do each thing

`no-restricted-imports` names each alternative library and the sanctioned one in its message
(see [Dependencies](#dependencies)). `no-console` (logging goes through a reporter the SDK
provides when it exists), `no-alert`, `eqeqeq`, `no-param-reassign`, `prefer-const`,
`object-shorthand`.

## Naming

**A name says what the thing is, so the reader does not open its definition.**

| Rule | Enforced by |
| --- | --- |
| Values and functions `camelCase`; components and types `PascalCase`; module-level constants may be `UPPER_CASE` | `@typescript-eslint/naming-convention` |
| **Booleans are predicates**: `is`, `has`, `can`, `should`, `was`, `did`, `will`, `needs`, `allows`, `shows` (`isMuted`, `hasSubtitles`) — variables, parameters, properties, accessors | `naming-convention` with `types: ["boolean"]` |
| **No cryptic abbreviations**: `button`, not `btn`; `message`, not `msg`; `error`, not `err`/`e`; `event`, not `evt`. `props`, `ref` and `env` are allowed as React/Vite vocabulary | `unicorn/name-replacements` (identifiers and filenames) |
| **No `utils`, `helpers`, `common`, `misc` or `shared` modules**: a module is named for what it holds (`duration-format.ts`, not `utils.ts`) | `no-restricted-imports` pattern; dependency-cruiser `no-utility-modules`; `unicorn/name-replacements` on the filename |
| **File names are kebab-case** and name the main export: `AppShell` lives in `app-shell.tsx` | `unicorn/filename-case` |
| **Units in names where the type does not carry them**: `positionSeconds`, `offsetMilliseconds`, `sizeBytes`, `bitrateBitsPerSecond` | review-only |
| **Components are named for what they show**: `PosterCard`, `SeasonEpisodeList`, not `Item`, `Wrapper`, `Container` | review-only |
| No leading or trailing underscores except unused parameters | `naming-convention` |
| **Named exports only**, so the import site uses the definition's name. Tool configs are exempt | `no-restricted-syntax: ExportDefaultDeclaration` |
| JSON on the wire is `camelCase`; generated types carry the server's names unchanged | the Go side's JSON tags; the generator |

## Module and package boundaries

| Rule | Why | Enforced by |
| --- | --- | --- |
| No import cycles | Neither module can be understood alone | dependency-cruiser `no-circular` |
| Apps import packages; **an app never imports another app** | Shared code lives in `packages/` | `app-not-to-other-app` |
| **Packages never import apps** | Layering | `package-not-to-app` |
| **The SDK imports no React, React DOM or player** | It must work from a plain page and from the device-lab harness ([D28](../decisions/working-notes.md), [web-client.md](../design/plans/web-client.md)) | `sdk-not-to-react-or-player`; `no-restricted-imports` for `packages/sdk/**` |
| **A package is imported by its name** (`@orpheum/sdk`), never by a path into its `src/` | Its public surface is one file | `exports` in each `package.json` (deep paths do not resolve); `not-into-another-package-by-path`; `no-restricted-imports` patterns |
| **No barrel files.** A package's `src/index.ts` is its one public entry; nowhere else has an `index.ts` | An import names the file that defines the thing | `no-barrel-files` |
| No module nothing imports | Dead code, or an entry point that should be listed | `no-orphans` |
| Every import resolves and is declared in a `package.json` | Typos and undeclared dependencies | `not-to-unresolvable`, `no-non-package-json` |
| Production code does not import devDependencies or test files | They are absent from a build | `not-to-dev-dep`, `not-to-test` |
| Generated code lives in `packages/sdk/src/gen/`, is committed, never edited, and is excluded from lint and format | Regenerated and diffed ([D85](../decisions/working-notes.md)) | ESLint `ignores`; `.prettierignore`; `scripts/gate_paths.py` |

## Server calls

**Every request to our server goes through `@orpheum/sdk`**, whose client is generated from
`api/openapi.json` with `openapi-typescript` + `openapi-fetch` ([cut-a.md](../design/plans/cut-a.md)
§5). A hand-written request drifts from the API silently; a generated one fails `tsc`.

- `fetch`, `XMLHttpRequest`, `WebSocket` and `EventSource` are banned outside `packages/sdk` —
  `no-restricted-globals`, and `no-restricted-properties` for `window.fetch`,
  `globalThis.fetch`, `self.fetch`.
- Credentials travel in a header or cookie, **never in a URL**
  ([D154](../decisions/working-notes.md)) — review-only in the SDK; the M4 HAR test checks it.
- Server state in components goes through TanStack Query once it lands; the SDK stays
  framework-free and React hooks wrap it in the app.

## Configuration

**The built web client reads no environment** ([D204](../decisions/working-notes.md)): what it
needs comes from the server through the SDK. `import.meta.env` is banned in `src/`
by ESLint's `no-restricted-syntax`. Vite's config reads only `ORPHEUM_DEV_*` from the process
environment, never a `.env` file (`envDir: false`), and refuses to serve without
`ORPHEUM_DEV_SERVER` (`src/dev-server.test.ts`).

## Errors

- **A refusal keeps its code.** The SDK turns a refused request into an `ApiProblem` (an
  `Error` carrying the API's `code` and `detail`, [cut-a.md](../design/plans/cut-a.md) §5.2)
  and throws it, through `dataOrThrow` and `throwIfRefused` (`packages/sdk/src/problem.ts`). A
  screen catches it and shows the person words chosen by its `code` (`problemText`), never the
  server's `message`. `code` is typed as a string (the OpenAPI document declares no
  enumeration), so a new code falls to the general words rather than breaking a `switch`. —
  `no-restricted-syntax` (the server's message is never shown); the rest review-only.
- **Every failure we expect has words of its own; the general words mean a bug.** The owner,
  2026-10-06: *"Unknown Error should only happen for truly novel situations, and be a prod for
  us to find and fix the bug, then produce messaging for that scenario."* A failure the client
  can foresee gets a code and its own words: the server's codes, the network's own failure
  (`unreachable`), and an answer from something in front of the server rather than the server,
  which the SDK's client gives a code of its own (`not_answering` for a gateway's 502, 503 or
  504; `unreadable_answer` for any other, which nothing expected sends). Only a failure no one
  foresaw reaches `problem.unknown`, whose words say it is likely a bug; when one turns up, find
  its cause, fix it, and give it words. A screen that asks again by itself says so
  (`willAskAgain`), so the person knows they need not. — `client.test.ts`, `use-resource.test.ts`;
  the rest review-only.
- **Parse, don't assert.** Data from outside (a WebSocket message, `localStorage`, a URL
  parameter) is `unknown` until a parser returns a typed value. — `consistent-type-assertions`.
- **No swallowed rejection.** `no-floating-promises`, `no-misused-promises`. An empty `catch`
  is `no-empty` (in `@eslint/js` recommended).
- **Every error shown to a person names the item, the cause and one next action**
  ([D188](../decisions/working-notes.md) §1). — review-only.
- **A form's refusal names the refused field in the form's words.** A form's submit tells its
  failure with `fieldProblemText`, mapping each location the server can refuse
  (`body.rootPaths`) to the label the person sees on that field; where no label fits, it falls
  back to `problemText`'s words. A refusal of one item in a list (a setting, a secret) is shown
  beside that item, not at the top of the page. — review-only; the server's side is enforced
  (go.md §7).
- React render errors are caught by an error boundary per screen, never one for the whole app.
  — review-only.

## React

| Rule | Enforced by |
| --- | --- |
| Function components only; no class components | `@eslint-react` strict (`no-class-component`) |
| Hooks at the top level of components and hooks only | `react-hooks/rules-of-hooks` |
| Effect dependencies complete; no `setState` in render or synchronously in an effect | `react-hooks/exhaustive-deps`, `set-state-in-effect`, `set-state-in-render` |
| No component defined inside another component (it remounts every render) | `react-hooks/static-components`, `@eslint-react/no-nested-component-definitions` |
| Every list item has a stable key from the data; never the array index | `@eslint-react/no-missing-key`, `no-array-index-key` |
| Listeners, timers and observers added in an effect are removed in its cleanup | `@eslint-react/web-api-no-leaked-*` |
| `{count && <Badge />}` does not render a stray `0` | `@eslint-react/no-leaked-conditional-rendering` |
| **Effects are for synchronising with something outside React** (the player, a socket). Derived values are computed in render; event responses go in the handler | `set-state-in-effect` catches the commonest case; the rest is review-only |
| **State lives in the lowest component that needs it.** Server state is loaded through the app's `useResource` (`apps/web/src/use-resource.ts`); UI state is `useState`/`useReducer`; context only for values nearly every screen reads (the signed-in person, the theme). No global store | `no-restricted-imports` bans Redux, Zustand, MobX, Jotai, Recoil; placement is review-only |
| Props are a named `…Props` type; no spreading unknown props onto DOM elements | review-only |

## Accessibility

**A design requirement from the first screen** ([D188](../decisions/working-notes.md) §2), not a
gate on Cut A's release. The lint half is strict now.

| Rule | Enforced by |
| --- | --- |
| Semantic elements first: `<button>`, `<a href>`, `<nav>`, `<main>`, `<ul>`; a `role` only where no element exists | `jsx-a11y-x` strict, `prefer-tag-over-role`, `no-static-element-interactions` |
| **Every focusable element has an accessible name derived from what it is** — a card's name is its item's title, never a badge's count | `control-has-associated-label` (on; the preset leaves it off), `alt-text`, `anchor-has-content`, `label-has-associated-control`; the "derived from what it is" half is review-only |
| Everything reachable by pointer is reachable by keyboard and remote | `click-events-have-key-events`, `interactive-supports-focus`, `no-noninteractive-tabindex`; the M1 Playwright keyboard walk |
| Nothing focusable is `aria-hidden` | `no-aria-hidden-on-focusable` |
| No state shown only by colour; text scales; `prefers-reduced-motion` honoured | review-only; `apps/web/src/tokens.css` zeroes transitions under reduced motion |
| `<html lang>` is set | `index.html`, review-only |

**Focus only adds** ([D189](../decisions/working-notes.md)). The focused item gets a ring
(two colours: light outer, dark inner), a lift, and at most a scale that stays inside half the
gutter. It **never darkens, dims, desaturates, blurs or overlays** the focused item or its
neighbours, and is always entirely on screen.
- The ring, lift and scale are custom properties in `apps/web/src/tokens.css`; components use
  them and never write their own `:focus` colours. — review-only.
- For every tile component, in every density **and every shipped theme**: the focused box, ring
  included, intersects no neighbour, and the focused artwork's mean luminance and contrast are
  not lower than unfocused. A theme can restyle the ring and lift but cannot pass the test
  while breaking rules 1, 3 or 5. — Playwright test, with the first tile
  ([testing-strategy.md](../design/plans/testing-strategy.md)).

## Styling

**Plain CSS files, one per component, with design tokens as custom properties. No CSS-in-JS
runtime, no CSS Modules, no utility framework.**
- **Why plain CSS over CSS Modules:** themes ([D138](../decisions/working-notes.md) §4) target
  stable class names, and CSS Modules hash them. Custom properties are the theming seam; a
  theme overrides tokens, not component rules. Modules' typed-class benefit is lost anyway
  under `noPropertyAccessFromIndexSignature` without a generator.
- **Why no CSS-in-JS:** runtime cost on TV-class hardware and the Cast receiver, and a second
  place styles live. Banned by `no-restricted-imports` (styled-components, Emotion, Tailwind).
- **Class names are prefixed with the component's file name** (`.app-shell`,
  `.app-shell__title`), so collisions cannot happen and a theme can find the rule. —
  review-only; stylelint is sanctioned if review misses it.
- Colours, sizes, spacing and focus effects appear only as `var(--…)` outside `tokens.css`. —
  review-only.
- Formatted by Prettier; SPDX header as the first comment — `scripts/check_source.py`.

## Text and translation

**Every user-visible string goes through react-i18next** ([D51](../decisions/working-notes.md)),
with `returnEmptyString: false`, English bundled and other catalogues served by the core. No
string is built by concatenating fragments; use interpolation.

**No user-visible text is written inline in JSX**: `JSXText` containing letters, and literal
`aria-label`, `aria-description`, `title`, `alt` or `placeholder` values, fail
`no-restricted-syntax`, whose message sends the author to the translation layer
(`apps/web/src/i18n.ts`, English in `apps/web/src/locales/`). The product name is a constant
(`apps/web/src/product-name.ts`) because it is never translated.

Sort order and date/number formatting come from the server or `Intl`, never `localeCompare` or
hand-built strings ([D47](../decisions/working-notes.md)). — review-only.

## Tests

The rules for what a test is for, the fakes-over-mocks policy and the uncontrolled list are
[testing.md](testing.md)'s. What is TypeScript-specific:

| Rule | Enforced by |
| --- | --- |
| Vitest; tests sit beside the code as `*.test.ts(x)`; imports are explicit (`import { it } from "vitest"`), no globals | `vitest.config.ts` `include`; no `globals` option |
| **No `.only`, `.skip`, `.skipIf`, `.runIf`, `.todo` or `.fixme`** on `describe`/`it`/`test`. A skip comes only from the one prerequisite helper of [testing.md](testing.md) §3 (its TypeScript twin is written with the first test that needs one, and is the one file exempted) | `no-restricted-properties` in test files; `allowOnly: false` in `vitest.config.ts`; `scripts/check_test_weakening.py` on a branch |
| **Fakes over mocks** ([testing.md](testing.md) §5): code takes its dependency (the SDK's transport, a clock) as a parameter and the test passes a hand-written fake; no `vi.mock`/`vi.doMock` of our own modules | `no-restricted-syntax` in test files (relative and `@orpheum/` specifiers); `sinon` banned by `no-restricted-imports` |
| **No snapshot of a large tree.** An inline snapshot of a few lines is allowed; `toMatchSnapshot` of a rendered screen is not — it passes whatever it is shown after `-u` | review-only |
| Components are tested through what a person perceives: role and accessible name, never class names or test IDs when a role exists | review-only. Tests render with React DOM (`renderToStaticMarkup`, or into `happy-dom`); Testing Library is sanctioned but not installed |
| Test files may use `as` on test doubles and `!`; nothing else relaxes | ESLint test-file override |

## Dependencies

**Adding a dependency is an owner action.** Every name in any `web/**/package.json`
(workspace packages excepted) must be in `deps/npm.allow` with a reason and the decision that
chose it — `scripts/check_deps.py`. Runtime dependencies go in the package that imports them;
tools go in `web/package.json` — dependency-cruiser `no-non-package-json`, `not-to-dev-dep`.

**One library per job.** The sanctioned library, and what `no-restricted-imports` sends you
to instead of the usual alternatives:

| Job | Sanctioned | Status | Instead of |
| --- | --- | --- | --- |
| UI | `react`, `react-dom` | in use | — |
| Build, dev server | `vite`, `@vitejs/plugin-react` | in use | — |
| Tests | `vitest` | in use | jest, mocha, chai, sinon |
| API client | `openapi-typescript` + `openapi-fetch`, inside the SDK | in use | axios, ky, got, superagent, node-fetch, cross-fetch |
| Server state in React | `@tanstack/react-query` | not installed; the app's `useResource` does the job | swr, redux, zustand, mobx, jotai, recoil |
| Translation | `react-i18next`, `i18next` | in use | react-intl, lingui |
| HLS playback | `hls.js`, in the player | in use | video.js, shaka-player, plyr |
| ASS/SSA subtitles | `jassub`, in the player | in use | libass-wasm |
| Component tests | `@testing-library/react` | not installed; tests render with React DOM and `happy-dom` | enzyme |
| Browser tests | `@playwright/test` | in use | — |
| Dates, ids, collections | the standard library (`Intl`, `crypto.randomUUID`, `structuredClone`) | — | moment, dayjs, date-fns, luxon, uuid, lodash, ramda |
| Class names | a template literal | — | classnames, clsx |
| Styling | plain CSS + custom properties | in use | styled-components, Emotion, Tailwind |

Routing is the app's own: `apps/web/src/router.ts` maps an address to a route, and
`apps/web/src/navigation.ts` reads and moves the browser's location. No routing library is
installed.

## Licence and headers

Everything under `web/` is MPL-2.0 ([D80](../decisions/working-notes.md)). The first comment
of every `.ts`, `.tsx`, `.js`, `.cjs`, `.css` and `.html` file is
`SPDX-License-Identifier: MPL-2.0`; JSON is exempt. — `scripts/check_source.py`.

## Package READMEs

Every package and app has a `README.md` with three headings, the TypeScript twin of the Go
side's `doc.go`: **Invariants** (each naming the rule or test that enforces it), **Read before
changing** (live decision IDs and plan sections), **Where to change**. — review-only; a cited
`TestArch_` name must exist, `scripts/check_claims.py`.
