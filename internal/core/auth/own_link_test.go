// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The link a person registered by is kept sealed in its grant, never as itself, and shown back to
// them as their own link; without a sealer, or for a link sealed under another key, the link is
// still theirs but cannot be shown (discovery-core.md K7).
func TestOwnLink_IsShownBackFromItsSealedSecretAndNeverKeptPlain(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sealer := secrets.NewStore(fixture.database, key, fixture.clock, rand.Reader, fixture.settings)
	fixture.service.SealLinksWith(sealer)
	id := fixture.createUser(t, "anna", "first", RoleUser)
	invitation, err := fixture.service.IssueRegistrationInvitation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Register(ctx, invitation.Secret, "", "second", NewDevice{}); err != nil {
		t.Fatal(err)
	}
	own, has, err := fixture.service.OwnLink(ctx, id)
	if err != nil || !has || own.Secret != invitation.Secret {
		t.Errorf("OwnLink = %+v, %v, %v; want the link she registered by", own, has, err)
	}
	row, err := fixture.database.Read().GetOwnLinkOfUser(ctx, sqlcgen.GetOwnLinkOfUserParams{
		UserID: &id, Now: new(fixture.clock.Now().UnixMilli()),
	})
	if err != nil || len(row.LinkSealed) == 0 || bytes.Contains(row.LinkSealed, []byte(invitation.Secret)) {
		t.Errorf("the grant keeps %x (%v); want the link sealed, never itself", row.LinkSealed, err)
	}
	other, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := secrets.NewStore(fixture.database, other, fixture.clock, rand.Reader, fixture.settings)
	fixture.service.SealLinksWith(elsewhere)
	if own, has, err = fixture.service.OwnLink(ctx, id); err != nil || !has || own.Secret != "" {
		t.Errorf("under another key OwnLink = %+v, %v, %v; want her link, unshown", own, has, err)
	}
}
