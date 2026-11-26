// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pion/webrtc/v4"
)

// candidatesOf names a connection's candidates and their pairs, as its stats hold them: what a
// connection that never connected tried, so its log says why rather than leaving it to be dug for
// (the owner's Firefox at home, 2026-10-15, whose connections to the core's LAN address closed
// unconnected with nothing said of what was tried).
func candidatesOf(report webrtc.StatsReport) (local, remote, pairs []string) {
	named := map[string]string{}
	for _, stats := range report {
		candidate, isCandidate := stats.(webrtc.ICECandidateStats)
		if !isCandidate {
			continue
		}
		text := candidateText(candidate)
		named[candidate.ID] = text
		if candidate.Type == webrtc.StatsTypeLocalCandidate {
			local = append(local, text)
		} else {
			remote = append(remote, text)
		}
	}
	for _, stats := range report {
		if pair, isPair := stats.(webrtc.ICECandidatePairStats); isPair {
			pairs = append(pairs, pairText(pair, named))
		}
	}
	slices.Sort(local)
	slices.Sort(remote)
	slices.Sort(pairs)
	return local, remote, pairs
}

// candidateText names a candidate by its type and address, "srflx 203.0.113.9:40001".
func candidateText(candidate webrtc.ICECandidateStats) string {
	return fmt.Sprintf("%s %s:%d", candidate.CandidateType, candidate.IP, candidate.Port)
}

// pairText names a pair by its two candidates, each as named holds it by its ID, its state and its
// traffic.
func pairText(pair webrtc.ICECandidatePairStats, named map[string]string) string {
	return fmt.Sprintf("%s -> %s %s sent %d received %d",
		named[pair.LocalCandidateID], named[pair.RemoteCandidateID], pair.State,
		pair.PacketsSent, pair.PacketsReceived)
}

// joined is texts as one log value.
func joined(texts []string) string {
	return strings.Join(texts, "; ")
}
