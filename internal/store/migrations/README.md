> **ABOUTME:** Where the schema's migration steps live: append-only, numbered SQL files that the
> store's own runner applies in order and that sqlc reads as the schema.

# Migrations

Empty until M1, whose first step is [schema.md](../../../docs/contributors/design/plans/schema.md)
as `0001_initial.sql`. The rules are in
[standards/sql.md](../../../docs/contributors/standards/sql.md) §Migrations; in short:

- **One file per step, named `NNNN_what_it_does.sql`**, numbered from `0001` with no gaps.
- **Append only.** A step that has been merged is never edited, renamed or deleted; a mistake is
  corrected by a new step. The pre-edit hook blocks edits here.
- **Forward only.** There are no down migrations; the snapshot taken before migrating is the way
  back ([migrations.md](../../../docs/contributors/design/plans/migrations.md)).
- **DDL and data statements only.** The runner owns `PRAGMA user_version`, `application_id` and
  `foreign_keys`; a step never sets them.
