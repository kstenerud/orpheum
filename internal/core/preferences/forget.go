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

// ForgetPreferencesOfPurgedFiles deletes what each person chose in removed files about to be
// deleted (D306 §8): their subtitle and audio choices and offsets were about those files alone.
// It is given the files' ids as a JSON array, and writes through queries, the transaction that
// deletes them (library.FilePurger.PurgeRemovedFiles).
func ForgetPreferencesOfPurgedFiles(ctx context.Context, queries *sqlcgen.Queries, fileIDsJSON string) error {
	if err := queries.DeletePreferencesOfPurgedFiles(ctx, fileIDsJSON); err != nil {
		return fmt.Errorf("delete the choices made in the files: %w", err)
	}
	return nil
}
