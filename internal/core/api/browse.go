// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

const tagBrowse = "Browse"

// EntryBody is one film, series or other video in a list.
type EntryBody struct {
	ID                  int64  `json:"id"`
	Kind                string `doc:"What it is: a film, a series, or an other video (D349)" enum:"movie,series,other" json:"kind"` //nolint:lll // one struct tag
	Name                string `json:"name"`
	Year                int64  `json:"year,omitempty"`
	IdentificationState string `json:"identificationState"`
	// IsGuessed is whether its match is a guess no person confirmed (D301 §4).
	IsGuessed    bool   `json:"isGuessed,omitempty"`
	AddedAt      int64  `json:"addedAt"`
	PosterSha256 string `doc:"Its poster's hash: GET /bytes/art/{sourceSha256}/portrait-m.webp" json:"posterSha256,omitempty"` //nolint:lll // one struct tag
	// LandscapeSha256 is an other video's 16:9 picture: its still, else its backdrop (D349).
	LandscapeSha256 string `doc:"An other video's 16:9 picture: GET /bytes/art/{sourceSha256}/landscape-s.webp" json:"landscapeSha256,omitempty"` //nolint:lll // one struct tag
	// NoPosterStillSha256 is, for a film or series with no poster, the 16:9 picture shown in the top of
	// its poster's place (D362).
	NoPosterStillSha256 string `doc:"A film's or series' 16:9 picture when it has no poster, shown above the mark of a missing poster: GET /bytes/art/{sourceSha256}/landscape-s.webp" json:"noPosterStillSha256,omitempty"` //nolint:lll // one struct tag
	// Recorded is when an other video was recorded, and where that came from; absent for the rest.
	Recorded *RecordedBody `json:"recorded,omitempty"`
	// Unavailable says why none of its files can be read now; absent when one can (D241 §3).
	Unavailable          *UnavailableBody `json:"unavailable,omitempty"`
	WatchedPercent       int64            `doc:"How far, in percent, the person is through a film, where Continue Watching would resume it; absent for a series, and for a film they are not partway through, have hidden since, or whose file's length is not known" json:"watchedPercent,omitempty"` //nolint:lll // one struct tag
	EpisodeCount         int64            `doc:"How many episodes a series has with a file, in the ordering the person watches it in; absent for a film" json:"episodeCount,omitempty"`                                                                                                                //nolint:lll // one struct tag
	FinishedEpisodeCount int64            `doc:"How many of those episodes the person watched, by playing or by hand (D370); absent when none, and for a film" json:"finishedEpisodeCount,omitempty"`                                                                                                  //nolint:lll // one struct tag
	IsWatched            bool             `doc:"Whether the person watched it: a film or other video by playing it to its finish or by hand, a series once they watched every episode it counts (D370)" json:"isWatched,omitempty"`                                                                    //nolint:lll // one struct tag
}

// RecordedBody is when an other video was recorded, where that came from, and its file's name (D349
// §4).
type RecordedBody struct {
	RecordedAt int64  `doc:"Unix milliseconds" json:"recordedAt"`
	From       string `doc:"Its container's recorded-time tag, a date in its file's or a folder's name, the file's own time, or a person's correction" enum:"tag,name,file,person" json:"from"` //nolint:lll // one struct tag
	FileName   string `doc:"Its file's name" json:"fileName"`
}

// recordedBody is an other video's date as the API says it, or nil for the rest.
func recordedBody(facts *library.OtherFacts) *RecordedBody {
	if facts == nil {
		return nil
	}
	return &RecordedBody{RecordedAt: facts.RecordedAtMs, From: facts.RecordedFrom, FileName: facts.FileName}
}

// entryBody is a list's entry as the API says it.
func entryBody(entry library.Entry, nodeName func(nodeID int64) string) EntryBody {
	return EntryBody{
		ID: entry.ID, Kind: entry.Kind, Name: entry.Name, Year: entry.Year,
		IdentificationState: entry.IdentificationState, IsGuessed: entry.IsGuessed,
		AddedAt: entry.AddedAt, PosterSha256: entry.PosterSha256, LandscapeSha256: entry.LandscapeSha256,
		NoPosterStillSha256: entry.NoPosterStillSha256,
		Recorded:            recordedBody(entry.Other), Unavailable: unavailableBody(entry.Unreadable, nodeName),
		WatchedPercent: entry.WatchedPercent, EpisodeCount: entry.EpisodeCount,
		FinishedEpisodeCount: entry.FinishedEpisodeCount, IsWatched: entry.IsWatched,
	}
}

