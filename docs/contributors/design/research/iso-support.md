> **ABOUTME:** Research for D350, the owner's ask to play DVD and Blu-ray disc images (.iso): how
> other servers and FFmpeg read them, what our pinned FFmpeg can do, how an image fits the node's
> pipeline (probe, read-through, chunks, fdproxy, Landlock), and the recommended approach with its
> cost. Read before changing internal/node/disc or how a film is served to FFmpeg.

# ISO disc images

- **Date:** 2026-10-26.
- **Requested by** the owner, 2026-10-26: *"I also want iso support now."* This revisits
  [D306](../../decisions/working-notes.md#d306--the-scanner-is-tested-on-names-of-our-own-places-an-unnumbered-episode-by-a-guessed-title-match-and-keeps-watching-by-title-and-edition)
  §5, which kept disc images out of the beta and listed each as skipped,
  `disc_image_not_supported_yet` (renamed `disc_folder_not_supported_yet` once an image is read).
- **Decision:** [D350](../../decisions/working-notes.md#d350--a-disc-image-plays-its-main-title-read-by-the-node-from-the-images-own-structure-and-served-to-ffmpeg-through-fdproxy).
- Web pages read are noted under `/yoloai/cache/iso-research/`; the libraries' sources read are
  libudfread, libdvdread and libbluray (VideoLAN, `master` of 2026-10-26) and jellyfin
  (`master`), cloned under `/yoloai/cache/git/`.

## 1. What a disc image is

An `.iso` of a video disc is the disc's filesystem, byte for byte: **UDF** (ECMA-167 with OSTA's
profile; a DVD's is UDF 1.02, usually with an ISO 9660 bridge; a Blu-ray's is UDF 2.50, with a
*metadata partition* holding its file entries and directories). In it:

- **DVD-Video:** `VIDEO_TS/VIDEO_TS.IFO` lists the disc's titles (its TT_SRPT: each title's title
  set, its number there, chapters and angles). Each title set has `VTS_nn_0.IFO`, describing its
  program chains (PGCs): their cells in play order, each cell a range of sectors of the set's
  title VOBs, `VTS_nn_1.VOB` … `VTS_nn_9.VOB` (MPEG-2 program stream, read as one), with a
  playing time; its audio and subpicture streams' languages and codecs; and each PGC's 16-colour
  subpicture palette. Menus are VOBs and PGCs of their own (`VIDEO_TS.VOB`, `VTS_nn_0.VOB`).
- **Blu-ray:** `BDMV/PLAYLIST/nnnnn.mpls` are the playlists: play items, each a clip
  (`BDMV/STREAM/nnnnn.m2ts`, MPEG-2 transport stream with a 4-byte timestamp before each packet)
  between an in and an out time; the streams' table (STN) with each stream's PID, codec and
  language; and the playlist marks, its chapters. `BDMV/CLIPINF/nnnnn.clpi` maps times to packets.
  Menus are HDMV or BD-J programs (`BDMV/MovieObject.bdmv`, `BDMV/BDJO`, Java).

A **disc's film is its main title**: the one longest title (DVD) or playlist (Blu-ray). Everything
else is menus, trailers, warnings and extras.

**Encryption.** A pressed DVD is CSS-scrambled and a Blu-ray AACS-encrypted; an image taken with
`dd` keeps that. Images in libraries are almost all made by tools that decrypt as they copy
(MakeMKV's backup mode, AnyDVD, DVD Decrypter). Decrypting is libdvdcss's and libaacs's business,
which raises legal questions in some countries; no server in §2 does it by default. An encrypted
image is recognisable cheaply: a DVD's PES packets carry a scrambling flag, and an AACS aligned
unit's first transport packet carries copy-permission bits and the packets after it lose their
sync bytes.

## 2. How others do it

| Server | DVD image | Blu-ray image | Title | Menus |
| --- | --- | --- | --- | --- |
| Plex | no | no | — | — |
| Jellyfin | not officially (an `.iso` is a video, typed by its UDF folders) | FFmpeg's `bluray:` protocol on the image | libbluray's longest playlist | never |
| Emby | as Jellyfin's ancestor | `bluray:` | longest | never |
| Kodi | libdvdnav | libbluray (+ BD-J with a JRE) | "Play main title", or the disc's menu | yes, best effort |

- **Plex** refuses every disc format (ISO, IMG, VIDEO_TS, BDMV): "these formats just don't fit into
  the idea behind the Plex ecosystem and hide content behind built-in menu systems" (Plex support
  article 201426506); people convert with MakeMKV.
