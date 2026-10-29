// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"encoding/json"
	"sync"
)

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
	// MessageStanding is what a device's page waits on before its film can play (Standing): shown
	// to everyone in the roster, it changes nothing of whom the group waits for (D246 §6).
	MessageStanding MessageType = "standing"
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

// DeviceRole is the part of a member a device fulfils (D120, D338 §4): a member is the person, one
// member however many of their devices are connected, and each device is attached in a role. There
// are two: the video viewer, which plays the film and takes part in the group's sync, and the control
// panel with voice, which plays nothing, carries the person's voice and sends their commands, as a
// cast's remote does. A device that is both, the one device most people watch on, is RoleBoth.
type DeviceRole string

// The device roles.
const (
	// RoleVideo is the video viewer: the device plays the film, as a cast's receiver does.
	RoleVideo DeviceRole = "video"
	// RolePanel is the control panel with voice: the device plays nothing, carries voice and
	// commands, as a cast's remote does.
	RolePanel DeviceRole = "panel"
	// RoleBoth is one device in both roles.
	RoleBoth DeviceRole = "both"
)

// plays is whether a device of the role plays the film, and so takes part in the group's sync.
func (role DeviceRole) plays() bool { return role == RoleVideo || role == RoleBoth }

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

// Standing is what a device's page waits on before its film can play, which only the page knows.
type Standing string

// The standings a page says.
const (
	// StandingNone is a page that waits on nothing of its person's.
	StandingNone Standing = ""
	// StandingNeedsTap is a page waiting for a tap before it may play with sound.
	StandingNeedsTap Standing = "needsTap"
)

// isKnown is whether standing is one a page may say.
func (standing Standing) isKnown() bool {
	switch standing {
	case StandingNone, StandingNeedsTap:
		return true
	}
	return false
}

// DeviceStatus is where a playing device stands with its group, in a word, for everyone's roster:
// what its page waits on, or its player's state, the server's own reading (the owner's Watch
// Together session, 2026-10-18: a guest not yet ready to play was shown as nothing at all).
type DeviceStatus string

