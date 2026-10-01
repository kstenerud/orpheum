// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// OfflineReminderOutput is the reminder an admin sees on every page while folders or disks are
// offline, until they dismiss it for offline_reminder_interval_days (D231 §7, P39).
type OfflineReminderOutput struct {
	Body struct {
		// IsShown is whether the admin is reminded now: something is offline, and they have not
		// dismissed the reminder within the interval, or something went offline since they did.
		IsShown bool `json:"isShown"`
		// RootCount and DiskCount are how many library folders, and disks mounted inside one, are
		// offline, as the System page lists them.
		RootCount int `json:"rootCount"`
		DiskCount int `json:"diskCount"`
		// Since is when the longest offline of them was first found missing.
		Since int64 `doc:"Unix milliseconds; absent when nothing is offline" json:"since,omitempty"`
	}
}

func (server *Server) registerOfflineReminder() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getOfflineReminder", Method: http.MethodGet, Path: "/offline-reminder",
		Summary: "Whether to remind the admin that folders or disks are offline, and how many since when " +
			"(admin; D231 §7)",
		Tags: []string{tagRunningIt},
	}, server.getOfflineReminder)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "dismissOfflineReminder", Method: http.MethodPost, Path: "/offline-reminder/dismissal",
		Summary: "Hide the offline reminder from this admin for offline_reminder_interval_days (admin; D231 §7)",
		Tags:    []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
	}, server.dismissOfflineReminder)
}

func (server *Server) getOfflineReminder(
	ctx context.Context, input *AuthorizationHeader,
) (*OfflineReminderOutput, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	offline, err := dependencies.Libraries.Offline(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list what is offline", err)
	}
	output := &OfflineReminderOutput{}
	if len(offline) == 0 {
		return output, nil
	}
	for _, missing := range offline {
		if missing.MountPath == "" {
			output.Body.RootCount++
		} else {
			output.Body.DiskCount++
		}
	}
	output.Body.Since = offline[0].Since.UnixMilli() // the longest offline comes first
	newestSince := offline[0].Since
	for _, missing := range offline {
		if missing.Since.After(newestSince) {
			newestSince = missing.Since
		}
	}
	hiddenFor, err := dependencies.Settings.Duration(ctx, settings.OfflineReminderIntervalDays)
	if err != nil {
		return nil, server.settingsError(ctx, "read offline_reminder_interval_days", err)
	}
	output.Body.IsShown, err = dependencies.Preferences.IsOfflineReminderDue(ctx, principal.UserID, hiddenFor,
		newestSince)
	if err != nil {
		return nil, server.logInternalError(ctx, "read whether the offline reminder was dismissed", err)
	}
	return output, nil
}

func (server *Server) dismissOfflineReminder(ctx context.Context, input *AuthorizationHeader) (*struct{}, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := dependencies.Preferences.DismissOfflineReminder(ctx, principal.UserID); err != nil {
		return nil, server.logInternalError(ctx, "keep the offline reminder's dismissal", err)
	}
	return &struct{}{}, nil
}
