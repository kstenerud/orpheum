// SPDX-License-Identifier: AGPL-3.0-or-later

package live

// MessageType names one message of a live connection.
type MessageType string

// The messages a device sends.
const (
	// MessageHello opens a live connection: the device's session token, never in the URL (D154).
	MessageHello MessageType = "live"
	// MessagePing keeps it open, as the session socket's pings do (clientPingEvery).
	MessagePing MessageType = "ping"
	// MessageFront says the device's tab came to the front, or went behind (D338 §2).
	MessageFront MessageType = "front"
)

// The messages the server sends.
const (
	// MessageDevices is the person's devices connected now, sent as the connection opens and each
	// time the set changes.
	MessageDevices MessageType = "devices"
	// MessageReceive hands the device a cast to receive (§4).
	MessageReceive MessageType = "receive"
	// MessagePong answers a ping, so the device knows its connection lives.
	MessagePong MessageType = "pong"
)

// Inbound is one message a device sends.
type Inbound struct {
	Type MessageType `json:"type"`
	// SessionToken, in a hello, is the device's sign-in.
	SessionToken string `json:"sessionToken,omitempty"`
	// Client, in a hello, is the web client's script the page runs, for the log (agent-failures A28).
	Client string `json:"client,omitempty"`
	// TMono is a ping's monotonic time, which its pong carries back.
	TMono int64 `json:"tMono,omitempty"`
	// IsBehind, in a hello or a front message, is whether the tab is behind another, hidden, rather
	// than in front, where a cast to the device goes (D338 §2).
	IsBehind bool `json:"behind,omitempty"`
}

// Outbound is one message the server sends.
type Outbound struct {
	Type MessageType `json:"type"`
	// DeviceIDs are a devices message's: the person's devices connected now, this one among them.
	DeviceIDs []int64 `json:"deviceIds,omitempty"`
	// InFrontDeviceIDs are a devices message's: those of DeviceIDs with an Orpheum tab in front,
	// which a cast can go to (D338 §2).
	InFrontDeviceIDs []int64 `json:"inFrontDeviceIds,omitempty"`
	// Receive is a receive message's cast.
	Receive *Receive `json:"receive,omitempty"`
	// TMono is a pong's: its ping's.
	TMono int64 `json:"tMono,omitempty"`
}

// Receive is a cast as its receiver is handed it: the session of one it leads, by a credential of
// its own, the film, its name and where it starts, and the device that cast it, whose name it shows
// as it asks whether to play it (D358 §1).
type Receive struct {
	CastID     int64  `json:"castId"`
	Credential string `json:"membershipCredential"`
	FileID     int64  `json:"fileId"`
	TitleName  string `json:"titleName"`
	// PositionMs is where the sender stood; absent, the receiver starts where the person left the
	// film, as a cast from a title's page does.
	PositionMs     *int64 `json:"positionMs,omitempty"`
	FromDeviceID   int64  `json:"fromDeviceId"`
	FromDeviceName string `json:"fromDeviceName"`
}

// Sender takes the messages for one live connection: its socket's writer, which must never block.
type Sender func(message Outbound)
