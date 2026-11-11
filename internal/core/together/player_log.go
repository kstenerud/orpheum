// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// slogx.EventKey's values, one per log line that carries it.
const (
	eventPlayerChanged     = "playerChanged"
	eventPlayerNoted       = "playerNoted"
	eventVoiceMeasured     = "voiceMeasured"
	eventVoiceCallStep     = "voiceCallStep"
	eventPongResidual      = "pongResidual"
	eventVoiceOff          = "voiceOff"
	eventMicrophoneChanged = "microphoneChanged"
	eventLoadHeldChanged   = "loadHeldChanged"
	eventCorrectsBySeeks   = "correctsBySeeks"
	eventOwnMarkExpired    = "ownMarkExpired"
	eventLeaderCommand     = "leaderCommand"
	eventPlayerDiagnosis   = "playerDiagnosis"
	eventPlayedMuted       = "playedMuted"
	eventUnmutedTap        = "unmutedTap"
	eventContinueAsked     = "continueAsked"
	eventFullScreen        = "fullScreen"
	eventTakenBack         = "takenBack"
	eventPlayedWithSound   = "playedWithSound"
	eventGroupChanged      = "groupChanged"
	eventGroupFollowed     = "groupFollowed"
	eventFollowedLeader    = "followedLeader"
	eventLeaderChanged     = "leaderChanged"
	eventFollowerCaughtUp  = "followerCaughtUp"
	eventLeaderCaughtUp    = "leaderCaughtUp"
	eventPlayerBuffering   = "playerBuffering"
	eventStallNotWaitedFor = "stallNotWaitedFor"
	eventSoundMoved        = "soundMoved"
	eventPlayedWhileHeld   = "playedWhileHeld"
	eventShownAgain        = "shownAgain"
	eventPlayRefused       = "playRefused"
	eventRunStarted        = "runStarted"
	eventRunEnded          = "runEnded"
)

// deviceAttributes name a device in a log line: its session, its member and itself.
func (current *session) deviceAttributes(who *member, from *device) []any {
	return []any{sessionIDAttribute, current.id, "memberId", who.id, "deviceId", from.id}
}

// logPlayer logs, at debug, a state that says a device's player began or stopped playing or
// buffering: where it is, how far behind the group, and how long its last state lasted. A viewer
// whose film jumped every few seconds left no trace of what its player did (staging, 2026-10-06).
func (current *session) logPlayer(ctx context.Context, now time.Time, who *member, from *device, message Inbound) {
	if message.Type != MessageState ||
		(!from.playerChangedAt.IsZero() && message.IsPlaying == from.playing && message.IsBuffering == from.buffering) {
		return
	}
	attributes := append(current.deviceAttributes(who, from),
		slogx.EventKey, eventPlayerChanged, "playing", message.IsPlaying, "buffering", message.IsBuffering,
		positionMsAttribute, message.PositionMs,
		"behindGroupMs", current.positionAt(now).Milliseconds()-message.PositionMs)
	if !from.playerChangedAt.IsZero() {
		attributes = append(attributes, "lastStateMs", now.Sub(from.playerChangedAt).Milliseconds())
	}
	if message.ReadyOnMs != 0 {
		attributes = append(attributes, "readyOnMs", message.ReadyOnMs, "loadsFaster", message.LoadsFaster)
	}
	current.logger.DebugContext(ctx, "watch together: a device's player changed", attributes...)
	from.playing, from.playerChangedAt = message.IsPlaying, now
}

// logNote logs, at debug, one thing a device's player did, with only its kind's fields; a note
// of no known kind is refused as invalid.
func (current *session) logNote(ctx context.Context, now time.Time, who *member, from *device, note *PlayerNote) {
	if note == nil || !note.isKnown() {
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
		return
	}
	if note.Kind == NoteGroupFollowed {
		current.logFollowed(ctx, now, who, from, note)
		return
	}
	if current.logCatchUp(ctx, now, who, from, note) || current.logAtInfo(ctx, who, from, note) {
		return
	}
	attributes := append(current.deviceAttributes(who, from), slogx.EventKey, eventPlayerNoted,
		"kind", string(note.Kind))
	for _, field := range []struct {
		name  string
		value int64
	}{
		{positionMsAttribute, note.PositionMs},
		{"toMs", note.ToMs},
		{"driftMs", note.DriftMs},
		{"returnCostMs", note.ReturnCostMs},
		{"loadMs", note.LoadMs},
		{"filmMs", note.FilmMs},
		{"loadedKbps", note.LoadedKbps},
		{"qualityKbps", note.QualityKbps},
		{"linkEstimateKbps", note.LinkEstimateKbps},
		{"fromQualityKbps", note.FromQualityKbps},
		{"offlineMs", note.OfflineMs},
	} {
		if field.value != 0 {
			attributes = append(attributes, field.name, field.value)
		}
	}
	if note.Rate != 0 {
		attributes = append(attributes, "rate", note.Rate)
	}
	if note.Why != "" {
		attributes = append(attributes, "why", note.Why)
	}
	if note.LinkSamples != "" {
		attributes = append(attributes, "linkSamples", note.LinkSamples)
	}
	if note.Track != "" {
		attributes = append(attributes, "track", note.Track)
	}
	current.logger.DebugContext(ctx, "watch together: a device's player noted", attributes...)
}

