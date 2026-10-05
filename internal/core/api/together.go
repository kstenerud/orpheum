// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// tagTogether groups Watch Together's operations.
const tagTogether = "Watch Together"

// OpenTogetherOutput is the host's session: started now, nothing playing, or the one they already
// host; and a membership credential their devices say hello with (D246 §5).
type OpenTogetherOutput struct {
	Body struct {
		SessionID            int64  `json:"sessionId"`
		MembershipCredential string `json:"membershipCredential"`
		IsNew                bool   `doc:"Started now, rather than one the host already had" json:"isNew"`
	}
}

// TogetherInput names a session its host acts on.
type TogetherInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
}

// TogetherLinkBody is one person's link, as the host sees it.
type TogetherLinkBody struct {
	ID        int64  `json:"id"`
	AdminNote string `doc:"Who it is for; only the host sees it" json:"adminNote"`
	// UsedByMemberID is the member who joined by it; absent while it waits.
	UsedByMemberID int64 `json:"usedByMemberId,omitempty"`
}

// TogetherMemberBody is one member, as the host sees them.
type TogetherMemberBody struct {
	ID        int64  `json:"id"`
	Name      string `doc:"The name they chose, which everyone sees" json:"name"`
	AdminNote string `doc:"The note of the link they joined by; only the host sees it" json:"adminNote,omitempty"`
	IsHost    bool   `json:"isHost"`
}

// TogetherDetailsOutput is a session as its host sees it: its links and members, with the host's
// notes, and what plays.
type TogetherDetailsOutput struct {
	Body struct {
		SessionID int64                `json:"sessionId"`
		Links     []TogetherLinkBody   `json:"links"`
		Members   []TogetherMemberBody `json:"members"`
		FileID    int64                `doc:"What plays; absent while nothing does" json:"fileId,omitempty"`
	}
}

// TogetherLinkInput makes one person's link, with the host's note of who it is for (D246 §3).
type TogetherLinkInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
	Body      struct {
		AdminNote string `doc:"Who it is for: \"Grandma\"; only the host sees it" json:"adminNote" maxLength:"200" minLength:"1"` //nolint:lll // one struct tag
	}
}

// TogetherLinkOutput is the new link: its id, and its secret, which goes after # in the link
// (D243 §3).
type TogetherLinkOutput struct {
	Body struct {
		LinkID int64  `json:"linkId"`
		Secret string `json:"secret"`
	}
}

// RemoveTogetherLinkInput names a link the host withdraws.
type RemoveTogetherLinkInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
	LinkID    int64 `minimum:"1" path:"link"`
}

// JoinTogetherInput joins a session by a link's secret, read from after # (D243 §3), with the name
// the others will see (D246 §3).
type JoinTogetherInput struct {
	Body struct {
		Secret string `json:"secret" maxLength:"100" minLength:"1"`
		Name   string `doc:"How the others see you" json:"name" maxLength:"100" minLength:"1"`
	}
}

// JoinTogetherOutput is the membership: its credential, which every device of the person says hello
// with, and which their player plays by.
type JoinTogetherOutput struct {
	Body struct {
		SessionID            int64  `json:"sessionId"`
		MemberID             int64  `json:"memberId"`
		MembershipCredential string `json:"membershipCredential"`
	}
}

// EjectInput names a member the host removes.
type EjectInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
	MemberID  int64 `minimum:"1" path:"member"`
}

// ViewerPlaybackInput starts a viewer's playback of what the session plays now: the membership
// credential in the Authorization header, never in the URL (D154), and how this device plays, as
// POST /playback is told, with the speaker setup the viewer chose as they joined (D246 §6).
type ViewerPlaybackInput struct {
	AuthorizationHeader
	Body struct {
		PlayableFormats PlayableFormatsBody `json:"playableFormats"`
		Network         string              `doc:"Where the client is" enum:"local,remote" json:"network"`
		AudioOutput     AudioOutputKind     `doc:"The viewer's speaker setup" json:"audioOutput,omitempty" required:"false"` //nolint:lll // one struct tag
	}
	ClientFacts
}

