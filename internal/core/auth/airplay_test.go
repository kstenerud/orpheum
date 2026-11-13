// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

// An AirPlay grant stands for its playback alone, for 24 hours and not a second more (D473 §2).
func TestAirPlayGrant_StandsForItsPlaybackForADayThenExpires(t *testing.T) {
	t.Parallel()
	setup := newFixture(t)
	sender, _ := guestSender(t, setup.service)
	grant, err := setup.service.IssueAirPlayGrant(sender, "media-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	setup.clock.Advance(AirPlayGrantLife - time.Second)
	kept, err := setup.service.AirPlayGrantOf(t.Context(), grant, tv)
	if err != nil {
		t.Fatalf("a second before its day ends: %v", err)
	}
	if kept.UserID != sender.UserID || kept.SessionKey != sender.SessionKey || kept.MediaToken != "media-token" ||
		kept.FileID != 7 {
		t.Errorf("the grant stands for %+v; want the sender's playback of file 7", kept)
	}
	setup.clock.Advance(time.Second)
	if _, err := setup.service.AirPlayGrantOf(t.Context(), grant, tv); !errors.Is(err, ErrNoAirPlayGrant) {
		t.Errorf("once its day is over: %v, want ErrNoAirPlayGrant", err)
	}
	if _, err := setup.service.AirPlayGrantOf(t.Context(), "not-a-grant", tv); !errors.Is(err, ErrNoAirPlayGrant) {
		t.Errorf("an unknown grant: %v, want ErrNoAirPlayGrant", err)
	}
}

// Stop revokes the sending session's AirPlay grants, and only its own: another session's stay,
// and the count says how many were revoked, for the page to show it was answered.
func TestAirPlayGrant_IsRevokedByItsSenderAlone(t *testing.T) {
	t.Parallel()
	setup := newFixture(t)
	sender, _ := guestSender(t, setup.service)
	grant, err := setup.service.IssueAirPlayGrant(sender, "media-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	other := anotherSession(t, setup.service)
	otherGrant, err := setup.service.IssueAirPlayGrant(other, "other-token", 8)
	if err != nil {
		t.Fatal(err)
	}
	if revoked := setup.service.RevokeAirPlayGrants(sender); revoked != 1 {
		t.Errorf("revoked %d; want the sender's one grant", revoked)
	}
	if _, err := setup.service.AirPlayGrantOf(t.Context(), grant, tv); !errors.Is(err, ErrNoAirPlayGrant) {
		t.Errorf("a revoked grant: %v, want ErrNoAirPlayGrant", err)
	}
	if _, err := setup.service.AirPlayGrantOf(t.Context(), otherGrant, tv); err != nil {
		t.Errorf("another session's grant, after the sender's Stop: %v; want it kept", err)
	}
	if revoked := setup.service.RevokeAirPlayGrants(sender); revoked != 0 {
		t.Errorf("a second Stop revoked %d; want none", revoked)
	}
}

// A guest receiver's session cannot AirPlay: it holds its cast's film alone (D465 §2).
func TestAirPlayGrant_RefusesAGuestReceiver(t *testing.T) {
	t.Parallel()
	setup := newFixture(t)
	sender, _ := guestSender(t, setup.service)
	guest := sender
	guest.isGuestReceiver = true
	if _, err := setup.service.IssueAirPlayGrant(guest, "media-token", 7); !errors.Is(err, ErrAirPlaySender) {
		t.Errorf("a guest receiver's grant: %v, want ErrAirPlaySender", err)
	}
}

// An AirPlay grant ends with its sender's session: signed out, or ended by a new password, the
// receiver holding the grant is refused, as the session's own media requests are; another session's
// grant stays.
func TestAirPlayGrant_EndsWithItsSendersSession(t *testing.T) {
	t.Parallel()
	setup := newFixture(t)
	ctx := t.Context()
	sender, issued := guestSender(t, setup.service)
	grant, err := setup.service.IssueAirPlayGrant(sender, "media-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	other := anotherSession(t, setup.service)
	otherGrant, err := setup.service.IssueAirPlayGrant(other, "other-token", 8)
	if err != nil {
		t.Fatal(err)
	}
	if err = setup.service.SignOut(ctx, issued.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, tv); !errors.Is(err, ErrNoAirPlayGrant) {
		t.Errorf("a grant of a session signed out: %v, want ErrNoAirPlayGrant", err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, otherGrant, tv); err != nil {
		t.Errorf("another session's grant, after the sender signed out: %v; want it kept", err)
	}
	if err = setup.service.ChangePassword(ctx, other.UserID, "pw", "a new password", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, otherGrant, tv); !errors.Is(err, ErrNoAirPlayGrant) {
		t.Errorf("a grant of a session a new password ended: %v, want ErrNoAirPlayGrant", err)
	}
}

// An AirPlay grant ends when its sender's device is revoked (D473 §2), as the device's sessions do.
func TestAirPlayGrant_EndsWhenItsSendersDeviceIsRevoked(t *testing.T) {
	t.Parallel()
	setup := newFixture(t)
	ctx := t.Context()
	sender, _ := guestSender(t, setup.service)
	if sender.DeviceID == 0 {
		t.Fatal("the sender's session has no device to revoke")
	}
	grant, err := setup.service.IssueAirPlayGrant(sender, "media-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, tv); err != nil {
		t.Fatalf("before the device is revoked: %v", err)
	}
	if err = setup.service.RemoveDevice(ctx, sender.UserID, sender.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, tv); !errors.Is(err, ErrNoAirPlayGrant) {
		t.Errorf("a grant of a revoked device's session: %v, want ErrNoAirPlayGrant", err)
	}
}

// A grant is bound to the first IPv4 address that presents it (D473 §3): another IPv4 address is
// answered as an unknown grant, the bound one still served, and an IPv4 address mapped into IPv6 is
// the address it maps.
func TestAirPlayGrant_IsBoundToTheFirstIPv4AddressThatPresentsIt(t *testing.T) {
	t.Parallel()
	setup := newFixture(t)
	ctx := t.Context()
	sender, _ := guestSender(t, setup.service)
	grant, err := setup.service.IssueAirPlayGrant(sender, "media-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, tv); err != nil {
		t.Fatalf("the first address: %v", err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, netip.MustParseAddr("192.0.2.31")); !errors.Is(err,
		ErrNoAirPlayGrant) {
		t.Errorf("another IPv4 address: %v, want ErrNoAirPlayGrant, as for a grant unknown", err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, netip.MustParseAddr("::ffff:192.0.2.30")); err != nil {
		t.Errorf("the bound address mapped into IPv6: %v; want it served", err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, tv); err != nil {
		t.Errorf("the bound address again, after another was refused: %v", err)
	}
}

// A grant is bound to the /64 of the first IPv6 address that presents it, beside its IPv4 address
// (D473 §3): an address within that /64 is served, one outside it refused as an unknown grant, and
// binding one family leaves the other's first address free to bind.
func TestAirPlayGrant_IsBoundToTheFirstIPv6AddresssSlash64(t *testing.T) {
	t.Parallel()
	setup := newFixture(t)
	ctx := t.Context()
	sender, _ := guestSender(t, setup.service)
	grant, err := setup.service.IssueAirPlayGrant(sender, "media-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, netip.MustParseAddr("2001:db8:1:2::10")); err != nil {
		t.Fatalf("the first IPv6 address: %v", err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, netip.MustParseAddr("2001:db8:1:2:aaaa::99")); err != nil {
		t.Errorf("another address in the same /64: %v; want it served", err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, netip.MustParseAddr("2001:db8:1:3::10")); !errors.Is(err,
		ErrNoAirPlayGrant) {
		t.Errorf("an address in another /64: %v, want ErrNoAirPlayGrant", err)
	}
	if _, err = setup.service.AirPlayGrantOf(ctx, grant, tv); err != nil {
		t.Errorf("the first IPv4 address, after IPv6 bound: %v; want it bound and served", err)
	}
}

// tv is the address of the AirPlay receiver presenting a grant.
var tv = netip.MustParseAddr("192.0.2.30")

// anotherSession is a second session of guestSender's person, on the same laptop.
func anotherSession(t *testing.T, service *Service) Principal {
	t.Helper()
	issued, err := service.SignIn(t.Context(), "owner", "pw", "192.0.2.10", laptop)
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Authenticate(t.Context(), issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	return other
}
