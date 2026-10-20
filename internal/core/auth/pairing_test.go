// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// pairDevice activates a device by its code, entered by byUserID for forUserID under name, the
// check codes confirmed, and returns its session; previousDeviceID is the device it was before.
func pairDevice(
	t *testing.T, service *Service, people [2]int64, name string, previousDeviceID int64,
) Issued {
	t.Helper()
	ctx := t.Context()
	asked, err := service.RequestPairing(ctx, "", "web", PreviousDevice{ID: previousDeviceID}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ClaimPairing(ctx, people[0], people[1], asked.Code, name); err != nil {
		t.Fatal(err)
	}
	if err = service.AnswerPairing(ctx, people[0], asked.Code, true); err != nil {
		t.Fatal(err)
	}
	issued, err := service.TakePairedSession(ctx, asked.Secret)
	if err != nil {
		t.Fatal(err)
	}
	return issued
}

// A device not signed in is given a code, the same one each time it asks while no one has entered
// it; the person entering it, however typed, gets a check code the device shows too, and no one can
// enter it again (D288 §1–§3).
func TestPairing_ADeviceKeepsItsCodeUntilEnteredAndThenShowsThePersonsCheckCode(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	asked, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if len(asked.Code) != 9 || asked.Code[4] != '-' || asked.ExpiresAt.Sub(fixture.clock.Now()) != 10*time.Minute {
		t.Errorf("the code = %+v, want xxxx-xxxx for ten minutes (D218 §3)", asked)
	}
	again, err := fixture.service.RequestPairing(ctx, asked.Secret, "web", PreviousDevice{}, "192.0.2.10")
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
	asked, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "192.0.2.10")
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
	token, err := tokenOf(fixture.service.TakePairedSession(ctx, asked.Secret))
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
	refused, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "192.0.2.10")
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
	late, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(10*time.Minute + time.Second)
	if _, err = fixture.service.ClaimPairing(ctx, id, id, late.Code, "TV"); !errors.Is(err, ErrNoPairing) {
		t.Errorf("a code past ten minutes: %v, want ErrNoPairing", err)
	}
	renewed, err := fixture.service.RequestPairing(ctx, late.Secret, "web", PreviousDevice{}, "192.0.2.10")
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
	asked, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "192.0.2.10")
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
	phone, paired := pairedPhoneAndTelevision(t, fixture.service, id)
	television := paired.Token
	listed, err := fixture.service.Authenticate(ctx, television)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.SignOut(ctx, television); err != nil {
		t.Fatal(err)
	}
	// Naming the television without its proof makes a new device, which needs a name (the second
	// audit's W4: a stranger's device named another's and took its place).
	unproven, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{ID: listed.DeviceID},
		"192.0.2.11")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.ClaimPairing(ctx, id, id, unproven.Code, ""); !errors.Is(err, ErrNoDeviceName) {
		t.Errorf("a code naming the television without its proof: %v, want a new device, ErrNoDeviceName", err)
	}
	asked, err := fixture.service.RequestPairing(ctx, "", "web",
		PreviousDevice{ID: listed.DeviceID, Proof: paired.DeviceSecret}, "192.0.2.10")
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
	again, err := tokenOf(fixture.service.TakePairedSession(ctx, asked.Secret))
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
	unnamed, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.ClaimPairing(ctx, id, id, unnamed.Code, " "); !errors.Is(err, ErrNoDeviceName) {
		t.Errorf("a new device with no name: %v, want ErrNoDeviceName", err)
	}
}

// Past five codes not yet entered from one network (an IPv4 address, or an IPv6 /64 however many
// addresses it uses), twenty from one site (an IPv6 /48), or a thousand in all, a new code is refused,
// and no code already waiting is dropped for it: the oldest giving way let a stranger push out a
// television's code before it was typed (the security audit's S13), and twenty networks filled a cap
// of a hundred (the second audit's K5, 2026-10-13; D288 §6).
// The pairing caps are settings (the constants sweep, 2026-10-20): at two a network and three a site,
// a third code from one /64 and a fourth from one /48 are refused.
func TestPairing_TheCapsAreTheSettings(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	settingstest.Set(t, fixture.settings, settings.PairingCodesPerNetworkMost, "2")
	settingstest.Set(t, fixture.settings, settings.PairingCodesPerSiteMost, "3")
	for index := range 2 {
		if _, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{},
			fmt.Sprintf("2001:db8:1:2::%x", index+1)); err != nil {
			t.Fatalf("code %d from one /64: %v", index+1, err)
		}
	}
	if _, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "2001:db8:1:2::ff"); !errors.Is(err,
		ErrTooManyPairings) {
		t.Errorf("a third code from one /64 at a cap of two: %v, want ErrTooManyPairings", err)
	}
	if _, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "2001:db8:1:3::1"); err != nil {
		t.Fatalf("a third code of the /48, from another /64: %v", err)
	}
	if _, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "2001:db8:1:4::1"); !errors.Is(err,
		ErrTooManyPairings) {
		t.Errorf("a fourth code from one /48 at a cap of three: %v, want ErrTooManyPairings", err)
	}
}

func TestPairing_ANewCodeIsRefusedPastTheCapsAndNoWaitingOneIsPushedOut(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	television, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "198.51.100.20")
	if err != nil {
		t.Fatal(err)
	}
	for index := range pairingsPerNetworkAtMost {
		if _, err = fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{},
			fmt.Sprintf("2001:db8:1:2::%x", index+1)); err != nil {
			t.Fatalf("code %d from one /64: %v", index+1, err)
		}
	}
	if _, err = fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "2001:db8:1:2::ffff"); !errors.Is(err,
		ErrTooManyPairings) {
		t.Errorf("a sixth code from the same /64: %v, want ErrTooManyPairings", err)
	}
	for index := range pairingsPerSiteAtMost {
		_, err = fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{},
			fmt.Sprintf("2001:db8:9:%x::1", index+1))
		if err != nil {
			t.Fatalf("code %d from as many networks of one /48: %v", index+1, err)
		}
	}
	if _, err = fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "2001:db8:9:ffff::1"); !errors.Is(err,
		ErrTooManyPairings) {
		t.Errorf("a code from one more network of that /48: %v, want ErrTooManyPairings", err)
	}
	waiting := 1 + pairingsPerNetworkAtMost + pairingsPerSiteAtMost
	for index := 0; ; index++ {
		_, err = fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{},
			fmt.Sprintf("10.%d.%d.1", index/256, index%256))
		if err != nil {
			break
		}
		waiting++
	}
	if !errors.Is(err, ErrTooManyPairings) || waiting != pairingsAtMost {
		t.Errorf("past %d waiting in all (%d): %v, want ErrTooManyPairings", pairingsAtMost, waiting, err)
	}
	if state := fixture.service.PairingStateOf(ctx, television.Secret); state.State != PairingWaiting {
		t.Errorf("the television's code, waiting first: %+v, want it still waiting", state)
	}
	again, err := fixture.service.RequestPairing(ctx, television.Secret, "web", PreviousDevice{}, "198.51.100.20")
	if err != nil || again.Code != television.Code {
		t.Errorf("the television asking again while full = %+v, %v; want its own code", again, err)
	}
}
