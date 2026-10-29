// SPDX-License-Identifier: AGPL-3.0-or-later

package live

import (
	"cmp"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

var (
	// ErrNotConnected is a cast to a device that is not one of the person's connected now.
	ErrNotConnected = errors.New("that device is not one of yours connected now")
	// ErrToItself is a cast from a device to itself.
	ErrToItself = errors.New("a device cannot cast to itself")
	// ErrNoCast is a cast that is not, or no longer, one of the device's.
	ErrNoCast = errors.New("no such cast")
	// ErrNotInFront is a cast to a device connected now on which no Orpheum tab is in front (D338 §2).
	ErrNotInFront = errors.New("that device has no Orpheum tab in front")
)

// Credential mints another credential for a cast's session, for a receiver loaded again: the hub's
// CastCredential.
type Credential func(castID, userID int64) (string, error)

// TogetherPart mints a credential for the Watch Together session userID is in, for a cast's receiver
// to take their part in it, and whether it is the host's, which the receiver leads (plans/cast-to-
// device.md §7; D371 §4), or "" when they are in none: the hub's TogetherPart.
type TogetherPart func(userID int64) (credential string, isHost bool)

// ReceiverGoneAfter is how long a cast's receiver may have no connection before its cast ends, the
// sender told (D364 §6): a television switched off or its page closed, not one loaded again or
// reconnecting, which comes back in seconds. The owner's two minutes.
const ReceiverGoneAfter = 2 * time.Minute

// Cast is one device of a person casting to another of theirs: a session with that device, in which
// whatever the person plays on the sender plays on the receiver, until it ends (D337, D364).
type Cast struct {
	// ID is the cast's session of one in Watch Together's hub (together.OpenCast).
	ID               int64
	UserID           int64
	SenderDeviceID   int64
	ReceiverDeviceID int64
	// SenderName and ReceiverName are the devices' names, as the person's list shows them.
	SenderName, ReceiverName string
	// FileID is what the receiver plays now; 0 while it plays nothing, ready between videos (D364 §5).
	FileID int64
	// TitleID and TitleName are the title the file is, which the remote shows; 0 and "" for none.
	TitleID   int64
	TitleName string
	// DurationMs is how long the film plays, for the remote's timeline; 0 when unknown.
	DurationMs int64
	// PositionMs is where the sender stood, or nil to start where the person left the film.
	PositionMs *int64
	// IsAccepted is whether the receiver said Use to the session, asked once (D364 §2).
	IsAccepted bool
}

// Connection is one registered live connection, which Disconnect takes.
type Connection struct {
	ID       int64
	UserID   int64
	DeviceID int64
}

// Registry is every person's live connections and casts, in memory.
type Registry struct {
	credential Credential
	clock      clock.Clock
	// togetherPart, when set, mints the credential of the person's part in a Watch Together session
	// for each hand-off (TakesPartBy).
	togetherPart TogetherPart

	mutex  sync.Mutex
	people map[int64]*person // by user; guarded by mutex
	lastID int64             // the last connection id given; guarded by mutex
	// fronts counts the connections that came to the front, ordering them; guarded by mutex.
	fronts int64
}

// person is one person's live connections and the casts between their devices.
type person struct {
	connections map[int64]*connection
	// casts are by receiving device: each receives one cast at most.
	casts map[int64]*heldCast
}

// connection is one open page of a device.
type connection struct {
	id       int64
	deviceID int64
	send     Sender
	// frontSince orders the connections in front: the registry's count as this one last came to the
	// front, or 0 while its tab is behind another (D338 §2).
	frontSince int64
}

// heldCast is a cast and the connection its receiver was handed it on, 0 while none holds it.
type heldCast struct {
	Cast
	heldBy int64
	// goneSince is when the receiver's last connection closed, zero while it has one (EndGone).
	goneSince time.Time
}

// NewRegistry is an empty registry, minting a receiver's credentials again by credential, and keeping
// time by clockSource.
func NewRegistry(credential Credential, clockSource clock.Clock) *Registry {
	return &Registry{credential: credential, clock: clockSource, people: map[int64]*person{}}
}

// TakesPartBy has each cast's receiver handed, by part, a credential for the Watch Together session
// its person is in, if any, to take their part in it: as their host's player, leading it, so that each
// film the person plays goes on for the group from it (D364 §11; plans/cast-to-device.md §7), or as a
// guest's, just another follower (D371 §4).
func (registry *Registry) TakesPartBy(part TogetherPart) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.togetherPart = part
}

