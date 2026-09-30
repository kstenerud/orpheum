// SPDX-License-Identifier: GPL-3.0-or-later

package preferences

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// DismissOfflineReminder keeps that an admin dismissed the reminder that folders or disks are
// offline, now: it stays hidden from them for offline_reminder_interval_days (D231 §7).
func (service *Service) DismissOfflineReminder(ctx context.Context, userID int64) error {
	dismissedAt := strconv.FormatInt(service.clock.Now().UnixMilli(), 10)
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return service.setRaw(ctx, queries, replacing,
			place{userID: userID, scope: scopeUser, axis: axisOfflineReminderDismissedAt}, dismissedAt)
	})
	if err != nil {
		return fmt.Errorf("keep user %d's dismissal of the offline reminder: %w", userID, err)
	}
	return nil
}

// IsOfflineReminderDue is whether an admin is reminded, now, of what is offline: never dismissed,
// or dismissed hiddenFor or longer ago (D231 §7). Whether anything is offline is the caller's to
// know.
func (service *Service) IsOfflineReminderDue(ctx context.Context, userID int64, hiddenFor time.Duration) (bool, error) {
	kept, err := service.database.Read().ListPreferencesAtScope(ctx, sqlcgen.ListPreferencesAtScopeParams{
		UserID: userID, Scope: scopeUser, ScopeID: 0,
	})
	if err != nil {
		return false, fmt.Errorf("read user %d's preferences: %w", userID, err)
	}
	for _, preference := range kept {
		if preference.Axis != axisOfflineReminderDismissedAt {
			continue
		}
		dismissedAtMs, parseErr := strconv.ParseInt(preference.ValueJSON, 10, 64)
		if parseErr != nil {
			return false, fmt.Errorf("read user %d's dismissal of the offline reminder: %w", userID, parseErr)
		}
		return !service.clock.Now().Before(time.UnixMilli(dismissedAtMs).Add(hiddenFor)), nil
	}
	return true, nil
}
