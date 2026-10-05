// SPDX-License-Identifier: GPL-3.0-or-later

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
	"github.com/kstenerud/orpheum/internal/platform/localtime"
)

// Queue is where backups are asked for: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, request jobs.Request) (bool, error)
}

// Settings are the backup settings the service reads when it needs them.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
	// Zone is the server's time zone, which backup_hour_local_tz is in.
	Zone(ctx context.Context) (localtime.Zone, error)
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
	if err = json.Unmarshal([]byte(first), &firstArgs); err != nil {
		return firstArgs, secondArgs, fmt.Errorf("read a backup job's arguments: %w", err)
	}
	if err = json.Unmarshal([]byte(second), &secondArgs); err != nil {
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

// QueueScheduled asks for the scheduled backup when it is due, and says how many it asked for:
// one when the newest backup is backup_every_days old and it is backup_hour_local_tz in the server's time
// zone, or when the newest is a day overdue, whatever the hour; none otherwise (D246 §7). A sweep
// calls it.
func (service *Service) QueueScheduled(ctx context.Context) (int, error) {
	every, err := service.settings.Duration(ctx, settings.BackupEveryDays)
	if err != nil {
		return 0, fmt.Errorf("read backup_every_days: %w", err)
	}
	hour, err := service.settings.Int(ctx, settings.BackupHourLocalTZ)
	if err != nil {
		return 0, fmt.Errorf("read backup_hour_local_tz: %w", err)
	}
	zone, err := service.settings.Zone(ctx)
	if err != nil {
		return 0, err
	}
	files, err := service.directories.ListBackups()
	if err != nil {
		return 0, err
	}
	now := service.clock.Now()
	newest, isAny := newestBackup(files)
	// A backup is due an hour early, so one made at the quiet hour is due again at the next.
	isDue := !isAny || now.Sub(newest) >= every-time.Hour
	isOverdue := !isAny || now.Sub(newest) >= every+24*time.Hour
	if !isDue || (int64(zone.Of(now).Hour()) != hour && !isOverdue) {
		return 0, nil
	}
	if enqueueErr := service.enqueue(ctx, JobArgs{IsScheduled: true}, jobs.PriorityBackground,
		"the scheduled backup is due"); enqueueErr != nil {
		return 0, enqueueErr
	}
	return 1, nil
}

// HandleBackup makes the backup a job asked for, then prunes the backups past backups_kept.
func (service *Service) HandleBackup(ctx context.Context, job jobs.Job) error {
	var args JobArgs
	if err := json.Unmarshal([]byte(job.ArgsJSON), &args); err != nil {
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

// prune removes the backups past the newest backups_kept; the copies kept before a restore or a
// migration are not backups the server made, and are never pruned (D243 §11).
func (service *Service) prune(ctx context.Context) error {
	kept, err := service.settings.Int(ctx, settings.BackupsKept)
	if err != nil {
		return fmt.Errorf("read backups_kept: %w", err)
	}
	files, err := service.directories.ListBackups()
	if err != nil {
		return err
	}
	names := backupNames(files)
	slices.Sort(names) // by when each was made, as its name says
	for len(names) > int(kept) {
		if removeErr := service.directories.RemoveBackupFile(names[0]); removeErr != nil {
			return removeErr
		}
		names = names[1:]
	}
	return nil
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
		stem := strings.TrimSuffix(strings.TrimPrefix(name, backupStemPrefix), archiveSuffix)
		if madeAt, err := time.Parse(timestampLayout, stem); err == nil && madeAt.After(newest) {
			newest = madeAt
		}
	}
	return newest, !newest.IsZero()
}

// argsJSON is a backup job's arguments as the queue keeps them.
func argsJSON(args JobArgs) string {
	encoded, err := json.Marshal(args)
	if err != nil {
		return "{}" // two booleans always encode
	}
	return string(encoded)
}
