// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// StartCastInput is a cast from the asking device to another of the person's, connected now (D337).
type StartCastInput struct {
	AuthorizationHeader
	Body struct {
		ToDeviceID int64 `doc:"Another of one's own devices, connected now" json:"toDeviceId" minimum:"1"`
		FileID     int64 `json:"fileId" minimum:"1"`
		// PositionMs is where the sender stood; absent, the receiver starts where the person left the
		// film, as a cast from a title's page does.
		PositionMs *int64 `doc:"Where to start, in ms; absent, where the person left the film" json:"positionMs,omitempty" minimum:"0" required:"false"` //nolint:lll // one struct tag
	}
}

// CastBody is one cast as one of its devices sees it, with a credential of its own for the cast's
// session of one, which its hello carries (D88).
type CastBody struct {
	CastID int64 `json:"castId"`
	// IsSender is whether the asking device cast it, and so is its remote; else it receives.
	IsSender             bool   `json:"isSender"`
	FileID               int64  `json:"fileId"`
	TitleID              int64  `doc:"The title the film is, 0 for none" json:"titleId"`
	TitleName            string `json:"titleName"`
	DurationMs           int64  `doc:"How long the film plays, 0 when unknown" json:"durationMs"`
	SenderDeviceID       int64  `json:"senderDeviceId"`
	SenderName           string `json:"senderName"`
	ReceiverDeviceID     int64  `json:"receiverDeviceId"`
	ReceiverName         string `json:"receiverName"`
	MembershipCredential string `doc:"What this device's session socket says hello with" json:"membershipCredential"`
}

// StartCastOutput is the cast started, as its sender sees it.
type StartCastOutput struct {
	Body CastBody
}

// MyCastsOutput is every cast the asking device sends or receives.
type MyCastsOutput struct {
	Body struct {
		Casts []CastBody `json:"casts"`
	}
}

// EndCastInput ends a cast the asking device sends or receives, saying why.
type EndCastInput struct {
	AuthorizationHeader
	CastID int64 `minimum:"1" path:"id"`
	Body   struct {
		Reason string `doc:"takenBack on the receiver, stopped on the remote, filmEnded at the film's end" enum:"takenBack,stopped,filmEnded" json:"reason"` //nolint:lll // one struct tag
	}
}

// EndCastOutput is where the cast stood as it ended, for the remote to offer to play from there
// (plans/cast-to-device.md §9 Q5).
type EndCastOutput struct {
	Body struct {
		PositionMs int64 `json:"positionMs"`
		IsPlaying  bool  `json:"isPlaying"`
	}
}

func (server *Server) registerCasts() {
	tags := []string{tagAccounts}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "startCast", Method: http.MethodPost, Path: "/me/casts",
		Summary: "Cast a film from this device to another of one's own, connected now (D337)", Tags: tags,
		DefaultStatus: http.StatusCreated,
	}, server.startCast)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listMyCasts", Method: http.MethodGet, Path: "/me/casts",
		Summary: "The casts this device sends or receives, each with a credential for it", Tags: tags,
	}, server.listMyCasts)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "endCast", Method: http.MethodPost, Path: "/me/casts/{id}/end",
		Summary: "End a cast this device sends or receives, and say where it stood", Tags: tags,
	}, server.endCast)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "moveCastOn", Method: http.MethodPost, Path: "/me/casts/{id}/next",
		Summary: "Move a cast this device sends or receives on to what plays after its film, the next " +
			"episode, which its receiver is handed (plans/up-next.md)",
		Tags: tags,
	}, server.moveCastOn)
}

// MoveCastOnInput moves a cast the asking device sends or receives on to its film's next episode.
type MoveCastOnInput struct {
	AuthorizationHeader
	CastID int64 `minimum:"1" path:"id"`
}