// UnavailableBody is why what is listed cannot be played now: its machine away, or its folder or
// disk missing, and since when (D241 §3).
type UnavailableBody struct {
	Reason   string `enum:"node_away,root_missing,disk_missing,still_reading" json:"reason"`
	NodeName string `doc:"The machine that holds it" json:"nodeName"`
	Path     string `doc:"The folder or disk missing on it; absent for a machine away" json:"path,omitempty"`
	// UnavailableAt is when it became unavailable, in Unix milliseconds; absent when not known.
	UnavailableAt int64 `json:"unavailableAt,omitempty"`
}

// unavailableBody is why something cannot be read now as the API says it, or nil when it can, its
// node named by nodeName.
func unavailableBody(unreadable *library.Unreadable, nodeName func(nodeID int64) string) *UnavailableBody {
	if unreadable == nil {
		return nil
	}
	body := &UnavailableBody{
		Reason: string(unreadable.Reason), NodeName: nodeName(unreadable.NodeID), Path: unreadable.Path,
	}
	if !unreadable.Since.IsZero() {
		body.UnavailableAt = unreadable.Since.UnixMilli()
	}
	return body
}

// ArtBody is the images an item shows, by the source SHA-256 GET /bytes/art/{sourceSha256}/{variant} takes.
type ArtBody struct {
	PosterSha256   string `json:"posterSha256,omitempty"`
	BackdropSha256 string `json:"backdropSha256,omitempty"`
	StillSha256    string `doc:"An episode's frame" json:"stillSha256,omitempty"`
}

func artBody(art library.Art) ArtBody {
	return ArtBody{PosterSha256: art.PosterSha256, BackdropSha256: art.BackdropSha256, StillSha256: art.StillSha256}
}

// TitlesInput asks for one page of films, series or other videos, of every library the person
// reaches or of one (D349).
type TitlesInput struct {
	AuthorizationHeader
	Kind    string `doc:"movie, series or other; any: every kind, every library's" enum:"movie,series,other,any" query:"kind" required:"true"`                                                                                                                                                                                                                                                                                                                                                                                                                 //nolint:lll // one struct tag
	Sort    string `doc:"name: in the library's locale. added: newest first, by the scan that found it, most recent first, then by its file's own modified time, newest first; a film is as new as the scan that first found it, a series as its newest episode. recorded: newest recorded first, and file_name: by the files' names, other videos only. released: newest first by a film's release date or a series' first air date, those with none last, films and series only" enum:"name,added,recorded,file_name,released" query:"sort" required:"true"` //nolint:lll // one struct tag
	Order   string `doc:"The way the sort runs (D366): ascending, A to Z or oldest first; descending, Z to A or newest first; absent for the sort's own, A to Z by a name and newest first by a date. What has no date is last either way" enum:"ascending,descending" query:"order"`                                                                                                                                                                                                                                                                          //nolint:lll // one struct tag
	Library int64  `doc:"One library's alone; absent for every library the person reaches, less, for films and series, those they left out (leftOutLibraryIds)" minimum:"0" query:"library"`                                                                                                                                                                                                                                                                                                                                                                   //nolint:lll // one struct tag
	IsHome  bool   `doc:"The list as a row of Home shows it, with no library: of every library the person reaches less those they left out of Home (leftOutOfHomeLibraryIds, D366), in place of those left out of Films and Series" query:"home"`                                                                                                                                                                                                                                                                                                              //nolint:lll // one struct tag
	Cursor  string `doc:"From the previous page's nextCursor" query:"cursor"`
	From    string `doc:"A name, by name or file name only and with no cursor: A to Z, the page starts at the first title sorted at or after it, as a letter of the alphabet does; Z to A, at the last title sorted before it, so the letter after a letter starts that letter's titles" maxLength:"100" query:"from"` //nolint:lll // one struct tag
	Limit   int64  `maximum:"500" minimum:"1" query:"limit" required:"true"`
}

// TitlesOutput is one page.
type TitlesOutput struct {
	Body struct {
		Titles     []EntryBody `json:"titles"`
		NextCursor string      `doc:"Empty on the last page" json:"nextCursor,omitempty"`
	}
}

// StreamBody is one stream of a file.
type StreamBody struct {
	ID             int64   `doc:"Stable while the file is unchanged" json:"id"`
	Kind           string  `json:"kind"`
	Codec          *string `json:"codec,omitempty"`
	Language       *string `doc:"BCP 47" json:"language,omitempty"`
	ContainerTitle *string `json:"containerTitle,omitempty"`
	ChannelCount   *int64  `json:"channelCount,omitempty"`
}

