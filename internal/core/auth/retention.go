// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// PurgeEndedCredentials deletes each credential that ended, revoked or expired, more than
// endedCredentialsKeptDays ago, with the locators and invitations that name it, and says how
// many it deleted (D304 §6): every sign-in ever made was kept. One a credential derived from it
// still names goes once that one has. A sweep runs it daily.
func (service *Service) PurgeEndedCredentials(ctx context.Context) (int, error) {
	kept, err := service.settings.Duration(ctx, settings.EndedCredentialsKeptDays)
	if err != nil {
		return 0, fmt.Errorf("read endedCredentialsKeptDays: %w", err)
	}
	endedBefore := service.clock.Now().Add(-kept).UnixMilli()
	var deleted int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if locatorsErr := queries.DeleteLocatorsOfEndedGrants(ctx, &endedBefore); locatorsErr != nil {
			return fmt.Errorf("delete the locators of ended credentials: %w", locatorsErr)
		}
		if invitationsErr := queries.DeleteInvitationLinksOfEndedGrants(ctx, &endedBefore); invitationsErr != nil {
			return fmt.Errorf("delete the invitations' links of ended credentials: %w", invitationsErr)
		}
		if invitationsErr := queries.DeleteWaitingInvitationsWithNoLinks(ctx); invitationsErr != nil {
			return fmt.Errorf("delete the invitations whose every link ended: %w", invitationsErr)
		}
		var deleteErr error
		deleted, deleteErr = queries.DeleteEndedGrants(ctx, &endedBefore)
		if deleteErr != nil {
			return fmt.Errorf("delete ended credentials: %w", deleteErr)
		}
		return nil
	})
	return int(deleted), err
}
