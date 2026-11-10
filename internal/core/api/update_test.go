// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// knownRelease is a server's discovery's answer, as discovery.Service keeps it.
type knownRelease struct {
	known discovery.Release
}

func (known knownRelease) Release() discovery.Release {
	return known.known
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
	releases := &knownRelease{known: discovery.Release{
		Version: "v0.3.1", Latest: "v0.3.3", Digest: "sha256:abc", CheckedAt: checked,
		Security: &release.Fix{Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"},
	}}
	dependencies.Releases, dependencies.PublishedImage = releases, "ghcr.io/example/orpheum"
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
	releases.known.Security = nil
	if plain := read(); plain.Security != nil || plain.Latest != "v0.3.3" || plain.Command != command {
		t.Errorf("a plain newer release: told %+v; want the latest and its command, no fix", plain)
	}
	// The signed record names where the release may be pulled, which the command then pulls from by
	// the release's own digest, not the image this build was published as (D391 §6).
	releases.known.Location, releases.known.Digest = "mirror.example/orpheum", "sha256:"+strings.Repeat("c", 64)
	pinned := "docker run --rm mirror.example/orpheum@sha256:" + strings.Repeat("c", 64) + " install-script | sh"
	if mirrored := read(); mirrored.Command != pinned {
		t.Errorf("a release the record names a location for: command %q; want %q", mirrored.Command, pinned)
	}
	releases.known = discovery.Release{Version: "v0.3.3", CheckedAt: checked}
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

// The answer says how the server was installed, as its config says, so the page shows only that
// way's steps to update; a server that does not know says nothing, and the page shows both (D450 §2).
func TestUpdate_SaysHowTheServerWasInstalled(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	dependencies, err := harness.server.ReadyDependencies()
	if err != nil {
		t.Fatal(err)
	}
	read := func() api.UpdateBody {
		t.Helper()
		got := harness.do(t, http.MethodGet, "/system/update", admin, nil)
		var body api.UpdateBody
		if err := json.Unmarshal([]byte(got.body), &body); err != nil || got.status != http.StatusOK {
			t.Fatalf("GET /system/update = %d %s", got.status, got.body)
		}
		return body
	}
	if unknown := read(); unknown.InstallRoute != "" {
		t.Errorf("a server that does not know its route: installRoute %q; want none", unknown.InstallRoute)
	}
	dependencies.InstallRoute = "omv"
	if told := read(); told.InstallRoute != "omv" {
		t.Errorf("knowing no release: installRoute %q; want omv", told.InstallRoute)
	}
	dependencies.Releases = &knownRelease{known: discovery.Release{Version: "v0.3.1", Latest: "v0.3.3"}}
	if told := read(); told.InstallRoute != "omv" {
		t.Errorf("knowing a newer release: installRoute %q; want omv", told.InstallRoute)
	}
}

// A server a compose file started is told of a newer compose file and what each version after its
// own brings, oldest first; one started from the newest, or by no compose file, is told of none
// (D461 §3).
func TestUpdate_SaysWhatANewerComposeFileBrings(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	dependencies, err := harness.server.ReadyDependencies()
	if err != nil {
		t.Fatal(err)
	}
	read := func() api.UpdateBody {
		t.Helper()
		got := harness.do(t, http.MethodGet, "/system/update", admin, nil)
		var body api.UpdateBody
		if err := json.Unmarshal([]byte(got.body), &body); err != nil || got.status != http.StatusOK {
			t.Fatalf("GET /system/update = %d %s", got.status, got.body)
		}
		return body
	}
	changes := map[int]string{2: "Hardware transcoding on Intel.", 3: "Subtitles from the media folders."}
	dependencies.InstallRoute = "omv"
	dependencies.ComposeFile = api.ComposeFile{Running: 1, Newest: 3, Changes: changes}
	want := &api.ComposeFileBody{
		Version: 1, Newest: 3, Brings: []string{"Hardware transcoding on Intel.", "Subtitles from the media folders."},
	}
	if told := read(); !reflect.DeepEqual(told.ComposeFile, want) {
		t.Errorf("started from version 1 of 3: composeFile %+v; want %+v", told.ComposeFile, want)
	}
	dependencies.ComposeFile.Running = 2
	want = &api.ComposeFileBody{Version: 2, Newest: 3, Brings: []string{"Subtitles from the media folders."}}
	if told := read(); !reflect.DeepEqual(told.ComposeFile, want) {
		t.Errorf("started from version 2 of 3: composeFile %+v; want %+v", told.ComposeFile, want)
	}
	for _, running := range []int{3, 0} {
		dependencies.ComposeFile.Running = running
		if told := read(); told.ComposeFile != nil {
			t.Errorf("started from compose file version %d of 3: composeFile %+v; want none", running, told.ComposeFile)
		}
	}
}
