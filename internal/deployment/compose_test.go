// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// composeService is what one service of a compose file says of its network, the network_mode it
// names, if any, and whether its sysctls turn MPTCP off; and of its filesystem, whether its root is
// read-only and the tmpfs mounts it names ("/tmp:rw,noexec,...").
type composeService struct {
	networkMode string
	isMPTCPOff  bool
	isReadOnly  bool
	tmpfs       []string
}

var composeServiceName = regexp.MustCompile(`^([a-z0-9_-]+):$`)

// composeServices reads the services of a compose template, which keeps compose's usual two-space
// indentation: a service's name at two spaces, its keys at four and a sysctl or a tmpfs at six.
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
			current.readKey(trimmed)
		default:
			current.readUnder(key, trimmed)
		}
	}
	return services
}

// readKey reads one of a service's keys, its line trimmed.
func (service *composeService) readKey(trimmed string) {
	if mode, isMode := strings.CutPrefix(trimmed, "network_mode:"); isMode {
		service.networkMode = strings.Trim(strings.TrimSpace(mode), `"'`)
	}
	service.isReadOnly = service.isReadOnly || trimmed == "read_only: true"
}

// readUnder reads a line under one of a service's keys, its line trimmed: a sysctl or a tmpfs.
func (service *composeService) readUnder(key, trimmed string) {
	switch {
	case key == "sysctls:" && trimmed == `net.mptcp.enabled: "0"`:
		service.isMPTCPOff = true
	case key == "tmpfs:" && strings.HasPrefix(trimmed, "- "):
		service.tmpfs = append(service.tmpfs, strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`))
	}
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

// A kernel with no MPTCP has no net.mptcp.enabled, and Docker refuses to start a container naming a
// setting its kernel has not; such a kernel has no MPTCP listener for the CVE to reach. Its compose
// file sets none, and leaves no sysctls key with nothing under it, while a service's other sysctls
// stay (the agent's report, 2026-10-31).
func TestCompose_AKernelWithNoMPTCPGetsAComposeFileThatDoesNotNameIt(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"templates/compose.yaml", "templates/compose-direct.yaml"} {
		text, err := templates.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		without := string(withoutMultipathTCP(text))
		if strings.Contains(without, "net.mptcp.enabled") {
			t.Errorf("%s: still names net.mptcp.enabled for a kernel without it", path.Base(name))
		}
		lines := strings.Split(without, "\n")
		for index, line := range lines {
			if strings.TrimSpace(line) == "sysctls:" &&
				(index+1 >= len(lines) || indent(lines[index+1]) <= indent(line)) {
				t.Errorf("%s: a sysctls key with nothing under it, at line %d", path.Base(name), index+1)
			}
		}
		kept := strings.Join(lines, "\n")
		for line := range strings.SplitSeq(string(text), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != multipathTCPSysctl && trimmed != "sysctls:" && !strings.Contains(kept, line) {
				t.Errorf("%s: removed %q, which is not MPTCP's", path.Base(name), line)
			}
		}
	}
	text, err := templates.ReadFile("templates/compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withoutMultipathTCP(text)), "net.ipv4.ip_unprivileged_port_start") {
		t.Error("compose.yaml: the tunnel's other sysctl went with MPTCP's")
	}
}

// Write asks for MPTCP off on a kernel that has it, and names none on one that has not.
func TestWrite_TheComposeFileTurnsMPTCPOffWhereTheKernelHasIt(t *testing.T) {
	t.Parallel()
	for _, lacks := range []bool{false, true} {
		dir := begun(t, Release)
		kernel := runtime
		kernel.KernelLacksMultipathTCP = lacks
		if err := Write(dir, filledValues(t, Release), kernel); err != nil {
			t.Fatal(err)
		}
		if names := strings.Contains(readFile(t, filepath.Join(dir, composeFile)), multipathTCPSysctl); names == lacks {
			t.Errorf("a kernel lacking MPTCP %v: the compose file names it %v", lacks, names)
		}
	}
}

// The node's root is read-only and its /tmp a tmpfs nothing runs from, in every compose file a
// deployment is written from, so a file an exploited FFmpeg writes outside the node's volumes can be
// neither run nor loaded as a library (navigation-and-widgets-answers-2026-10-27.md, noexec step 1).
func TestCompose_TheNodesRootIsReadOnlyAndItsTmpRunsNothing(t *testing.T) {
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
		node, isThere := composeServices(string(text))["orpheum-node"]
		if !isThere {
			t.Errorf("%s: no orpheum-node service", path.Base(name))
			continue
		}
		if !node.isReadOnly {
			t.Errorf("%s: the node's root is not read_only", path.Base(name))
		}
		if !slices.ContainsFunc(node.tmpfs, isTmpThatRunsNothing) {
			t.Errorf("%s: the node's tmpfs %v; want /tmp with noexec, nosuid and nodev", path.Base(name), node.tmpfs)
		}
	}
}

// isTmpThatRunsNothing is whether a tmpfs entry mounts /tmp with noexec, nosuid and nodev.
func isTmpThatRunsNothing(entry string) bool {
	target, options, _ := strings.Cut(entry, ":")
	given := strings.Split(options, ",")
	return target == "/tmp" && slices.Contains(given, "noexec") && slices.Contains(given, "nosuid") &&
		slices.Contains(given, "nodev")
}
