// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// verdictWord is how the command prints a verdict: a failure in capitals, so it stands out among
// the lines of a terminal.
func verdictWord(verdict api.Verdict) string {
	if verdict == api.VerdictFailure {
		return strings.ToUpper(string(verdict))
	}
	return string(verdict)
}

// finding is one line of doctor's report: what was checked, and for anything wrong, what to do.
type finding struct {
	verdict     api.Verdict
	description string
	remedy      string
}

// commandDoctor reports what is wrong and how to fix it. It names secrets, never their values
// (D194 §5), and exits 1 if anything failed.
func commandDoctor(ctx context.Context, call invocation) int {
	config, configReport, err := call.loadConfigFromFlags("doctor", kindCore, nil)
	if err != nil {
		return call.fail(err)
	}
	logger := slog.New(slog.NewTextHandler(call.stderr, nil))
	findings := append(configFindings(configReport), diagnose(ctx, config, logger, clock.System{}.Now())...)
	status := exitSuccess
	for _, doctorFinding := range findings {
		line := fmt.Sprintf("%-4s  %s", verdictWord(doctorFinding.verdict), doctorFinding.description)
		if doctorFinding.remedy != "" {
			line += " — " + doctorFinding.remedy
		}
		if write(call.stdout, line+"\n", exitSuccess) != exitSuccess {
			return exitWriteFailure
		}
		if doctorFinding.verdict == api.VerdictFailure {
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
			api.VerdictWarning,
			fmt.Sprintf("config: %s is not in the file; the server runs on its starting value, %s",
				key.path(), key.startingValue),
			"add it to the file to make the value a choice rather than a starting value",
		})
	}
	for _, key := range report.Unknown {
		findings = append(findings, finding{
			api.VerdictWarning, "config: " + key + " is in the file, but nothing reads it",
			"a typo, or a key a release removed; delete it if it is not a typo",
		})
	}
	if len(findings) == 0 {
		findings = append(findings, finding{api.VerdictOK, "config: every key is in the file, and nothing else", ""})
	}
	return findings
}

// diagnose runs every check that needs only the instance's directories, and probes what this
// machine can confine FFmpeg with.
func diagnose(ctx context.Context, config Config, logger *slog.Logger, now time.Time) []finding {
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		return []finding{{api.VerdictFailure, "the instance's directories", err.Error()}}
	}
	findings := []finding{checkWritable(directories, config.Instance.PersistentDir)}
	key, keyFinding := checkSecretKey(directories)
	findings = append(findings, keyFinding)
	databaseFindings, oldBackupAge := checkDatabase(ctx, directories, key, config.openOptions(), logger)
	findings = append(findings, databaseFindings...)
	findings = append(findings, checkBackups(directories, now, oldBackupAge))
	return append(findings, confinementOfThisMachine(confine.MechanismStates()))
}

// confinementOfThisMachine is what this machine can confine FFmpeg with, as a node here would
// report it in its hello, judged by the one check the server's System page makes of each node
// (api.ConfinementFinding, D112).
func confinementOfThisMachine(states []confine.State) finding {
	judged := api.ConfinementFinding("this machine", confinementReport(states))
	checked := finding{api.VerdictOK, judged.Check + ": " + judged.Detail, judged.Action}
	if judged.Verdict != api.VerdictOK {
		checked.verdict = api.VerdictWarning
	}
	return checked
}

func checkWritable(directories *instancedir.Directories, path string) finding {
	const probeFileName = "doctor-probe"
	err := errors.Join(
		directories.WritePersistentFile(probeFileName, []byte("probe")),
		directories.RemovePersistentFile(probeFileName))
	if err != nil {
		return finding{api.VerdictFailure, "persistent directory " + path + " is not writable", err.Error()}
	}
	return finding{api.VerdictOK, "persistent directory " + path + " is writable", ""}
}

func checkSecretKey(directories *instancedir.Directories) (*secrets.Key, finding) {
	contents, err := directories.ReadPersistentFile(secrets.KeyFileName)
	if errors.Is(err, instancedir.ErrNotFound) {
		return nil, finding{
			api.VerdictWarning, "secret.key is missing",
			"the server writes a new one at start; secrets sealed under the old one will need re-entering",
		}
	}
	if err != nil {
		return nil, finding{api.VerdictFailure, "secret.key cannot be read", err.Error()}
	}
	key, err := secrets.ParseKeyFile(contents)
	if err != nil {
		return nil, finding{api.VerdictFailure, "secret.key is damaged", err.Error()}
	}
	return &key, finding{api.VerdictOK, "secret.key is present", ""}
}

