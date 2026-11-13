// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/library"
)

// InvitationBody is an invitation: its role and the admin's note of who it is for; while it waits,
// how many of its links still work and when the last ends; once used, the account it made (D311
// §3, §4; D318).
type InvitationBody struct {
	ID        int64  `json:"id"`
	Role      string `json:"role"`
	AdminNote string `json:"adminNote"`
	IssuedAt  int64  `doc:"Unix milliseconds" json:"issuedAt"`
	LiveLinks int    `doc:"Its links that still work" json:"liveLinks"`
	ExpiresAt int64  `doc:"When the last live link ends, Unix milliseconds; 0 with none" json:"expiresAt"`
	// Links are its live links while it waits, to pass on again until used, withdrawn or ended (D267
	// §5, as the owner chose for invitations, 2026-10-28).
	Links []IssuedSecretBody `doc:"Its live links, oldest first, to pass on again" json:"links"`
}

// InviteInput is the admin inviting someone to make their own account (D246 §2): the role it will
// have, and the note of who it is for, which the admin must write and only the admin sees.
type InviteInput struct {
	AuthorizationHeader
	Body struct {
		Role      string `doc:"The role's name, their whole access (D243 §4)" json:"role" maxLength:"100" minLength:"1"`
		AdminNote string `doc:"Who it is for: \"Uncle Joe\"; only the admin sees it" json:"adminNote" maxLength:"200" minLength:"1"` //nolint:lll // one struct tag
	}
}

// InviteOutput is the new invitation and its link's secret, which goes after # in the address, so
// no request carries it (D243 §3).
type InviteOutput struct {
	Body struct {
		Invitation InvitationBody   `json:"invitation"`
		Link       IssuedSecretBody `json:"link"`
	}
}

// InvitationsOutput is the invitations still waiting, newest first; a used one is not listed (D403).
type InvitationsOutput struct {
	Body struct {
		Invitations []InvitationBody `json:"invitations"`
		NextCursor  string           `doc:"The next page's cursor; absent on the last" json:"nextCursor,omitempty"`
	}
}

// InvitationsInput asks for a page of the invitations still waiting.
type InvitationsInput struct {
	AuthorizationHeader
	PageInput
}

// InvitationInput names an invitation.
type InvitationInput struct {
	AuthorizationHeader
	InvitationID int64 `minimum:"1" path:"id"`
}

// LinkKindInput asks what a link is for, by its secret, from after # in the address.
type LinkKindInput struct {
	Body struct {
		Secret string `json:"secret" minLength:"1"`
	}
}

// LinkKindOutput is what a link is for: an invitation, which asks for a name and a password, or a
// registration link, which asks for a new password for the person it names, a reset link when they
// already have one (D466). A link that no longer works is refused, whatever ended it.
type LinkKindOutput struct {
	Body struct {
		IsInvitation bool   `json:"isInvitation"`
		Name         string `doc:"A registration link's person; absent for an invitation" json:"name,omitempty"`
		IsReset      bool   `doc:"A registration link whose person has a password: a reset link (D466)" json:"isReset"`
	}
}

// AdminNoteInput changes the admin's note of who a person is for.
type AdminNoteInput struct {
	AuthorizationHeader
	UserID int64 `minimum:"1" path:"id"`
	Body   struct {
		AdminNote string `doc:"Empty removes it" json:"adminNote" maxLength:"200"`
	}
}

func (server *Server) registerInvitations() {
	tags := []string{tagUsers}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "createInvitation", Method: http.MethodPost, Path: "/invitations",
		DefaultStatus: http.StatusCreated,
		Summary:       "Invite someone to make their own account under a role, noting who it is for (D246)",
		Tags:          tags,
		Extensions:    callableBy(capabilityAdmin),
	}, server.createInvitation)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listInvitations", Method: http.MethodGet, Path: "/invitations",
		Summary: "Every invitation: those waiting, and those used with the account each made", Tags: tags,
		Extensions: callableBy(capabilityAdmin),
	}, server.listInvitations)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "withdrawInvitation", Method: http.MethodDelete, Path: "/invitations/{id}",
		DefaultStatus: http.StatusNoContent,
		Summary:       "Withdraw an invitation still waiting: none of its links admits anyone",
		Tags:          tags,
		Extensions:    callableBy(capabilityAdmin),
	}, server.withdrawInvitation)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getLinkKind", Method: http.MethodPost, Path: "/registration/link",
		Summary: "What a link is for, without using it: an invitation or a registration link", Tags: tags,
		Extensions: callableBy(capabilityNone),
	}, server.getLinkKind)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setAdminNote", Method: http.MethodPut, Path: "/users/{id}/admin-note",
		DefaultStatus: http.StatusNoContent, Summary: "Change the admin's note of who a person is for",
		Tags:       tags,
		Extensions: callableBy(capabilityAdmin),
	}, server.setAdminNote)
}

