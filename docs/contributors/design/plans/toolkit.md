# The toolkit batch (D408)

The owner (2026-11-03) ordered the toolkit batch now, before the deferred UX rewrite: our own widget
layer, composed from small parts, with a pop-up apart from the dialog and a list that keeps context
([D408](../../decisions/working-notes.md#d408--the-app-builds-its-own-widget-layer-from-small-composed-parts-a-pop-up-is-not-a-dialog-and-carries-no-close-button-and-a-list-opens-on-its-current-choice-with-a-neighbour-either-side);
research: [ui-toolkit-2026-11-03.md](../research/ui-toolkit-2026-11-03.md)). Screens stay as they are.
End: one `make check-pr`, staging, the owner tries it.

## Steps

1. **The parts under the dialog.** Take out of `ModalDialog` (`web/apps/web/src/modal-dialog.tsx`) what
   every transient surface shares: the native `<dialog>` opened with `showModal()`, closing on Escape,
   a remote's Back, the browser's Back and a press outside, and focus back to the opener. `ModalDialog`
   keeps its ✕ and its width on top of them (D408 §2: dialogs keep the ✕).
2. **`Popover`**, built from the same parts: placed at its opener (today's `layAtOpener`,
   `drop-down-place.ts`), as big as its content (`fit-content`) within the space there is, no ✕, no
   minimum width. `DropDown` becomes a `Popover` holding a list; the ✕ band, its scroll padding and
   `keepFocusedClearOfClose` go.
3. **`ListBox`**: opens with focus on the current choice and at least one entry above and below in
   view; while the highlight moves, one entry of context stays either side (`scroll-padding-block` of
   one row, a maximum height a row short of the space, placement that keeps the current entry inside).
   Its options are its own component, so `drop-down.css` no longer styles whatever is inside it
   (`.drop-down__places a, button {width: 100%}`).
4. **The overlay's pop-ups on them**: 💬, 🎧 and ⁝ as `Popover` + `ListBox` (⁝'s steppers and
   switches as its own content), 🔈 as a `Popover` sized to its slider and mute button. The banner
   menu, ⁝ on pages and the choosers follow, since they share `DropDown`.
