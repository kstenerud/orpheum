// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

const tagAccounts = "Accounts"

// AccountBody is one person, as the admin's list shows them.
type AccountBody struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Role        string `enum:"admin,user" json:"role"`
	IsOutsider  bool   `json:"isOutsider"`
	HasPassword bool   `doc:"False until they register from their link" json:"hasPassword"`
}

// InvitationBody is a registration link's secret or a device code, and when it stops working.
// A registration link's secret goes after # in the address, so no request carries it (D218 §1).
type InvitationBody struct {
	Secret    string `json:"secret"`
	ExpiresAt int64  `doc:"Unix milliseconds" json:"expiresAt"`
}

func invitationBody(invitation auth.Invitation) InvitationBody {
	return InvitationBody{Secret: invitation.Secret, ExpiresAt: invitation.ExpiresAt.UnixMilli()}
}

// AccountsOutput is everyone.
type AccountsOutput struct {
	Body struct {
		Accounts []AccountBody `json:"accounts"`
	}
}

// CreateAccountInput is the admin making a person (D218 §1).
type CreateAccountInput struct {
	AuthorizationHeader
	Body struct {
		Name string `json:"name" maxLength:"100" minLength:"1"`
		Role string `enum:"admin,user" json:"role"`
	}
}

// CreateAccountOutput is the new person and their registration link.
type CreateAccountOutput struct {
	Body struct {
		Account                AccountBody    `json:"account"`
		RegistrationInvitation InvitationBody `json:"registrationInvitation"`
	}
}

// RegistrationLinkInput asks for a new registration link for a person.
type RegistrationLinkInput struct {
	AuthorizationHeader
	UserID int64 `minimum:"1" path:"id"`
}

// InvitationOutput is one registration link or device code.
type InvitationOutput struct {
	Body InvitationBody
}

// RegisterInput uses a registration link: its secret, the password the person chose, and the
// device they are on (D218 §2).
type RegisterInput struct {
	Body struct {
		Secret   string        `json:"secret"   minLength:"1"`
		Password string        `json:"password" minLength:"1"`
		Device   NewDeviceBody `json:"device"`
	}
	ClientFacts
}

// DeviceCodeInput asks for a code that signs a device in (D218 §3, D221 §2): a new device,
// named, or one already listed, by id; for oneself, or, for an admin, for anyone.
type DeviceCodeInput struct {
	AuthorizationHeader
	Body struct {
		DeviceName string `doc:"A new device's name: \"Living room TV\"" json:"deviceName,omitempty" maxLength:"100" required:"false"` //nolint:lll // one struct tag
		DeviceID   int64  `doc:"A listed device, to sign back in" json:"deviceId,omitempty" required:"false"`
		UserID     int64  `doc:"Whose device; an admin's to name" json:"userId,omitempty" required:"false"`
	}
}

// CodeSignInInput signs a device in with a device code (D218 §4).
type CodeSignInInput struct {
	Body struct {
		Code             string `doc:"As typed: case, dashes and spaces do not matter" json:"code"     minLength:"1"`
		Platform         string `json:"platform" maxLength:"40" minLength:"1"`
		PreviousDeviceID int64  `doc:"The device this client was signed in as before (D221)" json:"previousDeviceId,omitempty" required:"false"` //nolint:lll // one struct tag
	}
	ClientFacts
}

// ChangePasswordInput changes one's own password (D218 §6).
type ChangePasswordInput struct {
	AuthorizationHeader
	Body struct {
		CurrentPassword string `json:"currentPassword" minLength:"1"`
		NewPassword     string `json:"newPassword"     minLength:"1"`
	}
}

