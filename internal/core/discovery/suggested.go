// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

// This file is the one place Orpheum names its paid discovery (D391 §7): every other shipped source
// takes the suggestion from Suggested, through the core
// (TestSuggested_IsTheOnlyShippedSourceThatNamesTheOwnersDomains).

// Suggestion is the discovery a core's admin is offered first: its issuer, and the root key set it is
// known by, which the core pins at enrolment in place of trusting the set the issuer shows. A fork
// or a household that wants none offers none: the System page then asks for an issuer with no
// default.
type Suggestion struct {
	// Issuer is the discovery's typed origin.
	Issuer string
	// RootSetVersion is the version of the set RootKeys are; the issuer's later sets chain from it.
	RootSetVersion int64
	// RootKeys are the set's public keys, authorised keys' lines ("sk-ssh-ed25519@openssh.com AAAA…"):
	// one for each of the owner's YubiKeys (D391 §2).
	RootKeys []string
}

// Suggested is the paid service's suggestion (D390 §1, D391 §7).
//
// Its root keys are not known yet: the owner fills them in from `ssh-keygen -K` on each YubiKey
// (docs/users/discovery-signing.md). Until then the list is empty, and enrolling with the suggested
// issuer fails, saying so, rather than trusting whatever set the issuer shows.
func Suggested() Suggestion {
	return Suggestion{
		Issuer:         "https://orpheum.cc",
		RootSetVersion: 1,
		RootKeys:       []string{
			// The owner's three YubiKeys' public keys, one line each, go here.
		},
	}
}
