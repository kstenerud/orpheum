> **ABOUTME:** Principle §7: Migrations are a safety feature.

# §7 — Migrations are a safety feature

Back up before migrating, automatically, and say where. Forward-only, ordered, in-tree,
CI-verified. Detect-and-refuse across a version gap rather than auto-converting. Test against a
realistically-sized database, because hour-long hangs are invisible on a small one.

**Sharpened by [D181](../decisions/working-notes.md):** an *older* database is always
converted — every step is kept forever — and a *newer* one is refused unless it declared itself
compatible, naming the snapshot that leads back. The pre-migration snapshot is **kept after
success**, because that is when people want to go back. The policy is
[plans/migrations.md](../design/plans/migrations.md).

**Bites when:** a schema change seems too small to need a migration test.
