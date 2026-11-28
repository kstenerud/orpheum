> **ABOUTME:** The plan for starting a playback at a quality its node makes fast: the node's
> start-up trials of what it can decode and encode, what it tells the core, how the core caps the
> quality a playback starts on, the phases, and the questions left for the owner.

# A playback starts at a quality its node makes fast

- **Status:** PROPOSED — written 2026-11-27 by the lead from the owner's direction the same day:
  *"We may still need to start a cast at a lower quality if no suitable hardware codec exists"*
  and *"We'd likely need to have the core choose based on what the hardware can actually provide
  (i.e. do test encodes to find out what it's capable of, before choosing the base rate to start
  at)"*. Phase S1 (the node's trials) follows from those words; question 1 at the end is open, built at its
  proposed mark; question 2 is answered. §3 is the owner's addition of the same day. S1 and S2 are
  built (2026-11-27).
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
- **The lead's reading, built in S2** (open to the owner):
  - *A quality between the two a kind measured* is judged by the time each second of film takes,
    taken as growing in a straight line with the pixels made, from the lowest's to the top's: the
    decode costs the same at every quality, the encode grows with the pixels. The rows say each
    quality's height and the trials' picture is 16:9, so a quality's pixels are its height by the
    film's own shape. The rows cannot part the rate's share from the pixels', so the rate is not
    weighed. This line is slower than the straight line between the two speeds, the cautious one.
    Below the lowest a quality is no faster than the lowest; above the top, further along the line.
  - *A kind* counts only where both its trials measured a speed; one with either failed or not
    tried borrows, as a kind not tried does.
  - *No easier* is the same codec at the same or a greater bit depth, a height and a width no
    smaller than the film's (the trials' width is 16:9 of their height), the smallest such, by
    height then bit depth; else the kind whose top the node made slowest of all, wherever it was
    decoded, the slower reading of "the slowest the node measured on the CPU".
  - *The film's own picture, copied,* is never held: nothing is made of it. A node whose trials
    measured nothing starts as before.
  - The hold is the third bound on a start, after the link's and `StartAtMostKbps`'s, and can only
    lower it; the upload's ceiling stays the highest quality within the upload, so the player may
    climb to it. The start's log line says which bound decided it (`ladderStartBound`: top, link,
    startAtMost, nodeSpeed) and the speed expected of the quality (`ladderExpectedSpeedPercent`).
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
  went on its node. Since D572 the half minute is a window, the run's first half minute of film,
  and the figures are the window's. A node made slow by its load is `keepRunsFast`'s matter (D250 §6), not what
  its hardware can do.
  Another run is one that makes a picture, another quality's or the scrub-bar pictures': a
  sound's or a copied picture's run costs little and goes beside every playback's picture, so
  counting it would leave no run fair (the lead's reading, 2026-11-27; the owner's to change).
  **Found on staging, 2026-11-27:** while the scrub-bar pictures' sweep of a library ran, it went
  beside every quality's run, so none was fair: four finished runs of five minutes of film each
  said they went beside another, and none was kept. **Answered (D572, 2026-11-28):** the owner
  asked to *"pause bg jobs … for long enough to measure"*, since *"you only need a certain amount
  of film transcoded"*. A run is measured over its **speed window**, its first half minute of film,
  timed from its start, and judged beside another over the window alone; and the core holds the
  node's background work back for that window when it gives a played run of a quality its node's
  runs have not yet taken over, then lets it go on. A run beside the sweep still counts for
  nothing; the sweep makes way instead.
- **The real figure takes over from the trial's as runs are counted:** the trial alone until
  three fair runs, then the middle one of the last several. A kind the trials never tried gets a
  figure of its own this way, and no longer borrows its neighbour's.

The owner on these marks (half a minute, alone on its node, three runs, the middle one), the
same day: *"those sound fine"*.

## 4. Phases

| Phase | What | Done when |
| --- | --- | --- |
| S1 | **The node:** the clips, the trials, the kept results, the message | A node with a faked FFmpeg reports each kind once and nothing after a restart; no trial starts while a run goes. Built 2026-11-27 (node protocol 77) |
| S2 | **The core:** the figures kept for a node, the cap at a start | A film whose kind measured slow starts on the highest quality measured fast; one not measured starts as today. Built 2026-11-27 |
| S3 | **Real runs:** the run's speed and decode in its result, the kept figures, the trial's giving way | A kind with three fair runs starts by their figure and not the trial's; a run beside another counts for nothing. Built 2026-11-27 (node protocol 79, cache schema 58). Readings: "another run" is any run of chunks of any stream, or a trickplay run, at any time beside it; the speed is the film made over the run's whole time, its open included, as a trial's; only a finished run counts; the figure is kept per quality, and a quality with fewer than three keeps the trials' line; of an even count the slower middle is taken; the key is FFmpeg's version and SHA-256, the chosen encoder and the quality recipe, and a run under a new one deletes the node's others; a node not heard since the core started keeps and reads none |
| S3b | **The speed window and the hold for it** (D572) | A played run on a node that lacks three fair runs of its kind and quality holds the node's background work back until its window's expected end; the run's figures are its first half minute's, and a run beside it after the window spoils nothing. Built 2026-11-28 (node protocol 80). Readings: the window closes at the first chunk that reaches half a minute of film, so it may hold a little more; only a finished run still counts; a run already stopped when the played run starts is not beside it; the hold lasts twice the window's expected time at the speed the start expected, with 5 s for its start, never past 90 s, and 90 s when nothing judged the quality; it is asked before the run is given, for a run of at least the window's film; a node that encodes on a GPU is held too; a played run of another player is never stopped, and leaves the run unfair |
| S4 | **The player:** one jump to the link's quality, and today's climb after a jump that failed | A capped player with ten seconds buffered asks its link's quality at once; after a stall there it steps down and climbs by today's holds. Built 2026-11-27: the link's quality is the step-up's own (its confident estimate with its headroom), the jump is spent if the link carries nothing higher at 10 s ahead, a give-up falls back as a stall does, a held start ignores the remembered link (D381), and only a playback's own start, not the page's, is held |
| S5 | **On devices:** a VQ item on production's J3455 with the card's decode refused | The owner's run |

## Questions for the owner

1. **Is three times real time the mark?** Answered (the owner, 2026-11-27): *"3x sounds fine. If
   it becomes a problem we can change it"*. On the J3455 it starts a CPU-decoded HEVC Main 10 film
   at 432p (3.09) and not 1080p (2.65).
2. **How a player started low gets up.** Answered (the owner, 2026-11-27): *"We try jumping to
   the link's quality. If that fails, we drop back down and climb slowly."* A player capped by
   this plan goes straight to the quality its link carries once about ten seconds are buffered;
   if that quality then stalls or is given up, it drops back and climbs a quality at a time by
   the rule it has today (`step-up.ts`).
