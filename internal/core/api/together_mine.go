// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "context"

// MyTogetherBody is one session the person is in, and a credential for the device that asked.
type MyTogetherBody struct {
	SessionID            int64  `json:"sessionId"`
	MemberID             int64  `json:"memberId"`
	IsHost               bool   `json:"isHost"`
	MembershipCredential string `json:"membershipCredential"`
	Locator              string `doc:"What this device finds the server by through a discovery; the host's has none" json:"locator,omitempty"` //nolint:lll // one struct tag
}

// MyTogetherOutput is every session the signed-in person is in.
type MyTogetherOutput struct {
	Body struct {
		Sessions []MyTogetherBody `json:"sessions"`
	}
}

// listMyTogether is the sessions the signed-in person is in, as host or as a member who joined
// signed in: a person's session follows them to any device they sign in on, and having it back
// here cuts off none of their other devices (D267 §2, §3).
func (server *Server) listMyTogether(ctx context.Context, input *AuthorizationHeader) (*MyTogetherOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	mine, err := dependencies.Together.Mine(principal.UserID)
	if err != nil {
		return nil, server.togetherError(ctx, "list your sessions", err)
	}
	output := &MyTogetherOutput{}
	output.Body.Sessions = make([]MyTogetherBody, 0, len(mine))
	for _, each := range mine {
		output.Body.Sessions = append(output.Body.Sessions, MyTogetherBody{
			SessionID: each.SessionID, MemberID: each.MemberID, IsHost: each.IsHost,
			MembershipCredential: each.MembershipCredential, Locator: each.Locator,
		})
	}
	return output, nil
}
