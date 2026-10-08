// SPDX-License-Identifier: GPL-3.0-or-later

package together

import "slices"

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
	// NoteVoiceStats is a voice call's receiver's own statistics, every ten seconds (D262 §1).
	NoteVoiceStats NoteKind = "voiceStats"
	// NoteVoiceCall is a step a voice call took, so a call that never connects says how far it got.
	NoteVoiceCall NoteKind = "voiceCall"
	// NotePonged is a pong a follower heard: where it said the group is against where the follower
	// expected it, and whether it placed the group anew (D264, D268 §1). A placing kept for minutes
	// has residuals that are the two clocks' skew.
	NotePonged NoteKind = "ponged"
	// NoteVoiceOff is voice turning off, and why: the person left ("left"), or a call could not
	// connect ("unreachable"). Calls of 8 s ended with nothing to say whether a person or a failure
	// ended them (the owner's test, 2026-10-08).
	NoteVoiceOff NoteKind = "voiceOff"
	// NotePlayedMuted is a browser refusing to start the film with sound for want of a recent tap,
	// and the player playing it muted; NoteUnmuted, the person's tap turning the sound on (the
	// owner's test, 2026-10-08, where a guest's iPhone sat silent and nothing said why).
	NotePlayedMuted NoteKind = "playedMuted"
	// NoteDiagnosis is what a player holds when it moved itself over a gap ("movedItself"), stalled
	// ("stalled") or buffered a playlist's first chunk ("bufferedFirst"): an iPhone that jumped 10 s
	// on its own and then sat waiting said nothing of what it held (the owner's test, 2026-10-08).
	NoteDiagnosis NoteKind = "diagnosis"
	// NoteLeaderCommand is a leading player's command, why its kind ("pause", "seek", "play"), with the
	// player's last events: a host's play was stopped 0.4 s later by a pause no one could say the
	// source of (the owner's tests, 2026-10-08).
	// NoteOwnMarkExpired is a seek or pause a follower made itself, Why, firing no event within the
	// time its mark lasts: the mark is given up, so a person's later move is not taken for the
	// follower's own (D268 §4).
	NoteOwnMarkExpired NoteKind = "ownMarkExpired"
	// NoteCorrectsBySeeks is a follower whose rate's returns to its group's cost it ReturnCostMs,
	// more than the drift left alone, correcting by seeks alone from then on (D268 §2).
	NoteCorrectsBySeeks NoteKind = "correctsBySeeks"
	NoteLeaderCommand   NoteKind = "leaderCommand"
	NoteUnmuted         NoteKind = "unmuted"
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
	// ReturnCostMs is what a nudged player learned its rate going back to the group's costs it:
	// some 220 ms on an iPhone, which nudged it again and again (the owner's tests, 2026-10-08).
	ReturnCostMs int64 `json:"returnCostMs,omitempty"`
	// Why is a seek's ("catchUp", "hold", "skip") or a reconnection's ("closed", "silent").
	Why string `json:"why,omitempty"`
	// Track, LoadMs and LoadedKbps are a chunk's: "video" or "audio", how long it took, and how fast.
	Track      string `json:"track,omitempty"`
	LoadMs     int64  `json:"loadMs,omitempty"`
	LoadedKbps int64  `json:"loadedKbps,omitempty"`
	// QualityKbps is the quality a chunk was of, or that the player switched to; LinkEstimateKbps, the
	// player's estimate of its link at the switch.
	QualityKbps      int64 `json:"qualityKbps,omitempty"`
	LinkEstimateKbps int64 `json:"linkEstimateKbps,omitempty"`
	// OfflineMs is how long a reconnecting device was without its socket.
	OfflineMs int64 `json:"offlineMs,omitempty"`
	// Voice is a voiceStats note's call, as its receiver measured it (D262 §1).
	Voice *VoiceStats `json:"voice,omitempty"`
	// PartnerDeviceID is the other device of a voiceCall note's call; its Why, the step.
	PartnerDeviceID int64 `json:"partnerDeviceId,omitempty"`
	// RoundTripMs is a ponged note's pong's round trip; AheadOfExpectedMs, how far ahead of where the
	// follower expected the group the pong said it is (behind when negative); its Why, whether the
	// pong placed the group anew ("placed") or not ("kept").
	RoundTripMs       int64 `json:"roundTripMs,omitempty"`
	AheadOfExpectedMs int64 `json:"aheadOfExpectedMs,omitempty"`
	// ReadyState, VideoBuffered, AudioBuffered, LastError, Mapping and MediaSourceState are a diagnosis
	// note's: the video's readyState, what its picture's and sound's buffers hold, as "from-to"
	// ranges in seconds, the last error its player reported, how its player placed the first chunk
	// of each playlist, and whether an iPhone's media source lets it load ("streaming", "stopped").
	ReadyState       int64  `json:"readyState,omitempty"`
	VideoBuffered    string `json:"videoBuffered,omitempty"`
	AudioBuffered    string `json:"audioBuffered,omitempty"`
	LastError        string `json:"lastError,omitempty"`
	Mapping          string `json:"mapping,omitempty"`
	MediaSourceState string `json:"mediaSourceState,omitempty"`
	// RecentEvents are a leaderCommand note's player's last events, "<event>[(own)]@<monotonic ms>",
	// oldest first.
	RecentEvents string `json:"recentEvents,omitempty"`
}

