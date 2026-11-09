// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// Queue is where backups are asked for: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, request jobs.Request) (bool, error)
}

// Settings are the backup settings the service reads when it needs them.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// JobArgs are a backup job's arguments: whether it holds secret.key, which a person asks for and
// a scheduled backup never does, and whether the schedule asked for it (D243 §11).
type JobArgs struct {
	IncludesKey bool `json:"includesKey,omitempty"`
	IsScheduled bool `json:"isScheduled,omitempty"`
}

// JobArgsMerge is how a repeat request for a backup folds into the one queued: it holds the key
// when either asked for it, which only a person does, and is scheduled only when both were. A
// backup running covers a request unless the request asks for the key and the running one does not
// hold it.
func JobArgsMerge() jobs.ArgsMerge {
	return jobs.ArgsMerge{
		Merge: func(queued, requested string) (string, error) {
			queuedArgs, requestedArgs, err := readArgsPair(queued, requested)
			if err != nil {
				return "", err
			}
			return argsJSON(JobArgs{
				IncludesKey: queuedArgs.IncludesKey || requestedArgs.IncludesKey,
				IsScheduled: queuedArgs.IsScheduled && requestedArgs.IsScheduled,
			}), nil
		},
		Covers: func(running, requested string) (bool, error) {
			runningArgs, requestedArgs, err := readArgsPair(running, requested)
			return runningArgs.IncludesKey || !requestedArgs.IncludesKey, err
		},
	}
}

// readArgsPair reads two backup jobs' arguments.
func readArgsPair(first, second string) (firstArgs, secondArgs JobArgs, err error) {
	if err = strictjson.Unmarshal([]byte(first), &firstArgs); err != nil {
		return firstArgs, secondArgs, fmt.Errorf("read a backup job's arguments: %w", err)
	}
	if err = strictjson.Unmarshal([]byte(second), &secondArgs); err != nil {
		return firstArgs, secondArgs, fmt.Errorf("read a backup request's arguments: %w", err)
	}
	return firstArgs, secondArgs, nil
}

// UseQueue makes the service ask for backups as jobs on queue, reading its schedule from config.
// Set before the server answers requests.
func (service *Service) UseQueue(queue Queue, config Settings) {
	service.queue, service.settings = queue, config
}

// Request asks for a backup now, as a job the job view shows (D243 §11): a person's, at their
// priority, holding secret.key when includesKey says so. One that would hold a key the instance
// does not have is ErrNoKeyToInclude at once.
func (service *Service) Request(ctx context.Context, includesKey bool) error {
	if includesKey {
		if _, err := service.directories.ReadPersistentFile(secrets.KeyFileName); errors.Is(err,
			instancedir.ErrNotFound) {
			return ErrNoKeyToInclude
		} else if err != nil {
			return err
		}
	}
	return service.enqueue(ctx, JobArgs{IncludesKey: includesKey}, jobs.PriorityPersonsRequest,
		"an admin asked for a backup now")
}

// QueueScheduled asks for the scheduled backup when it is due, and says how many it asked for: one
// when the newest backup is backup_every_hours old and the database has changed since the last
// backup this server made, or there is none; none otherwise (D305 §2). A sweep calls it.
func (service *Service) QueueScheduled(ctx context.Context) (int, error) {
	every, err := service.settings.Duration(ctx, settings.BackupEveryHours)
	if err != nil {
		return 0, fmt.Errorf("read backup_every_hours: %w", err)
	}
	files, err := service.directories.ListBackups()
	if err != nil {
		return 0, err
	}
	newest, isAny := newestBackup(files)
	if isAny && (service.clock.Now().Sub(newest) < every || service.isUnchangedSinceLastBackup()) {
		return 0, nil
	}
	if enqueueErr := service.enqueue(ctx, JobArgs{IsScheduled: true}, jobs.PriorityBackground,
		"the scheduled backup is due"); enqueueErr != nil {
		return 0, enqueueErr
	}
	return 1, nil
}

// isUnchangedSinceLastBackup is whether nothing was committed since the last backup this server
// made; false when it has made none since it started, which a backup made then answers.
func (service *Service) isUnchangedSinceLastBackup() bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	return service.hasBackedUp && service.commits != nil && service.commits() == service.commitsAtBackup
}

// HandleBackup makes the backup a job asked for, then thins the backups (prune).
func (service *Service) HandleBackup(ctx context.Context, job jobs.Job) error {
	var args JobArgs
	if err := strictjson.Unmarshal([]byte(job.ArgsJSON), &args); err != nil {
		return jobs.MarkPermanent(fmt.Errorf("read backup job %d's arguments: %w", job.ID, err))
	}
	if _, err := service.Make(ctx, args.IncludesKey); err != nil {
		if errors.Is(err, ErrNoKeyToInclude) {
			return jobs.MarkPermanent(err)
		}
		return err
	}
	return service.prune(ctx)
}

