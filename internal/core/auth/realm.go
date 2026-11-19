// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/discovery/proof"
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

// Realm is the realm the core is found by, or empty: the household's name, which a problem report
// hides (D524 §4).
func (service *Service) Realm() string {
	realm, _ := service.realm.Load().(string) //nolint:errcheck // a realm never set is none
	return realm
}

// SetKeySalt is the core's key salt (identity.LoadOrMakeKeySalt), which each person's version-2 key
// is salted by (D527 §2.2). Until it is set no key is made: every command that sets a password sets
// it first.
func (service *Service) SetKeySalt(keySalt []byte) {
	kept := slices.Clone(keySalt)
	service.keySalt.Store(&kept)
}

// keySaltOf is the key salt SetKeySalt set, or nil.
func (service *Service) keySaltOf() []byte {
	if kept := service.keySalt.Load(); kept != nil {
		return *kept
	}
	return nil
}

// keepRealmKey makes and keeps the person's away-from-home key from their password: the version 2
// (proof.PersonKey), salted by the core's key salt, so it is made whether or not the core is linked
// to a discovery, and the household's name given or changed later leaves it working; making it
// deletes the person's version-1 key, which the core kept for them only until it saw their password
// again (D527 §2.2). A key made already is kept as it is unless isNewPassword: a sign-in proves the
// password it was made from, so it makes one only where the person has none.
func (service *Service) keepRealmKey(
	ctx context.Context, userID int64, name, password string, isNewPassword bool,
) error {
	keySalt := service.keySaltOf()
	if keySalt == nil {
		return nil
	}
	if !isNewPassword {
		has, err := service.HasPersonKey(ctx, userID)
		if err != nil || has {
			return err
		}
	}
	var key []byte
	// As many rounds as a password's hash costs, so it waits for the same slots.
	if err := service.hashing.run(ctx, func() { key = proof.PersonKey(keySalt, name, password) }); err != nil {
		return err
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if err := queries.SetPersonKey(ctx, sqlcgen.SetPersonKeyParams{
			UserID: userID, Key: key, MadeAt: service.clock.Now().UnixMilli(),
		}); err != nil {
			return fmt.Errorf("keep person %d's key: %w", userID, err)
		}
		if err := queries.DeleteRealmKeyOfUser(ctx, userID); err != nil {
			return fmt.Errorf("delete person %d's version-1 key: %w", userID, err)
		}
		return nil
	})
}

// keepRealmKeyOfUser is keepRealmKey for a new password, the person found by their id.
func (service *Service) keepRealmKeyOfUser(ctx context.Context, userID int64, password string) error {
	if service.keySaltOf() == nil {
		return nil
	}
	user, err := liveUser(ctx, service.database.Read(), userID)
	if err != nil {
		return err
	}
	return service.keepRealmKey(ctx, userID, user.Name, password, true)
}

// HasPersonKey is whether the core holds the person's version-2 key: whether linking needs their
// password, which only one who has not signed in since the core first made such keys must give (D527
// §2.2).
func (service *Service) HasPersonKey(ctx context.Context, userID int64) (bool, error) {
	has, err := service.database.Read().HasPersonKey(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("look up person %d's key: %w", userID, err)
	}
	return has, nil
}

// CheckPassword is nil when password is the live person's, and ErrWrongPassword when it is not.
func (service *Service) CheckPassword(ctx context.Context, userID int64, password string) error {
	user, err := liveUser(ctx, service.database.Read(), userID)
	if err != nil {
		return err
	}
	isMatch, _, verifyErr := service.verifyPassword(ctx, password, string(user.PasswordHash))
	if verifyErr != nil || !isMatch {
		return errors.Join(ErrWrongPassword, verifyErr)
	}
	return nil
}

// MakeRealmKey makes the person's away-from-home key from their password, checked by CheckPassword
// (D527 §2.2): an admin linking the core who has not signed in with their password since the core
// first made such keys, and so has none yet.
func (service *Service) MakeRealmKey(ctx context.Context, userID int64, password string) error {
	return service.keepRealmKeyOfUser(ctx, userID, password)
}

// RealmKeyOfPerson is the key of the live person a realm sign-in names, by the whole realm the core
// has and their name, which their offers through it are proven under too; ErrRealmRefused when no one
// is. Their version-2 key comes first, and their version-1 key, kept until the core sees their password
// again, after it (D527 §2.2).
func (service *Service) RealmKeyOfPerson(ctx context.Context, person string) ([]byte, error) {
	keys, err := service.realmKeysOfPerson(ctx, person)
	if err != nil {
		return nil, err
	}
	return keys[0], nil
}

// realmKeysOfPerson are the keys of the live person a realm sign-in names, version 2 first and then
// version 1, or ErrRealmRefused when no one is. A household holds few people, so each is matched in
// turn.
func (service *Service) realmKeysOfPerson(ctx context.Context, person string) ([][]byte, error) {
	realm := service.Realm()
	if realm == "" {
		return nil, ErrRealmRefused
	}
	reader := service.database.Read()
	personKeys, err := reader.ListLivePersonKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the person keys: %w", err)
	}
	var keys [][]byte
	for _, each := range personKeys {
		if proof.RealmPerson(realm, each.Name) == person {
			keys = append(keys, each.Key)
		}
	}
	realmKeys, err := reader.ListLiveRealmKeys(ctx, proof.MasterRealm(realm))
	if err != nil {
		return nil, fmt.Errorf("list the realm keys: %w", err)
	}
	for _, each := range realmKeys {
		if proof.RealmPerson(realm, each.Name) == person {
			keys = append(keys, each.Key)
		}
	}
	if len(keys) == 0 {
		return nil, ErrRealmRefused
	}
	return keys, nil
}

// TakeRealmSignIn takes a realm sign-in a discovery passed on, and is the person's key its proof holds
// under, version 2 or 1, which the core's answer is tagged under: ErrRealmRefused when it holds under
// neither, the one answer whichever was wrong (D297 §2; D527 §2.2), as ErrRateLimited once
// the person has failed past the account-wide limit, which a guesser asking through the discovery is
// held to as one asking the core's own sign-in is (the third audit's A13).
func (service *Service) TakeRealmSignIn(ctx context.Context, person, ask string) ([]byte, error) {
	maxFailures, window, err := service.failurePolicy(ctx)
	if err != nil {
		return nil, err
	}
	stretch, err := service.failureStretchOf(ctx)
	if err != nil {
		return nil, err
	}
	limitKey := realmLimitPrefix + person
	attemptedAt, isAllowed, _ := service.limiter.attempt(window, map[string]int64{
		limitKey: maxFailures * stretch.everywhere,
	}, nil)
	if !isAllowed {
		return nil, ErrRateLimited
	}
	keys, err := service.realmKeysOfPerson(ctx, person)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		if proof.VerifyRealmAsk(key, service.Realm(), person, ask) {
			service.limiter.withdraw(attemptedAt, limitKey)
			return key, nil
		}
	}
	return nil, ErrRealmRefused
}
