// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/node/confine"
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
	verdict     verdict
	description string
	remedy      string
}

// commandDoctor reports what is wrong and how to fix it. It names secrets, never their values
// (D194 §5), and exits 1 if anything failed.
func commandDoctor(ctx context.Context, call invocation) int {
	config, configReport, err := call.loadConfigFromFlags("doctor", nil)
	if err != nil {
		return call.fail(err)
	}
	logger := slog.New(slog.NewTextHandler(call.stderr, nil))
	findings := append(configFindings(configReport), diagnose(ctx, config, logger, clock.System{}.Now())...)
	status := exitSuccess
	for _, doctorFinding := range findings {
		line := fmt.Sprintf("%-4s  %s", doctorFinding.verdict, doctorFinding.description)
		if doctorFinding.remedy != "" {
			line += " — " + doctorFinding.remedy
		}
		if write(call.stdout, line+"\n", exitSuccess) != exitSuccess {
			return exitWriteFailure
		}
		if doctorFinding.verdict == verdictFailure {
			status = exitFailure
		}
	}
	return status
}

// configFindings lists what the config file lacks and what it has that nothing reads (D202).
func configFindings(report ConfigReport) []finding {
	findings := []finding{}
	for _, key := range report.OnStartingValue {
		findings = append(findings, finding{
			verdictWarning,
			fmt.Sprintf("config: %s is not in the file; the server runs on its starting value, %s",
				key.path(), key.startingValue),
			"add it to the file to make the value a choice rather than a starting value",
		})
	}
	for _, key := range report.Unknown {
		findings = append(findings, finding{
			verdictWarning, "config: " + key + " is in the file, but nothing reads it",
			"a typo, or a key a release removed; delete it if it is not a typo",
		})
	}
	if len(findings) == 0 {
		findings = append(findings, finding{verdictOK, "config: every key is in the file, and nothing else", ""})
	}
	return findings
}

// diagnose runs every check that needs only the state directory, and probes what this machine can
// confine FFmpeg with.
func diagnose(ctx context.Context, config Config, logger *slog.Logger, now time.Time) []finding {
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return []finding{{verdictFailure, "state directory " + config.Server.DataDir, err.Error()}}
	}
	findings := []finding{checkWritable(directories, config.Server.DataDir)}
	key, keyFinding := checkSecretKey(directories)
	findings = append(findings, keyFinding)
	databaseFindings, oldBackupAge := checkDatabase(ctx, directories, key, config.openOptions(), logger)
	findings = append(findings, databaseFindings...)
	findings = append(findings, checkBackups(directories, now, oldBackupAge))
	return append(findings, confinementFindings(confine.MechanismStates())...)
}

// confinementFindings are what this machine can confine FFmpeg with, as a node here would report it
// (D112): each mechanism in force, or not, with what would put it there.
func confinementFindings(states []confine.State) []finding {
	findings := make([]finding, 0, len(states))
	for _, state := range states {
		if state.IsInForce {
			what := "FFmpeg confinement: " + state.Mechanism + " is in force"
			findings = append(findings, finding{verdictOK, what, ""})
			continue
		}
		what := "FFmpeg confinement: " + state.Mechanism + " is not in force: " + state.Reason
		findings = append(findings, finding{verdictWarning, what, state.Remedy})
	}
	return findings
}

func checkWritable(directories *datadir.Directories, path string) finding {
	const probeFileName = "doctor-probe"
	err := errors.Join(
		directories.WriteStateFile(probeFileName, []byte("probe")), directories.RemoveStateFile(probeFileName))
	if err != nil {
		return finding{verdictFailure, "state directory " + path + " is not writable", err.Error()}
	}
	return finding{verdictOK, "state directory " + path + " is writable", ""}
}

