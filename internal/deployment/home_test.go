// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// The deployed core lists the LAN address's /24 and its own loopback as home outright, which only a
// command run in its container reaches (D334 §2, §3), never the tunnel's 10.77.0.0/24 (D297 §6). With
// no LAN address it lists the loopback alone, and the values name the address as a problem.
func TestWrite_TheCoreAdministersOnlyFromTheHousesNetwork(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	values.LAN.Address = "192.168.1.199"
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(decodeTOML(t, filepath.Join(dir, coreConfig))["server"]["home_networks"])
	if got != "[192.168.1.0/24 127.0.0.0/8 ::1/128]" {
		t.Errorf("the core's server.home_networks are %s; want the house's /24 and the loopback alone", got)
	}
	values.LAN.Address = ""
	if got := fmt.Sprint(values.HomeNetworks()); got != "[127.0.0.0/8 ::1/128]" {
		t.Errorf("with no address at home the home networks are %v; want the loopback alone", got)
	}
	if err := Write(dir, values, runtime); err == nil || !strings.Contains(err.Error(), "lan.address") {
		t.Errorf("writing with no address at home = %v; want it refused, naming lan.address", err)
	}
}

// A router that hands the house a new network locked admin out while home was the typed address's
// /24; the deployed core counts every private network as home, but the tunnel's, through which the
// internet arrives, and a deployment with no tunnel excepts none (D452 §1).
func TestWrite_TheCoreCountsEveryPrivateNetworkButTheTunnelsAsHome(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	server := decodeTOML(t, filepath.Join(dir, coreConfig))["server"]
	if isPrivate, isBool := server["home_is_private"].(bool); !isBool || !isPrivate {
		t.Errorf("the core's server.home_is_private is %v; want true", server["home_is_private"])
	}
	if got := fmt.Sprint(server["not_home_networks"]); got != "[10.77.0.0/24 fd77::/64]" {
		t.Errorf("the core's server.not_home_networks are %s; want the tunnel's", got)
	}
	values.VPS.Endpoint = ""
	if got := values.NotHomeNetworks(); got == nil || len(got) != 0 {
		t.Errorf("with no tunnel the networks never home are %#v; want an empty list", got)
	}
}
