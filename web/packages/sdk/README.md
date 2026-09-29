# @orpheum/sdk

The client side of the API, usable from a plain web page: generated API types and client,
probe-and-pick, the Watch Together client. MPL-2.0. So far it holds the API client, the
playable-formats probe (what this browser can play), the wait for a file the server is still
making, and the reading of refusals; the Watch Together client is still to come.

## Invariants

- **No React, React DOM or player import.** `sdk-not-to-react-or-player`
  (`web/.dependency-cruiser.cjs`) and `no-restricted-imports` for `packages/sdk/**`
  (`web/eslint.config.js`).
- **The only code that talks to the server.** `fetch`, `WebSocket`, `EventSource` and
  `XMLHttpRequest` are banned everywhere else by `no-restricted-globals` and
  `no-restricted-properties`; this package is the one exemption.
- **One public entry.** `exports` in `package.json` exposes only `src/index.ts`;
  `not-into-another-package-by-path` and `no-barrel-files` stop other routes in.
- **Generated code under `src/gen/` is never edited by hand**; it is excluded from lint and
  formatting and regenerated from `api/openapi.json` (`make generate`).

## Read before changing

[D28](../../../docs/contributors/decisions/working-notes.md), [D80](../../../docs/contributors/decisions/working-notes.md),
[D85](../../../docs/contributors/decisions/working-notes.md), [D154](../../../docs/contributors/decisions/working-notes.md),
[D191](../../../docs/contributors/decisions/working-notes.md), [D193](../../../docs/contributors/decisions/working-notes.md) §6;
[cut-a.md](../../../docs/contributors/design/plans/cut-a.md) §5;
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The public surface: `src/index.ts`.
- The API types and client: regenerate; never edit `src/gen/`. The client's setup:
  `src/client.ts`.
- What this browser can play: `src/playable-formats.ts`.
- Waiting for a file the server is still making (`whenReady`, `textWhenReady`): `src/ready.ts`.
- Reading a refusal (`ApiProblem`, `dataOrThrow`, `throwIfRefused`): `src/problem.ts`.
