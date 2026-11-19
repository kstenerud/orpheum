// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/identity"
)

// checkPersonKey fails the test unless the instance holds name's away-from-home key made from
// password and the core's key salt, as the server makes it (D527 §2.2). Check it before name signs
// in: a sign-in makes the key where there is none.
func checkPersonKey(t *testing.T, config Config, name, password string) {
	t.Helper()
	directories, database, err := openExistingDatabase(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close() //nolint:errcheck // read only
	keySalt, err := identity.LoadOrMakeKeySalt(directories, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := database.Read().ListLivePersonKeys(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := proof.PersonKey(keySalt, name, password)
	for _, each := range keys {
		if each.Name == name {
			if !bytes.Equal(each.Key, want) {
				t.Errorf("%s's away-from-home key is not the one their password and the core's salt make", name)
			}
			return
		}
	}
	t.Errorf("%s has no away-from-home key; want one made from their password", name)
}

// isAdminAt is whether name, signing in with password at the running server, administers.
func isAdminAt(t *testing.T, baseURL, name, password string) bool {
	t.Helper()
	me := requestAPI(t, http.MethodGet, baseURL+"/me", signInAs(t, baseURL, name, password), nil)
	var body struct {
		IsAdmin bool `json:"isAdmin"`
	}
	if err := json.Unmarshal([]byte(me.body), &body); err != nil {
		t.Fatalf("GET /me: %d %s", me.status, me.body)
	}
	return body.IsAdmin
}

// The operator, on the server's machine, makes one person the only admin while the server runs:
// the password from standard input, everyone who administered demoted, no account removed (D294).
func TestResetAdmin_FromTheCommandLineMakesOnePersonTheOnlyAdmin(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	t.Cleanup(func() { _ = server.stop() }) //nolint:errcheck // the test is over
	setUpOwner(t, server, path, "owner")
	reset := func(stdin string, arguments ...string) commandOutput {
		t.Helper()
		var stdout, stderr bytes.Buffer
		status := dispatch(t.Context(), append([]string{"reset-admin", "--instance", path}, arguments...),
			noEnvironment, strings.NewReader(stdin), &stdout, &stderr)
		return commandOutput{status: status, stdout: stdout.String(), stderr: stderr.String()}
	}
	if refused := reset("", "--name", "rescue"); refused.status == exitSuccess {
		t.Errorf("a reset with no password = %+v; want it refused", refused)
	}
	if refused := reset("pw\n"); refused.status == exitSuccess {
		t.Errorf("a reset naming no one = %+v; want it refused", refused)
	}
	done := reset("rescue-password\n", "--name", "rescue")
	if done.status != exitSuccess || !strings.Contains(done.stdout, "rescue is now the only admin") ||
		!strings.Contains(done.stdout, "no longer admins: owner") {
		t.Fatalf("reset-admin = %+v; want rescue made the admin and owner demoted", done)
	}
	checkPersonKey(t, config, "rescue", "rescue-password")
	if !isAdminAt(t, server.baseURL, "rescue", "rescue-password") ||
		isAdminAt(t, server.baseURL, "owner", ownerPassword) {
		t.Error("after the reset, want rescue alone the admin, and owner still able to sign in")
	}
}