// diagnosisTextMost is the longest a diagnosis note's text may be: ranges and an error's line.
const diagnosisTextMost = 300

// VoiceStats is one voice call as its receiver measured it since it began: which partner, how
// far the sound came and how long the jitter buffer held it and aimed to, the share of its samples
// concealed apart from silence, packets lost, arriving too late and recovered by FEC, and the path,
// "direct" or the relay's "udp", "tcp" or "tls" (D262 §1, §4).
type VoiceStats struct {
	PartnerDeviceID      int64   `json:"partnerDeviceId"`
	RoundTripMs          int64   `json:"roundTripMs"`
	JitterBufferMs       int64   `json:"jitterBufferMs"`
	JitterBufferTargetMs int64   `json:"jitterBufferTargetMs"`
	ConcealedShare       float64 `json:"concealedShare"`
	PacketsLost          int64   `json:"packetsLost"`
	PacketsLate          int64   `json:"packetsLate"`
	FECPackets           int64   `json:"fecPackets"`
	Path                 string  `json:"path"`
	// ReceivedLevel and SentLevel are how loud, on average from 0 to 1, what the receiver got and
	// what its microphone sent were; Playback, how it plays what it gets: "webAudio", or its audio
	// element while Web Audio is "elementSuspended", "elementInterrupted", "elementClosed", or absent
	// ("element"). A call that carries silence, or plays to no one, says so (the owner's test,
	// 2026-10-08, where an iPhone and an Android phone connected and neither heard the other).
	ReceivedLevel float64 `json:"receivedLevel"`
	SentLevel     float64 `json:"sentLevel"`
	Playback      string  `json:"playback"`
}

// positionMsAttribute names a position, in ms, in a log line.
const positionMsAttribute = "positionMs"

// isNoteWord is whether word may be a note's Why or Track: anything else is a malformed note.
func isNoteWord(word string) bool {
	switch word {
	case "", "catchUp", "hold", "skip", "closed", "silent", "video", "audio", "direct", "udp", "tcp", "tls":
		return true
	}
	return false
}

// isKnown is whether the note is one the server logs: a known kind, carrying only known words.
func (note PlayerNote) isKnown() bool {
	switch note.Kind {
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected:
		return isNoteWord(note.Why) && isNoteWord(note.Track)
	case NoteVoiceStats, NoteVoiceOff, NoteVoiceCall:
		return note.isKnownVoice()
	case NotePonged, NoteOwnMarkExpired, NoteCorrectsBySeeks:
		return note.isKnownFollowing()
	case NotePlayedMuted, NoteUnmuted:
		return note.PositionMs >= 0 && note.Why == "" && note.Track == ""
	case NoteDiagnosis, NoteLeaderCommand:
		return note.isKnownDiagnosis()
	}
	return false
}

// isKnownFollowing is whether a note of how a follower follows its group is one the server logs: a
// pong that placed the group or not, a mark given up on a seek or a pause, or a player that
// corrects by seeks alone.
func (note PlayerNote) isKnownFollowing() bool {
	if note.Track != "" {
		return false
	}
	switch note.Kind {
	case NotePonged:
		return note.RoundTripMs >= 0 && (note.Why == "placed" || note.Why == "kept")
	case NoteOwnMarkExpired:
		return note.Why == "seek" || note.Why == "pause"
	case NoteCorrectsBySeeks:
		return note.ReturnCostMs >= 0 && note.Why == ""
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected, NoteVoiceStats,
		NoteVoiceCall, NoteVoiceOff, NotePlayedMuted, NoteUnmuted, NoteDiagnosis,
		NoteLeaderCommand: // isKnown reads them
	}
	return false
}

