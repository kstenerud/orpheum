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

// Connect registers a connection of userID's device deviceID, sending to send: it is told the
// person's devices connected now, and so is every other connection of theirs when that changes; a
// cast to the device that no connection holds is handed to it.
func (registry *Registry) Connect(userID, deviceID int64, send Sender) Connection {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		owner = &person{connections: map[int64]*connection{}, casts: map[int64]*heldCast{}}
		registry.people[userID] = owner
	}
	wasConnected := owner.isConnected(deviceID)
	registry.lastID++
	opened := &connection{id: registry.lastID, deviceID: deviceID, send: send}
	owner.connections[opened.id] = opened
	if wasConnected {
		send(owner.devicesMessage())
	} else {
		owner.tellDevices()
	}
	if held := owner.casts[deviceID]; held != nil && held.heldBy == 0 {
		registry.handOff(held, opened)
	}
	return Connection{ID: opened.id, UserID: userID, DeviceID: deviceID}
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
	delete(owner.connections, gone.ID)
	for _, held := range owner.casts {
		if held.heldBy == gone.ID {
			held.heldBy = 0
		}
	}
	if !owner.isConnected(gone.DeviceID) {
		owner.tellDevices()
	}
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

// Cast starts cast, handing it to its receiver's newest connection by receiverCredential. It replaces the cast its
// receiver had, and the one its sender controlled, which it returns for their sessions to be
// ended (D337 §6: the last cast wins). The receiver must be another of the person's devices,
// connected now.
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
	for receiverID, held := range owner.casts {
		if receiverID == cast.ReceiverDeviceID || held.SenderDeviceID == cast.SenderDeviceID {
			replaced = append(replaced, held.Cast)
			delete(owner.casts, receiverID)
		}
	}
	held := &heldCast{Cast: cast}
	owner.casts[cast.ReceiverDeviceID] = held
	held.heldBy = owner.newestOf(cast.ReceiverDeviceID).id
	newest := owner.connections[held.heldBy]
	newest.send(Outbound{Type: MessageReceive, Receive: receiveOf(cast, receiverCredential)})
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
		CastID: cast.ID, Credential: credential, FileID: cast.FileID, PositionMs: cast.PositionMs,
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

// newestOf is deviceID's newest connection, which must have one.
func (owner *person) newestOf(deviceID int64) *connection {
	var newest *connection
	for _, each := range owner.connections {
		if each.deviceID == deviceID && (newest == nil || each.id > newest.id) {
			newest = each
		}
	}
	return newest
}

// connectedDevices is the person's devices with a connection open, by id, in order.
func (owner *person) connectedDevices() []int64 {
	devices := map[int64]bool{}
	for _, each := range owner.connections {
		devices[each.deviceID] = true
	}
	return slices.Sorted(maps.Keys(devices))
}

// devicesMessage is the devices message of the person's connected devices now.
func (owner *person) devicesMessage() Outbound {
	return Outbound{Type: MessageDevices, DeviceIDs: owner.connectedDevices()}
}

// tellDevices tells every connection of the person their connected devices now.
func (owner *person) tellDevices() {
	message := owner.devicesMessage()
	for _, each := range owner.connections {
		each.send(message)
	}
}
