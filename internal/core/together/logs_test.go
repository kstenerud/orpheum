// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"fmt"
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
	// A switch says the quality it came from: a Fire TV's first quality read as a switch to the top
	// (the owner's cast, staging, 2026-10-28).
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteQualitySwitched, QualityKbps: 2000, LinkEstimateKbps: 4200, FromQualityKbps: 15_635,
	}})
	logged := stage.logs.String()
	for _, want := range []string{
		`kind=sought positionMs=50000 toMs=124000 why=catchUp`,
		`kind=chunkLoaded loadMs=5200 filmMs=10200 loadedKbps=2300 qualityKbps=1200 track=video`,
		`kind=nudged positionMs=30000 driftMs=380 returnCostMs=220 rate=1.1`,
		`kind=qualitySwitched qualityKbps=2000 linkEstimateKbps=4200 fromQualityKbps=15635`,
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
	if strings.Count(stage.logs.String(), "logEvent=playerNoted") != 4 {
		t.Errorf("an unknown note was logged:\n%s", stage.logs)
	}
}

// A switch of quality is logged with why the player's controller chose it and the link's samples
// then, bounded as a diagnosis's text is: a Fire TV fell from 9018 to 696 kb/s 2.3 s into Nosferatu, its
// link estimated at 69 kb/s though its chunks had loaded at 14-18 Mb/s, and nothing in the log said
// why (the owner's cast, staging, 2026-10-28).
func TestHub_LogsWhyAPlayerSwitchedQuality(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	why := "gave up a chunk of 5: 0 B in 2004 ms; confident 69 kb/s (mean 69, error 0%, 5 samples), " +
		"8.1 s ahead, steadiness 1, hold 60 s"
	samples := "samples 5 2036000B 1100ms fb40ms, 5 1B 2004ms fb2004ms"
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteQualitySwitched, QualityKbps: 696, LinkEstimateKbps: 69, FromQualityKbps: 9018,
		Why: why, LinkSamples: samples,
	}})
	want := fmt.Sprintf(
		`kind=qualitySwitched qualityKbps=696 linkEstimateKbps=69 fromQualityKbps=9018 why=%q linkSamples=%q`,
		why, samples)
	if line := lineWith(t, stage.logs.String(), eventPlayerNoted); !strings.Contains(line, want) {
		t.Errorf("the switch's line does not say why:\n got %s\nwant %s", line, want)
	}
	tooLong := strings.Repeat("x", diagnosisTextMost+1)
	for _, unknown := range []*PlayerNote{
		{Kind: NoteQualitySwitched, QualityKbps: 696, Why: tooLong},
		{Kind: NoteQualitySwitched, QualityKbps: 696, LinkSamples: tooLong},
		{Kind: NoteQualitySwitched, QualityKbps: 696, Track: "video"},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: unknown})
		if rejected := box.last(MessageRejected); rejected.Code != codeInvalid {
			t.Errorf("note %+v answered %+v; want rejected as invalid", unknown, rejected)
		}
	}
	if strings.Count(stage.logs.String(), "logEvent=playerNoted") != 1 {
		t.Errorf("an overlong switch was logged:\n%s", stage.logs)
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

// A player's word that it followed a change of its group is logged at info: which change, how long
// after the server announced it the word came, and how long the player held it and why. A change the
// group has moved on from says only its number. A cast's receiver played on seven seconds after its
// remote paused it, and nothing said where those seconds went (the owner, 2026-10-28).
func TestHub_LogsHowLongAfterItsGroupsChangeAPlayerFollowedIt(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	player, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	remote, _ := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.ready(t, player, false)
	stage.command(t, player, CommandChangeItem, 7)
	stage.command(t, remote, CommandPause, 0)
	paused := box.last(MessageGroup).Seq
	stage.clock.Advance(6800 * time.Millisecond)
	stage.hub.Handle(t.Context(), player, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteGroupFollowed, Seq: paused, HeldMs: 6750, Why: "hidden",
	}})
	line := lineWith(t, stage.logs.String(), "groupFollowed")
	for _, want := range []string{"level=INFO", "heldMs=6750", "heldBy=hidden", "event=paused", "afterChangeMs=6800"} {
		if !strings.Contains(line, want) {
			t.Errorf("the followed change's line does not say %s:\n%s", want, line)
		}
	}
	stage.command(t, remote, CommandPlay, 0)
	stage.logs.Reset()
	stage.hub.Handle(t.Context(), player, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteGroupFollowed, Seq: paused, HeldMs: 20, Why: "atOnce",
	}})
	if line := lineWith(t, stage.logs.String(), "groupFollowed"); strings.Contains(line, "afterChangeMs") ||
		!strings.Contains(line, "groupSeq=") {
		t.Errorf("a change the group moved on from: %s; want its number and the group's, no time", line)
	}
	for _, unknown := range []*PlayerNote{
		{Kind: NoteGroupFollowed, Seq: paused, Why: "dawdled"},
		{Kind: NoteGroupFollowed, Seq: 0, Why: "atOnce"},
		{Kind: NoteGroupFollowed, Seq: paused, HeldMs: -1, Why: "atOnce"},
	} {
		stage.logs.Reset()
		stage.hub.Handle(t.Context(), player, Inbound{Type: MessageNote, Note: unknown})
		if strings.Contains(stage.logs.String(), "groupFollowed") || box.last(MessageRejected).Code != codeInvalid {
			t.Errorf("note %+v: logged or not refused:\n%s", unknown, stage.logs)
		}
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

// A following player beginning to hold its playing group up, or holding up its wait, is logged at
// info, with the group's state, what holds it, its readyState and rate, what it has loaded ahead and
// how long since its rate changed: a Fire TV's buffering after each nudge said none of it, nor a
// Firefox that held its group's wait for good (the owner's casts, staging, 2026-10-28). A note that
// names no known cause is refused.
func TestHub_LogsWhatHoldsAPlayerThatBeganBuffering(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleBoth)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteBuffering, Why: "waiting", PositionMs: 320_687, ReadyState: 2, Rate: 0.9,
		LoadedAheadMs: 11_400, SinceRateChangeMs: 356,
	}})
	line := lineWith(t, stage.logs.String(), eventPlayerBuffering)
	for _, want := range []string{
		"level=INFO", "groupState=Idle", "why=waiting", "positionMs=320687", "readyState=2", "rate=0.9",
		"loadedAheadMs=11400", "sinceRateChangeMs=356",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("the buffering line does not say %s:\n%s", want, line)
		}
	}
	stage.logs.Reset()
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteBuffering, Why: "bored",
	}})
	if strings.Contains(stage.logs.String(), eventPlayerBuffering) || box.last(MessageRejected).Code != codeInvalid {
		t.Errorf("a buffering note with no known cause: logged or not refused:\n%s", stage.logs)
	}
}

