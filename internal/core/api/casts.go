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
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// StartCastInput is a casting session from the asking device to another of the person's, connected
// now (D337, D364), with the video the asking device plays, if any, which moves to it (D364 §1).
type StartCastInput struct {
	AuthorizationHeader
	Body struct {
		ToDeviceID int64 `doc:"Another of one's own devices, connected now" json:"toDeviceId" minimum:"1"`
		CastItem
		// IsPaused is whether the sender's film stood paused, so the receiver opens it paused at
		// positionMs, waiting for a play (the owner, 2026-11-08).
		IsPaused bool `doc:"Whether the receiver opens the film paused at positionMs, as the sender had it" json:"isPaused,omitempty" required:"false"` //nolint:lll // one struct tag
		// TogetherCredential is the Watch Together membership the asking page holds as a guest, whose
		// part in the session the receiver takes, following the group (D371 §4); absent, the person's
		// own part, the session they host first.
		TogetherCredential string `doc:"The membership this page holds as a Watch Together guest, whose part the receiver takes" json:"togetherCredential,omitempty" maxLength:"100" required:"false"` //nolint:lll // one struct tag
	}
}

// CastItem is what a cast plays: an item's file from a position, or, with no fileId, nothing, its receiver
// ready for the next (D364 §5).
type CastItem struct {
	FileID int64 `doc:"What to play; absent, nothing, the receiver ready for the next" json:"fileId,omitempty" minimum:"1" required:"false"` //nolint:lll // one struct tag
	// PositionMs is where the sender stood; absent, the receiver starts where the person left the
	// film.
	PositionMs *int64 `doc:"Where to start, in ms; absent, where the person left the film" json:"positionMs,omitempty" minimum:"0" required:"false"` //nolint:lll // one struct tag
}

