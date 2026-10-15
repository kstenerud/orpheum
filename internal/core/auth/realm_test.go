// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/discovery/proof"
)

// realmOf is a realm the test changes, as the core's discovery would.
type realmOf struct {
	mutex sync.Mutex
	realm string
}

func (held *realmOf) get() string {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	return held.realm
}

func (held *realmOf) set(realm string) {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	held.realm = realm
}

// karlsKey is the realm key the core finds karl by in realm, or nil.
func karlsKey(t *testing.T, service *Service, realm string) []byte {
	t.Helper()
	return keyOf(t, service, realm, "karl")
}

// A person's realm key is made from their password when the core has a realm: on a sign-in that
// finds none for it, as after the realm was set or changed, and whenever the password changes; a
// realm sign-in's offers are proven by it (D297 §3).
func TestRealmKey_IsMadeFromThePasswordWheneverTheCoreSeesItForTheRealmItHas(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	realm := &realmOf{}
	fixture.service.UseRealm(realm.get)
	id := fixture.createUser(t, "Karl", "a password", RoleUser)
	if key := karlsKey(t, fixture.service, "karlnas"); key != nil {
		t.Fatal("a key was made before the core had a realm")
	}
	realm.set("karlnas")
	if _, err := fixture.service.SignIn(ctx, "karl", "a password", "192.0.2.1", laptop); err != nil {
		t.Fatal(err)
	}
	if key := karlsKey(t, fixture.service, "karlnas"); !bytes.Equal(key,
		proof.RealmKey("karlnas", "karl", "a password")) {
		t.Errorf("after signing in under the realm karlnas the key is %x; want the password's", key)
	}
	offerKey, err := fixture.service.OfferKey(ctx, proof.RealmPerson("karlnas", "karl"))
	if err != nil || !bytes.Equal(offerKey, proof.RealmKey("karlnas", "karl", "a password")) {
		t.Errorf("an offer under karl's person is proven under %x, %v; want his realm key", offerKey, err)
	}
	if err = fixture.service.ChangePassword(ctx, id, "a password", "another password", ""); err != nil {
		t.Fatal(err)
	}
	if key := karlsKey(t, fixture.service, "karlnas"); !bytes.Equal(key,
		proof.RealmKey("karlnas", "karl", "another password")) {
		t.Error("a new password did not make a new key")
	}
	// A subrealm given later needs no password: the key stands, and the person is found by the new realm.
	realm.set("karlnas.live")
	if key := karlsKey(t, fixture.service, "karlnas.live"); !bytes.Equal(key,
		proof.RealmKey("karlnas", "karl", "another password")) {
		t.Error("after a subrealm was given, karl is not found by it with his key")
	}
	if key := karlsKey(t, fixture.service, "karlnas"); key != nil {
		t.Error("the realm the core no longer has still finds karl")
	}
	realm.set("another")
	if _, err = fixture.service.SignIn(ctx, "Karl", "another password", "192.0.2.1", laptop); err != nil {
		t.Fatal(err)
	}
	if key := karlsKey(t, fixture.service, "another"); !bytes.Equal(key,
		proof.RealmKey("another", "karl", "another password")) {
		t.Error("signing in under another master realm made no key for it")
	}
}

// A realm sign-in the discovery passes on is taken only when its proof holds under the person's key,
// and a guesser asking through the discovery is held to the person's limit (D297 §2; A13).
func TestTakeRealmSignIn_TakesARightProofAndHoldsAGuesserToThePersonsLimit(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.service.UseRealm(func() string { return "karlnas" })
	fixture.createUser(t, "karl", "a password", RoleUser)
	person := proof.RealmPerson("karlnas", "karl")
	right := proof.RealmAsk(proof.RealmKey("karlnas", "karl", "a password"), "karlnas", person)
	key, err := fixture.service.TakeRealmSignIn(ctx, person, right)
	if err != nil || !bytes.Equal(key, proof.RealmKey("karlnas", "karl", "a password")) {
		t.Fatalf("karl's right sign-in = %x, %v; want his key", key, err)
	}
	wrong := proof.RealmAsk(proof.RealmKey("karlnas", "karl", "a guess"), "karlnas", person)
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

// Every way a password is set makes its person's realm key, with no sign-in after: the first admin's
// at setup, an invitation's person's, and the admin an operator's reset names (D297 §3).
func TestRealmKey_IsMadeWheneverAPasswordIsSet(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	ctx := t.Context()
	fixture.service.UseRealm(func() string { return "karlnas" })
	codes := &heldCode{}
	code, err := fixture.service.EnsureSetupCode(ctx, codes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.CompleteSetup(ctx, codes, code, NewUser{Name: "owner", Password: "first"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyOf(t, fixture.service, "karlnas", "owner"), proof.RealmKey("karlnas", "owner", "first")) {
		t.Error("the first admin's setup made no key")
	}
	_, link, err := fixture.service.Invite(ctx, NewInvitation{Role: RoleUser, AdminNote: "Grandma"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Register(ctx, link.Secret, "Gran", "gran's", laptop); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyOf(t, fixture.service, "karlnas", "gran"), proof.RealmKey("karlnas", "gran", "gran's")) {
		t.Error("an invitation's person made no key")
	}
	if _, err = fixture.service.ResetAdmin(ctx, "owner", "reset"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyOf(t, fixture.service, "karlnas", "owner"), proof.RealmKey("karlnas", "owner", "reset")) {
		t.Error("the operator's reset made no key from the new password")
	}
}
