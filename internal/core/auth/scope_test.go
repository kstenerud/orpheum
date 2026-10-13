// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"testing"
)

// A sign-in through a discovery's page, and a device a code activated, get the device scope: the
// person's role without administering and without changing the account's security; a sign-in at
// the core's own origin gets the whole role (D292 §4; the security audit's A1, A11).
func TestSession_ThroughADiscoveryOrByACodeCarriesNoAdminAndNoAccountSecurity(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "owner", "pw", RoleAdmin)
	principalOf := func(device NewDevice) Principal {
		t.Helper()
		token, err := fixture.service.SignIn(ctx, "owner", "pw", "192.0.2.10", device)
		if err != nil {
			t.Fatal(err)
		}
		principal, err := fixture.service.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		return principal
	}
	if own := principalOf(NewDevice{Name: "Laptop", Platform: "web"}); !own.IsAdmin() || !own.CanManageAccount() {
		t.Errorf("a sign-in at the core's own origin is %+v; want admin, managing its account", own)
	}
	through := principalOf(NewDevice{Name: "Phone", Platform: "web", IsThroughDiscovery: true})
	if through.IsAdmin() || through.CanManageAccount() || !through.CanHostWatchTogether() {
		t.Errorf("a sign-in through a discovery is %+v; want no admin, no account security, hosting kept", through)
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
	token, err := fixture.service.TakePairedSession(ctx, asked.Secret)
	if err != nil {
		t.Fatal(err)
	}
	television, err := fixture.service.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if television.IsAdmin() || television.CanManageAccount() {
		t.Errorf("an admin's television activated by a code is %+v; want no admin, no account security", television)
	}
}
