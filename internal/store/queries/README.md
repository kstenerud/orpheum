> **ABOUTME:** Where the SQL that sqlc compiles into internal/store/sqlcgen lives: one file per
> concern, named queries only, parameters only.

# Queries

Empty until M1. Each file holds the queries for one concern (`library.sql`, `play_state.sql`,
`job.sql`, `secret.sql`, …) and `make generate` turns them into typed Go in `../sqlcgen`, which
is committed and never edited by hand. The rules are in
[standards/sql.md](../../../docs/contributors/standards/sql.md) §Queries; in short:

- **Name every query for what it returns or does**, with its sqlc command:
  `-- name: ListLiveTitlesByLibrary :many`.
- **Read through the `live_*` views**; name every column; `SELECT *` fails `sqlc vet`.
- **Parameters only.** No SQL is assembled from strings anywhere in Go; the one exception is the
  search filter builder in `internal/store`.
- **Queries on the `secret` table carry `SealedSecret` in their name**, and only
  `internal/core/secrets` may call them.