// ViewerPlaybackOutput is the decision, as POST /playback answers, and the viewer cookie, which
// carries the membership to the byte paths for the player (D246 §3).
type ViewerPlaybackOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      StartPlaybackOutputBody
}

// LeaveTogetherInput is a viewer leaving: their membership credential in the Authorization header.
type LeaveTogetherInput struct {
	AuthorizationHeader
}

func (server *Server) registerTogether() {
	tags := []string{tagTogether}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "openTogether", Method: http.MethodPost, Path: "/together", Tags: tags,
		Summary: "Start a Watch Together session, nothing playing, or have back the one you host (D246 §5)",
	}, server.openTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getTogether", Method: http.MethodGet, Path: "/together/{id}", Tags: tags,
		Summary: "The session's links and members, with your notes (host)",
	}, server.getTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "addTogetherLink", Method: http.MethodPost, Path: "/together/{id}/links", Tags: tags,
		DefaultStatus: http.StatusCreated,
		Summary:       "A link for one person to join by, once, with a note of who it is for (host; D246 §3)",
	}, server.addTogetherLink)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "removeTogetherLink", Method: http.MethodDelete, Path: "/together/{id}/links/{link}",
		Tags: tags, DefaultStatus: http.StatusNoContent, Summary: "Withdraw a link not yet used (host)",
	}, server.removeTogetherLink)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "joinTogether", Method: http.MethodPost, Path: "/together/join", Tags: tags,
		Summary: "Join a session by your link, with the name the others see; no account needed (D246 §3)",
	}, server.joinTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "playTogether", Method: http.MethodPost, Path: "/together/playback", Tags: tags,
		Summary: "A viewer's playback of what the session plays now (D246 §3)",
	}, server.playTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "leaveTogether", Method: http.MethodPost, Path: "/together/leave", Tags: tags,
		DefaultStatus: http.StatusNoContent, Summary: "Leave the session you joined (a viewer)",
	}, server.leaveTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "ejectFromTogether", Method: http.MethodPost, Path: "/together/{id}/eject/{member}",
		Tags: tags, DefaultStatus: http.StatusNoContent,
		Summary: "Remove a member; their devices are told and their credential admits nothing (host; D119)",
	}, server.ejectFromTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "endTogether", Method: http.MethodDelete, Path: "/together/{id}", Tags: tags,
		DefaultStatus: http.StatusNoContent, Summary: "End the session for everyone (host)",
	}, server.endTogether)
}

func (server *Server) openTogether(ctx context.Context, input *AuthorizationHeader) (*OpenTogetherOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if !principal.CanHostWatchTogether() {
		return nil, newError(http.StatusForbidden, CodeForbidden, "your role may not host Watch Together")
	}
	opened, err := dependencies.Together.Open(together.Person{UserID: principal.UserID, Name: principal.Name})
	if err != nil {
		return nil, server.togetherError(ctx, "start a session", err)
	}
	output := &OpenTogetherOutput{}
	output.Body.SessionID, output.Body.MembershipCredential = opened.SessionID, opened.MembershipCredential
	output.Body.IsNew = opened.IsNew
	return output, nil
}

func (server *Server) getTogether(ctx context.Context, input *TogetherInput) (*TogetherDetailsOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	details, err := dependencies.Together.DetailsOf(input.SessionID, principal.UserID)
	if err != nil {
		return nil, server.togetherError(ctx, "read a session", err)
	}
	output := &TogetherDetailsOutput{}
	output.Body.SessionID, output.Body.FileID = details.SessionID, details.Group.FileID
	output.Body.Links = make([]TogetherLinkBody, 0, len(details.Links))
	for _, each := range details.Links {
		output.Body.Links = append(output.Body.Links, TogetherLinkBody{
			ID: each.ID, AdminNote: each.AdminNote, UsedByMemberID: each.UsedBy,
		})
	}
	output.Body.Members = make([]TogetherMemberBody, 0, len(details.Members))
	for _, each := range details.Members {
		output.Body.Members = append(output.Body.Members, TogetherMemberBody{
			ID: each.ID, Name: each.Name, AdminNote: each.AdminNote, IsHost: each.IsHost,
		})
	}
	return output, nil
}

