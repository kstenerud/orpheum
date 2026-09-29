// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/secrets"
)

// SecretBody is what may be shown about a secret: never its value (D194 §5).
type SecretBody struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	IsSet        bool   `json:"isSet"`
	Hint         string `doc:"An API key's last characters, secret_hint_chars of them" json:"hint,omitempty"`
	SetByUserID  *int64 `json:"setByUserId,omitempty"`
	SetAt        int64  `json:"setAt,omitempty"`
	NeedsReentry bool   `doc:"Sealed under a key this server no longer has" json:"needsReentry"`
}

// SecretsOutput lists the secrets.
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
	tags := []string{tagSettings}
	huma.Register(server.api, huma.Operation{
		OperationID: "getSecrets", Method: http.MethodGet, Path: "/secrets",
		Summary: "Which secrets are set; never their values (admin)", Tags: tags,
	}, func(ctx context.Context, input *AuthorizationHeader) (*SecretsOutput, error) {
		dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		return server.listSecrets(ctx, dependencies.Secrets)
	})
	huma.Register(server.api, huma.Operation{
		OperationID: "putSecret", Method: http.MethodPut, Path: "/secrets/{name}",
		DefaultStatus: http.StatusNoContent, Summary: "Set a secret; write-only (admin)", Tags: tags,
	}, func(ctx context.Context, input *PutSecretInput) (*struct{}, error) {
		value := secrets.NewValue(input.Body.Value)
		dependencies, principal, err := server.signedInAdmin(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		err = dependencies.Secrets.Set(ctx, secrets.Name(input.Name), value, &principal.UserID)
		return &struct{}{}, server.secretError(ctx, "set a secret", err)
	})
	huma.Register(server.api, huma.Operation{
		OperationID: "deleteSecret", Method: http.MethodDelete, Path: "/secrets/{name}",
		DefaultStatus: http.StatusNoContent, Summary: "Clear a secret (admin)", Tags: tags,
	}, func(ctx context.Context, input *SecretNameInput) (*struct{}, error) {
		dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		err = dependencies.Secrets.Clear(ctx, secrets.Name(input.Name))
		return &struct{}{}, server.secretError(ctx, "clear a secret", err)
	})
}

func (server *Server) listSecrets(ctx context.Context, store *secrets.Store) (*SecretsOutput, error) {
	statuses, err := store.Statuses(ctx)
	if err != nil {
		return nil, server.internal(ctx, "list secrets", err)
	}
	output := &SecretsOutput{}
	output.Body.Secrets = []SecretBody{}
	for _, status := range statuses {
		definition, _ := secrets.Lookup(status.Name)
		output.Body.Secrets = append(output.Body.Secrets, SecretBody{
			Name: string(status.Name), Description: definition.Describe, IsSet: status.IsSet, Hint: status.Hint,
			SetByUserID: status.SetBy, SetAt: status.SetAt, NeedsReentry: status.NeedsReentry,
		})
	}
	return output, nil
}

// secretError maps the secrets package's refusals to API errors.
func (server *Server) secretError(ctx context.Context, doing string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, secrets.ErrUnknownSecret):
		return newError(http.StatusNotFound, CodeNotFound, "no such secret")
	}
	return server.internal(ctx, doing, err)
}
