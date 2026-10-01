# Contributing to Orpheum

Orpheum is developed in a private repository while its first users test it, and will open for
public betas later ([D192](docs/contributors/decisions/working-notes.md)). These rules hold from
the first commit, so that nothing has to change when it opens.

## Licensing is by directory

| Directory | Licence | Header every source file carries |
| --- | --- | --- |
| everything outside `web/` (server, media node, relay, scripts) | GPL-3.0-or-later ([`LICENSE`](LICENSE)) | `SPDX-License-Identifier: GPL-3.0-or-later` |
| `web/` (the SDK, the player, the web client, the cast receiver) | MPL-2.0 ([`web/LICENSE`](web/LICENSE)) | `SPDX-License-Identifier: MPL-2.0` |

The split is deliberate: a GPL application cannot ship through Apple's App Store, and MPL's
per-file copyleft keeps the clients open while letting them ship
([D80](docs/contributors/decisions/working-notes.md)). `make check` fails on a file whose header
does not match its directory. **Code must not move from `web/` into the server tree, or the
other way, without the owner's decision**, because that changes its licence.

By contributing you agree that your contribution is licensed under the licence of the directory
it lands in.

## Setting up

You need Go 1.26, Node 22, Python 3, shellcheck and a C compiler (only for `make tools-sqlc`).

```
make tools     # the pinned golangci-lint
make setup     # git hooks, web dependencies
make check     # the gate; must pass before every commit
```

`make help` lists every target.

## Developing

```
make dev          # the app on http://127.0.0.1:5173, rebuilt on every change
```

The first run fetches the pinned FFmpeg and generates a small synthetic library, then prints
every setting it chose: users `owner`/`owner` and `partner`/`partner`, debug logs, pprof, the
race detector. The choices live in `deploy/dev/orpheum.dev.toml` and `deploy/dev/seed.toml`; all
state lives in `.dev/`, which `make dev-reset` and `make dev-clean` throw away. The debugging targets —
`dev-db`, `dev-logs`, `dev-debug`, `pprof`, `probe`, `test-one`, `cover` and more — are in
`make help` and [D197](docs/contributors/decisions/working-notes.md).

## Making a change

- Read [`AGENTS.md`](AGENTS.md). Its rules apply to humans too, and the commit-msg and
  pre-commit hooks enforce the mechanical ones.
- Standards: [`docs/contributors/standards/`](docs/contributors/standards/README.md).
- The build contract for the first release:
  [`docs/contributors/design/plans/cut-a.md`](docs/contributors/design/plans/cut-a.md).
- Before opening a pull request, run `make check-pr` against your base branch.
