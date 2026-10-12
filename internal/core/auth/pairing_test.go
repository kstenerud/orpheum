// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// pairDevice activates a device by its code, entered by byUserID for forUserID under name, the
// check codes confirmed, and returns its session; previousDeviceID is the device it was before.
func pairDevice(
	t *testing.T, service *Service, people [2]int64, name string, previousDeviceID int64,
) string {
	t.Helper()
	ctx := t.Context()
	asked, err := service.RequestPairing(ctx, "", "web", previousDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ClaimPairing(ctx, people[0], people[1], asked.Code, name); err != nil {
		t.Fatal(err)
	}
	if err = service.AnswerPairing(ctx, people[0], asked.Code, true); err != nil {
		t.Fatal(err)
	}
	token, err := service.TakePairedSession(ctx, asked.Secret)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// A device not signed in is given a code, the same one each time it asks while no one has entered
// it; the person entering it, however typed, gets a check code the device shows too, and no one can
// enter it again (D288 §1–§3).
func TestPairing_ADeviceKeepsItsCodeUntilEnteredAndThenShowsThePersonsCheckCode(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	asked, err := fixture.service.RequestPairing(ctx, "", "web", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked.Code) != 9 || asked.Code[4] != '-' || asked.ExpiresAt.Sub(fixture.clock.Now()) != 10*time.Minute {
		t.Errorf("the code = %+v, want xxxx-xxxx for ten minutes (D218 §3)", asked)
	}
	again, err := fixture.service.RequestPairing(ctx, asked.Secret, "web", 0)
	if err != nil || again != asked {
		t.Errorf("asking again = %+v, %v; want the same code (D288 §1)", again, err)
	}
	if state := fixture.service.PairingStateOf(ctx, asked.Secret); state.State != PairingWaiting {
		t.Errorf("before it is entered: %+v, want waiting", state)
	}
	typed := strings.ToUpper(strings.ReplaceAll(asked.Code, "-", " "))
	claim, err := fixture.service.ClaimPairing(ctx, id, id, typed, "Living room TV")
	if err != nil {
		t.Fatalf("entering %q: %v", typed, err)
	}
	state := fixture.service.PairingStateOf(ctx, asked.Secret)
	if state.State != PairingChecking || state.CheckCode != claim.CheckCode || len(claim.CheckCode) != 9 {
		t.Errorf("once entered: %+v, the person's check code %q; want both the same, xxxx-xxxx", state, claim.CheckCode)
	}
	if _, err = fixture.service.ClaimPairing(ctx, id, id, asked.Code, "Another"); !errors.Is(err, ErrNoPairing) {
		t.Errorf("a code entered twice: %v, want ErrNoPairing", err)
	}
}

// Only the person's match activates a device, signed in as them on a new device under the name they
// gave, and its session is taken once (D288 §3, §4).
func TestPairing_OnlyTheMatchSignsTheDeviceInAndItsSessionIsTakenOnce(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	asked, err := fixture.service.RequestPairing(ctx, "", "web", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.ClaimPairing(ctx, id, id, asked.Code, "Living room TV"); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.TakePairedSession(ctx, asked.Secret); !errors.Is(err, ErrNoPairing) {
		t.Errorf("a session before the person confirmed: %v, want none", err)
	}
	if err = fixture.service.AnswerPairing(ctx, id, asked.Code, true); err != nil {
		t.Fatal(err)
	}
	if state := fixture.service.PairingStateOf(ctx, asked.Secret); state.State != PairingActivated {
		t.Errorf("once confirmed: %+v, want activated", state)
	}
	token, err := fixture.service.TakePairedSession(ctx, asked.Secret)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := fixture.service.Authenticate(ctx, token)
	if err != nil || principal.UserID != id || principal.DeviceID == 0 {
		t.Errorf("the television is %+v, %v; want the person, on a new device", principal, err)
	}
	devices, err := fixture.service.Devices(ctx, id)
	if err != nil || len(devices) != 1 || devices[0].Name != "Living room TV" {
		t.Errorf("devices = %+v, %v; want the television, named as the person said", devices, err)
	}
	if _, err = fixture.service.TakePairedSession(ctx, asked.Secret); !errors.Is(err, ErrNoPairing) {
		t.Errorf("the session taken twice: %v, want ErrNoPairing", err)
	}
	if state := fixture.service.PairingStateOf(ctx, asked.Secret); state.State != PairingEnded {
		t.Errorf("once taken: %+v, want ended", state)
	}
}

// Check codes that do not match end the pairing, and so do ten minutes; only the person who entered
// a code answers it (D288 §3).
func TestPairing_NoMatchTenMinutesOrSomeoneElsesAnswerActivatesNothing(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	other := fixture.createUser(t, "other", "pw", RoleUser)
	refused, err := fixture.service.RequestPairing(ctx, "", "web", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.ClaimPairing(ctx, id, id, refused.Code, "TV"); err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.AnswerPairing(ctx, other, refused.Code, true); !errors.Is(err, ErrNoPairing) {
		t.Errorf("someone else's answer: %v, want ErrNoPairing", err)
	}
	if err = fixture.service.AnswerPairing(ctx, id, refused.Code, false); err != nil {
		t.Fatal(err)
	}
	if state := fixture.service.PairingStateOf(ctx, refused.Secret); state.State != PairingEnded {
		t.Errorf("after no match: %+v, want ended", state)
	}
	late, err := fixture.service.RequestPairing(ctx, "", "web", 0)
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(10*time.Minute + time.Second)
	if _, err = fixture.service.ClaimPairing(ctx, id, id, late.Code, "TV"); !errors.Is(err, ErrNoPairing) {
		t.Errorf("a code past ten minutes: %v, want ErrNoPairing", err)
	}
	renewed, err := fixture.service.RequestPairing(ctx, late.Secret, "web", 0)
	if err != nil || renewed.Code == late.Code {
		t.Errorf("asking after it expired = %+v, %v; want a new code", renewed, err)
	}
	if devices, err := fixture.service.Devices(ctx, id); err != nil || len(devices) != 0 {
		t.Errorf("devices = %+v, %v; want none activated", devices, err)
	}
}

// Codes no device waits with count against the person entering them, as wrong passwords do, until
// even a right one is refused (D288 §2).
func TestPairing_WrongCodesCountAgainstThePersonEnteringThem(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	asked, err := fixture.service.RequestPairing(ctx, "", "web", 0)
	if err != nil {
		t.Fatal(err)
	}
	var last error
	for attempt := range 20 {
		if _, last = fixture.service.ClaimPairing(ctx, id, id, "0000-000"+string(rune('0'+attempt%10)),
			"TV"); errors.Is(last, ErrRateLimited) {
			break
		}
	}
	if !errors.Is(last, ErrRateLimited) {
		t.Fatalf("twenty wrong codes: %v, want the person limited", last)
	}
	if _, err = fixture.service.ClaimPairing(ctx, id, id, asked.Code, "TV"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("the right code from a limited person: %v, want limited", err)
	}
}

// A device signed in before, which remembers which device it was, resumes as that device under its
// own name, whatever name the person typed; a new device needs one (D288 §4, D221).
func TestPairing_ADeviceSignedInBeforeResumesItselfAndANewOneNeedsAName(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	phone, television := phoneAndTelevision(t, fixture.service, id)
	listed, err := fixture.service.Authenticate(ctx, television)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.SignOut(ctx, television); err != nil {
		t.Fatal(err)
	}
	asked, err := fixture.service.RequestPairing(ctx, "", "web", listed.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.service.ClaimPairing(ctx, id, id, asked.Code, "")
	if err != nil || claim.DeviceName != "Living room TV" {
		t.Fatalf("entering the television's code = %+v, %v; want its own name", claim, err)
	}
	if err = fixture.service.AnswerPairing(ctx, id, asked.Code, true); err != nil {
		t.Fatal(err)
	}
	again, err := fixture.service.TakePairedSession(ctx, asked.Secret)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := fixture.service.Authenticate(ctx, again)
	if err != nil || resumed.DeviceID != listed.DeviceID {
		t.Errorf("the television signed in as device %d, %v; want %d", resumed.DeviceID, err, listed.DeviceID)
	}
	if _, err = fixture.service.Authenticate(ctx, phone); err != nil {
		t.Errorf("the phone: %v", err)
	}
	unnamed, err := fixture.service.RequestPairing(ctx, "", "web", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.ClaimPairing(ctx, id, id, unnamed.Code, " "); !errors.Is(err, ErrNoDeviceName) {
		t.Errorf("a new device with no name: %v, want ErrNoDeviceName", err)
	}
}

// At most pairingsAtMost wait at once: the oldest no one has entered gives way to a new one, and one
// entered stays (D288 §6).
func TestPairing_AtMostAHundredWaitTheOldestNotEnteredGivingWay(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	entered, err := fixture.service.RequestPairing(ctx, "", "web", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.ClaimPairing(ctx, id, id, entered.Code, "TV"); err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(time.Second)
	oldest, err := fixture.service.RequestPairing(ctx, "", "web", 0)
	if err != nil {
		t.Fatal(err)
	}
	for range pairingsAtMost - 1 {
		fixture.clock.Advance(time.Millisecond)
		if _, err = fixture.service.RequestPairing(ctx, "", "web", 0); err != nil {
			t.Fatal(err)
		}
	}
	if state := fixture.service.PairingStateOf(ctx, oldest.Secret); state.State != PairingEnded {
		t.Errorf("the oldest not entered: %+v, want it gone", state)
	}
	if state := fixture.service.PairingStateOf(ctx, entered.Secret); state.State != PairingChecking {
		t.Errorf("the one entered: %+v, want it kept", state)
	}
}
