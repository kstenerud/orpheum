// SPDX-License-Identifier: AGPL-3.0-or-later

package live

import (
	"cmp"
	"errors"
	"maps"
	"slices"
	"sync"
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

// Cast is one device of a person casting to another of theirs (D337).
type Cast struct {
	// ID is the cast's session of one in Watch Together's hub (together.OpenCast).
	ID               int64
	UserID           int64
	SenderDeviceID   int64
	ReceiverDeviceID int64
	// SenderName and ReceiverName are the devices' names, as the person's list shows them.
	SenderName, ReceiverName string
	FileID                   int64
	// TitleID and TitleName are the title the file is, which the remote shows; 0 and "" for none.
	TitleID   int64
	TitleName string
	// DurationMs is how long the film plays, for the remote's timeline; 0 when unknown.
	DurationMs int64
	// PositionMs is where the sender stood, or nil to start where the person left the film.
	PositionMs *int64
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
}

// NewRegistry is an empty registry, minting a receiver's credentials again by credential.
func NewRegistry(credential Credential) *Registry {
	return &Registry{credential: credential, people: map[int64]*person{}}
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
// device is no longer connected, and a cast it held waits for the device's next connection.
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
	if !owner.isConnected(gone.DeviceID) || wasInFront != owner.isInFront(gone.DeviceID) {
		owner.tellDevices()
	}
	registry.handOffWaiting(owner, gone.DeviceID)
	if len(owner.connections) == 0 && len(owner.casts) == 0 {
		delete(registry.people, gone.UserID)
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
// came to the front last (D338 §2). It replaces the cast its receiver had, and the one its sender
// controlled, which it returns for their sessions to be ended (D337 §6: the last cast wins). The
// receiver must be another of the person's devices, connected now, with a tab in front.
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
	front.send(Outbound{Type: MessageReceive, Receive: receiveOf(cast, receiverCredential)})
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

// MoveOn has the cast castID play film from where its person left it, at the asking of userID's
// device deviceID, its sender or its receiver: the next episode as the film ends (plans/up-next.md).
// The cast and its session of one go on; its receiver is handed it again, by a credential of its
// own, on the connection that holds it, or the next one it opens. It returns the cast as it is now.
func (registry *Registry) MoveOn(userID, deviceID, castID int64, film Film) (Cast, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		return Cast{}, ErrNoCast
	}
	for _, held := range owner.casts {
		if held.ID != castID || (held.SenderDeviceID != deviceID && held.ReceiverDeviceID != deviceID) {
			continue
		}
		held.FileID, held.TitleID, held.TitleName, held.DurationMs = film.FileID, film.TitleID, film.TitleName,
			film.DurationMs
		held.PositionMs = nil
		if holder := owner.connections[held.heldBy]; holder != nil {
			registry.handOff(held, holder)
		}
		return held.Cast, nil
	}
	return Cast{}, ErrNoCast
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
	to.send(Outbound{Type: MessageReceive, Receive: receiveOf(held.Cast, credential)})
}

// receiveOf is the receive message of cast, by credential.
func receiveOf(cast Cast, credential string) *Receive {
	return &Receive{
		CastID: cast.ID, Credential: credential, FileID: cast.FileID, TitleName: cast.TitleName,
		PositionMs:   cast.PositionMs,
		FromDeviceID: cast.SenderDeviceID, FromDeviceName: cast.SenderName,
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