// FileDetailBody is one file of a version; its path is shown to an admin only.
type FileDetailBody struct {
	ID   int64  `json:"id"`
	Path string `json:"path,omitempty"`
	// OtherPaths are the file's other places, copies of it found after its first (D363 §2): an admin
	// sees every place of a file (§8).
	OtherPaths []string     `doc:"The file's other places, its byte-identical copies, after its first; an admin's alone" json:"otherPaths,omitempty"` //nolint:lll // one struct tag
	SizeBytes  *int64       `json:"sizeBytes,omitempty"`
	Streams    []StreamBody `json:"streams"`
}

// VersionBody is one version of a title (D155): its files, in part order.
type VersionBody struct {
	ID        int64            `json:"id"`
	EditionID *int64           `doc:"Absent when the cut is unknown" json:"editionId,omitempty"`
	Files     []FileDetailBody `json:"files"`
}

// ExtraBody is one extra of a title (D162).
type ExtraBody struct {
	ID        int64  `json:"id"`
	ExtraType string `json:"extraType"`
	Name      string `json:"name"`
}

// EpisodePlaceBody is an episode's series, its place in the aired order, and its place in the
// ordering the person watches the series in, which labels it wherever they see it (D159 §1;
// episode-orderings.md §7 Q8).
type EpisodePlaceBody struct {
	SeriesID      int64  `json:"seriesId"`
	SeriesName    string `json:"seriesName"`
	SeasonNumber  int64  `doc:"Its season in the aired order" json:"seasonNumber"`
	EpisodeNumber int64  `doc:"Its number in its aired season" json:"episodeNumber"`
	// Ordering is the ordering the person watches the series in, when it is not aired: a provider's.
	Ordering string `doc:"The ordering the person watches the series in; absent for aired" json:"ordering,omitempty"`
	// InOrdering is its place in that ordering; absent in aired, whose place is the season and number,
	// and when the ordering leaves it out.
	InOrdering *OrderingPlaceBody `json:"inOrdering,omitempty"`
}

// OrderingPlaceBody is a member's place in a named ordering other than aired.
type OrderingPlaceBody struct {
	GroupNumber   int64  `doc:"Its group in the ordering, a disc or a part" json:"groupNumber"`
	NumberInGroup int64  `doc:"Its number in its group" json:"numberInGroup"`
	GroupName     string `doc:"The group's name, where the ordering names it (Disc 2)" json:"groupName,omitempty"`
	IsOnlyGroup   bool   `doc:"Whether the ordering is one run, with no headings" json:"isOnlyGroup,omitempty"`
}

// ItemDetailOutput is a title's detail page.
type ItemDetailOutput struct {
	Body struct {
		ID                  int64             `json:"id"`
		Kind                string            `enum:"movie,episode,extra,other" json:"kind"`
		Name                string            `json:"name"`
		Overview            string            `json:"overview,omitempty"`
		Tagline             string            `json:"tagline,omitempty"`
		Year                int64             `json:"year,omitempty"`
		RuntimeMs           int64             `json:"runtimeMs,omitempty"`
		IdentificationState string            `json:"identificationState"`
		IsGuessed           bool              `doc:"Whether its match is a guess no person confirmed (D301)" json:"isGuessed,omitempty"` //nolint:lll // one struct tag
		FollowedSource      string            `json:"followedSource,omitempty"`
		Versions            []VersionBody     `json:"versions"`
		Extras              []ExtraBody       `json:"extras"`
		Episode             *EpisodePlaceBody `json:"episode,omitempty"`
		Art                 ArtBody           `json:"art"`
		FetchFailure        *FetchFailureBody `json:"fetchFailure,omitempty"`
		RefreshRequestedAt  int64             `doc:"When a person asked for it to be refreshed, while no fetch has answered yet" json:"refreshRequestedAt,omitempty"`                                               //nolint:lll // one struct tag
		Unavailable         *UnavailableBody  `doc:"Why none of its files can be read now; absent when one can" json:"unavailable,omitempty"`                                                                       //nolint:lll // one struct tag
		IsWatched           bool              `doc:"Whether the person watched it, by playing it to its finish or by hand (D370)" json:"isWatched,omitempty"`                                                       //nolint:lll // one struct tag
		Libraries           []LibraryNameBody `doc:"The libraries its person reaches that list it, by name: its page goes up to the one the person came through, else the first (D367)" json:"libraries,omitempty"` //nolint:lll // one struct tag
		FilmOfExtra         *FilmOfExtraBody  `doc:"The film an extra belongs to, which its page goes up to (D367); absent for anything else" json:"filmOfExtra,omitempty"`                                         //nolint:lll // one struct tag
	}
}

