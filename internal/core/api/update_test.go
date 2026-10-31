// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// knownRelease is a server's discovery's answer, as discovery.Service keeps it.
type knownRelease struct {
	known discovery.Release
}

func (release knownRelease) Release() discovery.Release {
	return release.known
}

// An admin is told the security fix the server's version lacks, the latest and the command that
// installs it from the release's own image; anyone else is refused, and a plain newer release has no
// fix (security-updates.md Part B).
func TestUpdate_AnAdminIsToldTheFixPendingAndTheCommandThatInstallsIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	dependencies, err := harness.server.ReadyDependencies()
	if err != nil {
		t.Fatal(err)
	}
	checked := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	release := &knownRelease{known: discovery.Release{
		Version: "v0.3.1", Latest: "v0.3.3", Digest: "sha256:abc", CheckedAt: checked,
		Security: &discoveryproto.SecurityFix{Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"},
	}}
	dependencies.Releases, dependencies.PublishedImage = release, "ghcr.io/example/orpheum"
	read := func() api.UpdateBody {
		t.Helper()
		got := harness.do(t, http.MethodGet, "/system/update", admin, nil)
		var body api.UpdateBody
		if err := json.Unmarshal([]byte(got.body), &body); err != nil || got.status != http.StatusOK {
			t.Fatalf("GET /system/update = %d %s", got.status, got.body)
		}
		return body
	}
	command := "docker run --rm ghcr.io/example/orpheum:v0.3.3 install-script | sh"
	if told, want := read(), (api.UpdateBody{
		Version: "v0.3.1", Latest: "v0.3.3", Digest: "sha256:abc", CheckedAt: checked.UnixMilli(),
		Security: &api.SecurityFixBody{Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"},
		Command:  command,
	}); !reflect.DeepEqual(told, want) {
		t.Errorf("told %+v; want %+v", told, want)
	}
	release.known.Security = nil
	if plain := read(); plain.Security != nil || plain.Latest != "v0.3.3" || plain.Command != command {
		t.Errorf("a plain newer release: told %+v; want the latest and its command, no fix", plain)
	}
	release.known = discovery.Release{Version: "v0.3.3", CheckedAt: checked}
	if current := read(); current.Latest != "" || current.Command != "" {
		t.Errorf("the latest running: told %+v; want no update", current)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodGet, "/system/update", harness.signIn(t, "partner", "partner-password"), nil),
		http.StatusForbidden, api.CodeForbidden)
}
