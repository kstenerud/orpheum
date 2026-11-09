// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A credential that ended more than endedCredentialsKeptDays ago is deleted, with what names it;
// one that ended since, and every live one, stays (D304 §6).
func TestPurgeEndedCredentials_DeletesThoseEndedPastTheirKeeping(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	partner := fixture.createUser(t, "partner", "pw", RoleUser)
	phone, _ := phoneAndTelevision(t, fixture.service, partner)
	if err := fixture.service.SignOut(ctx, phone); err != nil {
		t.Fatal(err)
	}
	fixture.clock.Advance(91 * 24 * time.Hour) // past the 90 days the phone's ended session is kept
	laptop, err := fixture.service.SignIn(ctx, "partner", "pw", "192.0.2.1", NewDevice{Name: "Laptop", Platform: "web"})
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.SignOut(ctx, laptop.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.PurgeEndedCredentials(ctx); err != nil {
		t.Fatal(err)
	}
	for device, want := range map[string]string{"Phone": "0", "Laptop": "1"} {
		got := storetest.QueryRaw(t, fixture.database.Path(),
			`SELECT count(*) FROM "grant" JOIN device ON device.id = "grant".device_id `+
				`WHERE "grant".kind = 'session' AND device.name = '`+device+`'`)
		if !slices.Equal(got, []string{want}) {
			t.Errorf("the %s's session after the purge: %v; want %s, ended 91 days ago or today", device, got, want)
		}
	}
}
