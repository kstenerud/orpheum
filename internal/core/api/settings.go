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

// ConfigListInput asks for every server setting, or only those changed from their starting value.
type ConfigListInput struct {
	AuthorizationHeader
	Modified bool `doc:"Only the settings whose value differs from the value a new server starts with" query:"modified"`
}

// ConfigKeyInput names one setting.
type ConfigKeyInput struct {
	AuthorizationHeader
	Key string `path:"key"`
}

// SeedOutput names the settings a seed wrote, those that had no value, and the rows it found that
// no setting reads, which it leaves alone (D200).
type SeedOutput struct {
	Body struct {
		Written []string `json:"written"`
		Updated []string `doc:"Unmodified settings moved to a starting value this release changed" json:"updated"`
		Unknown []string `doc:"Rows no setting reads, such as a typo; reported, never deleted" json:"unknown"`
	}
}

// SettingOutput answers with one setting.
type SettingOutput struct {
	Body SettingBody
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

// tagSettings groups the settings, config and secrets operations in the OpenAPI document.
const tagSettings = "Settings"

// registerConfig is the raw view of every server setting, as Firefox's about:config is (D200).
func (server *Server) registerConfig() {
	tags := []string{tagSettings}
	huma.Register(server.api, huma.Operation{
		OperationID: "getConfig", Method: http.MethodGet, Path: "/config",
		Summary: "Every server setting, with its starting value and whether it was changed; ?modified=true " +
			"lists only the changed ones (admin)",
		Tags: tags,
	}, server.getConfig)
	huma.Register(server.api, huma.Operation{
		OperationID: "putConfig", Method: http.MethodPut, Path: "/config",
		Summary: "Change any server settings, all or none (admin)", Tags: tags,
	}, server.putConfig)
	huma.Register(server.api, huma.Operation{
		OperationID: "seedConfig", Method: http.MethodPost, Path: "/config/seed",
		Summary: "Write the starting value of every server setting that has none; changes nothing else (admin)",
		Tags:    tags,
	}, server.seedConfig)
	huma.Register(server.api, huma.Operation{
		OperationID: "resetConfig", Method: http.MethodPost, Path: "/config/{key}/reset",
		Summary: "Set a server setting to default: its starting value, no longer marked modified, so it " +
			"follows its starting value when a release changes it (admin)",
		Tags: tags,
	}, server.resetConfig)
}

func (server *Server) getConfig(ctx context.Context, input *ConfigListInput) (*SettingsOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	listed, err := server.listSettings(ctx, dependencies.Settings, nil, false)
	if err != nil || !input.Modified {
		return listed, err
	}
	changed := []SettingBody{}
	for _, setting := range listed.Body.Settings {
		if setting.IsModified {
			changed = append(changed, setting)
		}
	}
	listed.Body.Settings = changed
	return listed, nil
}

func (server *Server) putConfig(ctx context.Context, input *PutSettingsInput) (*SettingsOutput, error) {
	dependencies, principal, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.putSettings(ctx, dependencies.Settings, nil, input.Body.Values, principal.UserID, false)
}

func (server *Server) seedConfig(ctx context.Context, input *AuthorizationHeader) (*SeedOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	report, err := dependencies.Settings.Seed(ctx)
	if err != nil {
		return nil, server.internal(ctx, "seed the settings", err)
	}
	output := &SeedOutput{}
	output.Body.Written, output.Body.Updated, output.Body.Unknown = []string{}, []string{}, []string{}
	for _, key := range report.Written {
		output.Body.Written = append(output.Body.Written, string(key))
	}
	for _, key := range report.Updated {
		output.Body.Updated = append(output.Body.Updated, string(key))
	}
	for _, unknown := range report.Unknown {
		output.Body.Unknown = append(output.Body.Unknown, unknown.String())
	}
	return output, nil
}

func (server *Server) resetConfig(ctx context.Context, input *ConfigKeyInput) (*SettingOutput, error) {
	dependencies, principal, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Settings.Reset(ctx, settings.Key(input.Key), &principal.UserID)
	if errors.Is(err, settings.ErrUnknownSetting) || errors.Is(err, settings.ErrWrongScope) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such server setting")
	}
	if err != nil {
		return nil, server.internal(ctx, "reset a setting", err)
	}
	listed, err := server.listSettings(ctx, dependencies.Settings, nil, false)
	if err != nil {
		return nil, err
	}
	for _, setting := range listed.Body.Settings {
		if setting.Key == input.Key {
			return &SettingOutput{Body: setting}, nil
		}
	}
	return nil, server.internal(ctx, "reset a setting", errors.New("the reset setting is not listed"))
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
