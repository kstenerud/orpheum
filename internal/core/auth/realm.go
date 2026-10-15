// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrRealmRefused is a realm sign-in the core does not take: no live person is who it names, or its
// proof does not hold under their key. Which, it does not say (D297 §2).
var ErrRealmRefused = errors.New("the realm sign-in does not hold")

// realmLimitPrefix starts a person's key in the failed-sign-in limit for realm sign-ins, which name
// them by who they are at the discovery, not by their name.
const realmLimitPrefix = "realm:"

// SetRealm is the realm the core is found by at its discovery, empty for none, as the discovery's
// status last named it: a person's realm key is made for it (D297 §3).
func (service *Service) SetRealm(realm string) {
	service.realm.Store(proof.RealmName(realm))
}

// currentRealm is the realm the core is found by, or empty.
func (service *Service) currentRealm() string {
	realm, _ := service.realm.Load().(string) //nolint:errcheck // a realm never set is none
	return realm
}

// keepRealmKey makes and keeps the person's realm key from their password, for the master realm the
// core's realm names, when it has one. A key made already for this master realm is kept as it is
// unless isNewPassword: a sign-in proves the password the key was made from, so it makes one only
// where none serves this master realm, as after the core was first given a realm.
func (service *Service) keepRealmKey(
	ctx context.Context, userID int64, name, password string, isNewPassword bool,
) error {
	realm := service.currentRealm()
	if realm == "" {
		return nil
	}
	master := proof.MasterRealm(realm)
	if !isNewPassword {
		kept, err := store.OrNotFound(service.database.Read().GetRealmKeyMasterOfUser(ctx, userID))
		if err == nil && kept == master {
			return nil
		}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("look up person %d's realm key: %w", userID, err)
		}
	}
	var key []byte
	// As many rounds as a password's hash costs, so it waits for the same slots.
	if err := service.hashing.run(ctx, func() { key = proof.RealmKey(realm, name, password) }); err != nil {
		return err
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.SetRealmKey(ctx, sqlcgen.SetRealmKeyParams{
			UserID: userID, MasterRealm: master, Key: key, MadeAt: service.clock.Now().UnixMilli(),
		})
	})
}

// keepRealmKeyOfUser is keepRealmKey for a new password, the person found by their id.
func (service *Service) keepRealmKeyOfUser(ctx context.Context, userID int64, password string) error {
	if service.currentRealm() == "" {
		return nil
	}
	user, err := liveUser(ctx, service.database.Read(), userID)
	if err != nil {
		return err
	}
	return service.keepRealmKey(ctx, userID, user.Name, password, true)
}

// RealmKeyOfPerson is the key of the live person a realm sign-in names, by the whole realm the core
// has and their name, which their offers through it are proven under too; ErrRealmRefused when no one
// is. A household holds few people, so each is matched in turn.
func (service *Service) RealmKeyOfPerson(ctx context.Context, person string) ([]byte, error) {
	realm := service.currentRealm()
	if realm == "" {
		return nil, ErrRealmRefused
	}
	kept, err := service.database.Read().ListLiveRealmKeys(ctx, proof.MasterRealm(realm))
	if err != nil {
		return nil, fmt.Errorf("list the realm keys: %w", err)
	}
	for _, each := range kept {
		if proof.RealmPerson(realm, each.Name) == person {
			return each.Key, nil
		}
	}
	return nil, ErrRealmRefused
}

// TakeRealmSignIn takes a realm sign-in a discovery passed on, and is the person's key, which the
// core's answer is tagged under: ErrRealmRefused when its proof does not hold, as ErrRateLimited once
// the person has failed past the account-wide limit, which a guesser asking through the discovery is
// held to as one asking the core's own sign-in is (the third audit's A13).
func (service *Service) TakeRealmSignIn(ctx context.Context, person, ask string) ([]byte, error) {
	maxFailures, window, err := service.failurePolicy(ctx)
	if err != nil {
		return nil, err
	}
	limitKey := realmLimitPrefix + person
	attemptedAt, isAllowed, _ := service.limiter.attempt(window, map[string]int64{
		limitKey: maxFailures * accountEverywhereTimes,
	}, nil)
	if !isAllowed {
		return nil, ErrRateLimited
	}
	key, err := service.RealmKeyOfPerson(ctx, person)
	if err != nil {
		return nil, err
	}
	if !proof.VerifyRealmAsk(key, service.currentRealm(), person, ask) {
		return nil, ErrRealmRefused
	}
	service.limiter.withdraw(attemptedAt, limitKey)
	return key, nil
}

// ForgetRealmKeys forgets every person's realm key: the core left its discovery, whose realm sign-in
// was all they served; the people sign in by their passwords as before (D298 §6).
func (service *Service) ForgetRealmKeys(ctx context.Context) error {
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.DeleteRealmKeysMadeBy(ctx, service.clock.Now().UnixMilli())
	})
}
