// SPDX-License-Identifier: GPL-3.0-or-later

package together

import (
	"strings"
	"testing"
	"time"
)

// A device's player beginning or stopping playing or buffering is logged, with where it is, how far
// behind the group, and how long its last state lasted; a report that changes neither is not.
func TestHub_LogsADevicesPlayerStartingAndStoppingPlayingOrBuffering(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageState, Playing: true, PositionMs: 1000})
	stage.clock.Advance(3 * time.Second)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageState, Playing: true, PositionMs: 4000})
	stage.clock.Advance(time.Second)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageState, Buffering: true, PositionMs: 5000})
	var changes []string
	for line := range strings.Lines(stage.logs.String()) {
		if strings.Contains(line, `"watch together: a device's player changed"`) {
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
	device, box := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteSought, PositionMs: 50_000, ToMs: 124_000, Why: "catchUp",
	}})
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteChunkLoaded, Track: "video", LoadMs: 5200, Kbps: 2300, QualityKbps: 1200,
	}})
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteNudged, PositionMs: 30_000, DriftMs: 380, Rate: 1.1, ReturnCostMs: 220,
	}})
	logged := stage.logs.String()
	for _, want := range []string{
		`kind=sought positionMs=50000 toMs=124000 why=catchUp`,
		`kind=chunkLoaded loadMs=5200 kbps=2300 qualityKbps=1200 track=video`,
		`kind=nudged positionMs=30000 driftMs=380 returnCostMs=220 rate=1.1`,
	} {
		isNoted := strings.Contains(logged, `msg="watch together: a device's player noted"`)
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
	if strings.Count(stage.logs.String(), "a device's player noted") != 3 {
		t.Errorf("an unknown note was logged:\n%s", stage.logs)
	}
}

// A voice call's statistics, as its receiver measured them, are logged at info, so a call that broke
// up says whether its sound came late, was lost, or went through the relay over TCP (D262 §1, §4);
// one without its statistics, or with an unknown path or playback, is refused. They say how loud what
// it received and sent were, and how it plays: an iPhone and an Android phone connected and neither
// heard the other, and nothing said which side was silent (the owner's test, 2026-10-08).
func TestHub_LogsAVoiceCallsStatisticsAndRefusesThemWithoutAKnownPath(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVoice)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteVoiceStats, Voice: &VoiceStats{
			PartnerDeviceID: 4, RoundTripMs: 340, JitterBufferMs: 905, JitterBufferTargetMs: 880,
			ConcealedShare: 0.043, PacketsLost: 312, PacketsLate: 97, FECPackets: 40, Path: "udp",
			ReceivedLevel: 0.126, SentLevel: 0.032, Playback: "elementInterrupted",
		},
	}})
	want := `level=INFO msg="watch together: a voice call measured"`
	fields := `partnerDeviceId=4 roundTripMs=340 jitterBufferMs=905 jitterBufferTargetMs=880 concealedShare=0.043 ` +
		`packetsLost=312 packetsLate=97 fecPackets=40 path=udp receivedLevel=0.126 sentLevel=0.032 ` +
		`playback=elementInterrupted`
	if logged := stage.logs.String(); !strings.Contains(logged, want) || !strings.Contains(logged, fields) {
		t.Errorf("the log does not say the call's statistics:\n%s", logged)
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
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVoice)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteVoiceCall, PartnerDeviceID: 4, Why: "signalDropped",
	}})
	want := `level=INFO msg="watch together: a voice call took a step"`
	if logged := stage.logs.String(); !strings.Contains(logged, want) ||
		!strings.Contains(logged, "partnerDeviceId=4 step=signalDropped") {
		t.Errorf("the log does not say the call's step:\n%s", logged)
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
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	for _, note := range []PlayerNote{
		{Kind: NotePonged, RoundTripMs: 84, ResidualMs: 173, Why: "placed"},
		{Kind: NotePonged, RoundTripMs: 97, ResidualMs: -12, Why: "kept"},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &note})
	}
	want := `level=INFO msg="watch together: a player's pong said where the group is"`
	logged := stage.logs.String()
	for _, said := range []string{
		"roundTripMs=84 residualMs=173 placing=placed", "roundTripMs=97 residualMs=-12 placing=kept",
	} {
		if !strings.Contains(logged, want) || !strings.Contains(logged, said) {
			t.Errorf("the log does not say %q:\n%s", said, logged)
		}
	}
}

