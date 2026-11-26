// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pion/webrtc/v4"
)

// WhyRelayed is why a connection whose selected pair goes through a relay did not go without one, as
// its other pairs stood when it connected: what says whether devices take the relay needlessly
// (the owner, 2026-11-26; D558 §13).
type WhyRelayed string

// Why a connection is relayed.
const (
	// RelayedNoDirectPair: no pair without a relay was there to try; one end named no candidate but
	// a relay's, as a browser held to its relay does, or one whose own were all dropped.
	RelayedNoDirectPair WhyRelayed = "noDirectPair"
	// RelayedDirectFailed: every pair without a relay had failed. The relay was needed.
	RelayedDirectFailed WhyRelayed = "directFailed"
	// RelayedDirectUnfinished: a pair without a relay was still being checked, or waiting to be.
	// The relay answered first; the direct pair might have worked.
	RelayedDirectUnfinished WhyRelayed = "directUnfinished"
	// RelayedDirectSucceeded: a pair without a relay had succeeded, and the relayed one was selected
	// all the same. The relay was not needed.
	RelayedDirectSucceeded WhyRelayed = "directSucceeded"
)

// tried is what a connection's stats say of the candidates the client offered and of its pairs
// without a relay, as they stood when it connected.
type tried struct {
	// offered counts the client's candidates by type, "relay 2; srflx 1"; "" for none.
	offered string
	// direct names each pair with no relay at either end and its state, as candidatesOf does.
	direct []string
	// why is why the connection, if relayed, did not take one of them.
	why WhyRelayed
}

// triedOf reads report, a connection's stats.
func triedOf(report webrtc.StatsReport) tried {
	isRelay, named := map[string]bool{}, map[string]string{}
	offered := map[string]int{}
	for _, stats := range report {
		candidate, isCandidate := stats.(webrtc.ICECandidateStats)
		if !isCandidate {
			continue
		}
		isRelay[candidate.ID] = candidate.CandidateType == webrtc.ICECandidateTypeRelay
		named[candidate.ID] = candidateText(candidate)
		if candidate.Type == webrtc.StatsTypeRemoteCandidate {
			offered[candidate.CandidateType.String()]++
		}
	}
	found := tried{why: RelayedNoDirectPair}
	kinds := make([]string, 0, len(offered))
	for kind, count := range offered {
		kinds = append(kinds, fmt.Sprintf("%s %d", kind, count))
	}
	slices.Sort(kinds)
	found.offered = strings.Join(kinds, "; ")
	for _, stats := range report {
		pair, isPair := stats.(webrtc.ICECandidatePairStats)
		if !isPair || isRelay[pair.LocalCandidateID] || isRelay[pair.RemoteCandidateID] {
			continue
		}
		found.direct = append(found.direct, pairText(pair, named))
		found.why = worse(found.why, whyOf(pair.State))
	}
	slices.Sort(found.direct)
	return found
}

// whyOf is what one pair without a relay, in state, says of a relayed connection.
func whyOf(state webrtc.StatsICECandidatePairState) WhyRelayed {
	switch state {
	case webrtc.StatsICECandidatePairStateSucceeded:
		return RelayedDirectSucceeded
	case webrtc.StatsICECandidatePairStateFailed:
		return RelayedDirectFailed
	case webrtc.StatsICECandidatePairStateFrozen, webrtc.StatsICECandidatePairStateWaiting,
		webrtc.StatsICECandidatePairStateInProgress:
		return RelayedDirectUnfinished
	default:
		return RelayedDirectUnfinished
	}
}

// worse is whichever of two reasons says more against the relay having been needed: a pair that
// succeeded outweighs one unfinished, and that one that failed, and that there being none.
func worse(one, other WhyRelayed) WhyRelayed {
	order := []WhyRelayed{RelayedNoDirectPair, RelayedDirectFailed, RelayedDirectUnfinished, RelayedDirectSucceeded}
	if slices.Index(order, other) > slices.Index(order, one) {
		return other
	}
	return one
}
