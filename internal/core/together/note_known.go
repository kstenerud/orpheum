// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import "slices"

// Which of a device's notes the server logs: a note of a known kind, carrying only the words its kind
// knows, and nothing else, which is refused as invalid (player_log.go).

// isNoteWord is whether word may be a note's Why or Track: anything else is a malformed note.
func isNoteWord(word string) bool {
	switch word {
	case "", "catchUp", "hold", "skip", "join", "command", "undone", "closed", "silent", "shown", "back", "video",
		"audio", "direct", "udp", "tcp", "tls":
		return true
	}
	return false
}

// isCatchUpAimedBy is whether word may say what a catch-up seek's aim rested on (PlayerNote.AimedBy),
// or "" for none.
func isCatchUpAimedBy(word string) bool {
	return slices.Contains([]string{"", "loaded", "lastLoad", "fill", "mostAhead"}, word)
}

// isKnown is whether the note is one the server logs: a known kind, carrying only known words.
func (note PlayerNote) isKnown() bool {
	switch note.Kind {
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteReconnected:
		return isNoteWord(note.Why) && isNoteWord(note.Track) && isCatchUpAimedBy(note.AimedBy)
	case NoteQualitySwitched:
		return note.Track == "" && len(note.Why) <= diagnosisTextMost && len(note.LinkSamples) <= diagnosisTextMost
	case NoteVoiceStats, NoteVoiceOff, NoteVoiceCall, NoteVoiceTalk:
		return note.isKnownVoice()
	case NotePonged, NoteOwnMarkExpired, NoteCorrectsBySeeks, NoteLoadHeld, NoteGroupFollowed:
		return note.isKnownFollowing()
	case NoteBuffering:
		return note.isKnownBuffering()
	case NoteSoundMoved:
		return note.isKnownSoundMoved()
	case NotePlayedMuted, NoteUnmuted, NoteContinueAsked, NotePlayedWithSound:
		return note.isKnownSound()
	case NoteShownAgain, NotePlayRefused:
		return note.isKnownShownAgain()
	case NoteFullScreen, NotePlayedWhileHeld:
		return note.isKnownStart()
	case NoteDiagnosis, NoteLeaderCommand:
		return note.isKnownDiagnosis()
	}
	return false
}

// isKnownStart is whether a note of how a film started showing is one the server logs: its stage put
// full screen, refused or left, by a known press or none; or a play its group held, by a known cause.
func (note PlayerNote) isKnownStart() bool {
	if note.Track != "" {
		return false
	}
	if note.Kind == NotePlayedWhileHeld {
		return note.PositionMs >= 0 && (note.Why == "autoplay" || note.Why == "page") && note.By == ""
	}
	return isFullScreenWhy(note.Why) && isPressWord(note.By)
}

// isFullScreenWhy is whether word is what a fullScreen note may say happened: full screen entered
// or refused, or left by the browser unasked.
func isFullScreenWhy(word string) bool {
	return word == "entered" || word == "refused" || word == fullScreenLeft
}

// fullScreenLeft is a fullScreen note's word for full screen the browser left unasked, which a cast's
// receiver takes for Back (D338 §6).
const fullScreenLeft = "left"

// isPressWord is whether word says how a person pressed, as the web client's Press names it: a tap,
// a click or a key, or "" where it does not say.
func isPressWord(word string) bool {
	return slices.Contains([]string{"", "tap", "click", "key"}, word)
}

// isKnownSound is whether a note of a player's sound is one the server logs: a film played muted, or
// with its sound, with its page's activation, a press turning it on with its sound then, or a page
// asking for a tap to continue and how it was shown.
func (note PlayerNote) isKnownSound() bool {
	if note.Track != "" || len(note.Page) > diagnosisTextMost || len(note.SoundState) > diagnosisTextMost {
		return false
	}
	switch note.Kind {
	case NotePlayedMuted, NotePlayedWithSound:
		return note.PositionMs >= 0 && note.Why == "" && note.By == ""
	case NoteUnmuted:
		return note.PositionMs >= 0 && (note.Why == "" || note.Why == "after") && isPressWord(note.By)
	case NoteContinueAsked:
		return slices.Contains([]string{"navigate", "reload", "back_forward", "prerender"}, note.Why) &&
			note.By == ""
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected, NotePonged,
		NoteVoiceStats, NoteVoiceCall, NoteVoiceOff, NoteVoiceTalk, NoteDiagnosis, NoteLeaderCommand,
		NoteOwnMarkExpired, NoteCorrectsBySeeks, NoteLoadHeld, NoteFullScreen,
		NoteGroupFollowed, NoteBuffering, NoteSoundMoved, NotePlayedWhileHeld,
		NoteShownAgain, NotePlayRefused: // isKnown reads them
	}
	return false
}

