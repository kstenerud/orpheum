// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
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

// startingSettings are the backup settings: every hours apart (one when zero), and the thinning's
// counts (D305 §2's starting values when zero).
type startingSettings struct {
	every                 int64
	hourly, daily, weekly int64
}

func (config startingSettings) Int(_ context.Context, key settings.Key) (int64, error) {
	if key == settings.BackupsKeptHourly {
		return cmp.Or(config.hourly, 24), nil
	}
	if key == settings.BackupsKeptDaily {
		return cmp.Or(config.daily, 7), nil
	}
	return cmp.Or(config.weekly, 4), nil // backups_kept_weekly
}

func (config startingSettings) Duration(context.Context, settings.Key) (time.Duration, error) {
	return time.Duration(max(config.every, 1)) * time.Hour, nil // backup_every_hours
}

// scheduleBench is a backup service over an empty instance holding files, its clock at now, and
// what it asked for.
func scheduleBench(t *testing.T, now time.Time, files ...string) (*Service, *heardQueue, *instancedir.Directories) {
	t.Helper()
	return scheduleBenchWith(t, startingSettings{}, now, files...)
}

// scheduleBenchWith is scheduleBench with config's settings.
func scheduleBenchWith(
	t *testing.T, config startingSettings, now time.Time, files ...string,
) (*Service, *heardQueue, *instancedir.Directories) {
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
	service.UseQueue(queue, config)
	return service, queue, directories
}

// The scheduled backup is asked for once the newest is an hour old, at any hour, and never holding
// the key (D305 §2).
func TestQueueScheduled_AsksOnceTheNewestIsAnHourOld(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		what  string
		at    time.Time
		files []string
		want  int
	}{
		{"none yet", time.Date(2026, 10, 5, 13, 10, 0, 0, time.UTC), nil, 1},
		{
			"half an hour old", time.Date(2026, 10, 5, 13, 35, 0, 0, time.UTC),
			[]string{"backup-20261005T130500Z.tar.gz"},
			0,
		},
		{
			"an hour old", time.Date(2026, 10, 5, 14, 5, 0, 0, time.UTC),
			[]string{"backup-20261005T130500Z.tar.gz"},
			1,
		},
		{
			"a pre-restore copy is not a backup", time.Date(2026, 10, 5, 14, 5, 0, 0, time.UTC),
			[]string{"pre-restore-20261005T140000Z.tar.gz", "backup-20261005T130500Z.tar.gz"},
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

// A scheduled backup is skipped when nothing was committed since the last this server made, and
// asked for once something was; one made before the server started says nothing of what changed
// since (D305 §2).
func TestQueueScheduled_SkipsABackupWhenNothingChangedSinceTheLast(t *testing.T) {
	t.Parallel()
	service, queue, _ := scheduleBench(t, time.Date(2026, 10, 5, 14, 5, 0, 0, time.UTC),
		"backup-20261005T130500Z.tar.gz")
	commits := int64(7)
	service.commits = func() int64 { return commits }
	if asked, err := service.QueueScheduled(t.Context()); err != nil || asked != 1 {
		t.Errorf("no backup made since the server started: asked for %d (%v); want one", asked, err)
	}
	service.commitsAtBackup, service.hasBackedUp = 7, true
	if asked, err := service.QueueScheduled(t.Context()); err != nil || asked != 0 {
		t.Errorf("nothing changed since the last: asked for %d (%v); want none", asked, err)
	}
	commits = 8
	if asked, err := service.QueueScheduled(t.Context()); err != nil || asked != 1 || len(queue.requests) != 2 {
		t.Errorf("a commit since the last: asked for %d (%v); want one", asked, err)
	}
}

// Pruning thins forty days of hourly backups to the newest backups_kept_hourly, then the newest of
// each day back to backups_kept_daily days, and of each week back to backups_kept_weekly weeks, and
// never prunes the copies kept before a restore or a migration (D305 §2).
func TestPrune_ThinsToTheNewestHourlyThenOneADayThenOneAWeek(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 16, 12, 30, 0, 0, time.UTC) // a Friday
	var files []string
	for hour := range 40 * 24 {
		files = append(files, "backup-"+now.Add(-time.Duration(hour)*time.Hour).Format(timestampLayout)+archiveSuffix)
	}
	files = append(files, "pre-restore-20261001T050000Z.tar.gz", "pre-migrate-v31.sqlite")
	service, _, directories := scheduleBenchWith(t, startingSettings{hourly: 24, daily: 7, weekly: 4}, now, files...)
	if err := service.prune(t.Context()); err != nil {
		t.Fatal(err)
	}
	listed, err := directories.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, file := range listed {
		kept = append(kept, file.Name)
	}
	slices.Sort(kept)
	backups := slices.DeleteFunc(slices.Clone(kept), func(name string) bool {
		return !strings.HasPrefix(name, "backup-")
	})
	// The 24 newest, which reach back into yesterday; the newest of each of the five days of the
	// seven before those two; the newest of each of the two weeks of the four before the two the
	// daily ones reach.
	want := []string{
		"20260927T233000Z", "20261004T233000Z", "20261010T233000Z", "20261011T233000Z",
		"20261012T233000Z", "20261013T233000Z", "20261014T233000Z",
	}
	for hour := range 24 {
		want = append(want, now.Add(-time.Duration(hour)*time.Hour).Format(timestampLayout))
	}
	for index := range want {
		want[index] = "backup-" + want[index] + archiveSuffix
	}
	slices.Sort(want)
	if !slices.Equal(backups, want) {
		t.Errorf("kept %v; want %v: 24 hourly, then 5 days' and 2 weeks' newest", backups, want)
	}
	for _, copied := range []string{"pre-restore-20261001T050000Z.tar.gz", "pre-migrate-v31.sqlite"} {
		if !slices.Contains(kept, copied) {
			t.Errorf("pruning removed %s, a copy kept before a restore or a migration", copied)
		}
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
