// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
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
	}
}

// SeriesReadWronglyBody is a series some of whose episodes' file names were likely read wrongly.
type SeriesReadWronglyBody struct {
	SeriesID      int64  `json:"seriesId"`
	Name          string `json:"name"`
	UnplacedCount int    `doc:"How many of its episodes have no number outside a specials or extras folder" json:"unplacedCount"` //nolint:lll // one struct tag
}

// FilmWithoutSoundBody is a film whose video file has no sound track.
type FilmWithoutSoundBody struct {
	TitleID int64  `json:"titleId"`
	Name    string `json:"name"`
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
			FilmWithoutSoundBody{TitleID: film.TitleID, Name: film.Name})
	}
	return output, nil
}
