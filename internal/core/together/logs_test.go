// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"strings"
	"testing"
	"time"
)

// lineWith is the first line of logged whose logEvent attribute is event, matched by key rather
// than by the message's English sentence, so rewording the message does not break the match (test
// architecture review, 2026-10-26, finding 12).
func lineWith(t *testing.T, logged, event string) string {
	t.Helper()
	marker := "logEvent=" + event
	for line := range strings.Lines(logged) {
		if strings.Contains(line, marker) {
			return line
		}
	}
	t.Fatalf("no line with %s:\n%s", marker, logged)
	return ""
}

// A device's player beginning or stopping playing or buffering is logged, with where it is, how far
// behind the group, and how long its last state lasted; a report that changes neither is not.
func TestHub_LogsADevicesPlayerStartingAndStoppingPlayingOrBuffering(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageState, Playing: true, PositionMs: 1000})
	stage.clock.Advance(3 * time.Second)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageState, Playing: true, PositionMs: 4000})
	stage.clock.Advance(time.Second)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageState, Buffering: true, PositionMs: 5000})
	var changes []string
	for line := range strings.Lines(stage.logs.String()) {
		if strings.Contains(line, "logEvent=playerChanged") {
			changes = append(changes, line)
		}
	}
	if len(changes) != 2 {
		t.Fatalf("logged %d player changes, want 2, the start and the stall:\n%s", len(changes), stage.logs)
	}
	for _, want := range []string{"playing=false", "buffering=true", "positionMs=5000", "lastStateMs=4000"} {
		if !strings.Contains(changes[1], want) {
			t.Errorf("the stall's line %q does not say %s", changes[1], want)
		}
	}
}

// A device's note of what its player did is logged with its kind's fields alone; a note of no
// known kind, or carrying an unknown word, is refused as invalid and logged as nothing.
func TestHub_LogsADevicesPlayerNoteAndRefusesAnUnknownOne(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteSought, PositionMs: 50_000, ToMs: 124_000, Why: "catchUp",
	}})
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteChunkLoaded, Track: "video", LoadMs: 5200, FilmMs: 10_200, LoadedKbps: 2300, QualityKbps: 1200,
	}})
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteNudged, PositionMs: 30_000, DriftMs: 380, Rate: 1.1, ReturnCostMs: 220,
	}})
	logged := stage.logs.String()
	for _, want := range []string{
		`kind=sought positionMs=50000 toMs=124000 why=catchUp`,
		`kind=chunkLoaded loadMs=5200 filmMs=10200 loadedKbps=2300 qualityKbps=1200 track=video`,
		`kind=nudged positionMs=30000 driftMs=380 returnCostMs=220 rate=1.1`,
	} {
		isNoted := strings.Contains(logged, "logEvent=playerNoted")
		if !isNoted || !strings.Contains(logged, want) {
			t.Errorf("the log does not note %q:\n%s", want, logged)
		}
	}
	for _, unknown := range []*PlayerNote{nil, {Kind: "danced"}, {Kind: NoteSought, Why: "a\nforged line"}} {
		box.mutex.Lock()
		box.messages = nil
		box.mutex.Unlock()
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: unknown})
		if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
			t.Errorf("note %+v answered %+v; want rejected as invalid", unknown, rejected)
		}
	}
	if strings.Count(stage.logs.String(), "logEvent=playerNoted") != 3 {
		t.Errorf("an unknown note was logged:\n%s", stage.logs)
	}
}

// A voice call's statistics, as its receiver measured them, are logged at info, so a call that broke
// up says whether its sound came late, was lost, or went through the relay over TCP (D262 §1, §4);
// one without its statistics, or with an unknown path or playback, is refused. They say how loud what
// it received and sent were, and how it plays: an iPhone and an Android phone connected and neither
// heard the other, and nothing said which side was silent (the owner's test, 2026-10-08); and what
// the Talk button's ring read, still on an Android phone hosting (the owner's test, 2026-10-11).
func TestHub_LogsAVoiceCallsStatisticsAndRefusesThemWithoutAKnownPath(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteVoiceStats, Voice: &VoiceStats{
			PartnerDeviceID: 4, RoundTripMs: 340, JitterBufferMs: 905, JitterBufferTargetMs: 880,
			ConcealedShare: 0.043, PacketsLost: 312, PacketsLate: 97, FECPackets: 40, Path: "udp",
			ReceivedLevel: 0.126, SentLevel: 0.032, Playback: "elementInterrupted", RingLevel: 0.4,
			PartnerGain: new(0.0),
		},
	}})
	fields := `logEvent=voiceMeasured partnerDeviceId=4 roundTripMs=340 jitterBufferMs=905 jitterBufferTargetMs=880 ` +
		`concealedShare=0.043 packetsLost=312 packetsLate=97 fecPackets=40 path=udp receivedLevel=0.126 ` +
		`sentLevel=0.032 playback=elementInterrupted ringLevel=0.4 partnerGain=0`
	if line := lineWith(t, stage.logs.String(), "voiceMeasured"); !strings.Contains(line, "level=INFO") ||
		!strings.Contains(line, fields) {
		t.Errorf("the log does not say the call's statistics:\n%s", stage.logs.String())
	}
	unknowns := []*PlayerNote{
		{Kind: NoteVoiceStats},
		{Kind: NoteVoiceStats, Voice: &VoiceStats{Path: "carrier pigeon"}},
		{Kind: NoteVoiceStats, Voice: &VoiceStats{Path: "udp", Playback: "a gramophone"}},
	}
	for _, unknown := range unknowns {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: unknown})
		if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
			t.Errorf("note %+v answered %+v; want rejected as invalid", unknown, rejected)
		}
	}
}

