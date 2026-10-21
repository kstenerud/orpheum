// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// A root whose disk holds a reading that has not returned is left out of the node's start and of
// what is found of its devices, which the core then makes unavailable (D241 §2), where its open held
// the start for good; the others are opened and found (D233 §10; staging, 2026-10-21).
func TestOpenRoots_ARootWhoseDiskDoesNotAnswerIsLeftOutAndTheOthersOpen(t *testing.T) {
	t.Parallel()
	answers, stuck := t.TempDir(), t.TempDir()
	disks := diskwatch.NewGate(clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)))
	release, isHeld := disks.Hold(stuck, stuck)
	if !isHeld {
		t.Fatal("the root could not be held")
	}
	t.Cleanup(release)
	roots := openRootsThrough(t.Context(), []string{answers, stuck}, disks, slog.New(slog.DiscardHandler))
	t.Cleanup(func() {
		for _, root := range roots {
			_ = root.Close() //nolint:errcheck // the test is over
		}
	})
	if len(roots) != 1 || roots[0].Path() != answers {
		t.Errorf("the roots opened are %d; want %s alone", len(roots), answers)
	}
	if found := answering(t.Context(), []string{answers, stuck}, disks); len(found) != 1 || found[0] != answers {
		t.Errorf("the paths answering are %v; want %s alone", found, answers)
	}
}
