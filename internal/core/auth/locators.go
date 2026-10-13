// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/discovery/link"
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

// linkHashes are what a link's grant keeps of its secret: the hash a discovery locates the core by,
// and the key an offer made through the link is proven under. Neither gives the secret back.
func linkHashes(secret string) (locateHash, offerKey []byte, err error) {
	locateHash, err = hashBytes(link.LocateHash(secret))
	return locateHash, link.OfferKey(secret), err
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
	idleExpiry, _, err := service.sessionPolicy(ctx)
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
