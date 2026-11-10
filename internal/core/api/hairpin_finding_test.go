// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// The admin is warned while devices reach the server through the provider and back, told which UDP
// port at which of the house's addresses a device at home could not reach, and how it clears (D354 §2).
func TestHairpinFinding_WarnsThatDevicesAtHomeReachTheServerThroughTheProvider(t *testing.T) {
	t.Parallel()
	if none := api.HairpinFinding(rtcapi.Hairpin{}, false); none.Verdict != api.VerdictOK {
		t.Errorf("with no hairpin standing: %+v; want ok", none)
	}
	hairpin := rtcapi.Hairpin{
		LastAt: time.Date(2026, 10, 26, 20, 0, 0, 0, time.UTC), Client: "11.22.33.144", Count: 3,
		UDPPort: 8443, LANAddresses: []netip.Addr{netip.MustParseAddr("192.168.1.199")},
	}
	warned := api.HairpinFinding(hairpin, true)
	if warned.Verdict != api.VerdictWarning ||
		!strings.Contains(warned.Detail, "devices at home reach this server through your provider, not your "+
			"network: UDP port 8443 at 192.168.1.199 is not reachable") ||
		!strings.Contains(warned.Detail, "11.22.33.144") || !strings.Contains(warned.Action, "8443:8443/udp") {
		t.Errorf("with a hairpin standing: %+v; want a warning naming UDP 8443 at 192.168.1.199 and how to "+
			"publish it", warned)
	}
	hairpin.UDPPort, hairpin.LANAddresses = 0, nil
	unset := api.HairpinFinding(hairpin, true)
	if unset.Verdict != api.VerdictWarning ||
		!strings.Contains(unset.Action, "WebRTC port and the address at home on the System page") {
		t.Errorf("with a hairpin standing and no LAN port: %+v; want a warning naming the setting", unset)
	}
}
