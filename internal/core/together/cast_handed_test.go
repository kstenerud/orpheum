// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"testing"
	"time"
)

// A cast hands its castee the film put on it, from where it was put, until the cast's group plays
// that film on: then from where the group stands, paused as it is, so a castee loaded again picks it
// up there, not where it started (D555 §2). Another film put starts from where it is put, and the
// castee's taking the cast is kept across both (D468 §1).
func TestCast_ACastHandsItsCasteeTheFilmFromWhereItsGroupStands(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner, "")
	if err != nil {
		t.Fatal(err)
	}
	film := CastFilm{FileID: 7, ItemID: 70, ItemName: "synthetic film", DurationMs: 3_600_000}
	startMs := int64(5_000)
	if err = stage.hub.CastPuts(cast.CastID, stage.owner.UserID, film, &startMs, true); err != nil {
		t.Fatal(err)
	}
	stage.hub.CastHeard(cast.CastID, stage.owner.UserID)
	stage.wantHanded(t, cast.CastID, "a cast just put", CastHanded{
		Film: film, PositionMs: &startMs, IsPaused: true, IsAccepted: true,
	})
	castee, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	stage.command(t, castee, CommandChangeItem, film.FileID)
	stage.ready(t, castee, false)
	stage.clock.Advance(12 * time.Second)
	wantMs := stage.groupPositionMs(cast.TogetherSessionID)
	stage.wantHanded(t, cast.CastID, "a cast whose group plays on", CastHanded{
		Film: film, PositionMs: &wantMs, IsAccepted: true,
	})
	other := CastFilm{FileID: 8, ItemID: 80, ItemName: "movie name", DurationMs: 1_000}
	if err = stage.hub.CastPuts(cast.CastID, stage.owner.UserID, other, nil, false); err != nil {
		t.Fatal(err)
	}
	stage.wantHanded(t, cast.CastID, "another film put", CastHanded{Film: other, IsAccepted: true})
}

// wantHanded fails the test, saying what, unless the owner's cast castID hands its castee want now.
func (stage sessionStage) wantHanded(t *testing.T, castID int64, what string, want CastHanded) {
	t.Helper()
	handed, err := stage.hub.CastHandedOf(castID, stage.owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if handed.Film != want.Film || (handed.PositionMs == nil) != (want.PositionMs == nil) ||
		(want.PositionMs != nil && *handed.PositionMs != *want.PositionMs) || handed.IsPaused != want.IsPaused ||
		handed.IsAccepted != want.IsAccepted {
		t.Fatalf("%s is handed %+v from %v; want %+v from %v", what, handed, handed.PositionMs, want, want.PositionMs)
	}
}

// A cast alone that moves into the part its caster now takes in Watch Together hands its castee the
// film from where it stood alone, not from where it was first put: the session's group plays nothing
// of its yet (D555 §2, §7).
func TestCast_ACastMovingIntoItsCastersPartPlaysOnFromWhereItStood(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(alone, "")
	if err != nil {
		t.Fatal(err)
	}
	film := CastFilm{FileID: 7, ItemID: 70, ItemName: "synthetic film", DurationMs: 3_600_000}
	if err = stage.hub.CastPuts(cast.CastID, alone.UserID, film, nil, false); err != nil {
		t.Fatal(err)
	}
	castee, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	stage.command(t, castee, CommandChangeItem, film.FileID)
	stage.ready(t, castee, false)
	stage.clock.Advance(30 * time.Second)
	stoodMs := stage.groupPositionMs(castee.SessionID)
	if _, err = stage.hub.Open(alone); err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.CastCredential(cast.CastID, alone.UserID); err != nil {
		t.Fatal(err)
	}
	handed, err := stage.hub.CastHandedOf(cast.CastID, alone.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if handed.Film != film || handed.PositionMs == nil || *handed.PositionMs != stoodMs || handed.IsPaused {
		t.Errorf("a cast moved into its caster's part is handed %+v from %v; want %+v from %d ms, playing",
			handed, handed.PositionMs, film, stoodMs)
	}
}

// A person casting alone who joins another's session by its link takes the cast into their part at
// the join, before any device of theirs says hello there: the castee follows the session, handed the
// session's film from where the session stands, not its own film from where that stood (D555 §1,
// §7). Leaving, the cast plays on alone with the session's film from where the session stood as they
// left, not from where it stood at the join, and is told moved once (the walk on staging,
// 2026-11-25: the castee kept its own film, then at the leave was handed it from the join's
// position).
func TestCast_ACastJoiningASessionByLinkFollowsTheSessionAndLeavesWithItsFilm(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	var moved []EndedCast
	stage.hub.OnCastMoved(func(each EndedCast) { moved = append(moved, each) })
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 9)
	stage.ready(t, host, false)
	stage.clock.Advance(40 * time.Second)
	cast, err := stage.hub.OpenCast(alone, "")
	if err != nil {
		t.Fatal(err)
	}
	own := CastFilm{FileID: 7, ItemID: 70, ItemName: "synthetic film", DurationMs: 3_600_000}
	if err = stage.hub.CastPuts(cast.CastID, alone.UserID, own, nil, false); err != nil {
		t.Fatal(err)
	}
	castee, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	stage.command(t, castee, CommandChangeItem, own.FileID)
	stage.ready(t, castee, false)
	stage.clock.Advance(30 * time.Second)
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a friend", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := stage.hub.Join(made.Secret, "Friend", &alone, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	if len(joined.CastsMoved) != 1 || joined.CastsMoved[0] != cast.CastID {
		t.Fatalf("the join moved casts %v; want cast %d", joined.CastsMoved, cast.CastID)
	}
	if sessionID, isHost := stage.hub.CastPart(cast.CastID, alone.UserID); sessionID != stage.opened.SessionID ||
		isHost {
		t.Fatalf("the cast is a part in session %d (host %v); want a guest's in %d", sessionID, isHost,
			stage.opened.SessionID)
	}
	sessionMs := stage.groupPositionMs(stage.opened.SessionID)
	stage.wantPlaying(t, cast.CastID, "a cast that joined a session", 9, sessionMs)
	stage.clock.Advance(20 * time.Second)
	leftMs := stage.groupPositionMs(stage.opened.SessionID)
	if err = stage.hub.Leave(joined.MembershipCredential); err != nil {
		t.Fatal(err)
	}
	if sessionID, _ := stage.hub.CastPart(cast.CastID, alone.UserID); sessionID != 0 {
		t.Fatalf("after the leave the cast is a part in session %d; want it alone", sessionID)
	}
	stage.wantPlaying(t, cast.CastID, "a cast whose caster left the session", 9, leftMs)
	if len(moved) != 1 || moved[0].CastID != cast.CastID {
		t.Errorf("the hub told casts moved %+v; want cast %d once", moved, cast.CastID)
	}
}

