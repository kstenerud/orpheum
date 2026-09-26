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
		Account      AccountBody    `json:"account"`
		Registration InvitationBody `json:"registration"`
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

// DeviceCodeInput asks for a code that signs a device in (D218 §3): for oneself, or, for an
// admin, for anyone.
type DeviceCodeInput struct {
	AuthorizationHeader
	Body struct {
		DeviceName string `doc:"What the device will be listed as: \"Living room TV\"" json:"deviceName" maxLength:"100" minLength:"1"` //nolint:lll // one struct tag
		UserID     int64  `doc:"Whose device; an admin's to name, oneself otherwise"   json:"userId,omitempty"   required:"false"`      //nolint:lll // one struct tag
	}
}

// CodeSignInInput signs a device in with a device code (D218 §4).
type CodeSignInInput struct {
	Body struct {
		Code     string `doc:"As typed: case, dashes and spaces do not matter" json:"code"     minLength:"1"`
		Platform string `json:"platform" maxLength:"40" minLength:"1"`
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
	huma.Register(server.api, huma.Operation{
		OperationID: "listAccounts", Method: http.MethodGet, Path: "/users",
		Summary: "Everyone, for the admin", Tags: tags,
	}, server.listAccounts)
	huma.Register(server.api, huma.Operation{
		OperationID: "createAccount", Method: http.MethodPost, Path: "/users", DefaultStatus: http.StatusCreated,
		Summary: "Make a person, and a one-time link for them to set a password (D218)", Tags: tags,
	}, server.createAccount)
	huma.Register(server.api, huma.Operation{
		OperationID: "issueRegistrationLink", Method: http.MethodPost, Path: "/users/{id}/registration-link",
		Summary: "A new registration link for a person, withdrawing the old; their password works until they use it",
		Tags:    tags,
	}, server.issueRegistrationLink)
	huma.Register(server.api, huma.Operation{
		OperationID: "register", Method: http.MethodPost, Path: "/registration",
		Summary: "Use a registration link: set a password and sign this device in", Tags: tags,
	}, server.register)
	huma.Register(server.api, huma.Operation{
		OperationID: "issueDeviceCode", Method: http.MethodPost, Path: "/device-codes",
		Summary: "A code that signs a device in, for ten minutes, once (D218)", Tags: tags,
	}, server.issueDeviceCode)
	huma.Register(server.api, huma.Operation{
		OperationID: "signInWithCode", Method: http.MethodPost, Path: "/session/code",
		DefaultStatus: http.StatusCreated,
		Summary:       "Sign a device in with a device code", Tags: tags,
	}, server.signInWithCode)
	huma.Register(server.api, huma.Operation{
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
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	accounts, err := dependencies.Auth.Accounts(ctx)
	if err != nil {
		return nil, server.internal(ctx, "list accounts", err)
	}
	output := &AccountsOutput{}
	output.Body.Accounts = make([]AccountBody, 0, len(accounts))
	for _, account := range accounts {
		output.Body.Accounts = append(output.Body.Accounts, accountBody(account))
	}
	return output, nil
}

func (server *Server) createAccount(ctx context.Context, input *CreateAccountInput) (*CreateAccountOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	role := auth.Role(input.Body.Role)
	id, link, err := dependencies.Auth.CreateAccount(ctx, input.Body.Name, role)
	switch {
	case errors.Is(err, auth.ErrNameTaken):
		return nil, newError(http.StatusConflict, CodeConflict, "someone already has that name")
	case err != nil:
		return nil, server.internal(ctx, "create an account", err)
	}
	output := &CreateAccountOutput{}
	output.Body.Account = AccountBody{ID: id, Name: input.Body.Name, Role: string(role)}
	output.Body.Registration = invitationBody(link)
	return output, nil
}

func (server *Server) issueRegistrationLink(
	ctx context.Context, input *RegistrationLinkInput,
) (*InvitationOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	link, err := dependencies.Auth.IssueRegistrationLink(ctx, input.UserID)
	switch {
	case errors.Is(err, auth.ErrNoUser):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such person")
	case err != nil:
		return nil, server.internal(ctx, "make a registration link", err)
	}
	return &InvitationOutput{Body: invitationBody(link)}, nil
}

func (server *Server) register(ctx context.Context, input *RegisterInput) (*SessionOutput, error) {
	dependencies, err := server.ready()
	if err != nil {
		return nil, err
	}
	device := auth.NewDevice{Name: input.Body.Device.Name, Platform: input.Body.Device.Platform}
	token, err := dependencies.Auth.Register(ctx, input.Body.Secret, input.Body.Password, input.clientAddress, device)
	if err != nil {
		return nil, server.invitationRefusal(ctx, "register", err)
	}
	return server.sessionAnswer(ctx, dependencies, token, input.overTLS)
}

func (server *Server) issueDeviceCode(ctx context.Context, input *DeviceCodeInput) (*InvitationOutput, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
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
	code, err := dependencies.Auth.IssueDeviceCode(ctx, userID, input.Body.DeviceName)
	switch {
	case errors.Is(err, auth.ErrNoUser):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such person")
	case err != nil:
		return nil, server.internal(ctx, "make a device code", err)
	}
	return &InvitationOutput{Body: invitationBody(code)}, nil
}

func (server *Server) signInWithCode(ctx context.Context, input *CodeSignInInput) (*SessionOutput, error) {
	dependencies, err := server.ready()
	if err != nil {
		return nil, err
	}
	token, err := dependencies.Auth.SignInWithCode(ctx, input.Body.Code, input.Body.Platform, input.clientAddress)
	if err != nil {
		return nil, server.invitationRefusal(ctx, "sign in with a code", err)
	}
	return server.sessionAnswer(ctx, dependencies, token, input.overTLS)
}

// invitationRefusal is what a registration link or device code that does not work answers.
func (server *Server) invitationRefusal(ctx context.Context, doing string, err error) error {
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		return newError(http.StatusTooManyRequests, CodeRateLimited, "too many failed attempts; wait and try again")
	case errors.Is(err, auth.ErrBadInvitation):
		return newError(http.StatusUnauthorized, CodeAuthRequired, err.Error())
	}
	return server.internal(ctx, doing, err)
}

func (server *Server) changePassword(ctx context.Context, input *ChangePasswordInput) (*struct{}, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.ChangePassword(ctx, principal.UserID, input.Body.CurrentPassword, input.Body.NewPassword)
	switch {
	case errors.Is(err, auth.ErrBadCredentials):
		return nil, newError(http.StatusForbidden, CodeForbidden, "the current password is wrong")
	case err != nil:
		return nil, server.internal(ctx, "change a password", err)
	}
	return &struct{}{}, nil
}