// EpisodeBody is one episode of a series.
type EpisodeBody struct {
	ID                  int64  `json:"id"`
	SeasonNumber        int64  `json:"seasonNumber"`
	EpisodeNumber       int64  `json:"episodeNumber"`
	Name                string `json:"name"`
	Year                int64  `json:"year,omitempty"`
	RuntimeMs           int64  `json:"runtimeMs,omitempty"`
	IdentificationState string `json:"identificationState"`
	HasLiveFile         bool   `doc:"Whether it has a live file" json:"hasLiveFile"`
	StillSha256         string `doc:"Its still's hash: GET /bytes/art/{sourceSha256}/landscape-s.webp" json:"stillSha256,omitempty"` //nolint:lll // one struct tag
	// Unavailable says why none of its files can be read now; absent when one can (D241 §3).
	Unavailable *UnavailableBody `json:"unavailable,omitempty"`
	IsWatched   bool             `doc:"Whether the person watched it, by playing it to its finish or by hand (D370)" json:"isWatched,omitempty"` //nolint:lll // one struct tag
}

// SeasonBody is one season of a series, a group of the ordering its person watches it in, as its page
// lists it (D306 §9).
type SeasonBody struct {
	SeasonNumber int64  `doc:"Its group's number; aired's 0 for the specials and the episodes with no season" json:"seasonNumber"`              //nolint:lll // one struct tag
	Name         string `doc:"The ordering's name for it (Disc 2); absent where it names none, as aired names no season" json:"name,omitempty"` //nolint:lll // one struct tag
	EpisodeCount int64  `json:"episodeCount"`
	PosterSha256 string `doc:"Its own poster's hash, absent where it has none (D379): GET /bytes/art/{sourceSha256}/portrait-m.webp" json:"posterSha256,omitempty"` //nolint:lll // one struct tag
	StillSha256  string `doc:"Its first episode's still that has one" json:"stillSha256,omitempty"`
	IsWatched    bool   `doc:"Whether the person watched every one of its episodes with a file (D370)" json:"isWatched,omitempty"` //nolint:lll // one struct tag
}

// CollectionOutput is a series with its seasons, the groups of the ordering its person watches it in,
// in its order (aired's with the specials last); each season's episodes are
// GET /collections/{id}/seasons/{season} (D306 §9).
type CollectionOutput struct {
	Body struct {
		ID                  int64             `json:"id"`
		Kind                string            `enum:"series" json:"kind"`
		Name                string            `json:"name"`
		Overview            string            `json:"overview,omitempty"`
		Year                int64             `json:"year,omitempty"`
		IdentificationState string            `json:"identificationState,omitempty"`
		IsGuessed           bool              `doc:"Whether its match is a guess no person confirmed (D301)" json:"isGuessed,omitempty"`                                     //nolint:lll // one struct tag
		Ordering            string            `doc:"The ordering its person watches it in, which its seasons are the groups of; absent for aired" json:"ordering,omitempty"` //nolint:lll // one struct tag
		Seasons             []SeasonBody      `json:"seasons"`
		Orderings           []OrderingBody    `doc:"Every ordering it has, aired first; its page offers a choice when there is more than one" json:"orderings,omitempty"`             //nolint:lll // one struct tag
		NotInOrderingCount  int64             `doc:"How many of its episodes the ordering in use leaves out: GET /collections/{id}/not-in-order" json:"notInOrderingCount,omitempty"` //nolint:lll // one struct tag
		Art                 ArtBody           `json:"art"`
		FetchFailure        *FetchFailureBody `json:"fetchFailure,omitempty"`
		RefreshRequestedAt  int64             `doc:"When a person asked for it to be refreshed, while no fetch has answered yet" json:"refreshRequestedAt,omitempty"`                                                                //nolint:lll // one struct tag
		Unavailable         *UnavailableBody  `doc:"Why none of its episodes' files can be read now; absent when one can" json:"unavailable,omitempty"`                                                                              //nolint:lll // one struct tag
		NextUp              *NextUpBody       `doc:"The person's next episode, which the page offers to play; absent when every one is finished or none has a file" json:"nextUp,omitempty"`                                         //nolint:lll // one struct tag
		IsWatched           bool              `doc:"Whether the person watched every one of its episodes with a file, in the ordering they watch it in (D370)" json:"isWatched,omitempty"`                                           //nolint:lll // one struct tag
		Libraries           []LibraryNameBody `doc:"The libraries its person reaches that list one of its episodes, by name: its page goes up to the one the person came through, else the first (D367)" json:"libraries,omitempty"` //nolint:lll // one struct tag
	}
}