func (server *Server) registerAccounts() {
	tags := []string{tagAccounts}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listAccounts", Method: http.MethodGet, Path: "/users",
		Summary: "Everyone, for the admin", Tags: tags,
	}, server.listAccounts)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "createAccount", Method: http.MethodPost, Path: "/users", DefaultStatus: http.StatusCreated,
		Summary: "Make a person, and a one-time link for them to set a password (D218)", Tags: tags,
	}, server.createAccount)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "issueRegistrationLink", Method: http.MethodPost, Path: "/users/{id}/registration-link",
		Summary: "A new registration link for a person, withdrawing the old; their password works until they use it",
		Tags:    tags,
	}, server.issueRegistrationLink)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "register", Method: http.MethodPost, Path: "/registration",
		Summary: "Use a registration link: set a password and sign this device in", Tags: tags,
	}, server.register)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "issueDeviceCode", Method: http.MethodPost, Path: "/device-codes",
		Summary: "A code that signs a device in, for ten minutes, once (D218)", Tags: tags,
	}, server.issueDeviceCode)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "signInWithCode", Method: http.MethodPost, Path: "/session/code",
		DefaultStatus: http.StatusCreated,
		Summary:       "Sign a device in with a device code", Tags: tags,
	}, server.signInWithCode)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "changePassword", Method: http.MethodPut, Path: "/me/password",
		DefaultStatus: http.StatusNoContent,
		Summary:       "Change one's own password; other sessions stay signed in", Tags: tags,
	}, server.changePassword)
}

func accountBody(account auth.Account) AccountBody {
	return AccountBody{
		ID: account.ID, Name: account.Name, Role: string(account.Role), IsOutsider: account.IsOutsider,
		HasPassword: account.HasPassword,
	}
}

func (server *Server) listAccounts(ctx context.Context, input *struct{ AuthorizationHeader }) (*AccountsOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	accounts, err := dependencies.Auth.Accounts(ctx)
	if err != nil {
		return nil, server.logInternalError(ctx, "list accounts", err)
	}
	output := &AccountsOutput{}
	output.Body.Accounts = make([]AccountBody, 0, len(accounts))
	for _, account := range accounts {
		output.Body.Accounts = append(output.Body.Accounts, accountBody(account))
	}
	return output, nil
}

func (server *Server) createAccount(ctx context.Context, input *CreateAccountInput) (*CreateAccountOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	role := auth.Role(input.Body.Role)
	id, link, err := dependencies.Auth.CreateAccount(ctx, input.Body.Name, role)
	switch {
	case errors.Is(err, auth.ErrNameTaken):
		return nil, newError(http.StatusConflict, CodeConflict, "someone already has that name")
	case err != nil:
		return nil, server.logInternalError(ctx, "create an account", err)
	}
	output := &CreateAccountOutput{}
	output.Body.Account = AccountBody{ID: id, Name: input.Body.Name, Role: string(role)}
	output.Body.RegistrationInvitation = invitationBody(link)
	return output, nil
}

func (server *Server) issueRegistrationLink(
	ctx context.Context, input *RegistrationLinkInput,
) (*InvitationOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	link, err := dependencies.Auth.IssueRegistrationLink(ctx, input.UserID)
	switch {
	case errors.Is(err, auth.ErrNoUser):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such person")
	case err != nil:
		return nil, server.logInternalError(ctx, "make a registration link", err)
	}
	return &InvitationOutput{Body: invitationBody(link)}, nil
}

func (server *Server) register(ctx context.Context, input *RegisterInput) (*SessionOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	token, err := dependencies.Auth.Register(ctx, input.Body.Secret, input.Body.Password, newDevice(input.Body.Device))
	if err != nil {
		return nil, server.invitationRefusal(ctx, "register", err)
	}
	return server.newSessionOutput(ctx, dependencies, token, input.isOverTLS)
}

func (server *Server) issueDeviceCode(ctx context.Context, input *DeviceCodeInput) (*InvitationOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	userID := principal.UserID
	if input.Body.UserID != 0 && input.Body.UserID != userID {
		if !principal.IsAdmin() {
			return nil, newError(http.StatusForbidden, CodeForbidden,
				"only an administrator may add a device for someone else")
		}
		userID = input.Body.UserID
	}
	if (input.Body.DeviceName == "") == (input.Body.DeviceID == 0) {
		return nil, newError(http.StatusBadRequest, CodeInvalidRequest,
			"a code is for a new device, named, or a listed one")
	}
	deviceCode, err := dependencies.Auth.IssueDeviceCode(ctx, userID, input.Body.DeviceName, input.Body.DeviceID)
	switch {
	case errors.Is(err, auth.ErrNoUser):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such person")
	case errors.Is(err, auth.ErrNoDevice):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such device")
	case err != nil:
		return nil, server.logInternalError(ctx, "make a device code", err)
	}
	return &InvitationOutput{Body: invitationBody(deviceCode)}, nil
}