func (server *Server) addTogetherLink(ctx context.Context, input *TogetherLinkInput) (*TogetherLinkOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	made, err := dependencies.Together.AddLink(input.SessionID, principal.UserID, textindex.Line(input.Body.AdminNote))
	if err != nil {
		return nil, server.togetherError(ctx, "make a link", err)
	}
	output := &TogetherLinkOutput{}
	output.Body.LinkID, output.Body.Secret = made.LinkID, made.Secret
	return output, nil
}

func (server *Server) removeTogetherLink(ctx context.Context, input *RemoveTogetherLinkInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Together.RemoveLink(input.SessionID, principal.UserID, input.LinkID); err != nil {
		return nil, server.togetherError(ctx, "withdraw a link", err)
	}
	return &struct{}{}, nil
}

func (server *Server) joinTogether(ctx context.Context, input *JoinTogetherInput) (*JoinTogetherOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	name := textindex.Line(input.Body.Name)
	if name == "" {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.name", "a name is needed")
	}
	joined, err := dependencies.Together.Join(input.Body.Secret, name)
	if err != nil {
		return nil, server.togetherError(ctx, "join a session", err)
	}
	output := &JoinTogetherOutput{}
	output.Body.SessionID, output.Body.MemberID = joined.SessionID, joined.MemberID
	output.Body.MembershipCredential = joined.MembershipCredential
	return output, nil
}

// playTogether starts a viewer's playback of the session's film, in the host's libraries: a viewer
// plays nothing else, and nothing once the host no longer reaches it (D246 §3, D243 §4).
func (server *Server) playTogether(ctx context.Context, input *ViewerPlaybackInput) (*ViewerPlaybackOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	credential := bearerToken(input.Authorization)
	holder, isViewer := server.viewerHolder(ctx, dependencies, credential)
	if !isViewer {
		return nil, newError(http.StatusNotFound, CodeNotFound, "nothing plays for you in a session now")
	}
	started, err := dependencies.Playback.Start(ctx, holder, playback.Request{
		FileID: holder.OnlyFileID, PlayableFormats: playableFormatsOf(input.Body.PlayableFormats),
		Network: decide.Network(input.Body.Network), AudioOutput: string(input.Body.AudioOutput),
	})
	if unreadable, isUnreadable := errors.AsType[*library.UnreadableError](err); isUnreadable {
		return nil, mediaUnreachable(unreadable, server.nodeNamer(ctx, dependencies))
	}
	if err != nil {
		return nil, server.playbackError(ctx, "start a viewer's playback", err)
	}
	return &ViewerPlaybackOutput{
		SetCookie: viewerCookie(credential, input.isOverTLS), Body: startedOutput(started).Body,
	}, nil
}

func (server *Server) leaveTogether(ctx context.Context, input *LeaveTogetherInput) (*struct{}, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	if err = dependencies.Together.Leave(bearerToken(input.Authorization)); err != nil {
		return nil, server.togetherError(ctx, "leave a session", err)
	}
	return &struct{}{}, nil
}

func (server *Server) ejectFromTogether(ctx context.Context, input *EjectInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Together.Eject(input.SessionID, principal.UserID, input.MemberID); err != nil {
		return nil, server.togetherError(ctx, "eject a member", err)
	}
	return &struct{}{}, nil
}

func (server *Server) endTogether(ctx context.Context, input *TogetherInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Together.End(input.SessionID, principal.UserID); err != nil {
		return nil, server.togetherError(ctx, "end a session", err)
	}
	return &struct{}{}, nil
}

// togetherError maps Watch Together's refusals to the API's errors.
func (server *Server) togetherError(ctx context.Context, action string, err error) error {
	switch {
	case errors.Is(err, together.ErrNoSession), errors.Is(err, together.ErrNoMember),
		errors.Is(err, together.ErrNoLink):
		return newError(http.StatusNotFound, CodeNotFound, err.Error())
	case errors.Is(err, together.ErrNotHost):
		return newError(http.StatusForbidden, CodeForbidden, err.Error())
	case errors.Is(err, together.ErrNoNote):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.adminNote", err.Error())
	}
	return server.logInternalError(ctx, action, err)
}