// A voice call's steps are logged at info, so a call that never connects says how far it got (a
// browser tier's run, 2026-10-07, where nothing did); a step with no partner, or an unknown one, is
// refused.
func TestHub_LogsAVoiceCallsStepsAndRefusesAnUnknownOne(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteVoiceCall, PartnerDeviceID: 4, Why: "signalDropped",
	}})
	if line := lineWith(t, stage.logs.String(), "voiceCallStep"); !strings.Contains(line, "level=INFO") ||
		!strings.Contains(line, "partnerDeviceId=4 step=signalDropped") {
		t.Errorf("the log does not say the call's step:\n%s", stage.logs.String())
	}
	unknowns := []*PlayerNote{
		{Kind: NoteVoiceCall, Why: "offered"}, {Kind: NoteVoiceCall, PartnerDeviceID: 4, Why: "catchUp"},
	}
	for _, unknown := range unknowns {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: unknown})
		if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
			t.Errorf("note %+v answered %+v; want rejected as invalid", unknown, rejected)
		}
	}
}

// Each pong a follower hears is logged at info with where it said the group is against where the
// follower expected it, and whether it placed the group anew: a steady offset on a person's link,
// and two devices' clock skew, are read from the log (D264, D268 §1).
func TestHub_LogsEachPongAPlayerHeardWithItsResidual(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	for _, note := range []PlayerNote{
		{Kind: NotePonged, RoundTripMs: 84, AheadOfExpectedMs: 173, Why: "placed"},
		{Kind: NotePonged, RoundTripMs: 97, AheadOfExpectedMs: -12, Why: "kept"},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &note})
	}
	logged := stage.logs.String()
	var pongs []string
	for line := range strings.Lines(logged) {
		if strings.Contains(line, "logEvent=pongResidual") {
			pongs = append(pongs, line)
		}
	}
	if len(pongs) != 2 {
		t.Fatalf("logged %d pongs; want 2:\n%s", len(pongs), logged)
	}
	for index, said := range []string{
		"roundTripMs=84 aheadOfExpectedMs=173 placing=placed", "roundTripMs=97 aheadOfExpectedMs=-12 placing=kept",
	} {
		if !strings.Contains(pongs[index], "level=INFO") || !strings.Contains(pongs[index], said) {
			t.Errorf("pong %d does not say %q:\n%s", index, said, pongs[index])
		}
	}
}

// A follower's own move that fired no event, its mark given up, is logged at info: a mark left
// waiting took a person's later move for the follower's own (D268 §4).
func TestHub_LogsAPlayersOwnMarkGivenUp(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteOwnMarkExpired, Why: "pause",
	}})
	if line := lineWith(t, stage.logs.String(), "ownMarkExpired"); !strings.Contains(line, "level=INFO") ||
		!strings.Contains(line, "move=pause") {
		t.Errorf("the log does not say the mark given up:\n%s", stage.logs.String())
	}
}

// A follower that corrects by seeks alone from now on is logged at info, with what its rate's
// returns cost it: which device holds its group less tightly, and why, is read from the log (D268 §2).
func TestHub_LogsAPlayerThatCorrectsBySeeksAlone(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteCorrectsBySeeks, ReturnCostMs: 220,
	}})
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteSought, PositionMs: 60_000, ToMs: 60_600, Why: "skip",
	}})
	logged := stage.logs.String()
	line := lineWith(t, logged, "correctsBySeeks")
	if !strings.Contains(line, "level=INFO") || !strings.Contains(line, "returnCostMs=220") ||
		!strings.Contains(logged, "why=skip") {
		t.Errorf("the log does not say the player corrects by seeks, and skipped:\n%s", logged)
	}
}

