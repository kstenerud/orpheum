> **ABOUTME:** A proposal to amend D303 §1's M4: a library's files become titles in stages by
> weight (the listing, filing from the path, the small files beside it, the provider's match, the
> video's own read), each stage working in small batches of whole filing units and finding its
> work from stored state, so a first import shows its titles within seconds and fills in, rather
> than waiting for each video to be read and then filing the whole library in one write.

# Staged identification: trickle in the lightest data, then build out

*2026-10-16. For the owner's approval before anything is built. Read from the repository at
An earlier commit.*

## 1. What the owner asked

On the morning summary's note that a first import still holds the writer for its whole pass:

> *"A 'pass' involves (logically, not necessarily temporally) a few steps (scan files + stat
> them, interpret stat data - such as teasing out movie names + year etc, examine related small
> files, deep inspection of files), each getting heavier than the last. We should be able to
> trickle in the lightest data, then progressively build out the higher level data in small
> batches + jobs. We're aiming for eventual consistency (good enough for a nice UX), not a perfect
> view at every single moment."*

## 2. What happens today

A scan's work runs in this order:

| Step | Where | What it costs | How it is cut |
| --- | --- | --- | --- |
| 1. Listing: every entry's path, size, modified time, inode | the node lists, the core compares (`library/compare.go`) | one listing; one statement marks every unchanged file seen | the whole root |
| 2. Read: probe the video, hash its payload (D179), read the NFO and subtitles beside it | the node, in batches of one folder's videos (D240 §4, §8) | a few MB of seeks and an ffprobe per video: minutes to hours for a first import on a spinning disk | per folder batch |
| 3. Ingest: store the batch's files | the core (`library/ingest.go`) | small | per batch |
| 4. The pass: parse every path, group versions, extras, series and episodes, apply NFOs, resolve each title's values | the core (`identify/pass.go`), asked for after each batch that stored a new file | a snapshot read of the whole library (0.2 s at 5,000 films), then one write for everything new since the last pass (2.2 s if all 5,000 are new) | the whole library |
| 5. Match: search the provider for each title and series awaiting one | a job per title or series, queued by the first-matches sweep | the provider's rate | per title or series |

Two things follow.

- **A video is invisible until step 2 has read it.** A new video gets no row at the listing: it
  is kept in memory as a file to examine (`takeListed`), and the pass files only videos with a
  payload hash (`isVideo`). On a first import the library fills at the speed of the node's reads,
  although its titles' names were all known the moment the listing arrived.
- **Step 4 is the only step not cut into small pieces.** It reads the whole library each time and
  writes everything new in one transaction. During a slow import it runs after every batch, so
  each write is small but each read is the whole library's; after a fast import, or a pass that
  finds much to change, one write holds the writer for seconds.

## 3. The proposal

### 3.1 The stages

| Stage | Input | What it writes | Priority (D255) |
| --- | --- | --- | --- |
| **S1 Listing** | the listing | a row for every file listed, including a new video, which is *listed* and unread | as now |
| **S2 Filing from the path** | paths, sizes and what is stored | titles, versions, extras, series and episodes, named from the path | fast |
| **S3 The small files** | an NFO, `tvshow.nfo`, the images and subtitles beside a video, read by the node | the NFO's layer and locked fields; refiles the unit | fast |
| **S4 The match** | the provider's answer (step 5, unchanged) | the fetched layer; refiles the unit | as now |
| **S5 The video's read** | probe and payload hash (D179) | streams, duration, identity; the title becomes playable; a payload a tombstone holds brings back its history (D208 §4) | slow, unless a person asks (§3.6) |

S1 and S5 are today's steps 1 and 2 with the small files taken out of step 2. S2 is today's pass
with its work cut into units. S3 is new as a separate step: today the NFO is read with its video.

### 3.2 The unit of work: a filing unit

A **filing unit** is the set of files whose filing depends on each other, and on nothing outside
the set except stored rows found by an indexed lookup:

- **in a film library**, a title's folder (its videos, which may be versions of one film, its
  extras folders and its sidecars), or, for loose files at a root, the files sharing one parsed
  name and year, with their sidecars;
- **in a series library**, one series: every folder whose name folds to the same work
  (`textindex.SameWork`), with its seasons and episodes.

What crosses a unit is already a lookup: a series spelled another way finds its series row by its
folded name; a title that followed its files to another library is found by provider id (D208
§2); a deleted library's title comes back by provider id (D231 §7). Inside a write, each lookup is
made again and the identity's unique constraint holds (M1, M3), so two units never both create
one series.

A unit's derivation is the pass's code run over one unit instead of the library: it reads the
unit's rows, decides, compares, and writes only what differs (DL9). The snapshot (`snapshot.go`)
becomes a unit's rather than a library's.

