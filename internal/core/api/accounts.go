// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

const tagUsers = "Users"

// UserEntryBody is one user, as the admin's list and their page show them (D416 §1).
type UserEntryBody struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role string `doc:"The role's name, their whole access" json:"role"`
	// AdminNote is the admin's note of who the person is for, which only the admin sees (D246 §2).
	AdminNote   string `json:"adminNote,omitempty"`
	HasPassword bool   `doc:"False for an account from before D246 that never registered" json:"hasPassword"`
}

// IssuedSecretBody is a registration invitation's secret, and when it stops working.
// A registration invitation's secret goes after # in the address, so no request carries it (D218 §1).
type IssuedSecretBody struct {
	Secret    string `json:"secret"`
	ExpiresAt int64  `doc:"Unix milliseconds" json:"expiresAt"`
	Link      string `doc:"The link to share, at the discovery that claimed the server; absent when none has, and the client makes it on its own origin" json:"link,omitempty"` //nolint:lll // one struct tag
}

// issuedSecretBody is a registration link's secret, and the link to share while a discovery has
// claimed the core (discoveryLink).
func issuedSecretBody(issued auth.IssuedSecret, found *discovery.Service) IssuedSecretBody {
	body := IssuedSecretBody{Secret: issued.Secret, ExpiresAt: issued.ExpiresAt.UnixMilli()}
	if found != nil {
		body.Link = discoveryLink(found.Status(), "/register", issued.Secret)
	}
	return body
}

// togetherLink is a Watch Together link in the discovery's form while one has claimed the core
// (discoveryLink), else empty.
func togetherLink(dependencies *Dependencies, secret string) string {
	if dependencies.Discovery == nil {
		return ""
	}
	return discoveryLink(dependencies.Discovery.Status(), "/together", secret)
}

// discoveryLink is the link to share for a secret, in the discovery's form,
// <clientOrigin><page>#<coreId>.<secret>.<fingerprint>, while a discovery has claimed the core (the
// protocol's §3.1; D292 §4): the fingerprint lets the device check, on first contact, that it reached
// this core and no other. Empty when none has, and the client makes the link on its own origin.
func discoveryLink(status discovery.Status, page, secret string) string {
	if !status.IsPublished() || status.CoreID == "" || status.ClientOrigin == "" || status.Fingerprint == "" {
		return ""
	}
	return status.ClientOrigin + page + "#" + link.Fragment{
		CoreID: status.CoreID, Secret: secret, Fingerprint: status.Fingerprint,
	}.String()
}

// UsersOutput is everyone.
type UsersOutput struct {
	Body struct {
		Users []UserEntryBody `json:"users"`
	}
}

// CreateUserInput is the admin setting a person's account up in full (D246 §2): a name, a
// password and a role, and the admin's note of who it is for.
type CreateUserInput struct {
	AuthorizationHeader
	Body struct {
		Name      string `json:"name" maxLength:"100" minLength:"1"`
		Password  string `json:"password" minLength:"1"`
		Role      string `doc:"The role's name, their whole access (D243 §4)" json:"role" maxLength:"100" minLength:"1"`
		AdminNote string `doc:"Who it is for; only the admin sees it" json:"adminNote,omitempty" maxLength:"200"`
	}
}

// CreateUserOutput is the new user.
type CreateUserOutput struct {
	Body struct {
		User UserEntryBody `json:"user"`
	}
}

// RegistrationInvitationInput asks for a new registration invitation for a person.
type RegistrationInvitationInput struct {
	AuthorizationHeader
	UserID int64 `minimum:"1" path:"id"`
}

// IssuedSecretOutput is one registration invitation.
type IssuedSecretOutput struct {
	Body IssuedSecretBody
}

