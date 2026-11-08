// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto_test

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

func TestIsPublic_CarrierNATDSLiteAndDocumentationAreNeverPublic(t *testing.T) {
	t.Parallel()
	for address, want := range map[string]bool{
		"8.8.8.8": true, "11.22.44.139": true, "2a0f:5000::1": true, "2a0f:2000:1c16:2684::1": true,
		"::ffff:8.8.8.8": true,
		"100.64.0.1":     false, "100.127.255.254": false, // carrier NAT
		"192.0.0.2": false, // DS-Lite's B4
		"192.0.2.1": false, "198.51.100.9": false, "203.0.113.7": false, "2001:db8::1": false, "3fff::1": false,
		"10.0.0.1": false, "172.16.0.1": false, "192.168.1.20": false, "fd77::2": false, "::ffff:192.168.1.1": false,
		"127.0.0.1": false, "::1": false, "169.254.1.1": false, "fe80::1": false, "fe80::1%eth0": false,
		"224.0.0.1": false, "ff02::1": false, "0.0.0.0": false, "::": false, "255.255.255.255": false,
		"64:ff9b::808:808": false, "2001:0:4136:e378::1": false, "2002:c000:204::1": false,
		"198.18.0.1": false, "240.0.0.1": false, "100::1": false,
	} {
		if got := discoveryproto.IsPublic(netip.MustParseAddr(address)); got != want {
			t.Errorf("IsPublic(%s) = %t, want %t", address, got, want)
		}
	}
}

func TestAddressLabel_OneAddressOneLabelAndNoDoubleDash(t *testing.T) {
	t.Parallel()
	for address, want := range map[string]string{
		"192.168.1.20":         "192-168-1-20",
		"203.0.113.7":          "203-0-113-7",
		"::ffff:203.0.113.7":   "203-0-113-7",
		"2a0f:5000::1":         "2a0f-5000-0-0-0-0-0-1",
		"2A0F:5000:0:0::1":     "2a0f-5000-0-0-0-0-0-1",
		"2a0f:5000:00ab::1":    "2a0f-5000-ab-0-0-0-0-1",
		"::1":                  "0-0-0-0-0-0-0-1",
		"2001:db8::8a2e:370:1": "2001-db8-0-0-0-8a2e-370-1",
	} {
		got, isLabel := discoveryproto.AddressLabel(netip.MustParseAddr(address))
		if !isLabel || got != want {
			t.Errorf("AddressLabel(%s) = %q, %t; want %q", address, got, isLabel, want)
		}
		if strings.Contains(got, "--") {
			t.Errorf("AddressLabel(%s) = %q holds --", address, got)
		}
	}
	for _, address := range []netip.Addr{{}, netip.MustParseAddr("fe80::1%eth0")} {
		if got, isLabel := discoveryproto.AddressLabel(address); isLabel {
			t.Errorf("AddressLabel(%v) = %q; want none", address, got)
		}
	}
}
