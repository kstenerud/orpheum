// SPDX-License-Identifier: AGPL-3.0-or-later

package relay

import (
	"net"
	"sync"

	"github.com/pion/stun/v3"
	pionturn "github.com/pion/turn/v4"

	"github.com/kstenerud/orpheum/discovery/relaycred"
)

// pickedAtMost bounds the keys picked and not yet taken: a request whose integrity was checked but
// that pion answered before asking for its key (a stale nonce) leaves one behind.
const pickedAtMost = 4096

// pickKey is a request by the address it came from and the username it names.
type pickKey struct{ source, username string }

// secretPicker finds which of the relay's secrets a request's credential was made under. The TURN
// library asks its AuthHandler for one key and checks the request's integrity with it, but a relay holds several
// secrets (the plan's §1), so the picker reads each request as it arrives, before pion does, tries
// each secret's key against its MESSAGE-INTEGRITY, and keeps the one that holds for the
// AuthHandler, which pion calls in the same read loop.
type secretPicker struct {
	secrets []relaycred.Secret
	mutex   sync.Mutex
	picked  map[pickKey][]byte
}

func newSecretPicker(secrets []relaycred.Secret) *secretPicker {
	return &secretPicker{secrets: secrets, picked: map[pickKey][]byte{}}
}

// inspect keeps the key a STUN request from source holds its integrity under, if any secret's does.
func (picker *secretPicker) inspect(frame []byte, source net.Addr) {
	if source == nil || !stun.IsMessage(frame) {
		return
	}
	message := &stun.Message{Raw: append([]byte(nil), frame...)}
	if message.Decode() != nil || !message.Contains(stun.AttrMessageIntegrity) {
		return
	}
	var username stun.Username
	if username.GetFrom(message) != nil {
		return
	}
	for _, secret := range picker.secrets {
		named := username.String()
		key := pionturn.GenerateAuthKey(named, relaycred.Realm, relaycred.Password(secret, named))
		// Check compares with hmac.Equal, in constant time.
		if stun.MessageIntegrity(key).Check(message) == nil {
			picker.keep(pickKey{source: source.String(), username: named}, key)
			return
		}
	}
}

// keep keeps the key picked for a request.
func (picker *secretPicker) keep(request pickKey, key []byte) {
	picker.mutex.Lock()
	defer picker.mutex.Unlock()
	if len(picker.picked) >= pickedAtMost {
		picker.picked = map[pickKey][]byte{}
	}
	picker.picked[request] = key
}

// take is the key inspect kept for the request from source naming username, once.
func (picker *secretPicker) take(source net.Addr, username string) ([]byte, bool) {
	if source == nil {
		return nil, false
	}
	picker.mutex.Lock()
	defer picker.mutex.Unlock()
	asked := pickKey{source: source.String(), username: username}
	key, found := picker.picked[asked]
	delete(picker.picked, asked)
	return key, found
}

// inspectedPackets is the UDP listener, each datagram shown to the picker as pion reads it.
type inspectedPackets struct {
	net.PacketConn
	picker *secretPicker
}

// ReadFrom reads a datagram and shows it to the picker.
func (packets *inspectedPackets) ReadFrom(buffer []byte) (int, net.Addr, error) {
	count, source, err := packets.PacketConn.ReadFrom(buffer)
	if err != nil {
		return count, source, err
	}
	packets.picker.inspect(buffer[:count], source)
	return count, source, nil
}

// inspectedStreams is a TCP or TLS listener whose every connection shows its frames to the picker.
type inspectedStreams struct {
	net.Listener
	picker *secretPicker
}

// Accept is the next connection, its reads one TURN frame each, shown to the picker.
func (streams *inspectedStreams) Accept() (net.Conn, error) {
	conn, err := streams.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &inspectedConn{Conn: conn, frames: pionturn.NewSTUNConn(conn), picker: streams.picker}, nil
}

// inspectedConn reads a stream one TURN frame at a time, through pion's own framing, so the picker
// sees each request whole; pion's framing over it then finds one frame in each read.
type inspectedConn struct {
	net.Conn
	frames *pionturn.STUNConn
	picker *secretPicker
}

// Read reads the next frame and shows it to the picker.
func (conn *inspectedConn) Read(buffer []byte) (int, error) {
	count, source, err := conn.frames.ReadFrom(buffer)
	if err != nil {
		return count, err
	}
	conn.picker.inspect(buffer[:count], source)
	return count, nil
}
