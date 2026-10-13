// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

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
	if !isAdminAt(t, server.baseURL, "rescue", "rescue-password") ||
		isAdminAt(t, server.baseURL, "owner", ownerPassword) {
		t.Error("after the reset, want rescue alone the admin, and owner still able to sign in")
	}
}
