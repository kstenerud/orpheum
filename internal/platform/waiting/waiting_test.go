// SPDX-License-Identifier: AGPL-3.0-or-later

package waiting_test

import (
	"context"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/waiting"
)

func TestForAPerson_MarksTheContextAndWhatDerivesFromIt(t *testing.T) {
	t.Parallel()
	if waiting.IsAPerson(t.Context()) {
		t.Error("an unmarked context counts as a person's")
	}
	marked := waiting.ForAPerson(t.Context())
	derived, cancel := context.WithCancel(marked)
	defer cancel()
	if !waiting.IsAPerson(marked) || !waiting.IsAPerson(derived) {
		t.Error("a marked context, or one derived from it, does not count as a person's")
	}
}
