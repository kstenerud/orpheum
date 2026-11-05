// SPDX-License-Identifier: AGPL-3.0-or-later

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
	EpisodeGroupSource
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

// EpisodeGroupSource is how a provider lists a series' other orderings of its episodes, its episode
// groups, and each one's contents (episode-orderings.md §2.1, §3.4): the list comes with the series'
// own record where the provider allows, else from a request of its own; a group's contents are
// fetched only when someone needs it (D170).
type EpisodeGroupSource interface {
	// EpisodeGroupsIn is the list of a series' episode groups its own stored record carries, as
	// FetchEpisodeGroups returns it; false when the record does not carry one.
	EpisodeGroupsIn(seriesBody []byte) ([]byte, bool)
	// FetchEpisodeGroups returns the list of a series' episode groups, as stored.
	FetchEpisodeGroups(ctx context.Context, seriesID string) ([]byte, error)
	// ParseEpisodeGroups reads a stored list.
	ParseEpisodeGroups(body []byte) ([]EpisodeGroup, error)
	// FetchEpisodeGroup returns one episode group's contents, its parts and their episodes, as stored.
	FetchEpisodeGroup(ctx context.Context, groupID string) ([]byte, error)
	// ParseEpisodeGroup reads a stored group's contents.
	ParseEpisodeGroup(body []byte) ([]EpisodeGroupPart, error)
}

// EpisodeGroup is one of a series' episode groups as its list says it: its id, its name, its kind
// (Kind's values, "dvd" for TMDB's type 3) and how many episodes it lists.
type EpisodeGroup struct {
	ID           string
	Name         string
	Kind         string
	EpisodeCount int
}

// EpisodeGroupPart is one part of an episode group (a disc, a volume): its order among the parts,
// its name, and its episodes in order.
type EpisodeGroupPart struct {
	Order    int
	Name     string
	Episodes []EpisodeGroupEntry
}

// EpisodeGroupEntry is one episode of a part: its id at the provider, the one a season listing gives
// it, and its order in the part, from 0.
type EpisodeGroupEntry struct {
	EpisodeID string
	Order     int
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
	// Votes are how many people rated the work at the provider: what settles two works of one name
	// in one year (D301).
	Votes int
	// RuntimeMs is the work's runtime, which its details give and a search's results do not: read
	// for a film's candidates its name admits, to fit them to its files (D365 §4); 0 when unknown.
	RuntimeMs int64
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
	// ReleaseDate is a film's release date or a series' first air date, ISO 8601, as the provider
	// gives it; "" when it gives none (D366 §1).
	ReleaseDate string
	RuntimeMs   int64
	// IDs are the work's ids at every provider the response names, this one's included.
	IDs map[string]string
	// Artwork are the images the provider has for the work, fetched later (D212).
	Artwork []ImageRef
	// SeasonNumbers are a series' seasons the provider lists, the specials' 0 among them, each a
	// listing FetchSeason fetches (D307 §2).
	SeasonNumbers []int
	// Genres are the work's genres by name, in the provider's order.
	Genres []string
	// Credits are the work's cast, in billing order, and its directors and writers.
	Credits []Credit
}

// The roles a credit names, as credit.role stores them (step 0067).
const (
	RoleCast     = "cast"
	RoleDirector = "director"
	RoleWriter   = "writer"
)

// Credit is one person's part in a work, as its source says it: a provider's response or an NFO.
type Credit struct {
	Role string // RoleCast, RoleDirector or RoleWriter
	Name string
	// PersonID is the person's id at the provider whose response named them; "" for one an NFO
	// names, who is found again by their name.
	PersonID string
	// Character is a cast member's part; "" for crew, or a part not named.
	Character string
	// PictureRef is the person's picture as the source names it, a provider's path or an NFO's
	// thumb; not fetched.
	PictureRef string
}

// ImageRef is one image a provider's record names: its kind and the provider's reference to it.
type ImageRef struct {
	Kind string // library.ArtPoster, ArtBackdrop, ArtLogo, ArtStill
	// SeasonNumber is, for a series' season's poster, its season in the series' aired ordering, the
	// specials' 0 among them (D379); nil for an image of the work itself.
	GroupNumber *int64
	Ref         string // the provider's path, which its image service resolves
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
