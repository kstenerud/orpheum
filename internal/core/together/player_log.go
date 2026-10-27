// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"time"
)

// logEvent is the attribute key every log line in this file and in logChange (group.go) carries
// alongside its message: a stable name a test matches, so rewording the message does not break the
// match (test architecture review, 2026-10-26, finding 12). It is not current.group.event (what
// changed the group), a different attribute already named "event".
const logEvent = "logEvent"

// logEvent's values, one per log line that carries it.
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
	eventPlayedWithSound   = "playedWithSound"
	eventGroupChanged      = "groupChanged"
)

// deviceAttributes name a device in a log line: its session, its member and itself.
func (current *session) deviceAttributes(who *member, from *device) []any {
	return []any{"sessionId", current.id, "memberId", who.id, "deviceId", from.id}
}

// logPlayer logs, at debug, a state that says a device's player began or stopped playing or
// buffering: where it is, how far behind the group, and how long its last state lasted. A viewer
// whose film jumped every few seconds left no trace of what its player did (staging, 2026-10-06).
func (current *session) logPlayer(ctx context.Context, now time.Time, who *member, from *device, message Inbound) {
	if message.Type != MessageState ||
		(!from.playerChangedAt.IsZero() && message.Playing == from.playing && message.Buffering == from.buffering) {
		return
	}
	attributes := append(current.deviceAttributes(who, from),
		logEvent, eventPlayerChanged, "playing", message.Playing, "buffering", message.Buffering,
		positionMsAttribute, message.PositionMs,
		"behindGroupMs", current.positionAt(now).Milliseconds()-message.PositionMs)
	if !from.playerChangedAt.IsZero() {
		attributes = append(attributes, "lastStateMs", now.Sub(from.playerChangedAt).Milliseconds())
	}
	if message.StartLagMs != nil {
		attributes = append(attributes, "startLagMs", *message.StartLagMs)
	}
	if message.ReadyOnMs != 0 {
		attributes = append(attributes, "readyOnMs", message.ReadyOnMs, "loadsFaster", message.LoadsFaster)
	}
	current.logger.DebugContext(ctx, "watch together: a device's player changed", attributes...)
	from.playing, from.playerChangedAt = message.Playing, now
}

// logNote logs, at debug, one thing a device's player did, with only its kind's fields; a note
// of no known kind is refused as invalid.
func (current *session) logNote(ctx context.Context, who *member, from *device, note *PlayerNote) {
	if note == nil || !note.isKnown() {
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
		return
	}
	if current.logAtInfo(ctx, who, from, note) {
		return
	}
	attributes := append(current.deviceAttributes(who, from), logEvent, eventPlayerNoted, "kind", string(note.Kind))
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
	if note.Track != "" {
		attributes = append(attributes, "track", note.Track)
	}
	current.logger.DebugContext(ctx, "watch together: a device's player noted", attributes...)
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
			append(current.deviceAttributes(who, from), logEvent, eventVoiceCallStep,
				"partnerDeviceId", note.PartnerDeviceID, "step", note.Why)...)
		return true
	case NotePonged:
		current.logger.InfoContext(ctx, "watch together: a player's pong said where the group is",
			append(current.deviceAttributes(who, from), logEvent, eventPongResidual, "roundTripMs", note.RoundTripMs,
				"aheadOfExpectedMs", note.AheadOfExpectedMs, "placing", note.Why)...)
		return true
	case NoteVoiceOff:
		current.logger.InfoContext(ctx, "watch together: voice turned off",
			append(current.deviceAttributes(who, from), logEvent, eventVoiceOff, "why", note.Why)...)
		return true
	case NoteVoiceTalk:
		current.logger.InfoContext(ctx, "watch together: a person's microphone changed",
			append(current.deviceAttributes(who, from), logEvent, eventMicrophoneChanged, "microphone", note.Why)...)
		return true
	case NoteLoadHeld:
		current.logger.InfoContext(ctx, "watch together: a ready player's loading changed while its group waits",
			append(current.deviceAttributes(who, from), logEvent, eventLoadHeldChanged, "loading", note.Why)...)
		return true
	case NoteCorrectsBySeeks:
		current.logger.InfoContext(ctx, "watch together: a player corrects by seeks alone",
			append(current.deviceAttributes(who, from), logEvent, eventCorrectsBySeeks,
				"returnCostMs", note.ReturnCostMs)...)
		return true
	case NoteOwnMarkExpired:
		current.logger.InfoContext(ctx, "watch together: a player's own move fired no event; its mark was given up",
			append(current.deviceAttributes(who, from), logEvent, eventOwnMarkExpired, "move", note.Why)...)
		return true
	case NoteLeaderCommand:
		current.logger.InfoContext(ctx, "watch together: a leading player sent a command",
			append(current.deviceAttributes(who, from), logEvent, eventLeaderCommand,
				"command", note.Why, "recentEvents", note.RecentEvents)...)
		return true
	case NoteDiagnosis:
		current.logger.InfoContext(ctx, "watch together: a device's player said what it holds",
			append(current.deviceAttributes(who, from), logEvent, eventPlayerDiagnosis,
				"why", note.Why, positionMsAttribute, note.PositionMs,
				"readyState", note.ReadyState, "videoBuffered", note.VideoBuffered,
				"audioBuffered", note.AudioBuffered, "lastError", note.LastError,
				"mapping", note.Mapping, "mediaSourceState", note.MediaSourceState)...)
		return true
	case NotePlayedMuted, NoteUnmuted, NotePlayedWithSound, NoteFullScreen, NoteContinueAsked:
		current.logSound(ctx, who, from, note)
		return true
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected:
	}
	return false
}

