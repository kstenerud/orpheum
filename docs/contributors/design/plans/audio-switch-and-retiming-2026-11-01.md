> **ABOUTME:** Two designs for the owner (2026-11-01): switching a film's audio track without
> restarting it, and re-timing a source whose B-frames carry no display times (file 5667). Each:
> what happens today, the proposal, its cost, what it gives up, and the owner's questions.

# Seamless audio switching, and re-timing a picture with no display times

## Part 1: switching the audio track without a restart

The owner (2026-10-30): "When I switch audio tracks, the whole video stops and restarts. Is there a
way to make this more seamless?"

### 1.1 What happens today

- **The playlist already offers every track.** The master playlist of a film on its ladder lists
  every sound of every audio track as alternate audio, `EXT-X-MEDIA:TYPE=AUDIO`, one group for the
  lower qualities and one for the top (D368 §3): each track's standard mix, its boosted mix where it
  has a centre (`SoundsOf`, `internal/core/chunks/sounds.go`), and, at the top, each track the device
  plays as it is, copied (`internal/core/chunks/ownsound.go`, `offer.go`, `playlists.go`). So "one
  sound rendition per playback" is true only of where a playback *starts*: `Offer.SoundStreamName`
  (`internal/core/chunks/stall.go`), which the start's log line names `ladderSound`
  (`internal/core/api/playback.go`).
- **The node needs nothing new.** A run of any offered sound is made on request from the chunk a
  player asks for, led in from the chunk before (`internal/node/chunks/arguments.go`); a run no
  player wants stops (D309, D355).
- **The player can already switch sound in place.** The dialogue boost does: `playSound`
  (`web/apps/web/src/film-player.tsx`) → `setAudioRendition` → the chooser `keepAudioRendition`
  returns (`web/packages/player/src/hls.ts`), which sets hls.js's `nextAudioTrack`: the sound loaded
  plays on, hls.js asks for the new sound's chunk at the playhead, and once it has parsed, flushes the
  old sound from 0.25 s past the playhead (`nextAudioTrackBufferFlushForwardOffset`) and appends the
  new one (hls.js 1.7.3, `audio-stream-controller.ts`). Before that, with `audioTrack`, the boost
  froze the picture 2.2 s on staging (2026-10-29), which is how long the new sound's first chunk took.
- **A track choice restarts on purpose.** `LadderUnder` (`web/apps/web/src/play.tsx`) sends a track
  choice to `decideAgainFor(…, "audioChosen")`, which asks the server how the file plays again and
  reloads the player from where it stood. Its comment says why: "the offer's carried gain and the
  decision's levelling are the starting track's alone, so another would play at the wrong level".
  Levelling (D226 §3–§4) is two numbers the player holds for the one sound it starts with:
  `levelling`, the gain the player wants, and `carriedGainDb`, the gain already in the sound's chunks
  (`web/apps/web/src/levelling.tsx`, `addedGainDb`). Every other track's are not sent.
- **A latent error in the same numbers.** `Started.Levelling` is always the *main* track's
  (`decide.DirectLevelling`, `internal/core/playback/playback.go`), and a track copied at the top, or
  its mixes made at its own level (`-ownlevel`, D368 §1), carry no gain of their own and are levelled
  by it. A playback that starts on a non-main track the device plays as it is is levelled by the main
  track's gain, wrong by the difference between the two tracks' loudness. I have not seen it on a
  device; it is from the code.
- **Watch Together and casting.** A sound choice is each person's own: the hub relays a remote's
  choice (`MessageSound`) only to the same person's devices that play the film, and no one else's
  (`internal/core/together/doc.go`). A leader's track change does not reach followers, and should
  not. Today it does disturb them indirectly: the leader's player reloads. A cast's receiver carries
  out a remote's track choice as its own press (`useTracksWorkedByARemote`,
  `web/apps/web/src/audio-tracks.tsx`), so it takes whichever path the Play page takes.
- **The service worker and the loader** pass a chunk through unchanged (`web/apps/web/src/sw.ts`):
  an in-place switch asks for different chunks by the same route.
