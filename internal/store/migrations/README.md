> **ABOUTME:** Where the schema's migration steps live: numbered SQL files that the store's own
> runner applies in order and that sqlc reads as the schema — until migrations are live, a
> baseline and the one step after it.

# Migrations

The rules are in
[standards/sql.md](../../../docs/contributors/standards/sql.md) §Migrations and the tables in
[schema.md](../../../docs/contributors/design/plans/schema.md); in short:

- **One file per step, named `NNNN_what_it_does.sql`**, numbered with no gaps, each with its line
  in `SHA256SUMS`.
- **Until migrations are live, a baseline and one step** (D228). `NNNN_baseline.sql` builds a new
  database at version NNNN in one step, and its `-- orpheum:baseline folds <sha256>` line names
  the checksum of the last step it folded. A schema change folds the current last step into the
  baseline, which moves up to that step's version, and adds the change as the one step after
  it; the goldens in `../testdata/golden/` follow. A development database is only ever one step
  behind, and migrates through it.
- **Live, append only.** The owner declares migrations live by committing a file named `LIVE`
  here. From then on a step that has been merged is never edited, renamed or deleted; a mistake
  is corrected by a new step. The pre-edit hook blocks edits to committed steps, and to the
  baseline too once `LIVE` exists.
- **Forward only.** There are no down migrations; the snapshot taken before migrating is the way
  back ([migrations.md](../../../docs/contributors/design/plans/migrations.md)).
- **DDL and data statements only.** The runner owns `PRAGMA user_version`, `application_id` and
  `foreign_keys`; a step never sets them.
