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

// RecordedBody is when an other video was recorded, where that came from, and its file's name (D349
// §4).
type RecordedBody struct {
	RecordedAt int64  `doc:"Unix milliseconds" json:"recordedAt"`
	From       string `doc:"Its container's recorded-time tag, a date in its file's or a folder's name, the file's own time, or a user's correction" enum:"tag,name,file,user" json:"from"` //nolint:lll // one struct tag
	FileName   string `doc:"Its file's name" json:"fileName"`
}

// recordedBody is an other video's date as the API says it, or nil for the rest.
func recordedBody(facts *library.OtherFacts) *RecordedBody {
	if facts == nil {
		return nil
	}
	return &RecordedBody{RecordedAt: facts.RecordedAtMs, From: facts.RecordedFrom, FileName: facts.FileName}
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
	ID    int64            `json:"id"`
	Files []FileDetailBody `json:"files"`
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

// GroupInput asks for a page of one group's members: a season's episodes (D416 §3).
type GroupInput struct {
	AuthorizationHeader
	ID     int64  `path:"id"`
	Group  int64  `doc:"A group of the ordering the person watches it in, a season or a disc; aired's 0 for the specials and the episodes with no season" minimum:"0" path:"group"` //nolint:lll // one struct tag
	Cursor string `doc:"From the previous page's nextCursor" query:"cursor"`
	Limit  int64  `maximum:"500" minimum:"1" query:"limit" required:"true"`
}

// GroupOutput is a page of one group's members, a season's episodes, a group of the ordering its person watches the
// series in, in its order.
type GroupOutput struct {
	Body struct {
		Episodes   []EpisodeBody `json:"episodes"`
		NextCursor string        `doc:"The next page's cursor; absent on the last" json:"nextCursor,omitempty"`
	}
}

func (server *Server) registerBrowse() {
	tags := []string{tagBrowse}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getGroup", Method: http.MethodGet, Path: "/items/{id}/groups/{group}",
		Summary: "A page of one season's episodes, in the ordering the person watches the series in", Tags: tags,
	}, server.getGroup)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listNotInOrder", Method: http.MethodGet, Path: "/items/{id}/not-in-order",
		Summary: "A page of a series' episodes the ordering the person watches it in leaves out, in aired order",
		Tags:    tags,
	}, server.listNotInOrder)
}

func (server *Server) getGroup(ctx context.Context, input *GroupInput) (*GroupOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	page, err := dependencies.Browser.Season(ctx, principal.Reach, library.SeasonQuery{
		SeriesID: input.ID, UserID: principal.UserID, Season: input.Group, Cursor: input.Cursor, Limit: input.Limit,
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

// listNotInOrder is a page of the episodes the ordering the person watches a series in leaves out,
// at their aired places, which its page lists after the ordering's groups (episode-orderings.md
// §7 Q4); none in aired.
func (server *Server) listNotInOrder(ctx context.Context, input *NotInOrderingInput) (*GroupOutput, error) {
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
) *GroupOutput {
	output := &GroupOutput{}
	output.Body.Episodes = make([]EpisodeBody, 0, len(page.Episodes))
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, episode := range page.Episodes {
		var unreadable *library.Unreadable
		if why, isUnreadable := page.UnreadableByEpisodeID[episode.WorkID]; isUnreadable {
			unreadable = &why
		}
		output.Body.Episodes = append(output.Body.Episodes, EpisodeBody{
			Unavailable: unavailableBody(unreadable, nodeName),
			ID:          episode.WorkID, SeasonNumber: ptr.ValueOrZero(episode.GroupNumber),
			EpisodeNumber: ptr.ValueOrZero(episode.NumberInGroup),
			Name:          ptr.ValueOrZero(episode.Name),
			Year:          ptr.ValueOrZero(episode.Year), RuntimeMs: ptr.ValueOrZero(episode.RuntimeMs),
			IdentificationState: ptr.ValueOrZero(episode.IdentificationState),
			HasLiveFile:         episode.Available, StillSha256: page.StillSha256ByEpisodeID[episode.WorkID],
			IsWatched: page.IsWatchedByEpisodeID[episode.WorkID],
		})
	}
	output.Body.NextCursor = page.NextCursor
	return output
}