- **Jellyfin** (read in its source): `BaseVideoResolver.SetIsoType` opens the image with
  DiscUtils' `UdfReader` and types it by its `VIDEO_TS` or `BDMV` folder; a Blu-ray image is then
  read by FFmpeg as `bluray:<path>`, a DVD folder as a `concat:` of its VTS VOBs larger than 900 MB
  (`GetPrimaryPlaylistVobFiles`: a size heuristic, no IFO read), and a Blu-ray folder by BDInfo's
  playlist. Its forum calls ISOs "explicitly unsupported"; DVD images commonly fail. Never direct
  play, never menus.
- **Kodi** plays images through libdvdnav and libbluray with their menus; its "simplified menu"
  offers *Play main title*, *Select from all titles* or *Show Blu-ray menu*, and the main title is
  the reliable path (BD-J menus are "experimental at best").
- **HandBrake and MakeMKV** take the longest title as the film, with MakeMKV adding heuristics for
  Blu-rays whose publishers obfuscate with hundreds of near-identical playlists.

Every server that plays images plays **the main title, menus skipped**, and transcodes or remuxes
it: there is no direct play of a disc.

## 3. FFmpeg's readers, and our build

- **`dvdvideo`** demuxer (FFmpeg 7.0+, by Marth64): libdvdnav plays, libdvdread parses; needs
  `--enable-libdvdnav --enable-libdvdread` (GPL). It takes a title (default 1, "not always the main
  feature, validation suggested"), chapters, an angle, or a PGC; it keeps timestamps continuous
  across cells, names streams' languages, and passes the palette. Its seeking is, in its own
  words, "inherently unreliable" (`dvdnav_time_search`), and it opens its input **by path**
  (`DVDOpen2(…, s->url)`).
- **`bluray:`** protocol (libbluray, with libudfread for images): selects the longest playlist by
  default, an angle and a chapter, and serves the playlist's clips as one byte stream for the
  `mpegts` demuxer. It names no language (libbluray's STN is not passed on), and it opens **by path**
  (`bd_open(diskname, NULL)`).
- **Our pinned build** (ffmpeg-fdproxy 8.1.2-5-2, jellyfin-ffmpeg's builder) has `--enable-libbluray`
  with libudfread, so the `bluray` protocol is there; it has **no libdvdnav or libdvdread**, so no
  `dvdvideo`. Its demuxers `mpeg` (program stream, what a VOB is) and `mpegts` (with M2TS's 192-byte
  packets) read both discs' streams.

## 4. What our pipeline asks of a film

- **FFmpeg is given no path** ([D330](../../decisions/working-notes.md#d330--ffmpeg-reads-its-input-through-the-node-over-a-protocol-of-our-own-patched-into-jellyfin-ffmpeg)):
  every read is a `READ` over fdproxy that the node answers from the file it opened, through its
  disk's gate ([D329](../../decisions/working-notes.md#d329--work-is-scheduled-from-wants-each-resource-is-decided-next-to-it-playback-is-admitted-and-long-work-is-cut-into-pieces)),
  and Landlock grants the run no path to media. Both FFmpeg disc readers open a path: through them,
  an image could only be read by path, which gives up D330 for every disc (a dead share then holds
  FFmpeg in the kernel; reads bypass the scheduler).
- **Hostile bytes stop at the node** ([D126](../../decisions/working-notes.md#d126--hostile-bytes-never-reach-the-core-hostile-strings-reach-it-constantly-and-that-is-the-design)):
  whatever reads a disc's structure must be bounded, and is fuzzable when it is Go.
- **Chunks are cut on the film's grid** (D247 §10, D259): a film without an index of keyframes
  (an MPEG-TS, a program stream) has its keyframes found by a read-through, and each chunk run seeks
  to its chunk. A seek must land where asked: `dvdvideo`'s cannot be trusted, an `mpeg`/`mpegts`
  seek by timestamp over a continuous stream can (it is how a `.vob`, `.mpg` or `.m2ts` in a library
  plays today).
- **Direct play is never possible**: the bytes a browser would fetch are the image's. The core must
  never choose direct play for one, even when a person prefers it.
- **Subtitles**: a DVD's are bitmaps (`dvd_subtitle`), a Blu-ray's PGS bitmaps; bitmap subtitles are
  not offered at all (cut-a §1), so a disc's subtitles wait for that work like any MKV's PGS.
  Their languages are still worth recording.
- **Audio**: AC-3, DTS, LPCM, MPEG audio (DVD); AC-3, E-AC-3, DTS(-HD), TrueHD, LPCM (Blu-ray):
  prepared as any film's are. Languages come only from the IFO and the playlist: neither stream
  carries them.

## 5. The options

### A. The node reads the disc's structure and serves FFmpeg the main title (recommended)

The node parses the image itself, in Go with the standard library: UDF (with the metadata
partition), then the DVD's IFOs or the Blu-ray's playlists; it picks the main title and turns it
into a list of byte ranges of the image (the title's cells' sectors, or its clips' files). The
film FFmpeg is served over fdproxy is **those ranges, joined**: to FFmpeg it is a `.vob` or an
`.m2ts`, read by the `mpeg` or `mpegts` demuxer exactly as such a file in a library is read today.

- **Fits everything in §4 unchanged:** no path, every read through the disk's gate, Landlock as
  it is, the read-through and chunk ladder as for any program or transport stream. The probe record
  gets the disc's duration, chapters and languages, which ffprobe cannot see.
- **No FFmpeg change, no new dependency;** works with the pinned build today.
- **Cost:** about 1,500 lines of Go with tests: UDF ~350, IFO ~350, MPLS ~300, the title and
  encryption check ~150, a test image writer ~400; a day's work, built in this change.
- **Limits** (each says why, rather than playing wrong):
  - Menus never; the main title only. Other titles (extras) could later be offered as extras.
  - A DVD title whose cells are **interleaved** (multiple angles, seamless branching) is refused:
    its sectors hold the other angles' units too, and following them means reading each ILVU's
    navigation packet, which a later step can add.
  - A Blu-ray playlist of several play items is served as its **whole clips**, in order: an in or out
    point inside a clip is not cut (CLPI's map is not read yet), and a clip's timestamps restart
    where the next begins. Most films are one clip; a seamless-branching disc is a VQ to measure.
  - An **encrypted** image is refused as encrypted.
  - Obfuscated Blu-rays (hundreds of playlists of one length) may pick a wrong playlist of the
    right length; a later heuristic or a person's choice of title would fix that.

### B. FFmpeg reads the image by path

`-f dvdvideo -title N` and `bluray:<path>`, the run granted read of the one image.

- **Cost:** rebuild FFmpeg with libdvdread and libdvdnav (two builder scripts in ffmpeg-fdproxy, a
  new pin; an owner dependency and build change); the node still needs the IFO parsed to pick a
  title (dvdvideo defaults to title 1) and the playlist parsed for languages.
- **Gives up D330 and D329 for discs**: the run opens the share itself. Seeking in `dvdvideo` is
  unreliable, which the chunk ladder cannot live with. Rejected.

### C. FFmpeg's readers patched to read over fdproxy

Patch `dvdvideodec.c` to open libdvdread through `DVDOpenStream` over an AVIOContext, and
`bluray.c` to open libbluray through `bd_open_files` over libudfread's `udfread_open_input`, both on
the fdproxy input.

- Keeps D330, and gives menus-free playback with FFmpeg's own timestamp handling.
- **Cost:** two FFmpeg patches (~300 lines of C with their tests and fuzzing in ffmpeg-fdproxy),
  the two libraries added to the build, a new pin; still `dvdvideo`'s seeking, so the chunk ladder
  would need a remux to a seekable file first. Far more than A for less.

## 6. Recommendation

**A.** It is the approach that keeps the node's guarantees, needs nothing from the owner's build,
and treats a disc as what its main title is: a program or transport stream the node already plays.
Its limits are honest refusals that later steps can lift one by one (interleaved DVD cells by
following ILVUs; Blu-ray in/out points by reading CLPI; a person's choice of title; extras as
extras), each with a VQ against a real disc. Option C remains open if a disc ever needs FFmpeg's
navigation, and nothing in A stands in its way.

Order of the build:

1. `internal/node/disc`: UDF, IFO and MPLS readers, the main title, the encryption check; a test
   image writer; fuzzing.
2. The node serves a disc image's main title to every run (probe, read-through, chunks, prepare,
   subtitle extraction) through fdproxy as the title's ranges; a node that cannot stream refuses a
   disc with the reason; the probe record carries the disc's kind, duration, chapters and languages.
3. The core reads `.iso` as a video (disc folders, `VIDEO_TS` and `BDMV`, stay skipped), and never
   plays a disc direct.
4. VQs on real images: a DVD, a Blu-ray, and a seamless-branching or multi-clip Blu-ray.

**Not proposed:** decrypting (libdvdcss, libaacs); menus; disc folders (`VIDEO_TS`, `BDMV`) — the
same reader could serve them later from the folder's files, since a folder is the same structure
unpacked.