// logCatchUp logs, at info, a follower's move to catch up to its leader (D371 §2), and reports whether
// it did: a seek, why ("catchUp", ahead of where the leader is; "skip", a small one; "hold", to where a
// group standing for its leader stands), from where to where, or a nudge, at which rate, each with
// how far behind the leader it was, behind when positive, and a catch-up with its aim's parts (where
// it expected the group, how far ahead it aimed for its load, by what, and what that rested on), so a
// report can be read from the log; a leader catching up to its group after a stall the group played
// on through, as a follower does, is logged as one too, isLeaderCatchingUp (D374). A leader's own
// seeks, carried out for another device or as it joined, are logNote's.
func (current *session) logCatchUp(
	ctx context.Context, now time.Time, who *member, from *device, note *PlayerNote,
) bool {
	isLeaderCatchingUp := from.id == current.leaderID && !from.catchingUpSince.IsZero()
	if (note.Kind != NoteSought && note.Kind != NoteNudged) ||
		(from.id == current.leaderID && !isLeaderCatchingUp) {
		return false
	}
	attributes := append(current.deviceAttributes(who, from), slogx.EventKey, eventFollowerCaughtUp,
		leaderDeviceIDAttribute, current.leaderID, positionMsAttribute, note.PositionMs)
	if isLeaderCatchingUp {
		attributes = append(attributes, "isLeaderCatchingUp", true)
	}
	if note.Kind == NoteNudged {
		attributes = append(attributes, "by", "nudge", "behindMs", note.DriftMs, "rate", note.Rate)
	} else {
		attributes = append(attributes, "by", "seek", "why", note.Why, "toMs", note.ToMs,
			"behindMs", current.positionAt(now).Milliseconds()-note.PositionMs)
		if note.AimedBy != "" {
			attributes = append(attributes, "groupAtMs", note.GroupAtMs, "aheadForLoadMs", note.AheadForLoadMs,
				"aimedBy", note.AimedBy, "lastLoadMs", note.LastLoadMs, "loadsFaster", note.LoadsFaster)
		}
	}
	current.logger.InfoContext(ctx, "watch together: a follower caught up to its leader", attributes...)
	return true
}

// logFollowed logs, at info, a player moving as a change of its group asked: which change, how long
// after the server announced it the player's word of it arrived, which holds the trip there and back
// and the player's own wait, and how long the player held it and why. A cast's receiver played on
// seven seconds after its remote paused it, and nothing said where those seconds went (the owner,
// 2026-10-28). A change the group has since moved on from has no time kept: only its number is said.
func (current *session) logFollowed(ctx context.Context, now time.Time, who *member, from *device, note *PlayerNote) {
	attributes := append(current.deviceAttributes(who, from), slogx.EventKey, eventGroupFollowed,
		"seq", note.Seq, "heldMs", note.HeldMs, "heldBy", note.Why)
	if note.Seq == current.group.seq {
		attributes = append(attributes, "event", string(current.group.event),
			"afterChangeMs", now.Sub(current.group.changedAt).Milliseconds())
	} else {
		attributes = append(attributes, "groupSeq", current.group.seq)
	}
	current.logger.InfoContext(ctx, "watch together: a player followed its group's change", attributes...)
}

