// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/testutil"
)

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
		{Kind: NoteTakenBack, By: "select", How: "gapMs=450 upsBetween=0"},
		{Kind: NoteTakenBack, By: "nudge"},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &note})
	}
	logged := stage.logs.String()
	if muted := testutil.LineWith(t, logged, "playedMuted"); !strings.Contains(muted, `page="`+page+`"`) {
		t.Errorf("the log does not say the page's activation as it played muted:\n%s", logged)
	}
	if sounded := testutil.LineWith(t, logged, "playedWithSound"); !strings.Contains(sounded, "positionMs=1131172") ||
		!strings.Contains(sounded, `page="hasBeenActive=true"`) {
		t.Errorf("the log does not say the film played with sound, and the page's activation:\n%s", logged)
	}
	if unmuted := testutil.LineWith(t, logged, "unmutedTap"); !strings.Contains(unmuted, "by=key") {
		t.Errorf("the log does not say a key turned the sound on:\n%s", logged)
	}
	for _, want := range []string{"answer=refused by=\"\"", "answer=entered by=click", "answer=left by=\"\""} {
		if !strings.Contains(logged, "logEvent=fullScreen "+want) {
			t.Errorf("the log does not say full screen %s:\n%s", want, logged)
		}
	}
	// Which press took a cast's control back: a held OK on the Fire TV took it, and nothing said how
	// (VQ122, 2026-10-29).
	taken := testutil.LineWith(t, logged, "takenBack")
	if !strings.Contains(taken, `by=select how="gapMs=450 upsBetween=0"`) {
		t.Errorf("the log does not say which press took the cast's control back:\n%s", logged)
	}
	for _, unknown := range []string{"elbow", "maybe", "positionMs=7 ", "nudge"} {
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

// A page left for another site closes its socket at once and, back from the back/forward cache,
// connects again saying so: the log tells that return from a drop (the owner, 2026-10-29).
func TestHub_LogsAPageBackFromBeingLeft(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	note := PlayerNote{Kind: NoteReconnected, OfflineMs: 60000, Why: "left"}
	stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &note})
	if logged := stage.logs.String(); !strings.Contains(logged, "kind=reconnected offlineMs=60000 why=left") ||
		box.last(MessageRejected).Code != "" {
		t.Errorf("the log does not say the page connected again on coming back:\n%s", logged)
	}
}

// A page shown again says whether its socket answered and how fast, or connected again because it did
// not, and a play the browser refused then says how and whether the page had been pressed: an S23
// unlocked sat 20 s, paused, on a socket that heard nothing, and nothing said so (the owner's VQ105,
// 2026-10-29). An unknown word is refused.
func TestHub_LogsAPageShownAgainItsSocketsAnswerAndAPlayRefused(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	device, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	page := "hasBeenActive=true isActive=false loadedMsAgo=900000 navigation=navigate"
	for _, note := range []PlayerNote{
		{Kind: NoteShownAgain, Why: "alive", RoundTripMs: 80},
		{Kind: NoteReconnected, OfflineMs: 2100, Why: "shown"},
		{Kind: NotePlayRefused, Why: "AbortError", PositionMs: 2280010, Page: page},
	} {
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &note})
	}
	logged := stage.logs.String()
	if shown := testutil.LineWith(t, logged, "shownAgain"); !strings.Contains(shown, "level=INFO") ||
		!strings.Contains(shown, "socket=alive roundTripMs=80") {
		t.Errorf("the log does not say the socket answered in 80 ms:\n%s", logged)
	}
	if !strings.Contains(logged, "kind=reconnected offlineMs=2100 why=shown") {
		t.Errorf("the log does not say it connected again as nothing answered once shown:\n%s", logged)
	}
	if refused := testutil.LineWith(t, logged, "playRefused"); !strings.Contains(refused, "refusal=AbortError") ||
		!strings.Contains(refused, "positionMs=2280010") || !strings.Contains(refused, `page="`+page+`"`) {
		t.Errorf("the log does not say how the play was refused, where, and the page's activation:\n%s", logged)
	}
	for _, unknown := range []PlayerNote{
		{Kind: NoteShownAgain, Why: "dead"},
		{Kind: NotePlayRefused, Why: "Sulking"},
		{Kind: NoteReconnected, Why: "lost"},
	} {
		stage.logs.Reset()
		stage.hub.Handle(t.Context(), device, Inbound{Type: MessageNote, Note: &unknown})
		logged := stage.logs.String()
		if box.last(MessageRejected).Code != codeInvalid || strings.Contains(logged, string(unknown.Kind)) {
			t.Errorf("note %+v: logged or not refused:\n%s", unknown, stage.logs)
		}
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
	if line := testutil.LineWith(t, stage.logs.String(), "voiceOff"); !strings.Contains(line, "level=INFO") ||
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
// that it played with sound (the owner's film, staging, 2026-10-28). An unknown cause is
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
	played := testutil.LineWith(t, logged, "playedWhileHeld")
	for _, want := range []string{"level=INFO", "groupState=waiting", "by=autoplay", "positionMs=1607085"} {
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

// A catch-up's line says the parts of its aim: where its player expected the group, how far ahead it
// aimed for its load, by what, and what that rested on. A catch-up a minute ahead of its group said
// nothing of why (the owner's Chrome, staging, 2026-10-29). An aim by no known word is refused.
func TestHub_LogsACatchUpsAim(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	stage.clock.Advance(60 * time.Second)
	stage.hub.Handle(t.Context(), stage.guestDevice, Inbound{Type: MessageNote, Note: &PlayerNote{
		Kind: NoteSought, PositionMs: 50_000, ToMs: 72_000, Why: "catchUp", GroupAtMs: 60_000,
		AheadForLoadMs: 12_000, AimedBy: "fill", LastLoadMs: 2400, LoadsFaster: 2.5,
	}})
	caughtUp := testutil.LineWith(t, stage.logs.String(), "followerCaughtUp")
	want := "toMs=72000 behindMs=10000 groupAtMs=60000 aheadForLoadMs=12000 aimedBy=fill " +
		"lastLoadMs=2400 loadsFaster=2.5"
	if !strings.Contains(caughtUp, want) {
		t.Errorf("the catch-up is logged as %q; want %s", caughtUp, want)
	}
	if (PlayerNote{Kind: NoteSought, Why: "catchUp", AimedBy: "a\nforged line"}).isKnown() {
		t.Error("a catch-up aimed by an unknown word is known")
	}
}
