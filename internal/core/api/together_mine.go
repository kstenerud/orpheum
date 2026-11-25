// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"slices"
)

// MyTogetherBody is one session the person is in, where it is, and, for one they joined whose part is
// on no device, a credential for the device that asked.
type MyTogetherBody struct {
	SessionID            int64  `json:"sessionId"`
	MemberID             int64  `json:"memberId"`
	IsHost               bool   `json:"isHost"`
	MembershipCredential string `doc:"This device's credential for a session you joined whose part is on none of your devices; none while it is on one, nor for the one you host: each stays on its device until moved (D442 §1, D555)" json:"membershipCredential,omitempty" required:"false"` //nolint:lll // one struct tag
	Locator              string `doc:"What this device finds the server by through a discovery, with a credential alone" json:"locator,omitempty"`                                                                                                                                              //nolint:lll // one struct tag
	DeviceName           string `doc:"The name of your device your part in the session is on now, the host's or a guest's, where it is on one" json:"deviceName,omitempty" required:"false"`                                                                                                    //nolint:lll // one struct tag
}

// MyTogetherOutput is every session the signed-in person is in.
type MyTogetherOutput struct {
	Body struct {
		Sessions []MyTogetherBody `json:"sessions"`
	}
}

// listMyTogether is the sessions the signed-in person is in, as host or as a member who joined
// signed in, each said to be on, on the device their part is on, which it stays on until they move it
// (D442 §1; D555, the owner's point 3): asking from another device takes it from none.
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
		body := MyTogetherBody{
			SessionID: each.SessionID, MemberID: each.MemberID, IsHost: each.IsHost,
			MembershipCredential: each.MembershipCredential, Locator: each.Locator,
		}
		if len(each.Devices) > 0 {
			body.DeviceName, err = server.deviceNamed(ctx, dependencies, principal.UserID, each.Devices)
			if err != nil {
				return nil, err
			}
		}
		output.Body.Sessions = append(output.Body.Sessions, body)
	}
	return output, nil
}

// deviceNamed is the name of the first of userID's devices in ids, or "" when none is theirs.
func (server *Server) deviceNamed(ctx context.Context, dependencies *Dependencies, userID int64, ids []int64) (
	string, error,
) {
	devices, err := dependencies.Auth.Devices(ctx, userID)
	if err != nil {
		return "", server.authError(ctx, "list devices", err)
	}
	for _, device := range devices {
		if slices.Contains(ids, device.ID) {
			return device.Name, nil
		}
	}
	return "", nil
}
