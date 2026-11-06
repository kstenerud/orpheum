// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"net/netip"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// Status is where the core stands with its discovery, for its admin.
type Status struct {
	// Issuer is the discovery's, the origin its admin enrolled the core by; BaseURL the core API its
	// address document names, which the core calls. Both are empty when the core is enrolled with
	// none.
	Issuer  string
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
	// DeadEndCausesLastWeek is those dead ends by the cause each page reported, in the discovery's
	// words; nil from a discovery that keeps no causes (D398 §7).
	DeadEndCausesLastWeek map[string]int
	Published             []string
	Probe                 []discoveryproto.ProbeResult
	// LastHeartbeatAt is when the discovery last took the core's addresses.
	LastHeartbeatAt time.Time
	// Problem is why the last call failed, empty when it did not.
	Problem string
	// StoodAsideAt is when the discovery refused this core as a copy of another that serves, after
	// which it stopped asking, until its admin asks again (Service.AskAgain) or it restarts; zero while
	// it asks (D437 §3).
	StoodAsideAt time.Time
	// CopyRefusedFrom is the address a copy of this core last asked the discovery from, and
	// CopyRefusedAt when, the discovery having refused it because this one serves; empty while none
	// has in this run (D437 §4).
	CopyRefusedFrom string
	CopyRefusedAt   time.Time
	// DeprecatedUntil is the date the discovery stops answering this protocol, when it said so.
	DeprecatedUntil string
	// ICEServers are the STUN servers the discovery names for the core's WebRTC candidates.
	ICEServers []discoveryproto.ICEServer
	// ClientOrigin is where the discovery serves the core's own client, which its links lead to, and
	// LoaderOrigin where it serves the loader that hands a browser on to it (D293 §4, D308): its
	// address document's, and no other (D391 §3). Homepage says what the discovery is.
	ClientOrigin, LoaderOrigin, Homepage string
	// Identity is the discovery's, its root chain's genesis hash ("od1-…"), which the core's
	// signatures are bound to; Name is what its document calls it, and IsSuggested whether it is the
	// suggested service (the root key audit's §3.8).
	Identity, Name string
	IsSuggested    bool
	// RootKeys are the keys whose signatures count in the current root set (suspended keys left out),
	// each "<role> SHA256:…" as `ssh-keygen -l` shows its fingerprint, and RootVersion the set's version;
	// DocumentIssuedAt is when the address document held was issued, DocumentStaleSince when it expired,
	// zero while it is current (the server cannot confirm its addresses are current since then), and
	// DocumentProblem what the last look found wrong: a conflict of root statements, equivocation, a
	// document no longer signed.
	RootKeys           []string
	RootVersion        int64
	DocumentIssuedAt   time.Time
	DocumentStaleSince time.Time
	DocumentProblem    string
	// SuggestedIssuer is the discovery an admin is offered first (Suggested); empty offers none.
	SuggestedIssuer string
	// HomeAddresses are the core's addresses at home, "https://<label>.<coreId>.<zone>:<port>", from
	// the house's addresses it sends, which reach it directly in the house; none until the discovery
	// publishes it (D292 §6). IsHomeAddressReady is whether the certificate they need is held yet:
	// until it is, a browser at home reaches the core the long way, which gives no admin (D297 §6).
	HomeAddresses      []string
	IsHomeAddressReady bool
	// HouseNetworks are the house's public IPv6 /64s as the next heartbeat tells them: learnt from
	// devices proven at home, the most recently witnessed first, then those the admin named (D356 §3,
	// §10); none while the core is enrolled with no discovery.
	HouseNetworks []HouseNetwork
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

// homeAddresses are the https addresses of candidates only a client at home reaches, under the
// core's names: "https://<label>.<coreId>.<zone>", with the port unless it is 443. A private address
// is one, as the discovery publishes it, whatever its scope says: a core in a container names its
// house addresses in its configuration, unmarked (nas.example, 2026-10-20).
func homeAddresses(candidates []discoveryproto.Candidate, coreID, zone string) []string {
	if coreID == "" || zone == "" {
		return nil
	}
	var addresses []string
	for _, candidate := range candidates {
		address, err := netip.ParseAddr(candidate.Address)
		if err != nil || !address.Unmap().IsPrivate() {
			continue
		}
		label, isLabel := discoveryproto.AddressLabel(address)
		if !isLabel {
			continue
		}
		home := "https://" + label + "." + coreID + "." + zone
		if candidate.Port != 0 && candidate.Port != httpsPort {
			home += ":" + strconv.Itoa(candidate.Port)
		}
		addresses = append(addresses, home)
	}
	return addresses
}

// httpsPort is HTTPS's own port, which an address leaves unsaid.
const httpsPort = 443