// Connect registers a connection of userID's device deviceID, its tab in front or behind (isInFront),
// sending to send: it is told the person's devices connected now and those in front, and so is every
// other connection of theirs when that changes; a cast to the device that no connection holds is
// handed to it if it is in front (D338 §2).
func (registry *Registry) Connect(userID, deviceID int64, isInFront bool, send Sender) Connection {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		owner = &person{connections: map[int64]*connection{}, casts: map[int64]*heldCast{}}
		registry.people[userID] = owner
	}
	wasConnected, wasInFront := owner.isConnected(deviceID), owner.isInFront(deviceID)
	registry.lastID++
	opened := &connection{id: registry.lastID, deviceID: deviceID, send: send}
	if isInFront {
		registry.fronts++
		opened.frontSince = registry.fronts
	}
	owner.connections[opened.id] = opened
	if held := owner.casts[deviceID]; held != nil {
		held.goneSince = time.Time{}
	}
	send(owner.sendingMessage(deviceID)) // before the devices, so the page knows both at once
	if wasConnected && wasInFront == owner.isInFront(deviceID) {
		send(owner.devicesMessage())
	} else {
		owner.tellDevices()
	}
	registry.handOffWaiting(owner, deviceID)
	return Connection{ID: opened.id, UserID: userID, DeviceID: deviceID}
}

// ShowFront notes that the connection's tab came to the front, or went behind (isInFront false):
// the person's connections are told when their devices in front change, and a cast to the device
// that no connection holds is handed to the tab now in front (D338 §2). A connection that holds a
// cast keeps it behind, playing on.
func (registry *Registry) ShowFront(shown Connection, isInFront bool) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[shown.UserID]
	if owner == nil {
		return
	}
	changed := owner.connections[shown.ID]
	if changed == nil || (changed.frontSince != 0) == isInFront {
		return
	}
	wasInFront := owner.isInFront(shown.DeviceID)
	changed.frontSince = 0
	if isInFront {
		registry.fronts++
		changed.frontSince = registry.fronts
	}
	if wasInFront != owner.isInFront(shown.DeviceID) {
		owner.tellDevices()
	}
	registry.handOffWaiting(owner, shown.DeviceID)
}

// Disconnect removes a connection that ended: its person's other connections are told when its
// device is no longer connected, and a cast it held waits for the device's next connection, ended by
// EndGone if none comes within ReceiverGoneAfter.
func (registry *Registry) Disconnect(gone Connection) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[gone.UserID]
	if owner == nil {
		return
	}
	if _, found := owner.connections[gone.ID]; !found {
		return
	}
	wasInFront := owner.isInFront(gone.DeviceID)
	delete(owner.connections, gone.ID)
	for _, held := range owner.casts {
		if held.heldBy == gone.ID {
			held.heldBy = 0
		}
	}
	if held := owner.casts[gone.DeviceID]; held != nil && !owner.isConnected(gone.DeviceID) {
		held.goneSince = registry.clock.Now()
	}
	if !owner.isConnected(gone.DeviceID) || wasInFront != owner.isInFront(gone.DeviceID) {
		owner.tellDevices()
	}
	registry.handOffWaiting(owner, gone.DeviceID)
	if len(owner.connections) == 0 && len(owner.casts) == 0 {
		delete(registry.people, gone.UserID)
	}
}

