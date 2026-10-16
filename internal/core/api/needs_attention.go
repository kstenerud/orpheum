// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

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
		// or extras folder holds them: their files' names were likely read wrongly (D306 §6).
		SeriesReadWrongly []SeriesReadWronglyBody `json:"seriesReadWrongly"`
	}
}

// SeriesReadWronglyBody is a series some of whose episodes' file names were likely read wrongly.
type SeriesReadWronglyBody struct {
	SeriesID      int64  `json:"seriesId"`
	Name          string `json:"name"`
	UnplacedCount int    `doc:"How many of its episodes have no number outside a specials or extras folder" json:"unplacedCount"` //nolint:lll // one struct tag
}

func (server *Server) registerNeedsAttention() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getNeedsAttention", Method: http.MethodGet, Path: "/needs-attention",
		Summary: "How many folders and disks are offline, wait for the admin or could not be read (admin; D280)",
		Tags:    []string{tagRunningIt},
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
	output.Body.SeriesReadWrongly = make([]SeriesReadWronglyBody, 0, len(unplaced))
	for _, series := range unplaced {
		output.Body.SeriesReadWrongly = append(output.Body.SeriesReadWrongly, SeriesReadWronglyBody{
			SeriesID: series.SeriesID, Name: series.SeriesName, UnplacedCount: series.FileCount,
		})
	}
	return output, nil
}