- **Where it cannot switch in place.** The element's own HLS (iPhone below iOS 17.1, the only
  browser hls.js cannot run in: idiosyncrasies.md) chooses its own tracks; `playNative`'s
  `setAudioRendition` does nothing. A film played as it is, off the ladder (`DirectUnder`), has one
  sound in its file to the browser.

### 1.2 Proposal

A track chosen on the ladder goes the boost's way: one path for every change of sound as the film
plays.

1. **The server sends each sound's levelling.** Each sound in the playback's tracks
   (`Tracks.Sounds` and the top's `Pictures[0].Sounds`, `internal/core/playback/tracks.go`) carries
   the two numbers the player needs: the gain it wants for that sound and the gain its chunks carry,
   computed by the same functions that make the chunks (`decide.StereoLevelling`, `OwnLevelGainDB`,
   `asItIs`). A copied track and its `-ownlevel` mixes want that track's own levelling, not the main
   track's, which fixes the latent error above.
2. **The Play page switches in place.** `LadderUnder`'s `onAudioChosen` calls `playSound` with the
   track and the boost as the person left it (a stereo track has no boosted mix, so the boost switch
   hides). `SoundChoiceCause` gains `trackChosen`, so the player's note says why.
3. **The gain changes when the new sound is heard,** on hls.js's `AUDIO_TRACK_SWITCHED`, not when it
   is chosen, so the old track never plays at the new one's gain. The boost needed no such care,
   since its mix carries its track's gain (D226 §3).
4. **The core follows the sound playing, not the one started with.** A stall's offer starts the
   playback's *starting* sound (`startingSoundOf`, `internal/core/chunks/ownsound.go`); it takes the
   sound the player last said or asked for instead (`playerInterest.sound`,
   `internal/core/playback/interest.go`). The person's choice is kept as now (`keepAudioChoice`,
   D230), and the roster reports it as now.
5. **Kept as today:** the element's own HLS and a film played off the ladder ask again and restart;
   a non-main track already sends a film off the ladder onto it.

**What the viewer sees and hears.** The picture never stops. The old track plays on until the new
one's chunk at the playhead has been made and loaded; then the new one cuts in about 0.25 s ahead
of the playhead, mid-word, with no fade. Expect 1 to 2.5 s on a home link: 2.2 s was the boost's
measured wait, and a copied track needs no encode. A track already played in this playback has its
chunks made and comes in sooner. Whether the cut is clean between a copied 5.1 track and a made
stereo mix, or between AC-3 and AAC (one SourceBuffer, a `changeType` at the new init), is a VQ item
on Chrome, Firefox and the Fire TV's Silk. VQ116 heard the same kind of change between qualities.

**Amended (the owner, 2026-11-01; D400).** In Chrome on staging, the old language played on 1 to 3 s
and then the new one cut in with a gap in the sound. The owner accepts a wait at the choice, not one
later, so a track chosen now cuts in at once (`audioTrack`): the sound loaded is thrown away, the
picture waits at the choice for the new track's chunk at the playhead, and nothing is heard of the old
language after it. The boost still goes by `nextAudioTrack`.

**Browsers.** Chrome, Edge, Firefox, the Fire TV's Silk, desktop Safari and iPhone from iOS 17.1
play through hls.js over MSE or ManagedMediaSource, which already switches the boost this way in
fMP4. Safari's Web Audio is silent under MSE (idiosyncrasies.md), so there the gain cannot be
corrected after the switch, as at a start.

### 1.3 Cost

- Core: two numbers per sound in the tracks (API, `make generate`), the main-track levelling fix,
  the stall offer from the interest. Go tests for each.
- Web: `LadderUnder` and `useLadderPlayback` (`film-player.tsx`), the gain at `AUDIO_TRACK_SWITCHED`,
  the boost switch per track; `hls.test.ts` and the Play page's tests.
- About one agent day. No node, protocol, migration or dependency change.

### 1.4 What it gives up

- The old language plays on for a second or two after the choice, instead of a stop. *Amended
  (D400): the picture waits at the choice instead, for about as long, and the old language stops
  there.*
