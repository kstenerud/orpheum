// SPDX-License-Identifier: AGPL-3.0-or-later

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
	Key           string          `json:"key"`
	Kind          string          `enum:"bool,integer,string" json:"kind"`
	Value         json.RawMessage `doc:"A bool, a whole number or a string, by its kind; null when unset" json:"value"`
	StartingValue json.RawMessage `doc:"The value a new server starts with; null when it has none" json:"startingValue"`
	IsModified    bool            `json:"isModified"`
	Minimum       *int64          `json:"minimum,omitempty"`
	Maximum       *int64          `json:"maximum,omitempty"`
	Question      string          `json:"question,omitempty"`
	Description   string          `json:"description"`
	IsExposed     bool            `doc:"Shown on the settings page; every setting is in /config" json:"isExposed"`
	SetByUserID   *int64          `json:"setByUserId,omitempty"`
	SetAt         int64           `json:"setAt"`
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
		ValuesByKey map[string]json.RawMessage `doc:"Setting keys and their new JSON values" json:"valuesByKey"`
	}
}

func (server *Server) registerSettings() {
	tags := []string{tagSettings}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getSettings", Method: http.MethodGet, Path: "/settings",
		Summary: "The server settings the settings page shows (admin)", Tags: tags,
	}, server.getSettings)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "putSettings", Method: http.MethodPut, Path: "/settings",
		Summary: "Change server settings the settings page shows, all or none (admin)", Tags: tags,
	}, server.putSettings)
	server.registerConfig()
}

func (server *Server) listSettings(
	ctx context.Context, service *settings.Service, isExposedOnly bool,
) (*SettingsOutput, error) {
	entries, err := service.List(ctx, isExposedOnly)
	if err != nil {
		return nil, server.settingsError(ctx, "list settings", err)
	}
	output := &SettingsOutput{}
	output.Body.Settings = []SettingBody{}
	for _, entry := range entries {
		output.Body.Settings = append(output.Body.Settings, settingBody(entry))
	}
	return output, nil
}

// settingsError maps the settings package's refusals to API errors. Every handler that calls
// settings answers through it, so a refusal always leaves with one code and one message: a
// setting named in the path that is not there is not_found, and a value in the body that cannot
// be set is invalid_request at body.valuesByKey.<key>, so the settings page shows it beside that
// setting. A setting read as the wrong kind is the server's fault, not the request's.
func (server *Server) settingsError(ctx context.Context, action string, err error) error {
	refused, isRefused := errors.AsType[*settings.ValueError](err)
	switch {
	case errors.Is(err, settings.ErrNoServerSetting):
		return newError(http.StatusNotFound, CodeNotFound, "no such server setting")
	case isRefused:
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.valuesByKey."+string(refused.Key), err.Error())
	case errors.Is(err, settings.ErrInUse):
		return newError(http.StatusConflict, CodeConflict,
			"this release reads that setting; set it to default instead of deleting it")
	}
	return server.logInternalError(ctx, action, err)
}

func settingBody(entry settings.Entry) SettingBody {
	definition := entry.Definition
	body := SettingBody{
		Key: string(definition.Key), Kind: string(definition.Kind), Value: entry.Value,
		StartingValue: definition.StartJSON,
		IsModified:    entry.IsModified, Question: definition.Question, Description: definition.Description,
		IsExposed: definition.IsExposed, SetByUserID: entry.SetByUserID, SetAt: entry.SetAt,
	}
	if definition.Kind == settings.KindInteger {
		body.Minimum, body.Maximum = &definition.Minimum, &definition.Maximum
	}
	return body
}

func (server *Server) writeSettings(
	ctx context.Context, service *settings.Service, values map[string]json.RawMessage, setByUserID int64,
	isExposedOnly bool,
) (*SettingsOutput, error) {
	if err := service.Set(ctx, values, &setByUserID, isExposedOnly); err != nil {
		return nil, server.settingsError(ctx, "set settings", err)
	}
	return server.listSettings(ctx, service, isExposedOnly)
}

// getSettings answers GET /settings: the server settings the settings page shows (admin).
func (server *Server) getSettings(ctx context.Context, input *AuthorizationHeader) (*SettingsOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.listSettings(ctx, dependencies.Settings, true)
}

// putSettings answers PUT /settings: change server settings the settings page shows, all or none (admin).
func (server *Server) putSettings(ctx context.Context, input *PutSettingsInput) (*SettingsOutput, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.writeSettings(ctx, dependencies.Settings, input.Body.ValuesByKey, principal.UserID, true)
}
