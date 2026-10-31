// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"strings"
	"testing"
)

// offByOne is secret with its last character changed: its length and alphabet, but not it.
func offByOne(secret string) string {
	replacement := "A"
	if strings.HasSuffix(secret, "A") {
		replacement = "B"
	}
	return secret[:len(secret)-1] + replacement
}

// wrongFor are the wrong secrets for a check of the kind own whose live secret is live: one empty,
// one well formed but no one's, one a character off or with a space after the live one, and the
// live secret of each other kind.
func wrongFor(kinds map[string]string, own, live string) map[string]string {
	wrong := map[string]string{
		"empty": "", "well formed but no one's": strings.Repeat("A", len(live)),
		"one character off": offByOne(live), "with a space after": live + " ",
	}
	for kind, secret := range kinds {
		if kind != own {
			wrong[kind] = secret
		}
	}
	return wrong
}

// Every credential the service checks is checked for what it is, never for being there: a session,
// an invitation link, a paired device's secret and a hand-off's code each refuse one that is empty,
// well formed but no one's, one character off a live one, and each of the other kinds of secret the
// service issues, and the live one still works after. Plex2Shell's first step was a token checked
// only for being present (security-week-2026-10-30 §5).
func TestCredentials_EachCheckRefusesAWrongOneOfItsShapeAndEveryOtherKind(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "owner", "right", RoleAdmin)
	signedIn, err := fixture.service.SignIn(ctx, "owner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	_, invitation, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: "a cousin"})
	if err != nil {
		t.Fatal(err)
	}
	asked, err := fixture.service.RequestPairing(ctx, "", "web", PreviousDevice{}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.ClaimPairing(ctx, id, id, asked.Code, "Living room TV"); err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.AnswerPairing(ctx, id, asked.Code, true); err != nil {
		t.Fatal(err)
	}
	handOff, err := fixture.service.HandOff(ctx, "owner", "right", "192.0.2.1", laptop)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{
		"a session token": signedIn.Token, "a device secret": signedIn.DeviceSecret,
		"an invitation link": invitation.Secret, "a pairing device's secret": asked.Secret,
		"a pairing's code": asked.Code, "a hand-off's code": handOff,
	}
	// Each check, by the kind it takes and its live one: admits says whether it admitted a secret.
	checks := []struct {
		kind, live string
		admits     func(secret string) bool
	}{
		{"a session token", signedIn.Token, func(token string) bool {
			_, err := fixture.service.Authenticate(ctx, token)
			return err == nil
		}},
		{"an invitation link", invitation.Secret, func(secret string) bool {
			_, err := fixture.service.KindOfLink(ctx, secret)
			return err == nil
		}},
		{"a pairing device's secret", asked.Secret, func(secret string) bool {
			// Taking the live one's session ends its pairing, so the live one is asked once, last.
			stands := fixture.service.PairingStateOf(ctx, secret).State != PairingEnded
			_, err := fixture.service.TakePairedSession(ctx, secret)
			return stands || err == nil
		}},
		{"a hand-off's code", handOff, func(code string) bool {
			_, err := fixture.service.TakeHandOff(ctx, code)
			return err == nil
		}},
	}
	for _, check := range checks {
		for name, secret := range wrongFor(kinds, check.kind, check.live) {
			if check.admits(secret) {
				t.Errorf("%s: %s admitted", check.kind, name)
			}
		}
	}
	// What was refused above spent none of the live ones.
	for _, check := range checks {
		if !check.admits(check.live) {
			t.Errorf("%s: the live one no longer admits", check.kind)
		}
	}
}
