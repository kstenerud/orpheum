// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

// The closed sets the schema's checks name (schema.md) that only identification writes: a title's
// source and its identification, the overridable fields, and the extra types (D162). A file's
// filing, a title's kind and an image's kind are library's (library/vocabulary.go).
const (
	sourceNFO  = "nfo"
	sourceTMDB = "tmdb"
	sourceIMDb = "imdb"
	sourceTVDB = "tvdb"
	sourceNone = "none"

	identificationIdentified   = "identified"
	identificationUnidentified = "unidentified"
	identificationConfirmed    = "confirmed"

	fieldName      = "name"
	fieldOverview  = "overview"
	fieldRuntimeMs = "runtime_ms"
	fieldYear      = "year"
	fieldTagline   = "tagline"
	// When an other video was recorded, in Unix milliseconds: a person's correction of the date its
	// file gave it (D349 §4).
	fieldRecordedAt = "recorded_at"

	extraTrailer    = "trailer"
	extraFeaturette = "featurette"
	extraInterview  = "interview"
	extraScene      = "scene"
	extraShort      = "short"
	extraClip       = "clip"
	extraUnknown    = "unknown"
)
