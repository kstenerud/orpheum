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

// The pasted compose file keeps the node's root unwritable, as the compose files a deployment is
// written from do, and its core is the first start, given the address at home, which may be empty
// (D452 §5), and the media where the node finds them.
func TestOMVCompose_KeepsTheDeploymentsGuardsAndStartsFromItsCommandLine(t *testing.T) {
	t.Parallel()
	text := omvCompose(t)
	services := composeServices(text)
	for _, name := range []string{"orpheum-core", "orpheum-node"} {
		if _, isThere := services[name]; !isThere {
			t.Fatalf("%s: no %s service; services %v", omvComposeFile, name, services)
		}
	}
	if node := services["orpheum-node"]; !node.isReadOnly || !slices.ContainsFunc(node.tmpfs, isTmpThatRunsNothing) {
		t.Errorf("%s: the node's root is read-only %v, its tmpfs %v; want read-only, and /tmp running nothing",
			omvComposeFile, node.isReadOnly, node.tmpfs)
	}
	for _, want := range []string{
		"      - --lan-address\n      - ${ORPHEUM_LAN_ADDRESS:-}\n",
		"      - --http-port\n      - ${ORPHEUM_HTTP_PORT:-8440}\n",
		`command: ["node", "--instance", "/var/lib/orpheum/node"]`,
		"        source: ${ORPHEUM_MEDIA:?set ORPHEUM_MEDIA to the folder that holds your films and series}\n" +
			"        target: ${ORPHEUM_MEDIA}\n        read_only: true\n",
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
	for _, name := range []string{"PATH_TO_APPDATA", "ORPHEUM_MEDIA"} {
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

// Each media folder is mounted at its own path, so the library picker shows the name this machine
// gives it, and passed to start; one of the optional folders not set mounts the empty placeholder,
// so compose still has a bind's source, and comes to start empty, so it is no folder (D449 §4).
func TestOMVCompose_EachMediaFolderIsMountedAtItsOwnPathAndAnUnsetOneIsNone(t *testing.T) {
	t.Parallel()
	text := omvCompose(t)
	if !strings.Contains(text, "      - --media\n      - ${ORPHEUM_MEDIA:?") {
		t.Errorf("%s does not pass ORPHEUM_MEDIA to start as a --media", omvComposeFile)
	}
	if !strings.Contains(text, "mkdir -p /appdata/node /appdata/run /appdata/empty ") {
		t.Errorf("%s: orpheum-folders does not make empty/, the unset folders' source", omvComposeFile)
	}
	for _, name := range []string{"ORPHEUM_MEDIA_2", "ORPHEUM_MEDIA_3", "ORPHEUM_MEDIA_4"} {
		suffix := name[len(name)-1:]
		for _, want := range []string{
			"      - --media\n      - ${" + name + ":-}\n",
			"      - ${" + name + ":-${PATH_TO_APPDATA}/orpheum/empty}:${" + name + ":-/unset/" + suffix + "}:ro\n",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q: %s mounted at its own path, or the empty folder where nothing "+
					"looks, and passed to start empty when not set", omvComposeFile, want, name)
			}
		}
	}
}

// The server's own machine reaches the core on a port on its loopback alone, which start is told and
// the core takes there itself, sharing this machine's network (D454 §5), so it counts a request on it
// as from home (D449 §2, D334 §4); the health check asks it there.
func TestOMVCompose_ThisMachinesPortIsOnTheLoopbackAndToldToStart(t *testing.T) {
	t.Parallel()
	text := omvCompose(t)
	for _, want := range []string{
		"exec 3<>/dev/tcp/127.0.0.1/${ORPHEUM_THIS_MACHINE_PORT:-8441} && ",
		"      - --this-machine-port\n      - ${ORPHEUM_THIS_MACHINE_PORT:-8441}\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not say %q", omvComposeFile, want)
		}
	}
	if strings.Count(text, "ORPHEUM_THIS_MACHINE_PORT") != 2+strings.Count(text, "#   ORPHEUM_THIS_MACHINE_PORT") {
		t.Errorf("%s names ORPHEUM_THIS_MACHINE_PORT %d times; want only start's flag and the health check",
			omvComposeFile, strings.Count(text, "ORPHEUM_THIS_MACHINE_PORT"))
	}
}
