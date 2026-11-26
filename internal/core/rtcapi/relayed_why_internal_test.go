// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// relayedReport is the stats of a connection whose core has a host and a relayed candidate and whose
// client offered a server-reflexive and a relayed one: the relayed pair succeeded, and the pair
// without a relay stands in direct, none when direct is "".
func relayedReport(direct webrtc.StatsICECandidatePairState) webrtc.StatsReport {
	candidate := func(
		id string, side webrtc.StatsType, kind webrtc.ICECandidateType, ip string,
	) webrtc.ICECandidateStats {
		return webrtc.ICECandidateStats{ID: id, Type: side, CandidateType: kind, IP: ip, Port: 5000}
	}
	report := webrtc.StatsReport{
		"lh": candidate("lh", webrtc.StatsTypeLocalCandidate, webrtc.ICECandidateTypeHost, "192.168.1.199"),
		"lr": candidate("lr", webrtc.StatsTypeLocalCandidate, webrtc.ICECandidateTypeRelay, "203.0.113.1"),
		"rs": candidate("rs", webrtc.StatsTypeRemoteCandidate, webrtc.ICECandidateTypeSrflx, "198.51.100.4"),
		"rr": candidate("rr", webrtc.StatsTypeRemoteCandidate, webrtc.ICECandidateTypeRelay, "203.0.113.1"),
		"relayed": webrtc.ICECandidatePairStats{
			LocalCandidateID: "lr", RemoteCandidateID: "rr", State: webrtc.StatsICECandidatePairStateSucceeded,
		},
		"halfRelayed": webrtc.ICECandidatePairStats{
			LocalCandidateID: "lh", RemoteCandidateID: "rr", State: webrtc.StatsICECandidatePairStateSucceeded,
		},
	}
	if direct != "" {
		report["direct"] = webrtc.ICECandidatePairStats{
			LocalCandidateID: "lh", RemoteCandidateID: "rs", State: direct, PacketsSent: 4,
		}
	}
	return report
}

// A relayed connection's line says why it did not go without the relay, by how its pairs without
// one stood, and names them: whether a device took the relay needlessly is read from the log (the
// owner, 2026-11-26).
func TestRecordPath_SaysWhyAConnectionIsRelayed(t *testing.T) {
	t.Parallel()
	relay := webrtc.ICECandidateTypeRelay
	relayed := pairOf(relay, "203.0.113.1:5000", relay, "203.0.113.1:5000")
	for state, want := range map[webrtc.StatsICECandidatePairState]WhyRelayed{
		"":                                      RelayedNoDirectPair,
		webrtc.StatsICECandidatePairStateFailed: RelayedDirectFailed,
		webrtc.StatsICECandidatePairStateInProgress: RelayedDirectUnfinished,
		webrtc.StatsICECandidatePairStateWaiting:    RelayedDirectUnfinished,
		webrtc.StatsICECandidatePairStateFrozen:     RelayedDirectUnfinished,
		webrtc.StatsICECandidatePairStateSucceeded:  RelayedDirectSucceeded,
	} {
		t.Run(string(want)+" "+string(state), func(t *testing.T) {
			t.Parallel()
			log := &testutil.Log{}
			server := watchedServer(t, log, clocktest.NewFake(time.Unix(1_800_000_000, 0)))
			server.recordPath(PathRelayed, relayed, time.Second, relayedReport(state))
			wantPairs := `directPairs=""`
			if state != "" {
				wantPairs = `directPairs="host 192.168.1.199:5000 -> srflx 198.51.100.4:5000 ` +
					string(state) + ` sent 4 received 0"`
			}
			log.AwaitEvent(t, "clientConnected", "path=relayed", "relayedWhy="+string(want), wantPairs,
				`offered="relay 1; srflx 1"`)
		})
	}
}

// Of several pairs without a relay, the one that says most against the relay having been needed is
// the reason: one that succeeded over one that failed.
func TestTriedOf_TakesTheReasonThatSaysMostAgainstTheRelay(t *testing.T) {
	t.Parallel()
	report := relayedReport(webrtc.StatsICECandidatePairStateFailed)
	report["rh"] = webrtc.ICECandidateStats{
		ID: "rh", Type: webrtc.StatsTypeRemoteCandidate, CandidateType: webrtc.ICECandidateTypeHost,
		IP: "192.168.1.20", Port: 5000,
	}
	report["direct2"] = webrtc.ICECandidatePairStats{
		LocalCandidateID: "lh", RemoteCandidateID: "rh", State: webrtc.StatsICECandidatePairStateSucceeded,
	}
	found := triedOf(report)
	if found.why != RelayedDirectSucceeded || len(found.direct) != 2 || found.offered != "host 1; relay 1; srflx 1" {
		t.Errorf("tried is %+v; want directSucceeded, two pairs without a relay, and each kind offered", found)
	}
}

// A connection without a relay says what the client offered and nothing of a relay.
func TestRecordPath_SaysWhatTheClientOfferedOfAnyConnection(t *testing.T) {
	t.Parallel()
	log := &testutil.Log{}
	server := watchedServer(t, log, clocktest.NewFake(time.Unix(1_800_000_000, 0)))
	server.recordPath(PathPunchedIPv4, pairOf(webrtc.ICECandidateTypeSrflx, "203.0.113.9:40001",
		webrtc.ICECandidateTypeSrflx, "198.51.100.4:5000"), time.Second, relayedReport(""))
	line := log.AwaitEvent(t, "clientConnected", "path=punchedIpv4", `offered="relay 1; srflx 1"`)
	if strings.Contains(line, "relayedWhy") {
		t.Error("a connection without a relay names a reason for one")
	}
}
