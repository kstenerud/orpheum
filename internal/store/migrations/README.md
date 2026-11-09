> **ABOUTME:** Where the schema's migration steps live: numbered SQL files that the store's own
> runner applies in order and that sqlc reads as the schema: the baseline, 0086, production's
> schema at the migration origin (D460), and every step after it. Migrations are live.

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
- **Once production runs, a sliding window** (D272 §5, §6). The owner commits `CHECKPOINT` here,
  holding the version production last ran. The baseline stays at that version and is frozen like
  every committed step; a schema change appends a step, so production migrates through every
  step since it last ran. When the owner says production has migrated, they move the checkpoint,
  and the steps up to it fold into a new baseline at that version.
- **Live, append only.** The owner declares migrations live by committing a file named `LIVE`
  here, which they did at the baseline 0086 (D460, 2026-11-09): the two rules above are history.
  From then on a step that has been merged is never edited, renamed or deleted; a mistake is
  corrected by a new step. The pre-edit hook blocks edits to committed steps, and to the baseline
  too once `LIVE` exists.
- **Forward only.** There are no down migrations; the snapshot taken before migrating is the way
  back ([migrations.md](../../../docs/contributors/design/plans/migrations.md)).
- **DDL and data statements only.** The runner owns `PRAGMA user_version`, `application_id` and
  `foreign_keys`; a step never sets them.
