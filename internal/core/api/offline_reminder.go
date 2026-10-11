// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// OfflineReminderOutput is the reminder an admin sees on every page while folders or disks are
// offline, a scan's removals wait for them, or a scan could not read something, until they dismiss
// it for offline_reminder_interval_days (D231 §7, D277 §2–§3, P39).
type OfflineReminderOutput struct {
	Body struct {
		// IsShown is whether the admin is reminded now: something is offline, and they have not
		// dismissed the reminder within the interval, or something went offline since they did.
		IsShown bool `json:"isShown"`
		// RootCount and DiskCount are how many library folders, and disks mounted inside one, are
		// offline, as the System page lists them.
		RootCount int `json:"rootCount"`
		DiskCount int `json:"diskCount"`
		// WaitingRootCount is how many library folders' scans wait for the admin before removing
		// files; UnreadCount how many files and folders scans could not read (D277).
		WaitingRootCount int `json:"waitingRootCount"`
		UnreadCount      int `json:"unreadCount"`
		// OfflineAt is when the longest offline of them was first found missing, in Unix
		// milliseconds; absent when nothing is offline.
		OfflineAt int64 `json:"offlineAt,omitempty"`
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
	output := &OfflineReminderOutput{}
	newestSince, err := server.countWhatNeedsTheAdmin(ctx, dependencies, output)
	if err != nil {
		return nil, err
	}
	if newestSince.IsZero() {
		return output, nil
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

// countWhatNeedsTheAdmin counts into output what is offline, what waits for the admin and what could
// not be read, and answers when the newest of them began, zero when there is none: a reminder
// dismissed before then is shown again.
func (server *Server) countWhatNeedsTheAdmin(
	ctx context.Context, dependencies *Dependencies, output *OfflineReminderOutput,
) (time.Time, error) {
	offline, err := dependencies.Libraries.Offline(ctx)
	if err != nil {
		return time.Time{}, server.libraryError(ctx, "list what is offline", err)
	}
	waiting, err := dependencies.Libraries.Waiting(ctx)
	if err != nil {
		return time.Time{}, server.libraryError(ctx, "list what waits for the admin", err)
	}
	unread, err := dependencies.Libraries.UnreadPaths(ctx)
	if err != nil {
		return time.Time{}, server.libraryError(ctx, "list what could not be read", err)
	}
	var newestSince time.Time
	keepNewest := func(since time.Time) {
		if since.After(newestSince) {
			newestSince = since
		}
	}
	for _, missing := range offline {
		if missing.MountPath == "" {
			output.Body.RootCount++
		} else {
			output.Body.DiskCount++
		}
		keepNewest(missing.Since)
	}
	if len(offline) > 0 {
		output.Body.OfflineAt = offline[0].Since.UnixMilli() // the longest offline comes first
	}
	for _, root := range waiting {
		keepNewest(root.Since)
	}
	for _, path := range unread {
		keepNewest(path.Since)
	}
	output.Body.WaitingRootCount, output.Body.UnreadCount = len(waiting), len(unread)
	return newestSince, nil
}
