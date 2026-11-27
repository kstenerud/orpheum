> **ABOUTME:** The plan for starting a playback at a quality its node makes fast: the node's
> start-up trials of what it can decode and encode, what it tells the core, how the core caps the
> quality a playback starts on, the phases, and the questions left for the owner.

# A playback starts at a quality its node makes fast

- **Status:** PROPOSED — written 2026-11-27 by the lead from the owner's direction the same day:
  *"We may still need to start a cast at a lower quality if no suitable hardware codec exists"*
  and *"We'd likely need to have the core choose based on what the hardware can actually provide
  (i.e. do test encodes to find out what it's capable of, before choosing the base rate to start
  at)"*. Phase S1 (the node's trials) follows from those words; the two questions at the end are
  open. §3 is the owner's addition of the same day.
- **Depends on:** D568 (decode on the card), D457 (a machine too slow to convert), D257 and D381
  (the quality a playback starts on), D265 (the player's own choosing).

## What it ends with

A film whose first chunks its node would take long to make starts on a quality the node makes
fast, on a Chromecast, a TV and a browser alike, and reaches the quality its link carries soon
after. A node says what it can do from trials on its own hardware, with its own FFmpeg, on the
path a real run takes.

## What is there today

- The quality a playback starts on comes from the link alone (`chunks.StartOnLadder`): the top
  where no speed is remembered. The core can already cap it (`StartAtMostKbps`, for a slow
  upload).
- A node measures one thing at its start: its encoder making the lowest quality of a generated
  1080p picture (`encode.MeasureSpeed`). No film's codec is decoded in it. The core uses the figure
  only to offer no encode at all under 110 % of real time (D457).
- Measured on production's Celeron J3455, an HEVC Main 10 film at 1080p is made at 2.65 times
  real time decoded on the CPU and 5.94 on the card; a film the card cannot decode has only the
  first.

## 1. The node's trials

- **The clips are made on the node**, once, by its own FFmpeg (it has libx264, libx265, libvpx-vp9,
  libsvtav1, mpeg2video and mpeg4), from a generated picture with noise at a film's usual bit
  rate, a few seconds each, kept in the node's own directory. Nothing is shipped in the binary.
- **What is tried:** H.264 8-bit and 10-bit, HEVC 8-bit and 10-bit, VP9, AV1, MPEG-2 and MPEG-4
  at 1080p, and HEVC 10-bit at 2160p; each to the top quality of its size and to the lowest, with
  the arguments a run of chunks uses and the decode a run would end on (the card first, the CPU
  where it fails: D568 §2).
- **When:** after the node has said hello, in the background at idle priority, one at a time, and
  never while a run of chunks goes. The results are kept on disk under the FFmpeg's version, the
  encoder and the quality recipe, so a restart measures nothing again.
- **What the core is told:** for each trial the codec, bit depth, height, the quality made, the
  speed in percent of real time, and where it was decoded. Until they arrive the core starts as
  today.

## 2. The core's choice

- A playback's start is capped at the highest quality its node is expected to make at
  **3 times real time** or better (a six-second chunk in two seconds), beside the link's rule and
  the upload's cap. Where none is, the lowest.
- A film of a kind not tried takes the figures of the nearest kind tried that is no easier: the
  same codec at the next size up, or else the slowest the node measured on the CPU.
- The figure D457 judges by becomes the slowest of these and not the generated picture's, in a
  later phase, once nodes in use have shown the trials sound.

## 3. What real playbacks add

The owner (2026-11-27): *"We can also keep data on how real playbacks have performed, and use
those to fine-tune what we believe the hardware is capable of."*

- **Each finished run of a quality tells the core its speed and where it was decoded**, with the
  result the node already sends. The node logs both today and the core is not told.
- **The core keeps a figure a node, a kind of film and a quality**, in its cache database, so it
  outlasts a restart; a node's changed FFmpeg, encoder or recipe starts it over, as it does the
  trials.
- **Only a fair run counts:** one that made at least half a minute of film while no other run
  went on its node. A node made slow by its load is `keepRunsFast`'s matter (D250 §6), not what
  its hardware can do.
- **The real figure takes over from the trial's as runs are counted:** the trial alone until
  three fair runs, then the middle one of the last several. A kind the trials never tried gets a
  figure of its own this way, and no longer borrows its neighbour's.

The owner on these marks (half a minute, alone on its node, three runs, the middle one), the
same day: *"those sound fine"*.

## 4. Phases

| Phase | What | Done when |
| --- | --- | --- |
| S1 | **The node:** the clips, the trials, the kept results, the message | A node with a faked FFmpeg reports each kind once and nothing after a restart; no trial starts while a run goes |
| S2 | **The core:** the figures kept for a node, the cap at a start | A film whose kind measured slow starts on the highest quality measured fast; one not measured starts as today |
| S3 | **Real runs:** the run's speed and decode in its result, the kept figures, the trial's giving way | A kind with three fair runs starts by their figure and not the trial's; a run beside another counts for nothing |
| S4 | **The player** (question 2) | The owner's answer |
| S5 | **On devices:** a VQ item on production's J3455 with the card's decode refused | The owner's run |

## Questions for the owner

1. **Is three times real time the mark?** On the J3455 it starts a CPU-decoded HEVC Main 10 film
   at 432p (3.09) and not 1080p (2.65).
2. **How a player started low gets up.** Today it climbs a quality at a time, at least 30 seconds
   a step, by its link alone. The lead proposes that a player capped by this plan goes straight to
   the quality its link carries once about ten seconds are buffered.