func invitationBody(invitation auth.Invitation, found *discovery.Service) InvitationBody {
	body := InvitationBody{
		ID: invitation.ID, Role: string(invitation.Role), AdminNote: invitation.AdminNote,
		IssuedAt: invitation.IssuedAt.UnixMilli(), LiveLinks: invitation.LiveLinks,
		Links: make([]IssuedSecretBody, 0, len(invitation.Links)),
	}
	for _, link := range invitation.Links {
		body.Links = append(body.Links, issuedSecretBody(link, found))
	}
	if !invitation.ExpiresAt.IsZero() {
		body.ExpiresAt = invitation.ExpiresAt.UnixMilli()
	}
	return body
}

func (server *Server) createInvitation(ctx context.Context, input *InviteInput) (*InviteOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	id, link, err := dependencies.Auth.Invite(ctx, auth.NewInvitation{
		Role: auth.Role(input.Body.Role), AdminNote: input.Body.AdminNote,
	})
	if err != nil {
		return nil, server.authError(ctx, "invite someone", err)
	}
	// The new invitation is the newest: the first of the first page.
	listed, _, err := server.invitationBodies(ctx, dependencies, auth.InvitationsFirst(), 1)
	if err != nil {
		return nil, err
	}
	output := &InviteOutput{}
	if index := slices.IndexFunc(listed, func(each InvitationBody) bool { return each.ID == id }); index >= 0 {
		output.Body.Invitation = listed[index]
	}
	output.Body.Link = issuedSecretBody(link, dependencies.Discovery)
	return output, nil
}

func (server *Server) listInvitations(
	ctx context.Context, input *InvitationsInput,
) (*InvitationsOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	// The cursor is the last invitation's issue time, Unix milliseconds, and its id.
	before := auth.InvitationsFirst()
	if input.Cursor != "" {
		key, id, cursorErr := decodeCursor(input.Cursor)
		if cursorErr != nil {
			return nil, cursorErr
		}
		issuedAt, isTime := library.KeyInt64(key)
		if !isTime || id < 1 {
			return nil, errBadCursor()
		}
		before = auth.InvitationPlace{IssuedAt: issuedAt, ID: id}
	}
	output := &InvitationsOutput{}
	body := &output.Body
	body.Invitations, body.NextCursor, err = server.invitationBodies(ctx, dependencies, before, input.Limit)
	return output, err
}

// invitationBodies are a page of the invitations, as the API shows them, with the next page's cursor.
func (server *Server) invitationBodies(
	ctx context.Context, dependencies *Dependencies, before auth.InvitationPlace, limit int64,
) ([]InvitationBody, string, error) {
	invitations, err := dependencies.Auth.InvitationsBefore(ctx, before, limit)
	if err != nil {
		return nil, "", server.authError(ctx, "list invitations", err)
	}
	bodies := make([]InvitationBody, 0, len(invitations))
	for _, invitation := range invitations {
		bodies = append(bodies, invitationBody(invitation, dependencies.Discovery))
	}
	next := nextCursorOf(len(invitations), limit, func() string {
		last := invitations[len(invitations)-1]
		return library.EncodeCursor(library.Int64Key(last.IssuedAt.UnixMilli()), last.ID)
	})
	return bodies, next, nil
}

func (server *Server) withdrawInvitation(ctx context.Context, input *InvitationInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.WithdrawInvitation(ctx, input.InvitationID); err != nil {
		return nil, server.authError(ctx, "withdraw an invitation", err)
	}
	return &struct{}{}, nil
}

func (server *Server) getLinkKind(ctx context.Context, input *LinkKindInput) (*LinkKindOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	kind, err := dependencies.Auth.KindOfLink(ctx, input.Body.Secret)
	if err != nil {
		return nil, server.authError(ctx, "read a link", err)
	}
	output := &LinkKindOutput{}
	output.Body.IsInvitation, output.Body.Name, output.Body.IsReset = kind.IsInvitation, kind.Name, kind.IsReset
	return output, nil
}

func (server *Server) setAdminNote(ctx context.Context, input *AdminNoteInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.SetAdminNote(ctx, input.UserID, input.Body.AdminNote); err != nil {
		return nil, server.authError(ctx, "change a note", err)
	}
	return &struct{}{}, nil
}
