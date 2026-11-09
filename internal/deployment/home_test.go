// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"fmt"
	"path/filepath"
	"testing"
)

// The deployed core lists no network as home: every private one is home anyway, with its loopback,
// which only a command run in its container reaches (D334 §2, §3), and listing the house's /24 kept the
// typed address's network, which a new lease leaves (the owner, 2026-11-09). With no LAN address the
// config is still written, as the address is learned (D452 §5).
func TestWrite_TheCoreAdministersOnlyFromTheHousesNetwork(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	values.LAN.Address = "192.168.1.199"
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	server := decodeTOML(t, filepath.Join(dir, coreConfig))["server"]
	if got := fmt.Sprint(server["home_networks"]); got != "[]" {
		t.Errorf("the core's server.home_networks are %s; want none listed", got)
	}
	if _, named := server["home_is_private"]; named {
		t.Error("the core's config names server.home_is_private, which the core no longer reads")
	}
	values.LAN.Address = ""
	if err := Write(dir, values, runtime); err != nil {
		t.Errorf("writing with no address at home = %v; want it written, as it is learned (D452 §5)", err)
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
	if got := fmt.Sprint(server["not_home_networks"]); got != "[10.77.0.0/24 fd77::/64]" {
		t.Errorf("the core's server.not_home_networks are %s; want the tunnel's", got)
	}
	values.VPS.Endpoint = ""
	if got := values.NotHomeNetworks(); got == nil || len(got) != 0 {
		t.Errorf("with no tunnel the networks never home are %#v; want an empty list", got)
	}
}