// OrderingBody is one of a series' orderings, as its page offers it (episode-orderings.md §4).
type OrderingBody struct {
	Name string `doc:"Its key, which a choice of it names: aired, or a provider's (tmdb:<group id>)" json:"name"`
	Kind string `doc:"What order it is: aired, or a TMDB group's type (dvd, absolute, digital, story_arc, production, tv, aired_alternate)" json:"kind,omitempty"` //nolint:lll // one struct tag
	// Label is its provider's name for it ("DVD Order"); absent for aired.
	Label string `json:"label,omitempty"`
	// ListedCount is how many episodes its provider lists in it; absent when it did not say.
	ListedCount  int64 `json:"listedCount,omitempty"`
	EpisodeCount int64 `doc:"How many of the series' episodes it holds" json:"episodeCount"`
	// IsAwaitingFetch is whether it is a provider's group whose contents are not fetched yet: choosing
	// it fetches them, and it holds none until then.
	IsAwaitingFetch bool `json:"isAwaitingFetch,omitempty"`
}

// NextUpBody is a person's Next Up in a collection's ordering (D160), for a series the one they watch
// it in: the member its page offers to play, and where it is in the ordering.
type NextUpBody struct {
	TitleID       int64            `json:"titleId"`
	Name          string           `json:"name,omitempty"`
	GroupNumber   int64            `doc:"Its group in the ordering, a series' season; 0 for the specials and those with none" json:"groupNumber"` //nolint:lll // one struct tag
	NumberInGroup int64            `doc:"Its number in its group, a series' episode number" json:"numberInGroup"`
	GroupName     string           `doc:"The ordering's name for its group (Disc 2); absent where it names none" json:"groupName,omitempty"`             //nolint:lll // one struct tag
	FileID        int64            `doc:"The file its Play plays: the one the person is partway through, else its first" json:"fileId"`                  //nolint:lll // one struct tag
	IsPartway     bool             `doc:"Whether the person is partway through it, which its Play resumes" json:"isPartway"`                             //nolint:lll // one struct tag
	PositionMs    int64            `doc:"Where in the file the person is, when partway through it" json:"positionMs,omitempty"`                          //nolint:lll // one struct tag
	Unavailable   *UnavailableBody `doc:"Why none of its files can be read now, as its own page says; absent when one can" json:"unavailable,omitempty"` //nolint:lll // one struct tag
}

// SeasonInput asks for a page of one season's episodes.
type SeasonInput struct {
	AuthorizationHeader
	ID     int64  `path:"id"`
	Season int64  `doc:"0 for the specials and the episodes with no season" minimum:"0" path:"season"`
	Cursor string `doc:"From the previous page's nextCursor" query:"cursor"`
	Limit  int64  `maximum:"500" minimum:"1" query:"limit" required:"true"`
}

// SeasonOutput is a page of one season's episodes, a group of the ordering its person watches the
// series in, in its order.
type SeasonOutput struct {
	Body struct {
		Episodes   []EpisodeBody `json:"episodes"`
		NextCursor string        `doc:"The next page's cursor; absent on the last" json:"nextCursor,omitempty"`
	}
}

// SearchInput is a search.
type SearchInput struct {
	AuthorizationHeader
	Query   string `query:"query" required:"true"`
	Library int64  `doc:"One library's titles alone; absent for every library the person reaches" minimum:"0" query:"library"` //nolint:lll // one struct tag
	Limit   int64  `maximum:"100" minimum:"1" query:"limit" required:"true"`
}

// HitBody is one search result: a title, or a series.
type HitBody struct {
	ID   int64  `json:"id"`
	Kind string `enum:"movie,episode,extra,other,series" json:"kind"`
	Name string `json:"name"`
	Year int64  `json:"year,omitempty"`
}

// SearchOutput is a search's results, best first.
type SearchOutput struct {
	Body struct {
		Results []HitBody `json:"results"`
	}
}

