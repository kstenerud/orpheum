> **ABOUTME:** The build plan for D247: a slow link gets lower qualities, each cut into chunks at
> the source's keyframes and made when a player first asks for one, which hls.js switches between
> by itself. What each layer holds, the wire, and the order it is built in.

# Chunks made on request

- **Status:** BUILDING, from 2026-10-05. [D247](../../decisions/working-notes.md) is the decision;
  [slow-links.md](../research/slow-links.md) §10 is the measured recipe; this is the mechanism.
  It is [transcoding.md](transcoding.md) §(b), built for Cut A's two people.
- **Replaces** nothing yet: direct play and prepare stay as they are. A film starts as it does
  today, from the original file or a prepared rendition; this machinery starts at a player's first
  stall (D247 §2).

## 1. The words

- **The grid** is a film's chunk boundaries, in its own time: the source picture's keyframes,
  merged greedily so each chunk is at least `hls_segment_length_seconds` long. Chunk *n* runs from
  `grid[n]` to `grid[n+1]`; the last ends at the film's end.
- **A stream** is one thing cut on the grid: the **copied picture** (the original's, unchanged),
  a **quality** (the picture encoded to H.264 at one of the `chunk_qualities`), or a **sound** (one
  audio track's standard or boosted stereo mix, as prepare makes them).
- **A store** is one stream of one film on one grid, in the node's prepared directory:
  `<file_id>/chunks-<stream>/<grid_id>/`, holding `init.mp4` and `<n>.m4s`, five digits or more. `grid_id` is a hash
  of the grid, so a changed grid is a new store.
- **A run** is one FFmpeg making chunks *first*…*last* of one store, in order. Runs of a store do
  not overlap: a run stops where the next made chunk begins.

## 2. The node

- **`read_through`** (measure_loudness, made general): one FFmpeg run reads the file once and
  gathers what the core asks for: each audio track's loudness, as before (D226), and the picture's
  keyframe times and bytes per second of film, from a `framecrc` output of the copied picture,
  whose lines without `F=` are keyframes. The owner asked for one path that reads a file through
  with different things gathered (2026-10-05). The bytes per second are the film's bit-rate
  profile: its peaks, by which a link is sized (slow-links.md §3).
- **`make_chunks`** (a new command): the source by identity (D100), the stream's recipe, the grid's
  times from `grid[first]` to `grid[last+1]`, the first chunk's number, and the store's directory.
  The node runs FFmpeg's `segment` muxer (slow-links.md §10 has the arguments), and as each raw
  file is complete (the next has begun, or FFmpeg has ended) it writes the store's `init.mp4` if
  there is none, and `<n>.m4s`: the file's `moof`+`mdat` pairs with every `tfdt` set to the film's
  time. A sound run starts one chunk early and drops it (its encoder's priming); its chunks' times
  come from its sample counts. Progress is the last chunk made; the end is done, failed, refused or
  stalled, as a prepare's.
- **The byte socket** serves a store's files at `/bytes/v1/chunks/<file_id>/chunks-<stream>/<grid_id>/<name>`,
  and answers 404 for a chunk not made yet. A chunk is written whole, in one write, after it is
  complete, so no reader sees part of one.
- **The prepared directory's budget** counts stores as it counts renditions: a store is shelved
  with a manifest naming its film, stream and grid, marked being written while a run writes it,
  and evicted least recently served first.

## 3. The core

- **The keyframes** are kept in `persistent.db` beside the loudness, by payload hash
  (`video_timeline`; making them again reads the whole file, F1), made by the `read_through` sweep
  in the background as loudness is, so a film's grid is there before anyone stalls; a film without
  one is read through at play-now priority at a player's first stall.
- **The qualities** are the setting `chunk_qualities`, a list such as
  `3000k@720p,2000k@540p,1200k@480p,700k@432p,400k@360p` (D247 §6): each a bit rate and a picture
  height, highest first. A quality's peak is capped at 1.5× its bit rate. The sound is stereo AAC
  at the setting `chunk_sound_kbps` (96), every picture's playlist bandwidth counting it.
- **The master playlist**, at `/bytes/media/chunks/<token>/master.m3u8`, lists the copied picture
  when the client plays the source's codec, then every quality below the source's own bit rate,
  each with its sound group: every audio track's standard and boosted mixes, as prepare carries
  them (D217 §3). Each media playlist is VOD, every chunk from the grid, `#EXT-X-ENDLIST`.
- **A chunk request** is relayed to the node. When the node has not made it, the core finds a run
  of that store that will reach it soon (its last chunk made is before it, and within about 60 s
  of film), or queues one from it at play-now priority, and answers 425 with `Retry-After` at once
  (D250 §5): a player counts a held request as the chunk's download, and took its link for slower.
  A hold, asking the node again each quarter second, is the setting `chunk_hold_ms` (0); the
  player asks again every second or two for over a minute and a half.
- **A seek starts a run where it lands; catching up never does** (D250 §5). A request more than
  `chunk_new_run_beyond_seconds` (90) of film past the chunk a run of its store makes next was
  sought to, since a player catching up jumps at most a minute ahead: that run is cancelled, no one
  following it now, and a run is queued from the request. A request nearer waits for the run.
- **Every run stays faster than playback** (D250 §6). The core measures each run's speed from its
  progress, over 20 s. When one of a node's two or more runs falls below
  `chunk_run_slowest_speed_percent` (110) of playback, it evicts one: the oldest no player has
  asked for in 30 s, or, when every one is in use, the newest. Its job is cancelled, and its store
  answers its players 429, `no_room_to_make`, which the player does not retry and words as the
  reason its playback stops, until a stall asks for that quality again. A node whose encode slots
  hold its runs one at a time rarely meets this: its runs wait for a slot instead.
- **A run is cancelled** when no player has asked for its store's chunks for a while and it is
  far ahead of the last asked: nobody is coming.

## 4. The player

- **First stall** (D247 §2): `POST /playback/<token>/stalled` with the position and the link's
  measured speed (`link.ts`). The core makes the grid if it has none, chooses the highest quality
  within 70% of the measured speed (D247 §6), or one that another viewer's stall already started
  if it is that one or the one below (D247 §7), and starts its runs and its sound's at the
  position. It answers the master playlist and the quality to start at.
- **Second stall**: the player moves to the master playlist, once, at its position, starting at
  that quality (hls.js `startLevel`). From then hls.js steps between qualities on its own
  measurement of each chunk's download.
- **Stepping up** (D247 §9): hls.js is configured to step up only with about 1.5× the next
  quality's rate (`abrBandWidthUpFactor`), measured over minutes rather than seconds (its slower
  moving average), and one quality at a time.
- **The bottom** (D247 §8): a stall at the lowest quality, or a measured speed below it, tells the
  person it will not work, with both numbers; in Watch Together the group is told too.
- **The quality playing** is shown (D247 §3).

## 5. The order it is built in

1. The grid: `read_through` on the node and in the core, its keyframes kept, and the grid.
   *Built 2026-10-05: `internal/node/readthrough`, `internal/core/readthrough`, `video_timeline`
   in persistent.db (step 0035), `internal/core/chunks.GridOf`.*
2. `make_chunks` on the node, with the byte route and the budget.
   *Built 2026-10-05: `internal/node/chunks`, `prepared.Shelf`'s stores, the byte route. Every
   chunk's time is its time in the film plus `chunks.TimelineOffsetMs`, 10 s: a sound's priming
   comes before the film's start and a picture's first decode before its keyframe is shown, and a
   chunk's time cannot be negative.*
3. The core's playlists, chunk requests and runs, and the `chunk_qualities` setting.
   *Built 2026-10-05: `internal/core/chunks` (`Stream`, named `picture`, `q<kbps>k<height>p` and
   `sound<track>-<kbps>k` or `sound<track>-boost-<kbps>k`, so a changed `chunk_sound_kbps` makes a
   store of its own; `Grid.ID`, the first 16 hex characters of the SHA-256 of
   the grid; `StoreDir`; `MasterPlaylist`, `MediaPlaylist`, `StoreFileOf`; `Ensure`, with
   `lookaheadMs` 60 s and `runSpanMs` 5 min; the `make_chunks` job, `HandleMakeChunks` and its
   `Assignee`), `internal/core/chunks/qualities` (`Parse`, the setting's one check and reading),
   `playback.Service.ChunksOf`, and the routes `GET /bytes/media/chunks/{token}/master.m3u8` and
   `GET /bytes/media/chunks/{token}/{streamName}/{fileName}`, where `fileName` is `stream.m3u8`,
   `init.mp4` or `NNNNN.m4s`. A run is not yet cancelled when nobody is coming (§3's last bullet).
   A run's job targets its store, a `chunk_store` row in cache.db, so the runs of a film's streams
   queue side by side; a run asked for while another of its store waits queued (a seek) is folded
   into that one, and asked for again at the player's next look, every quarter second.*
4. The player's first and second stall, the move to the master playlist, and the quality shown.
5. Stepping up, the bottom and its words, and Watch Together's word to the group.

## 6. Next: cut points without reading the whole film (agreed 2026-10-06)

The grid came from reading the whole file through (§2), minutes over a network mount, and a film
whose read-through failed could never be offered a lower quality. The owner agreed to change it,
in this order, each a revision of D247 §10 written before it is built:

1. **A local rule for the grid.** A chunk starts at the first keyframe at or after each multiple
   of `hls_segment_length_seconds`, so each boundary depends only on the keyframes near it: any
   stretch of the film can be cut on its own and agrees with the whole film's grid. The greedy
   merge from the film's start does not.
2. **The keyframes from the file's own index**: an MP4's sync-sample table, a Matroska file's
   cues, read in a fraction of a second; the read-through stays the fallback. A sound's chunks do
   not wait for its loudness: unlevelled until it is measured.
3. **Read from the stall for a file with no index** (AVI, MPEG-TS, Matroska without cues): the
   node reads a few minutes ahead of the player's position, saying each keyframe as it reads, and
   the playlists grow from there. **Required before a public release** (the owner, 2026-10-06):
   archivers keep these older formats, and they are the likely customers.