// The statuses a roster gives a device.
const (
	// StatusNeedsTap is what the device's page waits on (Standing).
	StatusNeedsTap DeviceStatus = "needsTap"
	// StatusLoading is a player loading its first buffer of the film, not yet once ready.
	StatusLoading DeviceStatus = "loading"
	// StatusCatchingUp is a follower waiting for data, which catches up to the leader once it has it;
	// the group plays on without it (D371 §2).
	StatusCatchingUp DeviceStatus = "catchingUp"
	// StatusBuffering is the leader waiting for data: the group stands where it is (D371 §1).
	StatusBuffering DeviceStatus = "buffering"
	// StatusSuspended is a player its device suspended, as when its person changes the time.
	StatusSuspended DeviceStatus = "suspended"
	// StatusWatching is a player playing along with the group.
	StatusWatching DeviceStatus = "watching"
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
	// NoteLoadHeld is a ready player holding its loading while its group waits ("held"), or going on
	// as it plays ("goesOn"), so the others have the upload (D283 §2).
	NoteLoadHeld NoteKind = "loadHeld"
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
	// NoteVoiceTalk is a person's microphone, while they hear voice: opened ("talking"), closed
	// ("stopped"), muted or unmuted (D281 §2, §3).
	NoteVoiceTalk NoteKind = "voiceTalk"
	// NotePlayedMuted is a browser refusing to start the film with sound for want of a recent tap,
	// and the player playing it muted, Page what the page's activation was then; NoteUnmuted, the
	// person's tap turning the sound on, By how they pressed (the owner's test, 2026-10-08, where a
	// guest's iPhone sat silent and nothing said why; a cast's receiver on a Fire TV, 2026-10-27,
	// silent with nothing to say whether its page had ever been pressed).
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
	// NoteGroupFollowed is a player moving as a change of its group, Seq, asks, HeldMs after the
	// change reached it, Why what held it that long ("atOnce" for nothing): a cast's receiver went on
	// playing seven seconds after its remote paused it, and nothing said whether the pause came late
	// or the player held it (the owner, 2026-10-28).
	NoteGroupFollowed NoteKind = "groupFollowed"
	// NoteBuffering is a following player beginning to hold its group up while the group plays, or
	// holding up its wait (once for each wait and each change of what holds it), Why
	// what holds it ("notCaughtUp", "seeking", "notReady", or a refill begun by "waiting", "seeking" or
	// "notReady"), at PositionMs, with its ReadyState, Rate, LoadedAheadMs and SinceRateChangeMs: a Fire
	// TV reported buffering some 350 ms after each nudge of its rate, again and again, its chunks
	// loading fast, and nothing said what it held or that its rate had just changed (the owner's cast,
	// staging, 2026-10-28).
	NoteBuffering     NoteKind = "buffering"
	NoteLeaderCommand NoteKind = "leaderCommand"
	NoteUnmuted       NoteKind = "unmuted"
	// NoteContinueAsked is a guest's page asking for a tap to continue, Why how the browser came to
	// show it ("navigate", "reload", "back_forward", "prerender") and Page what else it knows: an
	// iPhone that joined by its keyboard's Go was asked straight after, and nothing said whether the
	// page had loaded again (the owner's test, 2026-10-13).
	NoteContinueAsked NoteKind = "continueAsked"
	// NoteFullScreen is a player's stage put full screen ("entered") or the browser refusing it
	// ("refused"), By the press that asked, absent as the player opened: full screen needs a press
	// made moments before, and a cast's receiver on a Fire TV stayed out of it with nothing said why
	// (the owner, 2026-10-27). A receiver whose full screen the browser left unasked, by a Fire TV's
	// Back or Escape, which takes control back as Back does (D338 §6), notes it "left", with no press.
	NoteFullScreen NoteKind = "fullScreen"
	// NoteSoundMoved is a player moving its sound from EarlierFromMs earlier than its picture to
	// EarlierMs, Why what moved it ("remembered", the latency its device kept; "measured", Web Audio's
	// latency measured, LatencyMs; "offset", its person's offset; "direct", its sound found playing
	// with no Web Audio after all), and IsFlushed whether it threw the sound loaded away; FromWhy what
	// moved it to EarlierFromMs, and RememberedMs the latency its device remembered: a Fire TV's
	// start waited twice as its move for Web Audio's latency threw its sound away, and nothing said
	// when or by how much the move changed (the owner's cast, staging, 2026-10-28).
	NoteSoundMoved NoteKind = "soundMoved"
	// NotePlayedWithSound is a cast's receiver's film starting with its sound on, at PositionMs, Page
	// what the page's activation was then: with NotePlayedMuted, whether the press that answered the
	// cast let it play with sound (D358 §2).
	NotePlayedWithSound NoteKind = "playedWithSound"
	// NotePlayedWhileHeld is a following player's element beginning to play, at PositionMs, while its
	// group did not play and no command came of it, so its follower paused it again; Why what began it:
	// "autoplay", the element's own autoplay as it loaded, or "page", a play on the page, a press or a
	// media key. A Fire TV's cast started with a play, a pause and a play, and nothing said what played
	// it first (the owner's Marrowgate Family, staging, 2026-10-28).
	NotePlayedWhileHeld NoteKind = "playedWhileHeld"
	// NoteShownAgain is a device's page shown again, its socket answering the ping it sent then, Why
	// "alive", after RoundTripMs; one that did not answer in time is a NoteReconnected whose Why is
	// "shown". An S23 unlocked sat 20 s on a socket that heard nothing, and nothing said so (the owner's
	// VQ105, 2026-10-29).
	NoteShownAgain NoteKind = "shownAgain"
	// NotePlayRefused is a following player's play, as its page was shown again, refused, Why the
	// browser's error ("NotAllowedError", "AbortError", "NotSupportedError", "other") or "stayedPaused"
	// (it said it played and stayed paused), at PositionMs, Page the page's activation then: it plays
	// muted and its page offers a tap (the owner, 2026-10-29: a phone unlocked may not let its film
	// play on).
	NotePlayRefused NoteKind = "playRefused"
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
	// Why is a seek's ("catchUp", "hold", "skip"; a leader's "join", "command" or "undone", D371 §1,
	// D372 §4) or a
	// reconnection's ("closed", "silent"); a qualitySwitched note's is free text, why the player's
	// bitrate controller chose the quality ("first", a chunk given up with its bytes and wait and the
	// estimate after, a step up, the link), at most diagnosisTextMost.
	Why string `json:"why,omitempty"`
	// Track, LoadMs and LoadedKbps are a chunk's: "video" or "audio", how long it took, and how fast.
	Track      string `json:"track,omitempty"`
	LoadMs     int64  `json:"loadMs,omitempty"`
	LoadedKbps int64  `json:"loadedKbps,omitempty"`
	// FilmMs is how much film a chunk plays: with LoadMs, whether the link keeps up with the film.
	FilmMs int64 `json:"filmMs,omitempty"`
	// QualityKbps is the quality a chunk was of, or that the player switched to; LinkEstimateKbps, the
	// player's estimate of its link at the switch; FromQualityKbps, the quality it switched from, 0 for
	// the first it plays, which is no switch.
	QualityKbps      int64 `json:"qualityKbps,omitempty"`
	LinkEstimateKbps int64 `json:"linkEstimateKbps,omitempty"`
	FromQualityKbps  int64 `json:"fromQualityKbps,omitempty"`
	// LinkSamples are a qualitySwitched note's link samples when its controller chose the quality,
	// each its quality, bytes, time to arrive and wait for its first byte, at most diagnosisTextMost:
	// a Fire TV fell from 9018 to 696 kb/s 2.3 s into Nosferatu, its link estimated at 69 kb/s though its
	// chunks had loaded at 14-18 Mb/s, and nothing in the log said why (the owner's cast, staging,
	// 2026-10-28).
	LinkSamples string `json:"linkSamples,omitempty"`
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
	// SoundState is an unmuted note's sound: whether the element is muted, its volume, whether it is
	// paused, and an iPhone's audio session; its Why, "after" for the state some seconds after the
	// tap. An iPhone that stayed silent after its tap could not say why (the owner's VQ30, 2026-10-12).
	SoundState string `json:"soundState,omitempty"`
	// Page is a continueAsked note's page: how long since it loaded, and whether it joined itself; a
	// playedMuted note's, whether the page has ever been pressed and is now, how long since it
	// loaded and how the browser came to show it.
	Page string `json:"page,omitempty"`
	// By is how the person pressed for an unmuted or fullScreen note: "tap", "click" or "key".
	By string `json:"by,omitempty"`
	// Seq is a groupFollowed note's change of the group, by its number; HeldMs, how long after the
	// change reached the player it moved.
	Seq    int64 `json:"seq,omitempty"`
	HeldMs int64 `json:"heldMs,omitempty"`
	// LoadedAheadMs is a buffering note's film loaded ahead of where the player is; SinceRateChangeMs,
	// how long since its follower last changed its rate, absent when it never has.
	LoadedAheadMs     int64 `json:"loadedAheadMs,omitempty"`
	SinceRateChangeMs int64 `json:"sinceRateChangeMs,omitempty"`
	// EarlierFromMs and EarlierMs are a soundMoved note's move of its sound, from and to, in ms
	// earlier than its picture; LatencyMs, Web Audio's latency as measured then, 0 where it was not;
	// IsFlushed, whether the move threw the sound loaded away.
	EarlierFromMs int64 `json:"earlierFromMs,omitempty"`
	EarlierMs     int64 `json:"earlierMs,omitempty"`
	LatencyMs     int64 `json:"latencyMs,omitempty"`
	IsFlushed     bool  `json:"isFlushed,omitempty"`
	// FromWhy is what moved a soundMoved note's sound to EarlierFromMs, empty where nothing had;
	// RememberedMs, the latency its device remembered as its levelling began, 0 where it had none.
	// A move made before the player's log listened is told no one: the Fire TV's casts of
	// 2026-10-28 left only the moves back to its latency, from where no line said.
	FromWhy      string `json:"fromWhy,omitempty"`
	RememberedMs int64  `json:"rememberedMs,omitempty"`
	// GroupAtMs, AheadForLoadMs and AimedBy are a catch-up seek's aim: where its player expected the
	// group then, how far ahead of the group it aimed for its load, and by what ("loaded", the film held
	// there; "lastLoad", its last seek's load; "fill", that and its link's time to fill its buffer,
	// D250 §4; "mostAhead", as far as a person waits); LastLoadMs and LoadsFaster, what that rested
	// on: how long its last seek took to load, and the film loaded per second of its link, 0 with no
	// measure. A catch-up a minute ahead of its group said nothing of why (the owner's Chrome,
	// staging, 2026-10-29).
	GroupAtMs      int64   `json:"groupAtMs,omitempty"`
	AheadForLoadMs int64   `json:"aheadForLoadMs,omitempty"`
	AimedBy        string  `json:"aimedBy,omitempty"`
	LastLoadMs     int64   `json:"lastLoadMs,omitempty"`
	LoadsFaster    float64 `json:"loadsFaster,omitempty"`
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
	// ("element"); or "elementBlocked", its element refused for want of a tap while Web Audio does not
	// run, so no one hears it (the owner's test, 2026-10-11). A call that carries silence, or plays
	// to no one, says so (the owner's test, 2026-10-08, where an iPhone and an Android phone
	// connected and neither heard the other).
	ReceivedLevel float64 `json:"receivedLevel"`
	SentLevel     float64 `json:"sentLevel"`
	Playback      string  `json:"playback"`
	// RingLevel is what the device's Talk button's ring read of its voice going out, from 0 to 1, as
	// the call was measured: a ring that stayed still on an Android phone hosting said nothing of
	// whether it read nothing or showed nothing (the owner's test, 2026-10-11).
	RingLevel float64 `json:"ringLevel,omitempty"`
	// PartnerGain is the volume the receiver's person set for the partner's voice, 1 as it came: a
	// voice heard with its slider at 0 said nothing of whether the slider reached the call (the
	// owner's test, 2026-10-12). Absent from a client that does not say.
	PartnerGain *float64 `json:"partnerGain,omitempty"`
}

