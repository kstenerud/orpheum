// SPDX-License-Identifier: GPL-3.0-or-later

package identify

// The names identify_test reads that the package keeps to itself. The tests that drive a library
// through identifytest.Provider live in identify_test, since identifytest imports this package;
// they compare what the pass stored against the schema's closed sets (vocabulary.go) and check
// two of its rules, and nothing outside the package needs any of them.
const (
	KindExtra = kindExtra

	SourceNFO  = sourceNFO
	SourceTMDB = sourceTMDB

	IdentificationIdentified   = identificationIdentified
	IdentificationUnidentified = identificationUnidentified
	IdentificationConfirmed    = identificationConfirmed
	IdentificationAmbiguous    = identificationAmbiguous

	FilingListed       = filingListed
	FilingUnidentified = filingUnidentified
	FilingExtra        = filingExtra

	FieldName    = fieldName
	FieldYear    = fieldYear
	FieldTagline = fieldTagline
)

// The rules two tests check by name.
var (
	// NoMatchReason is why a film its provider found nothing for is unidentified.
	NoMatchReason = noMatchReason
	// DueAfter is how long after its fetch the rolling refresh queues an item.
	DueAfter = dueAfter
)
