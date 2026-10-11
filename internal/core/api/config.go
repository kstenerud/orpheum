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

// tagSettings groups the settings, config and secrets operations in the OpenAPI document.
const tagSettings = "Settings"

// ConfigFileKey is one key of the config file as the server runs with it: read-only here, since the
// file is the machine owner's to edit (D200, D202).
type ConfigFileKey struct {
	Key           string          `json:"key"`
	Value         json.RawMessage `json:"value"`
	StartingValue json.RawMessage `doc:"The starting value; null when the key has none" json:"startingValue"`
	IsFromFile    bool            `doc:"False when the file lacks it: it runs on its starting value" json:"isFromFile"`
	Description   string          `json:"description"`
}

// ConfigListInput asks for every server setting, or only those a person changed.
type ConfigListInput struct {
	AuthorizationHeader
	OnlyModified bool `doc:"Only the settings a person set, even to their starting value" query:"onlyModified"`
}

// ConfigOutput is every server setting beside its starting value, and the config file's keys.
type ConfigOutput struct {
	Body struct {
		Settings       []SettingBody   `json:"settings"`
		ConfigFileKeys []ConfigFileKey `json:"configFileKeys"`
	}
}

// SettingKeyInput names one setting.
type SettingKeyInput struct {
	AuthorizationHeader
	Key string `path:"key"`
}

// SettingOutput answers with one setting.
type SettingOutput struct {
	Body SettingBody
}

// UnusedBody is a setting row this release does not read (D202).
type UnusedBody struct {
	Key           string          `json:"key"`
	Value         json.RawMessage `json:"value"`
	RetiredIn     string          `doc:"The release that stopped reading it; empty for a typo" json:"retiredIn,omitempty"` //nolint:lll // one struct tag
	RetiredReason string          `json:"retiredReason,omitempty"`
}

// UnusedOutput lists the unused rows.
type UnusedOutput struct {
	Body struct {
		UnusedSettings []UnusedBody `json:"unusedSettings"`
	}
}

// DeleteUnusedInput names one unused row.
type DeleteUnusedInput struct {
	AuthorizationHeader
	Key string `path:"key"`
}

// registerConfig is the raw view of every server setting, as Firefox's about:config is (D200).
func (server *Server) registerConfig() {
	tags := []string{tagSettings}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getConfig", Method: http.MethodGet, Path: "/config",
		Summary: "Every server setting beside its starting value, and the config file's keys (read-only); " +
			"?onlyModified=true lists only the settings a person set (admin)",
		Tags: tags,
	}, server.getConfig)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "putConfig", Method: http.MethodPut, Path: "/config",
		Summary: "Change any server settings, all or none (admin)", Tags: tags,
	}, server.putConfig)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "resetConfig", Method: http.MethodPost, Path: "/config/{key}/reset",
		Summary: "Set a server setting to default: its starting value, no longer marked modified, so it " +
			"follows its starting value when a release changes it (admin)",
		Tags: tags,
	}, server.resetConfig)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getUnusedConfig", Method: http.MethodGet, Path: "/config/unused",
		Summary: "Setting rows this release does not read: retired keys, or typos (admin)", Tags: tags,
	}, server.getUnusedConfig)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "deleteUnusedConfig", Method: http.MethodDelete, Path: "/config/unused/{key}",
		DefaultStatus: http.StatusNoContent,
		Summary: "Delete a setting row this release does not read; a setting in use is set to default " +
			"instead (admin)",
		Tags: tags,
	}, server.deleteUnusedConfig)
}

func (server *Server) getConfig(ctx context.Context, input *ConfigListInput) (*ConfigOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	listed, err := server.listSettings(ctx, dependencies.Settings, false)
	if err != nil {
		return nil, err
	}
	output := &ConfigOutput{}
	output.Body.Settings = []SettingBody{}
	for _, setting := range listed.Body.Settings {
		if !input.OnlyModified || setting.IsModified {
			output.Body.Settings = append(output.Body.Settings, setting)
		}
	}
	output.Body.ConfigFileKeys = append([]ConfigFileKey{}, dependencies.ConfigFileKeys...)
	return output, nil
}

func (server *Server) putConfig(ctx context.Context, input *PutSettingsInput) (*SettingsOutput, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.writeSettings(ctx, dependencies.Settings, input.Body.ValuesByKey, principal.UserID, false)
}

func (server *Server) resetConfig(ctx context.Context, input *SettingKeyInput) (*SettingOutput, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Settings.Reset(ctx, settings.Key(input.Key), &principal.UserID)
	if err != nil {
		return nil, server.settingsError(ctx, "reset a setting", err)
	}
	listed, err := server.listSettings(ctx, dependencies.Settings, false)
	if err != nil {
		return nil, err
	}
	for _, setting := range listed.Body.Settings {
		if setting.Key == input.Key {
			return &SettingOutput{Body: setting}, nil
		}
	}
	return nil, server.logInternalError(ctx, "reset a setting", errors.New("the reset setting is not listed"))
}

func (server *Server) getUnusedConfig(ctx context.Context, input *AuthorizationHeader) (*UnusedOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	unused, err := dependencies.Settings.Unused(ctx)
	if err != nil {
		return nil, server.settingsError(ctx, "list unused settings", err)
	}
	output := &UnusedOutput{}
	output.Body.UnusedSettings = []UnusedBody{}
	for _, row := range unused {
		body := UnusedBody{Key: row.Key, Value: json.RawMessage(row.ValueJSON)}
		if row.Retired != nil {
			body.RetiredIn, body.RetiredReason = row.Retired.Release, row.Retired.Reason
		}
		output.Body.UnusedSettings = append(output.Body.UnusedSettings, body)
	}
	return output, nil
}

func (server *Server) deleteUnusedConfig(ctx context.Context, input *DeleteUnusedInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Settings.DeleteUnused(ctx, input.Key)
	if err != nil {
		return nil, server.settingsError(ctx, "delete an unused setting", err)
	}
	return &struct{}{}, nil
}