// positionMsAttribute names a position, in ms, in a log line.
const positionMsAttribute = "positionMs"

// sessionIDAttribute and leaderDeviceIDAttribute are the log's names for a session and its leader.
const (
	sessionIDAttribute      = "sessionId"
	leaderDeviceIDAttribute = "leaderDeviceId"
)

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
	// Client, in a hello, is the web client's script the page runs, "index-DCLi77_b.js", logged so
	// a device's report says which client made it: an iPhone's tab ran an old one through a day of
	// updates, and nothing said so (agent-failures A28).
	Client string `json:"client,omitempty"`
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
	// Standing, in a standing, is what the device's page waits on, StandingNone for nothing.
	Standing Standing `json:"standing,omitempty"`
	// ReadyOnMs and LoadsFaster, in a state, are how much film loaded ahead the player is ready on
	// now, 3 s after a seek on a link that keeps up well, else 10 s (D283 §1), and how many times
	// faster than it plays its last chunk of picture loaded: whether the shorter rule applied.
	ReadyOnMs   int64   `json:"readyOnMs,omitempty"`
	LoadsFaster float64 `json:"loadsFaster,omitempty"`
	// Playback, in a state, is how the device's player plays the film (D371 §4); nil where it does
	// not say.
	Playback *Playback `json:"playback,omitempty"`
	// PageHidden, in a state, is the device's page hidden by its browser, its film paused: a
	// handheld's, a locked phone's. Such a device cannot lead (D372 §5).
	PageHidden bool        `json:"pageHidden,omitempty"`
	Command    CommandKind `json:"command,omitempty"`
	Rate       float64     `json:"rate,omitempty"`
	// FileID is a changeItem's film to put on; in a state, the film the device's player has
	// loaded, 0 when it has none: its player gone, its film handed to a cast's receiver, or not yet
	// loaded. Only a device whose player has the session's film contends for the lead (D374 §1).
	FileID     int64         `json:"fileId,omitempty"`
	BasedOnSeq int64         `json:"basedOnSeq,omitempty"`
	Reason     SuspendReason `json:"reason,omitempty"`
	// Note is a note's: one thing the device's player did, for the server's log.
	Note *PlayerNote `json:"note,omitempty"`
	// ToDeviceID, SDP and Candidate are an rtc message's: voice signalling relayed to another
	// member's device.
	ToDeviceID int64  `json:"toDeviceId,omitempty"`
	SDP        string `json:"sdp,omitempty"`
	Candidate  string `json:"candidate,omitempty"`
	// keyframeMs is the keyframe at or before a seek's PositionMs, when isKeyframeKnown: the hub
	// finds it before it is held (Handle); never on the wire.
	keyframeMs      int64
	isKeyframeKnown bool
}

