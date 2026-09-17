> **ABOUTME:** Index for the decisions directory — the append-only, D-numbered decision log
> that principles, standards and design docs cite by number.

# Decisions

The append-only decision log. Each entry is a proto-ADR — what was decided, what was rejected,
why, and the consequences — with a stable `D`-number that other docs cite.

- **[working-notes.md](working-notes.md)** — the live log. New decisions append to the bottom.

## Conventions

- One D-number per meaningful decision; trivial choices don't get one. Numbers are never reused
  or renumbered.
- Superseded decisions are marked `Superseded by Dnn` with a forward link, not deleted.
- A decision that has not been validated against real hardware or a real library says so, in
  its own **Status** line. Most of the early ones have not been.
