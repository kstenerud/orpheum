// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/identify"
)

// attentionListedAtMost bounds each list /needs-attention answers, so one library read badly makes
// no unbounded answer: a justified constant, a bound on one admin's page, its list's count field
// saying how many there are in all (D432 §4).
const attentionListedAtMost = 100

// NeedsAttentionOutput counts what needs an admin: library folders and disks offline, scans whose
// removals wait for them, and what scans could not read (D231 §7, D277 §2–§3). The status tray's
// warning sign shows while any is counted, and leads to the page that lists them (D280).
type NeedsAttentionOutput struct {
	Body struct {
		// RootCount and DiskCount are how many library folders, and disks mounted inside one, are
		// offline, as the System page lists them.
		RootCount int `json:"rootCount"`
		DiskCount int `json:"diskCount"`
		// WaitingRootCount is how many library folders' scans wait for the admin before removing
		// files; UnreadCount how many files and folders scans could not read (D277).
		WaitingRootCount int `json:"waitingRootCount"`
		UnreadCount      int `json:"unreadCount"`
		// MatchCount is how many films and series wait for the admin to choose their match, among
		// several or after none was found, as "Needs a match" lists them (the owner, 2026-10-13:
		// "Needs a match should show up as one of the things under the exclamation point tray icon").
		MatchCount int `json:"matchCount"`
		// SeriesReadWrongly are the series some of whose episodes have no number though no specials
		// or extras folder holds them: their files' names were likely read wrongly (D306 §6). At most
		// 100 are listed; SeriesReadWronglyCount is how many there are in all (D432 §4).
		SeriesReadWrongly      []SeriesReadWronglyBody `maxItems:"100" json:"seriesReadWrongly"`
		SeriesReadWronglyCount int                     `json:"seriesReadWronglyCount"`
		// FilmsWithoutSound are the films whose video file holds no sound track, a video-only download
		// (the owner, 2026-11-02: "videos without sound should be included in the list of problem films").
		// At most 100 are listed; FilmsWithoutSoundCount is how many there are in all (D432 §4).
		FilmsWithoutSound      []FilmWithoutSoundBody `maxItems:"100" json:"filmsWithoutSound"`
		FilmsWithoutSoundCount int                    `json:"filmsWithoutSoundCount"`
		// RescansGivenUp are the libraries whose rescan gave up, their files changing every time they
		// were read, until a later rescan of each finishes (the owner, 2026-11-13). At most 100 are
		// listed, one a library.
		RescansGivenUp []RescanGivenUpBody `maxItems:"100" json:"rescansGivenUp,omitempty"`
		// StoodAsideAt is when the discovery refused this server as a copy of another that serves, so
		// that it asks no more until an admin asks again (D437 §3, §4); 0 while it asks.
		StoodAsideAt int64 `json:"stoodAsideAt"`
		// CopyRefusedFrom and CopyRefusedAt are where and when a copy of this server last tried to
		// start, refused because this one serves (D437 §4); empty and 0 while none has.
		CopyRefusedFrom string `json:"copyRefusedFrom"`
		CopyRefusedAt   int64  `json:"copyRefusedAt"`
	}
}

// SeriesReadWronglyBody is a series some of whose episodes' file names were likely read wrongly.
type SeriesReadWronglyBody struct {
	SeriesID      int64  `json:"seriesId"`
	Name          string `json:"name"`
	UnplacedCount int    `doc:"How many of its episodes have no number outside a specials or extras folder" json:"unplacedCount"` //nolint:lll // one struct tag
}

// RescanGivenUpBody is a library whose rescan gave up.
type RescanGivenUpBody struct {
	LibraryID int64  `json:"libraryId"`
	Name      string `json:"name"`
	GivenUpAt int64  `doc:"When it gave up, in Unix milliseconds" json:"givenUpAt"`
}

// FilmWithoutSoundBody is a film whose video file has no sound track.
type FilmWithoutSoundBody struct {
	ItemID int64  `json:"itemId"`
	Name   string `json:"name"`
}

