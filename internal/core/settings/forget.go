// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ForgetSettingsSetBy forgets who set the settings a removed person changed, which stay
// (D305 §1). It writes through queries, the transaction that removes them (auth.RemoveUser).
func ForgetSettingsSetBy(ctx context.Context, queries *sqlcgen.Queries, userID int64) error {
	if err := queries.ClearSettingsSetByRemovedUser(ctx, &userID); err != nil {
		return fmt.Errorf("forget what user %d set: %w", userID, err)
	}
	return nil
}
