// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"errors"
	"testing"
)

// phoneAndTelevision signs a person in on a phone and adds a television by code, and returns the
// two sessions.
func phoneAndTelevision(t *testing.T, service *Service, id int64) (phone, television string) {
	t.Helper()
	ctx := t.Context()
	phone, err := service.SignIn(ctx, "partner", "pw", "192.0.2.1", NewDevice{Name: "Phone", Platform: "web"})
	if err != nil {
		t.Fatal(err)
	}
	code, err := service.IssueDeviceCode(ctx, id, "Living room TV", 0)
	if err != nil {
		t.Fatal(err)
	}
	television, err = service.SignInWithCode(ctx, code.Secret, "web", "192.0.2.2", 0)
	if err != nil {
		t.Fatal(err)
	}
	return phone, television
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