// CastBody is one cast as one of its devices sees it, with a credential of its own for the cast's
// session of one, which its hello carries (D88).
type CastBody struct {
	CastID int64 `json:"castId"`
	// IsSender is whether the asking device cast it, and so is its remote; else it receives.
	IsSender bool `json:"isSender"`
	// IsAccepted is whether its receiver took it, asking nothing (D468 §1).
	IsAccepted           bool   `json:"isAccepted"`
	FileID               int64  `doc:"What plays, 0 for nothing, the receiver ready (D364 §5)" json:"fileId"`
	ItemID               int64  `doc:"The item the film is, 0 for none" json:"itemId"`
	ItemName             string `json:"itemName"`
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
		Reason string `doc:"takenBack on the receiver, stopped on the remote, declined or unanswered as the receiver was asked (D358); no receiver takes the cast over (D446 §4)" enum:"takenBack,stopped,declined,unanswered" json:"reason"` //nolint:lll // one struct tag
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
	tags := []string{tagUsers}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "startCast", Method: http.MethodPost, Path: "/me/casts",
		Summary: "Start casting from this device to another of one's own, connected now (D337, D364)",
		Tags:    tags, DefaultStatus: http.StatusCreated,
		Extensions: callableBy(capabilitySignedIn),
	}, server.startCast)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "playInCast", Method: http.MethodPost, Path: "/me/casts/{id}/play",
		Summary:    "Play a film on a cast's receiver, or nothing, the receiver ready for the next (D364 §3, §5)",
		Tags:       tags,
		Extensions: callableBy(capabilitySignedIn),
	}, server.playInCast)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "acceptCast", Method: http.MethodPost, Path: "/me/casts/{id}/accept",
		Summary: "Say Use, on its receiver, to a cast it was asked about (D364 §2)", Tags: tags,
		Extensions: callableBy(capabilitySignedInOrGuest),
	}, server.acceptCast)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listMyCasts", Method: http.MethodGet, Path: "/me/casts",
		Summary: "The casts this device sends or receives, each with a credential for it", Tags: tags,
		Extensions: callableBy(capabilitySignedIn),
	}, server.listMyCasts)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "endCast", Method: http.MethodPost, Path: "/me/casts/{id}/end",
		Summary: "End a cast this device sends or receives, and say where it stood", Tags: tags,
		Extensions: callableBy(capabilitySignedInOrGuest),
	}, server.endCast)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "moveCastOn", Method: http.MethodPost, Path: "/me/casts/{id}/next",
		Summary: "Move a cast this device sends or receives on to what plays after its film, the next " +
			"episode, which its receiver is handed (plans/up-next.md)",
		Tags:       tags,
		Extensions: callableBy(capabilitySignedIn),
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
		return nil, server.castRefused(ctx, principal, "moveOn", live.ErrNoCast, "castId", input.CastID)
	}
	cast := casts[index]
	next, hasNext, err := dependencies.Browser.UpNext(ctx, principal.Reach, principal.UserID, cast.FileID)
	if err != nil {
		return nil, server.libraryError(ctx, "read what plays after a cast's film", err)
	}
	if !hasNext || next.Unreadable != nil {
		return nil, newError(http.StatusConflict, CodeConflict, "nothing that can be read now plays after this film")
	}
	if err = dependencies.Browser.ReachFile(ctx, principal.Reach, next.FileID); err != nil {
		return nil, server.playbackError(ctx, "reach a file", err)
	}
	work, err := dependencies.Browser.WorkOfFile(ctx, principal.Reach, next.FileID)
	if err != nil {
		return nil, server.libraryError(ctx, "read what a cast plays", err)
	}
	moved, err := dependencies.Live.Play(principal.UserID, principal.DeviceID, cast.ID, live.Film{
		FileID: next.FileID, ItemID: work.TitleID, ItemName: work.Name, DurationMs: work.DurationMs,
	}, nil)
	if err != nil {
		return nil, server.castRefused(ctx, principal, "moveOn", err, "castId", input.CastID)
	}
	credential, err := dependencies.Together.CastCredential(moved.ID, principal.UserID)
	if err != nil {
		// ended as it moved on
		return nil, server.castRefused(ctx, principal, "moveOn", live.ErrNoCast, "castId", input.CastID)
	}
	server.logger.InfoContext(ctx, "cast: a cast moved on to the next episode", "castId", moved.ID,
		logKeyUserID, moved.UserID, "by", principal.DeviceID, "fileId", moved.FileID)
	return &StartCastOutput{Body: castBodyOf(moved, principal.DeviceID, credential)}, nil
}

// startCast starts casting from the asking device to another of the person's, connected now, which
// is handed it on its live connection: a session of one in Watch Together's hub, which the receiver
// leads and the sender commands (D88, D337), playing the video given, if any, and every one the
// person plays after it until it ends (D364). A device that receives a cast is no device's target
// until that cast ends, receiving_a_cast (D386 §1), and one that casts to the asker, directly or
// through others, is not its target, casting_here; the cast the sender controlled before ends, as
// stopped (D337 §6).
func (server *Server) startCast(ctx context.Context, input *StartCastInput) (*StartCastOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
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
	film, err := server.castFilmOf(ctx, dependencies, principal, input.Body.FileID)
	if err != nil {
		return nil, err
	}
	opened, err := dependencies.Together.OpenCast(together.Person{UserID: principal.UserID, Name: principal.Name})
	if err != nil {
		return nil, server.logInternalError(ctx, "open a cast", err)
	}
	cast := live.Cast{
		ID: opened.SessionID, UserID: principal.UserID, SenderDeviceID: principal.DeviceID,
		ReceiverDeviceID: input.Body.ToDeviceID, SenderName: senderName, ReceiverName: receiverName,
		FileID: film.FileID, ItemID: film.ItemID, ItemName: film.ItemName, DurationMs: film.DurationMs,
		PositionMs: input.Body.PositionMs, IsPaused: input.Body.IsPaused,
		SendersMembership: input.Body.TogetherCredential,
	}
	cast, left, err := dependencies.Live.Cast(cast, opened.ReceiverCredential)
	if err != nil {
		//nolint:errcheck // just opened, it ends
		_ = dependencies.Together.EndCast(opened.SessionID, principal.UserID, together.CastStopped)
		return nil, server.castRefused(ctx, principal, "start", err, "to", input.Body.ToDeviceID)
	}
	for _, each := range left { // the sender went on to another receiver
		//nolint:errcheck // gone is ended
		_ = dependencies.Together.EndCast(each.ID, principal.UserID, together.CastStopped)
	}
	server.logger.InfoContext(ctx, "cast: a device cast to another", "castId", cast.ID, logKeyUserID, cast.UserID,
		"from", cast.SenderDeviceID, "to", cast.ReceiverDeviceID, "fileId", cast.FileID, "left", len(left))
	return &StartCastOutput{Body: castBodyOf(cast, principal.DeviceID, opened.SenderCredential)}, nil
}

