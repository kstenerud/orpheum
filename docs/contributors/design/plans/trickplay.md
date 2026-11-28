> **ABOUTME:** The plan for scrub-bar thumbnails (trickplay), B1's phase F: the pictures and their
> sheets, how the node makes them at idle priority from keyframes, the core's job, sweep and
> storage, interest's raise, the API and the scrub bar, the phases that build it, and the owner's
> questions.

# Trickplay: thumbnails on the scrub bar

- **Status:** AGREED — written 2026-11-21 by the agent for [b1.md](b1.md) phase F, from the
  owner's answer to question 17 the same day: *every file, at idle priority, with a bump on
  interest*. The owner answered its five questions the same day (at the end). F1 is built (the
  node, node protocol 74, 2026-11-27), F2 too (the core), and F3 (the API and the scrub bar, node
  protocol 76, cache schema 57, VQ423, 2026-11-27). F4's walk was done on the lead's own devices on
  staging the same day (its findings under §6); the owner's run of VQ423 is what is left.
- **Depends on:** [b1.md](b1.md), [work-scheduling.md](work-scheduling.md) (D329),
  [artwork.md](artwork.md) (D212)

## What it ends with

A person dragging or hovering over the scrub bar sees a picture of the film at that point, with
its time, on the first play of any file whose pictures are made. The node makes them for every
video in the background at the lowest CPU priority there is, so a playback's encode never waits for
them, and a film someone looks at or plays has its pictures made before the rest. A film whose
pictures are not made yet shows the time alone, as today.

## What it rests on

- **D102, D114:** trickplay is the one costly pass, since a picture of every 10 s read by decoding
  every frame costs a full decode: 14.3× realtime on one core, 9.2 days for the reference library
  of 3,167 hours. It is deferrable by default, and its absence costs a preview, not a feature.
- **D329** ([work-scheduling.md](work-scheduling.md) §3.1, §6, §7.2, §9): trick-play is a
  Background kind, cut into pieces of a time range, saved between them; Background CPU work yields
  when an encode's lead falls.
- **D324** ([interest](../../../../internal/core/interest/doc.go)): a page shown or a film played
  raises its work in the queue, and what anyone watched stays at the shown-interest rung. That is
  the bump: a trickplay job is raised like any other of the film's jobs.
- **D212, D33, D193 §1:** the node decodes every image and writes our own WebP; the core never
  decodes one, keeps the bytes in its cache directory under a ceiling, and remakes what it evicts.
- **D362:** the node already takes stills from a video's frames (`internal/node/artwork/still.go`),
  which is the nearest code to start from.

## 1. The pictures

- **One every 10 s, 320 px wide**, the height by the picture's shape (180 for 16:9), as question 17
  proposed. The interval and the width are one *trickplay version*, so a later change remakes
  them by the sweep and asks nothing of a migration.
- **In sheets of 10 × 10:** a sheet holds 100 pictures, 16 min 40 s of film; a two-hour film is
  eight sheets. One request fetches a hundred pictures, and a sheet is the unit made, stored and
  served.
- **WebP**, our own encoder's (D212), at quality 60, which phase F0 measured (§2):
  1 to 3 kB a picture, so about 1.5 MB for a two-hour film and about 2.5 GB for the reference
  library. Rebuildable, so in the cache and not backed up.

## 2. The cost: keyframes, not every frame

The 9.2 days are for decoding every frame. A picture every 10 s needs one frame in 240, and
FFmpeg can decode the keyframes alone (`-skip_frame nokey`), skipping the rest without decoding
them. A film's keyframes are usually 2 to 10 s apart, so the picture for each 10 s mark is the
keyframe at or before it, at most one keyframe gap early. The node already knows each film's
keyframes from its index (D259 §2), so it knows how far off each picture is.

That should cut the cost by tens of times. This is the expectation, not yet measured: Jellyfin
offers the same as a setting ("keyframes only") for the same reason. **Phase F0 measures it** on
staging's library before anything else is built: ×realtime on the NUC7 and on ainas's Celeron,
software decoding, for H.264, HEVC and an MPEG-2 disc rip, against the full decode, and how far
off the pictures are.

**Measured 2026-11-21** on staging's NUC7 (i5-7260U), in the node container with the pinned
FFmpeg, one thread, at `nice 19`, ten minutes of film from ten minutes in, a picture every 10 s at
320 px, WebP at quality 60:

| Video | Every frame | Keyframes alone | Keyframe gap, mean and longest | A picture |
| --- | --- | --- | --- | --- |
| H.264 1080p | 5× realtime | **169×** (34 times faster) | 3.5 s, 5.0 s | 2 kB |
| HEVC 1920 × 800 | 4× | **118×** (29 times faster) | 2.5 s, 15.3 s | 3 kB |
| HEVC 1920 × 800, another file | — | **273×** | 7.5 s, 41.7 s | 1 kB |

So the reference library's 3,167 hours take about **a day of one core** from keyframes, against
about a month from every frame on this machine (D114's 9.2 days were on a faster one). A picture
is 1 to 3 kB: about 1.5 MB for a two-hour film and about 2.5 GB for
the reference library. Keyframe gaps of 15 s and 42 s occur, so some marks show the same picture
as the one before. Not measured yet: an MPEG-2 file (none in staging's library), and ainas's
Celeron J3455, which serves production and was left alone; it is likely two or three times
slower, a few days for the reference library.

A file whose keyframes are farther apart than the interval (a few encoders put one every 20 s or
more) gets one picture per keyframe, each mark showing the nearest; one with almost none falls
back to decoding every frame for that file alone, which the job's estimate shows.

## 3. The node: a command, at idle priority

- **A command `makeTrickplay`** (a protocol version step): the video, named by payload hash and
  checked before FFmpeg reads it (D100), its picture stream, the version's interval, width and
  sheet size, its WebP quality, and the sheets already made. FFmpeg reads through the node's byte
  path (§8.1) under its confinement, started through `internal/node/execx`, as every run is.
- **At idle priority:** the run is put in Linux's `SCHED_IDLE` class, which gets the CPU only when
  nothing else wants it. That is stronger than `nice 19`, which still takes a share against normal
  work, and an unprivileged process may lower itself into it, so it needs no capability in the
  container. The node lowers no process's priority today; this adds the first, as an option of
  `execx` the encoder's best-effort slot can use later (§9). Its reads go through the disk
  scheduler at the Background tier, held to the background rate (D329 §8).
- **One trickplay run per node at a time**, its own slot, so it never takes the read-through's or
  an encode's.
- **Pieces of one sheet:** each sheet is written and sent to the core as it is finished, so a run
  stopped (a restart, a cancel, the NAS switched off) loses at most the sheet in progress and
  resumes from the next (§6). The run stays one FFmpeg across sheets, seeking only on a resume.
- **Its answer** is each sheet's WebP with its index, then the file's whole result: done, FFmpeg's
  word that it cannot (a fact about the bytes, kept), a refusal (the node did not try), or a stall,
  as a read-through answers (`nodeproto.ReadThroughResult`).

## 4. The core: a job, a sweep, the store

- **A job kind `trickplay`** in a new package `internal/core/trickplay`, the one writer of its
  table, modelled on `internal/core/readthrough`: each live video with a picture, a cover never,
  once per payload hash and trickplay version; failures as facts (D231 §4, D244 §1), parking on a
  root that is unavailable (D231 §5), two stalls in a row the file's failure.
- **The sweep** queues every video lacking its pictures at `PriorityBackground`, after the cheap
  passes in the queue's order (D102: ordering by user value). Every file, as the owner answered.
- **The bump is interest's** (D324): the job is the file's, so a film's page shown raises it to the
  looked-at rung, a play to a play's, and a watched film keeps it at shown interest. **Raising
  moves its place in the queue only:** its run stays `SCHED_IDLE` and its reads at the Background
  tier whatever its priority, so a raised trickplay never takes the CPU or the disk from the
  playback that raised it (question 4).
- **The store:** a table in `cache.db` (rebuildable, not `persistent.db`) of each file's sheets,
  by payload hash and version, each sheet's bytes content-addressed in the cache directory as
  artwork's images are, under the same ceiling, evicted by D530's weighing of cost, size and how lately it was served
  (question 2).
- **Forgotten** when the probe facts it was made from change (`library.DerivedDroppers`, D392 §3),
  so the sweep makes it again.

## 5. The API and the scrub bar

- **`GET /api/v1/files/{id}/trickplay`:** the version's interval, picture width and height,
  columns, rows, the picture count, and each sheet's address; 404 while none are made. A sheet is
  served as artwork is, content-addressed and cached for good, so it travels through Connect's
  relay as posters do.
- **The scrub bar** (`web/apps/web/src/timeline.tsx`): while a pointer hovers or drags, or a TV's
  arrow keys step the position, the picture of that point shows above it with its time, cut from
  its sheet by CSS position. It is shown at a fifth of the window's width, no narrower than 160 px nor
  wider than 240 px, its sheet scaled with it (the owner, 2026-11-28: at its made 320 px it filled a
  phone's width; other players show 100 to 240 px), and made at 320 px so a dense screen shows it sharp.
  The list of the film's sheets is read when the player opens and,
  while a sheet it calls for is not made yet (a 404, or fewer sheets than its picture count,
  columns and rows call for), again every 30 seconds as the film plays, until every sheet is made
  or the player closes (the owner, 2026-11-27), so a film opened before its pictures exist gains
  them in the order the node makes them. The sheets themselves are loaded once the film plays,
  never during its first seconds of buffering. The cast receiver
  shows none, since it has no scrub bar; a phone controlling a cast shows them from the same API,
  on its timeline and as the picture where the video would be: the picture of the TV's place, the
  film's still until the pictures exist (D571; `web/apps/web/src/cast-frame.tsx`). No key frame is
  made on demand.
- **A sheet not yet made** shows the time alone for its stretch: a film half made previews its
  first half.

## 6. Phases

Each phase ends green, with tests that fail when reverted (D190).

| Phase | What | Done when |
| --- | --- | --- |
| F0 | **Measure** (done 2026-11-21 on the NUC7, §2; ainas and MPEG-2 left): keyframes-only against a full decode, on the NUC7 and ainas, three codecs; the WebP quality at 320 px | The numbers are in this file and question 1 is answered |
| F1 | **Built 2026-11-27 (node protocol 74).** **The node:** `makeTrickplay`, `SCHED_IDLE` through `execx`, one slot, sheets as pieces | A generated fixture's sheets have each picture at its mark within one keyframe gap; a run stopped mid-film resumes at the next sheet; the run's scheduling class is idle (Linux) |
| F2 | **The core:** the job, the sweep, the table, interest's raise, the store, D530's eviction for every cache entry, forgetting | A swept library queues every video once; a page shown raises its film's job; FFmpeg's failure is not asked again |
| F3 | **The API and the scrub bar** | Hovering, dragging and arrow keys show the right picture of a made film; an unmade film shows the time alone (vitest and a Playwright run) |
| F4 | **On devices:** a VQ item for the Fire TV and a phone, and the staging library swept start to end with playback unaffected | The owner's run |

### What the lead's walk of F4 found (2026-11-27)

On staging, on the lead's Fire TV and phone (VQ423's steps, before the owner's run): the picture
shows within about 100 ms of a press on the Fire TV and about 200 ms of a touch on the phone, stays
wholly on the screen at the film's first and last minutes, and vanishes about two seconds after the
last press; a film with no pictures made shows its time alone. The film's start was no slower.

Not checked, for the owner's run to judge: a key truly held down on the Fire TV (the walk's presses
were single ones), and a drag of the cast remote's bar.

For the next batch, not built:

- **A Right after the Fire TV's controls hid themselves seeks with no picture.** The film steps ten
  seconds on, and the picture shows again only once Down has brought the controls back.
- **The phone's video stood paused after the walk's drag.** The drag was a made one, not a finger's,
  so whether a real drag leaves it paused is not known.

## 7. Tests (the main ones)

- The pictures fall at their marks within one keyframe gap, on a generated fixture whose frames
  show their own time (`internal/testutil`'s fixtures).
- A resume makes no sheet twice and skips none.
- The run is started in the idle class (Linux only), and an encode's run is not.
- A trickplay job raised by a page is still started idle and its reads are still Background.
- The sweep never queues a cover, nor a file whose failure is kept, nor one under a missing root.
- The API answers 404 before the first sheet and lists the sheets made after it.
- The scrub bar shows the picture of the hovered point, cut from the right sheet at the right place.

## The owner's questions, answered 2026-11-21

1. **Keyframe pictures, or exact marks?** Keyframes: *"It doesn't need to be exact; it needs to be
   fast."* Measured 29 to 34 times cheaper than exact 10 s marks (§2).
2. **Their place in the cache's eviction order:** not a place in an order. The owner: *"Evict
   cache items based on a balance of the cost of regenerating them, space recovery from eviction,
   and LRU."* Recorded as D530, which replaces D212 §6's fixed order for every entry the cache
   evicts; F2 builds it, with the pictures' cost set by the film's length.
3. **Hardware decoding for trickplay:** none; the hardware decoder's slots stay playback's.
4. **A play raises its film's trickplay to a play's place in the queue, but never its CPU or disk
   tier** (§4): yes.
5. **TV arrow keys** step the position with the picture shown (§5), Fire TV's and the web
   client's alike: yes, in F3.