// RegisterInput uses a link: its secret, the name the person chose (an invitation's; a
// registration link's person has one), the password they chose, and the device they are on (D218
// §2, D246 §2).
type RegisterInput struct {
	Body struct {
		Secret   string        `json:"secret"   minLength:"1"`
		Name     string        `doc:"An invitation's: how they are shown and sign in" json:"name,omitempty" maxLength:"100"` //nolint:lll // one struct tag
		Password string        `json:"password" minLength:"1"`
		Device   NewDeviceBody `json:"device"`
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

func (server *Server) registerUsers() {
	tags := []string{tagUsers}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listUsers", Method: http.MethodGet, Path: "/users",
		Summary: "Everyone, for the admin", Tags: tags,
	}, server.listUsers)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "createUser", Method: http.MethodPost, Path: "/users", DefaultStatus: http.StatusCreated,
		Summary: "Set a user up in full: a name, a password and a role (D246 §2)", Tags: tags,
	}, server.createUser)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "issueRegistrationInvitation", Method: http.MethodPost,
		Path:    "/users/{id}/registration-invitation",
		Summary: "A new registration link for a person, withdrawing the old; their password works until used",
		Tags:    tags,
	}, server.issueRegistrationInvitation)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "register", Method: http.MethodPost, Path: "/registration",
		Summary: "Use an invitation or a registration link: make the account or set a new password, and sign in",
		Tags:    tags,
	}, server.register)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "changePassword", Method: http.MethodPut, Path: "/me/password",
		DefaultStatus: http.StatusNoContent,
		Summary:       "Change one's own password; other sessions stay signed in", Tags: tags,
	}, server.changePassword)
}

func userEntryBody(account auth.Account) UserEntryBody {
	return UserEntryBody{
		ID: account.ID, Name: account.Name, Role: string(account.Role), AdminNote: account.AdminNote,
		HasPassword: account.HasPassword,
	}
}

func (server *Server) listUsers(ctx context.Context, input *struct{ AuthorizationHeader }) (*UsersOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	accounts, err := dependencies.Auth.Accounts(ctx)
	if err != nil {
		return nil, server.authError(ctx, "list accounts", err)
	}
	output := &UsersOutput{}
	output.Body.Users = make([]UserEntryBody, 0, len(accounts))
	for _, account := range accounts {
		output.Body.Users = append(output.Body.Users, userEntryBody(account))
	}
	return output, nil
}

func (server *Server) createUser(ctx context.Context, input *CreateUserInput) (*CreateUserOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	role := auth.Role(input.Body.Role)
	id, err := dependencies.Auth.CreateUser(ctx, auth.NewUser{
		Name: input.Body.Name, Password: input.Body.Password, Role: role, AdminNote: input.Body.AdminNote,
	})
	if err != nil {
		return nil, server.authError(ctx, "create an account", err)
	}
	output := &CreateUserOutput{}
	output.Body.User = UserEntryBody{
		ID: id, Name: input.Body.Name, Role: string(role), AdminNote: input.Body.AdminNote, HasPassword: true,
	}
	return output, nil
}

func (server *Server) issueRegistrationInvitation(
	ctx context.Context, input *RegistrationInvitationInput,
) (*IssuedSecretOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	invitation, err := dependencies.Auth.IssueRegistrationInvitation(ctx, input.UserID)
	if err != nil {
		return nil, server.authError(ctx, "make a registration invitation", err)
	}
	return &IssuedSecretOutput{Body: issuedSecretBody(invitation, dependencies.Discovery)}, nil
}

func (server *Server) register(ctx context.Context, input *RegisterInput) (*SessionOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	issued, err := dependencies.Auth.Register(ctx, input.Body.Secret, input.Body.Name, input.Body.Password,
		newDevice(input.Body.Device))
	if err != nil {
		return nil, server.authError(ctx, "register", err)
	}
	return server.newSessionOutput(ctx, dependencies, issued, input.isOverTLS)
}