// castFilmOf is the film fileID as a cast plays it, which the person must reach; nothing, the
// receiver ready, for 0 (D364 §5).
func (server *Server) castFilmOf(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, fileID int64,
) (live.Film, error) {
	if fileID == 0 {
		return live.Film{}, nil
	}
	if err := dependencies.Browser.ReachFile(ctx, principal.Reach, fileID); err != nil {
		return live.Film{}, server.playbackError(ctx, "reach a file", err)
	}
	work, err := dependencies.Browser.WorkOfFile(ctx, principal.Reach, fileID)
	if err != nil {
		return live.Film{}, server.libraryError(ctx, "read what a cast plays", err)
	}
	return live.Film{FileID: fileID, ItemID: work.TitleID, ItemName: work.Name, DurationMs: work.DurationMs}, nil
}

// PlayInCastInput plays a film on a cast's receiver, the asking device its sender or its receiver.
type PlayInCastInput struct {
	AuthorizationHeader
	CastID int64 `minimum:"1" path:"id"`
	Body   CastItem
}

// playInCast plays a film on a cast's receiver, which is handed the cast again with it: whatever the
// person plays on the sender while the cast lasts (D364 §3). With no film the receiver plays nothing,
// ready for the next, the cast's group stopped so the next is put on afresh: the remote's "Play on
// this device instead" (§4) and a film's end with nothing after it (§5).
func (server *Server) playInCast(ctx context.Context, input *PlayInCastInput) (*StartCastOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	film, err := server.castFilmOf(ctx, dependencies, principal, input.Body.FileID)
	if err != nil {
		return nil, err
	}
	if film.FileID == 0 {
		err = dependencies.Together.CastReady(input.CastID, principal.UserID)
		if err != nil {
			return nil, server.castRefused(ctx, principal, "play", live.ErrNoCast, "castId", input.CastID)
		}
	}
	played, err := dependencies.Live.Play(principal.UserID, principal.DeviceID, input.CastID, film,
		input.Body.PositionMs)
	if err != nil {
		return nil, server.castRefused(ctx, principal, "play", err, "castId", input.CastID)
	}
	credential, err := dependencies.Together.CastCredential(played.ID, principal.UserID)
	if err != nil {
		// ended as it played
		return nil, server.castRefused(ctx, principal, "play", live.ErrNoCast, "castId", input.CastID)
	}
	server.logger.InfoContext(ctx, "cast: a cast plays another film", "castId", played.ID,
		logKeyUserID, played.UserID, "by", principal.DeviceID, "fileId", played.FileID)
	return &StartCastOutput{Body: castBodyOf(played, principal.DeviceID, credential)}, nil
}

// AcceptCastInput is the receiver's Use to a cast it was asked about.
type AcceptCastInput struct {
	AuthorizationHeader
	CastID int64 `minimum:"1" path:"id"`
}