// A host whose cast is their session's only player ends the session: the cast plays on alone from
// where the session stood, playing, though its castee's leaving paused the session it left (D555 §5,
// §7; the walk on staging, 2026-11-25: the Chromecast loaded its film again paused).
func TestCast_ACastThatWasItsSessionsOnlyPlayerPlaysOnAsTheSessionEnds(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(alone, "")
	if err != nil {
		t.Fatal(err)
	}
	film := CastFilm{FileID: 7, ItemID: 70, ItemName: "synthetic film", DurationMs: 3_600_000}
	if err = stage.hub.CastPuts(cast.CastID, alone.UserID, film, nil, false); err != nil {
		t.Fatal(err)
	}
	party, err := stage.hub.Open(alone)
	if err != nil {
		t.Fatal(err)
	}
	inParty, err := stage.hub.CastCredential(cast.CastID, alone.UserID)
	if err != nil {
		t.Fatal(err)
	}
	castee, _ := stage.attach(t, inParty, RoleVideo)
	stage.command(t, castee, CommandChangeItem, film.FileID)
	stage.ready(t, castee, false)
	stage.clock.Advance(30 * time.Second)
	stoodMs := stage.groupPositionMs(party.SessionID)
	if err = stage.hub.End(party.SessionID, alone.UserID); err != nil {
		t.Fatal(err)
	}
	stage.wantPlaying(t, cast.CastID, "a cast whose host ended the session it alone played in", film.FileID, stoodMs)
}

// A device that said hello in the Watch Together session its cast then left, its connection ended
// since, is sent nothing as the cast ends: the cast kept it to tell its end, no session holding it,
// and the server sent on its closed connection, failing Stop casting (the walk on staging,
// 2026-11-25).
func TestCast_ADeviceGoneSinceItsCastLeftTheSessionIsSentNothingAtTheCastsEnd(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(alone, "")
	if err != nil {
		t.Fatal(err)
	}
	party, err := stage.hub.Open(alone)
	if err != nil {
		t.Fatal(err)
	}
	inParty, err := stage.hub.CastCredential(cast.CastID, alone.UserID)
	if err != nil {
		t.Fatal(err)
	}
	castee, sent := stage.attach(t, inParty, RoleVideo)
	if err = stage.hub.End(party.SessionID, alone.UserID); err != nil {
		t.Fatal(err)
	}
	stage.hub.Detach(castee, GoneClosed)
	before := len(sent.messages)
	if err = stage.hub.EndCast(cast.CastID, alone.UserID, CastStopped); err != nil {
		t.Fatal(err)
	}
	if after := len(sent.messages); after != before {
		t.Fatalf("a device gone was sent %d messages at its cast's end: %+v", after-before, sent.messages[before:])
	}
}

