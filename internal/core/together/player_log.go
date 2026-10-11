// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"time"
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
		"playing", message.Playing, "buffering", message.Buffering, positionMsAttribute, message.PositionMs,
		"behindGroupMs", current.positionAt(now).Milliseconds()-message.PositionMs)
	if !from.playerChangedAt.IsZero() {
		attributes = append(attributes, "lastStateMs", now.Sub(from.playerChangedAt).Milliseconds())
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
	attributes := append(current.deviceAttributes(who, from), "kind", string(note.Kind))
	for _, field := range []struct {
		name  string
		value int64
	}{
		{positionMsAttribute, note.PositionMs},
		{"toMs", note.ToMs},
		{"driftMs", note.DriftMs},
		{"returnCostMs", note.ReturnCostMs},
		{"loadMs", note.LoadMs},
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
			append(current.deviceAttributes(who, from), "partnerDeviceId", note.PartnerDeviceID, "step", note.Why)...)
		return true
	case NotePonged:
		current.logger.InfoContext(ctx, "watch together: a player's pong said where the group is",
			append(current.deviceAttributes(who, from), "roundTripMs", note.RoundTripMs,
				"aheadOfExpectedMs", note.AheadOfExpectedMs, "placing", note.Why)...)
		return true
	case NoteVoiceOff:
		current.logger.InfoContext(ctx, "watch together: voice turned off",
			append(current.deviceAttributes(who, from), "why", note.Why)...)
		return true
	case NoteCorrectsBySeeks:
		current.logger.InfoContext(ctx, "watch together: a player corrects by seeks alone",
			append(current.deviceAttributes(who, from), "returnCostMs", note.ReturnCostMs)...)
		return true
	case NoteOwnMarkExpired:
		current.logger.InfoContext(ctx, "watch together: a player's own move fired no event; its mark was given up",
			append(current.deviceAttributes(who, from), "move", note.Why)...)
		return true
	case NoteLeaderCommand:
		current.logger.InfoContext(ctx, "watch together: a leading player sent a command",
			append(current.deviceAttributes(who, from), "command", note.Why, "recentEvents", note.RecentEvents)...)
		return true
	case NoteDiagnosis:
		current.logger.InfoContext(ctx, "watch together: a device's player said what it holds",
			append(current.deviceAttributes(who, from), "why", note.Why, positionMsAttribute, note.PositionMs,
				"readyState", note.ReadyState, "videoBuffered", note.VideoBuffered,
				"audioBuffered", note.AudioBuffered, "lastError", note.LastError,
				"mapping", note.Mapping, "mediaSourceState", note.MediaSourceState)...)
		return true
	case NotePlayedMuted:
		current.logger.InfoContext(ctx,
			"watch together: the browser would not play the film with sound; it plays muted",
			append(current.deviceAttributes(who, from), positionMsAttribute, note.PositionMs)...)
		return true
	case NoteUnmuted:
		current.logger.InfoContext(ctx, "watch together: a tap turned a muted player's sound on",
			append(current.deviceAttributes(who, from), positionMsAttribute, note.PositionMs)...)
		return true
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected:
	}
	return false
}

// logVoice says how a voice call is going, as its receiver measured it: a call that broke up on a
// bad link said nothing of whether its sound came late, was lost, or went through a relay over TCP
// (the acceptance's congested run, 2026-10-06; D262 §1).
func (current *session) logVoice(ctx context.Context, who *member, from *device, voice VoiceStats) {
	current.logger.InfoContext(ctx, "watch together: a voice call measured", append(current.deviceAttributes(who, from),
		"partnerDeviceId", voice.PartnerDeviceID, "roundTripMs", voice.RoundTripMs,
		"jitterBufferMs", voice.JitterBufferMs, "jitterBufferTargetMs", voice.JitterBufferTargetMs,
		"concealedShare", voice.ConcealedShare, "packetsLost", voice.PacketsLost, "packetsLate", voice.PacketsLate,
		"fecPackets", voice.FECPackets, "path", voice.Path, "receivedLevel", voice.ReceivedLevel,
		"sentLevel", voice.SentLevel, "playback", voice.Playback)...)
}
