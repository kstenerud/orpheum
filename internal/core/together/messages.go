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
	// MessageNote is one thing a device's player did, for the server's log alone (PlayerNote).
	MessageNote MessageType = "note"
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

// NoteKind is what a PlayerNote says a device's player did.
type NoteKind string

// The kinds of note, as the web client's PlayerNote names them.
const (
	NoteNudged          NoteKind = "nudged"
	NoteSought          NoteKind = "sought"
	NoteChunkLoaded     NoteKind = "chunkLoaded"
	NoteQualitySwitched NoteKind = "qualitySwitched"
	NoteReconnected     NoteKind = "reconnected"
)

// PlayerNote is one thing a device's player did, which the server logs and keeps nowhere: why a
// viewer's film jumped or froze, where nothing said what her iPhone did between its jumps (the
// owner's test, 2026-10-06). Its fields are its kind's; the others are zero.
type PlayerNote struct {
	Kind NoteKind `json:"kind"`
	// PositionMs is where the player was; ToMs, where a seek took it.
	PositionMs int64 `json:"positionMs,omitempty"`
	ToMs       int64 `json:"toMs,omitempty"`
	// DriftMs is how far behind the group a nudge found it (ahead when negative); Rate, the rate it
	// played at for the nudge.
	DriftMs int64   `json:"driftMs,omitempty"`
	Rate    float64 `json:"rate,omitempty"`
	// Why is a seek's ("catchUp", "hold") or a reconnection's ("closed", "silent").
	Why string `json:"why,omitempty"`
	// Track, LoadMs and Kbps are a chunk's: "video" or "audio", how long it took, and how fast.
	Track  string `json:"track,omitempty"`
	LoadMs int64  `json:"loadMs,omitempty"`
	Kbps   int64  `json:"kbps,omitempty"`
	// QualityKbps is the quality a chunk was of, or that the player switched to; EstimateKbps, the
	// player's estimate of its link at the switch.
	QualityKbps  int64 `json:"qualityKbps,omitempty"`
	EstimateKbps int64 `json:"estimateKbps,omitempty"`
	// OfflineMs is how long a reconnecting device was without its socket.
	OfflineMs int64 `json:"offlineMs,omitempty"`
}

// positionMsAttribute names a position, in ms, in a log line.
const positionMsAttribute = "positionMs"

// isNoteWord is whether word may be a note's Why or Track: anything else is a malformed note.
func isNoteWord(word string) bool {
	switch word {
	case "", "catchUp", "hold", "closed", "silent", "video", "audio":
		return true
	}
	return false
}

// isKnown is whether the note is one the server logs: a known kind, carrying only known words.
func (note PlayerNote) isKnown() bool {
	switch note.Kind {
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected:
		return isNoteWord(note.Why) && isNoteWord(note.Track)
	}
	return false
}

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
	RTTMs int64 `json:"rttMs,omitempty"`
	// CannotKeepUp, in a state, is the device saying even its lowest quality is beyond its link,
	// which the group is told in the roster (D247 §8); nil while it keeps up.
	CannotKeepUp *CannotKeepUp `json:"cannotKeepUp,omitempty"`
	Command      CommandKind   `json:"command,omitempty"`
	Rate         float64       `json:"rate,omitempty"`
	FileID       int64         `json:"fileId,omitempty"`
	BasedOnSeq   int64         `json:"basedOnSeq,omitempty"`
	Reason       SuspendReason `json:"reason,omitempty"`
	// Note is a note's: one thing the device's player did, for the server's log.
	Note *PlayerNote `json:"note,omitempty"`
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
	// Tuning is welcome's: how the device follows the group, from the settings (D250 §1).
	Tuning *Tuning `json:"tuning,omitempty"`
}

// Tuning is how a device follows its group, the settings' numbers as each device is welcomed with
// them, so they are tuned while testing without a new client (D250 §1).
type Tuning struct {
	// LeaveDriftUpToMs is how far off the group's position a player is left alone.
	LeaveDriftUpToMs int64 `json:"leaveDriftUpToMs"`
	// NudgeMostPercent is the most a nudge plays faster or slower, as a percent of the group's rate.
	NudgeMostPercent int64 `json:"nudgeMostPercent"`
	// SeekDriftBeyondMs is how far off a player seeks rather than nudges.
	SeekDriftBeyondMs int64 `json:"seekDriftBeyondMs"`
	// CatchUpBufferPercent is how full a player catching up aims to have its buffer when the group
	// reaches it (D250 §4).
	CatchUpBufferPercent int64 `json:"catchUpBufferPercent"`
	// SyncWaitMs is how long a sync point waits for the players before playing without those not
	// ready (D250 §3): the server's own, told so a client may say so.
	SyncWaitMs int64 `json:"syncWaitMs"`
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
	// Event is what changed, by the member ByMemberID; a client words it (D188).
	Event      EventKind `json:"event,omitempty"`
	ByMemberID int64     `json:"byMemberId,omitempty"`
	// SeekedFromMs is where a seek jumped from; PositionMs is where it jumped to.
	SeekedFromMs int64 `json:"seekedFromMs,omitempty"`
}

// EventKind is what changed the group: a code a client words for its viewer, never words.
type EventKind string

// The events.
const (
	EventPlayed        EventKind = "played"
	EventPaused        EventKind = "paused"
	EventSeeked        EventKind = "seeked"
	EventChangedRate   EventKind = "changedRate"
	EventChangedItem   EventKind = "changedItem"
	EventStopped       EventKind = "stopped"
	EventEveryoneReady EventKind = "everyoneReady"
	// EventContinuedWithout is the group playing on while a member's player catches up alone: it
	// never stops for one member (D250 §2).
	EventContinuedWithout EventKind = "continuedWithout"
)

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
	// CannotKeepUp is the device's word that even its lowest quality is beyond its link (D247 §8).
	CannotKeepUp *CannotKeepUp `json:"cannotKeepUp,omitempty"`
}

// CannotKeepUp is a device whose link carries less than its lowest quality needs, with both
// numbers, which everyone in the session is told (D247 §8): LinkKbps 0 when it was not measured.
type CannotKeepUp struct {
	LinkKbps   int64 `json:"linkKbps,omitempty"`
	LowestKbps int64 `json:"lowestKbps"`
}
