// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// What a discovery finds the core by (discovery-core.md K7; the discovery protocol's §3.1): each
// link's secret registered by its hash, a used registration link kept as its person's own link, and
// each session's device locator. None of it lets the discovery sign anyone in.
const (
	grantKindLink = "link"
	grantScopeOwn = "own"
)

// ownLinkLife is how long a person's own link lives without a visit: each signed-in visit renews it
// (the third audit's A12; the number is the protocol's, marked [I]).
const ownLinkLife = 90 * 24 * time.Hour

// LinkSealer seals a link's secret for its grant's row, and opens it there alone
// (secrets.Store's SealBound and OpenBound).
type LinkSealer interface {
	SealBound(identity, text string) ([]byte, error)
	OpenBound(identity string, sealed []byte) (string, error)
}

// SealLinksWith has every link made from now on keep its secret sealed by sealer, so that the
// person whose own link it becomes can be shown it again (OwnLink; discovery-core.md K7). Called
// before the service is used.
func (service *Service) SealLinksWith(sealer LinkSealer) {
	service.linkSealer = sealer
}

// keptOfLink is what a link's grant keeps of its secret: the hash a discovery locates the core by,
// the key an offer made through the link is proven under, neither of which gives the secret back,
// and the secret sealed for the grant's row, nil without a sealer.
type keptOfLink struct {
	locateHash, offerKey, sealed []byte
}

func (service *Service) keepOfLink(secret string) (keptOfLink, error) {
	locateHash, err := hashBytes(link.LocateHash(secret))
	if err != nil {
		return keptOfLink{}, err
	}
	kept := keptOfLink{locateHash: locateHash, offerKey: link.OfferKey(secret)}
	if service.linkSealer != nil {
		kept.sealed, err = service.linkSealer.SealBound(grantIdentity(tokenHash(secret)), secret)
		if err != nil {
			return keptOfLink{}, fmt.Errorf("seal a link: %w", err)
		}
	}
	return kept, nil
}

// grantIdentity is a grant row's identity, which its sealed link is bound to.
func grantIdentity(secretHash []byte) string {
	return "grant " + hex.EncodeToString(secretHash)
}

// hashBytes is a base64url hash as the database keeps it, raw.
func hashBytes(encoded string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("a link's hash is not base64url: %w", err)
	}
	return raw, nil
}

// LiveLink is a link a discovery may find the core by: its secret's hash, and when it ends.
type LiveLink struct {
	LocateHash string
	ExpiresAt  time.Time
}

// LiveLinks are every live link's locate hash: unused invitations and registration links, and
// people's own links.
func (service *Service) LiveLinks(ctx context.Context) ([]LiveLink, error) {
	rows, err := service.database.Read().ListLiveLinkHashes(ctx, new(service.clock.Now().UnixMilli()))
	if err != nil {
		return nil, fmt.Errorf("list the live links: %w", err)
	}
	links := make([]LiveLink, 0, len(rows))
	for _, row := range rows {
		live := LiveLink{LocateHash: base64.RawURLEncoding.EncodeToString(row.LocateHash)}
		if row.ExpiresAt != nil {
			live.ExpiresAt = time.UnixMilli(*row.ExpiresAt)
		}
		links = append(links, live)
	}
	return links, nil
}

// LiveLocators are every live session's device locator, as their hashes (link.LocatorHash).
func (service *Service) LiveLocators(ctx context.Context) ([]string, error) {
	idleExpiry, err := service.sessionLocatorExpiry(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := service.database.Read().ListLiveLocators(ctx, new(service.clock.Now().Add(-idleExpiry).UnixMilli()))
	if err != nil {
		return nil, fmt.Errorf("list the live locators: %w", err)
	}
	hashes := make([]string, 0, len(rows))
	for _, hash := range rows {
		hashes = append(hashes, base64.RawURLEncoding.EncodeToString(hash))
	}
	return hashes, nil
}

// FreshLocator hands the signed-in device a new locator in place of its session's last, and renews
// the person's own link, which a visit keeps alive (the protocol's §3.1).
func (service *Service) FreshLocator(ctx context.Context, principal Principal) (string, error) {
	session, err := hex.DecodeString(principal.SessionKey)
	if err != nil || len(session) == 0 {
		return "", ErrNotAuthenticated
	}
	locator, err := randomSecret(service.random)
	if err != nil {
		return "", err
	}
	now := service.clock.Now()
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if setErr := setLocator(ctx, queries, session, locator, now.UnixMilli()); setErr != nil {
			return setErr
		}
		return queries.RenewOwnLinksOfUser(ctx, sqlcgen.RenewOwnLinksOfUserParams{
			ExpiresAt: new(now.Add(ownLinkLife).UnixMilli()), UserID: &principal.UserID, Now: new(now.UnixMilli()),
		})
	})
	if err != nil {
		return "", fmt.Errorf("hand the device a locator: %w", err)
	}
	service.linksChanged()
	return locator, nil
}

// setLocator makes locator the session's one locator.
func setLocator(ctx context.Context, queries *sqlcgen.Queries, session []byte, locator string, now int64) error {
	hash, err := hashBytes(link.LocatorHash(locator))
	if err != nil {
		return err
	}
	if err = queries.SetLocator(ctx, sqlcgen.SetLocatorParams{
		SessionSecretSha256: session, Hash: hash, IssuedAt: now,
	}); err != nil {
		return fmt.Errorf("record the session's locator: %w", err)
	}
	return nil
}

