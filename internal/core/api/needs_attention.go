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
	}
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
	return output, nil
}
