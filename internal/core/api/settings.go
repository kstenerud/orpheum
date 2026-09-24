// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// SettingBody is one setting as the settings page shows it: the value in force and whether it is
// the default, or the question still to answer (D183).
type SettingBody struct {
	Key       string          `json:"key"`
	Kind      string          `enum:"bool,integer,string" json:"kind"`
	Value     json.RawMessage `json:"value"                         nullable:"true"`
	IsDefault bool            `json:"isDefault"`
	Question  string          `json:"question,omitempty"`
	SetBy     *int64          `json:"setBy,omitempty"`
	SetAt     int64           `json:"setAt,omitempty"`
}

// SettingsOutput lists settings.
type SettingsOutput struct {
	Body struct {
		Settings []SettingBody `json:"settings"`
	}
}

// PutSettingsInput sets settings, all or none.
type PutSettingsInput struct {
	AuthorizationHeader
	Body struct {
		Values map[string]json.RawMessage `doc:"Setting keys and their new JSON values" json:"values"`
	}
}

func (server *Server) registerSettings() {
	tags := []string{"Settings"}
	huma.Register(server.api, huma.Operation{
		OperationID: "getSettings", Method: http.MethodGet, Path: "/settings",
		Summary: "The server's settings (admin)", Tags: tags,
	}, func(ctx context.Context, input *AuthorizationHeader) (*SettingsOutput, error) {
		dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		return server.listSettings(ctx, dependencies.Settings, nil)
	})
	huma.Register(server.api, huma.Operation{
		OperationID: "putSettings", Method: http.MethodPut, Path: "/settings",
		Summary: "Change server settings, all or none (admin)", Tags: tags,
	}, func(ctx context.Context, input *PutSettingsInput) (*SettingsOutput, error) {
		dependencies, principal, err := server.signedInAdmin(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		return server.putSettings(ctx, dependencies.Settings, nil, input.Body.Values, principal.UserID)
	})
	huma.Register(server.api, huma.Operation{
		OperationID: "getMySettings", Method: http.MethodGet, Path: "/me/settings",
		Summary: "This user's own settings", Tags: tags,
	}, func(ctx context.Context, input *AuthorizationHeader) (*SettingsOutput, error) {
		dependencies, principal, err := server.signedIn(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		return server.listSettings(ctx, dependencies.Settings, &principal.UserID)
	})
	huma.Register(server.api, huma.Operation{
		OperationID: "putMySettings", Method: http.MethodPut, Path: "/me/settings",
		Summary: "Change this user's own settings, all or none", Tags: tags,
	}, func(ctx context.Context, input *PutSettingsInput) (*SettingsOutput, error) {
		dependencies, principal, err := server.signedIn(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		return server.putSettings(ctx, dependencies.Settings, &principal.UserID, input.Body.Values, principal.UserID)
	})
}

func (server *Server) listSettings(ctx context.Context, service *settings.Service, owner *int64) (
	*SettingsOutput, error,
) {
	entries, err := service.List(ctx, owner)
	if err != nil {
		return nil, server.internal(ctx, "list settings", err)
	}
	output := &SettingsOutput{}
	output.Body.Settings = []SettingBody{}
	for _, entry := range entries {
		output.Body.Settings = append(output.Body.Settings, SettingBody{
			Key: entry.Definition.Key, Kind: string(entry.Definition.Kind), Value: entry.Value,
			IsDefault: entry.IsDefault, Question: entry.Definition.Question, SetBy: entry.SetBy, SetAt: entry.SetAt,
		})
	}
	return output, nil
}

func (server *Server) putSettings(
	ctx context.Context, service *settings.Service, owner *int64, values map[string]json.RawMessage, setBy int64,
) (*SettingsOutput, error) {
	err := service.Set(ctx, owner, values, &setBy)
	switch {
	case errors.Is(err, settings.ErrUnknownSetting), errors.Is(err, settings.ErrWrongScope),
		errors.Is(err, settings.ErrWrongKind):
		return nil, newError(http.StatusUnprocessableEntity, CodeInvalidRequest, err.Error())
	case err != nil:
		return nil, server.internal(ctx, "set settings", err)
	}
	return server.listSettings(ctx, service, owner)
}