// SignedOut tells every connection of userID's device deviceID that its sign-in ended: the device
// was removed, and a page left showing would learn it only at its next request.
func (registry *Registry) SignedOut(userID, deviceID int64) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		return
	}
	for _, each := range owner.connections {
		if each.deviceID == deviceID {
			each.send(Outbound{Type: MessageSignedOut})
		}
	}
}

// Connected is userID's devices connected now, by id, in order.
func (registry *Registry) Connected(userID int64) []int64 {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		return []int64{}
	}
	return owner.connectedDevices()
}

// Cast starts cast, handing it by receiverCredential to the receiver's tab in front, the one that
// came to the front last (D338 §2), and telling the sender's pages it sends it. It replaces the cast
// its receiver had, and the one its sender controlled, which it returns for their sessions to be
// ended, their senders told (D337 §6: the last cast wins). The receiver must be another of the
// person's devices, connected now, with a tab in front.
func (registry *Registry) Cast(cast Cast, receiverCredential string) (replaced []Cast, err error) {
	if cast.ReceiverDeviceID == cast.SenderDeviceID {
		return nil, ErrToItself
	}
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[cast.UserID]
	if owner == nil || !owner.isConnected(cast.ReceiverDeviceID) || cast.ReceiverDeviceID == 0 {
		return nil, ErrNotConnected
	}
	front := owner.frontOf(cast.ReceiverDeviceID)
	if front == nil {
		return nil, ErrNotInFront
	}
	for receiverID, held := range owner.casts {
		if receiverID == cast.ReceiverDeviceID || held.SenderDeviceID == cast.SenderDeviceID {
			replaced = append(replaced, held.Cast)
			delete(owner.casts, receiverID)
		}
	}
	owner.casts[cast.ReceiverDeviceID] = &heldCast{Cast: cast, heldBy: front.id}
	front.send(Outbound{Type: MessageReceive, Receive: registry.receiveOf(cast, receiverCredential)})
	for _, each := range replaced {
		if each.SenderDeviceID != cast.SenderDeviceID {
			owner.tellSending(each.SenderDeviceID)
		}
	}
	owner.tellSending(cast.SenderDeviceID)
	return replaced, nil
}

// CastsOf is the casts userID's device deviceID sends or receives.
func (registry *Registry) CastsOf(userID, deviceID int64) []Cast {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		return []Cast{}
	}
	casts := []Cast{}
	for _, held := range owner.casts {
		if held.SenderDeviceID == deviceID || held.ReceiverDeviceID == deviceID {
			casts = append(casts, held.Cast)
		}
	}
	slices.SortFunc(casts, func(one, other Cast) int { return cmp.Compare(one.ID, other.ID) })
	return casts
}

// End ends the cast castID, at the asking of userID's device deviceID, its sender or its receiver,
// and returns it for its session to be ended.
func (registry *Registry) End(userID, deviceID, castID int64) (Cast, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		return Cast{}, ErrNoCast
	}
	for receiverID, held := range owner.casts {
		if held.ID == castID && (held.SenderDeviceID == deviceID || held.ReceiverDeviceID == deviceID) {
			delete(owner.casts, receiverID)
			owner.tellSending(held.SenderDeviceID)
			if len(owner.connections) == 0 && len(owner.casts) == 0 {
				delete(registry.people, userID)
			}
			return held.Cast, nil
		}
	}
	return Cast{}, ErrNoCast
}

// Film is what a cast plays: the file, the title it is and its name, and how long it plays.
type Film struct {
	FileID     int64
	TitleID    int64
	TitleName  string
	DurationMs int64
}

