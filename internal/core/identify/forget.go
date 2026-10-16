// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ForgetOverridesSetBy forgets who set the overrides a removed person typed, which stay for everyone
// (D305 §1). It writes through queries, the transaction that removes them (auth.RemoveUser).
func ForgetOverridesSetBy(ctx context.Context, queries *sqlcgen.Queries, userID int64) error {
	if err := queries.ClearOverridesSetByRemovedUser(ctx, &userID); err != nil {
		return fmt.Errorf("forget what user %d set: %w", userID, err)
	}
	return nil
}