// Outbound is one message the server sends.
type Outbound struct {
	V    int         `json:"v"`
	Type MessageType `json:"type"`
	Seq  int64       `json:"seq,omitempty"`
	// Welcome's: the session, the device's own member, whether it is the host's, who alone
	// controls playback (D246 §8), and the roster; every group message's group; and each pong's,
	// where the group is as the server answers (D264).
	SessionID int64 `json:"sessionId,omitempty"`
	MemberID  int64 `json:"memberId,omitempty"`
	DeviceID  int64 `json:"deviceId,omitempty"`
	IsHost    bool  `json:"isHost,omitempty"`
	// Rights is welcome's: what the device's member may do to playback (D271 §4).
	Rights *Rights      `json:"rights,omitempty"`
	Group  *GroupView   `json:"group,omitempty"`
	Roster []MemberView `json:"roster,omitempty"`
	TMono  int64        `json:"tMono,omitempty"`
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
	// encoded is a broadcast's JSON, made by the first device's writer and shared by the rest: a
	// roster went to every device, each encoding its own copy (the audit of 2026-10-25, R2). Nil for
	// a message sent to one device.
	encoded *sharedEncoding
}

// sharedEncoding is one message's JSON, made once for every device it is sent to.
type sharedEncoding struct {
	once    sync.Once
	encoded []byte
	err     error
}

