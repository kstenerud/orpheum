// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// keptNetwork is a server's network settings, kept in memory, refusing a router that is "wrong".
type keptNetwork struct {
	state api.NetworkState
}

func (kept *keptNetwork) State() (api.NetworkState, error) { return kept.state, nil }

func (kept *keptNetwork) Set(_ context.Context, network api.Network) error {
	if network.Router == "wrong" {
		return api.NetworkErrors{{Setting: api.SettingRouter, Message: `lan.router "wrong": want a router`}}
	}
	kept.state.Kept = network
	return nil
}

// An admin reads the network settings kept and the ones the server runs with, keeps new ones,
// which wait for the next start, and is refused one that is wrong, naming the setting (D462 §2).
func TestNetwork_AnAdminKeepsTheSettingsForTheNextStartAndIsToldWhatIsWrong(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	dependencies, err := harness.server.ReadyDependencies()
	if err != nil {
		t.Fatal(err)
	}
	started := api.Network{HTTPSPort: 8443, WebRTCPort: 8443, Router: "192.168.1.1"}
	kept := &keptNetwork{state: api.NetworkState{Kept: started, KeptAtStart: started, Running: started}}
	dependencies.Network, dependencies.InstallRoute = kept, "omv"
	read := func(got response) api.NetworkStateBody {
		t.Helper()
		var body api.NetworkStateBody
		if err := json.Unmarshal([]byte(got.body), &body); err != nil || got.status != http.StatusOK {
			t.Fatalf("= %d %s; want 200", got.status, got.body)
		}
		return body
	}
	running := api.NetworkBody{HTTPSPort: 8443, WebRTCPort: 8443, Router: "192.168.1.1"}
	if told, want := read(harness.do(t, http.MethodGet, "/system/network", admin, nil)), (api.NetworkStateBody{
		Kept: running, Running: running, FellBack: []string{}, AppliedBy: "restart",
	}); !reflect.DeepEqual(told, want) {
		t.Errorf("GET /system/network told %+v; want %+v", told, want)
	}
	asked := map[string]any{"httpsPort": 9443, "webrtcPort": 9443, "router": ""}
	set := read(harness.do(t, http.MethodPut, "/system/network", admin, asked))
	if want := (api.NetworkBody{HTTPSPort: 9443, WebRTCPort: 9443}); set.Kept != want || set.Running != running ||
		!set.Pending {
		t.Errorf("after a PUT: %+v; want %+v kept, pending, the server still on %+v", set, want, running)
	}
	refused := harness.do(t, http.MethodPut, "/system/network", admin,
		map[string]any{"httpsPort": 9443, "webrtcPort": 9443, "router": "wrong"})
	if refused.status != http.StatusBadRequest || !strings.Contains(refused.body, `"location":"body.router"`) {
		t.Errorf("a wrong router = %d %s; want 400 naming body.router", refused.status, refused.body)
	}
	if kept.state.Kept.Router != "" {
		t.Errorf("a refused PUT kept %+v", kept.state.Kept)
	}
	dependencies.InstallRoute = "script"
	if script := read(harness.do(t, http.MethodGet, "/system/network", admin, nil)); script.AppliedBy != "script" {
		t.Errorf("on the install script's route, applied by %q; want script", script.AppliedBy)
	}
}

// A port kept that the server could not take at its start is said to have fallen back, as the server
// runs on the default (D462 §10.11).
func TestNetwork_APortThatWouldNotBindAtTheStartIsSaidToHaveFallenBack(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	dependencies, err := harness.server.ReadyDependencies()
	if err != nil {
		t.Fatal(err)
	}
	kept := api.Network{HTTPSPort: 443, WebRTCPort: 9000}
	dependencies.Network = &keptNetwork{state: api.NetworkState{
		Kept: kept, KeptAtStart: kept, Running: api.Network{HTTPSPort: 8443, WebRTCPort: 9000},
	}}
	got := harness.do(t, http.MethodGet, "/system/network", admin, nil)
	var body api.NetworkStateBody
	if err := json.Unmarshal([]byte(got.body), &body); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body.FellBack, []string{"httpsPort"}) || body.Pending {
		t.Errorf("with the HTTPS port fallen back: %+v; want httpsPort fallen back, nothing pending", body)
	}
}

// A restart is the admin's alone, on a compose route; with no network settings kept the page is
// told they are the config file's (D462 §2).
func TestNetwork_AnAdminRestartsAServerItsComposeFileStarted(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	dependencies, err := harness.server.ReadyDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if got := harness.do(t, http.MethodGet, "/system/network", admin, nil); got.status != http.StatusConflict {
		t.Errorf("with no network settings, GET = %d %s; want 409", got.status, got.body)
	}
	restarts := 0
	dependencies.Restart, dependencies.InstallRoute = func() { restarts++ }, "synology"
	if got := harness.do(t, http.MethodPost, "/system/restart", admin, nil); got.status != http.StatusAccepted ||
		restarts != 1 {
		t.Errorf("POST /system/restart = %d %s, %d restarts; want 202 and one", got.status, got.body, restarts)
	}
	dependencies.InstallRoute = "script"
	if got := harness.do(t, http.MethodPost, "/system/restart", admin, nil); got.status != http.StatusConflict ||
		restarts != 1 {
		t.Errorf("on the install script's route = %d, %d restarts; want 409 and none more", got.status, restarts)
	}
	if got := harness.do(t, http.MethodPost, "/system/restart", "", nil); got.status != http.StatusUnauthorized ||
		restarts != 1 {
		t.Errorf("signed out = %d, %d restarts; want 401 and none more", got.status, restarts)
	}
}
