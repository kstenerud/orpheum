// SPDX-License-Identifier: AGPL-3.0-or-later

package preferences

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ForgetPreferencesOf deletes a removed person's preferences (D173 §4.6). It writes through
// queries, the transaction that removes them (auth.RemoveUser).
func ForgetPreferencesOf(ctx context.Context, queries *sqlcgen.Queries, userID int64) error {
	if err := queries.DeletePreferencesOfUser(ctx, userID); err != nil {
		return fmt.Errorf("delete user %d's preferences: %w", userID, err)
	}
	return nil
}