// A ready player's loading held while its group waits, and going on, is logged, a word other than
// those two refused; and a player ready again says which rule it was ready by and how fast its last
// chunk loaded (D283; the owner, 2026-10-12: "add the logging first").
func TestHub_LogsAPlayersLoadingHeldAndTheRuleItWasReadyBy(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	for _, why := range []string{"held", "goesOn"} {
		note := &PlayerNote{Kind: NoteLoadHeld, Why: why}
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: note})
	}
	unknown := &PlayerNote{Kind: NoteLoadHeld, Why: "dancing"}
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: unknown})
	if box.last(MessageRejected).Code != codeInvalid {
		t.Errorf("a loading note with an unknown word was not refused")
	}
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageState, Buffering: true, PositionMs: 1000})
	stage.hub.Handle(t.Context(), device, Inbound{
		Type: MessageState, PositionMs: 1000, ReadyOnMs: 3000, LoadsFaster: 3.4,
	})
	logged := stage.logs.String()
	for _, want := range []string{
		"logEvent=loadHeldChanged", "loading=held",
		"loading=goesOn", "buffering=false", "readyOnMs=3000 loadsFaster=3.4",
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log has no %s:\n%s", want, logged)
		}
	}
}

// A browser refusing to play with sound, and the tap that turns it on, are logged at INFO: a guest's
// iPhone sat silent and nothing said why (the owner's test, 2026-10-08).
func TestHub_LogsAPlayerPlayingMutedAndATapTurningItsSoundOn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	for _, note := range []PlayerNote{
		{Kind: NotePlayedMuted, PositionMs: 4919}, {Kind: NoteUnmuted, PositionMs: 31585},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &note})
	}
	logged := stage.logs.String()
	muted := lineWith(t, logged, "playedMuted")
	if !strings.Contains(muted, "level=INFO") || !strings.Contains(muted, "positionMs=4919") {
		t.Errorf("the log does not say the player played muted at 4919ms:\n%s", logged)
	}
	unmuted := lineWith(t, logged, "unmutedTap")
	if !strings.Contains(unmuted, "level=INFO") || !strings.Contains(unmuted, "positionMs=31585") {
		t.Errorf("the log does not say the tap unmuted the player at 31585ms:\n%s", logged)
	}
}

// A guest's page asking for a tap to continue says how it came to be shown, and an unknown way is
// refused: an iPhone asked straight after its Join said nothing of whether it had loaded again (the
// owner's test, 2026-10-13).
func TestHub_LogsAGuestsPageAskingForATapToContinue(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	page := "loadedMsAgo=1400 joinedHere=false"
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteContinueAsked, Why: "reload", Page: page,
	}})
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteContinueAsked, Why: "teleported",
	}})
	logged := stage.logs.String()
	for _, want := range []string{
		"logEvent=continueAsked",
		"navigation=reload", `page="` + page + `"`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log does not say %s:\n%s", want, logged)
		}
	}
	if strings.Contains(logged, "teleported") {
		t.Errorf("the log took a way of showing it does not know:\n%s", logged)
	}
}

// A tap's sound state is logged, at the tap and again after it, so an iPhone that stays silent
// after its tap says whether its film was muted again, paused, or in which audio session (the
// owner's VQ30, 2026-10-12); an unknown why is refused.
func TestHub_LogsATapsSoundStateAtItAndAfter(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	after := "muted=false volume=1.00 paused=false session=playback"
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteUnmuted, PositionMs: 2843705, Why: "after", SoundState: after,
	}})
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteUnmuted, PositionMs: 1, Why: "later",
	}})
	logged := stage.logs.String()
	for _, want := range []string{"positionMs=2843705", "why=after", `soundState="` + after + `"`} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log does not say %s:\n%s", want, logged)
		}
	}
	if strings.Contains(logged, "positionMs=1 ") {
		t.Errorf("a note with an unknown why was logged:\n%s", logged)
	}
}

// Voice turning off is logged at INFO with why: calls of 8 s ended with nothing to say whether a
// person or a failure ended them (the owner's test, 2026-10-08); an unknown why is refused.
func TestHub_LogsVoiceTurningOffAndWhy(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	off := func(why string) Inbound {
		return Inbound{Type: MessageNote, Note: &PlayerNote{Kind: NoteVoiceOff, Why: why}}
	}
	stage.hub.Handle(t.Context(), device, off("left"))
	if line := lineWith(t, stage.logs.String(), "voiceOff"); !strings.Contains(line, "level=INFO") ||
		!strings.Contains(line, "why=left") {
		t.Errorf("the log does not say voice turned off, and why:\n%s", stage.logs.String())
	}
	stage.hub.Handle(t.Context(), device, off("bored"))
	if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
		t.Errorf("an unknown why answered %+v; want rejected as invalid", rejected)
	}
}

