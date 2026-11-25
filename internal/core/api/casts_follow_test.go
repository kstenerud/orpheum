// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// A cast outlives the Watch Together session it took a part in, its castee going where its member
// goes (D555 §7): a person's cast of the group, its receiver following in the part their page holds,
// plays on alone as the host ends the session, the caster having every right to it without the
// session; the TV is handed it again, alone, nothing following, and the sender still lists it. The
// owner's staging, 2026-10-30, was its opposite (D389 §1, which D555 §7 amends): the TV played on under
// a cast no page controlled (VQ193); now one does, the cast's own.
func TestCast_ACastPlaysOnAloneAsTheSessionItTookAPartInEnds(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	tv := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(tv)
	hosted := harness.openTogether(t, phoneToken)
	guest := harness.joinByLink(t, phoneToken, hosted.Body.SessionID, "my phone", "Phone")
	cast := startedCast(t, harness.do(t, http.MethodPost, "/me/casts", phoneToken, map[string]any{
		"toDeviceId": tv, "togetherCredential": guest.Body.MembershipCredential,
	}))
	handed := tvSocket.await(live.MessageReceive, func(live.Outbound) bool { return true }).Receive
	if handed.TogetherCredential == "" {
		t.Fatalf("the TV was handed %+v; want the guest's part", handed)
	}
	path := fmt.Sprintf("/together/%d", hosted.Body.SessionID)
	if ended := harness.do(t, http.MethodDelete, path, phoneToken, nil); ended.status != http.StatusNoContent {
		t.Fatalf("DELETE %s = %d %s", path, ended.status, ended.body)
	}
	again := tvSocket.await(live.MessageReceive, func(message live.Outbound) bool {
		return message.Receive.CastID == cast.CastID && message.Receive.TogetherCredential == ""
	}).Receive
	alone := harness.dialCast(t, again.Credential, together.RoleVideo)
	if welcome := alone.awaitWelcome(); !welcome.IsHost || welcome.SessionID == hosted.Body.SessionID {
		t.Errorf("the TV's welcome = %+v; want the cast's own session, its castee leading", welcome)
	}
	if listed := harness.do(t, http.MethodGet, "/me/casts", phoneToken, nil); !strings.Contains(listed.body,
		fmt.Sprintf(`"castId":%d`, cast.CastID)) {
		t.Errorf("GET /me/casts = %d %s; want the cast going on", listed.status, listed.body)
	}
}

// A guest whose right to the film came from the session loses their cast with it (D555 §7): a person
// who does not reach the host's film casts the group's to their TV, and as the host ends the session
// the cast ends as sessionEnded, the TV going home and the remote told, and they list no cast.
func TestCast_AGuestsCastOfTheHostsFilmEndsWithTheSession(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	hostToken := harness.createAdminAndSignIn(t)
	partnerToken := harness.signInPartner(t)
	tvToken := harness.signIn(t, "partner", "partner-password")
	television := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(television)
	hosted := harness.openTogether(t, hostToken)
	player := harness.dialCast(t, hosted.Body.MembershipCredential, together.RoleVideo)
	player.awaitWelcome()
	player.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandChangeItem, FileID: 1,
	})
	player.send(together.Inbound{V: together.ProtocolVersion, Type: together.MessageState, IsPlaying: true, FileID: 1})
	player.awaitGroup(together.GroupPlaying)
	partner := harness.joinByLinkAs(t, hostToken, partnerToken, hosted.Body.SessionID)
	cast := startedCast(t, harness.do(t, http.MethodPost, "/me/casts", partnerToken, map[string]any{
		"toDeviceId": television, "togetherCredential": partner.Body.MembershipCredential,
	}))
	handed := tvSocket.await(live.MessageReceive, func(live.Outbound) bool { return true }).Receive
	receiver := harness.dialCast(t, handed.Credential, together.RoleVideo)
	receiver.awaitWelcome()
	remote := harness.dialCast(t, cast.MembershipCredential, together.RolePanel)
	remote.awaitWelcome()
	path := fmt.Sprintf("/together/%d", hosted.Body.SessionID)
	if ended := harness.do(t, http.MethodDelete, path, hostToken, nil); ended.status != http.StatusNoContent {
		t.Fatalf("DELETE %s = %d %s", path, ended.status, ended.body)
	}
	if reason := receiver.awaitEnded(); reason != together.CastSessionEnded {
		t.Errorf("the TV's cast was told %q, want %q", reason, together.CastSessionEnded)
	}
	if reason := remote.awaitEnded(); reason != together.CastSessionEnded {
		t.Errorf("the phone's remote was told %q, want %q", reason, together.CastSessionEnded)
	}
	tvSocket.await(live.MessageDevices, func(message live.Outbound) bool {
		return len(message.ReceivingDeviceIDs) == 0
	})
	if listed := harness.do(t, http.MethodGet, "/me/casts", partnerToken, nil); !strings.Contains(listed.body,
		`"casts":[]`) {
		t.Errorf("GET /me/casts = %d %s; want no cast", listed.status, listed.body)
	}
}

