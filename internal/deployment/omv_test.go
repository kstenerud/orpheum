// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The one compose file a household pastes (D447), and the page that says what to fill into it. It
// is not a template written into a deployment's directory, so it lives beside the other deploy files.
const (
	omvComposeFile = "../../deploy/omv/compose.yaml"
	installPage    = "../../docs/users/install.md"
)

// composeVariable is a variable a compose file substitutes, its name and what follows it: ":?" when
// it is needed.
var composeVariable = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(:\?|:-|\})`)

// omvCompose is the compose file's lines, its comments left out.
func omvCompose(t *testing.T) string {
	t.Helper()
	text, err := os.ReadFile(omvComposeFile)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for line := range strings.SplitSeq(string(text), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// The pasted compose file keeps every service's MPTCP off and the node's root unwritable, as the
// compose files a deployment is written from do, and its core is the first start, given the address
// at home and the media where the node finds them.
func TestOMVCompose_KeepsTheDeploymentsGuardsAndStartsFromItsCommandLine(t *testing.T) {
	t.Parallel()
	text := omvCompose(t)
	services := composeServices(text)
	for _, name := range []string{"orpheum-core", "orpheum-node"} {
		if _, isThere := services[name]; !isThere {
			t.Fatalf("%s: no %s service; services %v", omvComposeFile, name, services)
		}
	}
	for name, said := range services {
		if !sharesANetwork(said.networkMode) && !said.isMPTCPOff {
			t.Errorf("%s: %s has a network of its own and does not set net.mptcp.enabled to \"0\"",
				omvComposeFile, name)
		}
	}
	if node := services["orpheum-node"]; !node.isReadOnly || !slices.ContainsFunc(node.tmpfs, isTmpThatRunsNothing) {
		t.Errorf("%s: the node's root is read-only %v, its tmpfs %v; want read-only, and /tmp running nothing",
			omvComposeFile, node.isReadOnly, node.tmpfs)
	}
	for _, want := range []string{
		"      - start\n      - --lan-address\n      - ${ORPHEUM_LAN_ADDRESS:?",
		`command: ["node", "--instance", "/var/lib/orpheum/node"]`,
		":/media:ro",
		"condition: service_healthy",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not say %q", omvComposeFile, want)
		}
	}
}

// Every variable the compose file reads is on the install page, so a household knows what each is,
// and the page names the ones it cannot start without.
func TestOMVCompose_TheInstallPageNamesEveryVariableItReads(t *testing.T) {
	t.Parallel()
	page, err := os.ReadFile(installPage)
	if err != nil {
		t.Fatal(err)
	}
	needed := map[string]bool{}
	for _, match := range composeVariable.FindAllStringSubmatch(omvCompose(t), -1) {
		needed[match[1]] = needed[match[1]] || match[2] == ":?"
	}
	for _, name := range []string{"PATH_TO_APPDATA", "ORPHEUM_MEDIA", "ORPHEUM_LAN_ADDRESS"} {
		if !needed[name] {
			t.Errorf("%s does not need %s (${%s:?...})", omvComposeFile, name, name)
		}
	}
	for name := range needed {
		if !strings.Contains(string(page), "`"+name+"`") {
			t.Errorf("%s reads %s, which %s does not name", omvComposeFile, name, installPage)
		}
	}
}
