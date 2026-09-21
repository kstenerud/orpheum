> **ABOUTME:** What a media node observes and publishes — a snapshot of its roots plus a
> journal of changes since — and how it detects that something changed. Runs in a media node,
> not in the core, and deliberately draws no conclusions. What the observations *mean* is
> [identification.md](identification.md).

# Scanner

- **Status:** PLANNED — designed 2026-09-17; relocated into the media node by [D33](../../decisions/working-notes.md); **shape and identification corrected 2026-09-19 by [D59](../../decisions/working-notes.md)**; spin-down and two extra observation passes added the same day ([D94](../../decisions/working-notes.md), [D83](../../decisions/working-notes.md), [D102](../../decisions/working-notes.md)); **the pipeline measured and made steerable 2026-09-20** ([D114](../../decisions/working-notes.md), [D116](../../decisions/working-notes.md), [D118](../../decisions/working-notes.md)).
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
undo [D42](../../decisions/working-notes.md).

Retention is therefore a purely local storage choice, but **taking a snapshot and discarding
journal are separate operations with opposite triggers** ([D60](../../decisions/working-notes.md)):

| Operation | Trigger |
| --- | --- |
| take a snapshot | **temporal** — every `snapshot_interval`, or on a `snapshot` command |
| discard journal | superseded by a snapshot **and** older than `journal_retention` |
| emergency discard | `journal_max_bytes` exceeded — a backstop, and **loud** |

Size is the wrong primary trigger for a reason beyond "the core might be behind", though it
might well be — a core switched off for the weekend plus a torrent client finishing a season
pack is enough. The sharper reason is *when* it fires: **the burst that blows a size bound is,
almost by definition, the anomalous event** — a mass deletion, a re-organisation, a share
returning with different contents. Size-triggered compaction forces a full re-enumeration at
the worst possible moment and replaces the ordered record of what happened with one reading
taken mid-disruption. It is the backup that runs during the incident. Temporal triggering
decorrelates the measurement from the events being measured.

The economics agree: **a quiet library produces almost no journal**, so temporal retention is
nearly free in the common case and only costs during churn — which is exactly when the history
is worth keeping. A size bound is most restrictive precisely when there is most to say.

What this buys is diagnosis, cost and not-measuring-during-an-anomaly — **not** library
correctness, which the snapshot already guarantees. And pure-temporal has its own failure mode,
which is why the valve exists: a rename loop fills the node's disk, and a node that cannot write
cannot write its status record either, so it stops looking wedged and starts looking dead. When
the valve trips: snapshot first, then trim, and report what was discarded.

The numbers are guesses until measured; they belong in
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
| size, mtime, and a **sampling** content hash — head, tail and size, measured at 3.2 ms against 0.49 s for the whole file ([D116](../../decisions/working-notes.md)) | whether two files are the same work |
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

## Spinning disks are a cost the interval was not priced against

Added by [D94](../../decisions/working-notes.md). Jellyfin issue #7689 is *Reduce hard disk
access during playback*, and the comparison inside it is that Plex has nearly no disk activity
where Jellyfin has a lot. **That comparison is wrong** — Plex's own forum carries *Plex is
constantly spinning HDD up and down* (22 posts), *HDD never turns off while PMS is running* and
*Plex wakes up Synology NAS every hour*. Both do it, which makes it a property of the workload
rather than of one implementation. For a NAS audience — and
[test-hardware.md](../research/test-hardware.md) names an 8 GB Celeron NAS as the honest low
end — spinning drives cost power, noise and wear, and "it keeps my array awake" is the kind of
complaint that ends in an uninstall.

`snapshot_interval` was justified above against two pressures: a silently-dead watcher wants it
short, an expensive SMB enumeration wants it long. **There is a third — the interval is how
often the disks must wake** — and it argues the number should be **per root and derived from
the setup probe**, not a global constant. An SSD-backed root and a spun-down array want
different answers.

Two rules:

- **Batch the wake.** All roots on one physical device enumerate together, so one pass spins the
  array up once. The adaptive per-directory cadence above must not be allowed to become a
  continuous trickle of small wake-ups — that is strictly worse than one long pass, and it is
  the failure mode adaptivity naturally produces.
- **`archival` means untouched.** The config field already exists in this plan; its meaning is
  now concrete: no walk, no probe, no thumbnailing, no loudness pass. The journal only, if a
  watcher is available. And the node **reports that it is honouring it**
  ([D73](../../decisions/working-notes.md)), so "why is my new file missing" has an answer
  instead of a mystery.

## The node's work is a pipeline, and it now has nine passes

[D102](../../decisions/working-notes.md). [D33](../../decisions/working-notes.md) gave the node
"everything requiring access to a media file", and at the time that was five things. Counting
what has accumulated: enumerate · probe · content hash · subtitle extraction · font subsetting
([D55](../../decisions/working-notes.md)) · thumbnails, trickplay and poster derivation
([D99](../../decisions/working-notes.md)) · loudness ([D83](../../decisions/working-notes.md)) ·
segment detection ([D82](../../decisions/working-notes.md)) · overlay rendering
([D78](../../decisions/working-notes.md)) — and later content-safety inspection
([D61](../../decisions/working-notes.md)).

Each was added in isolation and each was individually cheap. **Measured 2026-09-20**
([D114](../../decisions/working-notes.md)) the picture is sharper than "nine expensive passes":
they are **two populations a thousand-fold apart**, split by whether a pass decodes video.
Probe, remux, keyframe grid, thumbnail, fingerprint and subtitle overlay run at 500–38,000×
realtime; trickplay and blackdetect at ~14×. Over a 3,500-item reference library that is
**17 minutes to a browsable library**, about a day and a half for the whole cheap set, and
**9.2 single-core days for trickplay alone**. So:

