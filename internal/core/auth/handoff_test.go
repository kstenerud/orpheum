// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/proof"
)

// A loader's sign-in keeps its session for the client origin, which takes it by the code once; the
// code then works no more (D308 §4).
func TestHandOff_TheSessionIsTakenByItsCodeOnce(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "owner", "right", RoleAdmin)
	code, err := fixture.service.HandOff(ctx, "owner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatalf("HandOff: %v", err)
	}
	taken, err := fixture.service.TakeHandOff(ctx, code)
	if err != nil {
		t.Fatalf("TakeHandOff: %v", err)
	}
	if principal, err := fixture.service.Authenticate(ctx, taken.Token); err != nil || principal.UserID != id ||
		principal.DeviceID == 0 || taken.DeviceSecret == "" {
		t.Errorf("the session taken signs in %+v (%v), with a device secret %q; want the owner on a new device",
			principal, err, taken.DeviceSecret)
	}
	if _, err := fixture.service.TakeHandOff(ctx, code); !errors.Is(err, ErrNoHandOff) {
		t.Errorf("the code taken a second time = %v; want ErrNoHandOff", err)
	}
	if _, err := fixture.service.TakeHandOff(ctx, "never-made"); !errors.Is(err, ErrNoHandOff) {
		t.Errorf("a code never made = %v; want ErrNoHandOff", err)
	}
	if _, err := fixture.service.HandOff(ctx, "owner", "wrong", "192.0.2.1", laptop); !errors.Is(err,
		ErrBadCredentials) {
		t.Errorf("a hand-off with the wrong password = %v; want ErrBadCredentials", err)
	}
}

// A code not taken within its minute works no more, and its session, which no one ever held, ends.
func TestHandOff_ACodeNotTakenInItsMinuteEndsWithItsSession(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.createUser(t, "owner", "right", RoleAdmin)
	code, err := fixture.service.HandOff(ctx, "owner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	waiting := fixture.service.handOffs.byCode[code].issued
	fixture.clock.Advance(handOffLife + time.Second)
	if _, err := fixture.service.TakeHandOff(ctx, code); !errors.Is(err, ErrNoHandOff) {
		t.Errorf("the code past its minute = %v; want ErrNoHandOff", err)
	}
	if _, err := fixture.service.Authenticate(ctx, waiting.Token); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("the untaken session past its minute authenticates (%v); want it ended", err)
	}
}

// While its code is live, a hand-off's key proves offers for the person the realm names, so a
// client origin reached only over WebRTC can take its session; no other person's, and none once
// taken (D308 §4).
func TestHandOffOfferKeys_AreTheLiveCodesOfThePersonTheRealmNames(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.service.SetRealm("karlnas")
	fixture.createUser(t, "owner", "right", RoleAdmin)
	if keys := fixture.service.HandOffOfferKeys(proof.RealmPerson("karlnas", "owner")); len(keys) != 0 {
		t.Fatalf("before any hand-off the person's keys are %d; want none", len(keys))
	}
	code, err := fixture.service.HandOff(ctx, "Owner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	person := proof.RealmPerson("karlnas", "owner")
	keys := fixture.service.HandOffOfferKeys(person)
	if len(keys) != 1 || string(keys[0]) != string(proof.HandOffOfferKey(code)) {
		t.Errorf("the person's keys are %d; want the one hand-off's", len(keys))
	}
	if others := fixture.service.HandOffOfferKeys(proof.RealmPerson("karlnas", "partner")); len(others) != 0 {
		t.Errorf("another person's keys are %d; want none", len(others))
	}
	if _, err := fixture.service.TakeHandOff(ctx, code); err != nil {
		t.Fatal(err)
	}
	if keys := fixture.service.HandOffOfferKeys(person); len(keys) != 0 {
		t.Errorf("once taken the person's keys are %d; want none", len(keys))
	}
}