// isGroupHeldBy is whether word may say what held a player from following its group's change: its
// page hidden, its own command awaiting the group's answer, its film still loading, or nothing.
func isGroupHeldBy(word string) bool {
	return slices.Contains([]string{"atOnce", "hidden", "awaitingAnswer", "loading"}, word)
}

// isKnownBuffering is whether a buffering note is one the server logs: a known cause, and no
// negative position, load or time.
func (note PlayerNote) isKnownBuffering() bool {
	return note.Track == "" && slices.Contains([]string{"notCaughtUp", "seeking", "notReady", "waiting"}, note.Why) &&
		note.PositionMs >= 0 && note.LoadedAheadMs >= 0 && note.SinceRateChangeMs >= 0
}

// soundShiftMost is the furthest a player moves its sound earlier, in ms: the web player's
// longestSoundShiftMs.
const soundShiftMost = 2000

// isKnownSoundMoved is whether a soundMoved note is one the server logs: a known cause, and each move
// and latency within what a player can make.
func (note PlayerNote) isKnownSoundMoved() bool {
	isShift := func(ms int64) bool { return ms >= 0 && ms <= soundShiftMost }
	movedBys := []string{"remembered", "measured", "offset", "direct"}
	return note.Track == "" && slices.Contains(movedBys, note.Why) &&
		(note.FromWhy == "" || slices.Contains(movedBys, note.FromWhy)) &&
		isShift(note.EarlierFromMs) && isShift(note.EarlierMs) && note.EarlierFromMs != note.EarlierMs &&
		isShift(note.LatencyMs) && isShift(note.RememberedMs)
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
	case NoteLoadHeld:
		return note.Why == "held" || note.Why == "goesOn"
	case NoteGroupFollowed:
		return note.Seq > 0 && note.HeldMs >= 0 && isGroupHeldBy(note.Why)
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected, NoteVoiceStats,
		NoteVoiceCall, NoteVoiceOff, NoteVoiceTalk, NotePlayedMuted, NoteUnmuted, NoteDiagnosis,
		NoteLeaderCommand, NoteContinueAsked, NoteFullScreen, NotePlayedWithSound,
		NoteBuffering, NoteSoundMoved, NotePlayedWhileHeld, NoteShownAgain,
		NotePlayRefused: // isKnown reads them
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
	case NoteVoiceTalk:
		return slices.Contains([]string{"talking", "stopped", "muted", "unmuted"}, note.Why)
	case NoteVoiceCall:
		return note.PartnerDeviceID > 0 && isVoiceCallStep(note.Why)
	case NoteNudged, NoteSought, NoteChunkLoaded, NoteQualitySwitched, NoteReconnected, NotePonged,
		NotePlayedMuted, NoteUnmuted, NoteDiagnosis, NoteLeaderCommand, NoteOwnMarkExpired,
		NoteCorrectsBySeeks, NoteLoadHeld, NoteContinueAsked, NoteFullScreen,
		NotePlayedWithSound, NoteGroupFollowed, NoteBuffering,
		NoteSoundMoved, NotePlayedWhileHeld, NoteShownAgain, NotePlayRefused: // not voice's: isKnown reads them
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
	case "webAudio", "elementSuspended", "elementInterrupted", "elementClosed", "element", "elementBlocked":
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

// isKnownShownAgain is whether a note of a page shown again is one the server logs: its socket's
// answer, or a play the browser refused then, by a known refusal.
func (note PlayerNote) isKnownShownAgain() bool {
	if note.Track != "" || note.By != "" || len(note.Page) > diagnosisTextMost {
		return false
	}
	if note.Kind == NoteShownAgain {
		return note.Why == "alive" && note.RoundTripMs >= 0
	}
	refusals := []string{"NotAllowedError", "AbortError", "NotSupportedError", "other", "stayedPaused"}
	return note.Kind == NotePlayRefused && note.PositionMs >= 0 && slices.Contains(refusals, note.Why)
}