// moveCastOn moves a cast on to what plays after its film for its person (GET /files/{id}/up-next):
// the receiver is handed the cast again with it, and the remote reads it from the answer or, told by
// the cast's session that another film plays, from GET /me/casts. A film with nothing after it, or
// whose next episode cannot be read now, is a conflict; the cast goes on as it was.
func (server *Server) moveCastOn(ctx context.Context, input *MoveCastOnInput) (*StartCastOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	casts := dependencies.Live.CastsOf(principal.UserID, principal.DeviceID)
	index := slices.IndexFunc(casts, func(each live.Cast) bool { return each.ID == input.CastID })
	if index < 0 {
		return nil, castError(live.ErrNoCast)
	}
	cast := casts[index]
	next, hasNext, err := dependencies.Browser.UpNext(ctx, principal.UserID, cast.FileID)
	if err != nil {
		return nil, server.libraryError(ctx, "read what plays after a cast's film", err)
	}
	if !hasNext || next.Unreadable != nil {
		return nil, newError(http.StatusConflict, CodeConflict, "nothing that can be read now plays after this film")
	}
	if err = dependencies.Browser.ReachFile(ctx, principal.Reach, next.FileID); err != nil {
		return nil, server.playbackError(ctx, "reach a file", err)
	}
	work, err := dependencies.Browser.WorkOfFile(ctx, next.FileID)
	if err != nil {
		return nil, server.libraryError(ctx, "read what a cast plays", err)
	}
	moved, err := dependencies.Live.MoveOn(principal.UserID, principal.DeviceID, cast.ID, live.Film{
		FileID: next.FileID, TitleID: work.TitleID, TitleName: work.Name, DurationMs: work.DurationMs,
	})
	if err != nil {
		return nil, castError(err)
	}
	credential, err := dependencies.Together.CastCredential(moved.ID, principal.UserID)
	if err != nil {
		return nil, castError(live.ErrNoCast) // ended as it moved on
	}
	server.logger.InfoContext(ctx, "cast: a cast moved on to the next episode", "castId", moved.ID,
		logKeyUserID, moved.UserID, "by", principal.DeviceID, "fileId", moved.FileID)
	return &StartCastOutput{Body: castBodyOf(moved, principal.DeviceID, credential)}, nil
}

// startCast casts a film from the asking device to another of the person's, connected now, which is
// handed it on its live connection: a session of one in Watch Together's hub, which the receiver
// leads and the sender commands (D88, D337). The receiver's earlier cast, and the one the sender
// controlled, end (D337 §6: the last cast wins).
func (server *Server) startCast(ctx context.Context, input *StartCastInput) (*StartCastOutput, error) {
	dependencies, principal, err := server.requireSignedInReachingFile(ctx, input.Authorization, input.Body.FileID)
	if err != nil {
		return nil, err
	}
	if principal.DeviceID == 0 {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body", "this sign-in has no device to cast from")
	}
	senderName, receiverName, err := server.castNames(ctx, dependencies, principal, input.Body.ToDeviceID)
	if err != nil {
		return nil, err
	}
	work, err := dependencies.Browser.WorkOfFile(ctx, input.Body.FileID)
	if err != nil {
		return nil, server.libraryError(ctx, "read what a cast plays", err)
	}
	opened, err := dependencies.Together.OpenCast(together.Person{UserID: principal.UserID, Name: principal.Name})
	if err != nil {
		return nil, server.logInternalError(ctx, "open a cast", err)
	}
	cast := live.Cast{
		ID: opened.SessionID, UserID: principal.UserID, SenderDeviceID: principal.DeviceID,
		ReceiverDeviceID: input.Body.ToDeviceID, SenderName: senderName, ReceiverName: receiverName,
		FileID: input.Body.FileID, TitleID: work.TitleID, TitleName: work.Name, DurationMs: work.DurationMs,
		PositionMs: input.Body.PositionMs,
	}
	replaced, err := dependencies.Live.Cast(cast, opened.ReceiverCredential)
	if err != nil {
		//nolint:errcheck // just opened, it ends
		_ = dependencies.Together.EndCast(opened.SessionID, principal.UserID, together.CastStopped)
		return nil, castError(err)
	}
	for _, each := range replaced {
		reason := together.CastStopped // the sender went on to another receiver
		if each.ReceiverDeviceID == cast.ReceiverDeviceID {
			reason = together.CastReplaced
		}
		_ = dependencies.Together.EndCast(each.ID, principal.UserID, reason) //nolint:errcheck // gone is ended
	}
	server.logger.InfoContext(ctx, "cast: a device cast to another", "castId", cast.ID, logKeyUserID, cast.UserID,
		"from", cast.SenderDeviceID, "to", cast.ReceiverDeviceID, "fileId", cast.FileID, "replaced", len(replaced))
	return &StartCastOutput{Body: castBodyOf(cast, principal.DeviceID, opened.SenderCredential)}, nil
}

