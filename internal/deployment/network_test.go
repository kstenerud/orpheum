// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
)

// Every deployment's network had fd77:5e::/64, so a second one on a machine, a test kit's beside
// production, could not create its own (the production NAS, 2026-10-23).
func TestWrite_EachDeploymentsNetworkHasASubnetOfItsOwn(t *testing.T) {
	t.Parallel()
	subnetOf := func(directory string) string {
		t.Helper()
		dir := begun(t, Release)
		withDirectory := runtime
		withDirectory.Directory = directory
		if err := Write(dir, filledValues(t, Release), withDirectory); err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(readFile(t, filepath.Join(dir, environmentFile)), "\n") {
			if subnet, isIt := strings.CutPrefix(line, "ORPHEUM_OUTSIDE_IPV6_SUBNET="); isIt {
				return subnet
			}
		}
		t.Fatal(".env names no subnet")
		return ""
	}
	production, test := subnetOf("/srv/orpheum"), subnetOf("/home/orpheum/hwtest")
	if production == test {
		t.Errorf("two deployments' networks share %s", production)
	}
	for _, subnet := range []string{production, test} {
		prefix, err := netip.ParsePrefix(subnet)
		if err != nil || prefix.Bits() != 64 || !netip.MustParsePrefix("fd77:5e::/48").Overlaps(prefix) {
			t.Errorf("subnet %s, want a /64 of fd77:5e::/48 (%v)", subnet, err)
		}
	}
	if again := subnetOf("/srv/orpheum"); again != production {
		t.Errorf("the same directory gave %s, then %s; want the same each time", production, again)
	}
	if unknown := subnetOf(""); unknown != "fd77:5e::/64" {
		t.Errorf("a deployment whose directory is not known has %s, want fd77:5e::/64", unknown)
	}
}

// A browser on the server's own machine reaches the core from Docker's gateway, never home, so admin
// was refused there (staging, 2026-10-23): the port for that machine alone is published on its
// loopback and nowhere else, the same inside, and the core serves it (D334 §4).
func TestWrite_ThisMachinesPortIsPublishedOnItsLoopbackAlone(t *testing.T) {
	t.Parallel()
	for _, kind := range []Kind{Release, Staging} {
		dir := begun(t, kind)
		values := filledValues(t, kind)
		values.HTTP.ThisMachinePort = 18441
		if err := Write(dir, values, runtime); err != nil {
			t.Fatal(err)
		}
		compose := readFile(t, filepath.Join(dir, composeFile))
		published := `"127.0.0.1:${ORPHEUM_THIS_MACHINE_PORT:-8441}:${ORPHEUM_THIS_MACHINE_PORT:-8441}"`
		if !strings.Contains(compose, published) {
			t.Errorf("%s: the compose file does not publish this machine's port on its loopback alone", kind)
		}
		env := readFile(t, filepath.Join(dir, environmentFile))
		if !strings.Contains(env, "ORPHEUM_THIS_MACHINE_PORT=18441\n") {
			t.Errorf("%s: .env does not name this machine's port", kind)
		}
		listen := fmt.Sprint(decodeTOML(t, filepath.Join(dir, coreConfig))["server"]["this_machine_listen"])
		if listen != "0.0.0.0:18441" {
			t.Errorf("%s: the core's this_machine_listen is %q; want 0.0.0.0:18441", kind, listen)
		}
	}
	values := filledValues(t, Release)
	values.HTTP.ThisMachinePort = values.HTTP.Port
	if problems := values.Check(); problems == nil || !strings.Contains(problems.Error(), "http.this_machine_port") {
		t.Errorf("this machine's port the same as http.port = %v; want it refused, naming it", problems)
	}
}
