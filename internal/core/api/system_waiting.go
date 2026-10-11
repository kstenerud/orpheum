// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
)

// WaitingBody is a library folder whose scan waits for the admin before removing files (D208 §3,
// D277 §2): why, since when, how many files it would remove and the first of them.
type WaitingBody struct {
	LibraryID   int64  `json:"libraryId"`
	LibraryName string `json:"libraryName"`
	NodeName    string `json:"nodeName"`
	RootPath    string `json:"rootPath"`
	Reason      string `json:"reason"`
	// WaitingAt is when the scan that waits started, in Unix milliseconds.
	WaitingAt int64 `json:"waitingAt"`
	FileCount int   `doc:"How many files the scan would remove" json:"fileCount"`
	// FirstPaths are the first of them by path, at most ten.
	FirstPaths []string `json:"firstPaths"`
}

// UnreadBody is a file or folder a scan could not read, with the platform's account of why
// (D163 §8, D277 §3); the node's uid and gid, in its System entry, say whom to make it readable to.
type UnreadBody struct {
	LibraryID   int64  `json:"libraryId"`
	LibraryName string `json:"libraryName"`
	NodeName    string `json:"nodeName"`
	Path        string `json:"path"`
	// Code is the scan's error code: permission_denied, say.
	Code        string          `json:"code"`
	ErrorDetail ErrorDetailBody `json:"errorDetail"`
	// FoundAt is when a scan first found it, in Unix milliseconds.
	FoundAt int64 `json:"foundAt"`
}

// fillWhatWaits puts into output the folders whose removals wait for the admin and what scans could
// not read.
func (server *Server) fillWhatWaits(ctx context.Context, dependencies *Dependencies, output *SystemOutput) error {
	waiting, err := dependencies.Libraries.Waiting(ctx)
	if err != nil {
		return server.libraryError(ctx, "list what waits for the admin", err)
	}
	output.Body.Waiting = make([]WaitingBody, 0, len(waiting))
	for _, root := range waiting {
		output.Body.Waiting = append(output.Body.Waiting, WaitingBody{
			LibraryID: root.LibraryID, LibraryName: root.LibraryName, NodeName: root.NodeName,
			RootPath: root.RootPath, Reason: root.Reason, WaitingAt: root.Since.UnixMilli(),
			FileCount: root.FileCount, FirstPaths: root.FirstPaths,
		})
	}
	unread, err := dependencies.Libraries.UnreadPaths(ctx)
	if err != nil {
		return server.libraryError(ctx, "list what could not be read", err)
	}
	output.Body.Unread = make([]UnreadBody, 0, len(unread))
	for _, path := range unread {
		detail := path.Detail
		output.Body.Unread = append(output.Body.Unread, UnreadBody{
			LibraryID: path.LibraryID, LibraryName: path.LibraryName, NodeName: path.NodeName, Path: path.Path,
			Code: path.Code, FoundAt: path.Since.UnixMilli(),
			ErrorDetail: ErrorDetailBody{
				Text: detail.Text, OwnerUID: detail.OwnerUID, GroupGID: detail.GroupGID, ModeText: detail.ModeText,
				OccurrenceCount: detail.OccurrenceCount,
			},
		})
	}
	return nil
}
