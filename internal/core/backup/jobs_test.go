// SPDX-License-Identifier: GPL-3.0-or-later

package backup

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// heardQueue keeps the backup jobs asked for.
type heardQueue struct{ requests []jobs.Request }

func (queue *heardQueue) Enqueue(_ context.Context, request jobs.Request) (bool, error) {
	queue.requests = append(queue.requests, request)
	return true, nil
}

// startingSettings are the backup settings at their starting values.
type startingSettings struct{ kept int64 }

func (config startingSettings) Int(_ context.Context, key settings.Key) (int64, error) {
	if key == settings.BackupsKept {
		return config.kept, nil
	}
	return 4, nil // backup_hour_utc
}

func (startingSettings) Duration(context.Context, settings.Key) (time.Duration, error) {
	return 24 * time.Hour, nil // backup_every_hours
}

// scheduleBench is a backup service over an empty instance holding files, its clock at now, and
// what it asked for.
func scheduleBench(t *testing.T, now time.Time, files ...string) (*Service, *heardQueue, *instancedir.Directories) {
	t.Helper()
	directories := emptyInstance(t)
	for _, name := range files {
		path, err := directories.BackupPath(name)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	queue := &heardQueue{}
	service := NewService(nil, directories, clocktest.NewFake(now))
	service.UseQueue(queue, startingSettings{kept: 2})
	return service, queue, directories
}

// The scheduled backup is asked for at the quiet hour once the newest is a day old, at any hour once
// it is a day overdue, and never holding the key (D243 §11).
func TestQueueScheduled_AsksAtTheQuietHourWhenDueAndAtOnceWhenOverdue(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		what  string
		at    time.Time
		files []string
		want  int
	}{
		{"none yet, at the quiet hour", time.Date(2026, 10, 5, 4, 10, 0, 0, time.UTC), nil, 1},
		{"none yet, at another hour", time.Date(2026, 10, 5, 13, 0, 0, 0, time.UTC), nil, 1},
		{
			"a day old, at the quiet hour", time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC),
			[]string{"backup-20261004T040500Z.tar.gz"},
			1,
		},
		{
			"a day old, at another hour", time.Date(2026, 10, 5, 13, 0, 0, 0, time.UTC),
			[]string{"backup-20261004T040500Z.tar.gz"},
			0,
		},
		{
			"made today", time.Date(2026, 10, 5, 4, 30, 0, 0, time.UTC),
			[]string{"backup-20261005T040500Z.tar.gz"},
			0,
		},
		{
			"two days old, at another hour", time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC),
			[]string{"backup-20261004T040500Z.tar.gz"},
			1,
		},
		{
			"a pre-restore copy is not a backup", time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC),
			[]string{"pre-restore-20261005T030000Z.tar.gz", "backup-20261004T040500Z.tar.gz"},
			1,
		},
	} {
		service, queue, _ := scheduleBench(t, check.at, check.files...)
		asked, err := service.QueueScheduled(t.Context())
		if err != nil || asked != check.want || len(queue.requests) != check.want {
			t.Errorf("%s: asked for %d (%v), want %d", check.what, asked, err, check.want)
			continue
		}
		if asked == 1 && queue.requests[0].ArgsJSON != `{"isScheduled":true}` {
			t.Errorf("%s: a scheduled backup's arguments are %s; want it scheduled and without the key",
				check.what, queue.requests[0].ArgsJSON)
		}
	}
}

// Pruning keeps the newest backups_kept backups, and never the copies kept before a restore or a
// migration (D243 §11).
func TestPrune_KeepsTheNewestBackupsAndEveryCopyKeptBeforeARestoreOrMigration(t *testing.T) {
	t.Parallel()
	service, _, directories := scheduleBench(t, time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC),
		"backup-20261001T040000Z.tar.gz", "backup-20261002T040000Z.tar.gz", "backup-20261003T040000Z.tar.gz",
		"pre-restore-20261001T050000Z.tar.gz", "pre-migrate-v31.sqlite")
	if err := service.prune(t.Context()); err != nil {
		t.Fatal(err)
	}
	files, err := directories.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, file := range files {
		names = append(names, file.Name)
	}
	slices.Sort(names)
	want := []string{
		"backup-20261002T040000Z.tar.gz", "backup-20261003T040000Z.tar.gz", "pre-migrate-v31.sqlite",
		"pre-restore-20261001T050000Z.tar.gz",
	}
	if !slices.Equal(names, want) {
		t.Errorf("after pruning to two: %v, want %v", names, want)
	}
}

// A repeat request holds the key when either asked for it, and is scheduled only when both were;
// a running backup covers a request unless the request wants the key it does not hold.
func TestJobArgsMerge_HoldsTheKeyWhenEitherAskedAndARunningOneCoversWhatItHolds(t *testing.T) {
	t.Parallel()
	merge := JobArgsMerge()
	merged, err := merge.Merge(`{"isScheduled":true}`, `{"includesKey":true}`)
	var args JobArgs
	if err != nil || json.Unmarshal([]byte(merged), &args) != nil || !args.IncludesKey || args.IsScheduled {
		t.Errorf("a person's request with the key merged into a scheduled one = %s, %v; want the key, not scheduled",
			merged, err)
	}
	for _, check := range []struct {
		running, requested string
		want               bool
	}{
		{`{"isScheduled":true}`, `{}`, true},
		{`{"isScheduled":true}`, `{"includesKey":true}`, false},
		{`{"includesKey":true}`, `{"includesKey":true}`, true},
	} {
		if covers, err := merge.Covers(check.running, check.requested); err != nil || covers != check.want {
			t.Errorf("a running %s covers a request %s: %v (%v), want %v", check.running, check.requested, covers,
				err, check.want)
		}
	}
}