func (server *Server) signInWithCode(ctx context.Context, input *CodeSignInInput) (*SessionOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	token, err := dependencies.Auth.SignInWithCode(
		ctx, input.Body.Code, input.Body.Platform, input.clientAddress, input.Body.PreviousDeviceID)
	if err != nil {
		return nil, server.invitationRefusal(ctx, "sign in with a code", err)
	}
	return server.newSessionOutput(ctx, dependencies, token, input.isOverTLS)
}

// invitationRefusal is what a registration link or device code that does not work answers.
func (server *Server) invitationRefusal(ctx context.Context, action string, err error) error {
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		return newError(http.StatusTooManyRequests, CodeRateLimited, "too many failed attempts; wait and try again")
	case errors.Is(err, auth.ErrBadInvitation):
		return newError(http.StatusUnauthorized, CodeAuthRequired, err.Error())
	}
	return server.logInternalError(ctx, action, err)
}

func (server *Server) changePassword(ctx context.Context, input *ChangePasswordInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.ChangePassword(ctx, principal.UserID, input.Body.CurrentPassword, input.Body.NewPassword)
	switch {
	case errors.Is(err, auth.ErrBadCredentials):
		return nil, newError(http.StatusForbidden, CodeForbidden, "the current password is wrong")
	case err != nil:
		return nil, server.logInternalError(ctx, "change a password", err)
	}
	return &struct{}{}, nil
}

// RenamePersonInput renames a person, for the admin.
type RenamePersonInput struct {
	AuthorizationHeader
	UserID int64 `minimum:"1" path:"id"`
	Body   struct {
		Name string `json:"name" maxLength:"100" minLength:"1"`
	}
}

// PersonOutput is one person's page, for the admin: who they are, and what they have watched.
type PersonOutput struct {
	Body struct {
		Account    AccountBody    `json:"account"`
		WatchStats WatchStatsBody `json:"watchStats"`
	}
}

// WatchStatsBody is what a person has watched.
type WatchStatsBody struct {
	WatchedMs          int64 `json:"watchedMs"`
	TitlesWatchedCount int64 `json:"titlesWatchedCount"`
	LastWatchedAt      int64 `doc:"Unix milliseconds; absent when nothing was watched" json:"lastWatchedAt,omitempty"`
}

func (server *Server) registerPerson() {
	tags := []string{tagAccounts}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getPerson", Method: http.MethodGet, Path: "/users/{id}",
		Summary: "A person's page, for the admin: who they are and what they have watched", Tags: tags,
	}, server.getPerson)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "renamePerson", Method: http.MethodPut, Path: "/users/{id}",
		Summary: "Rename a person; names are for display and sign-in, and stay unique", Tags: tags,
		DefaultStatus: http.StatusNoContent,
	}, server.renamePerson)
}

func (server *Server) getPerson(ctx context.Context, input *PersonInput) (*PersonOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	accounts, err := dependencies.Auth.Accounts(ctx)
	if err != nil {
		return nil, server.logInternalError(ctx, "list accounts", err)
	}
	output := &PersonOutput{}
	for _, account := range accounts {
		if account.ID == input.UserID {
			output.Body.Account = accountBody(account)
		}
	}
	if output.Body.Account.ID == 0 {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such person")
	}
	stats, err := dependencies.Playback.WatchStatsOf(ctx, input.UserID)
	if err != nil {
		return nil, server.logInternalError(ctx, "read a person's watching", err)
	}
	output.Body.WatchStats = WatchStatsBody{WatchedMs: stats.WatchedMs, TitlesWatchedCount: stats.TitlesWatched}
	if !stats.LastWatchedAt.IsZero() {
		output.Body.WatchStats.LastWatchedAt = stats.LastWatchedAt.UnixMilli()
	}
	return output, nil
}

func (server *Server) renamePerson(ctx context.Context, input *RenamePersonInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.RenameUser(ctx, input.UserID, input.Body.Name)
	switch {
	case errors.Is(err, auth.ErrNoUser):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such person")
	case errors.Is(err, auth.ErrNameTaken):
		return nil, newError(http.StatusConflict, CodeConflict, "someone already has that name")
	case err != nil:
		return nil, server.logInternalError(ctx, "rename a person", err)
	}
	return &struct{}{}, nil
}