// castNames are the names of the asking device and of the one it casts to, which must be another of
// the person's; one that is not is answered as a device not connected.
func (server *Server) castNames(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, toDeviceID int64,
) (senderName, receiverName string, err error) {
	if toDeviceID == principal.DeviceID {
		return "", "", castError(live.ErrToItself)
	}
	devices, err := dependencies.Auth.Devices(ctx, principal.UserID)
	if err != nil {
		return "", "", server.authError(ctx, "list devices", err)
	}
	isOwn := false
	for _, device := range devices {
		switch device.ID {
		case principal.DeviceID:
			senderName = device.Name
		case toDeviceID:
			receiverName, isOwn = device.Name, true
		}
	}
	if !isOwn {
		return "", "", castError(live.ErrNotConnected)
	}
	return senderName, receiverName, nil
}

// listMyCasts is the casts the asking device sends or receives, each with a new credential for its
// session, for a page loaded again to be the remote or the receiver once more (§6.3).
func (server *Server) listMyCasts(ctx context.Context, input *AuthorizationHeader) (*MyCastsOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &MyCastsOutput{}
	output.Body.Casts = []CastBody{}
	for _, cast := range dependencies.Live.CastsOf(principal.UserID, principal.DeviceID) {
		credential, err := dependencies.Together.CastCredential(cast.ID, principal.UserID)
		if errors.Is(err, together.ErrNoSession) {
			continue // ended as it was read
		}
		if err != nil {
			return nil, server.logInternalError(ctx, "make a cast's credential", err)
		}
		output.Body.Casts = append(output.Body.Casts, castBodyOf(cast, principal.DeviceID, credential))
	}
	return output, nil
}

// endCast ends a cast the asking device sends or receives: each of its devices is told why, and the
// answer says where it stood.
func (server *Server) endCast(ctx context.Context, input *EndCastInput) (*EndCastOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	cast, err := dependencies.Live.End(principal.UserID, principal.DeviceID, input.CastID)
	if err != nil {
		return nil, castError(err)
	}
	output := &EndCastOutput{}
	positionMs, isPlaying, err := dependencies.Together.CastPosition(cast.ID, principal.UserID)
	if err == nil {
		output.Body.PositionMs, output.Body.IsPlaying = positionMs, isPlaying
		err = dependencies.Together.EndCast(cast.ID, principal.UserID, input.Body.Reason)
	}
	if err != nil && !errors.Is(err, together.ErrNoSession) {
		return nil, server.logInternalError(ctx, "end a cast", err)
	}
	server.logger.InfoContext(ctx, "cast: a cast ended", "castId", cast.ID, logKeyUserID, cast.UserID,
		"by", principal.DeviceID, "reason", input.Body.Reason)
	return output, nil
}

// castBodyOf is cast as the device thisDeviceID sees it, with its credential.
func castBodyOf(cast live.Cast, thisDeviceID int64, credential string) CastBody {
	return CastBody{
		CastID: cast.ID, IsSender: cast.SenderDeviceID == thisDeviceID, FileID: cast.FileID,
		TitleID: cast.TitleID, TitleName: cast.TitleName, DurationMs: cast.DurationMs,
		SenderDeviceID: cast.SenderDeviceID, SenderName: cast.SenderName,
		ReceiverDeviceID: cast.ReceiverDeviceID, ReceiverName: cast.ReceiverName, MembershipCredential: credential,
	}
}

// castError is the answer to a cast the registry refused.
func castError(err error) error {
	switch {
	case errors.Is(err, live.ErrNotConnected):
		return newError(http.StatusConflict, CodeConflict, err.Error())
	case errors.Is(err, live.ErrToItself):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.toDeviceId", err.Error())
	}
	return newError(http.StatusNotFound, CodeNotFound, err.Error())
}
