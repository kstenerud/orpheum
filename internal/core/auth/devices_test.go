// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"testing"
	"time"
)

// phoneAndTelevision signs a person in on a phone and activates a television by its code (D288), and
// returns the two sessions.
func phoneAndTelevision(t *testing.T, service *Service, userID int64) (phoneToken, televisionToken string) {
	t.Helper()
	ctx := t.Context()
	phoneToken, err := service.SignIn(ctx, "partner", "pw", "192.0.2.1", NewDevice{Name: "Phone", Platform: "web"})
	if err != nil {
		t.Fatal(err)
	}
	return phoneToken, pairDevice(t, service, [2]int64{userID, userID}, "Living room TV", 0)
}

func TestDevices_AreListedAndARemovedOneIsSignedOutAtItsNextRequest(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	phone, television := phoneAndTelevision(t, fixture.service, id)
	other := fixture.createUser(t, "owner", "pw", RoleAdmin)
	devices, err := fixture.service.Devices(ctx, id)
	if err != nil || len(devices) != 2 || !devices[0].IsSignedIn || !devices[1].IsSignedIn {
		t.Fatalf("devices = %+v, %v; want the phone and the television, signed in", devices, err)
	}
	televisionID := devices[0].ID
	if devices[0].Name != "Living room TV" {
		televisionID = devices[1].ID
	}
	err = fixture.service.RemoveDevice(ctx, other, televisionID)
	if !errors.Is(err, ErrNoDevice) {
		t.Errorf("removing someone else's device: %v, want ErrNoDevice", err)
	}
	err = fixture.service.RenameDevice(ctx, id, televisionID, "Lounge TV")
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.service.RemoveDevice(ctx, id, televisionID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.Authenticate(ctx, television)
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("the removed television's next request: %v, want signed out (D218 §4)", err)
	}
	_, err = fixture.service.Authenticate(ctx, phone)
	if err != nil {
		t.Errorf("the phone after removing the television: %v, want still signed in", err)
	}
	left, err := fixture.service.Devices(ctx, id)
	if err != nil || len(left) != 1 || left[0].Name != "Phone" {
		t.Errorf("devices after removing the television = %+v, %v; want the phone", left, err)
	}
}

func TestSignOutElsewhere_EndsEverySessionButThisDevicesAndKeepsThemListed(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	here, err := fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1", NewDevice{Name: "Phone", Platform: "web"})
	if err != nil {
		t.Fatal(err)
	}
	secondDevice := NewDevice{Name: "Laptop", Platform: "web"}
	elsewhere, err := fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1", secondDevice)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := fixture.service.Authenticate(ctx, here)
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.service.SignOutElsewhere(ctx, id, principal.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.Authenticate(ctx, elsewhere)
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("the laptop: %v, want signed out", err)
	}
	_, err = fixture.service.Authenticate(ctx, here)
	if err != nil {
		t.Errorf("this device: %v, want still signed in", err)
	}
	devices, err := fixture.service.Devices(ctx, id)
	if err != nil || len(devices) != 2 {
		t.Fatalf("devices = %+v, %v; want both still listed", devices, err)
	}
	for _, device := range devices {
		if device.IsSignedIn != (device.ID == principal.DeviceID) {
			t.Errorf("%s signed in = %v", device.Name, device.IsSignedIn)
		}
	}
}