// logAtInfo logs, at info, a note that says what a person asks about, voice and what a player held
// among them, and reports whether it did; a player's corrections and loads are left to logNote.
func (current *session) logAtInfo(ctx context.Context, who *member, from *device, note *PlayerNote) bool {
	switch note.Kind {
	case NoteVoiceStats:
		current.logVoice(ctx, who, from, *note.Voice)
		return true
	case NoteVoiceCall:
		current.logger.InfoContext(ctx, "watch together: a voice call took a step",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventVoiceCallStep,
				"partnerDeviceId", note.PartnerDeviceID, "step", note.Why)...)
		return true
	case NotePonged:
		current.logger.InfoContext(ctx, "watch together: a player's pong said where the group is",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventPongResidual,
				"roundTripMs", note.RoundTripMs, "aheadOfExpectedMs", note.AheadOfExpectedMs, "placing", note.Why)...)
		return true
	case NoteVoiceOff:
		current.logger.InfoContext(ctx, "watch together: voice turned off",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventVoiceOff, "why", note.Why)...)
		return true
	case NoteVoiceTalk:
		current.logger.InfoContext(ctx, "watch together: a person's microphone changed",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventMicrophoneChanged,
				"microphone", note.Why)...)
		return true
	case NoteLoadHeld:
		current.logger.InfoContext(ctx, "watch together: a ready player's loading changed while its group waits",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventLoadHeldChanged, "loading", note.Why)...)
		return true
	case NoteCorrectsBySeeks:
		current.logger.InfoContext(ctx, "watch together: a player corrects by seeks alone",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventCorrectsBySeeks,
				"returnCostMs", note.ReturnCostMs)...)
		return true
	case NoteOwnMarkExpired:
		current.logger.InfoContext(ctx, "watch together: a player's own move fired no event; its mark was given up",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventOwnMarkExpired, "move", note.Why)...)
		return true
	case NoteLeaderCommand:
		current.logger.InfoContext(ctx, "watch together: a leading player sent a command",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventLeaderCommand,
				"command", note.Why, "recentEvents", note.RecentEvents)...)
		return true
	case NoteDiagnosis:
		current.logger.InfoContext(ctx, "watch together: a device's player said what it holds",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventPlayerDiagnosis,
				"why", note.Why, positionMsAttribute, note.PositionMs,
				"readyState", note.ReadyState, "videoBuffered", note.VideoBuffered,
				"audioBuffered", note.AudioBuffered, "lastError", note.LastError,
				"mapping", note.Mapping, "mediaSourceState", note.MediaSourceState)...)
		return true
	case NoteBuffering:
		current.logBuffering(ctx, who, from, note)
		return true
	case NoteSoundMoved:
		current.logSoundMoved(ctx, who, from, note)
		return true
	case NotePlayedMuted, NoteUnmuted, NotePlayedWithSound, NoteFullScreen, NoteContinueAsked,
		NotePlayedWhileHeld, NotePlayRefused, NoteShownAgain, NoteTakenBack:
		current.logSound(ctx, who, from, note)
		return true
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected,
		NoteGroupFollowed: // logNote logs them
	}
	return false
}

// logBuffering logs, at info, a following player beginning to hold its group up while it plays, or
// holding up its wait: the group's state, what holds the player, where it is, its readyState and
// rate, how much it has loaded ahead, and how long since its rate last changed. A Fire TV's buffering
// after each nudge of its rate said none of it, and no stall diagnosis came, each wait's pause ending
// the stall before it was counted; a Firefox held its group's wait for good, its film loaded, and
// nothing said why (the owner's casts, staging, 2026-10-28).
func (current *session) logBuffering(ctx context.Context, who *member, from *device, note *PlayerNote) {
	attributes := append(current.deviceAttributes(who, from), slogx.EventKey, eventPlayerBuffering,
		"groupState", string(current.group.state),
		"why", note.Why, positionMsAttribute, note.PositionMs, "readyState", note.ReadyState,
		"rate", note.Rate, "loadedAheadMs", note.LoadedAheadMs)
	if note.SinceRateChangeMs > 0 {
		attributes = append(attributes, "sinceRateChangeMs", note.SinceRateChangeMs)
	}
	current.logger.InfoContext(ctx, "watch together: a player began holding its group up", attributes...)
}

// logSoundMoved logs, at info, a player moving its sound earlier than its picture: from and to, in ms,
// what moved it, Web Audio's latency as measured then where it was, and whether the move threw the
// sound loaded away; what moved it to where it moved from, and the latency its device remembered,
// each "none" where there was none, since a move made before the log listened has no line. A Fire
// TV's start waited twice as its move for Web Audio's latency threw its sound away, and nothing said
// when or by how much the move changed (the owner's cast, staging, 2026-10-28); its later casts moved
// back to its latency from a latency read too early, and no line said so (the same, 07:47 to 07:51).
func (current *session) logSoundMoved(ctx context.Context, who *member, from *device, note *PlayerNote) {
	attributes := append(current.deviceAttributes(who, from), slogx.EventKey, eventSoundMoved,
		"earlierFromMs", note.EarlierFromMs, "earlierMs", note.EarlierMs, "why", note.Why,
		"flushed", note.IsFlushed, "fromWhy", cmp.Or(note.FromWhy, "none"))
	if note.LatencyMs > 0 {
		attributes = append(attributes, "latencyMs", note.LatencyMs)
	}
	if note.RememberedMs > 0 {
		attributes = append(attributes, "rememberedMs", note.RememberedMs)
	} else {
		attributes = append(attributes, "rememberedMs", "none")
	}
	current.logger.InfoContext(ctx, "watch together: a player moved its sound", attributes...)
}

