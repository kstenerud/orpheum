// SPDX-License-Identifier: AGPL-3.0-or-later

// Package artwork re-encodes images for the core (D212): a provider's download, a file beside
// the media, an image in a container, a still it takes of a video's frames (D362). Every image the
// core stores is one this package's run of FFmpeg wrote, whatever it arrived as, so nothing a client
// decodes was written by anyone else; a source is decoded only here, under FFmpeg's confinement.
//
// # Invariants
//
//   - ffprobe's JSON is another program's (D458): read by strictjson.Foreign, a key said twice
//     keeping its first value, as the probe package reads it (ffprobeOutput).
//     TestArch_JSONIsDecodedStrictly.
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx and TestArch_OnlyExecxImportsOsExec.
//   - A source reaches FFmpeg on a pipe, never as a name, with every other protocol refused.
//     TestDerive_FFmpegReadsTheSourceOnlyFromThePipe.
//   - What is not an image, and an image over the pixel limit, is refused before it is decoded;
//     the output is our own WebP, the first frame only, with none of the source's metadata.
//     TestDerive_RefusesWhatIsNotAnImageOrTooLarge,
//     TestDerive_AJPEGPosterBecomesOurOwnWebPAtEverySizeItIsLargeEnoughFor (integration tier).
//   - Nothing is scaled up; posters and backdrops are cropped to their exact shape, logos fit
//     their box; every image gets its shape's smallest size. TestPlan_NeverUpscalesAndAlwaysMakesTheSmallest,
//     TestDerive_ALandscapeSourceIsCroppedToTheExactShape (integration tier).
//   - A still is taken from the film it is given and nothing else, as every run reads one (execx.Film:
//     the fdproxy protocol alone, or the file protocol alone), from its place for a bounded stretch;
//     black pictures are dropped before the thumbnail filter chooses, and it is one PNG of FFmpeg's
//     own with no metadata, which Derive then re-encodes as any image; a stretch with no picture
//     that is not black is ErrNoFrame, not a failure (D362 §1, §5).
//     TestStill_FFmpegReadsOnlyTheFilmFromItsPlaceAndSkipsTheBlack, TestStill_NothingWrittenIsAVideoWithNoFrame,
//     TestStill_AVideosStillSkipsItsBlackLeadInAndBecomesOurOwnWebP (integration tier).
//
// # Read before changing
//
//   - D212: artwork, re-encoded by the node, sizes and shapes.
//   - D99: the core does not decode an image it did not make.
//   - principles §22: untrusted data is cleaned where it enters.
//   - docs/contributors/design/plans/artwork.md.
//   - D362: a still of a video's frames, the last resort of a video nothing gives a picture.
//
// # Where to change
//
//   - derive.go: the probe, the planning of sizes, and FFmpeg's arguments. The size table is
//     internal/nodeproto's Shape.Sizes, shared with the core.
//   - still.go: taking a still from a video: where, how long a stretch, what counts as black.
package artwork