// acceptCast notes the receiver's Use to a cast, which its sender is told on its live connection
// (D364 §2): from then on what the person plays goes to it.
func (server *Server) acceptCast(ctx context.Context, input *AcceptCastInput) (*StartCastOutput, error) {
	// A guest receiver takes its one cast as any receiver does; the registry has it accept that
	// cast alone, the only one handed to it (D465 §2).
	dependencies, principal, err := server.requireSignedInOrGuest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	accepted, err := dependencies.Live.Accept(principal.UserID, principal.DeviceID, input.CastID)
	if err != nil {
		return nil, server.castRefused(ctx, principal, "accept", err, "castId", input.CastID)
	}
	credential, err := dependencies.Together.CastCredential(accepted.ID, principal.UserID)
	if err != nil {
		return nil, server.castRefused(ctx, principal, "accept", live.ErrNoCast, "castId", input.CastID)
	}
	server.logger.InfoContext(ctx, "cast: a receiver took it", "castId", accepted.ID, logKeyUserID,
		accepted.UserID, "by", principal.DeviceID)
	return &StartCastOutput{Body: castBodyOf(accepted, principal.DeviceID, credential)}, nil
}

// castNames are the names of the asking device and of the one it casts to, which must be another of
// the person's; one that is not is answered as a device not connected.
func (server *Server) castNames(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, toDeviceID int64,
) (senderName, receiverName string, err error) {
	if toDeviceID == principal.DeviceID {
		return "", "", server.castRefused(ctx, principal, "start", live.ErrToItself, "to", toDeviceID)
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
		// A guest receiver is no device of theirs to list, and its grant's sender alone casts to it
		// (D465 §1); to any other it is not connected, as a device of someone else's is.
		guestName, isCastable := dependencies.Live.GuestCastableBy(principal.UserID, toDeviceID, principal.DeviceID)
		if !isCastable {
			return "", "", server.castRefused(ctx, principal, "start", live.ErrNotConnected, "to", toDeviceID)
		}
		receiverName = guestName
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
// answer says where it stood. A guest receiver ends its one cast as any receiver does, Stop casting
// here on its page (D469 §3); the registry has it end that cast alone, the only one it receives
// (D465 §2).
func (server *Server) endCast(ctx context.Context, input *EndCastInput) (*EndCastOutput, error) {
	dependencies, principal, err := server.requireSignedInOrGuest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	cast, err := dependencies.Live.End(principal.UserID, principal.DeviceID, input.CastID)
	if err != nil {
		return nil, server.castRefused(ctx, principal, "end", err, "castId", input.CastID)
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
		CastID: cast.ID, IsSender: cast.SenderDeviceID == thisDeviceID, IsAccepted: cast.IsAccepted,
		FileID: cast.FileID,
		ItemID: cast.ItemID, ItemName: cast.ItemName, DurationMs: cast.DurationMs,
		SenderDeviceID: cast.SenderDeviceID, SenderName: cast.SenderName,
		ReceiverDeviceID: cast.ReceiverDeviceID, ReceiverName: cast.ReceiverName, MembershipCredential: credential,
	}
}

// castRefused logs a cast request the registry refused, what was asked, by whom and why, at Info: a
// refusal is an answer, not a fault of the core's, but without it a Stop that did nothing cannot be
// told from one never sent (principles §28; the prod cast of 2026-11-03). It returns the answer,
// castError's.
func (server *Server) castRefused(
	ctx context.Context, principal auth.Principal, request string, err error, about ...any,
) error {
	attributes := []any{
		"request", request, logKeyUserID, principal.UserID, "by", principal.DeviceID, "why", err.Error(),
	}
	attributes = append(attributes, about...)
	server.logger.InfoContext(ctx, "cast: a request refused",
		append(attributes, slogx.EventKey, "castRequestRefused")...)
	return castError(err)
}

// castError is the answer to a cast the registry refused.
func castError(err error) error {
	switch {
	case errors.Is(err, live.ErrNotConnected), errors.Is(err, live.ErrNotInFront):
		return newError(http.StatusConflict, CodeConflict, err.Error())
	case errors.Is(err, live.ErrReceiving):
		return newError(http.StatusConflict, CodeReceivingACast, err.Error())
	case errors.Is(err, live.ErrCastingHere):
		return newError(http.StatusConflict, CodeCastingHere, err.Error())
	case errors.Is(err, live.ErrToItself):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.toDeviceId", err.Error())
	}
	return newError(http.StatusNotFound, CodeNotFound, err.Error())
}
