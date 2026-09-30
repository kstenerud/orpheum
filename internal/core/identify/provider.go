// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"errors"
)

// Provider is a metadata source behind the interface that is the future plugin host-function
// list (identification.md, How a provider is called in Cut A). A response is fetched once and
// stored; Parse re-derives a title from the stored bytes without a call (D170, D182).
type Provider interface {
	WorkSource
	SeasonSource
}

// WorkSource is what a provider says about films and series: search, find, fetch and read.
type WorkSource interface {
	// Name is the provider's name as title.followed_source and item_provider_id store it: "tmdb".
	Name() string
	// Search lists the works a name and year may be, best first.
	Search(ctx context.Context, query Query) ([]Candidate, error)
	// Find turns another provider's id ("imdb", "tt0123456") into this provider's.
	Find(ctx context.Context, kind, source, id string) (string, error)
	// Fetch returns a work's full response, as stored: a film's, or a series' own.
	Fetch(ctx context.Context, kind, id string) ([]byte, error)
	// Parse reads a stored response.
	Parse(kind string, body []byte) (Metadata, error)
}

// SeasonSource is how a provider lists a series' episodes, a season at a time (D209).
type SeasonSource interface {
	// FetchSeason returns a series' listing of one season's episodes, as stored (D209).
	FetchSeason(ctx context.Context, seriesID string, season int) ([]byte, error)
	// ParseSeason reads a stored season listing.
	ParseSeason(body []byte) ([]Episode, error)
}

// The kinds of work a provider is asked about: a title's kind, or a series.
const (
	KindMovie  = "movie"
	KindSeries = "series"
)

// Episode is one episode in a season listing: what an episode takes from its series' source
// (D209).
type Episode struct {
	ID        string
	Season    int
	Number    int
	Name      string
	Overview  string
	Year      int
	RuntimeMs int64
	// StillRef is the provider's reference to the episode's still image, or "".
	StillRef string
}

// Query is what a search asks for.
type Query struct {
	Kind string // KindMovie or KindSeries
	Name string
	Year int // zero when unknown
}

// Candidate is one search result, kept for the chooser (D172).
type Candidate struct {
	ID                 string
	Name               string
	OriginalName       string
	Year               int
	ProviderPosterPath string
}

// Metadata is what a provider says about a work.
type Metadata struct {
	ID           string
	Name         string
	OriginalName string
	OriginalLang string
	Overview     string
	Tagline      string
	Year         int
	RuntimeMs    int64
	// IDs are the work's ids at every provider the response names, this one's included.
	IDs map[string]string
	// Artwork are the images the provider has for the work, fetched later (D212).
	Artwork []ImageRef
}

// ImageRef is one image a provider's record names: its kind and the provider's reference to it.
type ImageRef struct {
	Kind string // library.ArtPoster, ArtBackdrop, ArtLogo, ArtStill
	Ref  string // the provider's path, which its image service resolves
}

// Errors a provider returns, wrapped.
var (
	// ErrNoMatch is a lookup that found nothing: a Find with no result, or a Fetch of an id the
	// provider does not have.
	ErrNoMatch = errors.New("the provider has no such work")
	// ErrNoKey is a provider that cannot be called until its key is set.
	ErrNoKey = errors.New("the provider's key is not set")
	// ErrKeyRefused is a key the provider refused.
	ErrKeyRefused = errors.New("the provider refused its key")
)
