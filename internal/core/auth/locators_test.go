// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// liveLinkHashes are the locate hashes LiveLinks gives, with each one's life from now.
func liveLinkHashes(t *testing.T, service *Service, now time.Time) map[string]time.Duration {
	t.Helper()
	links, err := service.LiveLinks(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	lives := map[string]time.Duration{}
	for _, live := range links {
		lives[live.LocateHash] = live.ExpiresAt.Sub(now)
	}
	return lives
}

func TestLiveLinks_AreEveryLiveLinksHashAndAUsedInvitationLeadsItsPersonBack(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	admin := fixture.createUser(t, "owner", "right", RoleAdmin)
	_, invitation, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: "Grandma"})
	if err != nil {
		t.Fatal(err)
	}
	lives := liveLinkHashes(t, fixture.service, fixture.clock.Now())
	if life, isLive := lives[link.LocateHash(invitation.Secret)]; !isLive || life <= 0 {
		t.Fatalf("the live links are %v; want the invitation's locate hash among them, alive", lives)
	}
	for hash := range lives {
		if strings.Contains(hash, invitation.Secret) {
			t.Errorf("a live link's hash %q holds the invitation's secret", hash)
		}
	}
	issued, err := fixture.service.Register(ctx, invitation.Secret, "Grandma", "a password", NewDevice{Name: "phone"})
	if err != nil {
		t.Fatal(err)
	}
	lives = liveLinkHashes(t, fixture.service, fixture.clock.Now())
	if life := lives[link.LocateHash(invitation.Secret)]; life != ownLinkLife {
		t.Errorf("once used, the invitation's link lives %s; want it kept as her own link for %s", life, ownLinkLife)
	}
	_, err = fixture.service.Register(ctx, invitation.Secret, "Grandma2", "x", NewDevice{})
	if !errors.Is(err, ErrBadInvitation) {
		t.Errorf("registering again through the used link = %v; want it refused: it only locates", err)
	}
	principal, err := fixture.service.Authenticate(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.RemoveUser(ctx, admin, principal.UserID); err != nil {
		t.Fatal(err)
	}
	lives = liveLinkHashes(t, fixture.service, fixture.clock.Now())
	if _, isLive := lives[link.LocateHash(invitation.Secret)]; isLive {
		t.Error("removing the person left their own link locating")
	}
}

func TestRegister_ARegistrationLinkBecomesItsPersonsOwnLinkAndRevokesTheLast(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "anna", "first", RoleUser)
	first, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Register(ctx, first.Secret, "", "second", NewDevice{}); err != nil {
		t.Fatal(err)
	}
	second, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	lives := liveLinkHashes(t, fixture.service, fixture.clock.Now())
	if _, isLive := lives[link.LocateHash(first.Secret)]; !isLive {
		t.Error("the used registration link stopped locating; want it her own link")
	}
	if _, err = fixture.service.Register(ctx, second.Secret, "", "third", NewDevice{}); err != nil {
		t.Fatal(err)
	}
	lives = liveLinkHashes(t, fixture.service, fixture.clock.Now())
	if _, isLive := lives[link.LocateHash(first.Secret)]; isLive {
		t.Error("a new password from a new link left her old own link locating")
	}
	if _, isLive := lives[link.LocateHash(second.Secret)]; !isLive {
		t.Error("the new link, used, does not locate; want it her own link now")
	}
}

func TestLiveLocators_EachSessionHasOneFreshOnEachVisitEndingWithTheSession(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "owner", "right", RoleAdmin)
	issued, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", NewDevice{Name: "laptop"})
	if err != nil {
		t.Fatal(err)
	}
	locators := func() []string {
		t.Helper()
		hashes, listErr := fixture.service.LiveLocators(ctx)
		if listErr != nil {
			t.Fatal(listErr)
		}
		return hashes
	}
	if got := locators(); !slices.Equal(got, []string{link.LocatorHash(issued.Locator)}) || issued.Locator == "" {
		t.Errorf("after signing in the live locators are %v; want the session's own, by its hash", got)
	}
	principal, err := fixture.service.Authenticate(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := fixture.service.FreshLocator(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if got := locators(); fresh == issued.Locator || !slices.Equal(got, []string{link.LocatorHash(fresh)}) {
		t.Errorf("after a visit the live locators are %v; want the fresh one alone in place of the last", got)
	}
	if err = fixture.service.SignOut(ctx, issued.Token); err != nil {
		t.Fatal(err)
	}
	if got := locators(); len(got) != 0 {
		t.Errorf("after signing out the live locators are %v; want none", got)
	}
}

func TestFreshLocator_RenewsThePersonsOwnLink(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "anna", "first", RoleUser)
	registration, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := fixture.service.Register(ctx, registration.Secret, "", "second", NewDevice{})
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(30 * 24 * time.Hour)
	principal, err := fixture.service.Authenticate(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.FreshLocator(ctx, principal); err != nil {
		t.Fatal(err)
	}
	lives := liveLinkHashes(t, fixture.service, fixture.clock.Now())
	if life := lives[link.LocateHash(registration.Secret)]; life != ownLinkLife {
		t.Errorf("after a visit 30 days on, her own link lives %s; want a whole %s again", life, ownLinkLife)
	}
}

func TestOfferKey_IsALiveSessionsHashByItsLocatorOrALiveLinksKeyByItsHash(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "owner", "right", RoleAdmin)
	issued, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", NewDevice{Name: "laptop"})
	if err != nil {
		t.Fatal(err)
	}
	_, invitation, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: "Grandma"})
	if err != nil {
		t.Fatal(err)
	}
	if key, keyErr := fixture.service.OfferKey(ctx, link.LocatorHash(issued.Locator)); keyErr != nil ||
		string(key) != string(proof.SessionOfferKey(issued.Token)) {
		t.Errorf("the locator's key = %x, %v; want its session's, SHA-256 of the token the client proves with",
			key, keyErr)
	}
	if key, keyErr := fixture.service.OfferKey(ctx, link.LocateHash(invitation.Secret)); keyErr != nil ||
		string(key) != string(link.OfferKey(invitation.Secret)) {
		t.Errorf("the link's key = %x, %v; want link.OfferKey of its secret", key, keyErr)
	}
	_, keyErr := fixture.service.OfferKey(ctx, link.LocatorHash("never handed out"))
	if !errors.Is(keyErr, ErrNoOfferKey) {
		t.Errorf("an unknown hash's key = %v; want ErrNoOfferKey", keyErr)
	}
	if err = fixture.service.SignOut(ctx, issued.Token); err != nil {
		t.Fatal(err)
	}
	if _, keyErr := fixture.service.OfferKey(ctx, link.LocatorHash(issued.Locator)); !errors.Is(keyErr, ErrNoOfferKey) {
		t.Errorf("a signed-out session's locator's key = %v; want ErrNoOfferKey", keyErr)
	}
}
