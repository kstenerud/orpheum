// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// OwnLinkOutput is the signed-in person's own link (D292 §4; discovery-core.md K7): the link they
// registered by, which leads any device of theirs back to the server through its discovery.
type OwnLinkOutput struct {
	Body struct {
		// Secret is the link's secret, after # in its address; absent for a link made before the
		// server kept it, which the person asks the admin to make again.
		Secret string `json:"secret,omitempty"`
		// Link is the link to open or scan, while a discovery has claimed the server.
		Link      string `json:"link,omitempty"`
		ExpiresAt int64  `doc:"Unix milliseconds; each visit renews it" json:"expiresAt"`
	}
}

// getOwnLink is the person's own link, to open on another device or scan; 404 when they have none.
func (server *Server) getOwnLink(ctx context.Context, input *MeInput) (*OwnLinkOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	own, hasOwnLink, err := dependencies.Auth.OwnLink(ctx, principal.UserID)
	if err != nil {
		return nil, server.authError(ctx, "read your own link", err)
	}
	if !hasOwnLink {
		return nil, server.authError(ctx, "read your own link", auth.ErrNoOwnLink)
	}
	output := &OwnLinkOutput{}
	output.Body.Secret, output.Body.ExpiresAt = own.Secret, own.ExpiresAt.UnixMilli()
	if own.Secret != "" && dependencies.Discovery != nil {
		output.Body.Link = discoveryLink(dependencies.Discovery.Status(), "/register", own.Secret)
	}
	return output, nil
}
