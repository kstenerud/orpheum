> **ABOUTME:** User-story scenarios for Cut A: concrete things the two people do to their library
> and their viewing, each with what they should see and what the model must hold, walked through
> the design to find where it is silent or wrong before the code meets the same case.

# Scenarios

- **Status:** M2's scenarios walked through 2026-09-24; M3–M5's written, to be walked before
  each of those milestones starts. Asked for by the owner: *"We haven't done user story
  scenarios yet to test our model."*
- **Depends on:** [library-model.md](library-model.md), [scanner.md](scanner.md),
  [identification.md](identification.md), [play-state.md](play-state.md),
  [backup-restore.md](backup-restore.md), [schema.md](schema.md).

## Why

Plans describe mechanisms and decisions answer questions, one at a time. Nothing walked a
sequence of ordinary events through all of them and asked *what does each table hold now, and is
that what the person expects?* The first walk found a decided defence that was never built
(agent-failures A3) and three questions nobody had asked.

## How to read an entry

**Story** is what a person does. **Expected** is what they should see and what the model must
hold. **Design** names the decisions that answer it. **Status** is one of:

- **holds** — built, with the test that shows it;
- **holds on paper** — the design answers it and the code does not reach it yet;
- **gap G\<n\>** — the design answers it and the code does not do it; to build;
- **question Q\<n\>** — the design is silent or contradicts itself; for the owner;
- **walk before M\<n\>** — not walked yet.

Every M2 scenario that the code reaches becomes a test in the scenario tier: the ingest,
identification and API driven by scripted node reports, with no filesystem and no nodes, which is
what [D42](../../decisions/working-notes.md) says the core's decisions can be tested with.

The people are **the owner**, at home, admin, and **the partner**, remote, a user
([D174](../../decisions/working-notes.md): invited from outside).

## M2 — the library

### S1 — Fixing a typo in a filename

- **Story:** the owner renames `Metropolis (1959).mkv` to `Metropolis (1995).mkv` in the same folder.
- **Expected:** the same film, not a new one: its row, play state and any correction follow it.
- **Design:** a new path with a payload last seen under this root is a move (D179).
- **Status:** **holds**: the file's row follows it
  (`TestIngest_D179MatchesAFileByItsPayloadWhereverItIs`), and it is identified again from its new
  name (`TestPass_ARenamedFileIsIdentifiedAgainUnlessConfirmed`, D208 §1).

### S2 — Fixing track titles with mkvpropedit

- **Story:** the owner sets a film's title and renames its German audio track in place.
- **Expected:** the same file; the new track name shows; nothing else changes.
- **Design:** a tag edit keeps the payload hash (D179); the probe is re-read and the streams
  re-derived.
- **Status:** **holds** (`TestPayloadIdentity_D179TableWithTheRealTools`; the ingest re-derives streams
  on every observation).

### S3 — Replacing a film with a better copy at the same path

- **Story:** the owner overwrites `Metropolis (1995).mkv` with a 4K remux of the same name.
- **Expected:** the film is still watched; the old file's resume position does not carry to the
  new file, because two files' timelines are never related.
- **Design:** a different payload at a path is a new file and the old row is tombstoned (D173);
  watched and identification survive at the Title (D157, D173).
