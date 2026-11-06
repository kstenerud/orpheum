> **ABOUTME:** Repointing a library (D429): what exists, what D429 settles, and the questions the
> owner answers before it is built. Read before touching `library.RepointRoot`,
> `repoint-root.tsx` or enrolment.

# Repointing a library

- **Status:** QUESTIONS — 2026-11-05, the 11-07 batch's item 5. Built after the owner's answers,
  with the devnas.example rehoming tests of 2026-11-06 as input.
- **Depends on:** [D429](../../decisions/working-notes.md#d429--a-library-moves-by-one-operation-folder-x-on-node-a-is-now-folder-y-on-node-b-over-absolute-paths)
  (the model), [relocation-audit-2026-11-05.md](../research/relocation-audit-2026-11-05.md) (what the
  schema allows), [visibility-at-scale-2026-11-05.md](../research/visibility-at-scale-2026-11-05.md)
  (the rewrite measured: 252,000 places in about 2.5 s).

## What exists

- **`library.RepointRoot`** (`internal/core/library/move.go`, D241 §6), served as
  `POST /libraries/{id}/roots/repoint` and drawn by `repoint-root.tsx` from the library's page. It
  moves **one library's one root** and the live places under it, to the same or another node.
- **What D429 asks beyond it:** the operation is on a folder, not a library ("folder X on node A is
  now folder Y on node B"). It rewrites every place, every library root at or under X, and every
  other stored path under X, in one transaction, so it carries every library that covers the folder.
  It is confirmed before it commits.
- **Steps 0081–0083 (D432)** made the schema ready: a unique live node name, `place.is_unconfirmed`,
  and the `file_place` view, so a file can gain a place and lose one.

## What D429 already settles

Paths stay absolute; one operation at any level; speed is not a goal; confirmation before commit (same
disk: device and inode; another machine: size, modified time and a sample of payload hashes); what is
not found at Y is reported missing, as any vanished file is.

## Questions for the owner

Each has the lead's recommendation first.

1. **Where it is done.**
   - **(Recommended)** From the System page's Storage area, as "Move a folder…": pick the node and
     folder X from what libraries cover, then node B and folder Y with the folder picker. The library
     page's "Repoint this root" becomes a shortcut into the same dialog with X filled in. One
     operation, two doors (rule 11).
   - Or only from a library's page, as now.
2. **Confirming.**
   - **(Recommended)** A dry run first, drawn as a report: "12,279 files under X; 12,270 found at Y
     (12,261 by size and time, 9 by hash); 9 not found". Then Move, or Cancel. The core refuses Move
     if fewer than half are found, saying the folder is likely wrong.
   - Or move straight away and report after.
3. **Files not found at Y.**
   - **(Recommended)** Their places are rewritten to Y anyway, and they show missing until they turn
     up there; watched state and the rest stay on the `file` row.
   - Or they keep their path at X, still missing.
4. **The old folder.**
   - **(Recommended)** No fallback: after the move, X is not read. D429's single rewrite, nothing to
     keep in step, and "missing" says what is wrong.
   - Or X is kept as a fallback place until it is gone.
5. **A machine replaced** (the devnas.example case: the database moved, the new machine's node has another
   name).
   - **(Recommended)** Its own small operation beside the move: an admin adopts a new node's hello
     onto an old node ("this machine is now nas"), keeping the node id and so every row. Paths that
     differ on the new machine are then a folder move. The audit's §4 calls this the missing piece.
   - Or the folder move from node A to node B covers it, leaving node A retired.
6. **While it runs.**
   - **(Recommended)** Scans of the affected folders pause, and playback of their files keeps going
     from the place it had (a few seconds' rewrite). Other libraries are untouched.
   - Or the core refuses playback and scans everywhere for the move.
7. **Paths outside persistent.db** (the audit's §6: cache.db `job.destination`,
   `scan_batch.files_json`).
   - **(Recommended)** Discarded for the affected folders: they rebuild. Free text that quotes a path
     (`filing_reason`, error details, NFO bodies) stays as history.
   - Or rewritten too.
8. **Who may.**
   - **(Recommended)** The capability that manages libraries, as `RepointRoot` has today.
   - Or the server's owner only.

## From tomorrow's devnas.example tests (2026-11-06)

To note while testing: which folder moved, and on which node; whether the new machine's node kept its
name; how the move was done today (shown at the old path, or repointed); what was found missing; and
anything that had to be done by hand. Those answer questions 3, 5 and 6 from use.

## The owner's answers (2026-11-06)

The owner accepted the recommendations, with these changes and additions:

- **Typing a path (the owner):** "As a quick fix I'd like a text entry path that I can fill in
  manually." The folder picker is to be redesigned later. Two rules for what a typed path may be:
  - **A library's root** (the libraries page) must be a valid path inside the allowed roots, as
    today.
  - **A repoint's new folder** may be any path the process can reach, except known system-only paths
    (`/bin`, `/usr/bin`, `/sys`, `/dev`, `/etc` and such).
- **Its own privilege (question 8, changed):** "Anyone who can manage libraries can change a
  library's root via the libraries page to anything within the already allowed roots, but the ability
  to repoint a root path is a stronger privilege with a threat model, equivalent with adding new root
  paths, and can affect multiple libraries, so it needs its own privilege."
- **Detected, then confirmed (new):** when a directory inside a root is renamed or moved, the system
  detects it and asks the admin whether that is what happened, rather than waiting for a manual
  repoint. The owner's scenarios, with roots `/tank/media` and `/mnt/caspar/video`, and libraries
  movies1 `/tank/media/video/movies`, movies2 `/tank/media/video/movies/disney` and movies3
  `/mnt/caspar/video/movies`:
  1. Caspar goes down; `/mnt/balthasar/movies`, a backup of caspar's movies, is mounted.
  2. `/tank/media/video/movies/disney` moves to `/tank/media/video/kids-movies/disney`.
  3. `/tank/media` moves to `/tank/large-files/media`.
- **Question 7 asked back:** what path data cache.db holds, and why. Answered: three kinds of
  in-flight work state, all of it rebuilt by the next scan or retry (cache.db is rebuildable by
  definition):
  - `job.destination` names what a parked job waits on, such as `root:3:/media/films`, so a down
    folder parks its jobs instead of failing them;
  - `scan.root_path` and `read_again_json` name a scan in progress and what it reads again;
  - `scan_batch.files_json` holds the files a running scan has yet to examine, so a restarted core
    resumes it.
- **Wanted (the owner):** "more wargaming beyond this to look for holes in our plan." To be written in
  `design/research/repoint-wargame-2026-11-06-research.md`.
