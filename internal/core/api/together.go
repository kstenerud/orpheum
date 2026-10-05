// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/together"
)

// tagTogether groups Watch Together's operations.
const tagTogether = "Watch Together"

// CreateTogetherInput starts a session: what it plays, who may play, pause and seek, and how many
// people it holds (D121, D119).
type CreateTogetherInput struct {
	AuthorizationHeader
	Body struct {
		FileID    int64              `json:"fileId" minimum:"1"`
		Transport together.Transport `enum:"group,host" json:"transport"`
		PeopleCap int                `doc:"How many people it holds; two when absent (D243 §9)" json:"peopleCap,omitempty" maximum:"32" minimum:"0"` //nolint:lll // one struct tag
	}
}

// CreateTogetherOutput is the new session: its id, its join link's secret, which goes after # in
// the link (D243 §3), and the host's membership credential, which their devices say hello with.
type CreateTogetherOutput struct {
	Body struct {
		SessionID            int64  `json:"sessionId"`
		JoinSecret           string `json:"joinSecret"`
		MembershipCredential string `json:"membershipCredential"`
	}
}

// JoinTogetherInput joins a session by its link's secret, read from after # (D243 §3).
type JoinTogetherInput struct {
	Body struct {
		Secret string `json:"secret" maxLength:"100" minLength:"1"`
		Name   string `doc:"How the others see you" json:"name" maxLength:"100" minLength:"1"`
	}
}

// JoinTogetherOutput is the membership: its credential, which every device of the person says hello
// with.
type JoinTogetherOutput struct {
	Body struct {
		SessionID            int64  `json:"sessionId"`
		MemberID             int64  `json:"memberId"`
		MembershipCredential string `json:"membershipCredential"`
	}
}

// TogetherInput names a session its host acts on.
type TogetherInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
}

// RotateTogetherOutput is the session's new join link's secret.
type RotateTogetherOutput struct {
	Body struct {
		JoinSecret string `json:"joinSecret"`
	}
}

// EjectInput names a member the host removes.
type EjectInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
	MemberID  int64 `minimum:"1" path:"member"`
}

// PeopleCapInput changes how many people the session holds.
type PeopleCapInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
	Body      struct {
		PeopleCap int `json:"peopleCap" maximum:"32" minimum:"1"`
	}
}

func (server *Server) registerTogether() {
	tags := []string{tagTogether}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "createTogether", Method: http.MethodPost, Path: "/together", Tags: tags,
		DefaultStatus: http.StatusCreated,
		Summary:       "Start a Watch Together session of a film the host may play (D243 §1, §3)",
	}, server.createTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "joinTogether", Method: http.MethodPost, Path: "/together/join", Tags: tags,
		Summary: "Join a session by its link's secret; no account needed (D117, D119)",
	}, server.joinTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "rotateTogether", Method: http.MethodPost, Path: "/together/{id}/rotate", Tags: tags,
		Summary: "Give the session a new link; everyone already in stays (host; D119)",
	}, server.rotateTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "ejectFromTogether", Method: http.MethodPost, Path: "/together/{id}/eject/{member}",
		Tags: tags, DefaultStatus: http.StatusNoContent,
		Summary: "Remove a member; their devices are told and their credential admits nothing (host; D119)",
	}, server.ejectFromTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setTogetherCap", Method: http.MethodPut, Path: "/together/{id}/cap", Tags: tags,
		DefaultStatus: http.StatusNoContent, Summary: "Change how many people the session holds (host; D119)",
	}, server.setTogetherCap)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "endTogether", Method: http.MethodDelete, Path: "/together/{id}", Tags: tags,
		DefaultStatus: http.StatusNoContent, Summary: "End the session for everyone (host)",
	}, server.endTogether)
}

func (server *Server) createTogether(ctx context.Context, input *CreateTogetherInput) (*CreateTogetherOutput, error) {
	dependencies, principal, err := server.requireSignedInReachingFile(ctx, input.Authorization, input.Body.FileID)
	if err != nil {
		return nil, err
	}
	if !principal.CanHostWatchTogether() {
		return nil, newError(http.StatusForbidden, CodeForbidden, "your role may not host Watch Together")
	}
	created, err := dependencies.Together.Create(together.Person{UserID: principal.UserID, Name: principal.Name},
		input.Body.FileID, together.Abilities{Transport: input.Body.Transport}, input.Body.PeopleCap)
	if err != nil {
		return nil, server.togetherError(ctx, "start a session", err)
	}
	output := &CreateTogetherOutput{}
	output.Body.SessionID, output.Body.JoinSecret = created.SessionID, created.JoinSecret
	output.Body.MembershipCredential = created.MembershipCredential
	return output, nil
}

func (server *Server) joinTogether(ctx context.Context, input *JoinTogetherInput) (*JoinTogetherOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	joined, err := dependencies.Together.Join(input.Body.Secret, input.Body.Name)
	if err != nil {
		return nil, server.togetherError(ctx, "join a session", err)
	}
	output := &JoinTogetherOutput{}
	output.Body.SessionID, output.Body.MemberID = joined.SessionID, joined.MemberID
	output.Body.MembershipCredential = joined.MembershipCredential
	return output, nil
}

func (server *Server) rotateTogether(ctx context.Context, input *TogetherInput) (*RotateTogetherOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	secret, err := dependencies.Together.Rotate(input.SessionID, principal.UserID)
	if err != nil {
		return nil, server.togetherError(ctx, "rotate a session's link", err)
	}
	output := &RotateTogetherOutput{}
	output.Body.JoinSecret = secret
	return output, nil
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

func (server *Server) setTogetherCap(ctx context.Context, input *PeopleCapInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Together.SetCap(input.SessionID, principal.UserID, input.Body.PeopleCap); err != nil {
		return nil, server.togetherError(ctx, "change a session's cap", err)
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
	case errors.Is(err, together.ErrNoSession), errors.Is(err, together.ErrNoMember):
		return newError(http.StatusNotFound, CodeNotFound, err.Error())
	case errors.Is(err, together.ErrFull):
		return newError(http.StatusConflict, CodeConflict, err.Error())
	case errors.Is(err, together.ErrNotHost):
		return newError(http.StatusForbidden, CodeForbidden, err.Error())
	}
	return server.logInternalError(ctx, action, err)
}
