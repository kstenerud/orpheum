> **ABOUTME:** Repointing a library (D429): what exists, what D429 settles, and the questions the
> owner answers; built 2026-11-06 except where marked. Read before touching `library.Repoint`,
> `repoint-root.tsx` or enrolment.

# Repointing a library

- **Status:** SPECIFIED — 2026-11-06, from the owner's answers and the owner's three decisions on the
  wargame ("The spec" below). Built against it; what is not built yet is said there.
- **Depends on:** [D429](../../decisions/working-notes.md#d429--a-library-moves-by-one-operation-folder-x-on-node-a-is-now-folder-y-on-node-b-over-absolute-paths)
  (the model), [relocation-audit-2026-11-05.md](../research/relocation-audit-2026-11-05.md) (what the
  schema allows), [visibility-at-scale-2026-11-05.md](../research/visibility-at-scale-2026-11-05.md)
  (the rewrite measured: 252,000 places in about 2.5 s).

## What exists

- **Before 2026-11-06, `library.RepointRoot`** (D241 §6), served as
  `POST /libraries/{id}/roots/repoint` and drawn by `repoint-root.tsx` from the library's page,
  moved **one library's one root** and the live places under it, to the same or another node. It is
  replaced by `library.Repoint` and `library.PreviewRepoint` (`internal/core/library/repoint.go`),
  served as `POST /repoints` and `POST /repoints/preview`, built to the spec below.
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
5. **A machine replaced** (the development NAS case: the database moved, the new machine's node has another
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

## From tomorrow's the development NAS tests (2026-11-06)

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
- **Where a repoint may point (the owner, 2026-11-06, settling the above):** the node runs in a
  container given its roots read-only (`[media] roots`, `mediaOverride` in
  `internal/deployment/write.go`), and confines every read beneath them (`internal/node/confine`). An
  admin adds or removes access by editing the roots and restarting. So a repoint's new folder is
  anywhere beneath a root of the target node; the machine's owner draws that line, and no list of
  system paths is needed.
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
  [design/research/repoint-wargame-2026-11-06-research.md](../research/repoint-wargame-2026-11-06-research.md).

## The spec (the owner, 2026-11-06)

The owner's three decisions on the [wargame](../research/repoint-wargame-2026-11-06-research.md),
which override the research where they differ:

1. **An audit record, not "reversible".** Each repoint is recorded: who, when, from, to, the nodes,
   and the counts. Undoing one is another repoint, Y back to X; there is no rollback feature.
2. **Files not found at the new place stay missing.** A repoint neither deletes nor merges them away;
   the normal handling of a missing file applies (D241, D208).
3. **A repoint never merges into what is already known at the target.** If the target already holds
   known places, the repoint is refused, and the dry run shows the conflict, with counts and
   examples, before the person confirms.

### What the person states

One operation (D429 §2): **folder X on node A is now folder Y on node B.** The person names node A by
its name and X as a path; node B by its name and Y as a typed path (the owner's quick fix) or one
chosen in the folder picker.

- **Node A** is any live node, connected or not: a repoint is how a folder on a node that is down
  (scenario 1) is pointed elsewhere.
- **X** is matched by the bytes the node reported (`place.reported_path_raw`, D211), the spelling
  `library_root.root_path` keeps (D107). It need not be a library's root: any folder at any level.
- **Node B** must be connected, and **Y** must be beneath one of its configured roots (the owner,
  2026-11-06; `libraryFolderOn`).
- **Y the same as X on the same node** is refused: there is nothing to move.

### The dry run, and what it shows

`POST /repoints/preview` changes nothing and answers:

- **What moves:** the places under X on node A, live and removed, counted apart, with up to five
  examples, each old path beside its new one; every library root at or under X (live and retired),
  each with its library and new path.
- **What each library gains and loses:** for every library whose listing the move changes, how many
  live files it lists now that it would not after, and the reverse. A library whose root is above X
  loses the files; one whose root moves keeps them; one whose root holds Y gains them (the
  research's §4.5). Roles reach libraries (D243), so this is also who can see them.
- **Whether it crosses nodes.**
- **Y on node B:** the folder's state as node B lists it (readable, missing, not answering), as the
  folder picker shows it (D359 §2).
- **Conflicts (decision 3):** every place on node B under Y, live or removed, and every library root
  on node B at or under Y, that is not itself moving; counted, with up to five examples.
- **Look-alikes:** places on node A whose text path (NFC) is under X but whose bytes are not (a
  folder spelled in another normal form, the research's §4.10). They are a different folder on disk
  and do not move; the count is shown so the person is not surprised.
- **Refusals:** each reason the move would be refused, as codes the page words: nothing under X;
  the same folder; Y not readable on node B; conflicts at Y; a library would hold a folder inside
  another of its own folders (D363 §1, `ErrRootCovered`); a scan of a folder at, under or above X
  on node A, or Y on node B, in progress.

### The confirm

`POST /repoints` takes the same four values. It runs the dry run's checks again inside its
transaction and refuses with the same reasons (409) when any holds, so a page left open cannot move
what the person did not see. The person confirms on the page after reading the dry run; the move
button is not offered while it is refused.

### The transaction

One write transaction (D429 §2):

1. **Places** under X on node A, live and removed, take node B and Y plus the rest of their path:
   the bytes by prefix, the text recomputed from the new bytes (`PathText`). The path a file was
   identified from (`file_filing.identified_from_path`) follows its place where it equals the
   place's old path, so identification does not take it for a renamed file (D208 §1). A live place
   is marked unconfirmed (step 0081), as the first version did, so the next scan of Y confirms each
   by its read. Moved to another node, a place's device and inode are cleared: they name nothing
   there.
2. **Library roots** on node A at or under X, live and retired, take node B and Y plus the rest.
3. **Two steps through a temporary prefix.** SQLite checks a UNIQUE index row by row, so moving
   `/nest` to `/nest/sub` fails partway when written in one pass (visibility-at-scale §5). Every
   moved place and root is first written under a prefix no real path has (it does not start with
   `/`), then to its new path.
4. **The scan state of the old folders is forgotten:** `root_state`, `root_scan`, `root_mount` and
   `unavailable_place` rows of every root at or under X on node A, and every mount under X. It is
   derived and rebuilt by the scan of Y; carried over, X's device facts would mislead the removal
   checks at Y (D208 §3).
5. **The audit record** is written (below; for now a log line, after the commit).

After it commits: the nodes are told their watched folders changed, the work parked on the old
folders is cancelled, and a scan of each moved live library root at Y is asked for, or of the
library folders that hold Y when no root moved.

### The audit record (decision 1)

A table `repoint` (step 0084, appended: production runs, CHECKPOINT 0048): its time, the account
that did it, node A and X, node B and Y, and the counts moved: live places, removed places, roots.
`GET /repoints` lists them, newest first. Undoing is another repoint from Y to X; the record names
both.

**Not built yet (2026-11-06):** a migration step needs its golden schema
(`internal/store/testdata/golden/`), a gate file, so the step waits for the owner's
`Gate-Changed:` (open question 7). Until then each repoint logs `repointed a folder` at info with
the same fields: `byUserId`, `nodeName`, `path`, `newNodeName`, `newPath`, `livePlaceCount`,
`removedPlaceCount`, `rootCount`. The proposed step:

```sql
CREATE TABLE repoint (
  id INTEGER PRIMARY KEY,
  repointed_at INTEGER NOT NULL,
  by_user_id INTEGER REFERENCES user(id) ON DELETE SET NULL,
  node_id INTEGER NOT NULL REFERENCES node(id),
  path TEXT NOT NULL,
  new_node_id INTEGER NOT NULL REFERENCES node(id),
  new_path TEXT NOT NULL,
  live_place_count INTEGER NOT NULL,
  removed_place_count INTEGER NOT NULL,
  root_count INTEGER NOT NULL
) STRICT;
```

### Missing files (decision 2)

A moved place not found at Y is not deleted or merged: the scan of Y finds it missing and handles it
as any vanished file (D241, D208's removal checks). A moved live place stays unconfirmed until a scan
reads it.

### Conflicts (decision 3)

Known at the target means a place row (live or removed) on node B under Y, or a library root row
(live or retired) on node B at or under Y, that is not itself moving. Any one refuses the repoint;
the dry run counts them and shows up to five. Nothing is merged; `MakeFileACopy`'s join is not used.

### Nested and overlapping folders

- **Y under X on one node** (`/nest` to `/nest/sub`): allowed. The places already under Y are under X
  and move too (to `/nest/sub/sub/…`), as the disk says; the temporary prefix keeps the UNIQUE checks
  from tripping.
- **X under Y on one node** (`/nest/sub` to `/nest`): allowed when nothing else is known under Y;
  otherwise a conflict.
- **Libraries over X** (a root above X) keep their root and lose the files, unless Y is also under
  them. **Libraries under X** move with it. **Within one library**, a moved root that lands at, under
  or above another of its live roots on node B is refused (`ErrRootCovered`, D363 §1).

### Raw and NFC prefixes

X and Y are byte prefixes. A place's text path is recomputed from its new bytes, never edited as
text, so the two stay in step (D429 §4). The look-alike count says what a byte prefix leaves behind.

### Paths a prefix rewrite cannot reach

- **cache.db `job.destination`** (`root:3:/media/films`): the work parked on each old root, and on
  each mount under X, is cancelled; the sweeps derive it again for the new place (plan question 7).
- **cache.db `scan.root_path`, `read_again_json`, `scan_batch.files_json`:** a repoint is refused
  while a scan of a folder at, under or above X or Y is in progress (the research's §4.7), so no scan
  holds the old paths when it commits.
- **Free text that may quote a path** (`place.filing_reason`, `place.error_detail_json`,
  `file_filing.filing_reason`, `file_nfo.body`, `person.picture_ref`): history, left as it is.

### The API

- `POST /repoints/preview`: body `{nodeName, path, newNodeName, newPath}`; the dry run.
- `POST /repoints`: the same body; the move. 409 with the refusals when refused.
- `GET /repoints`: the audit record (not built: it waits for the table, open question 7).
- `POST /libraries/{id}/roots/repoint` (one library's one root, the first version) goes: the folder
  operation replaces it, one way to do the job (rule 11), and fixes the research's hole 3 (a nested
  library's files not moving).

All are an admin's for now (open question 1).

### The UI

The library page's "This folder moved…" on a root opens the repoint dialog with node A and X filled
in. In it the person chooses node B and Y (the places offered or the folder picker; a typed path is
not built), presses **Check what moves**, and reads the dry run: what moves, each library's gains and losses, Y's state, the
conflicts with examples, the look-alikes. **Use the new place** is offered only when nothing refuses it, as the
dangerous action; **Keep it where it is** comes first and has focus (D421). After the move the dialog
says what moved and that the scan of Y confirms each file.

### Open for the owner

Each is a question the three decisions leave; what is built picks the choice easiest to change.

1. **Who holds the repoint privilege** (the owner's answer to question 8; the research's decision 2
   and hole 11). There is no capability to stand beside: every library operation is the admin's,
   and an admin edits roles. Built: the admin's, as before; the check is in one place to change.
2. **What confirms a move before it commits** (D429 §3; the research's decision 5, hole 4), and
   plan question 2's "refused when fewer than half are found". Counting the files found at Y needs a
   listing of Y's files from node B, which the node does not offer outside a scan (it lists folders
   only). Built: Y's folder state is checked; each moved file is confirmed after, by the scan's read
   (`is_unconfirmed`), the full re-read the research's hole 4 names.
3. **Is a conflict any known place under Y, or only one at a path the move would write?** Built: any
   under Y (decision 3 read strictly), which refuses flattening `/a/b/c` into `/a/b` beside other
   known files. Loosening it later is one query.
4. **Removed places and retired roots at Y count as known**, so a target once scanned and emptied
   refuses. Built: they count, the stricter reading.
5. **Moves detected and then confirmed** (the owner's addition), **removals that look like a move
   waiting** (the research's decision 4), **a backup as a second place** (its decision 3),
   **adopting a replaced machine's node** (plan question 5), **a stable filesystem identity** (its
   §4.11) and **what the dry run says per role** (its decision 7): not built; each is its own piece.
6. **Where else the dialog opens:** the System page's Storage area "Move a folder…" (plan question 1)
   for a folder that is no library's root. Built: the API takes any folder; the page's door is from
   a library's root only.
7. **The audit table** (decision 1): its step and golden schema are gate files, so they need the
   owner's `Gate-Changed:` trailer. Built: the log line above; the table, and `GET /repoints`, when
   the owner adds or approves the step.
