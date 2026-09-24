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

// SettingBody is one setting: the value in force, whether it differs from the value a new server
// starts with, and the question still to answer if it is one (D200).
type SettingBody struct {
	Key        string          `json:"key"`
	Kind       string          `enum:"bool,integer,string" json:"kind"`
	Value      json.RawMessage `json:"value"                         nullable:"true"`
	Start      json.RawMessage `doc:"The value a new server starts with" json:"start" nullable:"true"`
	IsModified bool            `json:"isModified"`
	Minimum    *int64          `json:"minimum,omitempty"`
	Maximum    *int64          `json:"maximum,omitempty"`
	Question   string          `json:"question,omitempty"`
	Describe   string          `json:"describe"`
	IsExposed  bool            `doc:"Shown on the settings page; every setting is in /config" json:"isExposed"`
	SetBy      *int64          `json:"setBy,omitempty"`
	SetAt      int64           `json:"setAt"`
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
	tags := []string{tagSettings}
	huma.Register(server.api, huma.Operation{
		OperationID: "getSettings", Method: http.MethodGet, Path: "/settings",
		Summary: "The server settings the settings page shows (admin)", Tags: tags,
	}, func(ctx context.Context, input *AuthorizationHeader) (*SettingsOutput, error) {
		dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		return server.listSettings(ctx, dependencies.Settings, nil, true)
	})
	huma.Register(server.api, huma.Operation{
		OperationID: "putSettings", Method: http.MethodPut, Path: "/settings",
		Summary: "Change server settings the settings page shows, all or none (admin)", Tags: tags,
	}, func(ctx context.Context, input *PutSettingsInput) (*SettingsOutput, error) {
		dependencies, principal, err := server.signedInAdmin(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		return server.putSettings(ctx, dependencies.Settings, nil, input.Body.Values, principal.UserID, true)
	})
	huma.Register(server.api, huma.Operation{
		OperationID: "getMySettings", Method: http.MethodGet, Path: "/me/settings",
		Summary: "This user's own settings", Tags: tags,
	}, func(ctx context.Context, input *AuthorizationHeader) (*SettingsOutput, error) {
		dependencies, principal, err := server.signedIn(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		return server.listSettings(ctx, dependencies.Settings, &principal.UserID, true)
	})
	huma.Register(server.api, huma.Operation{
		OperationID: "putMySettings", Method: http.MethodPut, Path: "/me/settings",
		Summary: "Change this user's own settings, all or none", Tags: tags,
	}, func(ctx context.Context, input *PutSettingsInput) (*SettingsOutput, error) {
		dependencies, principal, err := server.signedIn(ctx, input.Authorization)
		if err != nil {
			return nil, err
		}
		return server.putSettings(ctx, dependencies.Settings, &principal.UserID, input.Body.Values,
			principal.UserID, true)
	})
	server.registerConfig()
}

func (server *Server) listSettings(ctx context.Context, service *settings.Service, owner *int64, exposedOnly bool) (
	*SettingsOutput, error,
) {
	entries, err := service.List(ctx, owner, exposedOnly)
	if err != nil {
		return nil, server.internal(ctx, "list settings", err)
	}
	output := &SettingsOutput{}
	output.Body.Settings = []SettingBody{}
	for _, entry := range entries {
		output.Body.Settings = append(output.Body.Settings, settingBody(entry))
	}
	return output, nil
}

func settingBody(entry settings.Entry) SettingBody {
	definition := entry.Definition
	body := SettingBody{
		Key: string(definition.Key), Kind: string(definition.Kind), Value: entry.Value, Start: definition.Start,
		IsModified: entry.IsModified, Question: definition.Question, Describe: definition.Describe,
		IsExposed: definition.Exposed, SetBy: entry.SetBy, SetAt: entry.SetAt,
	}
	if definition.Kind == settings.KindInteger {
		body.Minimum, body.Maximum = &definition.Minimum, &definition.Maximum
	}
	return body
}

func (server *Server) putSettings(
	ctx context.Context, service *settings.Service, owner *int64, values map[string]json.RawMessage, setBy int64,
	exposedOnly bool,
) (*SettingsOutput, error) {
	err := service.Set(ctx, owner, values, &setBy, exposedOnly)
	switch {
	case errors.Is(err, settings.ErrUnknownSetting), errors.Is(err, settings.ErrWrongScope),
		errors.Is(err, settings.ErrWrongKind), errors.Is(err, settings.ErrNotExposed):
		return nil, newError(http.StatusUnprocessableEntity, CodeInvalidRequest, err.Error())
	case err != nil:
		return nil, server.internal(ctx, "set settings", err)
	}
	return server.listSettings(ctx, service, owner, exposedOnly)
}
