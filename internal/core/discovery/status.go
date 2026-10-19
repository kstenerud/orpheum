// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// Status is where the core stands with its discovery, for its admin.
type Status struct {
	// BaseURL is the discovery's, empty when the core is enrolled with none.
	BaseURL string
	CoreID  string
	State   discoveryproto.State
	Zone    string
	// Realm is what a person signs in to the core by at the discovery's page, beside their name and
	// password: the realm the host's account there gave it, empty at a discovery with none (D297 §1).
	Realm          string
	ClaimCode      string
	ClaimExpiresAt time.Time
	// Account is the address of the account the core is the server of; RemovedAt when its identity
	// key was removed from an account, another core claimed in its place (the protocol's §2.2).
	Account       string
	RemovedAt     time.Time
	ReachableFrom string
	// DeadEndsLastWeek is how many times a client found no path in the last week, as the discovery
	// counted (D293 §7).
	DeadEndsLastWeek int
	Published        []string
	Probe            []discoveryproto.ProbeResult
	// LastHeartbeatAt is when the discovery last took the core's addresses.
	LastHeartbeatAt time.Time
	// Problem is why the last call failed, empty when it did not.
	Problem string
	// DeprecatedUntil is the date the discovery stops answering this protocol, when it said so.
	DeprecatedUntil string
	// ICEServers are the STUN servers the discovery names for the core's WebRTC candidates.
	ICEServers []discoveryproto.ICEServer
	// ClientOrigin is where the discovery serves its client, which the core's links lead to; empty
	// until it says, or when what it said is not an https origin (D293 §4).
	ClientOrigin string
	// LoaderOrigin is where the discovery serves the loader that hands a browser on to the client
	// origin (D308); empty until it says, or when what it said is not an https origin.
	LoaderOrigin string
	// Fingerprint is the identity key's, which the core's links carry (proof.Fingerprint), and
	// FingerprintText the same as a person reads and compares it, in safe32 groups
	// (proof.FingerprintText).
	Fingerprint, FingerprintText string
}

// IsPublished is whether the discovery publishes the core's addresses: it has claimed the core, and
// its subscription has not ended.
func (status Status) IsPublished() bool {
	return status.State == discoveryproto.StateClaimed || status.State == discoveryproto.StateGrace
}

// CertificateName is the wildcard every name the discovery publishes for the core is under,
// *.<coreId>.<zone>, which its certificate is for; empty before the discovery has given it a zone.
func (status Status) CertificateName() string {
	if status.CoreID == "" || status.Zone == "" {
		return ""
	}
	return "*." + status.CoreID + "." + status.Zone
}
