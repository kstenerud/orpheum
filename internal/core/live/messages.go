// SPDX-License-Identifier: AGPL-3.0-or-later

package live

// MessageType names one message of a live connection.
type MessageType string

// The messages a device sends.
const (
	// MessageHello, with Channel ChannelLive, opens a live connection: the device's session token,
	// never in the URL (D154). The socket's channels share one hello type (D416 §11, D425 §3).
	MessageHello MessageType = "hello"
	// MessageOldHello opens a live connection as a client of the last release does (D425 §3, goes next
	// batch).
	MessageOldHello MessageType = "live"
	// MessagePing keeps it open, as the session socket's pings do (clientPingEvery).
	MessagePing MessageType = "ping"
	// MessageFront says the device's tab came to the front, or went behind (D338 §2).
	MessageFront MessageType = "front"
)

// The messages the server sends.
const (
	// MessageDevices is the person's devices connected now, those in front and those receiving a
	// cast, sent as the connection opens and each time one of the sets changes.
	MessageDevices MessageType = "devices"
	// MessageReceive hands the device a cast to receive (§4), again each time what it plays changes
	// (D364 §3).
	MessageReceive MessageType = "receive"
	// MessageSending tells the device the cast it sends, or with none that it sends none: as the
	// connection opens, and each time the cast starts, changes or ends (D364 §6, §7).
	MessageSending MessageType = "sending"
	// MessagePong answers a ping, so the device knows its connection lives.
	MessagePong MessageType = "pong"
	// MessageSignedOut tells the device its sign-in ended, the device removed from another (D218 §4):
	// its page asks whether its session still works and, refused, goes to sign in at once rather than
	// at its person's next press (the owner's Fire TV, 2026-10-27).
	MessageSignedOut MessageType = "signedOut"
)

// Inbound is one message a device sends.
type Inbound struct {
	Type MessageType `json:"type"`
	// Channel, in a hello, is ChannelLive.
	Channel string `json:"channel,omitempty"`
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
	// ReceivingDeviceIDs are a devices message's: those of the person's devices receiving a cast now,
	// which no device may cast to until it ends (D386 §1). A receiver gone a while stays one.
	ReceivingDeviceIDs []int64 `json:"receivingDeviceIds,omitempty"`
	// Receive is a receive message's cast.
	Receive *Receive `json:"receive,omitempty"`
	// Sending is a sending message's cast; absent, the device sends none.
	Sending *Sending `json:"sending,omitempty"`
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
	ItemName   string `json:"itemName"`
	// TitleName is ItemName by its old name, for a client of the last release (D425 §3, goes next
	// batch).
	TitleName string `json:"titleName"`
	// PositionMs is where the sender stood; absent, the receiver starts where the person left the
	// film, as a cast from a title's page does.
	PositionMs     *int64 `json:"positionMs,omitempty"`
	FromDeviceID   int64  `json:"fromDeviceId"`
	FromDeviceName string `json:"fromDeviceName"`
	// IsAccepted is whether the receiver said Use to the cast already, asked once (D364 §2).
	IsAccepted bool `json:"isAccepted,omitempty"`
	// TogetherCredential is a credential for the Watch Together session the person is in, if any: one
	// they host, which the receiver plays as their player, leading it, so what they play plays for the
	// group (D364 §11), or, IsTogetherFollower, one they joined as a guest, which the receiver follows as
	// just another follower (D371 §4).
	TogetherCredential string `json:"togetherCredential,omitempty"`
	IsTogetherFollower bool   `json:"isTogetherFollower,omitempty"`
}

// Sending is the cast a device sends, as its pages show it: to which device, what it plays there
// (FileID 0 for nothing, ready between videos, D364 §5), and whether its receiver said Use.
type Sending struct {
	CastID           int64  `json:"castId"`
	ReceiverDeviceID int64  `json:"receiverDeviceId"`
	ReceiverName     string `json:"receiverName"`
	FileID           int64  `json:"fileId"`
	ItemName         string `json:"itemName"`
	// TitleName is ItemName by its old name, for a client of the last release (D425 §3, goes next
	// batch).
	TitleName  string `json:"titleName"`
	DurationMs int64  `json:"durationMs"`
	IsAccepted bool   `json:"isAccepted"`
}

// ChannelLive is the channel a live connection's hello names on the socket (D416 §11).
const ChannelLive = "live"

// Sender takes the messages for one live connection: its socket's writer, which must never block.
type Sender func(message Outbound)