- **Each pass is independently deferrable, per library.** [D94](../../decisions/working-notes.md)'s
  `archival` is the extreme setting of a control that wants every gradation — a downloads folder
  wants everything now, a 2004 films folder wants enumeration and little else until something
  plays.
- **Each pass is separately reported.** [D38](../../decisions/working-notes.md)'s `task` is
  already a bounded enum the core baselines against, so this is that enum growing — but the
  user-facing question changes from *"is it scanning?"* to *"which passes have finished on this
  library?"*, which is what the legibility complaint above is actually asking for.
- **Each pass is restartable and idempotent**, because a NAS gets switched off mid-pass.
- **Ordering is by user value.** Enumeration and probe make the library exist; everything after
  is enrichment, and a user who can watch a film in five minutes will forgive a trickplay
  sprite that arrives on Tuesday — which is the literal measured trade: 17 minutes against 9.2
  days.
- **Passes that share a decode may be fused**, and the scheduler has to know which. Measured:
  running loudness, trickplay and blackdetect as one invocation beats three separate runs by
  **38% on six cores and 45% on one** — the saving is larger on the machine that needs it more.

### Playability first, and the user can jump the queue

[D118](../../decisions/working-notes.md). The ordering rule is not "cheap passes first", it is
**an item becomes playable as early as possible**; everything that only adds a feature to
something that already plays comes after everything that makes something play. For one
110-minute film, single-core, from the measured rates: **playable in under a second**,
subtitles and overlays at ~2 s, loudness at ~65 s, **trickplay at ~8 minutes**.

**And the scheduler is a priority queue keyed by `(item, pass)`, not a pipeline**, because the
user gets to say *"I want to watch this one now"* in the middle of a fifty-thousand-file scan.
A boost preempts at the next pass boundary.

Two things that are easy to build wrong:

- **The core expresses the request in the node's vocabulary.** A node has no items
  ([D42](../../decisions/working-notes.md)) — it has paths. So the core translates the title
  into a set of paths and sends those, which is
  [D101](../../decisions/working-notes.md)'s rule again.
- **It joins [D38](../../decisions/working-notes.md)'s command list** as
  `prioritise <paths>`, and satisfies that decision's re-execution rule trivially: prioritising
  something already done is a no-op.

The UI says what it can actually deliver — *playable now, scrub previews in eight minutes* is
true and useful; *Preparing…* is not.

### The core reads the report while it is still being written

[D118](../../decisions/working-notes.md) **changes a rule**:
[D39](../../decisions/working-notes.md)'s *"the core never parses a file still being written"*
becomes **"the core may tail a `.partial` report, and everything it reads from one is
incomplete by definition"**. Results appear in the library as they are produced rather than
when the scan ends.

Nothing about safety changes, because safety was never the filename — it was *unknown means
incomplete means add-or-update only*, which already covered a crashed scan and covers a
partial read identically. The atomic rename keeps its one job: it is the moment **removals may
be applied**.

Two mechanical details that bite:

- **This is a local-filesystem feature** ([D127](../../decisions/working-notes.md)). Over NFS
  the core's view of a growing file's length is served from the client attribute cache —
  `acregmax` defaults to **60 s** and *doubles towards the maximum while nothing appears to
  change* — so "items appear as they are found" becomes "items appear in sixty-second lumps",
  and the atomic rename itself can be invisible to the core for as long again. The transport
  that gives this away for nothing is HTTP, where the core parses the body as it arrives:
  measured, **first record at 0.1 ms** against 551 ms waiting for a rename.
- **Consume only up to the last newline.** A tailed JSONL file ends mid-record about half the
  time; this is precisely the "degrades per line" property the format was chosen for.
- **Batch the ingest.** Parsing a 50,000-entry snapshot costs 1.8 s and the write ceiling is
  9,415 commits/s, so tailing must not turn one scan into fifty thousand transactions. Batch on
  a time or count bound — the user still sees results inside a second.

**This is also what makes the scan legible**, which was the original complaint. A progress bar
is a promise about the future; items appearing in the library are evidence about the present.

**These are passes over bytes, not stages that draw conclusions** — the shape
[D59](../../decisions/working-notes.md) removed stays removed.

## Two more observation passes

Both are node work, both are [D42](../../decisions/working-notes.md) facts rather than
judgements, and both are things the node is uniquely placed to do because it already has the
file open.

- **Loudness** ([D83](../../decisions/working-notes.md),
  [audio-presentation.md](audio-presentation.md)). Integrated loudness, loudness range and true
  peak per audio track, via FFmpeg's `ebur128`. It is an extra decode pass, which is free on a
  desktop and is not free on a Celeron with fifty thousand files — so it is interruptible,
  resumable and reported like any other progress, and whether it can be deferred per library is
  a [performance-targets.md](performance-targets.md) question.
- **Segment candidates** ([D82](../../decisions/working-notes.md),
  [media-segments.md](media-segments.md)). Chapter marks are already in the probe output and
  cost nothing. Audio-fingerprint and black-frame detection across a season are a real decode
  cost and are a later, plugin-shaped job — the node **reports candidate boundaries**, the core
  decides what to store. And **the core supplies the set of files to compare**
  ([D101](../../decisions/working-notes.md)): "these twelve files are one season" is library
  knowledge the node does not have and must not reconstruct, or it has re-implemented the
  filename parser [D59](../../decisions/working-notes.md) moved out.

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
node whose probe cache is deleted publishes a byte-identical snapshot; a library declared
archival is not touched at all and says so; a scan of one array spins it up once rather than
repeatedly; **items appear in the library while the first scan is still running**; **a title
the user asks for becomes playable within seconds of asking, mid-scan**; and the node's records
answer "what did you see, and what could you not read?" without a debug log.