func TestSetAudioOutput_IsKeptOnTheDeviceAndOnlyItsOwnerSetsIt(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	phoneAndTelevision(t, fixture.service, id)
	other := fixture.createUser(t, "owner", "pw", RoleAdmin)
	devices, err := fixture.service.Devices(ctx, id)
	if err != nil || len(devices) == 0 || devices[0].AudioOutput != "" {
		t.Fatalf("devices = %+v, %v; want none asked yet", devices, err)
	}
	if err = fixture.service.SetAudioOutput(ctx, other, devices[0].ID, "receiver"); !errors.Is(err, ErrNoDevice) {
		t.Errorf("setting someone else's device: %v, want ErrNoDevice", err)
	}
	if err = fixture.service.SetAudioOutput(ctx, id, devices[0].ID, "built_in_speakers"); err != nil {
		t.Fatal(err)
	}
	device, err := fixture.service.DeviceOf(ctx, id, devices[0].ID)
	if err != nil || device.AudioOutput != "built_in_speakers" {
		t.Errorf("the device after = %+v, %v; want built_in_speakers kept", device, err)
	}
}

func TestDevices_ADeviceWasLastUsedWhenItsLatestSessionWasIssuedOrUsed(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	first, err := fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(2 * time.Minute)
	principal, err := fixture.service.Authenticate(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(30 * 24 * time.Hour)
	resumed := NewDevice{Name: laptop.Name, Platform: laptop.Platform, PreviousDeviceID: principal.DeviceID}
	if _, err = fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1", resumed); err != nil {
		t.Fatal(err)
	}
	devices, err := fixture.service.Devices(ctx, id)
	if err != nil || len(devices) != 1 {
		t.Fatalf("devices = %+v, %v; want the laptop", devices, err)
	}
	if want := fixture.clock.Now(); !devices[0].LastUsedAt.Equal(want) {
		t.Errorf("last used %v, want %v: the fresh sign-in, not the old session's last use",
			devices[0].LastUsedAt, want)
	}
}

func TestNames_APersonsNameIsCleanedWhereItEntersAndSignsInAsTyped(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	// A right-to-left override, a zero-width space and a control: what the sanitiser removes
	// (D211 §2). Signing in by the name as typed still finds the person.
	const hostile = "  part\u202ener\u200b\u0007 "
	id := fixture.createUser(t, hostile, "pw", RoleUser)
	if _, err := fixture.service.SignIn(ctx, hostile, "pw", "192.0.2.1", NewDevice{Name: "Phone"}); err != nil {
		t.Fatalf("signing in by the name as typed: %v", err)
	}
	accounts, err := fixture.service.Accounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].Name != "partner" {
		t.Errorf("accounts = %+v, %v; want the name cleaned to %q", accounts, err, "partner")
	}
	if err = fixture.service.RenameUser(ctx, id, "\u202epartner 2 "); err != nil {
		t.Fatal(err)
	}
	accounts, err = fixture.service.Accounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].Name != "partner 2" {
		t.Errorf("renamed = %+v, %v; want the name cleaned to %q", accounts, err, "partner 2")
	}
}

func TestNames_ADevicesNameAndPlatformAreCleanedWhereTheyEnter(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "partner", "pw", RoleUser)
	_, err := fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1",
		NewDevice{Name: "Pho\u202ene\u2066", Platform: "we\u200bb"})
	if err != nil {
		t.Fatal(err)
	}
	asked, err := fixture.service.RequestPairing(ctx, "", "we\u200bb", 0)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.service.ClaimPairing(ctx, id, id, asked.Code, "Living\u202e room\u0000")
	if err != nil || claim.DeviceName != "Living room" {
		t.Errorf("the entered device = %+v, %v; want its name cleaned to %q", claim, err, "Living room")
	}
	devices, err := fixture.service.Devices(ctx, id)
	if err != nil || len(devices) != 1 || devices[0].Name != "Phone" || devices[0].Platform != "web" {
		t.Fatalf("devices = %+v, %v; want the phone's name and platform cleaned", devices, err)
	}
	if err = fixture.service.RenameDevice(ctx, id, devices[0].ID, "\u202ePhone 2"); err != nil {
		t.Fatal(err)
	}
	if devices, err = fixture.service.Devices(ctx, id); err != nil || devices[0].Name != "Phone 2" {
		t.Errorf("the renamed device = %+v, %v; want %q", devices, err, "Phone 2")
	}
}