// logSound logs, at info, a note of a player's sound or screen: the browser refusing the film's sound
// or playing it, a press turning it on, full screen entered, refused or left, and a guest's page asking for
// a tap to continue, each with the page's presses where it says them (D358).
func (current *session) logSound(ctx context.Context, who *member, from *device, note *PlayerNote) {
	attributes := current.deviceAttributes(who, from)
	switch note.Kind {
	case NotePlayedMuted:
		current.logger.InfoContext(ctx,
			"watch together: the browser would not play the film with sound; it plays muted",
			append(attributes, logEvent, eventPlayedMuted, positionMsAttribute, note.PositionMs, "page", note.Page)...)
	case NoteUnmuted:
		current.logger.InfoContext(ctx, "watch together: a press turned a muted player's sound on",
			append(attributes, logEvent, eventUnmutedTap, positionMsAttribute, note.PositionMs,
				"why", note.Why, "by", note.By, "soundState", note.SoundState)...)
	case NotePlayedWithSound:
		current.logger.InfoContext(ctx, "watch together: the browser played the film with sound",
			append(attributes, logEvent, eventPlayedWithSound, positionMsAttribute, note.PositionMs,
				"page", note.Page)...)
	case NoteFullScreen:
		current.logger.InfoContext(ctx, "watch together: a player's full screen was entered, refused or left",
			append(attributes, logEvent, eventFullScreen, "answer", note.Why, "by", note.By)...)
	case NoteContinueAsked:
		current.logger.InfoContext(ctx, "watch together: a guest's page asked for a tap to continue",
			append(attributes, logEvent, eventContinueAsked, "navigation", note.Why, "page", note.Page)...)
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected, NoteVoiceStats,
		NoteVoiceCall, NoteVoiceOff, NoteVoiceTalk, NotePonged, NoteOwnMarkExpired, NoteCorrectsBySeeks,
		NoteLoadHeld, NoteDiagnosis, NoteLeaderCommand: // logAtInfo logs them
	}
}

// logVoice says how a voice call is going, as its receiver measured it: a call that broke up on a
// bad link said nothing of whether its sound came late, was lost, or went through a relay over TCP
// (the acceptance's congested run, 2026-10-06; D262 §1).
func (current *session) logVoice(ctx context.Context, who *member, from *device, voice VoiceStats) {
	attributes := append(current.deviceAttributes(who, from),
		logEvent, eventVoiceMeasured, "partnerDeviceId", voice.PartnerDeviceID, "roundTripMs", voice.RoundTripMs,
		"jitterBufferMs", voice.JitterBufferMs, "jitterBufferTargetMs", voice.JitterBufferTargetMs,
		"concealedShare", voice.ConcealedShare, "packetsLost", voice.PacketsLost, "packetsLate", voice.PacketsLate,
		"fecPackets", voice.FECPackets, "path", voice.Path, "receivedLevel", voice.ReceivedLevel,
		"sentLevel", voice.SentLevel, "playback", voice.Playback, "ringLevel", voice.RingLevel)
	if voice.PartnerGain != nil {
		attributes = append(attributes, "partnerGain", *voice.PartnerGain)
	}
	current.logger.InfoContext(ctx, "watch together: a voice call measured", attributes...)
}
