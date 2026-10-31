// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// composeService is what one service of a compose file says of its network: the network_mode it
// names, if any, and whether its sysctls turn MPTCP off.
type composeService struct {
	networkMode string
	isMPTCPOff  bool
}

var composeServiceName = regexp.MustCompile(`^([a-z0-9_-]+):$`)

// composeServices reads the services of a compose template, which keeps compose's usual two-space
// indentation: a service's name at two spaces, its keys at four and a sysctl at six.
func composeServices(text string) map[string]*composeService {
	services := map[string]*composeService{}
	var current *composeService
	section, key := "", ""
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		switch indent := len(line) - len(strings.TrimLeft(line, " ")); {
		case indent == 0:
			section, current = trimmed, nil
		case section != "services:":
		case indent == 2:
			current, key = nil, ""
			if match := composeServiceName.FindStringSubmatch(trimmed); match != nil {
				current = &composeService{}
				services[match[1]] = current
			}
		case current == nil:
		case indent == 4:
			key = trimmed
			if mode, isMode := strings.CutPrefix(trimmed, "network_mode:"); isMode {
				current.networkMode = strings.Trim(strings.TrimSpace(mode), `"'`)
			}
		case key == "sysctls:" && trimmed == `net.mptcp.enabled: "0"`:
			current.isMPTCPOff = true
		}
	}
	return services
}

// sharesANetwork reports whether a network_mode puts a service in a network not its own: another
// service's or container's, or the host's. Docker refuses a net.* sysctl there.
func sharesANetwork(networkMode string) bool {
	return networkMode == "host" || strings.HasPrefix(networkMode, "service:") ||
		strings.HasPrefix(networkMode, "container:")
}

// CVE-2026-98432 is reached through MPTCP listeners, and net.mptcp.enabled is each network
// namespace's own, so the host's setting does not reach a container: each service with a network of
// its own (network_mode none included, which is a namespace of its own) turns it off, and one that
// shares another's network has that one's setting and may not set it (Docker refuses).
func TestCompose_EveryServiceWithANetworkOfItsOwnTurnsMPTCPOff(t *testing.T) {
	t.Parallel()
	names, err := fs.Glob(templates, "templates/compose*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 2 {
		t.Fatalf("compose templates %v; want compose.yaml and compose-direct.yaml at least", names)
	}
	for _, name := range names {
		text, readErr := templates.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		services := composeServices(string(text))
		if len(services) == 0 {
			t.Errorf("%s: no services read", path.Base(name))
		}
		for service, said := range services {
			switch {
			case sharesANetwork(said.networkMode) && said.isMPTCPOff:
				t.Errorf("%s: %s shares the network %q and sets net.mptcp.enabled, which Docker refuses",
					path.Base(name), service, said.networkMode)
			case !sharesANetwork(said.networkMode) && !said.isMPTCPOff:
				t.Errorf("%s: %s has a network of its own and does not set net.mptcp.enabled to \"0\"",
					path.Base(name), service)
			}
		}
	}
}

// A service that shares another service's network runs under that one's setting, as the core in
// compose.yaml runs under the tunnel's, so the service it shares with must turn MPTCP off.
func TestCompose_AServiceSharingANetworkSharesOneWithMPTCPOff(t *testing.T) {
	t.Parallel()
	names, err := fs.Glob(templates, "templates/compose*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		text, readErr := templates.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		services := composeServices(string(text))
		for service, said := range services {
			shared, isService := strings.CutPrefix(said.networkMode, "service:")
			if !isService {
				continue
			}
			if owner, ok := services[shared]; !ok || !owner.isMPTCPOff {
				t.Errorf("%s: %s shares %s's network, which does not turn MPTCP off", path.Base(name), service, shared)
			}
		}
	}
}
