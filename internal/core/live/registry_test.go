// SPDX-License-Identifier: AGPL-3.0-or-later

package live

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
)

// inbox keeps what one connection was sent.
type inbox struct {
	mutex    sync.Mutex
	messages []Outbound
}

func (box *inbox) send(message Outbound) {
	box.mutex.Lock()
	defer box.mutex.Unlock()
	box.messages = append(box.messages, message)
}

// last is the last message of kind it was sent, and whether there was one.
func (box *inbox) last(kind MessageType) (Outbound, bool) {
	box.mutex.Lock()
	defer box.mutex.Unlock()
	for _, message := range slices.Backward(box.messages) {
		if message.Type == kind {
			return message, true
		}
	}
	return Outbound{}, false
}

// count is how many messages of kind it was sent.
func (box *inbox) count(kind MessageType) int {
	box.mutex.Lock()
	defer box.mutex.Unlock()
	count := 0
	for _, message := range box.messages {
		if message.Type == kind {
			count++
		}
	}
	return count
}

// Minted mints a credential for a receiver loaded again: "again-1", then "again-2", and on.
func minted() Credential {
	count := 0
	return func(_, _ int64) (string, error) {
		count++
		return fmt.Sprintf("again-%d", count), nil
	}
}

const (
	owner      = int64(1)
	partner    = int64(2)
	phone      = int64(10)
	television = int64(11)
	laptop     = int64(12)
)

// Whether a connection's tab is in front, as Connect is told.
const (
	inFront = true
	behind  = false
)

// A cast goes to the receiver's tab in front, the one that came to the front last; to a device with
// none in front it is refused, and the person's devices are told which have one; a tab come to the
// front, or the one in front as the tab holding the cast goes, is handed a cast none holds (D338 §2).
func TestRegistry_TheTabInFrontReceives(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted())
	phoneBox, older, newer := &inbox{}, &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, phoneBox.send)
	olderTab := registry.Connect(owner, television, behind, older.send)
	cast := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	if _, err := registry.Cast(cast, "a"); !errors.Is(err, ErrNotInFront) {
		t.Errorf("a cast to a device with no tab in front was %v, want ErrNotInFront", err)
	}
	if devices, _ := phoneBox.last(MessageDevices); !slices.Equal(devices.InFrontDeviceIDs, []int64{phone}) {
		t.Errorf("with the TV's tab behind the phone was told %v in front, want itself alone", devices.InFrontDeviceIDs)
	}
	registry.ShowFront(olderTab, inFront)
	newerTab := registry.Connect(owner, television, inFront, newer.send)
	devices, _ := phoneBox.last(MessageDevices)
	if !slices.Equal(devices.InFrontDeviceIDs, []int64{phone, television}) {
		t.Errorf("with the TV's tab in front the phone was told %v in front, want both", devices.InFrontDeviceIDs)
	}
	if _, err := registry.Cast(cast, "a"); err != nil {
		t.Fatal(err)
	}
	if _, isTold := older.last(MessageReceive); isTold {
		t.Error("the tab that came to the front first was handed the cast, not the one in front last")
	}
	if received, isTold := newer.last(MessageReceive); !isTold || received.Receive.Credential != "a" {
		t.Errorf("the tab in front last was handed %+v (%v), want the cast", received.Receive, isTold)
	}
	registry.ShowFront(olderTab, behind)
	registry.Disconnect(newerTab)
	if _, isTold := older.last(MessageReceive); isTold {
		t.Error("a tab behind was handed the cast as the one holding it went")
	}
	registry.ShowFront(olderTab, inFront)
	if received, isTold := older.last(MessageReceive); !isTold || received.Receive.Credential != "again-1" {
		t.Errorf("the tab come to the front was handed %+v (%v), want the cast by a new credential",
			received.Receive, isTold)
	}
	last := &inbox{}
	registry.Connect(owner, television, inFront, last.send)
	registry.Disconnect(olderTab)
	if received, isTold := last.last(MessageReceive); !isTold || received.Receive.Credential != "again-2" {
		t.Errorf("the tab in front as the holder went was handed %+v (%v), want the cast", received.Receive, isTold)
	}
}