func (server *Server) changePassword(ctx context.Context, input *ChangePasswordInput) (*struct{}, error) {
	dependencies, principal, err := server.requireAccountManager(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.ChangePassword(ctx, principal.UserID, input.Body.CurrentPassword, input.Body.NewPassword,
		bearerToken(input.Authorization))
	if err != nil {
		return nil, server.authError(ctx, "change a password", err)
	}
	return &struct{}{}, nil
}

// RenameUserInput renames a user, for the admin.
type RenameUserInput struct {
	AuthorizationHeader
	UserID int64 `minimum:"1" path:"id"`
	Body   struct {
		Name string `json:"name" maxLength:"100" minLength:"1"`
	}
}

// UserPageOutput is one user's page, for the admin: who they are, and what they have watched.
type UserPageOutput struct {
	Body struct {
		User       UserEntryBody  `json:"user"`
		WatchStats WatchStatsBody `json:"watchStats"`
		// OwnLinkExpiresAt is when the person's own link stops leading them back through a discovery,
		// unless a visit renews it; absent when they have none (D292 §4).
		OwnLinkExpiresAt int64 `doc:"Unix milliseconds" json:"ownLinkExpiresAt,omitempty"`
	}
}

// WatchStatsBody is what a person has watched.
type WatchStatsBody struct {
	WatchedMs          int64 `json:"watchedMs"`
	TitlesWatchedCount int64 `json:"titlesWatchedCount"`
	LastWatchedAt      int64 `doc:"Unix milliseconds; absent when nothing was watched" json:"lastWatchedAt,omitempty"`
}

// pathUser is one user's address, which their page, a rename and a removal share.
const pathUser = "/users/{id}"

func (server *Server) registerUser() {
	tags := []string{tagUsers}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getUser", Method: http.MethodGet, Path: pathUser,
		Summary: "A user's page, for the admin: who they are and what they have watched", Tags: tags,
	}, server.getUser)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "renameUser", Method: http.MethodPut, Path: pathUser,
		Summary: "Rename a user; names are for display and sign-in, and stay unique", Tags: tags,
		DefaultStatus: http.StatusNoContent,
	}, server.renameUser)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "removeUser", Method: http.MethodDelete, Path: pathUser,
		Summary: "Remove a user: their sessions and devices end, and their own watching is deleted (S40)",
		Tags:    tags, DefaultStatus: http.StatusNoContent,
	}, server.removeUser)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "revokeOwnLink", Method: http.MethodDelete, Path: pathUser + "/own-link",
		Summary: "End a person's own link, which leads them back to the server through a discovery (D292 §4)",
		Tags:    tags, DefaultStatus: http.StatusNoContent,
	}, server.revokeOwnLink)
}

func (server *Server) revokeOwnLink(ctx context.Context, input *UserInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Auth.RevokeOwnLink(ctx, input.UserID); err != nil {
		return nil, server.authError(ctx, "revoke a person's own link", err)
	}
	server.logger.InfoContext(ctx, "an admin revoked a person's own link", "userId", input.UserID)
	return &struct{}{}, nil
}

func (server *Server) getUser(ctx context.Context, input *UserInput) (*UserPageOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	accounts, err := dependencies.Auth.Accounts(ctx)
	if err != nil {
		return nil, server.authError(ctx, "list accounts", err)
	}
	output := &UserPageOutput{}
	for _, account := range accounts {
		if account.ID == input.UserID {
			output.Body.User = userEntryBody(account)
		}
	}
	if output.Body.User.ID == 0 {
		return nil, server.authError(ctx, "read a person", auth.ErrNoUser)
	}
	stats, err := dependencies.Playback.WatchStatsOf(ctx, input.UserID)
	if err != nil {
		return nil, server.playbackError(ctx, "read a person's watching", err)
	}
	output.Body.WatchStats = WatchStatsBody{WatchedMs: stats.WatchedMs, TitlesWatchedCount: stats.TitlesWatched}
	if !stats.LastWatchedAt.IsZero() {
		output.Body.WatchStats.LastWatchedAt = stats.LastWatchedAt.UnixMilli()
	}
	own, hasOwnLink, err := dependencies.Auth.OwnLink(ctx, input.UserID)
	if err != nil {
		return nil, server.authError(ctx, "read a person's own link", err)
	}
	if hasOwnLink {
		output.Body.OwnLinkExpiresAt = own.ExpiresAt.UnixMilli()
	}
	return output, nil
}

func (server *Server) renameUser(ctx context.Context, input *RenameUserInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Auth.RenameUser(ctx, input.UserID, input.Body.Name)
	if err != nil {
		return nil, server.authError(ctx, "rename a person", err)
	}
	return &struct{}{}, nil
}

func (server *Server) removeUser(ctx context.Context, input *UserInput) (*struct{}, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	// What was theirs goes, and who set what everyone shares is forgotten (D305 §1).
	err = dependencies.Auth.RemoveUser(ctx, principal.UserID, input.UserID,
		playback.ForgetWatchingOf, preferences.ForgetPreferencesOf, identify.ForgetOverridesSetBy,
		settings.ForgetSettingsSetBy, secrets.ForgetSecretsSetBy)
	if err != nil {
		return nil, server.authError(ctx, "remove a person", err)
	}
	return &struct{}{}, nil
}