// logSound logs, at info, a note of a player's sound or screen: the browser refusing the film's sound
// or playing it, the film played while its group held it, a press turning it on, full screen
// entered, refused or left, a guest's page asking for a tap to continue, each with the page's
// presses where it says them (D358), and a page shown again: whether its socket answered, and a play
// the browser refused then (the owner's VQ105, 2026-10-29).
func (current *session) logSound(ctx context.Context, who *member, from *device, note *PlayerNote) {
	attributes := current.deviceAttributes(who, from)
	switch note.Kind {
	case NotePlayedMuted:
		current.logger.InfoContext(ctx,
			"watch together: the browser would not play the film with sound; it plays muted",
			append(attributes, slogx.EventKey, eventPlayedMuted, positionMsAttribute, note.PositionMs,
				"page", note.Page)...)
	case NoteUnmuted:
		current.logger.InfoContext(ctx, "watch together: a press turned a muted player's sound on",
			append(attributes, slogx.EventKey, eventUnmutedTap, positionMsAttribute, note.PositionMs,
				"why", note.Why, "by", note.By, "soundState", note.SoundState)...)
	case NotePlayedWithSound:
		current.logger.InfoContext(ctx, "watch together: the browser played the film with sound",
			append(attributes, slogx.EventKey, eventPlayedWithSound, positionMsAttribute, note.PositionMs,
				"page", note.Page)...)
	case NotePlayedWhileHeld:
		// What played a player its group held, and the group's state then: a Fire TV's cast started
		// with a play, a pause and a play, and its log said only that it played (the owner's film,
		// staging, 2026-10-28).
		current.logger.InfoContext(ctx, "watch together: a player played while its group held it; it is paused",
			append(attributes, slogx.EventKey, eventPlayedWhileHeld, "groupState", string(current.group.state),
				"by", note.Why, positionMsAttribute, note.PositionMs)...)
	case NoteFullScreen:
		current.logger.InfoContext(ctx, "watch together: a player's full screen was entered, refused or left",
			append(attributes, slogx.EventKey, eventFullScreen, "answer", note.Why, "by", note.By)...)
	case NoteTakenBack:
		current.logger.InfoContext(ctx, "watch together: a cast's receiver took its control back",
			append(attributes, slogx.EventKey, eventTakenBack, "by", note.By, "how", note.How)...)
	case NoteContinueAsked:
		current.logger.InfoContext(ctx, "watch together: a guest's page asked for a tap to continue",
			append(attributes, slogx.EventKey, eventContinueAsked, "navigation", note.Why, "page", note.Page)...)
	case NoteShownAgain:
		current.logger.InfoContext(ctx, "watch together: a device's page was shown again and its socket answered",
			append(attributes, slogx.EventKey, eventShownAgain, "socket", note.Why, "roundTripMs", note.RoundTripMs)...)
	case NotePlayRefused:
		current.logger.InfoContext(ctx,
			"watch together: the browser refused a player's play as its page was shown again; it offers a tap",
			append(attributes, slogx.EventKey, eventPlayRefused, "refusal", note.Why,
				positionMsAttribute, note.PositionMs, "page", note.Page)...)
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected, NoteVoiceStats,
		NoteVoiceCall, NoteVoiceOff, NoteVoiceTalk, NotePonged, NoteOwnMarkExpired, NoteCorrectsBySeeks,
		NoteLoadHeld, NoteDiagnosis, NoteLeaderCommand, NoteGroupFollowed, NoteBuffering,
		NoteSoundMoved: // logAtInfo logs them
	}
}

// logVoice says how a voice call is going, as its receiver measured it: a call that broke up on a
// bad link said nothing of whether its sound came late, was lost, or went through a relay over TCP
// (the acceptance's congested run, 2026-10-06; D262 §1).
func (current *session) logVoice(ctx context.Context, who *member, from *device, voice VoiceStats) {
	attributes := append(current.deviceAttributes(who, from),
		slogx.EventKey, eventVoiceMeasured, "partnerDeviceId", voice.PartnerDeviceID, "roundTripMs", voice.RoundTripMs,
		"jitterBufferMs", voice.JitterBufferMs, "jitterBufferTargetMs", voice.JitterBufferTargetMs,
		"concealedShare", voice.ConcealedShare, "packetsLost", voice.PacketsLost, "packetsLate", voice.PacketsLate,
		"fecPackets", voice.FECPackets, "path", voice.Path, "receivedLevel", voice.ReceivedLevel,
		"sentLevel", voice.SentLevel, "playback", voice.Playback, "ringLevel", voice.RingLevel)
	if voice.PartnerGain != nil {
		attributes = append(attributes, "partnerGain", *voice.PartnerGain)
	}
	current.logger.InfoContext(ctx, "watch together: a voice call measured", attributes...)
}
