// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// karlsKey is the realm key the core finds karl by in realm, or nil.
func karlsKey(t *testing.T, service *Service, realm string) []byte {
	t.Helper()
	return keyOf(t, service, realm, "karl")
}

// karlsPersonKey is karl's version-2 key from password, under the fixture's key salt.
func karlsPersonKey(password string) []byte {
	return proof.PersonKey(fixtureKeySalt, "karl", password)
}

// A person's away-from-home key is the version 2, made from their password whenever the core sees it,
// whether or not it is linked to a discovery, and whenever the password changes; it is salted by the
// core's key salt, not the household's name, so a name given or changed later leaves it working, and
// a realm sign-in's offers are proven by it (D527 §2.2).
func TestPersonKey_IsMadeFromThePasswordWheneverTheCoreSeesItLinkedOrNot(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	id := fixture.createUser(t, "Karl", "a password", RoleUser)
	if has, err := fixture.service.HasPersonKey(ctx, id); err != nil || !has {
		t.Fatalf("before the core had a realm, karl has a key: %t, %v; want one", has, err)
	}
	fixture.service.SetRealm("karlnas")
	if key := karlsKey(t, fixture.service, "karlnas"); !bytes.Equal(key, karlsPersonKey("a password")) {
		t.Errorf("once the core has the realm karlnas, karl's key is %x; want his password's", key)
	}
	offerKey, err := fixture.service.OfferKey(ctx, proof.RealmPerson("karlnas", "karl"))
	if err != nil || !bytes.Equal(offerKey, karlsPersonKey("a password")) {
		t.Errorf("an offer under karl's person is proven under %x, %v; want his key", offerKey, err)
	}
	if err = fixture.service.ChangePassword(ctx, id, "a password", "another password", ""); err != nil {
		t.Fatal(err)
	}
	if key := karlsKey(t, fixture.service, "karlnas"); !bytes.Equal(key, karlsPersonKey("another password")) {
		t.Error("a new password did not make a new key")
	}
	// Another name for the household needs no password: the key stands, found by the new realm.
	fixture.service.SetRealm("another")
	if key := karlsKey(t, fixture.service, "another"); !bytes.Equal(key, karlsPersonKey("another password")) {
		t.Error("after the household was named again, karl is not found by the new name with his key")
	}
	if key := karlsKey(t, fixture.service, "karlnas"); key != nil {
		t.Error("the realm the core no longer has still finds karl")
	}
	// A subrealm given later needs no password either: the person is found by it with the same key.
	fixture.service.SetRealm("another.live")
	if key := karlsKey(t, fixture.service, "another.live"); !bytes.Equal(key, karlsPersonKey("another password")) {
		t.Error("after a subrealm was given, karl is not found by it with his key")
	}
	// Signing in under the new name makes no other key: the key salt, not the name, salts it.
	if _, err = fixture.service.SignIn(ctx, "Karl", "another password", "192.0.2.1", laptop); err != nil {
		t.Fatal(err)
	}
	if key := karlsKey(t, fixture.service, "another.live"); !bytes.Equal(key, karlsPersonKey("another password")) {
		t.Error("signing in under the household's new name changed karl's key")
	}
}

// A person whose version-1 key the core kept from before signs in from away with it until the core
// sees their password, which makes the version 2 and drops the version 1; while both are held, a
// sign-in under either is taken and answered under that one (D527 §2.2).
func TestTakeRealmSignIn_TakesTheOldKeyUntilThePasswordIsSeenAndThenTheNew(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.service.SetRealm("karlnas")
	id := fixture.createUser(t, "karl", "a password", RoleUser)
	person := proof.RealmPerson("karlnas", "karl")
	oldKey := proof.RealmKey("karlnas", "karl", "a password")
	underOld := proof.RealmAsk(oldKey, "karlnas", person)
	underNew := proof.RealmAsk(karlsPersonKey("a password"), "karlnas", person)
	// Both held, as for a moment no code path leaves them: each sign-in is answered under its own key.
	storetest.ExecRaw(t, fixture.database.Path(), fmt.Sprintf(
		"INSERT INTO realm_key (user_id, master_realm, key, made_at) VALUES (%d, 'karlnas', X'%x', 1)", id, oldKey))
	for what, ask := range map[string]string{"old": underOld, "new": underNew} {
		want := map[string][]byte{"old": oldKey, "new": karlsPersonKey("a password")}[what]
		if key, err := fixture.service.TakeRealmSignIn(ctx, person, ask); err != nil || !bytes.Equal(key, want) {
			t.Errorf("with both keys held, a sign-in under the %s = %x, %v; want that key", what, key, err)
		}
	}
	// Only the old, as the core holds a person's key from before this release.
	storetest.ExecRaw(t, fixture.database.Path(), fmt.Sprintf("DELETE FROM person_key WHERE user_id = %d", id))
	if key, err := fixture.service.TakeRealmSignIn(ctx, person, underOld); err != nil || !bytes.Equal(key, oldKey) {
		t.Errorf("a sign-in under the old key the core kept = %x, %v; want the old key", key, err)
	}
	if _, err := fixture.service.TakeRealmSignIn(ctx, person, underNew); !errors.Is(err, ErrRealmRefused) {
		t.Errorf("a sign-in under a new key the core has not made = %v; want ErrRealmRefused", err)
	}
	if _, err := fixture.service.SignIn(ctx, "karl", "a password", "192.0.2.1", laptop); err != nil {
		t.Fatal(err)
	}
	if key, err := fixture.service.TakeRealmSignIn(ctx, person, underNew); err != nil ||
		!bytes.Equal(key, karlsPersonKey("a password")) {
		t.Errorf("after a sign-in at home, a sign-in under the new key = %x, %v; want it", key, err)
	}
	if _, err := fixture.service.TakeRealmSignIn(ctx, person, underOld); !errors.Is(err, ErrRealmRefused) {
		t.Errorf("after a sign-in at home, a sign-in under the old key = %v; want ErrRealmRefused", err)
	}
}