- The element's own HLS and a film played off the ladder still restart.
- The switch cuts in without a crossfade.

### 1.5 Questions for the owner

1. **Old track or silence while the new one loads?** The picture plays on in both. Either the old
   track plays on until the new one cuts in, or the old one is muted at once and the new one fades
   in. *Recommend the old track playing on*: silence reads as broken, and a second more of the old
   language costs nothing. It is also how the boost behaves now.
2. **Fix the levelling of non-main tracks?** The fix changes how loud such tracks play today, to
   their own measured level. *Recommend yes, in the same change.*
3. **Off the ladder (played as it is) and on old iPhones: keep the restart?** *Recommend keeping
   it.* Choosing a non-main track already moves a film onto the ladder, and from there every later
   choice switches in place.
4. *Implementation:* change the player's gain when the new sound is heard (recommended), or at the
   choice (simpler; the old track plays at the wrong level for the seconds of the wait).
5. *Implementation:* the stall offer and the play statistics follow the sound playing, from the
   player's interest (recommended), or the playback's starting track is replaced on each choice
   (another write path for the same fact).

## Part 2: re-timing a picture whose B-frames carry no display times

File 5667 (MP4, HEVC `hev1`, `has_b_frames=2`): every packet's pts equals its dts. Our copy keeps
that, its chunks' `trun` boxes have no composition offsets, and Chrome decodes it in low-delay mode
and drops the reordered frames (14 to 16 fps). The owner chose to detect this at scan and rebuild the
display times from the picture order count (POC) in the slice headers.

### 2.1 What happens today

- The top of the ladder copies the picture (`StreamPicture`, `internal/node/chunks/arguments.go`:
  `-c:v copy -avoid_negative_ts make_zero`, `-tag:v hvc1` for HEVC). The pts go through untouched.
- Each chunk is placed by its first sample: its keyframe is shown at its bound, decoded as much
  earlier as its composition offset says (`startTicks`, `internal/node/chunks/maker.go`;
  `FirstShownAfterDecode`, `fmp4.go`). Two runs' chunks join at the film's times however each run
  started.
- The grid's keyframe times come from the file's index or the read-through (D259 §1), here the
  keyframes' dts, since pts = dts.
- Encodes (the twin and the qualities) decode the picture, and FFmpeg's decoder reorders by POC
  itself. Measured on a flattened 30 s clip: 716 of 720 frames out, the losses at the film's start.
  They are not the failure the owner saw.
- Our FFmpeg fork patches only the fdproxy protocol and fftools' input errors
  (`ffmpeg-fdproxy/patches`): nothing about timestamps.

### 2.2 (a) Can FFmpeg give each frame's POC without decoding? Yes: the `dts2pts` filter

- **The parsers compute it and keep it.** `hevc/parser.c` and `h264_parser.c` work out each
  picture's POC (`ff_hevc_compute_poc`, `ff_h264_init_poc`) into the parser's
  `output_picture_number`. libavformat never reads that field, and ffprobe never prints it.
- **`ffprobe -show_frames` decodes** (`process_frame` calls `avcodec_send_packet`,
  `fftools/ffprobe.c`): a whole film's worth costs a decode.
- **`setts` cannot reach the POC.** Its expressions see `N`, `PTS`, `DTS`, `PREV_*`, `NEXT_*`,
  `STARTPTS`, `STARTDTS` and the time bases, nothing from the bitstream. `hevc_metadata` and
  `h264_redundant_pps` rewrite parameter sets, not times.
- **FFmpeg already has the filter the owner described:** `dts2pts` (`libavcodec/bsf/dts2pts.c`,
  "Derive PTS by reordering DTS"). It reads each access unit's slice headers through CBS, with no
  decode, computes the POC (H.264 by `ff_h264_init_poc`, HEVC from `slice_pic_order_cnt_lsb` and the
  last temporal-layer-0 picture, IDR resetting it), and gives each packet, in POC order, the dts of
  the packet at that rank. It holds a FIFO the size of the decoded picture buffer, so it adds no read.
  **Our pinned build has it for both codecs:** `ffmpeg -h bsf=dts2pts` with the fdproxy 8.1.3-1
  build says "Supported codecs: h264 hevc". No fork patch is needed.