// Play has the cast castID play film from positionMs, or from where its person left it for nil, at
// the asking of userID's device deviceID, its sender or its receiver: whatever the person plays on the
// sender, and the next episode as a film ends (D364 §3; plans/up-next.md). A film of FileID 0 plays
// nothing: the receiver waits ready for the next (§5). The cast goes on; its receiver is handed it
// again, by a credential of its own, on the connection that holds it, or the next one it opens, and
// its sender's pages are told. It returns the cast as it is now.
func (registry *Registry) Play(userID, deviceID, castID int64, film Film, positionMs *int64) (Cast, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	held := owner.castByEither(castID, deviceID)
	if held == nil {
		return Cast{}, ErrNoCast
	}
	held.FileID, held.TitleID, held.TitleName, held.DurationMs = film.FileID, film.TitleID, film.TitleName,
		film.DurationMs
	held.PositionMs = positionMs
	if holder := owner.connections[held.heldBy]; holder != nil {
		registry.handOff(held, holder)
	}
	owner.tellSending(held.SenderDeviceID)
	return held.Cast, nil
}

// Accept notes that the receiver, userID's device deviceID, said Use to the cast castID, which it is
// asked once (D364 §2), and tells its sender's pages.
func (registry *Registry) Accept(userID, deviceID, castID int64) (Cast, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		return Cast{}, ErrNoCast
	}
	held := owner.casts[deviceID]
	if held == nil || held.ID != castID {
		return Cast{}, ErrNoCast
	}
	held.IsAccepted = true
	owner.tellSending(held.SenderDeviceID)
	return held.Cast, nil
}

// PartChanged hands each cast userID sends again to its receiver, by the connection that holds it,
// as the Watch Together session they are in changed: one started while they cast, which the receiver
// takes their part in, as their host's player leading it with the film it plays (D364 §11, D371 §4,
// D372 §8). Where each cast stands now, in milliseconds, is positionOf's, the film playing on from
// there; false leaves where it was to start. It returns the casts handed again.
func (registry *Registry) PartChanged(userID int64, positionOf func(castID int64) (int64, bool)) []Cast {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		return []Cast{}
	}
	handed := []Cast{}
	for _, held := range owner.casts {
		holder := owner.connections[held.heldBy]
		if holder == nil {
			continue // the next connection of the receiver is handed it, with the part as it is then
		}
		if positionMs, isKnown := positionOf(held.ID); isKnown && held.FileID != 0 {
			held.PositionMs = &positionMs
		}
		registry.handOff(held, holder)
		handed = append(handed, held.Cast)
	}
	slices.SortFunc(handed, func(one, other Cast) int { return cmp.Compare(one.ID, other.ID) })
	return handed
}

// EndGone ends every cast whose receiver has had no connection for ReceiverGoneAfter (D364 §6), its
// sender's pages told, and returns them for their sessions to be ended.
func (registry *Registry) EndGone() []Cast {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	now := registry.clock.Now()
	ended := []Cast{}
	for userID, owner := range registry.people {
		for receiverID, held := range owner.casts {
			if held.goneSince.IsZero() || now.Sub(held.goneSince) < ReceiverGoneAfter {
				continue
			}
			delete(owner.casts, receiverID)
			owner.tellSending(held.SenderDeviceID)
			ended = append(ended, held.Cast)
		}
		if len(owner.connections) == 0 && len(owner.casts) == 0 {
			delete(registry.people, userID)
		}
	}
	slices.SortFunc(ended, func(one, other Cast) int { return cmp.Compare(one.ID, other.ID) })
	return ended
}

// handOffWaiting hands the cast to deviceID that no connection holds, if any, to its tab in front,
// if any: a receiver loaded again, or a tab of it come to the front (D338 §2).
func (registry *Registry) handOffWaiting(owner *person, deviceID int64) {
	held := owner.casts[deviceID]
	if held == nil || held.heldBy != 0 {
		return
	}
	if front := owner.frontOf(deviceID); front != nil {
		registry.handOff(held, front)
	}
}

// handOff hands held to the connection to, by a credential minted for it: a receiver loaded again.
// A credential that cannot be made leaves it for the next connection.
func (registry *Registry) handOff(held *heldCast, to *connection) {
	credential, err := registry.credential(held.ID, held.UserID)
	if err != nil {
		return
	}
	held.heldBy = to.id
	to.send(Outbound{Type: MessageReceive, Receive: registry.receiveOf(held.Cast, credential)})
}