// A player's move of its sound earlier than its picture is logged at info: from and to, what moved it,
// Web Audio's latency as measured then and whether the move threw the sound loaded away. A Fire TV's
// start waited twice as its move threw its sound away, and nothing said when or by how much the move
// changed (the owner's cast, staging, 2026-10-28). It says too what moved the sound to where it moved
// from and the latency the device remembered, "none" where nothing had and it remembered none: the
// casts after left only moves back to the latency, from a latency no line said. A note with no known
// cause, a move of nothing, or one beyond what a player makes, is refused.
func TestHub_LogsAPlayersMoveOfItsSound(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleBoth)
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteSoundMoved, EarlierFromMs: 331, EarlierMs: 384, Why: "measured", LatencyMs: 384, IsFlushed: true,
		FromWhy: "remembered", RememberedMs: 331,
	}})
	line := lineWith(t, stage.logs.String(), eventSoundMoved)
	for _, want := range []string{
		"level=INFO", "earlierFromMs=331", "earlierMs=384", "why=measured", "latencyMs=384", "flushed=true",
		"fromWhy=remembered", "rememberedMs=331",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("the sound's move line does not say %s:\n%s", want, line)
		}
	}
	stage.logs.Reset()
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteSoundMoved, EarlierMs: 331, Why: "remembered",
	}})
	line = lineWith(t, stage.logs.String(), eventSoundMoved)
	if !strings.Contains(line, "earlierFromMs=0") || !strings.Contains(line, "flushed=false") ||
		strings.Contains(line, "latencyMs") || !strings.Contains(line, "fromWhy=none") ||
		!strings.Contains(line, "rememberedMs=none") {
		t.Errorf("a remembered move before any chunk: %s; want from 0 by none, not flushed, no latency, "+
			"none remembered", line)
	}
	for _, unknown := range []*PlayerNote{
		{Kind: NoteSoundMoved, EarlierMs: 331, Why: "guessed"},
		{Kind: NoteSoundMoved, EarlierFromMs: 331, EarlierMs: 331, Why: "measured"},
		{Kind: NoteSoundMoved, EarlierMs: 2001, Why: "offset"},
		{Kind: NoteSoundMoved, EarlierMs: 300, Why: "measured", LatencyMs: -1},
		{Kind: NoteSoundMoved, EarlierMs: 300, Why: "measured", FromWhy: "guessed"},
		{Kind: NoteSoundMoved, EarlierMs: 300, Why: "measured", RememberedMs: 2001},
	} {
		stage.logs.Reset()
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: unknown})
		if strings.Contains(stage.logs.String(), eventSoundMoved) || box.last(MessageRejected).Code != codeInvalid {
			t.Errorf("note %+v: logged or not refused:\n%s", unknown, stage.logs)
		}
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

