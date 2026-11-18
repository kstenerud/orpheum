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
// A grant a Watch Together guest by link asked for (D518 §1) signs nothing in: it answers no token
// and no device, but a credential of the guest's membership, whose part the Cast device takes in
// togetherSessionId, and that credential's locator.
type TradeGuestGrantOutput struct {
	Body struct {
		Token    string `doc:"The guest receiver's session; absent for a Watch Together guest's grant" json:"token,omitempty" required:"false"` //nolint:lll // one struct tag
		Locator  string `json:"locator"`
		DeviceID int64  `doc:"The device it is; absent for a Watch Together guest's grant" json:"deviceId,omitempty" required:"false"` //nolint:lll // one struct tag
		// MembershipCredential is a credential of the guest's membership for a grant a Watch Together
		// guest asked for (D518 §1), which the device says hello and plays with.
		MembershipCredential string `doc:"For a Watch Together guest's grant: the credential of their membership the device takes their part by" json:"membershipCredential,omitempty" required:"false"` //nolint:lll // one struct tag
		TogetherSessionID    int64  `doc:"For a Watch Together guest's grant: the session the device takes their part in" json:"togetherSessionId,omitempty" required:"false"`                           //nolint:lll // one struct tag
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
		OperationID: "issueTogetherGuestGrant", Method: http.MethodPost, Path: "/together/cast-grants",
		Summary: "A grant for a Google Cast device to take your part in the Watch Together session you " +
			"joined by your link, with your rights, while you are in (D518 §1)",
		Tags: tags, DefaultStatus: http.StatusCreated,
		Extensions: callableBy(capabilityMember),
	}, server.issueTogetherGuestGrant)
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
	if traded.IsForMembership() {
		return server.takeGuestsPart(ctx, dependencies, traded)
	}
	dependencies.Live.ExpectGuest(traded.UserID, traded.DeviceID, traded.SenderDeviceID, traded.Name)
	server.logger.InfoContext(ctx, "cast: a guest receiver signed in", slogx.EventKey, "guestReceiverSignedIn",
		logKeyUserID, traded.UserID, logKeyDeviceID, traded.DeviceID, "sender", traded.SenderDeviceID)
	output := &TradeGuestGrantOutput{}
	output.Body.Token, output.Body.Locator, output.Body.DeviceID = traded.Token, traded.Locator, traded.DeviceID
	return output, nil
}

// GuestPartGrantInput asks, by a Watch Together guest's membership credential, for a grant for a
// Google Cast device to take their part (D518 §1), named as the Cast device names itself.
type GuestPartGrantInput struct {
	AuthorizationHeader
	Body struct {
		ReceiverName string `doc:"The Cast device's name, as Chrome's Cast menu shows it" json:"receiverName" maxLength:"100" minLength:"1"` //nolint:lll // one struct tag
	}
}

// issueTogetherGuestGrant is issueGuestGrant for a Watch Together guest who joined by their link,
// signed in to nothing (D518 §1): the grant is asked for under their membership credential and bound
// to their membership, traded at POST /guest-receivers as a person's is, within the same minute and
// once. The host's credential is refused: the host is signed in, and casts as themselves.
func (server *Server) issueTogetherGuestGrant(
	ctx context.Context, input *GuestPartGrantInput,
) (*GuestGrantOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	viewer, isViewer := dependencies.Together.ViewerOf(bearerToken(input.Authorization))
	if !isViewer {
		return nil, newError(http.StatusNotFound, CodeNotFound, "you are in no session as a guest now")
	}
	grant, err := dependencies.Auth.IssueMembershipGuestGrant(
		auth.GrantMembership{SessionID: viewer.SessionID, MemberID: viewer.MemberID}, input.Body.ReceiverName)
	if err != nil {
		return nil, server.authError(ctx, "issue a Watch Together guest's cast grant", err)
	}
	server.logger.InfoContext(ctx, "cast: a Watch Together guest asked for a cast grant",
		"sessionId", viewer.SessionID, "memberId", viewer.MemberID)
	output := &GuestGrantOutput{}
	output.Body.Grant = grant
	return output, nil
}

// takeGuestsPart answers a grant a Watch Together guest asked for, traded: a credential of their
// membership for the Cast device, which with it is another device of the guest's in their session,
// following the group with their rights alone (D518 §1). A membership ended since, the guest removed
// or the session over, gives none, answered as a grant unknown.
func (server *Server) takeGuestsPart(
	ctx context.Context, dependencies *Dependencies, traded auth.GuestReceiver,
) (*TradeGuestGrantOutput, error) {
	part, err := dependencies.Together.GuestCastPart(traded.Membership.SessionID, traded.Membership.MemberID)
	if err != nil {
		server.logger.InfoContext(ctx, "cast: a Watch Together guest's grant was traded after their membership ended",
			"sessionId", traded.Membership.SessionID, "memberId", traded.Membership.MemberID, "why", err.Error())
		return nil, newError(http.StatusNotFound, CodeNotFound, auth.ErrNoGuestGrant.Error())
	}
	server.logger.InfoContext(ctx, "cast: a Google Cast device took a Watch Together guest's part",
		slogx.EventKey, "guestPartTaken", "sessionId", part.SessionID, "memberId", part.MemberID)
	output := &TradeGuestGrantOutput{}
	output.Body.Locator, output.Body.MembershipCredential = part.Locator, part.MembershipCredential
	output.Body.TogetherSessionID = part.SessionID
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
