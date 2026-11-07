// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"slices"
)

// MyTogetherBody is one session the person is in, and, for one they joined, a credential for the
// device that asked.
type MyTogetherBody struct {
	SessionID            int64  `json:"sessionId"`
	MemberID             int64  `json:"memberId"`
	IsHost               bool   `json:"isHost"`
	MembershipCredential string `doc:"This device's credential for a session you joined; none for the one you host, which stays on its device (D442 §1)" json:"membershipCredential,omitempty" required:"false"` //nolint:lll // one struct tag
	Locator              string `doc:"What this device finds the server by through a discovery; the host's has none" json:"locator,omitempty"`                                                                   //nolint:lll // one struct tag
	HostDeviceName       string `doc:"The name of your device the session you host is on now, where it is on one" json:"hostDeviceName,omitempty" required:"false"`                                              //nolint:lll // one struct tag
}

// MyTogetherOutput is every session the signed-in person is in.
type MyTogetherOutput struct {
	Body struct {
		Sessions []MyTogetherBody `json:"sessions"`
	}
}

// listMyTogether is the sessions the signed-in person is in, as host or as a member who joined
// signed in: one they joined follows them to any device they sign in on, and having it back here cuts
// off none of their other devices (D267 §2, §3); the one they host is said to be on, on the device it
// is on, which it stays on until it is moved (D442 §1).
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
		if len(each.HostDevices) > 0 {
			body.HostDeviceName, err = server.deviceNamed(ctx, dependencies, principal.UserID, each.HostDevices)
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
