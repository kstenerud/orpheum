// SPDX-License-Identifier: GPL-3.0-or-later

package together

// MessageType names one message of the session WebSocket (cut-a §5.1).
type MessageType string

// The messages a client sends.
const (
	MessageHello   MessageType = "hello"
	MessagePing    MessageType = "ping"
	MessageState   MessageType = "state"
	MessageCommand MessageType = "command"
	MessageSuspend MessageType = "suspend"
	MessageResume  MessageType = "resume"
	MessageRTC     MessageType = "rtc"
)

// The messages the server sends.
const (
	MessageWelcome  MessageType = "welcome"
	MessagePong     MessageType = "pong"
	MessageGroup    MessageType = "group"
	MessageRoster   MessageType = "roster"
	MessageRejected MessageType = "rejected"
	MessageEnded    MessageType = "ended"
)

// ProtocolVersion is the version of the messages, every message's v (cut-a §5.1).
const ProtocolVersion = 2

// DeviceRole is what a device does for its member (D120): plays the film, carries voice, or both.
type DeviceRole string

// The device roles.
const (
	RolePlayback DeviceRole = "playback"
	RoleVoice    DeviceRole = "voice"
	RoleBoth     DeviceRole = "both"
)

// plays is whether a device of the role plays the film, and so takes part in the group's sync.
func (role DeviceRole) plays() bool { return role == RolePlayback || role == RoleBoth }

// CommandKind is one thing a command asks of the group.
type CommandKind string

// The commands.
const (
	CommandPlay       CommandKind = "play"
	CommandPause      CommandKind = "pause"
	CommandSeek       CommandKind = "seek"
	CommandRate       CommandKind = "rate"
	CommandChangeItem CommandKind = "changeItem"
	// CommandStop ends what plays: everyone's screen goes back to black (D246 §5).
	CommandStop CommandKind = "stop"
)

// SuspendReason is why a device detaches from the group for a while (watch-together.md, Sync).
type SuspendReason string

// The reasons a device suspends.
const (
	SuspendStallRecovery           SuspendReason = "stallRecovery"
	SuspendAudioSessionInterrupted SuspendReason = "audioSessionInterrupted"
	SuspendUserActionRequired      SuspendReason = "userActionRequired"
	SuspendUserPausedLocally       SuspendReason = "userPausedLocally"
	SuspendUserChangingTime        SuspendReason = "userIsChangingCurrentTime"
)

// Inbound is one message a client sends, its fields by its type.
type Inbound struct {
	V    int         `json:"v"`
	Type MessageType `json:"type"`
	// Credential, in a hello, is the membership's (D243 §2); SessionToken, in a hello, a signed-in
	// person's session token, which makes the membership theirs and records their viewing.
	Credential   string     `json:"credential,omitempty"`
	SessionToken string     `json:"sessionToken,omitempty"`
	Role         DeviceRole `json:"role,omitempty"`
	CanSetRate   bool       `json:"canSetRate,omitempty"`
	// TMono is the sender's monotonic time in milliseconds: a ping's, and a state's.
	TMono      int64 `json:"tMono,omitempty"`
	PositionMs int64 `json:"positionMs,omitempty"`
	Playing    bool  `json:"playing,omitempty"`
	Buffering  bool  `json:"buffering,omitempty"`
	// RTTMs, in a state, is the shortest round trip the device's pings have measured: the others
	// see it in the roster, and only the device can measure it (D66).
	RTTMs      int64         `json:"rttMs,omitempty"`
	Command    CommandKind   `json:"command,omitempty"`
	Rate       float64       `json:"rate,omitempty"`
	FileID     int64         `json:"fileId,omitempty"`
	BasedOnSeq int64         `json:"basedOnSeq,omitempty"`
	Reason     SuspendReason `json:"reason,omitempty"`
	// ToDeviceID, SDP and Candidate are an rtc message's: voice signalling relayed to another
	// member's device.
	ToDeviceID int64  `json:"toDeviceId,omitempty"`
	SDP        string `json:"sdp,omitempty"`
	Candidate  string `json:"candidate,omitempty"`
}

// Outbound is one message the server sends.
type Outbound struct {
	V    int         `json:"v"`
	Type MessageType `json:"type"`
	Seq  int64       `json:"seq,omitempty"`
	// Welcome's: the session, the device's own member, whether it is the host's, who alone
	// controls playback (D246 §8), and the roster; and every group message's group.
	SessionID int64        `json:"sessionId,omitempty"`
	MemberID  int64        `json:"memberId,omitempty"`
	DeviceID  int64        `json:"deviceId,omitempty"`
	IsHost    bool         `json:"isHost,omitempty"`
	Group     *GroupView   `json:"group,omitempty"`
	Roster    []MemberView `json:"roster,omitempty"`
	TMono     int64        `json:"tMono,omitempty"`
	// Rejected's: the command's basedOnSeq, and why.
	BasedOnSeq int64  `json:"basedOnSeq,omitempty"`
	Code       string `json:"code,omitempty"`
	// Rtc's: the device it is from, and its signalling.
	FromDeviceID int64  `json:"fromDeviceId,omitempty"`
	SDP          string `json:"sdp,omitempty"`
	Candidate    string `json:"candidate,omitempty"`
	// Ended's reason.
	Reason string `json:"reason,omitempty"`
}

// GroupState is the group's play state (watch-together.md, Sync): Waiting is paused until every
// playing device is ready.
type GroupState string

// The group states.
const (
	GroupIdle    GroupState = "Idle"
	GroupWaiting GroupState = "Waiting"
	GroupPaused  GroupState = "Paused"
	GroupPlaying GroupState = "Playing"
)

// GroupView is the group as a client is told it: its state, where it is, how fast, what plays, and
// the named event that made it so (D188).
type GroupView struct {
	State      GroupState `json:"state"`
	PositionMs int64      `json:"positionMs"`
	Rate       float64    `json:"rate"`
	FileID     int64      `json:"fileId"`
	// Event names the change: "paused", "played", "seeked from 723000", "buffering", "left", by the
	// member ByMemberID.
	Event      string `json:"event,omitempty"`
	ByMemberID int64  `json:"byMemberId,omitempty"`
}

// MemberView is one member in the roster: a person, with their devices.
type MemberView struct {
	ID      int64        `json:"id"`
	Name    string       `json:"name"`
	IsHost  bool         `json:"isHost,omitempty"`
	Devices []DeviceView `json:"devices"`
}

// DeviceView is one of a member's devices, its role and its health.
type DeviceView struct {
	ID          int64         `json:"id"`
	Role        DeviceRole    `json:"role"`
	IsBuffering bool          `json:"isBuffering,omitempty"`
	Suspended   SuspendReason `json:"suspended,omitempty"`
	RTTMs       int64         `json:"rttMs,omitempty"`
}