**Measured with the pinned build**, on synthetic clips made like file 5667 (B-frames, then
`setts=pts=DTS` to flatten them; scripts kept in the session's scratchpad, not the repo):

| Clip | Whole file copied through `dts2pts` | A run: seek past a keyframe, `-to` the next |
| --- | --- | --- |
| HEVC, 3 B-frames, closed GOP | every pts the clip's own but the last packet's | every packet's offset exact |
| HEVC, 3 B-frames, open GOP (CRA, RASL) | every pts the clip's own but the last packet's | order exact; every pts one frame later than the clip's, constantly |
| H.264, 3 B-frames, B-pyramid (POC type 0) | the clip's own but two packets | exact but the last B-frames before `-to`; exact when the run reads 0.5 s past its end |

Two edges show in the table:

- **A run's constant shift (open GOP)** does not matter. A chunk is placed by its keyframe's own
  composition offset, so each run's shift cancels.
- **The frames just before a run's end are guessed** (`dts2pts` at end of input), off by a fraction
  of a frame. The run must read past its last bound, about half a second (at least the reorder depth
  past the next keyframe); the segment muxer still cuts at the bound, and what lies past it is
  dropped as now. At the film's own end, the last frame or two keep a guessed time: a few hundredths
  of a second.

### 2.3 (b) A table in the store, or worked out per run? Per run

`dts2pts` is exact within a run, as above, so a per-frame table adds nothing a run needs. A table
would cost a whole-file read to build it, about 170,000 entries for a two-hour film, a column and a
query, and a way to apply it: either the node rewrites every `trun`, inserting composition offsets
and moving the data offsets, or we write our own filter that reads a table (a fork patch). The store
keeps only **one fact per file: its picture needs re-timing.** It lives in the probe record (2.4),
keyed like the probe, so a rescan of changed bytes re-decides it.

### 2.4 (c) Detecting it at scan, cheaply

The probe already runs one ffprobe with `-show_format -show_streams -show_chapters`
(`internal/node/probe/probe.go`). ffprobe already reads the file's first megabytes to find its
streams (`avformat_find_stream_info`). So:

- **Add `-show_entries packet=stream_index,pts,dts -read_intervals %+#200`** to the same call: the
  first 200 packets, already read, with no second process. Parse the main picture's `has_b_frames`
  too, which `-show_streams` already prints.
- **The node marks the picture `isUntimed`** (name to settle) when it is H.264 or HEVC,
  `has_b_frames > 0`, and every one of its packets among those 200 has pts = dts, with at least 24 of
  them.
- **A false mark is harmless:** a stream that has no B-frames is reordered by `dts2pts` to the pts it
  had. The filter is applied only where marked, to keep its risk where it pays.
- The core reads the mark from the probe and puts it in the run's command (`nodeproto.MakeChunks`,
  a protocol bump), and the node adds `-bsf:v dts2pts` to a copied picture's run and reads it 0.5 s
  past its end. For an encode, `-bsf:v dts2pts` goes before `-i`: FFmpeg 8 applies a filter on an
  input. On the flattened clip, that recovered all 240 frames of a run that seeks, and 717 of 720
  from the start.

### 2.5 (d) H.264 and HEVC

- **H.264, POC type 0** (x264 and most encoders): the POC is in each slice header. `dts2pts` handles
  it, measured above.
- **H.264, POC type 1:** the POC comes from `frame_num` and the cycle the SPS declares;
  `ff_h264_init_poc` does it, so `dts2pts` does. Not measured: x264 never writes it.
- **H.264, POC type 2:** the output order *is* the decode order, so such a stream cannot carry
  reordered B-frames. pts = dts is right, and the filter changes nothing.
- **H.264 field pictures** (interlaced PAFF): `dts2pts` gives a frame's two fields one pair of
  nodes. Not measured.
