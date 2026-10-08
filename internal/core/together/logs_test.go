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
	logged := stage.logs.String()
	for _, want := range []string{
		`kind=sought positionMs=50000 toMs=124000 why=catchUp`,
		`kind=chunkLoaded loadMs=5200 kbps=2300 qualityKbps=1200 track=video`,
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
	if strings.Count(stage.logs.String(), "a device's player noted") != 2 {
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

// A follower placing its group by a shorter round trip is logged at info with how far it moved
// where the group is expected, so a steady offset on a person's link is read from the log (D264).
func TestHub_LogsAPlayerPlacingItsGroupByAShorterRoundTrip(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NotePlaced, RoundTripMs: 84, MovedMs: 173,
	}})
	want := `level=INFO msg="watch together: a player placed its group by a shorter round trip"`
	if logged := stage.logs.String(); !strings.Contains(logged, want) ||
		!strings.Contains(logged, "roundTripMs=84 movedMs=173") {
		t.Errorf("the log does not say the placing:\n%s", logged)
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
