// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// Carrying a choice to a file reads the file's own choice in the transaction that carries it, so
// an own choice kept earlier in that transaction stands; read from the reader pool, it would not be
// seen yet, and the carried choice would replace it.
func TestCarryChoice_SeesAnOwnChoiceKeptEarlierInItsTransaction(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := &Service{database: database, clock: clocktest.NewFake(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))}
	userID := storetest.InsertUser(t, database, "alice", "admin")
	const fileID = 6
	french := &SubtitleTrack{Language: "fr"}
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		if err := service.keepChoice(t.Context(), queries, userID, fileID, &SubtitleChoice{IsOff: true}); err != nil {
			return err
		}
		return service.carryChoice(t.Context(), queries, userID, fileID, french)
	})
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := service.FilePrefs(t.Context(), userID, fileID)
	if err != nil {
		t.Fatal(err)
	}
	if choice := prefs.SubtitleChoice; choice == nil || !choice.IsOff || choice.IsCarriedFromWork {
		t.Errorf("the file's choice = %+v, want its own choice, off, standing", choice)
	}
}