// A person's devices connected now are listed to their own connections alone, a device while any of
// its connections is open (D337 §2).
func TestRegistry_ListsAPersonsConnectedDevicesToThemAlone(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted())
	phoneBox, tvBox, otherTab, partnerBox := &inbox{}, &inbox{}, &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, phoneBox.send)
	tvConnection := registry.Connect(owner, television, inFront, tvBox.send)
	secondTab := registry.Connect(owner, television, inFront, otherTab.send)
	registry.Connect(partner, 20, inFront, partnerBox.send)
	if devices, _ := phoneBox.last(MessageDevices); !slices.Equal(devices.DeviceIDs, []int64{phone, television}) {
		t.Errorf("the phone was told %v, want the phone and the TV", devices.DeviceIDs)
	}
	if devices, _ := partnerBox.last(MessageDevices); !slices.Equal(devices.DeviceIDs, []int64{20}) {
		t.Errorf("another person was told %v, want their own device alone", devices.DeviceIDs)
	}
	devices, isTold := otherTab.last(MessageDevices)
	if !isTold || !slices.Equal(devices.DeviceIDs, []int64{phone, television}) {
		t.Errorf("a second tab of the TV was told %v (%v), want the phone and the TV", devices.DeviceIDs, isTold)
	}
	before := phoneBox.count(MessageDevices)
	registry.Disconnect(tvConnection)
	if after := phoneBox.count(MessageDevices); after != before {
		t.Errorf("the phone was told again (%d times, from %d) though the TV's other tab is open", after, before)
	}
	registry.Disconnect(secondTab)
	if devices, _ := phoneBox.last(MessageDevices); !slices.Equal(devices.DeviceIDs, []int64{phone}) {
		t.Errorf("with the TV closed the phone was told %v, want itself alone", devices.DeviceIDs)
	}
	if connected := registry.Connected(owner); !slices.Equal(connected, []int64{phone}) {
		t.Errorf("connected = %v, want the phone", connected)
	}
}

// A cast goes only to another of the asker's own devices connected now (D337 §1, §2).
func TestRegistry_ACastGoesOnlyToAnotherConnectedDeviceOfTheSamePerson(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted())
	tvBox := &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.Connect(owner, television, inFront, tvBox.send)
	registry.Connect(partner, 20, inFront, (&inbox{}).send)
	for name, cast := range map[string]Cast{
		"to itself":            {ID: 1, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: phone},
		"to a device not open": {ID: 1, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: laptop},
		"to another's device":  {ID: 1, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: 20},
		"from someone unknown": {ID: 1, UserID: 99, SenderDeviceID: 98, ReceiverDeviceID: television},
	} {
		if _, err := registry.Cast(cast, "secret"); err == nil {
			t.Errorf("a cast %s was taken", name)
		}
	}
	position := int64(61_000)
	replaced, err := registry.Cast(Cast{
		ID: 7, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television, SenderName: "Pixel", FileID: 3,
		PositionMs: &position,
	}, "secret")
	if err != nil || len(replaced) != 0 {
		t.Fatalf("the cast to the TV: %v, replacing %v", err, replaced)
	}
	received, isTold := tvBox.last(MessageReceive)
	if !isTold || *received.Receive != (Receive{
		CastID: 7, Credential: "secret", FileID: 3, PositionMs: &position, FromDeviceID: phone,
		FromDeviceName: "Pixel",
	}) {
		t.Errorf("the TV was handed %+v (%v)", received.Receive, isTold)
	}
	if casts := registry.CastsOf(owner, phone); len(casts) != 1 || casts[0].ID != 7 {
		t.Errorf("the phone's casts are %+v, want 7", casts)
	}
	if _, err := registry.End(partner, 20, 7); !errors.Is(err, ErrNoCast) {
		t.Errorf("another person ended the cast: %v", err)
	}
	if _, err := registry.End(owner, laptop, 7); !errors.Is(err, ErrNoCast) {
		t.Errorf("a device neither sending nor receiving ended the cast: %v", err)
	}
	if ended, err := registry.End(owner, television, 7); err != nil || ended.ID != 7 {
		t.Errorf("the receiver ended %+v, %v; want cast 7", ended, err)
	}
	if casts := registry.CastsOf(owner, phone); len(casts) != 0 {
		t.Errorf("after its end the phone's casts are %+v", casts)
	}
}

// The last cast wins: a cast to a receiver replaces its cast, and a sender casting elsewhere leaves
// the receiver it controlled (D337 §6).
func TestRegistry_TheLastCastWins(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted())
	tvBox := &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.Connect(owner, television, inFront, tvBox.send)
	registry.Connect(owner, laptop, inFront, (&inbox{}).send)
	first := Cast{ID: 1, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	if _, err := registry.Cast(first, "a"); err != nil {
		t.Fatal(err)
	}
	second := Cast{ID: 2, UserID: owner, SenderDeviceID: laptop, ReceiverDeviceID: television}
	replaced, err := registry.Cast(second, "b")
	if err != nil || len(replaced) != 1 || replaced[0].ID != 1 {
		t.Fatalf("the laptop's cast replaced %+v, %v; want the phone's, 1", replaced, err)
	}
	if received, _ := tvBox.last(MessageReceive); received.Receive.CastID != 2 {
		t.Errorf("the TV receives cast %d, want 2", received.Receive.CastID)
	}
	replaced, err = registry.Cast(Cast{ID: 3, UserID: owner, SenderDeviceID: laptop, ReceiverDeviceID: phone}, "c")
	if err != nil || len(replaced) != 1 || replaced[0].ID != 2 {
		t.Errorf("the laptop casting to the phone replaced %+v, %v; want its cast to the TV, 2", replaced, err)
	}
	if casts := registry.CastsOf(owner, television); len(casts) != 0 {
		t.Errorf("the TV still has %+v", casts)
	}
}

