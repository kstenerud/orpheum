// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// requireSignedInReachingFile is requireSignedIn for a request about one file, which must be in a
// library the person reaches: one that is not is answered as a file that does not exist
// (D243 §4).
func (server *Server) requireSignedInReachingFile(
	ctx context.Context, authorization string, fileID int64,
) (*Dependencies, auth.Principal, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, authorization)
	if err != nil {
		return nil, principal, err
	}
	if err = dependencies.Browser.ReachFile(ctx, principal.Reach, fileID); err != nil {
		return nil, principal, server.playbackError(ctx, "reach a file", err)
	}
	return dependencies, principal, nil
}

// requireSignedInReachingItem is requireSignedIn for a request about one title or series, which
// must be in a library the person reaches: one that is not is answered by errorOf as an item that
// does not exist (D243 §4).
func (server *Server) requireSignedInReachingItem(
	ctx context.Context, authorization string, itemID int64,
	errorOf func(ctx context.Context, what string, err error) error,
) (*Dependencies, auth.Principal, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, authorization)
	if err != nil {
		return nil, principal, err
	}
	if err = dependencies.Browser.ReachItem(ctx, principal.Reach, itemID); err != nil {
		return nil, principal, errorOf(ctx, "reach an item", err)
	}
	return dependencies, principal, nil
}
