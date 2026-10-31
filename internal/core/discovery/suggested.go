// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import "github.com/kstenerud/orpheum/discovery/trust"

// This file is the one place Orpheum names its paid discovery and its publisher root (D391 §7, D393
// §2): every other shipped source takes them from here, through the core
// (TestSuggested_IsTheOnlyShippedSourceThatNamesTheOwnersDomains).

// Suggestion is the discovery a core's admin is offered first: its issuer, and the genesis of its
// root chain, whose hash is its identity. A core enrolling with the suggested issuer takes its chain
// only from this genesis, never from one the issuer shows (the root key audit's §3.8). A fork or a
// household that wants none offers none: the System page then asks for an issuer with no default.
type Suggestion struct {
	Issuer  string
	Genesis trust.Signed
}

// Suggested is the paid service's suggestion (D390 §1, D391 §7).
//
// Its genesis is not made yet: the owner makes it at the day-one ceremony (the audit's §4) and puts
// the statement and its signature here, the identity in the commit's message. Until then enrolling
// with the suggested issuer fails, saying so, rather than trusting whatever chain the issuer shows.
func Suggested() Suggestion {
	return Suggestion{
		Issuer: "https://orpheum.cc",
		// The discovery chain's genesis statement and its signature file go here, verbatim.
		Genesis: trust.Signed{},
	}
}

// PublisherRoot is Orpheum's publisher root, which signs release records (D393 §2): the genesis of its
// chain, compiled into every binary, and the places its chain and record are served at.
type PublisherRoot struct {
	Genesis   trust.Signed
	Locations []string
}

// Publisher is the publisher root this build trusts. Its genesis is made at the day-one ceremony too;
// until it is here a release build checks no release record, and says so in its log.
func Publisher() PublisherRoot {
	return PublisherRoot{
		// The publisher chain's genesis statement and its signature file go here, verbatim.
		Genesis:   trust.Signed{},
		Locations: []string{"https://orpheum.cc"},
	}
}