// thinning is how many backups prune keeps: the newest hourly, then the newest of each of the
// daily days and the weekly weeks before (D305 §2).
type thinning struct {
	hourly, daily, weekly int64
}

// prune thins the backups to backups_kept_hourly of the newest, then the newest of each of the
// backups_kept_daily days and the backups_kept_weekly weeks, in UTC, its names' time; the copies kept
// before a restore or a migration are not backups the server made, and are never pruned (D305 §2).
func (service *Service) prune(ctx context.Context) error {
	var keeps thinning
	for _, read := range []struct {
		key  settings.Key
		into *int64
	}{
		{settings.BackupsKeptHourly, &keeps.hourly},
		{settings.BackupsKeptDaily, &keeps.daily},
		{settings.BackupsKeptWeekly, &keeps.weekly},
	} {
		value, err := service.settings.Int(ctx, read.key)
		if err != nil {
			return fmt.Errorf("read %s: %w", read.key, err)
		}
		*read.into = value
	}
	files, err := service.directories.ListBackups()
	if err != nil {
		return err
	}
	for _, name := range backupsPastThinning(backupNames(files), keeps) {
		if removeErr := service.directories.RemoveBackupFile(name); removeErr != nil {
			return removeErr
		}
	}
	return nil
}

// backupsPastThinning are the backups, of names, that keeps does not keep: past the newest hourly,
// all but the newest of each day back to the daily'th day with one, and of each week to the weekly'th.
func backupsPastThinning(names []string, keeps thinning) []string {
	sorted := slices.Clone(names)
	slices.SortFunc(sorted, func(first, second string) int { return strings.Compare(second, first) }) // newest first
	kept := map[string]bool{}
	for index, name := range sorted {
		if int64(index) < keeps.hourly {
			kept[name] = true
		}
	}
	keepNewestOf := func(periodOf func(time.Time) string, count int64) {
		periods := map[string]bool{}
		for _, name := range sorted {
			madeAt, isMade := madeAtOf(name)
			if !isMade {
				continue
			}
			period := periodOf(madeAt)
			if periods[period] {
				continue
			}
			if int64(len(periods)) >= count {
				return
			}
			periods[period] = true
			kept[name] = true
		}
	}
	keepNewestOf(func(madeAt time.Time) string { return madeAt.UTC().Format(time.DateOnly) }, keeps.daily)
	keepNewestOf(func(madeAt time.Time) string {
		year, week := madeAt.UTC().ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	}, keeps.weekly)
	return slices.DeleteFunc(sorted, func(name string) bool { return kept[name] })
}

// enqueue asks for a backup job.
func (service *Service) enqueue(ctx context.Context, args JobArgs, priority int64, reason string) error {
	if service.queue == nil {
		return errors.New("backups are not jobs yet: the server's queue is not set")
	}
	if _, err := service.queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindBackup, ArgsJSON: argsJSON(args), Priority: priority, Reason: reason,
	}); err != nil {
		return fmt.Errorf("queue a backup: %w", err)
	}
	return nil
}

// backupStemPrefix begins the name of every backup the server makes, which only those have.
const backupStemPrefix = "backup-"

// backupNames are the names of the backups the server made, among the backup directory's files.
func backupNames(files []instancedir.BackupFile) []string {
	var names []string
	for _, file := range files {
		if strings.HasPrefix(file.Name, backupStemPrefix) && strings.HasSuffix(file.Name, archiveSuffix) {
			names = append(names, file.Name)
		}
	}
	return names
}

// newestBackup is when the newest backup the server made was made, as its name says.
func newestBackup(files []instancedir.BackupFile) (time.Time, bool) {
	var newest time.Time
	for _, name := range backupNames(files) {
		if madeAt, isMade := madeAtOf(name); isMade && madeAt.After(newest) {
			newest = madeAt
		}
	}
	return newest, !newest.IsZero()
}

// madeAtOf is when a backup was made, as its name says.
func madeAtOf(name string) (time.Time, bool) {
	stem := strings.TrimSuffix(strings.TrimPrefix(name, backupStemPrefix), archiveSuffix)
	madeAt, err := time.Parse(timestampLayout, stem)
	return madeAt, err == nil
}

// argsJSON is a backup job's arguments as the queue keeps them.
func argsJSON(args JobArgs) string {
	encoded, err := json.Marshal(args)
	if err != nil {
		return "{}" // two booleans always encode
	}
	return string(encoded)
}
