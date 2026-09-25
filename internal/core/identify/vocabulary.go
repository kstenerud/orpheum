// SPDX-License-Identifier: GPL-3.0-or-later

package identify

// The closed sets the schema's checks name (schema.md): a title's kind, its source and its
// identification, a file's filing, the overridable fields, and the extra types (D162).
const (
	kindMovie   = KindMovie
	kindEpisode = "episode"
	kindExtra   = "extra"

	sourceNFO  = "nfo"
	sourceTMDB = "tmdb"
	sourceIMDb = "imdb"
	sourceTVDB = "tvdb"
	sourceNone = "none"

	identIdentified   = "identified"
	identUnidentified = "unidentified"
	identConfirmed    = "confirmed"

	filingListed       = "listed"
	filingUnidentified = "unidentified"
	filingExtra        = "extra"

	fieldName     = "name"
	fieldOverview = "overview"
	fieldRuntime  = "runtime"
	fieldYear     = "year"
	fieldTagline  = "tagline"

	extraTrailer    = "trailer"
	extraFeaturette = "featurette"
	extraInterview  = "interview"
	extraScene      = "scene"
	extraShort      = "short"
	extraClip       = "clip"
	extraUnknown    = "unknown"

	itemCollection = "collection"
	// The language of an original text not known yet (item_text.lang).
	langUnknown = "und"
)