// checkDatabase checks persistent.db and returns, from its settings, the age at which a backup is
// old; nil when the settings cannot be read.
func checkDatabase(
	ctx context.Context, directories *instancedir.Directories, key *secrets.Key, options store.OpenOptions,
	logger *slog.Logger,
) (
	[]finding, *time.Duration,
) {
	exists, err := directories.PersistentDatabaseExists()
	if err != nil {
		return []finding{{api.VerdictFailure, "persistent.db", err.Error()}}, nil
	}
	if !exists {
		return []finding{{
			api.VerdictWarning, "there is no persistent.db yet", "start the server with `orpheum core`",
		}}, nil
	}
	database, err := store.Open(ctx, directories.PersistentDatabasePath(), options)
	if err != nil {
		return []finding{{api.VerdictFailure, "persistent.db does not open", err.Error()}}, nil
	}
	defer func() { _ = database.Close() }() //nolint:errcheck // doctor reports; a close error changes no finding
	findings := []finding{checkSchema(ctx, database)}
	settingsService := settings.NewService(database, clock.System{}, logger)
	count, err := auth.NewService(database, clock.System{}, rand.Reader, settingsService).CountUsers(ctx)
	switch {
	case err != nil:
		findings = append(findings, finding{api.VerdictFailure, "users cannot be counted", err.Error()})
	case count == 0:
		findings = append(findings, finding{
			api.VerdictWarning, "setup is not complete: there is no admin yet",
			"create one with the code `orpheum setup-code` prints",
		})
	default:
		findings = append(findings, finding{api.VerdictOK, fmt.Sprintf("setup is complete; %d users", count), ""})
	}
	if key != nil {
		findings = append(findings, checkSecrets(ctx, database, *key, settingsService))
	}
	findings = append(findings, unusedFindings(ctx, settingsService)...)
	backupAgeWarning, err := settingsService.Duration(ctx, settings.BackupAgeWarningHours)
	if err != nil {
		findings = append(findings, finding{
			api.VerdictWarning, "the settings cannot be read: " + err.Error(),
			"start the server once; it writes every setting's starting value",
		})
		return findings, nil
	}
	return findings, &backupAgeWarning
}

func checkSchema(ctx context.Context, database *store.Database) finding {
	state, err := database.State(ctx)
	if err != nil {
		return finding{api.VerdictFailure, "the schema version cannot be read", err.Error()}
	}
	binaryVersion, err := store.SchemaVersion()
	if err != nil {
		return finding{api.VerdictFailure, "this binary's schema version", err.Error()}
	}
	switch {
	case state.Version == binaryVersion:
		return finding{
			api.VerdictOK, fmt.Sprintf("persistent.db opens, pragmas as set, at schema %d", state.Version), "",
		}
	case store.RefuseBeforeBaseline(state) != nil:
		return finding{
			api.VerdictFailure, fmt.Sprintf("persistent.db is at schema %d, below the baseline this release "+
				"starts from; no release migrates it (D228)", state.Version),
			"move it aside and start with an empty one; in development, `make dev-reset`",
		}
	case state.Version < binaryVersion:
		return finding{api.VerdictWarning, fmt.Sprintf("persistent.db is at schema %d; this release migrates it to %d",
			state.Version, binaryVersion), "start the server, or run `orpheum migrate`"}
	case state.UsableBy(binaryVersion):
		return finding{api.VerdictOK, fmt.Sprintf("persistent.db is at schema %d, from a newer release that left it "+
			"usable by this one (%d)", state.Version, binaryVersion), ""}
	}
	return finding{
		api.VerdictFailure, fmt.Sprintf("persistent.db is at schema %d and needs a newer release than this "+
			"one (%d)", state.Version, binaryVersion),
		"restore the pre-migrate snapshot in the backups directory with `orpheum restore --from`, or upgrade",
	}
}

func checkSecrets(
	ctx context.Context, database *store.Database, key secrets.Key, settingsService *settings.Service,
) finding {
	needing, err := secrets.NewStore(database, key, clock.System{}, rand.Reader, settingsService).NeedingReentry(ctx)
	if err != nil {
		return finding{api.VerdictFailure, "sealed secrets cannot be listed", err.Error()}
	}
	if len(needing) == 0 {
		return finding{api.VerdictOK, "every sealed secret opens with this secret.key", ""}
	}
	var names []string
	for _, status := range needing {
		names = append(names, string(status.Name))
	}
	return finding{
		api.VerdictWarning, "these secrets need re-entering: " + strings.Join(names, ", "),
		"they were sealed under another secret.key; set them again in settings",
	}
}

func checkBackups(directories *instancedir.Directories, now time.Time, oldBackupAge *time.Duration) finding {
	files, err := directories.ListBackups()
	if err != nil {
		return finding{api.VerdictFailure, "the backup directory cannot be read", err.Error()}
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name, "backup-") && strings.HasSuffix(file.Name, ".tar.gz") {
			age := now.Sub(file.ModifiedAt).Round(time.Hour)
			if oldBackupAge == nil {
				return finding{api.VerdictOK, fmt.Sprintf("the newest backup, %s, is %s old; not judged without the "+
					"backup_age_warning_hours setting", file.Name, age), ""}
			}
			if age > *oldBackupAge {
				return finding{
					api.VerdictWarning, fmt.Sprintf("the newest backup, %s, is %s old", file.Name, age),
					"run `orpheum backup`",
				}
			}
			return finding{api.VerdictOK, fmt.Sprintf("the newest backup is %s", file.Name), ""}
		}
	}
	return finding{
		api.VerdictWarning, "there is no backup yet in " + directories.BackupDirectory(),
		"run `orpheum backup`; a backup on the same disk does not survive that disk",
	}
}

// unusedFindings lists the setting rows this release does not read, and says why each is unused.
func unusedFindings(ctx context.Context, settingsService *settings.Service) []finding {
	unused, err := settingsService.Unused(ctx)
	if err != nil {
		return []finding{{api.VerdictFailure, "the setting rows cannot be listed", err.Error()}}
	}
	findings := []finding{}
	for _, row := range unused {
		why := "no release has used this key: probably a typo"
		if row.Retired != nil {
			why = fmt.Sprintf("retired in %s: %s", row.Retired.Release, row.Retired.Reason)
		}
		findings = append(findings, finding{
			api.VerdictWarning, "setting " + row.String() + " is unused; " + why,
			"if it is not needed, delete it on the System page, under \"Settings this release does not read\", " +
				"or with DELETE /api/v1/config/unused/" + row.Key,
		})
	}
	return findings
}