// Joining a session takes the member's running cast into it (D555 §7): a person casting alone joins
// another's session by its link signed in, and their TV is handed the cast again with their part in
// it, following the host.
func TestCast_JoiningASessionTakesTheCastIntoIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	hostToken := harness.createAdminAndSignIn(t)
	partnerToken := harness.signInPartner(t)
	tvToken := harness.signIn(t, "partner", "partner-password")
	tv := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(tv)
	cast := startedCast(t, harness.do(t, http.MethodPost, "/me/casts", partnerToken, map[string]any{"toDeviceId": tv}))
	tvSocket.await(live.MessageReceive, func(message live.Outbound) bool {
		return message.Receive.CastID == cast.CastID && message.Receive.TogetherCredential == ""
	})
	hosted := harness.openTogether(t, hostToken)
	harness.joinByLinkAs(t, hostToken, partnerToken, hosted.Body.SessionID)
	again := tvSocket.await(live.MessageReceive, func(message live.Outbound) bool {
		return message.Receive.CastID == cast.CastID && message.Receive.TogetherCredential != ""
	}).Receive
	if !again.IsTogetherFollower {
		t.Errorf("the TV was handed %+v; want the partner's part, following the host", again)
	}
	castee := harness.dialCast(t, again.Credential, together.RoleVideo)
	if welcome := castee.awaitWelcome(); welcome.IsHost || welcome.SessionID != hosted.Body.SessionID {
		t.Errorf("the TV's welcome = %+v; want the partner's part in session %d", welcome, hosted.Body.SessionID)
	}
}

// A person casting a film alone who joins a session playing another takes the cast into their part
// as its follower (D555 §7): the TV is handed the session's film, by name, from where the session
// stands, and the caster's remote is told it, not the film it played alone (the walk on staging,
// 2026-11-25: the TV kept its own film while joined to the session, the remote naming it too).
func TestCast_JoiningASessionWhileCastingAFilmPutsTheSessionsFilmOn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	hostToken := harness.createAdminAndSignIn(t)
	partnerToken := harness.signInPartner(t)
	harness.shelveFilms(t)
	harness.fileFilmAsTitle(t, "Metropolis")
	tvToken := harness.signIn(t, "partner", "partner-password")
	tv := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(tv)
	phoneSocket := harness.dialLive(t, partnerToken)
	cast := startedCast(t, harness.do(t, http.MethodPost, "/me/casts", partnerToken, map[string]any{"toDeviceId": tv}))
	playPath := fmt.Sprintf("/me/casts/%d/play", cast.CastID)
	if played := harness.do(t, http.MethodPost, playPath, partnerToken,
		map[string]any{"fileId": 2, "positionMs": 7_000}); played.status != http.StatusOK {
		t.Fatalf("POST %s = %d %s", playPath, played.status, played.body)
	}
	tvSocket.await(live.MessageReceive, func(message live.Outbound) bool { return message.Receive.FileID == 2 })
	hosted := harness.openTogether(t, hostToken)
	player := harness.dialCast(t, hosted.Body.MembershipCredential, together.RoleVideo)
	player.awaitWelcome()
	player.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandChangeItem, FileID: 1,
	})
	player.send(together.Inbound{V: together.ProtocolVersion, Type: together.MessageState, IsPlaying: true, FileID: 1})
	player.awaitGroup(together.GroupPlaying)
	harness.joinByLinkAs(t, hostToken, partnerToken, hosted.Body.SessionID)
	handed := tvSocket.await(live.MessageReceive, func(message live.Outbound) bool {
		return message.Receive.CastID == cast.CastID && message.Receive.TogetherCredential != "" &&
			message.Receive.FileID == 1 && message.Receive.ItemName == "Metropolis"
	}).Receive
	if !handed.IsTogetherFollower || handed.PositionMs == nil {
		t.Errorf("the TV was handed %+v; want the session's film 1 from where it stands, following", handed)
	}
	phoneSocket.await(live.MessageSending, func(message live.Outbound) bool {
		return message.Sending != nil && message.Sending.FileID == 1 && message.Sending.ItemName == "Metropolis"
	})
}

// A host starting another session keeps their cast (D555 §7): the host casts their part in one
// session, opens another, and their TV is handed the cast again with their part in the new one,
// leading it, rather than ending with the first.
func TestCast_AHostOpeningAnotherSessionKeepsTheCast(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	phoneToken := harness.createAdminAndSignIn(t)
	tvToken := harness.signIn(t, "owner", "owner-password")
	television := harness.thisDevice(t, tvToken)
	tvSocket := harness.dialLive(t, tvToken)
	tvSocket.awaitDevices(television)
	first := harness.openTogether(t, phoneToken)
	cast := startedCast(t, harness.do(t, http.MethodPost, "/me/casts", phoneToken, map[string]any{
		"toDeviceId": television, "togetherCredential": first.Body.MembershipCredential,
	}))
	tvSocket.await(live.MessageReceive, func(message live.Outbound) bool {
		return message.Receive.CastID == cast.CastID && message.Receive.TogetherCredential != ""
	})
	second := harness.openTogether(t, phoneToken)
	var welcome together.Outbound
	tvSocket.await(live.MessageReceive, func(message live.Outbound) bool {
		if message.Receive.CastID != cast.CastID || message.Receive.TogetherCredential == "" {
			return false
		}
		castee := harness.dialCast(t, message.Receive.Credential, together.RoleVideo)
		welcome = castee.awaitWelcome()
		return welcome.SessionID == second.Body.SessionID
	})
	if !welcome.IsHost {
		t.Errorf("the TV's welcome = %+v; want the host's part in session %d", welcome, second.Body.SessionID)
	}
}

