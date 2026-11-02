> **ABOUTME:** How established players and servers handle a DVD title whose clock starts again
> partway, read to check D404's restamp, and the other DVD and Blu-ray pitfalls they meet, ranked
> by how likely they are to reach Orpheum's disc images.

# DVD playback: clock restarts and other pitfalls (2026-11-02)

Gathered 2026-11-02 at the owner's request, after VQ73's DVD (the Der Golem concert) was
found to start its clock again at 1:28:39 and D404 restamped it. **(V)** marks a claim read in
source cloned to `/yoloai/cache/research-dvd/` (FFmpeg, HandBrake, Jellyfin, libdvdnav,
libdvdread, libbluray); **(F)** one from a forum, mailing list or issue tracker. Kodi, mpv and
MakeMKV were not read.

## 1. What others do about a clock that starts again

- **FFmpeg's `dvdvideo` demuxer (V, `libavformat/dvdvideodec.c`, FFmpeg 7.0+).** Built on
  libdvdnav and libdvdread; no CSS (its documentation says it is not for encrypted discs). At
  **every navigation pack** it compares the previous VOBU's `vobu_e_ptm` with the new
  `vobu_s_ptm`, with no tolerance; when they differ it adds `prev_e_ptm - cur_s_ptm` to a running
  offset, ends and resets its inner `mpegps` demuxer, and adds the offset to each packet's PTS and
  DTS. It does not rewrite bytes or SCRs. After a break it drops AC-3 packets whose PTS does not
  move forward and those whose size is not a whole frame ("DVD muxers which end seamless PGs on
  duplicate or partial AC3 samples"). It declares a 32-bit wrap ("VOBUs use 32 (PES allows 33)").
  `-trim` (on by default) skips cells under a second; `-angle` picks an angle, and a seek on a
  multi-angle title forces angle 1 and skips 3 VOBUs because libdvdnav loses navigation packs
  there (F: VLC has the same fault); `-preindex` reads every navigation pack for exact chapter times.
- **HandBrake (V, `libhb/dvdnav.c`, `reader.c`, `stream.c`).** Reads through libdvdnav and hands
  the navigation packs to its program-stream demuxer, which acts on SCR jumps: on a new clock it
  flushes what was timed by the old one, and `splice_discontinuity` rejoins buffers split across
  the break. Nothing is restamped from the navigation packs.
- **libdvdnav (V).** Reports the breaks (`doc/dvd_structures`: navigation packs "detect PTS
  discontinuities") and leaves them to the player. VLC treats a cell change as a discontinuity (F),
  with reports of a frozen picture and skipped sound at cell joins (F: VLC #25854) and of seamless
  branching that is not seamless (F: VLC #3645).
- **Jellyfin (V, `MediaEncoder.cs`, `EncodingHelper.cs`).** Reads no IFO: for a `VIDEO_TS` folder
  it lists the title VOBs over 900 MB and joins them with FFmpeg's `concat` demuxer, each file's
  duration from ffprobe. A break inside a VOB is left to FFmpeg's transcode-time
  `dts_delta_threshold` (10 s), which does not help a seek. Plex plays no disc image; Emby only
  direct-plays one (F).
- **The same symptom elsewhere (F).** FFmpeg trac #1438, "Seeking in vob file with timestamp
  discontinuities fails"; ffmpeg-devel 2011-03, "DVD (MPEG2 Program Stream) with resetting
  DTS/PTS & SCR" (the clock drops to 0 partway); ffmpeg-user 2014-06, a 2-hour DVD read as 59
  minutes.

## 2. D404 against that practice

- **Agrees:** the navigation packs' VOBU start and end times are the source, as FFmpeg's; the
  offset carries on from where the last VOBU ended, as FFmpeg's; audio, video and private stream 1
  timestamps are all moved; the arithmetic is modulo 2^33.
- **Differs, by design:** D404 rewrites the bytes, SCR included, so FFmpeg's `mpegps` seek and
  every other reader of the stream see one clock. No project found does this; they offset packets
  inside the player.
- **Gaps it suggests:**
  1. **Breaks inside a cell, and small ones.** D404 compares cells only, with a second's slack.
     The stream's cells (the navigation pack's CellID) need not match the IFO's (libdvdnav docs),
     and FFmpeg checks every VOBU. Checking each VOBU join, with about a frame (3003 ticks) of
     slack, would catch both; it costs a read of every navigation pack once per title.
  2. **Sound at a join.** FFmpeg drops AC-3 frames that repeat or are cut short at a seamless join;
     D404 moves them and leaves them, so a join may click or repeat a frame.
  3. **The IFO's own flag.** libdvdread parses each cell's `stc_discontinuity` (V,
     `ifo_types.h`); nothing uses it, and discs are said to set it unreliably (F). It could serve
     as a cross-check.

## 3. Other DVD and Blu-ray pitfalls, most likely first

1. **Breaks inside a cell or under a second** (§2, gap 1). Small: the navigation packs are already
   read.
2. **Sound and subpictures at a join** (§2, gap 2). Medium: check that each stream's times move
   forward after the rewrite, or accept a click.
3. **The wrong main title, and decoy titles.** HandBrake takes the longest title, checks it
   against the menus (a title within 0.7 to 0.75 of the longest), and ignores menu buttons that
   last under 15 s (V). Structure-protected discs ("99 titles", many titles sharing cells) defeat
   any choice made without running the DVD's virtual machine (F); some discs will need a person to
   choose. Medium.
4. **Multi-angle titles (interleaved units).** Reading a cell from its first to its last sector
   plays every angle in turn; the next VOBU must be followed through the DSI's links (V,
   libdvdnav `dvdnav.c`). Medium; angle 1 alone is the safe default.
5. **The 2^33 wrap** (about 26.5 h). Rare on a DVD; D404's arithmetic already wraps.
6. **CSS.** FFmpeg's demuxer refuses it; without libdvdcss an encrypted disc must be decrypted
   first, as Orpheum already says (D350).
7. **NTSC soft telecine.** Film at 23.976 with repeat-field flags reads as 29.97; the filters
   matter (not read in source).
8. **Stills, padding cells and cell commands.** A joined stream ignores stills, as FFmpeg does;
   FFmpeg skips cells under a second.
9. **Chapters.** FFmpeg estimates them from the IFO unless it pre-indexes; Orpheum's reading of
   the navigation packs would give exact ones.
10. **Subtitle palettes and stream names.** The PGC's palette colours the subpictures; the IFO
    names each stream's language (AC-3 0x80+, DTS 0x88+, LPCM 0xA0+, subpicture 0x20+). HandBrake
    and FFmpeg both read them (V).
11. **Aspect.** From the title set's video attributes; the per-VOBU letterbox and pan-scan flags
    are unreliable.
12. **Blu-ray playlists of several clips** (D404 §5). FFmpeg's `bluray:` protocol (V) serves
    libbluray's joined stream; libbluray restamps nothing and marks each clip join. A PlayItem's
    `connection_condition` of 5 or 6 joins seamlessly, with one clock; 1 is a cut, where the clock
    may start again. A restamp there must rewrite PCRs, PTSs and DTSs inside transport-stream
    packets. FFmpeg's `mpegts` behaviour across such a join was not checked.
