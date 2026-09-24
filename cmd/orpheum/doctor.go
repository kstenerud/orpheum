// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// verdict is one doctor finding's weight.
type verdict string

const (
	verdictOK      verdict = "ok"
	verdictWarning verdict = "warn"
	verdictFailure verdict = "FAIL"
)

// finding is one line of doctor's report: what was checked, and for anything wrong, what to do.
type finding struct {
	verdict verdict
	what    string
	remedy  string
}

// commandDoctor reports what is wrong and how to fix it. It names secrets, never their values
// (D194 §5), and exits 1 if anything failed.
func commandDoctor(ctx context.Context, call invocation) int {
	config, err := call.configFlags("doctor", nil)
	if err != nil {
		return call.fail(err)
	}
	findings := diagnose(ctx, config, clock.System{}.Now())
	status := exitSuccess
	for _, found := range findings {
		line := fmt.Sprintf("%-4s  %s", found.verdict, found.what)
		if found.remedy != "" {
			line += " — " + found.remedy
		}
		if write(call.stdout, line+"\n", exitSuccess) != exitSuccess {
			return exitWriteFailure
		}
		if found.verdict == verdictFailure {
			status = exitFailure
		}
	}
	return status
}

// diagnose runs every check that needs only the state directory.
func diagnose(ctx context.Context, config Config, now time.Time) []finding {
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return []finding{{verdictFailure, "state directory " + config.Server.DataDir, err.Error()}}
	}
	findings := []finding{checkWritable(directories, config.Server.DataDir)}
	key, keyFinding := checkKey(directories)
	findings = append(findings, keyFinding)
	databaseFindings, backupAgeWarning := checkDatabase(ctx, directories, key)
	findings = append(findings, databaseFindings...)
	findings = append(findings, checkBackups(directories, now, backupAgeWarning),
		finding{verdictOK, "media node: not checked yet; the node arrives in M2", ""})
	return findings
}

func checkWritable(directories *datadir.Directories, path string) finding {
	const probe = "doctor-probe"
	err := errors.Join(directories.WriteStateFile(probe, []byte("probe")), directories.RemoveStateFile(probe))
	if err != nil {
		return finding{verdictFailure, "state directory " + path + " is not writable", err.Error()}
	}
	return finding{verdictOK, "state directory " + path + " is writable", ""}
}

func checkKey(directories *datadir.Directories) (*secrets.Key, finding) {
	contents, err := directories.ReadStateFile(secrets.KeyFileName)
	if errors.Is(err, datadir.ErrNotFound) {
		return nil, finding{
			verdictWarning, "secret.key is missing",
			"the server writes a new one at start; secrets sealed under the old one will need re-entering",
		}
	}
	if err != nil {
		return nil, finding{verdictFailure, "secret.key cannot be read", err.Error()}
	}
	key, err := secrets.ParseKeyFile(contents)
	if err != nil {
		return nil, finding{verdictFailure, "secret.key is damaged", err.Error()}
	}
	return &key, finding{verdictOK, "secret.key is present", ""}
}

// checkDatabase checks library.db and returns, from its settings, the age at which a backup is
// old; nil when the settings cannot be read.
func checkDatabase(ctx context.Context, directories *datadir.Directories, key *secrets.Key) (
	[]finding, *time.Duration,
) {
	exists, err := directories.LibraryDatabaseExists()
	if err != nil {
		return []finding{{verdictFailure, "library.db", err.Error()}}, nil
	}
	if !exists {
		return []finding{{verdictWarning, "there is no library.db yet", "start the server with `orpheum run`"}}, nil
	}
	database, err := store.Open(ctx, directories.LibraryDatabase())
	if err != nil {
		return []finding{{verdictFailure, "library.db does not open", err.Error()}}, nil
	}
	defer func() { _ = database.Close() }() //nolint:errcheck // doctor reports; a close error changes no finding
	findings := []finding{checkSchema(ctx, database)}
	count, err := database.Read().CountLiveUsers(ctx)
	switch {
	case err != nil:
		findings = append(findings, finding{verdictFailure, "users cannot be counted", err.Error()})
	case count == 0:
		findings = append(findings, finding{
			verdictWarning, "setup is not complete: there is no admin yet",
			"create one with the code `orpheum setup-code` prints",
		})
	default:
		findings = append(findings, finding{verdictOK, fmt.Sprintf("setup is complete; %d users", count), ""})
	}
	settingsService := settings.NewService(database, clock.System{})
	if key != nil {
		findings = append(findings, checkSecrets(ctx, database, *key, settingsService))
	}
	hours, err := settingsService.Int(ctx, settings.BackupAgeWarningH)
	if err != nil {
		findings = append(findings, finding{
			verdictWarning, "the settings cannot be read: " + err.Error(),
			"start the server once; it writes every setting's starting value",
		})
		return findings, nil
	}
	return findings, new(time.Duration(hours) * time.Hour)
}

