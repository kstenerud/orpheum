// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/home"
)

// houseInterfaces is a machine with a wired and a wireless address in the house, a Docker bridge, a
// loopback and a link-local address: only the first two are where a browser at home reaches it.
func houseInterfaces() []home.Interface {
	return []home.Interface{
		{Name: "lo", Addresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")}},
		{Name: "eth0", Addresses: []netip.Addr{
			netip.MustParseAddr("192.168.1.20"), netip.MustParseAddr("169.254.9.9"),
		}},
		{Name: "docker0", Addresses: []netip.Addr{netip.MustParseAddr("172.17.0.1")}},
		{Name: "wlan0", Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.7")}},
	}
}

// The magic link (D527 §1.2) is one for each address at home, on the HTTP port, carrying the code in
// the fragment, so no server log sees it.
func TestSetupReach_LinksOneForEachAddressAtHomeWithTheCodeInTheFragment(t *testing.T) {
	t.Parallel()
	reach := setupReach{port: 8096, interfaces: houseInterfaces}
	got := reach.links("7kqm-2xhp-9rtw")
	want := []string{
		"http://192.168.1.20:8096/#code=7kqm-2xhp-9rtw",
		"http://10.0.0.7:8096/#code=7kqm-2xhp-9rtw",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("links = %v; want %v", got, want)
	}
}

// Port 80 is the scheme's own and is left out; no port known, or no address at home, makes no link, so
// the log falls back to the address in words.
func TestSetupReach_LeavesOutPort80AndMakesNoLinkWithoutAPortOrAnAddress(t *testing.T) {
	t.Parallel()
	if got := (setupReach{port: 80, interfaces: houseInterfaces}).links("code")[0]; got !=
		"http://192.168.1.20/#code=code" {
		t.Errorf("port 80 link = %q; want the port left out", got)
	}
	if got := (setupReach{port: 0, interfaces: houseInterfaces}).links("code"); len(got) != 0 {
		t.Errorf("with no port known: %v; want no link", got)
	}
	none := func() []home.Interface { return []home.Interface{{Name: "lo"}} }
	if got := (setupReach{port: 8096, interfaces: none}).links("code"); len(got) != 0 {
		t.Errorf("with no address at home: %v; want no link", got)
	}
}

// `orpheum setup-code` prints the code alone on the standard output, which the install script reads as
// the code, and the links that carry it on the standard error.
func TestSetupCode_PrintsTheCodeAloneAndTheLinksBesideIt(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	// The server was handed its listener; the config on disk names the port the links carry.
	configFile := config.Instance.fullConfigPath()
	written, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	moved := strings.Replace(string(written), `listen = "127.0.0.1:0"`, `listen = "127.0.0.1:8096"`, 1)
	if moved == string(written) {
		t.Fatal("the test config names no listen address to move")
	}
	if err = os.WriteFile(configFile, []byte(moved), 0o600); err != nil { //nolint:gosec // G703: the test's own file
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	status := commandSetupCode(t.Context(), invocation{
		arguments: []string{"--instance", path}, lookupEnv: noEnvironment, stdout: &stdout, stderr: &stderr,
		interfaces: houseInterfaces,
	})
	code := strings.TrimSpace(stdout.String())
	if status != exitSuccess || len(code) != len("AAAA-BBBB-CCCC") {
		t.Fatalf("setup-code = %d, stdout %q; want the code alone", status, stdout.String())
	}
	if want := "http://192.168.1.20:8096/#code=" + code; !strings.Contains(stderr.String(), want) {
		t.Errorf("setup-code's links = %q; want %q", stderr.String(), want)
	}
	if err = server.stop(); err != nil {
		t.Fatal(err)
	}
}