- **Status:** file level **holds** (the D179 test's replaced case); Title level **holds on paper**.

### S4 — Keeping the 1080p beside a new 4K

- **Story:** the owner adds `Metropolis (1995) 2160p.mkv` next to the 1080p file.
- **Expected:** one film with two playable entries, told apart by resolution and filename; not a
  quality menu, and no "Unknown Edition".
- **Design:** two Versions under the Title, edition unknown (D155).
- **Status:** **holds on paper**; identification builds it.

### S5 — Reorganising folders inside a library

- **Story:** the owner moves twenty films into `Crime/`, `Sci-Fi/` and so on, within one root.
- **Expected:** all twenty are the same films, with their history.
- **Status:** **holds** (the D179 test's moved case).

### S6 — Moving a film to another library

- **Story:** the owner moves a film from *Films* to *Kids*, a different root.
- **Expected:** the same film, still watched by whoever watched it.
- **Design:** a payload match in another root is a duplicate, not a move, because that root was
  not looked at (fixed 2026-09-24, `TestIngest_AFileInAnotherRootIsNeverTakenForAMove`); the old
  row is tombstoned when its own root's next scan completes. A Title belongs to a library
  (`item.library_id`).
- **Status:** **holds** for a film with a provider id: its Title, with no live files left in
  *Films*, moves to *Kids* (`TestPass_ATitleFollowsItsFilesToAnotherLibrary`, D208 §2). A film
  with no provider id arrives as a new Title.

### S7 — A USB disk unplugged, leaving an empty mount point

- **Story:** the disk holding *Films* is unplugged. `/media/films` is now the empty directory
  under the mount point, and a scan runs.
- **Expected:** nothing is removed; the films show as unavailable; the admin is told the root
  looks unmounted.
- **Design:** the silent case (D41): the listing succeeds and is empty, no error fires. The node
  reports each root's device id and mount facts and tells "listed, empty" from "not
  enumerated"; the core compares the device id against what it recorded and withholds removals
  on implausible shrinkage, **asking** rather than refusing (D41, D42).
- **Status:** **holds** (`TestScenario_S7_S8_AnUnpluggedDiskRemovesNothingAndItsReturnRestoresEverything`,
  `TestScenario_S7_RemovalsPastTheThresholdWaitAndFewerGoAhead`): the node reports the root's
  device and mount facts; a changed device or mount, or a removal past D208 §3's threshold, is
  withheld and recorded in `root_state` with its reason. The admin's confirmation, which applies
  a withheld removal, comes with the scanner view.

### S7b — A disk unplugged inside a root, with /mnt as the root

- **Story:** the owner's root is `/mnt`, with disks mounted at `/mnt/disk1` and `/mnt/disk2`.
  `disk1` is unplugged; meanwhile a film on `disk2` is deleted.
- **Expected:** `disk1`'s films stay, unavailable; the deleted film goes.
- **Design:** the root's own facts do not change, and `/mnt/disk1` lists as an empty directory. The
  node reports each mount boundary it crosses and the core compares them with the last complete
  scan's (D208, as corrected; D42).
- **Status:** **holds** (`TestScenario_S7b_ADiskUnpluggedInsideARootRemovesNothingUnderIt`,
  `TestScenario_S7b_ADifferentDiskAtAMountPointHidesTheOldOnesFiles`,
  `TestScenario_S7b_AnEmptiedDirectoryThatWasNeverAMountIsRemovedAsUsual`). Asked by the owner
  after the first walk, which had missed it.

### S8 — The same disk plugged back in a week later

- **Story:** the disk comes back and a scan runs.
- **Expected:** every film as it was, with its history.
- **Design:** "recovery restores; it does not re-add" (D41).
- **Status:** **holds** (the same test). While it was away, `root_state` recorded the root's
  outcome; showing its films as unavailable (D179) is the scanner view's and browse's.

### S9 — A disk whose mount point is gone too

- **Story:** the directory itself is missing, or unreadable.
- **Expected:** nothing removed.
- **Status:** **holds** (a root error withholds every removal:
  `TestIngest_OnlyACompletedScanRemovesAndNeverWhereItCouldNotRead`).

### S10 — Deleting a film by mistake, and putting it back

- **Story:** the owner deletes a film in the file manager, a scan runs, and the next day they
  copy it back from their own backup to the same place.
- **Expected:** the film is back with its history: they watched it last month.
- **Design:** the scan tombstones it (correctly: it was gone). When the same payload reappears,
  the ingest looks only at live rows, so it makes a new row, and the history stays on the
  tombstone. D41 says a returning mount is restored, not re-added; it does not say what happens to
  a returning file.
- **Status:** **holds** (`TestScenario_S10_AFilmPutBackRevivesItsTombstone`, D208 §4).

### S11 — A copy of a film in a second folder

- **Story:** the owner copies a film to `To Watch/` and leaves the original.
- **Expected:** both play; the admin view shows them as duplicates, naming both paths (D179 §4).
- **Status:** **holds** (`TestIngest_ACopyIsADuplicateAndTheRecordedPathKeepsTheIdentity`); the
  admin view is M2's scanner view.

### S12 — A scan cut off by a reboot

- **Story:** the machine reboots mid-scan.
- **Expected:** nothing removed; the next scan finishes the job.
- **Status:** **holds** (the same test). A file moved within its root appears only after a scan
  completes, because a move is decided at completion; recorded, not a gap.

### S13 — A film still downloading during a scan

- **Story:** a scan reaches a film that is half copied.
- **Expected:** it appears once it is complete, and nothing odd happens to it later.
- **Design:** a truncated container is hashed whole; the complete file has a payload hash, so the
  next scan sees a different payload at the path and replaces the row.
- **Status:** **holds on paper**, with one row of churn. Harmless unless someone started playing
  the half-copied file; not worth a mechanism now.

### S14 — A wrong match fixed by hand, then the file renamed

- **Story:** identification matched the wrong *Heat*; the owner picks the right one; later they
  rename the file.
- **Expected:** the right *Heat*, still.
- **Design:** a confirmed match is permanent (D172); the file's row follows the rename (D179).
- **Status:** **holds** (`TestPass_ARenamedFileIsIdentifiedAgainUnlessConfirmed`: a confirmed
  match does not move).

### S15 — Arriving from Kodi with years of NFO files

- **Story:** the owner points a library at a Kodi library with `.nfo` files and locked fields.
- **Expected:** the locked fields come through as corrections; the rest of each NFO is the
  source where the library was told to use NFO; nobody's `playcount` is imported.
- **Design:** identification.md, Reading the sidecars (D140, D182, D183).
- **Status:** **holds** for NFO read (`TestParseNFO_AKodiMovieWithItsCorrectionsLocked`,
  `TestPass_TheDevLibrarysFilms`): the node sends NFO bytes as sidecars, locked fields become
  overrides, watch counts are ignored. A conflict with a later edit here is shown once overrides
  have their API.

### S16 — An NFO edited in Kodi after a correction here

- **Status:** **holds on paper** (identification.md: the later change wins and the conflict is
  shown).

### S17 — The database lost, with a backup

- **Story:** the disk holding `library.db` dies; the owner restores last night's backup and the
  server scans.
- **Expected:** everything as of last night; files added since appear as new.
- **Status:** **holds on paper**: restore is built (M1) and the ingest matches by path and
  payload. Becomes a scenario test.

### S18 — The database lost, with no backup

- **Expected:** the library rebuilds from the files; watch history and corrections not in NFO
  files are gone. The backup page said so ([backup-restore.md](backup-restore.md)).
- **Status:** **holds on paper**.

### S19 — Libraries built from symlinks, as Jellyfin users do

- **Story:** a migrating owner has *Kids* as a folder of symlinks into *Films*.
- **Expected:** it works; each film in both.
- **Design:** links into a media root are followed (D206); the same files seen through two roots
  are duplicates, and each library has its own Titles.
- **Status:** **holds** at file level; watch history is split between the two Titles, which is
  **Q2** again.

### S20 — A new season arrives

- **Status:** **holds** for a TMDB library (`TestSeries_ANewEpisodeFetchesItsSeasonAgain`): the
  series is matched once, each season's listing is fetched once, and a new episode missing from a
  listing fetched before it arrived fetches that listing again (D209 §3).

### S21 — A name only macOS writes

- **Story:** `Fièvre.mkv` copied from a Mac arrives decomposed.
- **Status:** **holds** (`TestIngest_APathIsStoredInNFCBesideItsBytes`).

## M3 — play in a browser (walk before M3)

- **S30** — The owner starts a film on the laptop, pauses at 1:12:00, and resumes on the phone in
  bed: it starts at 1:12:00.
- **S31** — Two devices report out of order; the newer position is never rewound (M3
  acceptance).
- **S32** — The owner resumes a film whose file was replaced by a better copy (S3): what do they
  see? The position does not carry (D173); is anything offered?
- **S33** — The owner hides a film from Continue Watching and takes it back.
- **S34** — The partner fetches Spanish subtitles for a film; later the owner renames the file
  (the subtitle follows the row), and later still replaces it (does the partner spend another
  download?).
- **S35** — Dialogue boost on the TV; `cache.db` deleted; the next play prepares it again.
- **S36** — An MKV with TrueHD and ASS plays in Chrome, Firefox and iPhone Safari with the reason
  on screen (M3 acceptance).

## M4 — two people, one remote (walk before M4)

- **S40** — The owner invites the partner; the partner signs in from home; the owner revokes
  access; the partner's next request fails.
- **S41** — Watch Together across two houses for a whole film: one pauses, the other's network
  drops for a minute, they rejoin and stay in sync.
- **S42** — A friend joins Watch Together without an account; their viewing is not recorded; the
  partner's is.

## M5 — the remote TV (walk before M5)

- **S50** — The partner watches a whole film on their TV with voice on the phone, and resumes it
  at home later from where they stopped.
- **S51** — "Get this ready" before a film night; it plays over a congested link without
  stalling.

## Running it (walk before M3)

- **S60** — An upgrade runs a migration: a snapshot is kept, `/health` says migrating, and a
  failure leaves the server up saying why (M1, holds).
- **S61** — A typo in the config file: doctor names it, and the server runs on the starting
  value.

## Gaps and questions

| | What | Scenarios |
| --- | --- | --- |
| **G1** | Built: the node reports each root's device id and mount facts; the core withholds removals past D208 §3 and records why. The admin's confirmation comes with the scanner view | S7, S8 |
| **G2** | Recorded: `root_state` keeps each root's last outcome; the scanner view and browse show a root's files as unavailable | S8 |
| **Q1** | Answered by D208 §1: re-identified from its new path unless a person confirmed the match | S1, S14 |
| **Q2** | Answered by D208 §2: a Title with no live files follows its files to the new library | S6, S19 |
| **Q3** | Answered by D208 §3: withhold past half and twenty, or all, or a changed device or mount | S7 |
| **Q4** | Answered by D208 §4: a returning payload revives its tombstone | S10 |