// receiveOf is the receive message of cast, by credential, with a credential for the Watch Together
// session its person is in, if any, and whether the receiver follows it (TakesPartBy).
func (registry *Registry) receiveOf(cast Cast, credential string) *Receive {
	partCredential, isHost := "", false
	if registry.togetherPart != nil {
		partCredential, isHost = registry.togetherPart(cast.UserID)
	}
	return &Receive{
		CastID: cast.ID, Credential: credential, FileID: cast.FileID, TitleName: cast.TitleName,
		PositionMs: cast.PositionMs, FromDeviceID: cast.SenderDeviceID, FromDeviceName: cast.SenderName,
		IsAccepted: cast.IsAccepted, TogetherCredential: partCredential,
		IsTogetherFollower: partCredential != "" && !isHost,
	}
}

// castByEither is the cast castID that deviceID sends or receives; nil for none, or no person.
func (owner *person) castByEither(castID, deviceID int64) *heldCast {
	if owner == nil {
		return nil
	}
	for _, held := range owner.casts {
		if held.ID == castID && (held.SenderDeviceID == deviceID || held.ReceiverDeviceID == deviceID) {
			return held
		}
	}
	return nil
}

// sendingMessage is the sending message of the cast deviceID sends, without one when it sends none.
func (owner *person) sendingMessage(deviceID int64) Outbound {
	for _, held := range owner.casts {
		if held.SenderDeviceID == deviceID {
			return Outbound{Type: MessageSending, Sending: &Sending{
				CastID: held.ID, ReceiverDeviceID: held.ReceiverDeviceID, ReceiverName: held.ReceiverName,
				FileID: held.FileID, TitleName: held.TitleName, DurationMs: held.DurationMs,
				IsAccepted: held.IsAccepted,
			}}
		}
	}
	return Outbound{Type: MessageSending}
}

// tellSending tells every connection of deviceID the cast it sends now, or that it sends none.
func (owner *person) tellSending(deviceID int64) {
	message := owner.sendingMessage(deviceID)
	for _, each := range owner.connections {
		if each.deviceID == deviceID {
			each.send(message)
		}
	}
}

// isConnected is whether any connection of deviceID is open.
func (owner *person) isConnected(deviceID int64) bool {
	for _, each := range owner.connections {
		if each.deviceID == deviceID {
			return true
		}
	}
	return false
}

// isInFront is whether any connection of deviceID has its tab in front.
func (owner *person) isInFront(deviceID int64) bool {
	return owner.frontOf(deviceID) != nil
}

// frontOf is deviceID's connection whose tab came to the front last, of those in front now; nil for
// none (D338 §2).
func (owner *person) frontOf(deviceID int64) *connection {
	var front *connection
	for _, each := range owner.connections {
		if each.deviceID == deviceID && each.frontSince != 0 && (front == nil || each.frontSince > front.frontSince) {
			front = each
		}
	}
	return front
}

// connectedDevices is the person's devices with a connection open, by id, in order.
func (owner *person) connectedDevices() []int64 {
	devices := map[int64]bool{}
	for _, each := range owner.connections {
		devices[each.deviceID] = true
	}
	return slices.Sorted(maps.Keys(devices))
}

// devicesInFront is the person's devices with a tab in front, by id, in order.
func (owner *person) devicesInFront() []int64 {
	devices := map[int64]bool{}
	for _, each := range owner.connections {
		if each.frontSince != 0 {
			devices[each.deviceID] = true
		}
	}
	return slices.Sorted(maps.Keys(devices))
}

// devicesMessage is the devices message of the person's connected devices now, and those in front.
func (owner *person) devicesMessage() Outbound {
	return Outbound{
		Type: MessageDevices, DeviceIDs: owner.connectedDevices(), InFrontDeviceIDs: owner.devicesInFront(),
	}
}

// tellDevices tells every connection of the person their connected devices now.
func (owner *person) tellDevices() {
	message := owner.devicesMessage()
	for _, each := range owner.connections {
		each.send(message)
	}
}
