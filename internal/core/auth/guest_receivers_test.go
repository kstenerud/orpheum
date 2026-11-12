// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"bytes"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// guestSender is an admin signed in on a laptop, who casts to Google Cast devices, and the session
// the laptop holds.
func guestSender(t *testing.T, service *Service) (Principal, Issued) {
	t.Helper()
	if _, err := service.CreateUser(t.Context(), NewUser{Name: "owner", Password: "pw", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	issued, err := service.SignIn(t.Context(), "owner", "pw", "192.0.2.10", laptop)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := service.Authenticate(t.Context(), issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	return sender, issued
}

// A guest grant is traded once (D465 §2): the trade signs a guest receiver in as the sender's person,
// on a device of its own named as the Cast device is, and a second trade of the same grant, as a
// listener on the network would try, is refused, as is one unknown.
func TestGuestGrant_IsTradedOnceForAGuestReceiversSession(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	sender, _ := guestSender(t, fixture.service)
	grant, err := fixture.service.IssueGuestGrant(sender, "Living room TV")
	if err != nil {
		t.Fatal(err)
	}
	traded, err := fixture.service.TradeGuestGrant(ctx, grant)
	if err != nil {
		t.Fatal(err)
	}
	if traded.UserID != sender.UserID || traded.SenderDeviceID != sender.DeviceID || traded.DeviceID == 0 ||
		traded.DeviceID == sender.DeviceID || traded.Locator == "" || traded.Name != "Living room TV" {
		t.Errorf("traded = user %d, sender %d, device %d; want the sender's person on a device of its own, "+
			"from the sender's device, with a locator", traded.UserID, traded.SenderDeviceID, traded.DeviceID)
	}
	receiver, err := fixture.service.Authenticate(ctx, traded.Token)
	if err != nil || receiver.UserID != sender.UserID || receiver.DeviceID != traded.DeviceID {
		t.Fatalf("the guest receiver is %+v, %v", receiver, err)
	}
	if _, err := fixture.service.TradeGuestGrant(ctx, grant); !errors.Is(err, ErrNoGuestGrant) {
		t.Errorf("a second trade: %v, want ErrNoGuestGrant", err)
	}
	if _, err := fixture.service.TradeGuestGrant(ctx, "not-a-grant"); !errors.Is(err, ErrNoGuestGrant) {
		t.Errorf("an unknown grant: %v, want ErrNoGuestGrant", err)
	}
}

// A guest grant waits a minute to be traded, and no longer (google-cast.md §2, step 1); a sending
// device keeps guestGrantsPerDevice waiting at most, a new one forgetting the oldest.
func TestGuestGrant_ExpiresAfterAMinuteAndASenderKeepsFewWaiting(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	sender, _ := guestSender(t, fixture.service)
	late, err := fixture.service.IssueGuestGrant(sender, "TV")
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(guestGrantLife - time.Second)
	inTime, err := fixture.service.IssueGuestGrant(sender, "TV")
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(time.Second)
	if _, err = fixture.service.TradeGuestGrant(ctx, late); !errors.Is(err, ErrNoGuestGrant) {
		t.Errorf("a grant traded a minute after it was made: %v, want ErrNoGuestGrant", err)
	}
	if _, err = fixture.service.TradeGuestGrant(ctx, inTime); err != nil {
		t.Errorf("a grant traded within its minute: %v", err)
	}
	first, err := fixture.service.IssueGuestGrant(sender, "TV")
	if err != nil {
		t.Fatal(err)
	}
	for range guestGrantsPerDevice {
		fixture.clock.Advance(time.Millisecond)
		if _, err := fixture.service.IssueGuestGrant(sender, "TV"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fixture.service.TradeGuestGrant(ctx, first); !errors.Is(err, ErrNoGuestGrant) {
		t.Errorf("the oldest of more grants than a device keeps: %v, want ErrNoGuestGrant", err)
	}
}

// A guest receiver's session carries nothing of its person's role (D465 §2): not administering, not
// hosting, not the account's security; it is known as a guest receiver, and cannot itself ask for a
// grant. Nor can a session of no device.
func TestGuestGrant_TheGuestReceiversSessionCarriesNoCapabilityAndSendsNoGrant(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	sender, _ := guestSender(t, fixture.service)
	if sender.IsGuestReceiver() {
		t.Fatal("the sender's own session is taken for a guest receiver's")
	}
	grant, err := fixture.service.IssueGuestGrant(sender, "TV")
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := fixture.service.TradeGuestGrant(ctx, grant)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := fixture.service.Authenticate(ctx, receiver.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !principal.IsGuestReceiver() || len(principal.Capabilities) != 0 || principal.IsAdmin() ||
		principal.CanManageAccount() || principal.CanHostWatchTogether() {
		t.Errorf("the guest receiver is %+v; want a guest receiver with no capability", principal)
	}
	if _, err := fixture.service.IssueGuestGrant(principal, "TV"); !errors.Is(err, ErrGuestSender) {
		t.Errorf("a guest receiver asking for a grant: %v, want ErrGuestSender", err)
	}
	noDevice := Principal{UserID: sender.UserID}
	if _, err := fixture.service.IssueGuestGrant(noDevice, "TV"); !errors.Is(err, ErrGuestSender) {
		t.Errorf("a session of no device asking for a grant: %v, want ErrGuestSender", err)
	}
}

// guestTraded is a guest receiver of sender's, its grant traded.
func guestTraded(t *testing.T, service *Service, sender Principal) GuestReceiver {
	t.Helper()
	grant, err := service.IssueGuestGrant(sender, "TV")
	if err != nil {
		t.Fatal(err)
	}
	traded, err := service.TradeGuestGrant(t.Context(), grant)
	if err != nil {
		t.Fatal(err)
	}
	return traded
}

// isSignedIn is whether token's session is live.
func isSignedIn(t *testing.T, service *Service, token string) bool {
	t.Helper()
	_, err := service.Authenticate(t.Context(), token)
	if err != nil && !errors.Is(err, ErrNotAuthenticated) {
		t.Fatal(err)
	}
	return err == nil
}

// A guest receiver is never one of its person's devices to list (D465 §1), and keeps nothing once
// removed: its session ends, and no other's.
func TestGuestGrant_AGuestReceiverIsUnlistedAndRemovedWithItsSession(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	sender, _ := guestSender(t, fixture.service)
	removed, kept := guestTraded(t, fixture.service, sender), guestTraded(t, fixture.service, sender)
	devices, err := fixture.service.Devices(ctx, sender.UserID)
	if err != nil || len(devices) != 1 || devices[0].ID != sender.DeviceID {
		t.Fatalf("the person's devices are %+v, %v; want the laptop alone", devices, err)
	}
	if err = fixture.service.RemoveGuestReceiver(ctx, sender.UserID, removed.DeviceID); err != nil {
		t.Fatal(err)
	}
	if isSignedIn(t, fixture.service, removed.Token) {
		t.Error("a removed guest receiver's session is still signed in")
	}
	if !isSignedIn(t, fixture.service, kept.Token) {
		t.Error("the other guest receiver's session, not removed, was signed out")
	}
}

// At start every guest receiver a stopped core left is removed, and no other device, nor one signed
// in since.
func TestGuestGrant_TheGuestReceiversAStoppedCoreLeftAreRemoved(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	sender, senderSession := guestSender(t, fixture.service)
	left, since := guestTraded(t, fixture.service, sender), guestTraded(t, fixture.service, sender)
	isLive := func(_, deviceID int64) bool { return deviceID == since.DeviceID }
	if err := fixture.service.RemoveEveryGuestReceiver(t.Context(), isLive); err != nil {
		t.Fatal(err)
	}
	if isSignedIn(t, fixture.service, left.Token) {
		t.Error("a guest receiver left at start is still signed in")
	}
	if !isSignedIn(t, fixture.service, since.Token) || !isSignedIn(t, fixture.service, senderSession.Token) {
		t.Error("a guest receiver signed in since the start, or the sender, was signed out")
	}
}

// A Google Cast device proves its first offer by its grant (google-cast.md §2, step 2): the grant's
// key holds under the locator of the session that asked for it while the grant waits, and under no
// other locator, nor once traded or expired.
func TestGuestGrant_ItsOfferKeyHoldsUnderItsSendersLocatorWhileItWaits(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	sender, senderSession := guestSender(t, fixture.service)
	underSender := link.LocatorHash(senderSession.Locator)
	keysUnder := func(hash string) [][]byte {
		t.Helper()
		keys, err := fixture.service.GuestGrantOfferKeys(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		return keys
	}
	traded, err := fixture.service.IssueGuestGrant(sender, "TV")
	if err != nil {
		t.Fatal(err)
	}
	expired, err := fixture.service.IssueGuestGrant(sender, "TV")
	if err != nil {
		t.Fatal(err)
	}
	holds := func(keys [][]byte, grant string) bool {
		want := proof.GuestGrantOfferKey(grant)
		return slices.ContainsFunc(keys, func(key []byte) bool { return bytes.Equal(key, want) })
	}
	keys := keysUnder(underSender)
	if len(keys) != 2 || !holds(keys, traded) || !holds(keys, expired) {
		t.Errorf("the keys under the sender's locator are %d; want both waiting grants'", len(keys))
	}
	if others := keysUnder(link.LocatorHash("another device's locator")); len(others) != 0 {
		t.Errorf("another locator has %d grant keys; want none", len(others))
	}
	if _, err = fixture.service.TradeGuestGrant(ctx, traded); err != nil {
		t.Fatal(err)
	}
	keys = keysUnder(underSender)
	if len(keys) != 1 || !holds(keys, expired) {
		t.Errorf("after one trade the keys are %d; want the other grant's alone", len(keys))
	}
	fixture.clock.Advance(guestGrantLife)
	if left := keysUnder(underSender); len(left) != 0 {
		t.Errorf("after the grant's minute %d keys are left; want none", len(left))
	}
}