// isKnownVoice is whether a voice note is one the server logs: statistics with a known path and
// playback, voice turned off for a known reason, or a call's known step with a partner.
func (note PlayerNote) isKnownVoice() bool {
	switch note.Kind {
	case NoteVoiceStats:
		return note.Voice != nil && isNoteWord(note.Voice.Path) && isVoicePlayback(note.Voice.Playback)
	case NoteVoiceOff:
		return note.Why == "left" || note.Why == "unreachable"
	case NoteVoiceCall:
		return note.PartnerDeviceID > 0 && isVoiceCallStep(note.Why)
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected, NotePonged,
		NotePlayedMuted, NoteUnmuted, NoteDiagnosis, NoteLeaderCommand, NoteOwnMarkExpired,
		NoteCorrectsBySeeks: // not voice's: isKnown reads them
	}
	return false
}

// isKnownDiagnosis is whether a diagnosis or leaderCommand note is one the server logs: why it was sent, a
// known word for its kind, and its text no longer than diagnosisTextMost.
func (note PlayerNote) isKnownDiagnosis() bool {
	whys := []string{"movedItself", "stalled", "bufferedFirst"}
	if note.Kind == NoteLeaderCommand {
		whys = []string{"pause", "seek", "play"}
	}
	return slices.Contains(whys, note.Why) && note.Track == "" && len(note.VideoBuffered) <= diagnosisTextMost &&
		len(note.AudioBuffered) <= diagnosisTextMost && len(note.LastError) <= diagnosisTextMost &&
		len(note.RecentEvents) <= diagnosisTextMost && len(note.Mapping) <= diagnosisTextMost &&
		slices.Contains([]string{"", "streaming", "stopped"}, note.MediaSourceState)
}

// isVoicePlayback is whether word is how a voice call's sound plays, as the web client's
// VoicePlayback names them.
func isVoicePlayback(word string) bool {
	switch word {
	case "webAudio", "elementSuspended", "elementInterrupted", "elementClosed", "element":
		return true
	}
	return false
}

// isVoiceCallStep is whether word is a step a voice call takes, as the web client's VoiceCallStep
// names them.
func isVoiceCallStep(word string) bool {
	switch word {
	case "offered", "answered", "accepted", "connected", "failed", "signalDropped":
		return true
	}
	return false
}

// Inbound is one message a client sends, its fields by its type.
type Inbound struct {
	V    int         `json:"v"`
	Type MessageType `json:"type"`
	// Credential, in a hello, is the membership's (D243 §2); SessionToken, in a hello, a signed-in
	// person's session token, which makes the membership theirs. Recording their viewing waits
	// with D246 §4.
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
	// controls playback (D246 §8), and the roster; every group message's group; and each pong's,
	// where the group is as the server answers (D264).
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
	// SeekOnlyLeaveDriftUpToMs is how far off the group's position a player that corrects by seeks
	// alone is left alone: one whose rate's changes cost it more than LeaveDriftUpToMs (D268 §2).
	SeekOnlyLeaveDriftUpToMs int64 `json:"seekOnlyLeaveDriftUpToMs"`
	// CatchUpBufferPercent is how full a player catching up aims to have its buffer when the group
	// reaches it (D250 §4).
	CatchUpBufferPercent int64 `json:"catchUpBufferPercent"`
	// SyncWaitMs is how long a sync point waits for the players before playing without those not
	// ready (D250 §3): the server's own, told so a client may say so.
	SyncWaitMs int64 `json:"syncWaitMs"`
	// StartLeadMs is how long after a sync point's wait ends every player starts playing: the
	// server's own, as each group message's StartsInMs says what is left of it (D268 §3).
	StartLeadMs int64 `json:"startLeadMs"`
	// SettleAfterChangeMs is how long after it starts playing, finishes a seek or changes its rate
	// a player's position is not read for drift: Safari's stands still for a moment then (D250 §7).
	SettleAfterChangeMs int64 `json:"settleAfterChangeMs"`
	// ShouldNudgeKeepPitch is whether a nudged player's sound keeps its pitch (D250 §7).
	ShouldNudgeKeepPitch bool `json:"shouldNudgeKeepPitch"`
	// ShouldCancelMicrophoneEcho is whether the browser cancels echo on a person's microphone for
	// voice (D63: headphones need none); on Android it makes a call of it.
	ShouldCancelMicrophoneEcho bool `json:"shouldCancelMicrophoneEcho"`
	// ShouldBrowserLevelMicrophone is whether the browser's gain control and noise suppression work on a
	// person's microphone for voice.
	ShouldBrowserLevelMicrophone bool `json:"shouldBrowserLevelMicrophone"`
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
	// StartsInMs is, for a group that is Playing, how long after the message was sent it starts
	// moving from PositionMs: what is left of the start's lead, so every player starts at once
	// (D268 §3). Zero once it moves.
	StartsInMs int64 `json:"startsInMs,omitempty"`
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
