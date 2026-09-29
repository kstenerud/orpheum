// SPDX-License-Identifier: GPL-3.0-or-later

package library

// The closed sets the schema's checks name (schema.md) that both the ingest and identification
// write or read: a file's filing (D163), a title's kind (D162), and the kinds of image an item
// shows. Defined here, the lower of the two packages, so each value is written once.
const (
	FilingListed       = "listed"
	FilingUnidentified = "unidentified"
	FilingExtra        = "extra"
	FilingAttached     = "attached" // a sidecar: it belongs to a video beside it
	FilingSkipped      = "skipped"
	FilingErrored      = "errored"

	TitleKindMovie   = "movie"
	TitleKindEpisode = "episode"
	TitleKindExtra   = "extra"

	ArtPoster   = "poster"
	ArtBackdrop = "backdrop"
	ArtLogo     = "logo"
	ArtStill    = "still"
)