func (server *Server) registerBrowse() {
	tags := []string{tagBrowse}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getTitles", Method: http.MethodGet, Path: "/titles",
		Summary: "Films or series, by name in the library's locale or newest first", Tags: tags,
	}, server.getTitles)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getTitle", Method: http.MethodGet, Path: "/items/{id}",
		Summary: "A title's detail: versions, files, streams, extras and, for an episode, its place", Tags: tags,
	}, server.getTitle)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getSeries", Method: http.MethodGet, Path: "/collections/{id}",
		Summary: "A series with its seasons, each with how many episodes it has", Tags: tags,
	}, server.getSeries)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getSeason", Method: http.MethodGet, Path: "/collections/{id}/seasons/{season}",
		Summary: "A page of one season's episodes, in the ordering the person watches the series in", Tags: tags,
	}, server.getSeason)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getNotInOrdering", Method: http.MethodGet, Path: "/collections/{id}/not-in-order",
		Summary: "A page of a series' episodes the ordering the person watches it in leaves out, in aired order",
		Tags:    tags,
	}, server.getNotInOrdering)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "search", Method: http.MethodGet, Path: "/search",
		Summary: "Titles and series by name, in any script (D48)", Tags: tags,
	}, server.search)
}

func (server *Server) getTitles(ctx context.Context, input *TitlesInput) (*TitlesOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isByName := input.Sort == library.SortName || input.Sort == library.SortFileName
	if input.From != "" && (!isByName || input.Cursor != "") {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "query.from",
			"from starts a list by name, in place of its first page: it takes no cursor")
	}
	if input.IsHome && input.Library != 0 {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "query.home",
			"a row of Home is of every library the person keeps on it, not of one library")
	}
	reach, err := server.listReach(ctx, dependencies, principal,
		listOf{kind: input.Kind, libraryID: input.Library, isHome: input.IsHome})
	if err != nil {
		return nil, err
	}
	page, err := dependencies.Browser.List(ctx, library.ListQuery{
		Kind: input.Kind, Sort: input.Sort, Order: input.Order, Cursor: input.Cursor, From: input.From,
		Limit: input.Limit, Reach: reach, UserID: principal.UserID,
	})
	if err != nil {
		return nil, server.libraryError(ctx, "list titles", err)
	}
	output := &TitlesOutput{}
	output.Body.Titles = make([]EntryBody, 0, len(page.Entries))
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, entry := range page.Entries {
		output.Body.Titles = append(output.Body.Titles, entryBody(entry, nodeName))
	}
	output.Body.NextCursor = page.NextCursor
	return output, nil
}

func (server *Server) getTitle(ctx context.Context, input *ItemInput) (*ItemDetailOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	detail, err := dependencies.Browser.Title(ctx, principal.Reach, principal.UserID, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "read a title", err)
	}
	output := &ItemDetailOutput{}
	body := &output.Body
	body.ID, body.Kind, body.Name = detail.ID, detail.Kind, ptr.ValueOrZero(detail.Name)
	body.IdentificationState, body.IsGuessed = detail.IdentificationState, detail.IsGuessed
	body.Overview = ptr.ValueOrZero(detail.Overview)
	body.Tagline = ptr.ValueOrZero(detail.Tagline)
	body.FollowedSource = ptr.ValueOrZero(detail.FollowedSource)
	body.Year = ptr.ValueOrZero(detail.Year)
	body.RuntimeMs = ptr.ValueOrZero(detail.RuntimeMs)
	body.FetchFailure = fetchFailureBody(detail.FetchFailure, detail.FetchFailedAt)
	body.RefreshRequestedAt = ptr.ValueOrZero(detail.RefreshRequestedAt)
	body.Versions = make([]VersionBody, 0, len(detail.Versions))
	for _, version := range detail.Versions {
		versionBody := VersionBody{ID: version.ID, EditionID: version.EditionID, Files: []FileDetailBody{}}
		for _, file := range version.Files {
			fileBody := FileDetailBody{ID: file.ID, SizeBytes: file.SizeBytes, Streams: []StreamBody{}}
			// Paths are shown where the admin administers, never from away (the Plex-chain review's T2).
			if administersHere(ctx, principal) {
				fileBody.Path, fileBody.OtherPaths = file.Path, file.Paths[1:]
			}
			for _, stream := range file.Streams {
				fileBody.Streams = append(fileBody.Streams, StreamBody{
					ID: stream.ID, Kind: stream.Kind, Codec: stream.Codec, Language: stream.Language,
					ContainerTitle: stream.ContainerTitle,
					ChannelCount:   stream.ChannelCount,
				})
			}
			versionBody.Files = append(versionBody.Files, fileBody)
		}
		body.Versions = append(body.Versions, versionBody)
	}
	body.Extras = make([]ExtraBody, 0, len(detail.Extras))
	for _, extra := range detail.Extras {
		body.Extras = append(body.Extras, ExtraBody{
			ID: extra.ExtraTitle, ExtraType: extra.ExtraType, Name: ptr.ValueOrZero(extra.Name),
		})
	}
	body.Art = artBody(detail.Art)
	body.Episode = episodePlaceBody(detail.Episode)
	whereabouts, err := server.whereabouts(ctx, dependencies, principal.Reach, detail.ID, detail.Kind)
	if err != nil {
		return nil, err
	}
	body.Libraries = libraryNameBodies(whereabouts.Libraries)
	body.FilmOfExtra = filmOfExtraBody(whereabouts.FilmOfExtra)
	body.Unavailable = unavailableBody(detail.Unreadable, server.nodeNamer(ctx, dependencies))
	body.IsWatched = detail.IsWatched
	if detail.Unreadable != nil && detail.Unreadable.Reason == library.StillReading {
		// Its page open, its files are read before what else waits, so Play works soon (D304 §4).
		if readErr := dependencies.Playback.ReadWhenViewed(ctx, detail.ID); readErr != nil {
			server.logger.WarnContext(ctx, "could not ask for a title's read", "titleId", detail.ID, "err", readErr)
		}
	}
	return output, nil
}