// wantPlaying fails the test, saying what, unless alone's cast castID hands its castee the film
// fileID from positionMs, playing.
func (stage sessionStage) wantPlaying(t *testing.T, castID int64, what string, fileID, positionMs int64) {
	t.Helper()
	handed, err := stage.hub.CastHandedOf(castID, alone.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if handed.Film.FileID != fileID || handed.PositionMs == nil || *handed.PositionMs != positionMs ||
		handed.IsPaused {
		t.Fatalf("%s is handed %+v from %v; want film %d from %d ms, playing", what, handed, handed.PositionMs,
			fileID, positionMs)
	}
}

// A cast its castee has not taken within CastUnansweredAfter of its start ends, its castee told
// unanswered, and not before; one taken goes on however long (D358 §4, principles §28): a castee
// never handed the question, its caster's page gone, left the caster asking (the owner, 2026-11-03).
// The wait is the hub's, beside the silence's (D555 §2, §3).
func TestCast_ACastNeverAnsweredEndsAndOneAcceptedGoesOn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	unanswered, err := stage.hub.OpenCast(stage.owner, "")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := stage.hub.OpenCast(alone, "")
	if err != nil {
		t.Fatal(err)
	}
	_, castee := stage.attach(t, unanswered.ReceiverCredential, RoleVideo)
	stage.hub.CastHeard(accepted.CastID, alone.UserID)
	stage.clock.Advance(CastUnansweredAfter - time.Second)
	if ended := stage.hub.EndUnansweredCasts(); len(ended) != 0 {
		t.Fatalf("a cast asked %s ago was ended: %+v", CastUnansweredAfter-time.Second, ended)
	}
	stage.clock.Advance(time.Second)
	ended := stage.hub.EndUnansweredCasts()
	if len(ended) != 1 || ended[0] != (EndedCast{CastID: unanswered.CastID, UserID: stage.owner.UserID}) {
		t.Fatalf("after %s the hub ended %+v; want cast %d alone, %d taken", CastUnansweredAfter, ended,
			unanswered.CastID, accepted.CastID)
	}
	if told := castee.last(MessageEnded).Reason; told != CastUnanswered {
		t.Errorf("the unanswered castee was told %q; want %s", told, CastUnanswered)
	}
	if _, _, isMember := stage.hub.MemberOf(unanswered.ReceiverCredential); isMember {
		t.Error("the unanswered cast's castee credential still admits; want it taken back")
	}
	if _, err = stage.hub.CastCredential(unanswered.CastID, stage.owner.UserID); err == nil {
		t.Error("the unanswered cast is still held; want it gone, its caster's pages to be told")
	}
	if _, err = stage.hub.CastCredential(accepted.CastID, alone.UserID); err != nil {
		t.Errorf("the cast taken: %v; want it going on", err)
	}
}

// A guest's next or previous moves the host's cast along with the session, as it is for everyone
// (D471 §2, D555 §1): the castee, the host's playing device, is handed the episode put on, from where
// it was put on, and the cast is told once the hub is let go, so its castee is handed it again; a
// next refused moves nothing (the walk on staging, 2026-11-26: the television stayed on the episode
// before, and stopping the cast put that back on for the group).
func TestCast_AGuestsNextMovesTheHostsCastAlongWithTheSession(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	stage.hub.MovesAlongBy(neighbourOneAlong(stage.owner.UserID))
	var moved []EndedCast
	stage.hub.OnCastMovedAlong(func(each EndedCast) {
		if _, err := stage.hub.CastHandedOf(each.CastID, each.UserID); err != nil { // let go: it may call the hub
			t.Error(err)
		}
		moved = append(moved, each)
	})
	cast, err := stage.hub.OpenCast(stage.owner, "")
	if err != nil {
		t.Fatal(err)
	}
	film := CastFilm{FileID: 7, ItemName: "synthetic film", DurationMs: 3_600_000}
	if err = stage.hub.CastPuts(cast.CastID, stage.owner.UserID, film, nil, false); err != nil {
		t.Fatal(err)
	}
	castee, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	stage.command(t, castee, CommandChangeItem, film.FileID)
	stage.ready(t, castee, false)
	stage.clock.Advance(30 * time.Second)
	withheld, _ := stage.guestWith(t, Rights{CanPlayPause: true})
	stage.command(t, withheld, CommandNext, 0)
	if len(moved) != 0 {
		t.Fatalf("a refused next moved casts %v; want none", moved)
	}
	given, _ := stage.guestWith(t, Rights{CanNext: true})
	stage.command(t, given, CommandNext, 0)
	want := EndedCast{CastID: cast.CastID, UserID: stage.owner.UserID}
	if len(moved) != 1 || moved[0] != want {
		t.Fatalf("a guest's next moved casts %v; want %+v", moved, want)
	}
	startMs := int64(0)
	stage.wantHanded(t, cast.CastID, "the host's cast after a guest's next", CastHanded{
		Film: CastFilm{FileID: 8}, PositionMs: &startMs,
	})
	stage.ready(t, castee, false)
	stage.command(t, given, CommandPrevious, 0)
	backMs := int64(120_000)
	stage.wantHanded(t, cast.CastID, "the host's cast after a guest's previous", CastHanded{
		Film: CastFilm{FileID: 7}, PositionMs: &backMs,
	})
	if len(moved) != 2 {
		t.Errorf("a guest's next and previous moved casts %v; want the host's cast twice", moved)
	}
}
