// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/interest"
	"github.com/kstenerud/orpheum/internal/core/waits"
)

// PageInterestInput names the film or series whose page a session shows, and the group (a season)
// of a series' page that shows one.
type PageInterestInput struct {
	AuthorizationHeader
	Body struct {
		Item  int64  `doc:"The film or series whose page it is" json:"item" minimum:"1"`
		Group *int64 `doc:"The season a series' page shows, whose episodes it raises, 0 for the specials; absent for a film's page or a series' own" json:"group,omitempty" minimum:"0"` //nolint:lll // one struct tag
	}
}

// PageLeftInput names the film or series whose page a session left, and the group (a season) of a
// series' page that showed one, as PUT /interest/page named them.
type PageLeftInput struct {
	AuthorizationHeader
	Item  int64 `doc:"The film or series whose page it was" minimum:"1" query:"item" required:"true"`
	Group int64 `default:"-1" doc:"The season the series' page showed, 0 for the specials; -1, the default, for a film's page or a series' own" minimum:"-1" query:"group"` //nolint:lll // one struct tag
}

// TilesInterestInput names the films, series and videos whose tiles a list shows in view.
type TilesInterestInput struct {
	AuthorizationHeader
	Body struct {
		Items []int64 `doc:"The items whose tiles are in view" json:"items" maxItems:"200"`
	}
}

// The register keeps two kinds of interest a session says, so each is its own sub-resource (D425
// §1): the page shown, one item left only while still shown, and the tiles in view, a set replaced
// whole.
const (
	pathPageInterest  = "/interest/page"
	pathTilesInterest = "/interest/tiles"
)

func (server *Server) registerPageInterest() {
	tags := []string{tagBrowse}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "sayTilesInterest", Method: http.MethodPut, Path: pathTilesInterest,
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Say this session shows the tiles of these films, series and videos in view, which raises the " +
			"work of their pictures, again every interestSaidEverySeconds while it does (D362, D324)",
		Extensions: callableBy(capabilitySignedIn),
	}, server.sayTilesInterest)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "withdrawTilesInterest", Method: http.MethodDelete, Path: pathTilesInterest,
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary:    "Say this session shows no list's tiles, which lets their work fall back (D362, D324)",
		Extensions: callableBy(capabilitySignedIn),
	}, server.withdrawTilesInterest)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "sayPageInterest", Method: http.MethodPut, Path: pathPageInterest,
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Say this session shows a film's or series' page, or a series' season's, which raises its " +
			"work, again every interestSaidEverySeconds while it does (D324)",
		Extensions: callableBy(capabilitySignedIn),
	}, server.sayPageInterest)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "withdrawPageInterest", Method: http.MethodDelete, Path: pathPageInterest,
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Say this session left a film's or series' page, or a series' season's, which lets its work " +
			"fall back (D324)",
		Extensions: callableBy(capabilitySignedIn),
	}, server.withdrawPageInterest)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFileWaits", Method: http.MethodGet, Path: "/files/{id}/waits", Tags: tags,
		Summary: "The work a file's viewer waits on, its read-through for its loudness and its timeline, " +
			"and about how long each will take (D324 §6)",
		Extensions: callableBy(capabilitySignedIn),
	}, server.getFileWaits)
}

// WaitBody is one piece of work a viewer waits on.
type WaitBody struct {
	IsRunning   bool   `doc:"Whether it is being done now, rather than waiting its turn" json:"isRunning"`
	SecondsLeft *int64 `doc:"About how long until it is done; absent when nothing says" json:"secondsLeft,omitempty"`
}

// FileWaitsOutput is the work a file's viewer waits on: each absent when none is open.
type FileWaitsOutput struct {
	Body struct {
		ReadThrough *WaitBody `doc:"Its read-through, for its loudness" json:"readThrough,omitempty"`
		//nolint:lll // one struct tag
		Levellings []LevellingBody `doc:"What brings each of its audio tracks to the level, by its place among them, played as it is (D226): a player whose film was measured as it played takes it up from here; absent until one is measured" json:"levellings,omitempty"`
	}
}

// getFileWaits answers GET /files/{id}/waits: the work a viewer of a file it reaches waits on, and
// about how long each will take.
func (server *Server) getFileWaits(ctx context.Context, input *ItemInput) (*FileWaitsOutput, error) {
	dependencies, _, err := server.requireSignedInReachingFile(ctx, input.Authorization, input.ID)
	if err != nil {
		return nil, err
	}
	found, err := dependencies.Waits.Of(ctx, input.ID)
	if err != nil {
		return nil, server.playbackError(ctx, "say how long a file's work will take", err)
	}
	output := &FileWaitsOutput{}
	output.Body.ReadThrough = waitBody(found.ReadThrough)
	levellings, err := dependencies.Playback.TrackLevellings(ctx, input.ID)
	if err != nil {
		return nil, server.playbackError(ctx, "say how a file's sound is levelled", err)
	}
	if slices.ContainsFunc(levellings, func(levelling decide.Levelling) bool { return levelling.IsMeasured }) {
		for _, levelling := range levellings {
			output.Body.Levellings = append(output.Body.Levellings, levellingBody(levelling))
		}
	}
	return output, nil
}

// waitBody is a wait as the API says it; nil for none.
func waitBody(wait *waits.Wait) *WaitBody {
	if wait == nil {
		return nil
	}
	body := &WaitBody{IsRunning: wait.IsRunning}
	if wait.IsKnown {
		seconds := int64(wait.Left.Round(time.Second) / time.Second)
		body.SecondsLeft = &seconds
	}
	return body
}

// sayPageInterest answers PUT /interest/page: the session shows the item's page, or its season's,
// which it reaches.
func (server *Server) sayPageInterest(ctx context.Context, input *PageInterestInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Browser.ReachItem(ctx, principal.Reach, input.Body.Item); err != nil {
		return nil, server.identifyError(ctx, "reach an item", err)
	}
	season := interest.NoSeason
	if input.Body.Group != nil {
		season = *input.Body.Group
	}
	dependencies.Interest.Look(principal.SessionKey, principal.UserID, input.Body.Item, season)
	return &struct{}{}, nil
}

// sayTilesInterest answers PUT /interest/tiles: the session shows the tiles of the items it names,
// those it reaches; an item it does not reach raises nothing.
func (server *Server) sayTilesInterest(ctx context.Context, input *TilesInterestInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	reached := make([]int64, 0, len(input.Body.Items))
	for _, itemID := range input.Body.Items {
		if dependencies.Browser.ReachItem(ctx, principal.Reach, itemID) == nil {
			reached = append(reached, itemID)
		}
	}
	dependencies.Interest.SeeTiles(principal.SessionKey, principal.UserID, reached)
	return &struct{}{}, nil
}

// withdrawTilesInterest answers DELETE /interest/tiles: the session shows no list's tiles.
func (server *Server) withdrawTilesInterest(ctx context.Context, input *AuthorizationHeader) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	dependencies.Interest.LeaveTiles(principal.SessionKey)
	return &struct{}{}, nil
}

// withdrawPageInterest answers DELETE /interest/page: the session left the item's page, or its
// season's. Leaving one it was not on changes nothing.
func (server *Server) withdrawPageInterest(ctx context.Context, input *PageLeftInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	dependencies.Interest.Leave(principal.SessionKey, input.Item, input.Group)
	return &struct{}{}, nil
}
