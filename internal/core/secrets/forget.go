// SPDX-License-Identifier: AGPL-3.0-or-later

package secrets

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ForgetSecretsSetBy forgets who set the keys a removed person entered, which stay the server's
// (D305 §1). It writes through queries, the transaction that removes them (auth.RemoveUser).
func ForgetSecretsSetBy(ctx context.Context, queries *sqlcgen.Queries, userID int64) error {
	if err := queries.ClearSecretsSetByRemovedUser(ctx, &userID); err != nil {
		return fmt.Errorf("forget what user %d set: %w", userID, err)
	}
	return nil
}