// MarshalJSON is the message's JSON: a broadcast's made once, for every device it goes to.
func (message Outbound) MarshalJSON() ([]byte, error) {
	type fields Outbound // the same fields, without this method
	if message.encoded == nil {
		return json.Marshal(fields(message))
	}
	shared := message.encoded
	shared.once.Do(func() {
		shared.encoded, shared.err = json.Marshal(fields(message))
	})
	return shared.encoded, shared.err
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
	// SettleAfterChangeMs is how long after it starts playing, finishes a seek or changes its rate
	// a player's position is not read for drift: Safari's stands still for a moment then (D250 §7).
	SettleAfterChangeMs int64 `json:"settleAfterChangeMs"`
	// SnapSeeksBeyondMs is how far a seek must jump to be snapped to a keyframe (D285 §1): the
	// server's own, not sent; zero keeps the session's.
	SnapSeeksBeyondMs int64 `json:"-"`
	// ShouldNudgeKeepPitch is whether a nudged player's sound keeps its pitch (D250 §7).
	ShouldNudgeKeepPitch bool `json:"shouldNudgeKeepPitch"`
	// ShouldCancelMicrophoneEcho is whether the browser cancels echo on a person's microphone for
	// voice (D63: headphones need none); on Android it makes a call of it.
	ShouldCancelMicrophoneEcho bool `json:"shouldCancelMicrophoneEcho"`
	// ShouldBrowserLevelMicrophone is whether the browser's gain control and noise suppression work on a
	// person's microphone for voice.
	ShouldBrowserLevelMicrophone bool `json:"shouldBrowserLevelMicrophone"`
}

// GroupState is the group's play state (watch-together.md, Sync): Waiting is to play, standing where
// its leader is until the leader plays (D371 §1).
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
	EventPlayed      EventKind = "played"
	EventPaused      EventKind = "paused"
	EventSeeked      EventKind = "seeked"
	EventChangedRate EventKind = "changedRate"
	EventChangedItem EventKind = "changedItem"
	EventStopped     EventKind = "stopped"
	// EventLeaderPlays is the group playing from where its leader plays, the leader playing after
	// the group stood for it: put on, played, sought or stalled (D371 §1).
	EventLeaderPlays EventKind = "leaderPlays"
	// EventWaitingFor is the group standing where its leader is, the leader paused by its browser, or
	// a cast's receiver stalled: the group's position is the leader's (D371 §1). A Watch Together
	// group waits for no one's stall (D373 §1).
	EventWaitingFor EventKind = "waitingFor"
	// EventEveryoneLeft is the group paused where it stands, its leader gone with no one left who
	// can lead: every player gone, its page elsewhere or hidden (D372 §7).
	EventEveryoneLeft EventKind = "everyoneLeft"
)

// Rights are what a guest may do to playback, each given or withheld by the host in their link
// (D271 §4). Changing what plays, stopping it and its rate stay the host's.
type Rights struct {
	CanPlayPause bool `json:"canPlayPause"`
	CanSeek      bool `json:"canSeek"`
}

// everyRight is the host's rights, and what a session's first link starts with (D271 §4).
func everyRight() Rights { return Rights{CanPlayPause: true, CanSeek: true} }

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
	// Status is where a playing device stands with its group; empty for a voice device, and for
	// one with nothing to wait on while the session plays nothing.
	Status DeviceStatus `json:"status,omitempty"`
	// IsLeader is whether the device leads the session: the screen that is the film, whose position
	// is the group's (D371 §1).
	IsLeader bool `json:"isLeader,omitempty"`
	// LeadsBecause is why the leader leads, the why of its leaderChanged log line with a gone
	// leader's goneBecause after a colon ("leaderGone:hidden"); empty for every other device. A
	// person's debug view shows it (plans: Show debug tools).
	LeadsBecause string `json:"leadsBecause,omitempty"`
	// Playback is how the device's player says it plays the film, which a cast's remote shows
	// (D371 §4); nil until it says.
	Playback *Playback `json:"playback,omitempty"`
}

// CannotKeepUp is a device whose link carries less than its lowest quality needs, with both
// numbers, which everyone in the session is told (D247 §8): LinkKbps 0 when it was not measured.
type CannotKeepUp struct {
	LinkKbps   int64 `json:"linkKbps,omitempty"`
	LowestKbps int64 `json:"lowestKbps"`
}