func (server *Server) registerNeedsAttention() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getNeedsAttention", Method: http.MethodGet, Path: "/needs-attention",
		Summary:    "How many folders and disks are offline, wait for the admin or could not be read (D280)",
		Tags:       []string{tagRunningIt},
		Extensions: callableBy(capabilityAdmin),
	}, server.getNeedsAttention)
}

func (server *Server) getNeedsAttention(
	ctx context.Context, input *AuthorizationHeader,
) (*NeedsAttentionOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	offline, err := dependencies.Libraries.Offline(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list what is offline", err)
	}
	waiting, err := dependencies.Libraries.Waiting(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list what waits for the admin", err)
	}
	unread, err := dependencies.Libraries.UnreadPaths(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list what could not be read", err)
	}
	output := &NeedsAttentionOutput{}
	if dependencies.Discovery != nil {
		addOneCopyWarnings(output, dependencies.Discovery.Status())
	}
	for _, missing := range offline {
		if missing.MountPath == "" {
			output.Body.RootCount++
		} else {
			output.Body.DiskCount++
		}
	}
	output.Body.WaitingRootCount, output.Body.UnreadCount = len(waiting), len(unread)
	output.Body.MatchCount, err = dependencies.Chooser.WaitingForAPerson(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "count what needs a match", err)
	}
	unplaced, err := dependencies.Chooser.SeriesWithUnplacedEpisodes(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "find the series read wrongly", err)
	}
	output.Body.SeriesReadWronglyCount = len(unplaced)
	unplaced = unplaced[:min(len(unplaced), attentionListedAtMost)]
	output.Body.SeriesReadWrongly = make([]SeriesReadWronglyBody, 0, len(unplaced))
	for _, series := range unplaced {
		output.Body.SeriesReadWrongly = append(output.Body.SeriesReadWrongly, SeriesReadWronglyBody{
			SeriesID: series.SeriesID, Name: series.SeriesName, UnplacedCount: series.FileCount,
		})
	}
	silent, silentTotal, err := dependencies.Chooser.FilmsWithoutSound(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "find the films with no sound", err)
	}
	output.Body.FilmsWithoutSoundCount = silentTotal
	silent = silent[:min(len(silent), attentionListedAtMost)]
	output.Body.FilmsWithoutSound = make([]FilmWithoutSoundBody, 0, len(silent))
	for _, film := range silent {
		output.Body.FilmsWithoutSound = append(output.Body.FilmsWithoutSound,
			FilmWithoutSoundBody{ItemID: film.TitleID, Name: film.Name})
	}
	if err := addRescansGivenUp(ctx, output, dependencies.Rescans); err != nil {
		return nil, server.libraryError(ctx, "list the rescans given up", err)
	}
	return output, nil
}

// addRescansGivenUp lists into output the libraries whose rescan gave up (the owner, 2026-11-13).
func addRescansGivenUp(ctx context.Context, output *NeedsAttentionOutput, rescans *identify.Rescans) error {
	if rescans == nil {
		return nil
	}
	given, err := rescans.GivenUp(ctx)
	if err != nil {
		return fmt.Errorf("list them: %w", err)
	}
	for _, library := range given[:min(len(given), attentionListedAtMost)] {
		output.Body.RescansGivenUp = append(output.Body.RescansGivenUp, RescanGivenUpBody{
			LibraryID: library.LibraryID, Name: library.LibraryName, GivenUpAt: library.GivenUpAt,
		})
	}
	return nil
}

// addOneCopyWarnings puts the discovery's two warnings of D437 §4 into output, as the System page
// shows them: this core refused as a copy of another that serves, and a copy of it refused.
func addOneCopyWarnings(output *NeedsAttentionOutput, status discovery.Status) {
	standing := discoveryStatusBody(status)
	output.Body.StoodAsideAt = standing.StoodAsideAt
	output.Body.CopyRefusedFrom, output.Body.CopyRefusedAt = standing.CopyRefusedFrom, standing.CopyRefusedAt
}