func checkSecretKey(directories *datadir.Directories) (*secrets.Key, finding) {
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
func checkDatabase(
	ctx context.Context, directories *datadir.Directories, key *secrets.Key, options store.OpenOptions,
	logger *slog.Logger,
) (
	[]finding, *time.Duration,
) {
	exists, err := directories.LibraryDatabaseExists()
	if err != nil {
		return []finding{{verdictFailure, "library.db", err.Error()}}, nil
	}
	if !exists {
		return []finding{{verdictWarning, "there is no library.db yet", "start the server with `orpheum run`"}}, nil
	}
	database, err := store.Open(ctx, directories.LibraryDatabasePath(), options)
	if err != nil {
		return []finding{{verdictFailure, "library.db does not open", err.Error()}}, nil
	}
	defer func() { _ = database.Close() }() //nolint:errcheck // doctor reports; a close error changes no finding
	findings := []finding{checkSchema(ctx, database)}
	settingsService := settings.NewService(database, clock.System{}, logger)
	count, err := auth.NewService(database, clock.System{}, rand.Reader, settingsService).CountUsers(ctx)
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
	if key != nil {
		findings = append(findings, checkSecrets(ctx, database, *key, settingsService))
	}
	findings = append(findings, unusedFindings(ctx, settingsService)...)
	backupAgeWarning, err := settingsService.Duration(ctx, settings.BackupAgeWarningH)
	if err != nil {
		findings = append(findings, finding{
			verdictWarning, "the settings cannot be read: " + err.Error(),
			"start the server once; it writes every setting's starting value",
		})
		return findings, nil
	}
	return findings, &backupAgeWarning
}

func checkSchema(ctx context.Context, database *store.Database) finding {
	state, err := database.State(ctx)
	if err != nil {
		return finding{verdictFailure, "the schema version cannot be read", err.Error()}
	}
	binaryVersion, err := store.SchemaVersion()
	if err != nil {
		return finding{verdictFailure, "this binary's schema version", err.Error()}
	}
	switch {
	case state.Version == binaryVersion:
		return finding{verdictOK, fmt.Sprintf("library.db opens, pragmas as set, at schema %d", state.Version), ""}
	case store.RefuseBeforeBaseline(state) != nil:
		return finding{
			verdictFailure, fmt.Sprintf("library.db is at schema %d, below the baseline this release "+
				"starts from; no release migrates it (D228)", state.Version),
			"move it aside and start with an empty one; in development, `make dev-reset`",
		}
	case state.Version < binaryVersion:
		return finding{verdictWarning, fmt.Sprintf("library.db is at schema %d; this release migrates it to %d",
			state.Version, binaryVersion), "start the server, or run `orpheum migrate`"}
	case state.UsableBy(binaryVersion):
		return finding{verdictOK, fmt.Sprintf("library.db is at schema %d, from a newer release that left it "+
			"usable by this one (%d)", state.Version, binaryVersion), ""}
	}
	return finding{
		verdictFailure, fmt.Sprintf("library.db is at schema %d and needs a newer release than this "+
			"one (%d)", state.Version, binaryVersion),
		"restore the pre-migrate snapshot in the backup directory, or upgrade",
	}
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
		names = append(names, string(status.Name))
	}
	return finding{
		verdictWarning, "these secrets need re-entering: " + strings.Join(names, ", "),
		"they were sealed under another secret.key; set them again in settings",
	}
}

func checkBackups(directories *datadir.Directories, now time.Time, oldBackupAge *time.Duration) finding {
	files, err := directories.ListBackups()
	if err != nil {
		return finding{verdictFailure, "the backup directory cannot be read", err.Error()}
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name, "backup-") && strings.HasSuffix(file.Name, ".sqlite") {
			age := now.Sub(file.ModifiedAt).Round(time.Hour)
			if oldBackupAge == nil {
				return finding{verdictOK, fmt.Sprintf("the newest backup, %s, is %s old; not judged without the "+
					"backup_age_warning_h setting", file.Name, age), ""}
			}
			if age > *oldBackupAge {
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

// unusedFindings lists the setting rows this release does not read, and says why each is unused.
func unusedFindings(ctx context.Context, settingsService *settings.Service) []finding {
	unused, err := settingsService.Unused(ctx)
	if err != nil {
		return []finding{{verdictFailure, "the setting rows cannot be listed", err.Error()}}
	}
	findings := []finding{}
	for _, row := range unused {
		why := "no release has used this key: probably a typo"
		if row.Retired != nil {
			why = fmt.Sprintf("retired in %s: %s", row.Retired.Release, row.Retired.Reason)
		}
		findings = append(findings, finding{
			verdictWarning, "setting " + row.String() + " is unused; " + why,
			"delete it with DELETE /api/v1/config/unused/" + row.Key + " if it is not needed",
		})
	}
	return findings
}