// A film played muted says whether its page had ever been pressed, the press that turned its sound on
// says how it was made, and full screen entered or refused says by what, and left unasked says so: a
// cast's receiver on a Fire TV played silent and out of full screen with nothing to say why, and its
// Back left full screen alone, unseen by the page (the owner, 2026-10-27). An unknown
// press, or one on a note that has none, is refused.
func TestHub_LogsThePagesActivationAndThePressThatAnswered(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	page := "hasBeenActive=false isActive=false loadedMsAgo=23000 navigation=navigate"
	for _, note := range []PlayerNote{
		{Kind: NotePlayedMuted, PositionMs: 1131171, Page: page},
		{Kind: NotePlayedWithSound, PositionMs: 1131172, Page: "hasBeenActive=true"},
		{Kind: NoteUnmuted, PositionMs: 1140000, By: "key"},
		{Kind: NoteFullScreen, Why: "refused"},
		{Kind: NoteFullScreen, Why: "entered", By: "click"},
		{Kind: NoteFullScreen, Why: "left"},
		{Kind: NoteFullScreen, Why: "entered", By: "elbow"},
		{Kind: NoteFullScreen, Why: "maybe"},
		{Kind: NotePlayedMuted, PositionMs: 7, By: "tap"},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &note})
	}
	logged := stage.logs.String()
	if muted := lineWith(t, logged, "playedMuted"); !strings.Contains(muted, `page="`+page+`"`) {
		t.Errorf("the log does not say the page's activation as it played muted:\n%s", logged)
	}
	if sounded := lineWith(t, logged, "playedWithSound"); !strings.Contains(sounded, "positionMs=1131172") ||
		!strings.Contains(sounded, `page="hasBeenActive=true"`) {
		t.Errorf("the log does not say the film played with sound, and the page's activation:\n%s", logged)
	}
	if unmuted := lineWith(t, logged, "unmutedTap"); !strings.Contains(unmuted, "by=key") {
		t.Errorf("the log does not say a key turned the sound on:\n%s", logged)
	}
	for _, want := range []string{"answer=refused by=\"\"", "answer=entered by=click", "answer=left by=\"\""} {
		if !strings.Contains(logged, "logEvent=fullScreen "+want) {
			t.Errorf("the log does not say full screen %s:\n%s", want, logged)
		}
	}
	for _, unknown := range []string{"elbow", "maybe", "positionMs=7 "} {
		if strings.Contains(logged, unknown) {
			t.Errorf("the log took a note it does not know (%s):\n%s", unknown, logged)
		}
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

// A play its follower did not make, while the group does not play, says at info what began it and
// the group's state: a Fire TV's cast started with a play, a pause and a play, and the log said only
// that it played with sound (the owner's Marrowgate Family, staging, 2026-10-28). An unknown cause is
// refused.
func TestHub_LogsWhatPlayedAPlayerItsGroupHeld(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, device, CommandChangeItem, 7) // Waiting: its player not yet ready
	for _, note := range []PlayerNote{
		{Kind: NotePlayedWhileHeld, PositionMs: 1607085, Why: "autoplay"},
		{Kind: NotePlayedWhileHeld, PositionMs: 1607086, Why: "elbow"},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &note})
	}
	logged := stage.logs.String()
	played := lineWith(t, logged, "playedWhileHeld")
	for _, want := range []string{"level=INFO", "groupState=Waiting", "by=autoplay", "positionMs=1607085"} {
		if !strings.Contains(played, want) {
			t.Errorf("the log of a play the group held lacks %s:\n%s", want, played)
		}
	}
	if strings.Contains(logged, "elbow") {
		t.Errorf("the log took a play's cause it does not know:\n%s", logged)
	}
}

// A follower's every move to catch up to its leader is logged at info, a seek with why, from where to
// where, and a nudge with its rate, each with how far behind the leader it was and who leads, so a
// report can be read from the log (D371 §2). The leader's own seeks are noted at debug as before.
func TestHub_LogsEachFollowersCatchUpAndHowFarBehindItsLeader(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	stage.clock.Advance(60 * time.Second)
	for _, note := range []*PlayerNote{
		{Kind: NoteSought, PositionMs: 50_000, ToMs: 64_000, Why: "catchUp"},
		{Kind: NoteNudged, PositionMs: 59_620, DriftMs: 380, Rate: 1.1},
	} {
		stage.hub.Handle(t.Context(), stage.guestDevice, Inbound{Type: MessageNote, Note: note})
	}
	var caughtUp []string
	for line := range strings.Lines(stage.logs.String()) {
		if strings.Contains(line, "logEvent=followerCaughtUp") {
			caughtUp = append(caughtUp, line)
		}
	}
	leads := fmt.Sprintf("leaderDeviceId=%d", stage.host.DeviceID)
	for index, want := range []string{
		"by=seek why=catchUp toMs=64000 behindMs=10000",
		"by=nudge behindMs=380 rate=1.1",
	} {
		if index >= len(caughtUp) || !strings.Contains(caughtUp[index], want) ||
			!strings.Contains(caughtUp[index], leads) || !strings.Contains(caughtUp[index], "level=INFO") {
			t.Errorf("catch-up %d is logged as %v; want at info, %s, %s", index, caughtUp, leads, want)
		}
	}
	stage.hub.Handle(t.Context(), stage.host, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteSought, PositionMs: 60_000, ToMs: 600_000, Why: "command",
	}})
	if strings.Count(stage.logs.String(), "logEvent=followerCaughtUp") != 2 {
		t.Errorf("the leader's own seek was logged as a follower's catch-up:\n%s", stage.logs)
	}
}