// episodePlaceBody is an episode's place as the API shows it, or nil for a title in no series.
func episodePlaceBody(place *library.EpisodePlace) *EpisodePlaceBody {
	if place == nil {
		return nil
	}
	body := &EpisodePlaceBody{
		SeriesID: place.CollectionID, SeriesName: ptr.ValueOrZero(place.SeriesName),
		SeasonNumber: ptr.ValueOrZero(place.GroupNumber), EpisodeNumber: ptr.ValueOrZero(place.NumberInGroup),
	}
	if place.Ordering != library.OrderingAired {
		body.Ordering = place.Ordering
	}
	if in := place.InOrdering; in != nil {
		body.InOrdering = &OrderingPlaceBody{
			GroupNumber: in.Group, NumberInGroup: in.NumberInGroup, GroupName: in.GroupName,
			IsOnlyGroup: in.IsOnlyGroup,
		}
	}
	return body
}

func (server *Server) getSeries(ctx context.Context, input *ItemInput) (*CollectionOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	series, err := dependencies.Browser.Series(ctx, principal.Reach, principal.UserID, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "read a series", err)
	}
	output := &CollectionOutput{}
	body := &output.Body
	body.ID, body.Kind = series.ID, "series"
	if series.Ordering != library.OrderingAired {
		body.Ordering = series.Ordering
	}
	body.Name, body.Overview = ptr.ValueOrZero(series.Name), ptr.ValueOrZero(series.Overview)
	body.Year = ptr.ValueOrZero(series.Year)
	body.IdentificationState, body.IsGuessed = ptr.ValueOrZero(series.IdentificationState), series.IsGuessed
	body.FetchFailure = fetchFailureBody(series.FetchFailure, series.FetchFailedAt)
	body.RefreshRequestedAt = ptr.ValueOrZero(series.RefreshRequestedAt)
	body.Unavailable = unavailableBody(series.Unreadable, server.nodeNamer(ctx, dependencies))
	body.Seasons = make([]SeasonBody, 0, len(series.Seasons))
	for _, season := range series.Seasons {
		body.Seasons = append(body.Seasons, SeasonBody{
			SeasonNumber: season.Number, Name: season.Name, EpisodeCount: season.EpisodeCount,
			PosterSha256: season.PosterSha256, StillSha256: season.StillSha256, IsWatched: season.IsWatched,
		})
	}
	body.Orderings = make([]OrderingBody, 0, len(series.Orderings))
	for _, ordering := range series.Orderings {
		body.Orderings = append(body.Orderings, OrderingBody{
			Name: ordering.Name, Kind: ordering.Kind, Label: ordering.Label, ListedCount: ordering.ListedCount,
			EpisodeCount: ordering.EpisodeCount, IsAwaitingFetch: ordering.IsAwaitingFetch,
		})
	}
	body.NotInOrderingCount = series.NotInOrderingCount
	body.IsWatched = series.IsWatched
	whereabouts, err := server.whereabouts(ctx, dependencies, principal.Reach, series.ID, "series")
	if err != nil {
		return nil, err
	}
	body.Libraries = libraryNameBodies(whereabouts.Libraries)
	body.Art = artBody(series.Art)
	next, hasNext, err := dependencies.Browser.NextUp(ctx, principal.Reach, principal.UserID, series.ID,
		series.Ordering)
	if err != nil {
		return nil, server.libraryError(ctx, "read a series' next episode", err)
	}
	if hasNext {
		nextUp := server.nextUpBody(ctx, dependencies, next)
		body.NextUp = &nextUp
	}
	return output, nil
}

