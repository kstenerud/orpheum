// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/deployment"
)

// networkSettings are the house's network settings the System page keeps (D462 §2), in the core's
// network.toml, which every start, or the install script's every run, writes into the config. The
// ones the server runs with are the config's.
type networkSettings struct {
	path        string
	running     deployment.Network
	keptAtStart deployment.Network
	// httpPort is the plain HTTP port, which the HTTPS port may not share.
	httpPort int
	// binds is whether this machine lets the core take ports it does not hold (portBinds).
	binds func(Ports) error
	// mutex keeps one Set at a time, so two never interleave their checks and writes.
	mutex sync.Mutex
}

// newNetworkSettings are the network settings kept in the persistent directory persistent for a core
// running with config; nil when it was not deployed (no install route), as its config file alone
// then sets them.
func newNetworkSettings(config Config, persistent string, binds func(Ports) error) (*networkSettings, error) {
	if config.Server.InstallRoute == "" {
		return nil, nil //nolint:nilnil // none, and nothing wrong
	}
	settings := &networkSettings{
		path: filepath.Join(persistent, deployment.NetworkFile), httpPort: portOf(config.Server.Listen),
		binds: binds,
		running: deployment.Network{
			HTTPSPort: httpsPort(config), WebRTCPort: config.WebRTC.UDPPort, Router: config.Discovery.Router,
		},
	}
	kept, err := settings.kept()
	if err != nil {
		return nil, err
	}
	settings.keptAtStart = kept
	return settings, nil
}

// kept are the settings kept; the ones the server runs with while none are.
func (settings *networkSettings) kept() (deployment.Network, error) {
	network, err := deployment.ReadNetwork(settings.path)
	if errors.Is(err, deployment.ErrNoNetwork) {
		return settings.running, nil
	}
	return network, err
}

// State is the settings kept, kept at the start, and run with.
func (settings *networkSettings) State() (api.NetworkState, error) {
	settings.mutex.Lock()
	defer settings.mutex.Unlock()
	kept, err := settings.kept()
	if err != nil {
		return api.NetworkState{}, err
	}
	return api.NetworkState{
		Kept: apiNetwork(kept), KeptAtStart: apiNetwork(settings.keptAtStart), Running: apiNetwork(settings.running),
	}, nil
}

// Set keeps asked once each setting is good and each port that is not one the server holds now
// binds (D462 §10.11), refusing it with api.NetworkErrors otherwise.
func (settings *networkSettings) Set(_ context.Context, asked api.Network) error {
	network := deployment.Network(asked)
	settings.mutex.Lock()
	defer settings.mutex.Unlock()
	problems := networkProblems(network.Problems(settings.httpPort))
	if len(problems) == 0 {
		problems = settings.bindProblems(network)
	}
	if len(problems) != 0 {
		return problems
	}
	return deployment.WriteNetwork(settings.path, network)
}

// bindProblems are network's ports that will not bind, other than the ones the server holds now.
func (settings *networkSettings) bindProblems(network deployment.Network) api.NetworkErrors {
	var problems api.NetworkErrors
	if network.HTTPSPort != settings.running.HTTPSPort {
		if err := settings.binds(Ports{HTTPS: network.HTTPSPort}); err != nil {
			problems = append(problems, api.NetworkProblem{Setting: api.SettingHTTPSPort, Message: err.Error()})
		}
	}
	if network.WebRTCPort != settings.running.WebRTCPort {
		if err := settings.binds(Ports{WebRTC: network.WebRTCPort}); err != nil {
			problems = append(problems, api.NetworkProblem{Setting: api.SettingWebRTCPort, Message: err.Error()})
		}
	}
	return problems
}

// networkProblems are deployment.Network.Problems' lines, each naming the System page's setting the
// values file's key it starts with is.
func networkProblems(lines []string) api.NetworkErrors {
	problems := make(api.NetworkErrors, 0, len(lines))
	for _, line := range lines {
		key, _, _ := strings.Cut(line, " ")
		setting := api.SettingHTTPSPort
		switch key {
		case "lan.webrtc_port":
			setting = api.SettingWebRTCPort
		case "lan.router":
			setting = api.SettingRouter
		}
		problems = append(problems, api.NetworkProblem{Setting: setting, Message: line})
	}
	return problems
}

// apiNetwork is network as the API takes it.
func apiNetwork(network deployment.Network) api.Network {
	return api.Network(network)
}

// restartBySignal stops this process as an interrupt from outside would: serve's shutdown finishes
// the requests in flight, the restart's answer among them, and the container's restart policy
// (`restart: unless-stopped`) starts it again (D462 §2).
func restartBySignal() {
	if process, err := os.FindProcess(os.Getpid()); err == nil {
		_ = process.Signal(syscall.SIGTERM) //nolint:errcheck // a signal to itself, which cannot be refused
	}
}
