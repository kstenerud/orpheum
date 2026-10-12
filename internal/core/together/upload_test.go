// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"testing"
)

// The last resort for a server whose upload cannot carry everyone: the member who joined most
// recently with a player, never the host nor one with only voice, is told the session ended for them,
// and their membership still admits them, so they may come back at once (D286 §5).
func TestHub_SendAwayNewestTellsTheNewestPlayerAndKeepsTheirMembership(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	older := stage.join(t)
	_, olderTV := stage.attach(t, older.MembershipCredential, RolePlayback)
	newer := stage.join(t)
	_, newerPhone := stage.attach(t, newer.MembershipCredential, RolePlayback)
	voiceOnly := stage.join(t)
	_, voicePhone := stage.attach(t, voiceOnly.MembershipCredential, RoleVoice)
	sessionID, memberID, isSent := stage.hub.SendAwayNewest("uploadFull")
	if !isSent || sessionID != stage.opened.SessionID || memberID != newer.MemberID {
		t.Fatalf("sent away %d of %d (%t); want the newer player, %d", memberID, sessionID, isSent, newer.MemberID)
	}
	if ended := newerPhone.last(MessageEnded); ended.Reason != "uploadFull" {
		t.Errorf("the newer player was told %+v; want ended, uploadFull", ended)
	}
	if ended := olderTV.last(MessageEnded); ended.Type != "" {
		t.Errorf("the older player was told %+v; want nothing", ended)
	}
	if ended := voicePhone.last(MessageEnded); ended.Type != "" {
		t.Errorf("the member with voice alone was told %+v; want nothing", ended)
	}
	if _, err := stage.hub.Attach(newer.MembershipCredential, RolePlayback, true, nil, testTuning,
		(&inbox{}).send); err != nil {
		t.Errorf("the member sent away, coming back: %v; want admitted", err)
	}
}

// With no guest playing, no one is sent away: the host never is.
func TestHub_SendAwayNewestSendsNoHost(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, hostTV := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	if _, _, isSent := stage.hub.SendAwayNewest("uploadFull"); isSent {
		t.Error("someone was sent away; want no one, the host alone playing")
	}
	if ended := hostTV.last(MessageEnded); ended.Type != "" {
		t.Errorf("the host was told %+v; want nothing", ended)
	}
}