func checkSchema(ctx context.Context, database *store.Database) finding {
	state, err := database.State(ctx)
	if err != nil {
		return finding{verdictFailure, "the schema version cannot be read", err.Error()}
	}
	binary, err := store.SchemaVersion()
	if err != nil {
		return finding{verdictFailure, "this binary's schema version", err.Error()}
	}
	switch {
	case state.Version == binary:
		return finding{verdictOK, fmt.Sprintf("library.db opens, pragmas as set, at schema %d", state.Version), ""}
	case state.Version < binary:
		return finding{verdictWarning, fmt.Sprintf("library.db is at schema %d; this release migrates it to %d",
			state.Version, binary), "start the server, or run `orpheum migrate`"}
	case state.UsableBy(binary):
		return finding{verdictOK, fmt.Sprintf("library.db is at schema %d, from a newer release that left it "+
			"usable by this one (%d)", state.Version, binary), ""}
	}
	return finding{verdictFailure, fmt.Sprintf("library.db is at schema %d and needs a newer release than this "+
		"one (%d)", state.Version, binary), "restore the pre-migrate snapshot in the backup directory, or upgrade"}
}

func checkSecrets(
	ctx context.Context, database *store.Database, key secrets.Key, settingsService *settings.Service,
) finding {
	needing, err := secrets.NewStore(database, key, clock.System{}, rand.Reader, settingsService).NeedingReentry(ctx)
	if err != nil {
		return finding{verdictFailure, "sealed secrets cannot be listed", err.Error()}
	}
	if len(needing) == 0 {
		return finding{verdictOK, "every sealed secret opens with this secret.key", ""}
	}
	var names []string
	for _, status := range needing {
		name := string(status.Name)
		if status.UserID != nil {
			name += fmt.Sprintf(" (user %d)", *status.UserID)
		}
		names = append(names, name)
	}
	return finding{
		verdictWarning, "these secrets need re-entering: " + strings.Join(names, ", "),
		"they were sealed under another secret.key; set them again in settings",
	}
}

func checkBackups(directories *datadir.Directories, now time.Time, ageWarning *time.Duration) finding {
	files, err := directories.ListBackups()
	if err != nil {
		return finding{verdictFailure, "the backup directory cannot be read", err.Error()}
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name, "backup-") && strings.HasSuffix(file.Name, ".sqlite") {
			age := now.Sub(file.ModifiedAt).Round(time.Hour)
			if ageWarning == nil {
				return finding{verdictOK, fmt.Sprintf("the newest backup, %s, is %s old; not judged without the "+
					"backup_age_warning_h setting", file.Name, age), ""}
			}
			if age > *ageWarning {
				return finding{
					verdictWarning, fmt.Sprintf("the newest backup, %s, is %s old", file.Name, age),
					"run `orpheum backup`",
				}
			}
			return finding{verdictOK, fmt.Sprintf("the newest backup is %s", file.Name), ""}
		}
	}
	return finding{
		verdictWarning, "there is no backup yet in " + directories.BackupDirectory(),
		"run `orpheum backup`; a backup on the same disk does not survive that disk",
	}
}