// A realm sign-in the discovery passes on is taken only when its proof holds under the person's key,
// and a guesser asking through the discovery is held to the person's limit (D297 §2; A13).
func TestTakeRealmSignIn_TakesARightProofAndHoldsAGuesserToThePersonsLimit(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.service.SetRealm("karlnas")
	fixture.createUser(t, "karl", "a password", RoleUser)
	person := proof.RealmPerson("karlnas", "karl")
	right := proof.RealmAsk(karlsPersonKey("a password"), "karlnas", person)
	key, err := fixture.service.TakeRealmSignIn(ctx, person, right)
	if err != nil || !bytes.Equal(key, karlsPersonKey("a password")) {
		t.Fatalf("karl's right sign-in = %x, %v; want his key", key, err)
	}
	wrong := proof.RealmAsk(karlsPersonKey("a guess"), "karlnas", person)
	anna := proof.RealmPerson("karlnas", "anna")
	if _, err = fixture.service.TakeRealmSignIn(ctx, anna, right); !errors.Is(err, ErrRealmRefused) {
		t.Errorf("a sign-in for no one = %v; want ErrRealmRefused", err)
	}
	for range 200 {
		if _, err = fixture.service.TakeRealmSignIn(ctx, person, wrong); errors.Is(err, ErrRateLimited) {
			break
		}
		if !errors.Is(err, ErrRealmRefused) {
			t.Fatalf("a wrong guess = %v; want ErrRealmRefused", err)
		}
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatal("200 wrong guesses were never held back")
	}
	if _, err = fixture.service.TakeRealmSignIn(ctx, person, right); !errors.Is(err, ErrRateLimited) {
		t.Errorf("the right sign-in past the limit = %v; want ErrRateLimited until the window passes", err)
	}
}

// heldCode is a setup code kept in memory, as SetupCodes.
type heldCode struct{ code string }

func (held *heldCode) Read() (string, error) {
	if held.code == "" {
		return "", ErrNoSetupCode
	}
	return held.code, nil
}

func (held *heldCode) Write(code string) error {
	held.code = code
	return nil
}

func (held *heldCode) Remove() error {
	held.code = ""
	return nil
}

// keyOf is the realm key the core finds name by in realm, or nil.
func keyOf(t *testing.T, service *Service, realm, name string) []byte {
	t.Helper()
	key, err := service.RealmKeyOfPerson(t.Context(), proof.RealmPerson(realm, name))
	if err != nil && !errors.Is(err, ErrRealmRefused) {
		t.Fatal(err)
	}
	return key
}

// Every way a password is set makes its person's away-from-home key, with no sign-in after: the first
// admin's at setup, an invitation's person's, and the admin an operator's reset names (D297 §3; D527
// §2.2).
func TestRealmKey_IsMadeWheneverAPasswordIsSet(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.service.SetRealm("karlnas")
	codes := &heldCode{}
	code, err := fixture.service.EnsureSetupCode(ctx, codes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.CompleteSetup(ctx, codes, code, NewUser{Name: "owner", Password: "first"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyOf(t, fixture.service, "karlnas", "owner"), proof.PersonKey(fixtureKeySalt, "owner", "first")) {
		t.Error("the first admin's setup made no key")
	}
	_, link, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: "Grandma"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Register(ctx, link.Secret, "Gran", "gran's", laptop); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyOf(t, fixture.service, "karlnas", "gran"), proof.PersonKey(fixtureKeySalt, "gran", "gran's")) {
		t.Error("an invitation's person made no key")
	}
	if _, err = fixture.service.ResetAdmin(ctx, "owner", "reset"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyOf(t, fixture.service, "karlnas", "owner"), proof.PersonKey(fixtureKeySalt, "owner", "reset")) {
		t.Error("the operator's reset made no key from the new password")
	}
}
