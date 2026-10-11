// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ForgetWatchingOf deletes a removed person's own watching: their spans and events, their places
// in files and what they hid from Continue Watching (D173 §4.6). It writes through queries, the
// transaction that removes them (auth.RemoveUser).
func ForgetWatchingOf(ctx context.Context, queries *sqlcgen.Queries, userID int64) error {
	for what, forget := range map[string]func(context.Context, int64) error{
		"spans":         queries.DeletePlaySpansOfUser,
		"events":        queries.DeletePlayEventsOfUser,
		"positions":     queries.DeletePlayPositionsOfUser,
		"hidden titles": queries.DeleteProgressHidesOfUser,
	} {
		if err := forget(ctx, userID); err != nil {
			return fmt.Errorf("delete user %d's %s: %w", userID, what, err)
		}
	}
	return nil
}
