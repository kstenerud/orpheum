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
