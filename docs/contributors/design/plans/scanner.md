> **ABOUTME:** What a media node observes and publishes — a snapshot of its roots plus a
> journal of changes since — and how it detects that something changed. Runs in a media node,
> not in the core, and deliberately draws no conclusions. What the observations *mean* is
> [identification.md](identification.md).

# Scanner

- **Status:** PLANNED — designed 2026-09-17; relocated into the media node by [D33](../../decisions/working-notes.md); **Shape and identification corrected 2026-09-19 by [D59](../../decisions/working-notes.md)**.
- **Depends on:** library-model.md
- **Feeds:** [identification.md](identification.md)

## Where this runs

**Not in the core.** Per [D33](../../decisions/working-notes.md) the scanner is part of a
**media node** — the same binary in `node` mode, running wherever the files are, alongside
probing, subtitle extraction and thumbnailing. In the default single-machine install the core
supervises a co-located node as a child process, so this is invisible to the user.

The consequence for everything below: **the scanner is normally local to its files**, so native
filesystem watching is the usual case rather than the lucky one, and the network-share problem
only applies to a node deliberately pointed at a mount.

## Shape: a snapshot plus a journal

A node sees the directories and files under its roots and nothing else. No library, no user, no
history but its own observations, and no idea that the file in front of it is episode 4 of
anything. So what it publishes is a **temporal view of a subtree** — what is there now, and what
has changed since ([D59](../../decisions/working-notes.md)).

- **Snapshot** — a complete enumeration of the node's roots at a point in time. This is
  [D39](../../decisions/working-notes.md)'s report under its honest name, with every property
  intact: JSONL, header/entries/trailer, completeness signalled by `.partial` → atomic rename,
  and completeness **scoped per root and per errored subtree**.
- **Journal** — append-only observations since that snapshot: one line per observed change,
  same record grammar, same error and skip records with their scopes
  ([D40](../../decisions/working-notes.md)).

Both carry a **monotonic sequence**, and the snapshot names the sequence it corresponds to. The
core tracks what it last consumed; if the oldest journal line the node still holds is newer than
that, there is a gap and the core re-reads the snapshot.

**The snapshot is always a valid recovery point, so the node never needs to know what the core
has read** — no acknowledgement protocol, no node-side model of core state, nothing that would
undo [D42](../../decisions/working-notes.md). Compaction is therefore a purely local storage
choice: a maximum journal length or size, then a fresh snapshot and older pairs may be dropped.
The bound is a guess until measured; it belongs in
[performance-targets.md](performance-targets.md).

### The snapshot is also the only thing that corrects a missed event

This sets the interval, and it is easy to miss. The journal is exactly as good as the watcher
feeding it, and [D27](../../decisions/working-notes.md) established that watchers **fail
silently**. A journal cannot contain the event nobody delivered. A snapshot enumerates, so it
catches whatever the journal missed.

Compaction and correctness happen to want the same mechanism, but it is the correctness
requirement that sets the floor: **periodic full snapshots are mandatory even with a perfect
watcher**, because you cannot know the watcher is perfect.

### What the node reports, and what it refuses to conclude

| The node reports | The core decides |
| --- | --- |
| the filename, verbatim — NFC form and raw bytes ([D49](../../decisions/working-notes.md)) | what the filename *means* |
| size, mtime, and where cheap, a content hash | whether two files are the same work |
| probe output: container, streams, codecs, durations, disposition flags, track titles, attachments | which track is "the" audio track |
| a track's BCP 47 tag **beside** the raw `ger`/`deu` the container carried | how to resolve a conflict between them |
| `A gone` and `B appeared` | that it was a move |
| "the extension says `.mkv`, the probe says something else" | whether that is a mislabel, a corruption, or a deliberate swap |

**The extension is a hint about whether probing is worthwhile, never a conclusion about
content** — people misname things constantly, so the node looks inside. Probing stays here
because the core never opens a media file ([D26](../../decisions/working-notes.md)).

Two rules worth stating as rules:

- **Report the event you were given; never synthesise linkage.** Do not emit "moved" from a
  disappearance and an appearance — that needs hash or inode history the core holds. But when
  `inotify` hands over a `MOVED_FROM`/`MOVED_TO` pair sharing a cookie, the kernel linked them
  and passing the link along *is* reporting what you saw.
