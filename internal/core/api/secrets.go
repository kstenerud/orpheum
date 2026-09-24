// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/secrets"
)

// SecretBody is what may be shown about a secret: never its value (D194 §5).
type SecretBody struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	IsSet        bool   `json:"isSet"`
	Hint         string `doc:"The last four characters of an API key" json:"hint,omitempty"`
	SetBy        *int64 `json:"setBy,omitempty"`
	SetAt        int64  `json:"setAt,omitempty"`
	NeedsReentry bool   `doc:"Sealed under a key this server no longer has" json:"needsReentry"`
}

// SecretsOutput lists a scope's secrets.
type SecretsOutput struct {
	Body struct {
		Secrets []SecretBody `json:"secrets"`
	}
}

// SecretNameInput names one secret.
type SecretNameInput struct {
	AuthorizationHeader
	Name string `path:"name"`
}

// PutSecretInput sets one secret. The value is write-only: no response ever carries it back.
type PutSecretInput struct {
	AuthorizationHeader
	Name string `path:"name"`
	Body struct {
		Value string `json:"value" minLength:"1" writeOnly:"true"`
	}
}

func (server *Server) registerSecrets() {
	tags := []string{"Settings"}
	for _, scope := range []struct {
		path, suffix, who string
		signIn            func(context.Context, string) (*Dependencies, auth.Principal, error)
		owner             func(auth.Principal) *int64
	}{
		{"/secrets", "", " (admin)", server.signedInAdmin, func(auth.Principal) *int64 { return nil }},
		{"/me/secrets", "Mine", " of this user", server.signedIn, func(principal auth.Principal) *int64 {
			return &principal.UserID
		}},
	} {
		huma.Register(server.api, huma.Operation{
			OperationID: "get" + scope.suffix + "Secrets", Method: http.MethodGet, Path: scope.path,
			Summary: "Which secrets" + scope.who + " are set; never their values", Tags: tags,
		}, func(ctx context.Context, input *AuthorizationHeader) (*SecretsOutput, error) {
			dependencies, principal, err := scope.signIn(ctx, input.Authorization)
			if err != nil {
				return nil, err
			}
			return server.listSecrets(ctx, dependencies.Secrets, scope.owner(principal))
		})
		huma.Register(server.api, huma.Operation{
			OperationID: "put" + scope.suffix + "Secret", Method: http.MethodPut, Path: scope.path + "/{name}",
			DefaultStatus: http.StatusNoContent, Summary: "Set a secret" + scope.who + "; write-only", Tags: tags,
		}, func(ctx context.Context, input *PutSecretInput) (*struct{}, error) {
			value := secrets.NewValue(input.Body.Value)
			dependencies, principal, err := scope.signIn(ctx, input.Authorization)
			if err != nil {
				return nil, err
			}
			err = dependencies.Secrets.Set(ctx, secrets.Name(input.Name), scope.owner(principal), value,
				&principal.UserID)
			return &struct{}{}, server.secretError(ctx, "set a secret", err)
		})
		huma.Register(server.api, huma.Operation{
			OperationID: "delete" + scope.suffix + "Secret", Method: http.MethodDelete, Path: scope.path + "/{name}",
			DefaultStatus: http.StatusNoContent, Summary: "Clear a secret" + scope.who, Tags: tags,
		}, func(ctx context.Context, input *SecretNameInput) (*struct{}, error) {
			dependencies, principal, err := scope.signIn(ctx, input.Authorization)
			if err != nil {
				return nil, err
			}
			err = dependencies.Secrets.Clear(ctx, secrets.Name(input.Name), scope.owner(principal))
			return &struct{}{}, server.secretError(ctx, "clear a secret", err)
		})
	}
}

func (server *Server) listSecrets(ctx context.Context, store *secrets.Store, owner *int64) (*SecretsOutput, error) {
	statuses, err := store.Statuses(ctx, owner)
	if err != nil {
		return nil, server.internal(ctx, "list secrets", err)
	}
	output := &SecretsOutput{}
	output.Body.Secrets = []SecretBody{}
	for _, status := range statuses {
		definition, _ := secrets.Lookup(status.Name)
		output.Body.Secrets = append(output.Body.Secrets, SecretBody{
			Name: string(status.Name), Description: definition.Describe, IsSet: status.IsSet, Hint: status.Hint,
			SetBy: status.SetBy, SetAt: status.SetAt, NeedsReentry: status.NeedsReentry,
		})
	}
	return output, nil
}

// secretError maps the secrets package's refusals to API errors.
func (server *Server) secretError(ctx context.Context, doing string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, secrets.ErrUnknownSecret), errors.Is(err, secrets.ErrWrongScope):
		return newError(http.StatusNotFound, CodeNotFound, "no such secret here")
	}
	return server.internal(ctx, doing, err)
}
