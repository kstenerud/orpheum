// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/deployment"
)

// deployedConfig is a core's config as a compose file's start writes it: HTTPS on 8443, WebRTC on
// 8443, the router named.
func deployedConfig() Config {
	return Config{
		Server:    ServerConfig{InstallRoute: "omv", Listen: ":8080"},
		HTTPS:     HTTPSConfig{Listen: ":8443"},
		WebRTC:    WebRTCConfig{UDPPort: 8443},
		Discovery: DiscoveryConfig{Router: "192.168.1.1"},
	}
}

// The System page's settings are kept in the core's network.toml once each is good and each port
// changed binds; a port the server holds now is not tried, and a refusal names its setting and keeps
// nothing (D462 §2, §10.11).
func TestNetworkSettings_KeepsGoodSettingsOnceEachChangedPortBinds(t *testing.T) {
	t.Parallel()
	persistent := t.TempDir()
	var tried []Ports
	held := 9000
	binds := func(ports Ports) error {
		tried = append(tried, ports)
		if ports.HTTPS == held || ports.WebRTC == held {
			return fmt.Errorf("the port %d %w", held, errWontBind)
		}
		return nil
	}
	settings, err := newNetworkSettings(deployedConfig(), persistent, binds)
	if err != nil {
		t.Fatal(err)
	}
	running := api.Network{HTTPSPort: 8443, WebRTCPort: 8443, Router: "192.168.1.1"}
	if state, stateErr := settings.State(); stateErr != nil ||
		state != (api.NetworkState{Kept: running, KeptAtStart: running, Running: running}) {
		t.Errorf("with none kept, the state is %+v (%v); want the config's throughout", state, stateErr)
	}
	if err = settings.Set(t.Context(), api.Network{HTTPSPort: 8443, WebRTCPort: 9443}); err != nil {
		t.Fatal(err)
	}
	if want := []Ports{{WebRTC: 9443}}; !reflect.DeepEqual(tried, want) {
		t.Errorf("tried %v; want only the WebRTC port changed, %v", tried, want)
	}
	kept, err := deployment.ReadNetwork(filepath.Join(persistent, deployment.NetworkFile))
	if want := (deployment.Network{HTTPSPort: 8443, WebRTCPort: 9443}); err != nil || kept != want {
		t.Errorf("network.toml keeps %+v (%v); want %+v", kept, err, want)
	}
	wantRefused(t, settings.Set(t.Context(), api.Network{HTTPSPort: held, WebRTCPort: 9443}),
		api.SettingHTTPSPort)
	wantRefused(t, settings.Set(t.Context(), api.Network{HTTPSPort: 8080, WebRTCPort: 9443, Router: "8.8.8.8"}),
		api.SettingHTTPSPort, api.SettingRouter)
	state, err := settings.State()
	if err != nil || state.Kept != (api.Network{HTTPSPort: 8443, WebRTCPort: 9443}) || state.KeptAtStart != running {
		t.Errorf("after the refusals, the state is %+v (%v); want the first set kept, the start's unchanged",
			state, err)
	}
}

// wantRefused fails t unless err refuses network settings naming settings, in order.
func wantRefused(t *testing.T, err error, settings ...string) {
	t.Helper()
	named := []string{}
	if problems, isRefusal := errors.AsType[api.NetworkErrors](err); isRefusal {
		for _, problem := range problems {
			named = append(named, problem.Setting)
		}
	}
	if !reflect.DeepEqual(named, settings) {
		t.Errorf("refused %v, naming %v; want %v", err, named, settings)
	}
}

// A core no deployment started keeps no network settings: its config file alone sets them.
func TestNetworkSettings_ACoreNoDeploymentStartedKeepsNone(t *testing.T) {
	t.Parallel()
	config := deployedConfig()
	config.Server.InstallRoute = ""
	settings, err := newNetworkSettings(config, t.TempDir(), portBinds)
	if err != nil || settings != nil {
		t.Errorf("with no install route = %v, %v; want none", settings, err)
	}
}