// joinByLinkAs joins the session sessionID that host hosts by a new link of theirs, signed in as
// guestToken's person.
func (harness testHarness) joinByLinkAs(
	t *testing.T, host, guestToken string, sessionID int64,
) api.JoinTogetherOutput {
	t.Helper()
	made := harness.do(t, http.MethodPost, fmt.Sprintf("/together/%d/links", sessionID), host,
		map[string]string{"hostNote": "a friend"})
	var link api.TogetherLinkOutput
	if err := json.Unmarshal([]byte(made.body), &link.Body); err != nil || made.status != http.StatusCreated {
		t.Fatalf("POST /together/%d/links = %d %s", sessionID, made.status, made.body)
	}
	joined := harness.do(t, http.MethodPost, "/together/join", guestToken,
		map[string]any{"secret": link.Body.Secret, "name": "Friend"})
	var membership api.JoinTogetherOutput
	if err := json.Unmarshal([]byte(joined.body), &membership.Body); err != nil || joined.status != http.StatusOK {
		t.Fatalf("POST /together/join = %d %s", joined.status, joined.body)
	}
	return membership
}

// A caster coming back is told its running cast and where it stands (D555 §4): the phone's page,
// loaded again while its cast plays paused at 42 s, lists it with that position, not playing.
func TestCast_ACasterComingBackIsToldWhereItsCastStands(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	session, tokens, sockets := harness.sessionWithNothingOn(t)
	playPath := fmt.Sprintf("/me/casts/%d/play", session.CastID)
	if played := harness.do(t, http.MethodPost, playPath, tokens[0],
		map[string]any{"fileId": 1, "positionMs": 7_000}); played.status != http.StatusOK {
		t.Fatalf("POST %s = %d %s", playPath, played.status, played.body)
	}
	handed := sockets[1].await(live.MessageReceive, func(message live.Outbound) bool {
		return message.Receive.FileID == 1
	}).Receive
	castPlayer := harness.dialCast(t, handed.Credential, together.RoleVideo)
	castPlayer.awaitWelcome()
	castPlayer.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandChangeItem,
		FileID: 1, PositionMs: 7_000,
	})
	castPlayer.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageState, IsPlaying: true, PositionMs: 42_000, FileID: 1,
	})
	castPlayer.awaitGroup(together.GroupPlaying)
	castPlayer.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandPause,
	})
	castPlayer.awaitGroup(together.GroupPaused)
	listed := harness.do(t, http.MethodGet, "/me/casts", tokens[0], nil)
	var mine api.MyCastsOutput
	if err := json.Unmarshal([]byte(listed.body), &mine.Body); err != nil || len(mine.Body.Casts) != 1 {
		t.Fatalf("GET /me/casts = %d %s; want the one cast", listed.status, listed.body)
	}
	if cast := mine.Body.Casts[0]; cast.CastID != session.CastID || cast.PositionMs != 42_000 || cast.IsPlaying {
		t.Errorf("the phone's cast came back as %+v; want cast %d paused at 42 s", cast, session.CastID)
	}
	if state := mine.Body.Casts[0].CastState; state != "starting" {
		t.Errorf("the phone's cast came back %q, its castee saying nothing; want starting", state)
	}
	castPlayer.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCastState, CastState: together.CastRunning,
	})
	castPlayer.awaitRoster(func(roster []together.MemberView) bool {
		return slices.ContainsFunc(roster, func(each together.MemberView) bool {
			return slices.ContainsFunc(each.Devices, func(attached together.DeviceView) bool {
				return attached.CastState == together.CastRunning
			})
		})
	})
	listed = harness.do(t, http.MethodGet, "/me/casts", tokens[0], nil)
	if err := json.Unmarshal([]byte(listed.body), &mine.Body); err != nil || len(mine.Body.Casts) != 1 ||
		mine.Body.Casts[0].CastState != "running" {
		t.Errorf("GET /me/casts = %d %s; want the cast running, as its castee said (D555 §4)", listed.status,
			listed.body)
	}
}

// awaitRoster reads until a roster isIt takes, which fails the test when none comes in time.
func (opened socket) awaitRoster(isIt func([]together.MemberView) bool) {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	for {
		var message together.Outbound
		if err := wsjson.Read(ctx, opened.connection, &message); err != nil {
			opened.t.Fatalf("no such roster: %v", err)
		}
		if message.Type == together.MessageRoster && isIt(message.Roster) {
			return
		}
	}
}
