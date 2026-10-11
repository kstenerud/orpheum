// SPDX-License-Identifier: AGPL-3.0-or-later

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

// MetadataSourceTMDB is a library that takes its data from TMDB only; the others take NFO files
// first, or only (D184).
const MetadataSourceTMDB = "tmdb"

// PrefersImagesBesideTheMedia is whether a library shows an image beside the media over its
// provider's of the same kind: every library but one that takes TMDB only (D215 §1), as
// ListShownArtwork orders them.
func PrefersImagesBesideTheMedia(metadataSource string) bool {
	return metadataSource != MetadataSourceTMDB
}