// becomeOwnLink makes a used link its person's own link, revoking the one they had, so the link
// that brought them leads them back (D292 §4; A12); a link made before the core kept its hashes is
// spent, as every link was.
func becomeOwnLink(
	ctx context.Context, queries *sqlcgen.Queries, secretHash, locateHash []byte, userID int64, now time.Time,
) error {
	usedAt := now.UnixMilli()
	if locateHash == nil {
		return queries.MarkGrantRevoked(ctx, sqlcgen.MarkGrantRevokedParams{
			SecretSha256: secretHash, RevokedAt: &usedAt,
		})
	}
	if err := queries.RevokeLiveGrantsOfUser(ctx, sqlcgen.RevokeLiveGrantsOfUserParams{
		RevokedAt: &usedAt, UserID: &userID, Kind: grantKindLink, Scope: grantScopeOwn,
	}); err != nil {
		return fmt.Errorf("revoke the person's earlier link: %w", err)
	}
	return queries.BecomeOwnLink(ctx, sqlcgen.BecomeOwnLinkParams{
		UserID: &userID, ExpiresAt: new(now.Add(ownLinkLife).UnixMilli()), Now: &usedAt, SecretSha256: secretHash,
	})
}

// OwnLink is a person's own link: when it stops locating the core unless a visit renews it, and its
// secret, empty for a link made before the core kept it or one that no longer opens, as after a
// restore without secret.key.
type OwnLink struct {
	Secret    string
	ExpiresAt time.Time
}

// ErrNoOwnLink is a person with no own link: none used, or it ended.
var ErrNoOwnLink = errors.New("you have no link of your own")

// OwnLink is the person's own link; false when they have none.
func (service *Service) OwnLink(ctx context.Context, userID int64) (OwnLink, bool, error) {
	row, err := store.OrNotFound(service.database.Read().GetOwnLinkOfUser(ctx,
		sqlcgen.GetOwnLinkOfUserParams{UserID: &userID, Now: new(service.clock.Now().UnixMilli())}))
	switch {
	case errors.Is(err, store.ErrNotFound), err == nil && row.ExpiresAt == nil:
		return OwnLink{}, false, nil
	case err != nil:
		return OwnLink{}, false, fmt.Errorf("read the person's own link: %w", err)
	}
	return OwnLink{
		ExpiresAt: time.UnixMilli(*row.ExpiresAt), Secret: service.openLink(row.SecretSha256, row.LinkSealed),
	}, true, nil
}

// openLink is a link's secret, opened from its grant's sealed copy; empty without a sealer, for a
// link made before the core kept it, or for one that does not open, as after a restore without
// secret.key: the link is not shown, and a new one is asked for.
func (service *Service) openLink(secretHash, sealed []byte) string {
	if service.linkSealer == nil || sealed == nil {
		return ""
	}
	secret, err := service.linkSealer.OpenBound(grantIdentity(secretHash), sealed)
	if err != nil {
		return ""
	}
	return secret
}

// RevokeOwnLink is the admin ending a person's own link: it locates the core no more, and the
// discovery is told at once (D292 §4; A12). Their sessions and devices are untouched.
func (service *Service) RevokeOwnLink(ctx context.Context, userID int64) error {
	if _, err := liveUser(ctx, service.database.Read(), userID); err != nil {
		return err
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.RevokeLiveGrantsOfUser(ctx, sqlcgen.RevokeLiveGrantsOfUserParams{
			RevokedAt: new(service.clock.Now().UnixMilli()), UserID: &userID, Kind: grantKindLink, Scope: grantScopeOwn,
		})
	})
	if err != nil {
		return fmt.Errorf("revoke the person's own link: %w", err)
	}
	service.linksChanged()
	return nil
}

// linksChanged tells whoever registers the core's links with its discovery that they changed.
func (service *Service) linksChanged() {
	if service.onLinksChanged != nil {
		service.onLinksChanged()
	}
}

// OnLinksChanged has changed called whenever a link or a locator is made, so the discovery learns it
// at once; what ends one is found by the discovery's next look.
func (service *Service) OnLinksChanged(changed func()) {
	service.onLinksChanged = changed
}

// ErrNoOfferKey is a hash no live locator or link has: an offer under it proves nothing.
var ErrNoOfferKey = errors.New("no live locator or link has this hash")

// OfferKey is what an offer a discovery passed on under hash is proven under: for a device's locator,
// its live session's hash, proof.SessionOfferKey of its token; for a link's bootstrap, the live link's
// offer key, link.OfferKey of its secret; for a person a realm sign-in found the core for, their realm
// key (the protocol's §2.5 item 3; D297 §3).
func (service *Service) OfferKey(ctx context.Context, hash string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(hash)
	if err != nil {
		return nil, ErrNoOfferKey
	}
	idleExpiry, err := service.sessionLocatorExpiry(ctx)
	if err != nil {
		return nil, err
	}
	now := service.clock.Now()
	session, err := store.OrNotFound(service.database.Read().GetLiveSessionOfLocator(ctx,
		sqlcgen.GetLiveSessionOfLocatorParams{Hash: raw, IdleSince: new(now.Add(-idleExpiry).UnixMilli())}))
	switch {
	case err == nil:
		return session, nil
	case !errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("look up a locator: %w", err)
	}
	key, err := store.OrNotFound(service.database.Read().GetLiveLinkOfferKey(ctx,
		sqlcgen.GetLiveLinkOfferKeyParams{LocateHash: raw, Now: new(now.UnixMilli())}))
	switch {
	case errors.Is(err, store.ErrNotFound):
		// A person a realm sign-in found the core for: their offers are proven by their realm key.
		if realmKey, realmErr := service.RealmKeyOfPerson(ctx, hash); realmErr == nil {
			return realmKey, nil
		} else if !errors.Is(realmErr, ErrRealmRefused) {
			return nil, realmErr
		}
		return nil, ErrNoOfferKey
	case err != nil:
		return nil, fmt.Errorf("look up a link: %w", err)
	}
	return key, nil
}