func (server *Server) getSeason(ctx context.Context, input *SeasonInput) (*SeasonOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	page, err := dependencies.Browser.Season(ctx, principal.Reach, library.SeasonQuery{
		SeriesID: input.ID, UserID: principal.UserID, Season: input.Season, Cursor: input.Cursor, Limit: input.Limit,
	})
	if err != nil {
		return nil, server.libraryError(ctx, "read a season", err)
	}
	return server.episodePageOutput(ctx, dependencies, page), nil
}

// NotInOrderingInput asks for a page of the episodes a series' ordering in use leaves out.
type NotInOrderingInput struct {
	AuthorizationHeader
	ID     int64  `path:"id"`
	Cursor string `doc:"From the previous page's nextCursor" query:"cursor"`
	Limit  int64  `maximum:"500" minimum:"1" query:"limit" required:"true"`
}

// getNotInOrdering is a page of the episodes the ordering the person watches a series in leaves out,
// at their aired places, which its page lists after the ordering's groups (episode-orderings.md
// §7 Q4); none in aired.
func (server *Server) getNotInOrdering(ctx context.Context, input *NotInOrderingInput) (*SeasonOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	page, err := dependencies.Browser.NotInOrdering(ctx, principal.Reach, library.SeasonQuery{
		SeriesID: input.ID, UserID: principal.UserID, Cursor: input.Cursor, Limit: input.Limit,
	})
	if err != nil {
		return nil, server.libraryError(ctx, "read the episodes not in a series' ordering", err)
	}
	return server.episodePageOutput(ctx, dependencies, page), nil
}

// episodePageOutput is a page of a series' episodes as the API says it.
func (server *Server) episodePageOutput(
	ctx context.Context, dependencies *Dependencies, page library.SeasonPage,
) *SeasonOutput {
	output := &SeasonOutput{}
	output.Body.Episodes = make([]EpisodeBody, 0, len(page.Episodes))
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, episode := range page.Episodes {
		var unreadable *library.Unreadable
		if why, isUnreadable := page.UnreadableByEpisodeID[episode.TitleID]; isUnreadable {
			unreadable = &why
		}
		output.Body.Episodes = append(output.Body.Episodes, EpisodeBody{
			Unavailable: unavailableBody(unreadable, nodeName),
			ID:          episode.TitleID, SeasonNumber: ptr.ValueOrZero(episode.GroupNumber),
			EpisodeNumber: ptr.ValueOrZero(episode.NumberInGroup),
			Name:          ptr.ValueOrZero(episode.Name),
			Year:          ptr.ValueOrZero(episode.Year), RuntimeMs: ptr.ValueOrZero(episode.RuntimeMs),
			IdentificationState: episode.IdentificationState,
			HasLiveFile:         episode.Available, StillSha256: page.StillSha256ByEpisodeID[episode.TitleID],
			IsWatched: page.IsWatchedByEpisodeID[episode.TitleID],
		})
	}
	output.Body.NextCursor = page.NextCursor
	return output
}

func (server *Server) search(ctx context.Context, input *SearchInput) (*SearchOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	reach := principal.Reach
	if input.Library != 0 {
		var isReached bool
		if reach, isReached = reach.Narrowed(input.Library); !isReached {
			return nil, newError(http.StatusNotFound, CodeNotFound, "no such library")
		}
	}
	hits, err := dependencies.Browser.Search(ctx, reach, input.Query, input.Limit)
	if err != nil {
		return nil, server.libraryError(ctx, "search", err)
	}
	output := &SearchOutput{}
	output.Body.Results = make([]HitBody, 0, len(hits))
	for _, hit := range hits {
		kind := "series"
		if hit.TitleKind != nil {
			kind = *hit.TitleKind
		}
		output.Body.Results = append(output.Body.Results, HitBody{
			ID: hit.ItemID, Kind: kind, Name: hit.Name, Year: ptr.ValueOrZero(hit.Year),
		})
	}
	return output, nil
}
