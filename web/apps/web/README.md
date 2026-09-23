# @orpheum/web

The web client: the browser and phone interface, served embedded by the core. MPL-2.0.

## Invariants

- **Imports packages by name, never another app.** `app-not-to-other-app` and
  `not-into-another-package-by-path` (`web/.dependency-cruiser.cjs`).
- **Server calls go through `@orpheum/sdk`.** `no-restricted-globals` (`fetch`,
  `WebSocket`, `EventSource`, `XMLHttpRequest`).
- **Every focusable element has an accessible name** (D188). `jsx-a11y-x` strict plus
  `control-has-associated-label`.
- **No user-visible text written inline in JSX** (D51). `no-restricted-syntax` on `JSXText`
  and literal `aria-label`/`title`/`alt`/`placeholder`.
- **The shell renders one `main` landmark headed by the product name.**
  `src/app-shell.test.tsx`.
- **Focus only adds** (D189): ring, lift and modest scale are custom properties in
  `src/tokens.css`. Review-only until the first tile lands; then the geometry and luminance
  tests of [testing-strategy.md](../../../docs/contributors/design/plans/testing-strategy.md) enforce it.

## Read before changing

[D51](../../../docs/contributors/decisions/working-notes.md), [D138](../../../docs/contributors/decisions/working-notes.md),
[D188](../../../docs/contributors/decisions/working-notes.md), [D189](../../../docs/contributors/decisions/working-notes.md),
[D193](../../../docs/contributors/decisions/working-notes.md) §6;
[web-client.md](../../../docs/contributors/design/plans/web-client.md);
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The frame every screen renders in: `src/app-shell.tsx`.
- Colours, sizes, focus effects: `src/tokens.css`, never a component's CSS.
- The mount point: `src/main.tsx` and `index.html`.
