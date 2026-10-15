// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// AdminReset is what ResetAdmin did: who lost administering, and whether the account was made.
type AdminReset struct {
	Demoted   []string
	IsCreated bool
}

// ResetAdmin is the operator's way back in, from the server's own machine (`orpheum reset-admin`,
// D294): every person whose role administers is given the user role, and the person named is given
// the admin role and password, made if no one has the name. Their sessions end, so a session someone
// else held does not carry the new rights. No account is removed.
func (service *Service) ResetAdmin(ctx context.Context, name, password string) (AdminReset, error) {
	name = textindex.Line(name)
	if name == "" || password == "" {
		return AdminReset{}, errors.New("the admin needs a name and a password")
	}
	hash, err := service.hashWithCurrentCosts(ctx, password)
	if err != nil {
		return AdminReset{}, err
	}
	var reset AdminReset
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		demoted, demoteErr := queries.DemoteAdministrators(ctx)
		if demoteErr != nil {
			return fmt.Errorf("demote the administrators: %w", demoteErr)
		}
		reset.Demoted = demoted
		admin, roleErr := queries.GetRoleByName(ctx, string(RoleAdmin))
		if roleErr != nil {
			return fmt.Errorf("find the admin role: %w", roleErr)
		}
		user, lookErr := store.OrNotFound(queries.GetLiveUserByName(ctx, name))
		if errors.Is(lookErr, store.ErrNotFound) {
			reset.IsCreated = true
			_, insertErr := queries.InsertUser(ctx, sqlcgen.InsertUserParams{
				Name: name, RoleID: admin.ID, PasswordHash: hash, CreatedAt: service.clock.Now().UnixMilli(),
			})
			return insertErr
		}
		if lookErr != nil {
			return fmt.Errorf("find %q: %w", name, lookErr)
		}
		return promote(ctx, queries, user.ID, admin.ID, hash, service.clock.Now().UnixMilli())
	})
	if err != nil {
		return AdminReset{}, err
	}
	if service.currentRealm() != "" {
		user, lookErr := service.database.Read().GetLiveUserByName(ctx, name)
		if lookErr != nil {
			return AdminReset{}, fmt.Errorf("find %q: %w", name, lookErr)
		}
		if keepErr := service.keepRealmKey(ctx, user.ID, user.Name, password, true); keepErr != nil {
			return AdminReset{}, keepErr
		}
	}
	return reset, nil
}

// promote gives a person the admin role and a new password, and ends their sessions.
func promote(ctx context.Context, queries *sqlcgen.Queries, userID, adminRoleID int64, hash []byte, now int64) error {
	if err := queries.UpdateUserRole(ctx, sqlcgen.UpdateUserRoleParams{ID: userID, RoleID: adminRoleID}); err != nil {
		return fmt.Errorf("make person %d the admin: %w", userID, err)
	}
	if err := queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
		ID: userID, PasswordHash: hash,
	}); err != nil {
		return fmt.Errorf("set person %d's password: %w", userID, err)
	}
	return queries.RevokeSessionsOfUserExceptSession(ctx, sqlcgen.RevokeSessionsOfUserExceptSessionParams{
		RevokedAt: &now, UserID: &userID,
	})
}