5. **Layout primitives** `Stack` and `Cluster` (the 2026-10-05 architecture research's R2), used by
   the new widgets and the pop-ups' contents; components stop taking a `className` that restyles them
   where this batch touches them (R5).
6. **The native `<select>`s** a TV cannot work (the 2026-10-27 widget audit: the audio and subtitle
   menus under the film first, `audio-tracks.tsx`, `subtitles.tsx`) become the chooser.
7. **Docs**: the web README's invariants, the `/ui-gallery` specimens, D408's status. **Done**: the
   README's invariants were kept with each step; the gallery shows a pop-up holding a list and the
   layout primitives; D408 is Built.

## The owner's comments on steps 1–4 (2026-11-03), folded into the batch

8. **A pop-up's width.** The banner's menu came out too narrow, wrapping its entries. The rule (the
   lead's wording of the owner's): a pop-up is as wide as its widest entry, but never wider than half
   the screen; an entry longer than that wraps. Half a phone's screen is too narrow for a word, so the
   cap is never under 16rem, and the margin (9) always wins. The width is its choices' alone: the
   menu's "Signed in as …" line only informs, and wraps rather than widening it. The owner
   generalised it (2026-11-03): informative content may wrap, action content only where nothing
   else will do (D408 §7, `ListNote`).
9. **A margin pop-ups keep clear of**: a small one (0.5rem, plus the device's safe-area insets) at
   every edge of the screen, which no pop-up covers.
10. **Cast and Watch Together open at once.** Their first press shows "Loading…" in place of the
    icon until the code arrives; it is loaded ahead, once the app is idle after its first page.
11. **🔈 grows upward out of its button.** The pop-up is placed so that its speaker sits where the
    button was, the slider above it: pressing 🔈 looks like the button growing into the control, and
    it shrinks back once something is chosen.

**Separately:** a one-day Preact spike measured on the Fire TV stick (D408 §5). Trying Preact adds a
dependency, which the hooks refuse in a feature session; it needs a session started with
`ORPHEUM_ALLOW_DEPS=1` (the owner starts one after steps 1–4). The Fire TV stick and the Pixel 3a are
dedicated to testing (the owner, 2026-11-03), so the spike uses them without asking.

## Folded in (the owner, 2026-11-03: "put it in this batch")

12. **A chunk FFmpeg cannot cut stops the film for good, and the viewer is not told** (found
    2026-11-03 by the Preact spike's runs on the Fire TV).
    The Gold Rush (file 241, the picture ladder an earlier commit…`) stops at 42:13 on staging, every time.
    Chunk 349's run fails: the node's check finds FFmpeg's raw `00349.mp4` holding 12.4 s (31531 to
    43960 ms of the run) where the chunk should be 2.3 s (31281 to 33575 ms), "FFmpeg cut it
    elsewhere or was stopped in it"; a run started at the chunk itself fails the same way (0 to
    12679 ms for 0 to 2294 ms). The core then answers each request for the chunk with "FFmpeg failed
    on the film at this chunk each time it was made", and the player asks again every 2 s,
    indefinitely: paused, a black picture, the controls up, no message. Two faults: the cut (a
    keyframe the ladder's cuts expect is not where FFmpeg cuts, so chunk 349 needs a cut the encoder
    did not make), and the player, which should tell the person and offer to skip past the chunk
    rather than wait forever. Evidence: staging's node log, job 25680 and 25682, 2026-11-03 09:25
    UTC.

    **Built ("fix(node): a re-timed copy is cut early enough for its keyframes' own delays", "fix(player): a chunk the core cannot make ends the playback, saying why").** The cut was the copy's, not an encode's: the film's x265 is
    open-GOP with its B-frames flattened, so its copy is re-timed by dts2pts, which shows each
    keyframe 83 to 250 ms after its decode time, differently at each; the segment muxer times its
    cuts from its first packet as shown (FFmpeg's `segment.c`, `reference_stream_first_pts`), so a
    keyframe shown less late than the run's first came early by the difference, and a cut 50 ms early
    missed it. A re-timed copy is now cut 400 ms early; an open-GOP HEVC test film fails the old
    margin as staging did. On staging after the fix, chunk 349 was made at once (job 25802), 55
    pictures, 2.29 s. The player now ends the playback at the core's 422 and says the server failed
    on this part of the film each time it tried. **Not built:** offering to skip past such a chunk,
    which needs the owner's look (where the offer sits, its words, what it skips to).

13. **`IconButton` takes no `className`** (the owner, 2026-11-03, agreeing to the lead's proposal):
    what its callers style becomes named options, each look unchanged; what a parent places or finds
    it by goes on a wrapper the parent owns. `Stack` gets no reversed option: 🔈's upright pop-up,
    the only case, keeps its own `column-reverse`, until a second case appears.

## The owner's look at the batch on staging (2026-11-03)

- **The Gold Rush plays through 42:13** "without any visual or audible problems": item 12's cut holds.
- **The UI gallery from the device lab** ("it's a pain to type in on that device"): built, "feat(web): the device lab's menu leads to the UI gallery, a few presses away on a TV",
  on staging.
- **A pop-up per widget type showing it in its states**: "For each UI widget type, we should have a
  popup dialog that shows that widget in its various states. For example the chooser with only one
  item, 2 items, 3 items, many items." Moved to the next batch (the owner, 2026-11-03).
- **The gallery's "player's bar over a film" is the old layout**: it draws `PlayerControls` as it was
  before D407, not the full-screen overlay's three rows nor the bar under the film (D407 §9). To
  build: the gallery draws the real rows from a source it gives, so it cannot drift again. Moved to
  the next batch (the owner, 2026-11-03).
- **The tray's signs never seem to pulse**: "I've never actually seen the watch together or cast
  icon pulse. I still don't see it on /ui-gallery". In Chromium and Firefox (vitest's browser mode)
  the running sign's icon breathes, opacity 1 to 0.55 over 3 s, in the gallery as in the banner; it
  is off by design where the system asks for less motion (tokens.css, icon-button.css). **Resolved**
  (the owner, 2026-11-03): "tray icons pulse and look good. The only thing that was inhibiting them
  before was the reduce motion setting on macos."
- **A chunk the server cannot make** (item 12's unbuilt half), the owner's design: "Can it just skip
  automatically, and maybe put up an exclamation mark item in the non-fullscreen video page about
  the problem? If it's able to skip forward, and will skip more than 15 seconds (or simply can't
  play any further), show a pop-up dialog as well." The lead's details, which the owner agreed
  ("Agreed", 2026-11-03): the sign stays for the viewing; the pop-up shows in full screen too; the
  player steps over two minutes at most, past which the film cannot go on. **Built:** the player
  plays up to a chunk the core answers 422 and on from its end, chunks in a row one stretch
  (`skip-unmade.ts`); the bar under the film shows D278's warning sign naming each stretch, and a
  stretch over 15 s, or one the film cannot go on past, puts up a pop-up over the film
  (`skipped-parts.tsx`). VQ255, which the lead ran in Chrome against staging: a film with several
  qualities never got hls.js's fatal error, only a step down at each refusal; the quality is now kept
  and the chunk stepped over at its first refusal (the owner, 2026-11-03: "It should only step down
  when playback problems are due to bandwidth or processor issues that warrant it").
- **Play from The Gold Rush's page opened the player without playing**: "I had to manually start it."
  Firefox on a Mac, the film's first play. Its own picture was given up three seconds in, before the
  first frame (`bufferAppendNoProgress`), and the twin opened paused, the video having read as
  paused while waiting to start. **Fixed** ("fix(web): a film given up on its own picture before its first frame opens again playing"): the twin opens paused only where the person
  paused it. Why Firefox on a Mac cannot append The Gold Rush's own picture is still open.
- **Idea: the scrub bar's time under the pointer**: "When moving the mouse over the scrub bar, show
  a tooltip with the offset there. Eventually we'll be showing the preview thumbnail as well (much
  like how Youtube does it)." Moved to the next batch (the owner, 2026-11-03: "item 6: next
  batch").

## Next batch (filed during this one)

- **The §28 review's proposals S2, L1, L2 and D1** (the owner, 2026-11-03: "the rest in another
  batch"; [messages-get-lost-audit-2026-11-03.md](../research/messages-get-lost-audit-2026-11-03.md)):
  a group's buttons saying they wait while Watch Together reconnects; the last position sent as a
  page closes (`keepalive`, a flush after one in flight); a playback report sent again under its own
  `reportSeq`; Cast's device list shown as reconnecting while the live connection is down.

- **A pop-up per widget type in the gallery, and the gallery's player bar drawing the real rows**
  (the owner, 2026-11-03, above).

- **The scrub bar's time under the pointer** (the owner, 2026-11-03, above): a tooltip with the
  time where the pointer is over the scrub bar; the preview thumbnail later.

- **Core and node record their own CPU and memory use** (the owner, 2026-11-03: "how much cpu and
  memory the core uses and the media node uses, so that we can gauge what kinds of specs will be
  necessary for running it when we open up beta … have the process record automatically"). The
  lead's proposal: each of core and node reads its own cgroup's counters (`cpu.stat`'s `usage_usec`,
  `memory.stat`'s `anon` and `file`, `memory.peak`) once a minute and keeps the minute's figures in
  the store, the node's including its FFmpeg runs, since they share its cgroup; outside a cgroup, the
  process's own `/proc/self` figures. The Diagnostics page (D341) shows them beside what was
  happening (plays, transcodes, scans), so a figure can be tied to its load. Until it is built, the
  lead samples staging's two containers on stagehost from the sandbox the same way
  (`/yoloai/files/resource-usage/staging.csv`; the script is in the lead's cache). The first figures
  (2026-11-03, staging idle, 56 minutes after a start): core 9% of one core on average, 82 MiB held,
  363 MiB peak; node 2% of one core, 11 MiB held, 2.5 GiB of page cache from reading media, which
  the kernel reclaims.

- **`ModalDialog` takes no `className`** (the owner, 2026-11-03, moved here from step 5): its seven
  callers (Cast, the folder browser, match correction twice, the notice note, the sign-out question,
  Watch Together, the listing search) mostly set their width or lay out their contents; as named
  options some may look different, so the owner reviews the dialogs together.
- **A re-timed copy is cut at exact times, without item 12's margin** (the owner, 2026-11-03: keep
  the margin for now; the information comes from the one full scan of a file, with the other tasks
  that run on it). A film whose B-frames carry no display times has its keyframes listed at their
  decode times, but its copy, re-timed by dts2pts, shows each keyframe 83 to 250 ms later than that
  (The Gold Rush), differently at each, and the segment muxer times its cuts by display time from the
  run's first keyframe; item 12 cuts 400 ms early to cover the difference. The read-through already
  copies the picture's packets, undecoded, into its timeline (`internal/node/readthrough`, D247 §10);
  with dts2pts on that copy for a film marked `NeedsPictureRetiming`, it can record each keyframe's
  display delay beside its time, at a little CPU and no further read. A run then gives the muxer each
  cut as (bound + its delay) − (first keyframe + its delay), and the margin goes, with its two limits:
  a delay difference past 400 ms, and a keyframe that is not a bound within 400 ms before one that
  is, which would take the cut. To check first, as a cheaper way: whether the muxer's
  `-segment_frames` (cuts by picture count) can be fed from the container's own index (MP4's sync
  samples list keyframes by number; Matroska's cues do not, it seems).
