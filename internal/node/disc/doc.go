// SPDX-License-Identifier: AGPL-3.0-or-later

// Package disc reads a DVD's or a Blu-ray's image (an .iso) for its main title, its film (D350):
// the image's UDF volume (UDF 1.02 on a DVD, UDF 2.50 with its metadata partition on a Blu-ray),
// then the DVD's IFOs or the Blu-ray's playlists. The title is the runs of the image that hold its
// stream, in play order, which joined are a program stream or an M2TS transport stream, as FFmpeg
// reads a .vob or an .m2ts; and what only the disc says of it: its duration, chapters and its
// streams' languages. It reads through an io.ReaderAt, so the caller decides how the image is read.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Removes, renames and truncates nothing. Enforced by the forbidigo rule tagged [delete].
//   - A DVD's main title is its longest, served as its cells in play order wherever they lie, from
//     title VOBs cut across files and files cut into extents, with its chapters at its programs'
//     cells and its streams' languages by the identifiers FFmpeg's program stream demuxer gives
//     them. TestMainTitle_ADVDsLongestTitleIsItsCellsInPlayOrder.
//   - Of an angle block only the first angle is served; an interleaved title is refused
//     (ErrInterleaved). TestMainTitle_ADVDsOtherAnglesAreLeftOutAndInterleavedCellsRefused.
//   - A DVD title whose cells' clock starts again partway (a cell starting before the one before it
//     ended, or more than a second after) carries its clock shifts, and Restamp moves every SCR, PTS
//     and DTS from there to carry on, so FFmpeg seeking by time finds where the title plays it
//     (D404). TestMainTitle_ADVDsClockStartedAgainIsMovedToCarryOn.
//   - A Blu-ray's main title is its longest playlist that plays no clip twice, served as its clips
//     whole in play order, with its chapters across them and its streams' languages by PID, read
//     through the metadata partition. TestMainTitle_ABluRaysLongestPlaylistIsItsClipsInOrder.
//   - A broken title, a title set without its IFO or a playlist that is no MPLS file, is passed over
//     for the others. TestMainTitle_ABrokenTitleIsPassedOverForTheOthers.
//   - An encrypted title (AACS, CSS) is refused (ErrEncrypted).
//     TestMainTitle_AnEncryptedTitleIsRefused.
//   - What is not a video disc's image is ErrNotADisc, and a read the storage failed is a
//     *ReadError. TestMainTitle_SaysWhatIsNotADiscAndWhatCouldNotBeRead.
//   - Whatever an image holds, reading it ends and never panics, every title run inside the image:
//     each structure is bounded, each offset checked, and the bytes read to find the title are
//     budgeted (D126). FuzzMainTitle.
//
// The images the tests read are written by disctest, from the specifications; Linux's UDF driver
// mounts them and reads each file as this package does (checked by hand, 2026-10-26: short
// allocation descriptors are in their entry's partition, so a Blu-ray's files, whose entries are
// in the metadata partition, give their data by long ones).
//
// # Read before changing
//
//   - D350: the main title, served through fdproxy; what is refused.
//   - D404: a DVD title served on one clock.
//   - D126: hostile bytes stop at the node.
//   - docs/contributors/design/research/iso-support.md: the formats, and how others read them.
//
// # Where to change
//
//   - read.go: bounds, errors and bounded reads. udf.go and fileentry.go: the UDF volume, its
//     directories and files. dvd.go: a DVD's titles. bluray.go: a Blu-ray's playlists.
//     encrypted.go: telling an encrypted title. clock.go: a DVD title's clock, and restamping it.
//     title.go: the Title, and choosing which disc.
//     disctest: images for tests.
package disc
