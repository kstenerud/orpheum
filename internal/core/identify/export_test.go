// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// The names identify_test reads that the package keeps to itself. The tests that drive a library
// through identifytest.Provider live in identify_test, since identifytest imports this package;
// they compare what the pass stored against the schema's closed sets (vocabulary.go) and check
// two of its rules, and nothing outside the package needs any of them.
const (
	KindExtra = library.TitleKindExtra

	SourceNFO  = sourceNFO
	SourceTMDB = sourceTMDB

	IdentificationIdentified   = identificationIdentified
	IdentificationUnidentified = identificationUnidentified
	IdentificationConfirmed    = identificationConfirmed
	IdentificationAmbiguous    = identificationAmbiguous

	FilingListed       = library.FilingListed
	FilingUnidentified = library.FilingUnidentified
	FilingExtra        = library.FilingExtra

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
	// WorkName is a name as the matcher compares it.
	WorkName = workName
	// SpellingVariants are the other spellings a match searches for.
	SpellingVariants = spellingVariants
)

// ReadAsItGoes has a pass read each row when it needs it rather than from its snapshot, as a
// match's filer does, so a test can compare what the two passes write.
func ReadAsItGoes(pass *Pass) { pass.readsAsItGoes = true }

// BatchRows has a pass's batches commit after rows changed rows, so a test's small library files in
// many batches.
func BatchRows(pass *Pass, rows int64) { pass.batchRows = rows }

// BetweenBatches has a pass run between after each batch it commits, with the rows it changed.
func BetweenBatches(pass *Pass, between func(ctx context.Context, rowsChanged int64)) {
	pass.betweenBatches = between
}
