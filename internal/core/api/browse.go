// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

const tagBrowse = "Browse"

// EntryBody is one film or series in a list.
type EntryBody struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Year                int64  `json:"year,omitempty"`
	IdentificationState string `json:"identificationState"`
	// IsGuessed is whether its match is a guess no person confirmed (D301 §4).
	IsGuessed    bool   `json:"isGuessed,omitempty"`
	LibraryID    int64  `json:"libraryId"`
	AddedAt      int64  `json:"addedAt"`
	PosterSha256 string `doc:"Its poster's hash: GET /bytes/art/{sourceSha256}/portrait-m.webp" json:"posterSha256,omitempty"` //nolint:lll // one struct tag
	// Unavailable says why none of its files can be read now; absent when one can (D241 §3).
	Unavailable *UnavailableBody `json:"unavailable,omitempty"`
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

// TitlesInput asks for one page of films or series.
type TitlesInput struct {
	AuthorizationHeader
	Kind   string `enum:"movie,series" query:"kind" required:"true"`
	Sort   string `enum:"name,added" query:"sort" required:"true"`
	Cursor string `doc:"From the previous page's nextCursor" query:"cursor"`
	Limit  int64  `maximum:"500" minimum:"1" query:"limit" required:"true"`
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
	ID        int64        `json:"id"`
	Path      string       `json:"path,omitempty"`
	SizeBytes *int64       `json:"sizeBytes,omitempty"`
	Streams   []StreamBody `json:"streams"`
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

// EpisodePlaceBody is an episode's series and place in its aired order.
type EpisodePlaceBody struct {
	SeriesID      int64  `json:"seriesId"`
	SeriesName    string `json:"seriesName"`
	SeasonNumber  int64  `json:"seasonNumber"`
	EpisodeNumber int64  `json:"episodeNumber"`
}

// ItemDetailOutput is a title's detail page.
type ItemDetailOutput struct {
	Body struct {
		ID                  int64             `json:"id"`
		Kind                string            `enum:"movie,episode,extra" json:"kind"`
		Name                string            `json:"name"`
		Overview            string            `json:"overview,omitempty"`
		Tagline             string            `json:"tagline,omitempty"`
		Year                int64             `json:"year,omitempty"`
		RuntimeMs           int64             `json:"runtimeMs,omitempty"`
		IdentificationState string            `json:"identificationState"`
		IsGuessed           bool              `doc:"Whether its match is a guess no person confirmed (D301)" json:"isGuessed,omitempty"` //nolint:lll // one struct tag
		FollowedSource      string            `json:"followedSource,omitempty"`
		LibraryID           int64             `json:"libraryId"`
		Versions            []VersionBody     `json:"versions"`
		Extras              []ExtraBody       `json:"extras"`
		Episode             *EpisodePlaceBody `json:"episode,omitempty"`
		Art                 ArtBody           `json:"art"`
		FetchFailure        *FetchFailureBody `json:"fetchFailure,omitempty"`
		RefreshRequestedAt  int64             `doc:"When a person asked for it to be refreshed, while no fetch has answered yet" json:"refreshRequestedAt,omitempty"` //nolint:lll // one struct tag
		Unavailable         *UnavailableBody  `doc:"Why none of its files can be read now; absent when one can" json:"unavailable,omitempty"`                         //nolint:lll // one struct tag
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
}

// SeasonBody is one season of a series, as its page lists it (D306 §9).
type SeasonBody struct {
	SeasonNumber int64  `doc:"0 for the specials and the episodes with no season" json:"seasonNumber"`
	EpisodeCount int64  `json:"episodeCount"`
	StillSha256  string `doc:"Its first episode's still that has one" json:"stillSha256,omitempty"`
}

// CollectionOutput is a series with its seasons in aired order, the specials last; each season's
// episodes are GET /collections/{id}/seasons/{season} (D306 §9).
type CollectionOutput struct {
	Body struct {
		ID                  int64             `json:"id"`
		Kind                string            `enum:"series" json:"kind"`
		Name                string            `json:"name"`
		Overview            string            `json:"overview,omitempty"`
		Year                int64             `json:"year,omitempty"`
		IdentificationState string            `json:"identificationState,omitempty"`
		IsGuessed           bool              `doc:"Whether its match is a guess no person confirmed (D301)" json:"isGuessed,omitempty"` //nolint:lll // one struct tag
		LibraryID           int64             `json:"libraryId"`
		Seasons             []SeasonBody      `json:"seasons"`
		Art                 ArtBody           `json:"art"`
		FetchFailure        *FetchFailureBody `json:"fetchFailure,omitempty"`
		RefreshRequestedAt  int64             `doc:"When a person asked for it to be refreshed, while no fetch has answered yet" json:"refreshRequestedAt,omitempty"` //nolint:lll // one struct tag
		Unavailable         *UnavailableBody  `doc:"Why none of its episodes' files can be read now; absent when one can" json:"unavailable,omitempty"`               //nolint:lll // one struct tag
	}
}

// SeasonInput asks for a page of one season's episodes.
type SeasonInput struct {
	AuthorizationHeader
	ID     int64  `path:"id"`
	Season int64  `doc:"0 for the specials and the episodes with no season" minimum:"0" path:"season"`
	Cursor string `doc:"From the previous page's nextCursor" query:"cursor"`
	Limit  int64  `maximum:"500" minimum:"1" query:"limit" required:"true"`
}

// SeasonOutput is a page of one season's episodes in aired order.
type SeasonOutput struct {
	Body struct {
		Episodes   []EpisodeBody `json:"episodes"`
		NextCursor string        `doc:"The next page's cursor; absent on the last" json:"nextCursor,omitempty"`
	}
}

// SearchInput is a search.
type SearchInput struct {
	AuthorizationHeader
	Query string `query:"query" required:"true"`
	Limit int64  `maximum:"100" minimum:"1" query:"limit" required:"true"`
}

// HitBody is one search result: a title, or a series.
type HitBody struct {
	ID   int64  `json:"id"`
	Kind string `enum:"movie,episode,extra,series" json:"kind"`
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
		Summary: "A page of one season's episodes in aired order", Tags: tags,
	}, server.getSeason)
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
	page, err := dependencies.Browser.List(ctx, library.ListQuery{
		Kind: input.Kind, Sort: input.Sort, Cursor: input.Cursor, Limit: input.Limit, Reach: principal.Reach,
	})
	if err != nil {
		return nil, server.libraryError(ctx, "list titles", err)
	}
	output := &TitlesOutput{}
	output.Body.Titles = make([]EntryBody, 0, len(page.Entries))
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, entry := range page.Entries {
		output.Body.Titles = append(output.Body.Titles, EntryBody{
			ID: entry.ID, Name: entry.Name, Year: entry.Year, IdentificationState: entry.IdentificationState,
			IsGuessed: entry.IsGuessed, LibraryID: entry.LibraryID,
			AddedAt: entry.AddedAt, PosterSha256: entry.PosterSha256,
			Unavailable: unavailableBody(entry.Unreadable, nodeName),
		})
	}
	output.Body.NextCursor = page.NextCursor
	return output, nil
}

