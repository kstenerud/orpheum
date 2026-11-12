// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// GuestGrantInput asks for a grant for a Google Cast device to receive a cast from the asking device
// (D465 §2), named as the Cast device names itself.
type GuestGrantInput struct {
	AuthorizationHeader
	Body struct {
		ReceiverName string `doc:"The Cast device's name, as Chrome's Cast menu shows it" json:"receiverName" maxLength:"100" minLength:"1"` //nolint:lll // one struct tag
	}
}

// GuestGrantOutput is the grant, which the sender hands the receiver over the Cast channel and never
// in a URL (D465 §2; D154 §6).
type GuestGrantOutput struct {
	Body struct {
		Grant string `doc:"Traded once, within a minute, by the receiver at POST /guest-receivers" json:"grant"`
	}
}

// TradeGuestGrantInput is a Google Cast device trading the grant its sender handed it.
type TradeGuestGrantInput struct {
	Body struct {
		Grant string `json:"grant" maxLength:"100" minLength:"1"`
	}
}

// TradeGuestGrantOutput is a guest receiver's session: the token its requests and sockets carry, the
// locator its discovery finds the core by, and the device it is, which its sender casts to. It keeps
// none of it after its cast (D465 §1).
type TradeGuestGrantOutput struct {
	Body struct {
		Token    string `json:"token"`
		Locator  string `json:"locator"`
		DeviceID int64  `json:"deviceId"`
	}
}

func (server *Server) registerGuestReceivers() {
	tags := []string{tagUsers}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "issueGuestGrant", Method: http.MethodPost, Path: "/me/casts/guest-grants",
		Summary: "A grant for a Google Cast device to receive a cast from this device (D465 §2)",
		Tags:    tags, DefaultStatus: http.StatusCreated,
		Extensions: callableBy(capabilitySignedIn),
	}, server.issueGuestGrant)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "tradeGuestGrant", Method: http.MethodPost, Path: "/guest-receivers",
		Summary: "Trade a Google Cast device's grant, once, for its session for one cast (D465 §2)",
		Tags:    tags, DefaultStatus: http.StatusCreated,
		Extensions: callableBy(capabilityNone),
	}, server.tradeGuestGrant)
}

func (server *Server) issueGuestGrant(ctx context.Context, input *GuestGrantInput) (*GuestGrantOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	grant, err := dependencies.Auth.IssueGuestGrant(principal, input.Body.ReceiverName)
	if err != nil {
		return nil, server.authError(ctx, "issue a guest receiver's grant", err)
	}
	output := &GuestGrantOutput{}
	output.Body.Grant = grant
	return output, nil
}

// tradeGuestGrant signs a Google Cast device in as a guest receiver: a device of the grant's person,
// listed nowhere, which only the grant's sender casts to, once, and which is removed as that cast ends
// or if none starts (D465 §1). The grant is never logged.
func (server *Server) tradeGuestGrant(
	ctx context.Context, input *TradeGuestGrantInput,
) (*TradeGuestGrantOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	traded, err := dependencies.Auth.TradeGuestGrant(ctx, input.Body.Grant)
	if err != nil {
		return nil, server.authError(ctx, "trade a guest receiver's grant", err)
	}
	dependencies.Live.ExpectGuest(traded.UserID, traded.DeviceID, traded.SenderDeviceID, traded.Name)
	server.logger.InfoContext(ctx, "cast: a guest receiver signed in", slogx.EventKey, "guestReceiverSignedIn",
		logKeyUserID, traded.UserID, logKeyDeviceID, traded.DeviceID, "sender", traded.SenderDeviceID)
	output := &TradeGuestGrantOutput{}
	output.Body.Token, output.Body.Locator, output.Body.DeviceID = traded.Token, traded.Locator, traded.DeviceID
	return output, nil
}

// guestPlays is whether a guest receiver may start fileID: the film of the cast it receives, as handed
// or played in the cast since, or the film its cast's session plays (D465 §2).
func guestPlays(dependencies *Dependencies, guest auth.Principal, fileID int64) bool {
	for _, cast := range dependencies.Live.CastsOf(guest.UserID, guest.DeviceID) {
		if cast.ReceiverDeviceID != guest.DeviceID {
			continue
		}
		if cast.FileID == fileID {
			return true
		}
		details, err := dependencies.Together.DetailsOf(cast.ID, guest.UserID)
		if err == nil && details.Group.FileID == fileID {
			return true
		}
	}
	return false
}

// guestRefusal is the API's error for auth's refusal of a guest receiver's grant, or nil for another.
func guestRefusal(err error) *Error {
	switch {
	case errors.Is(err, auth.ErrNoGuestGrant):
		return newError(http.StatusNotFound, CodeNotFound, auth.ErrNoGuestGrant.Error())
	case errors.Is(err, auth.ErrGuestSender):
		return newError(http.StatusForbidden, CodeForbidden, auth.ErrGuestSender.Error())
	}
	return nil
}
