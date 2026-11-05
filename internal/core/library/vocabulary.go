// SPDX-License-Identifier: AGPL-3.0-or-later

package library

// The closed sets the schema's checks name (schema.md) that both the ingest and identification
// write or read: a file's filing (D163), a title's kind (D162), and the kinds of image an item
// shows. Defined here, the lower of the two packages, so each value is written once.
const (
	// How a kind of library's identification filed a video (file_filing, D363 §5).
	FilingListed       = "listed"
	FilingUnidentified = "unidentified"
	FilingExtra        = "extra"
	// How a scan filed what it found at a place (file.filing): a video, which each kind of library
	// that covers it files; a copy of a video found first at another place, that video's place (D363
	// §2); a sidecar, which belongs to a video beside it; and a path skipped or that could not be read.
	FilingVideo    = "video"
	FilingCopy     = "copy"
	FilingAttached = "attached"
	FilingSkipped  = "skipped"
	FilingErrored  = "errored"

	// LibraryKindMovies is a library of films, whose names may also make episodes (D363 removed the
	// mixed kind).
	LibraryKindMovies = "movies"
	// LibraryKindShows is a series library, every video of which is an episode of a series (the
	// owner, 2026-10-13).
	LibraryKindShows = "shows"
	// LibraryKindOther is a library of videos that are neither films nor series, home movies,
	// concerts, lectures and clips, every one a title of the kind other and none identified online
	// (D349 §3).
	LibraryKindOther = "other"

	TitleKindMovie   = "movie"
	TitleKindEpisode = "episode"
	TitleKindExtra   = "extra"
	// TitleKindOther is a video of an other library: one file, never matched (D349 §3).
	TitleKindOther = "other"

	// The kinds itemKindCollection and itemKindPerson are an item's kinds beside a title (D303).
	itemKindCollection = "collection"
	itemKindPerson     = "person"

	ArtPoster   = "poster"
	ArtBackdrop = "backdrop"
	ArtLogo     = "logo"
	ArtStill    = "still"

	// ArtSourceFrame is the source of a frame still the node took from an item's own video: the last
	// resort of a film, an episode or an other video nothing else gives a picture (D362).
	ArtSourceFrame = "frame"
)

// MetadataSourceTMDB is a library that takes its data from TMDB only; the others take NFO files
// first, or only (D184).
const MetadataSourceTMDB = "tmdb"

// MetadataSourceNFO is a library that takes its data from NFO files only, as every other library does
// (D349 §10).
const MetadataSourceNFO = "nfo"

// PrefersImagesBesideTheMedia is whether a library shows an image beside the media over its
// provider's of the same kind: every library but one that takes TMDB only (D215 §1), as
// ListShownArtwork orders them.
func PrefersImagesBesideTheMedia(metadataSource string) bool {
	return metadataSource != MetadataSourceTMDB
}
