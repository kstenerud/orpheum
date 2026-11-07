// SPDX-License-Identifier: AGPL-3.0-or-later

package live

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
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

// newClock is a fake clock at a fixed start.
func newClock() *clocktest.Fake {
	return clocktest.NewFake(time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC))
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
	registry := NewRegistry(minted(), newClock())
	phoneBox, older, newer := &inbox{}, &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, phoneBox.send)
	olderTab := registry.Connect(owner, television, behind, older.send)
	cast := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	if _, _, err := registry.Cast(cast, "a"); !errors.Is(err, ErrNotInFront) {
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
	if _, _, err := registry.Cast(cast, "a"); err != nil {
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
	registry := NewRegistry(minted(), newClock())
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
	registry := NewRegistry(minted(), newClock())
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
		if _, _, err := registry.Cast(cast, "secret"); err == nil {
			t.Errorf("a cast %s was taken", name)
		}
	}
	position := int64(61_000)
	_, left, err := registry.Cast(Cast{
		ID: 7, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television, SenderName: "Pixel", FileID: 3,
		PositionMs: &position,
	}, "secret")
	if err != nil || len(left) != 0 {
		t.Fatalf("the cast to the TV: %v, leaving %v", err, left)
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

// A device receiving a cast is no device's target until that cast ends, the same sender's included:
// a cast to it is refused, its cast going on untouched, and the person's every connection is told
// which of their devices receive, as each cast starts and ends (D386 §1, the owner: "When a device
// is being cast to, no other device may cast to it until the current casting ends").
func TestRegistry_ADeviceReceivingACastIsNoOnesTargetUntilItEnds(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	tvBox, laptopBox := &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.Connect(owner, television, inFront, tvBox.send)
	registry.Connect(owner, laptop, inFront, laptopBox.send)
	castToTheTV(t, registry, 1, phone)
	if told, _ := laptopBox.last(MessageDevices); !slices.Equal(told.ReceivingDeviceIDs, []int64{television}) {
		t.Errorf("the laptop was told %v receive; want the TV", told.ReceivingDeviceIDs)
	}
	for name, sender := range map[string]int64{"another device": laptop, "the same sender": phone} {
		_, _, err := registry.Cast(
			Cast{ID: 2, UserID: owner, SenderDeviceID: sender, ReceiverDeviceID: television}, "b")
		if !errors.Is(err, ErrReceiving) {
			t.Errorf("a cast from %s to the receiving TV: %v; want ErrReceiving", name, err)
		}
	}
	if casts := registry.CastsOf(owner, television); len(casts) != 1 || casts[0].ID != 1 {
		t.Errorf("the TV holds %+v; want cast 1 alone", casts)
	}
	if handed := tvBox.count(MessageReceive); handed != 1 {
		t.Errorf("the TV was handed %d casts; want the first alone", handed)
	}
	if _, err := registry.End(owner, phone, 1); err != nil {
		t.Fatal(err)
	}
	if told, _ := laptopBox.last(MessageDevices); len(told.ReceivingDeviceIDs) != 0 {
		t.Errorf("after the cast ended the laptop was told %v receive; want none", told.ReceivingDeviceIDs)
	}
	castToTheTV(t, registry, 3, laptop)
}

// A device casting to another may not be cast to by it, nor by a device it casts to through others:
// the cast back would close a loop, each device both remote and receiver of the other (the owner's
// 11-20 batch, finding 6: the receiver's Cast to the Mac casting to it left each believing it cast to
// the other). A cast elsewhere goes on, and the receiver still casts to a third device.
func TestRegistry_ACastBackToADeviceCastingHereIsRefused(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	phoneBox := &inbox{}
	registry.Connect(owner, phone, inFront, phoneBox.send)
	registry.Connect(owner, television, inFront, (&inbox{}).send)
	registry.Connect(owner, laptop, inFront, (&inbox{}).send)
	castToTheTV(t, registry, 1, phone)
	_, _, err := registry.Cast(Cast{ID: 2, UserID: owner, SenderDeviceID: television, ReceiverDeviceID: phone}, "b")
	if !errors.Is(err, ErrCastingHere) {
		t.Errorf("the TV's cast back to the phone casting to it: %v; want ErrCastingHere", err)
	}
	if phoneBox.count(MessageReceive) != 0 {
		t.Error("the phone was handed the cast back")
	}
	if _, _, err = registry.Cast(
		Cast{ID: 3, UserID: owner, SenderDeviceID: television, ReceiverDeviceID: laptop}, "c"); err != nil {
		t.Fatalf("the TV casting on to the laptop: %v", err)
	}
	_, _, err = registry.Cast(Cast{ID: 4, UserID: owner, SenderDeviceID: laptop, ReceiverDeviceID: phone}, "d")
	if !errors.Is(err, ErrCastingHere) {
		t.Errorf("the laptop's cast to the phone, which casts to it through the TV: %v; want ErrCastingHere", err)
	}
	if casts := registry.CastsOf(owner, television); len(casts) != 2 {
		t.Errorf("the TV holds %+v; want the phone's cast and its own to the laptop", casts)
	}
}

// A sender casting to another device leaves the receiver it controlled, whose cast it returns for its
// session to be ended (D337 §6: each sender controls one receiver).
func TestRegistry_ASenderCastingElsewhereLeavesTheReceiverItControlled(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	phoneBox := &inbox{}
	registry.Connect(owner, phone, inFront, phoneBox.send)
	registry.Connect(owner, television, inFront, (&inbox{}).send)
	registry.Connect(owner, laptop, inFront, (&inbox{}).send)
	castToTheTV(t, registry, 1, phone)
	_, left, err := registry.Cast(Cast{ID: 2, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: laptop}, "b")
	if err != nil || len(left) != 1 || left[0].ID != 1 {
		t.Fatalf("the phone casting to the laptop left %+v, %v; want its cast to the TV, 1", left, err)
	}
	if casts := registry.CastsOf(owner, television); len(casts) != 0 {
		t.Errorf("the TV still has %+v", casts)
	}
	if told, _ := phoneBox.last(MessageDevices); !slices.Equal(told.ReceivingDeviceIDs, []int64{laptop}) {
		t.Errorf("the phone was told %v receive; want the laptop alone", told.ReceivingDeviceIDs)
	}
}

// castToTheTV starts the cast castID from the owner's device sender to their television, nothing
// playing, and is the cast as it started.
func castToTheTV(t *testing.T, registry *Registry, castID, sender int64) Cast {
	t.Helper()
	started, _, err := registry.Cast(
		Cast{ID: castID, UserID: owner, SenderDeviceID: sender, ReceiverDeviceID: television}, "credential")
	if err != nil {
		t.Fatal(err)
	}
	return started
}

// A receiver whose page is loaded again is handed its cast again, with a credential of its own, once
// the connection that held it is gone; a second tab behind it is not (§4).
func TestRegistry_AReceiverLoadedAgainIsHandedItsCastAgain(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	first, second, third := &inbox{}, &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	held := registry.Connect(owner, television, inFront, first.send)
	cast := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	if _, _, err := registry.Cast(cast, "a"); err != nil {
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

// A cast plays another film, the next episode, at the asking of its sender or its receiver and no
// other device: its receiver is handed it again with the film, from where the person left it, in the
// same cast (D364 §3; plans/up-next.md).
func TestRegistry_ACastMovesOnToTheNextFilmAndItsReceiverIsHandedIt(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	receiver := &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.Connect(owner, laptop, inFront, (&inbox{}).send)
	registry.Connect(owner, television, inFront, receiver.send)
	from := int64(61_000)
	cast := Cast{
		ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television, FileID: 1, PositionMs: &from,
	}
	if _, _, err := registry.Cast(cast, "a"); err != nil {
		t.Fatal(err)
	}
	next := Film{FileID: 2, ItemID: 22, ItemName: "Cat", DurationMs: 1000}
	if _, err := registry.Play(owner, laptop, 5, next, nil); !errors.Is(err, ErrNoCast) {
		t.Errorf("a device neither sending nor receiving moved it on: %v; want ErrNoCast", err)
	}
	if _, err := registry.Play(partner, television, 5, next, nil); !errors.Is(err, ErrNoCast) {
		t.Errorf("another person moved it on: %v; want ErrNoCast", err)
	}
	moved, err := registry.Play(owner, television, 5, next, nil)
	if err != nil || moved.FileID != 2 || moved.ItemName != "Cat" || moved.PositionMs != nil {
		t.Fatalf("moved on by its receiver: %+v (%v); want file 2, Cat, from where the person left it", moved, err)
	}
	received, isTold := receiver.last(MessageReceive)
	want := "true 2 &{CastID:5 Credential:again-1 FileID:2 ItemName:Cat TitleName:Cat " +
		"PositionMs:<nil> FromDeviceID:10 " +
		"FromDeviceName: IsAccepted:false TogetherCredential: IsTogetherFollower:false}"
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
	registry := NewRegistry(minted(), newClock())
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

// castWithNothingOn connects a phone and a TV of the owner's, the phone casting to the TV with
// nothing playing, and returns what each was sent.
func castWithNothingOn(t *testing.T, registry *Registry) (sender, receiver *inbox) {
	t.Helper()
	sender, receiver = &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, sender.send)
	registry.Connect(owner, television, inFront, receiver.send)
	if told, _ := sender.last(MessageSending); told.Sending != nil {
		t.Fatalf("the phone was told it sends %+v before it cast", told.Sending)
	}
	cast := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television, ReceiverName: "TV"}
	if _, _, err := registry.Cast(cast, "a"); err != nil {
		t.Fatal(err)
	}
	return sender, receiver
}

// A cast is a session with the receiver (D364): it may start with nothing playing, the sender's pages
// told what it sends at once, a page of it opened later told before its devices; only the receiver
// says Use, which the sender is told; and its end tells the sender it sends none (§1, §2, §6).
func TestRegistry_ACastIsASessionTheSenderIsToldOf(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	sender, _ := castWithNothingOn(t, registry)
	if told, _ := sender.last(MessageSending); told.Sending == nil || *told.Sending != (Sending{
		CastID: 5, ReceiverDeviceID: television, ReceiverName: "TV",
	}) {
		t.Errorf("the phone was told it sends %+v; want cast 5 to the TV, nothing playing, not yet accepted",
			told.Sending)
	}
	if _, err := registry.Accept(owner, phone, 5); !errors.Is(err, ErrNoCast) {
		t.Errorf("the sender said Use for the receiver: %v; want ErrNoCast", err)
	}
	if _, err := registry.Accept(owner, television, 5); err != nil {
		t.Fatal(err)
	}
	if told, _ := sender.last(MessageSending); !told.Sending.IsAccepted {
		t.Error("the phone was not told the TV said Use")
	}
	later := &inbox{}
	registry.Connect(owner, phone, inFront, later.send)
	if first := later.messages; len(first) < 2 || first[0].Type != MessageSending || first[0].Sending == nil {
		t.Errorf("a page of the phone opened later was first told %+v; want the cast it sends, before its devices",
			first)
	}
	if _, err := registry.End(owner, phone, 5); err != nil {
		t.Fatal(err)
	}
	if told, _ := sender.last(MessageSending); told.Sending != nil {
		t.Errorf("after its end the phone was told it sends %+v; want none", told.Sending)
	}
}

// Each film the sender plays in a cast, and the ready state between, is handed to its receiver in the
// same cast, and the sender's pages are told what it plays (D364 §3, §5).
func TestRegistry_EachFilmPlayedInACastIsHandedToItsReceiver(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	sender, receiver := castWithNothingOn(t, registry)
	from := int64(5_000)
	for _, film := range []Film{{FileID: 2, ItemName: "Cat"}, {FileID: 3, ItemName: "Dog"}, {}} {
		if _, err := registry.Play(owner, phone, 5, film, &from); err != nil {
			t.Fatal(err)
		}
		handed, _ := receiver.last(MessageReceive)
		if handed.Receive.CastID != 5 || handed.Receive.FileID != film.FileID || *handed.Receive.PositionMs != from {
			t.Errorf("playing %+v, the TV was handed %+v; want cast 5, of that file from 5 s", film, handed.Receive)
		}
		if told, _ := sender.last(MessageSending); told.Sending.FileID != film.FileID ||
			told.Sending.ItemName != film.ItemName {
			t.Errorf("playing %+v, the phone was told %+v", film, told.Sending)
		}
	}
}

// A cast's receiver handed it while its person hosts a Watch Together session is handed a credential
// for that session too, to play it as their player (D364 §11); one whose person is a guest in one is
// handed their guest's credential, saying it follows (D371 §4); with none, none.
func TestRegistry_AReceiverIsHandedTheSessionItsPersonHosts(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	parts := map[int64]string{owner: "hosted"}
	registry.TakesPartBy(func(userID int64, _ string) (string, bool, int64) { return parts[userID], true, 1 })
	receiver, partnerReceiver := &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.Connect(owner, television, inFront, receiver.send)
	registry.Connect(partner, 20, inFront, (&inbox{}).send)
	registry.Connect(partner, 21, inFront, partnerReceiver.send)
	ownCast := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	if _, _, err := registry.Cast(ownCast, "a"); err != nil {
		t.Fatal(err)
	}
	partnerCast := Cast{ID: 6, UserID: partner, SenderDeviceID: 20, ReceiverDeviceID: 21}
	if _, _, err := registry.Cast(partnerCast, "b"); err != nil {
		t.Fatal(err)
	}
	if handed, _ := receiver.last(MessageReceive); handed.Receive.TogetherCredential != "hosted" ||
		handed.Receive.IsTogetherFollower {
		t.Errorf("the host's TV was handed %+v for their session; want its credential, leading",
			handed.Receive)
	}
	if handed, _ := partnerReceiver.last(MessageReceive); handed.Receive.TogetherCredential != "" {
		t.Errorf("a person hosting nothing had their TV handed %q; want none", handed.Receive.TogetherCredential)
	}
}

// A cast's receiver whose person is a guest in a Watch Together session is handed their guest's
// credential, saying it follows: the cast-to device takes the guest's part, just another follower
// (D371 §4).
func TestRegistry_AGuestsReceiverIsHandedTheirPartToFollow(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	registry.TakesPartBy(func(int64, string) (string, bool, int64) { return "guest", false, 1 })
	receiver := &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.Connect(owner, television, inFront, receiver.send)
	cast := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	if _, _, err := registry.Cast(cast, "a"); err != nil {
		t.Fatal(err)
	}
	if handed, _ := receiver.last(MessageReceive); handed.Receive.TogetherCredential != "guest" ||
		!handed.Receive.IsTogetherFollower {
		t.Errorf("the guest's TV was handed %+v; want their guest's credential, following", handed.Receive)
	}
}

// A cast from a page that holds a guest's membership hands its receiver that guest's part, by the
// membership the sender gave, following, here and as it is handed again: the person's own device
// joined as a guest of the session they host casts as its guest (VQ193).
func TestRegistry_AReceiverTakesThePartItsSendersPageHolds(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	registry.TakesPartBy(func(_ int64, held string) (string, bool, int64) {
		if held == "guest-membership" {
			return "guest-part", false, 1
		}
		return "hosted", true, 1
	})
	receiver := &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.Connect(owner, television, inFront, receiver.send)
	cast := Cast{
		ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television,
		SendersMembership: "guest-membership",
	}
	if _, _, err := registry.Cast(cast, "a"); err != nil {
		t.Fatal(err)
	}
	if handed, _ := receiver.last(MessageReceive); handed.Receive.TogetherCredential != "guest-part" ||
		!handed.Receive.IsTogetherFollower {
		t.Errorf("the TV was handed %+v; want the sender's guest's part, following", handed.Receive)
	}
	registry.PartChanged(owner, func(int64) (int64, bool) { return 0, false })
	if handed, _ := receiver.last(MessageReceive); handed.Receive.TogetherCredential != "guest-part" {
		t.Errorf("the TV was handed again %+v; want the sender's guest's part still", handed.Receive)
	}
}

// A person who starts Watch Together while they cast has their receiver handed the cast again, on the
// connection that holds it, with their part in the new session and the film from where the cast
// stands, so the receiver leads that one session with the film it plays (D364 §11, D372 §8); another
// person's casts are left alone.
func TestRegistry_StartingWatchTogetherWhileCastingHandsTheReceiverItsPart(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	parts := map[int64]string{}
	registry.TakesPartBy(func(userID int64, _ string) (string, bool, int64) { return parts[userID], true, 1 })
	receiver, partnerReceiver := &inbox{}, &inbox{}
	registry.Connect(owner, phone, inFront, (&inbox{}).send)
	registry.Connect(owner, television, inFront, receiver.send)
	registry.Connect(partner, 20, inFront, (&inbox{}).send)
	registry.Connect(partner, 21, inFront, partnerReceiver.send)
	sevenSeconds := int64(7_000)
	ownCast := Cast{
		ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television, FileID: 3,
		ItemName: "Metropolis", PositionMs: &sevenSeconds,
	}
	if _, _, err := registry.Cast(ownCast, "a"); err != nil {
		t.Fatal(err)
	}
	partnerCast := Cast{ID: 6, UserID: partner, SenderDeviceID: 20, ReceiverDeviceID: 21}
	if _, _, err := registry.Cast(partnerCast, "b"); err != nil {
		t.Fatal(err)
	}
	if handed, _ := receiver.last(MessageReceive); handed.Receive.TogetherCredential != "" {
		t.Fatalf("before any session the TV was handed %q", handed.Receive.TogetherCredential)
	}
	parts[owner], parts[partner] = "hosted", "partner's"
	handed := registry.PartChanged(owner, func(castID int64) (int64, bool) { return castID * 10_000, true })
	if len(handed) != 1 || handed[0].ID != 5 {
		t.Fatalf("PartChanged handed %+v again; want cast 5 alone", handed)
	}
	told, _ := receiver.last(MessageReceive)
	if told.Receive.CastID != 5 || told.Receive.FileID != 3 || told.Receive.TogetherCredential != "hosted" ||
		told.Receive.IsTogetherFollower || told.Receive.PositionMs == nil || *told.Receive.PositionMs != 50_000 {
		t.Errorf("the TV was handed %+v; want cast 5 again, film 3 from 50 s, leading the host's session",
			told.Receive)
	}
	if count := partnerReceiver.count(MessageReceive); count != 1 {
		t.Errorf("the partner's TV was handed %d casts; want its one, left alone", count)
	}
}

// A cast ends once its receiver has had no connection for ReceiverGoneAfter, its sender told, and not
// before; a receiver that comes back in time, loaded again, keeps it (D364 §6).
func TestRegistry_ACastEndsOnceItsReceiverIsGoneForTwoMinutes(t *testing.T) {
	t.Parallel()
	fake := newClock()
	registry := NewRegistry(minted(), fake)
	sender := &inbox{}
	registry.Connect(owner, phone, inFront, sender.send)
	tvPage := registry.Connect(owner, television, inFront, (&inbox{}).send)
	ownCast := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	if _, _, err := registry.Cast(ownCast, "a"); err != nil {
		t.Fatal(err)
	}
	registry.Disconnect(tvPage)
	fake.Advance(ReceiverGoneAfter - time.Second)
	if ended := registry.EndGone(); len(ended) != 0 {
		t.Fatalf("a receiver gone %s had its cast ended: %+v", ReceiverGoneAfter-time.Second, ended)
	}
	tvPage = registry.Connect(owner, television, inFront, (&inbox{}).send)
	fake.Advance(time.Hour)
	if ended := registry.EndGone(); len(ended) != 0 {
		t.Fatalf("a receiver that came back had its cast ended: %+v", ended)
	}
	registry.Disconnect(tvPage)
	fake.Advance(ReceiverGoneAfter)
	if ended := registry.EndGone(); len(ended) != 1 || ended[0].ID != 5 {
		t.Fatalf("a receiver gone %s ended %+v; want cast 5", ReceiverGoneAfter, ended)
	}
	if told, _ := sender.last(MessageSending); told.Sending != nil {
		t.Errorf("the sender was told it sends %+v; want none", told.Sending)
	}
	if casts := registry.CastsOf(owner, phone); len(casts) != 0 {
		t.Errorf("the sender still has %+v", casts)
	}
	if told, _ := sender.last(MessageDevices); len(told.ReceivingDeviceIDs) != 0 {
		t.Errorf("the sender was told %v receive; want none, the cast over", told.ReceivingDeviceIDs)
	}
}

// A cast its receiver has not said Use to within UnansweredAfter ends, its sender told it sends none,
// and not before; one accepted goes on however long (D358 §4, principles §28): a receiver never
// handed the question, its sender's page gone, left the sender asking (the owner, 2026-11-03).
func TestRegistry_ACastNeverAnsweredEndsAndOneAcceptedGoesOn(t *testing.T) {
	t.Parallel()
	fake := newClock()
	registry := NewRegistry(minted(), fake)
	sender := &inbox{}
	registry.Connect(owner, phone, inFront, sender.send)
	registry.Connect(owner, television, inFront, (&inbox{}).send)
	registry.Connect(partner, phone, inFront, (&inbox{}).send)
	registry.Connect(partner, television, inFront, (&inbox{}).send)
	unanswered := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	accepted := Cast{ID: 6, UserID: partner, SenderDeviceID: phone, ReceiverDeviceID: television}
	for _, cast := range []Cast{unanswered, accepted} {
		if _, _, err := registry.Cast(cast, "a"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := registry.Accept(partner, television, 6); err != nil {
		t.Fatal(err)
	}
	fake.Advance(UnansweredAfter - time.Second)
	if ended := registry.EndUnanswered(); len(ended) != 0 {
		t.Fatalf("a cast asked %s ago was ended: %+v", UnansweredAfter-time.Second, ended)
	}
	fake.Advance(time.Second)
	if ended := registry.EndUnanswered(); len(ended) != 1 || ended[0].ID != 5 {
		t.Fatalf("after %s the core ended %+v; want cast 5 alone, 6 accepted", UnansweredAfter, ended)
	}
	if told, _ := sender.last(MessageSending); told.Sending != nil {
		t.Errorf("the sender was told it sends %+v; want none", told.Sending)
	}
	if casts := registry.CastsOf(partner, phone); len(casts) != 1 || casts[0].ID != 6 {
		t.Errorf("the accepted cast's sender has %+v; want cast 6 still", casts)
	}
}

// A Watch Together session's end ends every cast whose receiver was handed its person's part in it,
// and no other: a cast in another session goes on (D389 §1). The sender is told it sends none, and
// the person's devices that it receives no more.
func TestRegistry_ASessionsEndEndsTheCastsThatTakeAPartInIt(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	sessions := map[int64]int64{owner: 7, partner: 8}
	registry.TakesPartBy(func(userID int64, _ string) (string, bool, int64) {
		return "part", false, sessions[userID]
	})
	sender := &inbox{}
	partnersTV := television + 10
	registry.Connect(owner, phone, inFront, sender.send)
	registry.Connect(owner, television, inFront, (&inbox{}).send)
	registry.Connect(partner, laptop, inFront, (&inbox{}).send)
	registry.Connect(partner, partnersTV, inFront, (&inbox{}).send)
	ownCast := Cast{ID: 5, UserID: owner, SenderDeviceID: phone, ReceiverDeviceID: television}
	if _, _, err := registry.Cast(ownCast, "a"); err != nil {
		t.Fatal(err)
	}
	partnersCast := Cast{ID: 6, UserID: partner, SenderDeviceID: laptop, ReceiverDeviceID: partnersTV}
	if _, _, err := registry.Cast(partnersCast, "b"); err != nil {
		t.Fatal(err)
	}
	if ended := registry.EndPartIn(7); len(ended) != 1 || ended[0].ID != 5 {
		t.Fatalf("session 7's end ended %+v; want cast 5", ended)
	}
	if told, _ := sender.last(MessageSending); told.Sending != nil {
		t.Errorf("the sender was told it sends %+v; want none", told.Sending)
	}
	if told, _ := sender.last(MessageDevices); len(told.ReceivingDeviceIDs) != 0 {
		t.Errorf("the sender was told %v receive; want none, the cast over", told.ReceivingDeviceIDs)
	}
	if casts := registry.CastsOf(partner, laptop); len(casts) != 1 {
		t.Errorf("the partner's cast in session 8 = %+v; want it going on", casts)
	}
}

// A receiver and a sender's pages are told the item's name as itemName and, for one release, as
// titleName too, which a client of the last release reads (D425 §3, goes next batch).
func TestRegistry_TheItemsNameIsSentByItsOldNameTooForOneRelease(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(minted(), newClock())
	sender, receiver := castWithNothingOn(t, registry)
	if _, err := registry.Play(owner, phone, 5, Film{FileID: 2, ItemName: "Cat"}, nil); err != nil {
		t.Fatal(err)
	}
	handed, _ := receiver.last(MessageReceive)
	told, _ := sender.last(MessageSending)
	for _, message := range []any{handed.Receive, told.Sending} {
		wire, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(wire), `"itemName":"Cat"`) || !strings.Contains(string(wire), `"titleName":"Cat"`) {
			t.Errorf("sent %s; want the name as itemName and titleName", wire)
		}
	}
}
