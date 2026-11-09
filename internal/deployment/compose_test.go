// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// composeService is what one service of a compose file says of its network, the network_mode it
// names, if any, and whether it publishes ports; and of its filesystem, whether its root is
// read-only and the tmpfs mounts it names ("/tmp:rw,noexec,...").
type composeService struct {
	networkMode  string
	isPublishing bool
	isReadOnly   bool
	tmpfs        []string
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
	service.isPublishing = service.isPublishing || trimmed == "ports:" || strings.HasPrefix(trimmed, "networks:")
}

// readUnder reads a line under one of a service's keys, its line trimmed: a tmpfs.
func (service *composeService) readUnder(key, trimmed string) {
	if key == "tmpfs:" && strings.HasPrefix(trimmed, "- ") {
		service.tmpfs = append(service.tmpfs, strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`))
	}
}

// householdComposeFiles are the compose files a household runs, each by the path the test reads it
// at: the one pasted into a NAS's panel, the catalogues' and the install script's with no tunnel.
var householdComposeFiles = []string{
	"../../deploy/omv/compose.yaml", "../../deploy/catalogue/casaos/docker-compose.yml",
	"templates/compose-direct.yaml",
}

// householdCompose is a household's compose file by its path in householdComposeFiles.
func householdCompose(t *testing.T, name string) string {
	t.Helper()
	if path.Dir(name) == "templates" {
		text, err := templates.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(text)
	}
	text, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

// A household's kernel is not ours: a compose file naming a kernel setting or feature the kernel has
// not is refused by Docker, and the server does not start (D454 §2). No household compose file names
// one, and the tunnel's, which the owner's deployments run, names no MPTCP (D454 §4).
func TestCompose_AHouseholdsFileAsksTheKernelForNothing(t *testing.T) {
	t.Parallel()
	for _, name := range householdComposeFiles {
		text := householdCompose(t, name)
		for line := range strings.SplitSeq(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			for _, refused := range []string{"net.mptcp", "ip_unprivileged_port_start", "enable_ipv6", "sysctls:"} {
				if strings.Contains(trimmed, refused) {
					t.Errorf("%s names %s: %q", name, refused, trimmed)
				}
			}
		}
	}
	tunnel, err := templates.ReadFile("templates/compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(tunnel), "net.mptcp") {
		t.Error("compose.yaml names net.mptcp")
	}
}

// With no tunnel the core shares this machine's network, publishing nothing, so it sees the house's
// addresses and takes its ports itself; the node keeps no network (D454 §5).
func TestCompose_AHouseholdsCoreSharesThisMachinesNetwork(t *testing.T) {
	t.Parallel()
	for _, name := range householdComposeFiles {
		services := composeServices(householdCompose(t, name))
		core, node := services["orpheum-core"], services["orpheum-node"]
		if core == nil || node == nil {
			t.Fatalf("%s: services %v; want orpheum-core and orpheum-node", name, services)
		}
		if core.networkMode != "host" || core.isPublishing {
			t.Errorf("%s: the core's network_mode %q, publishing ports or naming networks %v; want host, neither",
				name, core.networkMode, core.isPublishing)
		}
		if node.networkMode != "none" {
			t.Errorf("%s: the node's network_mode %q; want none", name, node.networkMode)
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
	if len(names) < 2 {
		t.Fatalf("compose templates %v; want compose.yaml and compose-direct.yaml at least", names)
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
