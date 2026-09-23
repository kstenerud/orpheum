# @orpheum/cast-receiver

Our Google Cast receiver: a second Vite app over the same SDK and player, served from
`https://media.<domain>/cast/`. MPL-2.0. Does nothing until M5.

## Invariants

- **Imports packages by name, never the web app.** `app-not-to-other-app`
  (`web/.dependency-cruiser.cjs`).
- **Served under `/cast/`.** `base` in `vite.config.ts`.
- **Server calls go through `@orpheum/sdk`.** `no-restricted-globals`.

## Read before changing

[D124](../../../docs/contributors/decisions/working-notes.md), [D188](../../../docs/contributors/decisions/working-notes.md) §3,
[D192](../../../docs/contributors/decisions/working-notes.md), [D193](../../../docs/contributors/decisions/working-notes.md) §6;
[cut-a.md](../../../docs/contributors/design/plans/cut-a.md) §2, §8;
[web-client.md](../../../docs/contributors/design/plans/web-client.md).

## Where to change

- The entry point: `src/main.ts`.
- The CAF receiver SDK script tag: `index.html`, in M5.
