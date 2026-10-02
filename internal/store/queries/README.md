> **ABOUTME:** Where the SQL that sqlc compiles into internal/store/sqlcgen lives: one file per
> concern, named queries only, parameters only.

# Queries

Each file holds persistent.db's queries for one concern (`library.sql`, `play.sql`, `secret.sql`,
…) and `make generate` turns them into typed Go in `../sqlcgen`, which is committed and never
edited by hand. cache.db's queries (`job.sql` among them) are in `../cache/queries` and generate
into `../cachegen`. The rules are in
[standards/sql.md](../../../docs/contributors/standards/sql.md) §Queries; in short:

- **Name every query for what it returns or does**, with its sqlc command:
  `-- name: ListFilmsByName :many`.
- **Read through the `live_*` views**; name every column: `SELECT *` fails
  `TestArch_QueriesNameEveryColumn`.
- **Parameters only.** No SQL is assembled from strings anywhere in Go. Search's three statements
  are written by hand in `../search.go`, because sqlc cannot parse FTS5's `MATCH`, and are static
  too.
- **Queries on the `secret` table carry `SealedSecret` in their name**, and only
  `internal/core/secrets` may call them.
- **Query files are ASCII.** sqlc 1.31 rewrites `sqlc.arg(…)` by position and miscounts a
  multibyte character in a comment, which fails the file with *edited query syntax is invalid*.
  Write "section 3", not the section sign.
- **A file's leading comment becomes the first query's Go doc comment**, so keep it short.