// A receiver whose page is loaded again is handed its cast again, with a credential of its own, once
// the connection that held it is gone; a second tab behind it is not (§4).
func TestRegistry_AReceiverLoadedAgainIsHandedItsCastAgain(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted())
	first, second, third := &inbox{}, &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	held := registry.Connect(owner, television, inFront, first.send)
	cast := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	if _, err := registry.Cast(cast, "a"); err != nil {
		t.Fatal(err)
	}
	registry.Connect(owner, television, behind, second.send)
	if _, isTold := second.last(MessageReceive); isTold {
		t.Error("a second tab behind the one receiving was handed the cast")
	}
	registry.Disconnect(held)
	registry.Connect(owner, television, inFront, third.send)
	received, isTold := third.last(MessageReceive)
	if !isTold || received.Receive.CastID != 5 || received.Receive.Credential != "again-1" {
		t.Errorf("the page loaded again was handed %+v (%v), want cast 5 by a new credential", received.Receive, isTold)
	}
}

// A cast moves on to another film, the next episode, at the asking of its sender or its receiver and
// no other device: its receiver is handed it again with the film, from where the person left it, in
// the same cast (plans/up-next.md).
func TestRegistry_ACastMovesOnToTheNextFilmAndItsReceiverIsHandedIt(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted())
	receiver := &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.Connect(owner, laptop, inFront, (&inbox{}).send)
	registry.Connect(owner, television, inFront, receiver.send)
	from := int64(61_000)
	cast := Cast{
		ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television, FileID: 1, PositionMs: &from,
	}
	if _, err := registry.Cast(cast, "a"); err != nil {
		t.Fatal(err)
	}
	next := Film{FileID: 2, TitleID: 22, TitleName: "Cat", DurationMs: 1000}
	if _, err := registry.MoveOn(owner, laptop, 5, next); !errors.Is(err, ErrNoCast) {
		t.Errorf("a device neither sending nor receiving moved it on: %v; want ErrNoCast", err)
	}
	if _, err := registry.MoveOn(partner, television, 5, next); !errors.Is(err, ErrNoCast) {
		t.Errorf("another person moved it on: %v; want ErrNoCast", err)
	}
	moved, err := registry.MoveOn(owner, television, 5, next)
	if err != nil || moved.FileID != 2 || moved.TitleName != "Cat" || moved.PositionMs != nil {
		t.Fatalf("moved on by its receiver: %+v (%v); want file 2, Cat, from where the person left it", moved, err)
	}
	received, isTold := receiver.last(MessageReceive)
	want := "true 2 &{CastID:5 Credential:again-1 FileID:2 TitleName:Cat PositionMs:<nil> FromDeviceID:10 " +
		"FromDeviceName:}"
	if got := fmt.Sprintf("%v %d %+v", isTold, receiver.count(MessageReceive), received.Receive); got != want {
		t.Errorf("the receiver was handed %s; want cast 5 again, of file 2, Cat, by a new credential", got)
	}
	if listed := registry.CastsOf(owner, phone); len(listed) != 1 || listed[0].FileID != 2 {
		t.Errorf("the sender's casts = %+v; want the one, now of file 2", listed)
	}
}

// A device removed is told its sign-in ended on each of its connections, and no other device is
// (D218 §4).
func TestRegistry_ARemovedDeviceIsToldItsSignInEnded(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted())
	phoneBox, firstTab, secondTab, partnerBox := &inbox{}, &inbox{}, &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, phoneBox.send)
	registry.Connect(owner, television, inFront, firstTab.send)
	registry.Connect(owner, television, behind, secondTab.send)
	registry.Connect(partner, television, inFront, partnerBox.send)
	registry.SignedOut(owner, television)
	for name, box := range map[string]*inbox{"the TV's first tab": firstTab, "the TV's second tab": secondTab} {
		if box.count(MessageSignedOut) != 1 {
			t.Errorf("%s was told its sign-in ended %d times, want once", name, box.count(MessageSignedOut))
		}
	}
	for name, box := range map[string]*inbox{"the phone": phoneBox, "the partner's device": partnerBox} {
		if box.count(MessageSignedOut) != 0 {
			t.Errorf("%s was told its sign-in ended", name)
		}
	}
	registry.SignedOut(laptop, laptop) // a person with no connection: nothing to tell
}