### 3.3 Batches and the writer budget

A stage takes the units it owes in order and writes them in batches: as many whole units as fit a
budget, measured in statements written (a stand-in for time that a test can hold), with a ceiling
near 25 ms of writer hold on the reference machine. A unit never splits across writes, so
nobody sees a film with half its versions (M4). A first import of 5,000 films becomes a few
hundred writes of a few milliseconds each, and a person's write waits for one of them at most.

### 3.4 How each stage finds its work

Each stage's work is derived from stored state, as every sweep's is (D231 §3), so a stop loses
nothing:

| Stage | Owes work for | Found by |
| --- | --- | --- |
| S2 | a unit with a listed video not yet filed, or a file moved, renamed or removed | the filing column, indexed on its unfiled value |
| S3 | a unit with a sidecar listed and not read, or changed | the sidecar's row, unread |
| S4 | a title or series never answered for | as now (`ListTitlesAwaitingMatch`) |
| S5 | a video with no payload hash, or changed | the file's row, unread |

A change in a later stage marks its unit as owing S2 again, in memory (DL14, as the owed passes
are now). A stop loses only those marks, so the backstop stays: the **whole-library comparison**,
hourly and at start, reads each library on a reader, derives every unit, and writes, in batches,
only the units that differ. This is DL13 as agreed this morning: compare before writing, with the
whole-library run as the backstop.

### 3.5 What a person sees

Within seconds of the listing, every title is in the library under the name its path gives,
with its year. Over the next minutes the names, posters and overviews change as the NFOs and the
matches arrive. A title whose video is not yet read shows that it is still being read, and cannot
play yet. Eventual consistency means only this: a page can show a title one stage behind
another, never half a title.

### 3.6 Play asks for the read

A person who plays a title whose video is unread asks for its S5 at a person's priority (D255): the
node reads that video before anything else waiting, a few seconds on most disks. The read that
would have come in path order comes at once for the film someone wants.

## 4. What it changes

- **D240 §3:** a new video is stored at the listing, unread, instead of being kept in memory until
  its read. **§4 and §8:** a folder's small files are read in their own fast command, before its
  videos.
- **D179:** unchanged. A file's identity is still its payload hash. A listed file has no identity
  yet. Until its read, it is known only by its path and inode.
- **D303 §1, M4:** "a chunked job resumes" becomes the stages above. **DL12:** filing (S2) and
  resolving a title's values are already separate derivations; S2's unit is the filing unit.
  **DL13:** amended as agreed. **DL14:** the owed marks become per unit, not per library.
- **D208, D231 §7:** a moved file, a returning payload and a title coming back are told by the
  payload hash, so they are told at S5. Until then a moved file at a new path files as a new
  title. When its read shows the payload a tombstone holds, the title it brings back absorbs the
  new one and its history returns. A person's correction made in the meantime is kept, as the
  latest edit (M13).

## 5. Risks, and what guards each

- **A moved file appears twice until its read.** For a folder rename, the listing's inode tells
  the move, and D240 §3's move by inode, deferred by §8, stops the duplicate. I propose building
  it now. It is the cheap read §8 deferred.
- **A person acts on a title that is later absorbed.** Their watchlist entry or rating moves with
  the title, under a test that absorbs a title a person has rated.
- **A unit too large for the budget**, such as a series of 2,000 episodes, is still one write.
  Its cost is bounded by its own size, not the library's, and the metrics name it.
- **The order a person sees things in.** Filing runs in path order. Reads run in path order too,
  which is what a spinning disk wants (D240 §5). Matching runs in the order filed.

Tests: the cost test becomes per stage (a batch writes the same statements however large the
library), and the differential test compares a staged import with today's pass, step by step,
until they hold the same tables. Each stage also gets a test that a stop between any two batches
loses nothing and duplicates nothing.

## 6. Questions for the owner

1. **Titles before they can play** (§3.5): show every title from the listing, marked as not yet
   read and unplayable, with Play asking for its read at once (§3.6)? Or keep a title hidden until
   its video is read, which keeps today's view but loses most of the gain?
2. **Moves by inode** (§5): build D240 §3's deferred move by inode with this, so a renamed folder
   does not show its films twice until they are read?

## 7. Order of building

1. S2 by unit: the pass's derivation run per unit, batched within the budget, with the cost and
   differential tests. That alone ends the long write, whatever the answers above.
2. The backstop as a reader's comparison writing units in batches; owed marks per unit.
3. S1 storing unread videos and S5 bringing back what a payload holds (question 1).
4. S3 as its own fast read.
5. The move by inode (question 2).
6. Play asking for the read.
