// SPDX-License-Identifier: AGPL-3.0-or-later

package live

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// guestOfThePhone is a registry with the owner's phone and laptop connected, and a guest receiver of
// the phone's, the television, connected too; and the phone's inbox.
func guestOfThePhone() (*Registry, *inbox) {
	registry := NewRegistry(minted(), newClock())
	phoneBox := &inbox{}
	registry.Connect(owner, phone, inFront, phoneBox.send)
	registry.Connect(owner, laptop, inFront, (&inbox{}).send)
	registry.ExpectGuest(owner, television, phone, "TV")
	registry.Connect(owner, television, inFront, (&inbox{}).send)
	return registry, phoneBox
}

// A guest receiver (D465 §1) is no device of the person's to show: it is in no devices message,
// connected, in front or receiving.
func TestRegistry_AGuestReceiverIsUnlisted(t *testing.T) {
	t.Parallel()
	registry, phoneBox := guestOfThePhone()
	if !registry.IsGuest(owner, television) || registry.IsGuest(owner, laptop) ||
		registry.IsGuest(partner, television) {
		t.Error("IsGuest names other than the guest receiver alone")
	}
	devices, _ := phoneBox.last(MessageDevices)
	if slices.Contains(devices.DeviceIDs, television) || slices.Contains(devices.InFrontDeviceIDs, television) {
		t.Errorf("the phone was told of the guest receiver: %+v", devices)
	}
	if connected := registry.Connected(owner); slices.Contains(connected, television) {
		t.Errorf("connected = %v, the guest receiver among them", connected)
	}
	if _, _, err := registry.Cast(Cast{ID: 2, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television},
		"secret"); err != nil {
		t.Fatal(err)
	}
	devices, _ = phoneBox.last(MessageDevices)
	if slices.Contains(devices.ReceivingDeviceIDs, television) {
		t.Errorf("the phone was told the guest receiver receives: %+v", devices)
	}
}

// Only the device that sent a guest receiver its grant casts to it, once, and it casts to no one;
// once that cast ends it is no one's target (D465 §1).
func TestRegistry_AGuestReceiverTakesOneCastFromItsSenderAlone(t *testing.T) {
	t.Parallel()
	registry, _ := guestOfThePhone()
	if name, may := registry.GuestCastableBy(owner, television, phone); name != "TV" || !may {
		t.Errorf("GuestCastableBy(its sender) = %q, %v; want its name, castable", name, may)
	}
	if _, may := registry.GuestCastableBy(owner, television, laptop); may {
		t.Error("GuestCastableBy(another device) is castable")
	}
	for name, cast := range map[string]Cast{
		"from another device": {ID: 1, UserID: owner, SenderDeviceID: laptop, ReceiverDeviceID: television},
		"from the guest":      {ID: 1, UserID: owner, SenderDeviceID: television, ReceiverDeviceID: laptop},
	} {
		if _, _, err := registry.Cast(cast, "secret"); !errors.Is(err, ErrNotConnected) {
			t.Errorf("a cast %s: %v, want ErrNotConnected", name, err)
		}
	}
	if _, _, err := registry.Cast(Cast{ID: 2, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television},
		"secret"); err != nil {
		t.Fatalf("its sender's cast to the guest receiver: %v", err)
	}
	if _, err := registry.End(owner, phone, 2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.Cast(Cast{ID: 3, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television},
		"secret"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("a second cast to the guest receiver: %v, want ErrNotConnected", err)
	}
	if _, may := registry.GuestCastableBy(owner, television, phone); may {
		t.Error("GuestCastableBy(its sender) after its one cast is castable")
	}
}

// A guest receiver keeps nothing after its cast (D465 §1): EndGuests ends it once its one cast has
// ended, however it ended, or once it was never cast to within UnansweredAfter of its trade, and
// never while its cast goes on; its connections are told their sign-in ended and let go.
func TestRegistry_AGuestReceiverEndsWithItsCastOrUnusedAfterUnansweredAfter(t *testing.T) {
	t.Parallel()
	fake := newClock()
	registry := NewRegistry(minted(), fake)
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.ExpectGuest(owner, television, phone, "TV")
	guestBox := &inbox{}
	registry.Connect(owner, television, inFront, guestBox.send)
	registry.ExpectGuest(owner, laptop, phone, "Laptop")
	if _, _, err := registry.Cast(Cast{ID: 2, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television},
		"secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Accept(owner, television, 2); err != nil {
		t.Fatal(err)
	}
	fake.Advance(UnansweredAfter - time.Second)
	if ended := registry.EndGuests(); len(ended) != 0 {
		t.Fatalf("guests ended before UnansweredAfter, one casting: %+v", ended)
	}
	fake.Advance(time.Second)
	if ended := registry.EndGuests(); !slices.Equal(ended, []Guest{{UserID: owner, DeviceID: laptop}}) {
		t.Fatalf("after UnansweredAfter the guests ended were %+v; want the one never cast to alone", ended)
	}
	fake.Advance(time.Hour)
	if ended := registry.EndGuests(); len(ended) != 0 {
		t.Fatalf("a guest receiving its cast was ended: %+v", ended)
	}
	if _, err := registry.End(owner, phone, 2); err != nil {
		t.Fatal(err)
	}
	if ended := registry.EndGuests(); !slices.Equal(ended, []Guest{{UserID: owner, DeviceID: television}}) {
		t.Fatalf("after its cast ended the guests ended were %+v; want the TV", ended)
	}
	if guestBox.count(MessageSignedOut) != 1 {
		t.Error("the guest receiver's connection was not told its sign-in ended")
	}
	if registry.IsGuest(owner, television) || slices.Contains(registry.Connected(owner), television) {
		t.Error("the ended guest receiver is still kept")
	}
}