// A follower's own move that fired no event, its mark given up, is logged at info: a mark left
// waiting took a person's later move for the follower's own (D268 §4).
func TestHub_LogsAPlayersOwnMarkGivenUp(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteOwnMarkExpired, Why: "pause",
	}})
	want := `level=INFO msg="watch together: a player's own move fired no event; its mark was given up"`
	if logged := stage.logs.String(); !strings.Contains(logged, want) || !strings.Contains(logged, "move=pause") {
		t.Errorf("the log does not say the mark given up:\n%s", logged)
	}
}

// A browser refusing to play with sound, and the tap that turns it on, are logged at INFO: a guest's
// iPhone sat silent and nothing said why (the owner's test, 2026-10-08).
func TestHub_LogsAPlayerPlayingMutedAndATapTurningItsSoundOn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	for _, note := range []PlayerNote{
		{Kind: NotePlayedMuted, PositionMs: 4919}, {Kind: NoteUnmuted, PositionMs: 31585},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &note})
	}
	logged := stage.logs.String()
	for _, want := range []string{
		`level=INFO msg="watch together: the browser would not play the film with sound; it plays muted"`,
		"positionMs=4919",
		`level=INFO msg="watch together: a tap turned a muted player's sound on"`,
		"positionMs=31585",
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log does not say %s:\n%s", want, logged)
		}
	}
}

// Voice turning off is logged at INFO with why: calls of 8 s ended with nothing to say whether a
// person or a failure ended them (the owner's test, 2026-10-08); an unknown why is refused.
func TestHub_LogsVoiceTurningOffAndWhy(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVoice)
	off := func(why string) Inbound {
		return Inbound{Type: MessageNote, Note: &PlayerNote{Kind: NoteVoiceOff, Why: why}}
	}
	stage.hub.Handle(t.Context(), device, off("left"))
	if logged := stage.logs.String(); !strings.Contains(logged, `level=INFO msg="watch together: voice turned off"`) ||
		!strings.Contains(logged, "why=left") {
		t.Errorf("the log does not say voice turned off, and why:\n%s", logged)
	}
	stage.hub.Handle(t.Context(), device, off("bored"))
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
	device, box := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteDiagnosis, Why: "movedItself", PositionMs: 10_319, ReadyState: 1,
		VideoBuffered: "10.14-30.20", AudioBuffered: "0.00-30.10", LastError: "bufferSeekOverHole: hole",
	}})
	logged := stage.logs.String()
	for _, want := range []string{
		`level=INFO msg="watch together: a device's player said what it holds"`,
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
	device, box := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteDiagnosis, Why: "bufferedFirst", PositionMs: 300_000, ReadyState: 1,
		Mapping: "initPTS main 10.000; main sn 30 at 300.00 pts 300.00", Streaming: "streaming",
	}})
	want := `why=bufferedFirst positionMs=300000 readyState=1`
	mapping := `mapping="initPTS main 10.000; main sn 30 at 300.00 pts 300.00" streaming=streaming`
	if logged := stage.logs.String(); !strings.Contains(logged, want) || !strings.Contains(logged, mapping) {
		t.Errorf("the log does not say how the player placed its first chunk:\n%s", logged)
	}
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteDiagnosis, Why: "bufferedFirst", Streaming: "maybe",
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
	device, box := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteLed, Why: "pause", Events: "play@100 pause(own)@120 play@125 pause@510",
	}})
	logged := stage.logs.String()
	if !strings.Contains(logged, `msg="watch together: a leading player sent a command"`) ||
		!strings.Contains(logged, `command=pause events="play@100 pause(own)@120 play@125 pause@510"`) {
		t.Errorf("the log does not say the command and its events:\n%s", logged)
	}
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{Kind: NoteLed, Why: "dance"}})
	if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
		t.Errorf("an unknown command answered %+v; want rejected as invalid", rejected)
	}
}