// A person's microphone opening, closing, muting and unmuting is logged at INFO, so a voice no one
// heard says whether its person was talking (D281 §2, §3); an unknown why is refused.
func TestHub_LogsAPersonsMicrophoneChanging(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	talk := func(why string) Inbound {
		return Inbound{Type: MessageNote, Note: &PlayerNote{Kind: NoteVoiceTalk, Why: why}}
	}
	for _, why := range []string{"talking", "muted", "unmuted", "stopped"} {
		stage.hub.Handle(t.Context(), device, talk(why))
		logged := stage.logs.String()
		if !strings.Contains(logged, "logEvent=microphoneChanged") || !strings.Contains(logged, "microphone="+why) {
			t.Errorf("the log does not say the microphone was %s:\n%s", why, logged)
		}
	}
	stage.hub.Handle(t.Context(), device, talk("shouting"))
	if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
		t.Errorf("an unknown why answered %+v; want rejected as invalid", rejected)
	}
}

// What a player holds as it moved itself over a gap, or stalled, is logged at INFO: an iPhone that
// jumped 10 s on its own and then sat waiting said nothing of what it held (the owner's test,
// 2026-10-08). An unknown why, or text past diagnosisTextMost, is refused.
func TestHub_LogsWhatAPlayerHoldsWhenItMovedItselfOrStalled(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteDiagnosis, Why: "movedItself", PositionMs: 10_319, ReadyState: 1,
		VideoBuffered: "10.14-30.20", AudioBuffered: "0.00-30.10", LastError: "bufferSeekOverHole: hole",
	}})
	logged := stage.logs.String()
	for _, want := range []string{
		"logEvent=playerDiagnosis",
		`why=movedItself positionMs=10319 readyState=1 videoBuffered=10.14-30.20 audioBuffered=0.00-30.10`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log does not say %s:\n%s", want, logged)
		}
	}
	for _, unknown := range []*PlayerNote{
		{Kind: NoteDiagnosis, Why: "bored"},
		{Kind: NoteDiagnosis, Why: "stalled", LastError: strings.Repeat("x", diagnosisTextMost+1)},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: unknown})
		if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
			t.Errorf("note %+v answered %+v; want rejected as invalid", unknown, rejected)
		}
	}
}

// How a player placed its first chunks, and whether an iPhone's media source lets it load, are
// logged with what it holds as it buffers each playlist's first chunk: an iPhone's jump of 10 s at
// its start, the chunks' offset, was left to a guess (the sync review, 2026-10-08). A streaming
// state the server does not know is refused.
func TestHub_LogsHowAPlayerPlacedItsFirstChunks(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteDiagnosis, Why: "bufferedFirst", PositionMs: 300_000, ReadyState: 1,
		Mapping: "initPTS main 10.000; main sn 30 at 300.00 pts 300.00", MediaSourceState: "streaming",
	}})
	want := `why=bufferedFirst positionMs=300000 readyState=1`
	mapping := `mapping="initPTS main 10.000; main sn 30 at 300.00 pts 300.00" mediaSourceState=streaming`
	if logged := stage.logs.String(); !strings.Contains(logged, want) || !strings.Contains(logged, mapping) {
		t.Errorf("the log does not say how the player placed its first chunk:\n%s", logged)
	}
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteDiagnosis, Why: "bufferedFirst", MediaSourceState: "maybe",
	}})
	if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
		t.Errorf("an unknown streaming state answered %+v; want rejected as invalid", rejected)
	}
}

// A leading player's command is logged at INFO with its player's last events: a host's play was
// stopped 0.4 s later by a pause no one could say the source of (the owner's tests, 2026-10-08).
func TestHub_LogsALeadingPlayersCommandWithItsEvents(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteLeaderCommand, Why: "pause", RecentEvents: "play@100 pause(own)@120 play@125 pause@510",
	}})
	logged := stage.logs.String()
	if !strings.Contains(logged, "logEvent=leaderCommand") ||
		!strings.Contains(logged, `command=pause recentEvents="play@100 pause(own)@120 play@125 pause@510"`) {
		t.Errorf("the log does not say the command and its events:\n%s", logged)
	}
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteLeaderCommand, Why: "dance",
	}})
	if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
		t.Errorf("an unknown command answered %+v; want rejected as invalid", rejected)
	}
}
