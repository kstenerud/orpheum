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
  `<file_id>/chunks/<stream>/<grid_id>/`, holding `init.mp4` and `<n>.m4s`. `grid_id` is a hash
  of the grid, so a changed grid is a new store.
- **A run** is one FFmpeg making chunks *first*…*last* of one store, in order. Runs of a store do
  not overlap: a run stops where the next made chunk begins.

## 2. The node

- **`scan_keyframes`** (a new command): the picture's keyframe times, from
  `ffprobe -select_streams v:<i> -show_entries packet=pts_time,flags`, as milliseconds. About 5 s
  for a 96-minute MP4 here (§10); a slow disk or a large MKV takes longer.
- **`make_chunks`** (a new command): the source by identity (D100), the stream's recipe, the grid's
  times from `grid[first]` to `grid[last+1]`, the first chunk's number, and the store's directory.
  The node runs FFmpeg's `segment` muxer (slow-links.md §10 has the arguments), and as each raw
  file is complete (the next has begun, or FFmpeg has ended) it writes the store's `init.mp4` if
  there is none, and `<n>.m4s`: the file's `moof`+`mdat` pairs with every `tfdt` set to the film's
  time. A sound run starts one chunk early and drops it (its encoder's priming); its chunks' times
  come from its sample counts. Progress is the last chunk made; the end is done, failed, refused or
  stalled, as a prepare's.
- **The byte socket** serves a store's files at `/bytes/v1/chunks/<file_id>/<stream>/<grid_id>/<name>`,
  and answers 404 for a chunk not made yet. A chunk is written whole, in one write, after it is
  complete, so no reader sees part of one.
- **The prepared directory's budget** counts stores as it counts renditions: a store is shelved
  with a manifest naming its film, stream and grid, marked being written while a run writes it,
  and evicted least recently served first.

## 3. The core

- **The grid** is kept in `cache.db` (`keyframe_grid`: the file, its payload hash, the grid as
  JSON, when it was made), made by a `scan_keyframes` job at play-now priority when a player first
  stalls on the film, and made again when the file's payload changes.
- **The qualities** are the setting `chunk_qualities`, a list such as
  `3000k@720p,2000k@540p,1200k@480p,700k@432p` (D247 §6): each a bit rate and a picture height,
  highest first. A quality's peak is capped at 1.5× its bit rate.
- **The master playlist**, at `/bytes/media/chunks/<token>/master.m3u8`, lists the copied picture
  when the client plays the source's codec, then every quality below the source's own bit rate,
  each with its sound group: every audio track's standard and boosted mixes, as prepare carries
  them (D217 §3). Each media playlist is VOD, every chunk from the grid, `#EXT-X-ENDLIST`.
- **A chunk request** is relayed to the node. When the node has not made it, the core finds a run
  of that store that will reach it soon (its last chunk made is before it, and within about 60 s
  of film), or queues one from it at play-now priority, and waits for the chunk, asking the node
  again each quarter second, for up to 20 s; then 425 with `Retry-After`.
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

1. The grid: `scan_keyframes` on the node, the job and `keyframe_grid` in the core.
2. `make_chunks` on the node, with the byte route and the budget.
3. The core's playlists, chunk requests and runs, and the `chunk_qualities` setting.
4. The player's first and second stall, the move to the master playlist, and the quality shown.
5. Stepping up, the bottom and its words, and Watch Together's word to the group.