- **HEVC:** one POC scheme, its high bits carried from the last temporal-layer-0 picture and reset
  at an IDR. Open-GOP CRA pictures with leading RASL ones work, with the constant shift above. RASL
  frames at a run's start are a matter for any open-GOP source, not this one.

### 2.6 Recommendation

Mark the picture at scan from the probe's first 200 packets. Re-time it per run with FFmpeg's own
`dts2pts`: on the copied picture's runs, and on encodes' inputs, which also fixes their start. A run
reads half a second past its end. No table, no fork patch, no new dependency. The A/V sync is what it
is today: a chunk's keyframe is still shown at its listed time, and the frames around it are only
put back in order.

### 2.7 Cost

- Node: the probe's packet lines and the mark (`probe.go`, `parse.go`); the run's argument and
  margin (`arguments.go`). The integration tier gains a generated flattened HEVC clip and a
  flattened H.264 clip: their chunks must have the composition offsets of the clip's own, joined
  across two runs.
- Core and protocol: the mark into `MakeChunks`; a protocol bump.
- A rescan to mark files already scanned (the probe is redone; no migration).
- About half an agent day, then a VQ: file 5667 at the top in Chrome plays at its own frame rate.

### 2.8 What it gives up

- The film's last frame or two keep a guessed time.
- A file marked wrongly (too few packets read, or B-frames that start late) plays as today until
  someone notices. The mark could be confirmed later from the read-through, which reads every packet.
- We trust `dts2pts` from the pinned 8.1.3. Upstream master's copy has since gained handling of
  damaged frames (`MAX_DAMAGED_FRAMES`); a fault in a marked film is the place to look.

### 2.9 Questions for the owner

1. **Re-time by FFmpeg's `dts2pts` rather than our own POC reader?** It rebuilds the display times
   from the POC, as you chose; FFmpeg does the reading, per run, with nothing kept but a mark per
   file. *Recommend yes.*
2. **Apply it to encodes too** (the twin and the qualities)? Today they lose a few frames at the
   film's start, no more. *Recommend yes*: it is the same flag and one argument.
3. **Which files are checked at scan:** every H.264 and HEVC picture whose stream says it may
   reorder (`has_b_frames > 0`, nearly all of them), at no extra read? Or only MP4 and MOV, where
   this fault comes from a missing `ctts`? *Recommend every one*: Matroska, AVI and MPEG-TS remuxes
   can lose pts too.
4. *Implementation:* keep the mark in the probe record (recommended: the node judges hostile bytes,
   and the core stores what it says), or in a column of its own beside the timeline.
5. *Implementation:* a run's margin past its end: a fixed 0.5 s (recommended, measured enough for
   three B-frames), or computed from the stream's reorder depth and frame rate.

### 2.10 As built (2026-10-31), and what the build found

Built as answered (the mark `ProbeVideo.NeedsRetiming`, the run's `NeedsPictureRetiming`, node
protocol 55), with three findings for the owner (idiosyncrasies.md, "`dts2pts` guesses an H.264
IDR's time"):

- **The margin is a second, not half.** A run ends at a keyframe, and `dts2pts` timed an H.264 IDR
  right only once it had read about 0.65 s past it; at 0.5 s it was timed three pictures early, the
  muxer did not cut there, and the run's last chunk took in the next one's first half second. HEVC
  was right at 0.5 s. Still a fixed margin, as answered; the value changed.
- **An encode's input is moved back as well as re-timed** (`dts2pts,setts=pts=PTS-STARTPTS+STARTDTS`):
  `dts2pts` shows a run's first keyframe as late as the B-frames reorder, which put an encode's forced
  keyframes after its bounds and its cuts were missed. On the flattened 12 s clips the encode then
  has every picture but the film's last one or two; without the filter it had one picture too many.
  The design's measured loss at the film's start did not show on these clips.
- **A rescan does not mark a file already scanned:** a file unchanged on disk is not probed again
  (D357 §2). File 5667 is marked only when
  examined afresh. Whether to re-probe the H.264 and HEVC files already stored, and how, is the
  owner's question.
