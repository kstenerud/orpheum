// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

// A connection that never connected says what it tried: its own candidates, the client's, and each
// pair's state and traffic.
func TestCandidatesOf_NamesEachCandidateAndPairOfAConnection(t *testing.T) {
	t.Parallel()
	report := webrtc.StatsReport{
		"l": webrtc.ICECandidateStats{
			ID: "l", Type: webrtc.StatsTypeLocalCandidate, CandidateType: webrtc.ICECandidateTypeHost,
			IP: "192.168.1.199", Port: 8451,
		},
		"r": webrtc.ICECandidateStats{
			ID: "r", Type: webrtc.StatsTypeRemoteCandidate, CandidateType: webrtc.ICECandidateTypePrflx,
			IP: "192.168.1.20", Port: 55000,
		},
		"p": webrtc.ICECandidatePairStats{
			LocalCandidateID: "l", RemoteCandidateID: "r", State: webrtc.StatsICECandidatePairStateFailed,
			PacketsSent: 3, PacketsReceived: 1,
		},
	}
	local, remote, pairs := candidatesOf(report)
	if joined(local) != "host 192.168.1.199:8451" || joined(remote) != "prflx 192.168.1.20:55000" {
		t.Errorf("the candidates are %q and %q", local, remote)
	}
	want := "host 192.168.1.199:8451 -> prflx 192.168.1.20:55000 failed sent 3 received 1"
	if joined(pairs) != want {
		t.Errorf("the pairs are %q; want %q", pairs, want)
	}
}