- **Node-local state may change cost, never content.** A probe cache keyed by path, size and
  mtime is necessary or every snapshot re-probes fifty thousand files. The boundary that keeps
  it a cache rather than judgement: **delete it and the node must report exactly the same
  thing, only slower.**

### Legibility, which the phases were for

"Logging around library scanning being pretty difficult to interpret... which for me tends to
be the trouble spot in these systems" is a direct Jellyfin complaint and still the target. It is
met by [D34](../../decisions/working-notes.md)'s status record and
[D38](../../decisions/working-notes.md)'s `state`/`task` pair rather than by a pipeline: named
activities survive as **progress labels**, not as stages that draw conclusions. The user-facing
question — "why is this file not in my library?" — is answered by the node's error and skip
records joined to the core's identification log, and it needs both halves.

## Track facts the node records

Observation only; the *derivation* of track kind is in
[identification.md](identification.md) ([D59](../../decisions/working-notes.md)).

- **The raw language tag exactly as the container carried it**, plus its BCP 47 normalisation
  alongside ([D24](../../decisions/working-notes.md)). ISO 639-2 has two competing 3-letter
  codes per language — `ger`/`deu`, `fre`/`fra`, `chi`/`zho`, `dut`/`nld` — and real containers
  use both inconsistently; FFmpeg's 695-entry `avlanguage.c` is the reference. Normalisation is
  lossless and context-free, so it belongs here; keeping the original is the same
  normalise-for-use-keep-the-original rule as [D49](../../decisions/working-notes.md)'s paths.
- **Disposition flags as found** — `FORCED`, `HEARING_IMPAIRED`, `COMMENT`, `DESCRIPTIONS`,
  `DUB`, `ORIGINAL` — and **the track title string, verbatim**. Real files set the flags wrongly
  or not at all, so the title is often the only signal; but reading "English (SDH)" or
  "Signs & Songs" and concluding a *kind* is a heuristic over text, and heuristics improve, so
  that inference happens in the core where it can be re-run without touching a filesystem.

## Watching: a strategy interface, not a mechanism

Researched — see [change-detection.md](../research/change-detection.md) and
[D27](../../decisions/working-notes.md). `inotify` does not work over SMB or NFS, has watch
limits a large library exhausts, and **fails silently**, which is why "the scanner did not
notice my new file" and "the scanner rescanned everything at 3 a.m." are the same bug.

So: a per-root strategy with an **explicit capability report**. Refined by
[D32](../../decisions/working-notes.md), which corrects the ordering — **build the inbound
endpoint first**, because being told beats every form of watching and Sonarr/Radarr already emit
import webhooks with paths. For this audience that is the expected path, not a fallback.

The ladder: **be told** (webhook/API/CLI) → native watching (local) → SMB `CHANGE_NOTIFY`
(deferred; no Go client implements it and an OS mount cannot expose it) → **directory-mtime
pruned walk** → full `stat` walk as the net → content hashing on demand. The NAS index agent
sits between native watching and the walk for those who will install it.

**The load-bearing corollary stands: periodic walking must be good enough on its own**, because
most installs will use it. Its throughput is a number in
[performance-targets.md](performance-targets.md). Three techniques make it achievable, and all
three belong in this plan:

- **Directory-mtime pruning**, which suits media libraries specifically because they are
  append-mostly — turning *O(all files)* into *O(changed directories)*. Defeated by `rsync -t`
  and by in-place edits, hence the periodic full walk behind it.
- **One round trip per directory, never a `stat` per file.** SMB2 and NFS carry attributes in
  the listing; `filepath.WalkDir` without a stray `Info()` call is the difference between
  seconds and minutes on a share.
- **Adaptive per-directory cadence**, because a downloads folder and a 2004 films folder do not
  deserve the same interval — plus a config field letting the user declare a library archival.

**Probe at setup and report measured facts**: write a temp file to test whether this share
updates directory mtimes at all, time a few hundred entries to estimate deep-scan duration, and
then surface **last successful deep scan and current staleness** — which is the question a
polling user actually has.

## Done when

A pathological real-world library enumerates correctly; an interrupted snapshot resumes without
the core inferring a deletion; a watcher that silently stops is caught by the next snapshot; a
node whose probe cache is deleted publishes a byte-identical snapshot; and the node's records
answer "what did you see, and what could you not read?" without a debug log.