func (server *Server) getTitle(ctx context.Context, input *ItemInput) (*ItemDetailOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	detail, err := dependencies.Browser.Title(ctx, principal.Reach, input.ID)
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
	body.LibraryID = ptr.ValueOrZero(detail.LibraryID)
	body.FetchFailure = fetchFailureBody(detail.FetchFailure, detail.FetchFailedAt)
	body.RefreshRequestedAt = ptr.ValueOrZero(detail.RefreshRequestedAt)
	body.Versions = make([]VersionBody, 0, len(detail.Versions))
	for _, version := range detail.Versions {
		versionBody := VersionBody{ID: version.ID, EditionID: version.EditionID, Files: []FileDetailBody{}}
		for _, file := range version.Files {
			fileBody := FileDetailBody{ID: file.ID, SizeBytes: file.SizeBytes, Streams: []StreamBody{}}
			// Paths are shown where the admin administers, never from away (the Plex-chain review's T2).
			if administersHere(ctx, principal) {
				fileBody.Path = file.Path
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
	body.Unavailable = unavailableBody(detail.Unreadable, server.nodeNamer(ctx, dependencies))
	if detail.Unreadable != nil && detail.Unreadable.Reason == library.StillReading {
		// Its page open, its files are read before what else waits, so Play works soon (D304 §4).
		if readErr := dependencies.Playback.ReadWhenViewed(ctx, detail.ID); readErr != nil {
			server.logger.WarnContext(ctx, "could not ask for a title's read", "titleId", detail.ID, "err", readErr)
		}
	}
	return output, nil
}

// episodePlaceBody is an episode's place as the API shows it, or nil for a title in no series.
func episodePlaceBody(place *sqlcgen.GetEpisodePlaceRow) *EpisodePlaceBody {
	if place == nil {
		return nil
	}
	return &EpisodePlaceBody{
		SeriesID: place.CollectionID, SeriesName: ptr.ValueOrZero(place.SeriesName),
		SeasonNumber: ptr.ValueOrZero(place.GroupNumber), EpisodeNumber: ptr.ValueOrZero(place.NumberInGroup),
	}
}

func (server *Server) getSeries(ctx context.Context, input *ItemInput) (*CollectionOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	series, err := dependencies.Browser.Series(ctx, principal.Reach, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "read a series", err)
	}
	output := &CollectionOutput{}
	body := &output.Body
	body.ID, body.Kind = series.ID, "series"
	body.Name, body.Overview = ptr.ValueOrZero(series.Name), ptr.ValueOrZero(series.Overview)
	body.Year, body.LibraryID = ptr.ValueOrZero(series.Year), ptr.ValueOrZero(series.LibraryID)
	body.IdentificationState, body.IsGuessed = ptr.ValueOrZero(series.IdentificationState), series.IsGuessed
	body.FetchFailure = fetchFailureBody(series.FetchFailure, series.FetchFailedAt)
	body.RefreshRequestedAt = ptr.ValueOrZero(series.RefreshRequestedAt)
	body.Unavailable = unavailableBody(series.Unreadable, server.nodeNamer(ctx, dependencies))
	body.Seasons = make([]SeasonBody, 0, len(series.Seasons))
	for _, season := range series.Seasons {
		body.Seasons = append(body.Seasons, SeasonBody{
			SeasonNumber: season.Number, EpisodeCount: season.EpisodeCount, StillSha256: season.StillSha256,
		})
	}
	body.Art = artBody(series.Art)
	return output, nil
}

func (server *Server) getSeason(ctx context.Context, input *SeasonInput) (*SeasonOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	page, err := dependencies.Browser.Season(ctx, principal.Reach, library.SeasonQuery{
		SeriesID: input.ID, Season: input.Season, Cursor: input.Cursor, Limit: input.Limit,
	})
	if err != nil {
		return nil, server.libraryError(ctx, "read a season", err)
	}
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
		})
	}
	output.Body.NextCursor = page.NextCursor
	return output, nil
}

func (server *Server) search(ctx context.Context, input *SearchInput) (*SearchOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	hits, err := dependencies.Browser.Search(ctx, principal.Reach, input.Query, input.Limit)
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
