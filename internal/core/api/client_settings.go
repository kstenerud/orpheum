// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// ClientSettingsBody is the server settings every client reads, whoever is signed in.
type ClientSettingsBody struct {
	// ScreenRefreshFirstWaitSeconds is how long a screen showing what the server changes in the
	// background waits before asking again, and after an answer that changed it (D237).
	ScreenRefreshFirstWaitSeconds int64 `json:"screenRefreshFirstWaitSeconds"`
	// ScreenRefreshLongestWaitSeconds is the longest it waits: the wait doubles to it while nothing
	// changes (D237).
	ScreenRefreshLongestWaitSeconds int64 `json:"screenRefreshLongestWaitSeconds"`
	// ScreenRefreshFailedWaitSeconds is how long it waits after a read that failed, every time
	// (D237).
	ScreenRefreshFailedWaitSeconds int64 `json:"screenRefreshFailedWaitSeconds"`
	// InterestSaidEverySeconds is how often a shown film's or series' page says so (D324 §1).
	InterestSaidEverySeconds int64 `json:"interestSaidEverySeconds"`
	// ShouldShowDeveloperPages is whether the client's menu offers its pages for development, such
	// as the UI gallery (the owner, 2026-10-17).
	ShouldShowDeveloperPages bool `json:"shouldShowDeveloperPages"`
	// ShouldOfferDeviceTests is whether the menu offers the device lab's tests to everyone signed
	// in, and the server takes their results (the owner, 2026-10-25).
	ShouldOfferDeviceTests bool `json:"shouldOfferDeviceTests"`
	// ReadyAheadOfNextChunkPercent is how much film a player waits to have loaded ahead before it
	// starts, and plays on after a seek: this percent of its next chunk's expected load time (the
	// owner, 2026-10-28).
	ReadyAheadOfNextChunkPercent int64 `json:"readyAheadOfNextChunkPercent"`
}

// ClientSettingsOutput is GET /client-settings's answer.
type ClientSettingsOutput struct {
	Body ClientSettingsBody
}

func (server *Server) registerClientSettings() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getClientSettings", Method: http.MethodGet, Path: "/client-settings",
		Summary: "The server settings every client reads: how often a screen asks again (D237), and more",
		Tags:    []string{tagSettings},
	}, server.getClientSettings)
	server.registerClientManifest()
}

func (server *Server) getClientSettings(
	ctx context.Context, input *AuthorizationHeader,
) (*ClientSettingsOutput, error) {
	dependencies, _, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &ClientSettingsOutput{}
	for _, read := range []struct {
		key  settings.Key
		into *int64
	}{
		{settings.ScreenRefreshFirstWaitSeconds, &output.Body.ScreenRefreshFirstWaitSeconds},
		{settings.ScreenRefreshLongestWaitSeconds, &output.Body.ScreenRefreshLongestWaitSeconds},
		{settings.ScreenRefreshFailedWaitSeconds, &output.Body.ScreenRefreshFailedWaitSeconds},
		{settings.InterestSaidEverySeconds, &output.Body.InterestSaidEverySeconds},
		{settings.ReadyAheadOfNextChunkPercent, &output.Body.ReadyAheadOfNextChunkPercent},
	} {
		value, err := dependencies.Settings.Int(ctx, read.key)
		if err != nil {
			return nil, server.settingsError(ctx, "read "+string(read.key), err)
		}
		*read.into = value
	}
	for _, read := range []struct {
		key  settings.Key
		into *bool
	}{
		{settings.ShouldShowDeveloperPages, &output.Body.ShouldShowDeveloperPages},
		{settings.ShouldOfferDeviceTests, &output.Body.ShouldOfferDeviceTests},
	} {
		value, err := dependencies.Settings.Bool(ctx, read.key)
		if err != nil {
			return nil, server.settingsError(ctx, "read "+string(read.key), err)
		}
		*read.into = value
	}
	return output, nil
}
