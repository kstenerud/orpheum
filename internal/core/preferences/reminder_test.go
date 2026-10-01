// SPDX-License-Identifier: GPL-3.0-or-later

package preferences

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestOfflineReminder_ADismissalHidesItForTheIntervalFromThatAdminAlone(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	service := NewService(database, fakeClock)
	alice := storetest.InsertUser(t, database, "alice", "admin")
	bob := storetest.InsertUser(t, database, "bob", "admin")
	const week = 7 * 24 * time.Hour
	offlineSince := fakeClock.Now().Add(-time.Hour)
	isDue := func(userID int64) bool {
		t.Helper()
		due, err := service.IsOfflineReminderDue(t.Context(), userID, week, offlineSince)
		if err != nil {
			t.Fatal(err)
		}
		return due
	}
	if !isDue(alice) {
		t.Fatal("a reminder never dismissed is not due")
	}
	if err := service.DismissOfflineReminder(t.Context(), alice); err != nil {
		t.Fatal(err)
	}
	if isDue(alice) {
		t.Error("a reminder just dismissed is still due")
	}
	if !isDue(bob) {
		t.Error("one admin's dismissal hid the reminder from another")
	}
	fakeClock.Advance(week - time.Minute)
	if isDue(alice) {
		t.Error("the reminder came back before its interval passed")
	}
	fakeClock.Advance(time.Minute)
	if !isDue(alice) {
		t.Error("the reminder did not come back once its interval passed")
	}
}

func TestOfflineReminder_SomethingNewlyOfflineBringsADismissedReminderBackAtOnce(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	service := NewService(database, fakeClock)
	admin := storetest.InsertUser(t, database, "admin", "admin")
	const week = 7 * 24 * time.Hour
	isDue := func(newestOfflineSince time.Time) bool {
		t.Helper()
		due, err := service.IsOfflineReminderDue(t.Context(), admin, week, newestOfflineSince)
		if err != nil {
			t.Fatal(err)
		}
		return due
	}
	seen := fakeClock.Now().Add(-time.Hour)
	if err := service.DismissOfflineReminder(t.Context(), admin); err != nil {
		t.Fatal(err)
	}
	fakeClock.Advance(time.Hour)
	if isDue(seen) {
		t.Error("what was offline when the admin dismissed it brought the reminder back within the week")
	}
	if !isDue(fakeClock.Now().Add(-time.Minute)) {
		t.Error("a disk that went missing after the dismissal did not bring the reminder back")
	}
}
