// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/backup"
	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
)

// invocation is one subcommand's run: its arguments, the environment and its streams.
type invocation struct {
	arguments []string
	lookupEnv func(string) (string, bool)
	stdout    io.Writer
	stderr    io.Writer
}

// fail writes a failure to stderr and returns the failure status.
func (call invocation) fail(err error) int {
	return write(call.stderr, "orpheum: "+err.Error()+"\n", exitFailure)
}

// loadConfigFromFlags parses a subcommand's flags, which always include --config, and loads and
// checks the config file it names. When addFlags is set, it adds the subcommand's own flags.
func (call invocation) loadConfigFromFlags(name string, addFlags func(*flag.FlagSet)) (Config, ConfigReport, error) {
	flags := flag.NewFlagSet("orpheum "+name, flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	configPath := flags.String("config", "", "the config file (required)")
	if addFlags != nil {
		addFlags(flags)
	}
	if err := flags.Parse(call.arguments); err != nil {
		return Config{}, ConfigReport{}, err
	}
	if *configPath == "" {
		return Config{}, ConfigReport{}, errors.New("--config is required")
	}
	return loadConfig(*configPath)
}

// printConfigNotes writes the config file's report to stderr, for commands with no log.
func (call invocation) printConfigNotes(report ConfigReport) {
	notes := report.Notes()
	if len(notes) > 0 {
		write(call.stderr, strings.Join(notes, "\n")+"\n", exitSuccess)
	}
}

// logger builds the process's logger from the config (D73; standards/go.md §10).
func (call invocation) logger(config Config) (*slog.Logger, error) {
	level, err := parseLevel(config.Log.Level)
	if err != nil {
		return nil, err
	}
	format, err := slogx.ParseFormat(config.Log.Format)
	if err != nil {
		return nil, err
	}
	return slogx.New(call.stderr, format, level)
}

// openExistingDatabase opens an existing library.db, refusing to create one where there is none.
func openExistingDatabase(ctx context.Context, config Config) (*datadir.Directories, *store.Database, error) {
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return nil, nil, err
	}
	exists, err := directories.LibraryDatabaseExists()
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, fmt.Errorf("there is no database at %s yet; start the server once with `orpheum run`",
			directories.LibraryDatabasePath())
	}
	database, err := store.Open(ctx, directories.LibraryDatabasePath(), config.openOptions())
	return directories, database, err
}

// commandMigrate migrates the database without starting the server (D181).
func commandMigrate(ctx context.Context, call invocation) int {
	config, configReport, err := call.loadConfigFromFlags("migrate", nil)
	if err != nil {
		return call.fail(err)
	}
	call.printConfigNotes(configReport)
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return call.fail(err)
	}
	database, err := store.Open(ctx, directories.LibraryDatabasePath(), config.openOptions())
	if err != nil {
		return call.fail(err)
	}
	var report strings.Builder
	err = database.Migrate(ctx, config.migrateOptions(directories.BackupDirectory(), func(step, total int) {
		fmt.Fprintf(&report, "migrating: step %d of %d\n", step, total)
	}))
	state, stateErr := database.State(ctx)
	if err = errors.Join(err, stateErr, database.Close()); err != nil {
		return call.fail(err)
	}
	fmt.Fprintf(&report, "the database is at schema %d\n", state.Version)
	return write(call.stdout, report.String(), exitSuccess)
}

// commandBackup writes a backup with VACUUM INTO; --include-key writes secret.key beside it.
func commandBackup(ctx context.Context, call invocation) int {
	var shouldIncludeKey bool
	config, configReport, err := call.loadConfigFromFlags("backup", func(flags *flag.FlagSet) {
		flags.BoolVar(&shouldIncludeKey, "include-key", false,
			"also write secret.key beside the backup, for moving to a new machine")
	})
	if err != nil {
		return call.fail(err)
	}
	call.printConfigNotes(configReport)
	directories, database, err := openExistingDatabase(ctx, config)
	if err != nil {
		return call.fail(err)
	}
	written, err := backup.Write(ctx, database, directories, clock.System{}.Now(), shouldIncludeKey)
	if err = errors.Join(err, database.Close()); err != nil {
		return call.fail(err)
	}
	report := fmt.Sprintf("wrote %s (sealed secrets are ciphertext in it)\n", written.DatabasePath)
	if written.KeyPath != "" {
		report += fmt.Sprintf("wrote %s: whoever has both files has every secret; keep them apart, or safe\n",
			written.KeyPath)
	}
	return write(call.stdout, report, exitSuccess)
}

// commandRestore restores a backup, with the server stopped. Without --yes it only says what it
// would do (backup-restore.md: restore says what it will overwrite before doing it).
func commandRestore(ctx context.Context, call invocation) int {
	var backupPath, keyPath string
	var isConfirmed bool
	config, configReport, err := call.loadConfigFromFlags("restore", func(flags *flag.FlagSet) {
		flags.StringVar(&backupPath, "from", "", "the backup file to restore (required)")
		flags.StringVar(&keyPath, "key", "", "a secret.key to install with it, from `backup --include-key`")
		flags.BoolVar(&isConfirmed, "yes", false, "restore; without it, only say what would happen")
	})
	if err != nil {
		return call.fail(err)
	}
	call.printConfigNotes(configReport)
	if backupPath == "" {
		return call.fail(errors.New("--from is required"))
	}
	var keyFileContents []byte
	if keyPath != "" {
		// The operator names the key file on the command line; reading it is the point.
		if keyFileContents, err = os.ReadFile(keyPath); err != nil { //nolint:gosec // G304: see above
			return call.fail(fmt.Errorf("read the key: %w", err))
		}
	}
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return call.fail(err)
	}
	plan, err := backup.MakePlan(ctx, directories, backupPath, keyFileContents, clock.System{}.Now())
	if err != nil {
		return call.fail(err)
	}
	if !isConfirmed {
		return write(call.stdout, plan.Describe()+"Nothing was changed. Run again with --yes to restore.\n",
			exitUsageError)
	}
	if write(call.stdout, plan.Describe(), exitSuccess) != exitSuccess {
		return exitWriteFailure
	}
	if err = backup.Restore(ctx, directories, plan, keyFileContents, config.openOptions()); err != nil {
		return call.fail(err)
	}
	return write(call.stdout, "Restored. Start the server; it migrates an older backup on the way up.\n", exitSuccess)
}

// commandSetupCode prints the one-time setup code, for a server whose log has scrolled away.
func commandSetupCode(ctx context.Context, call invocation) int {
	config, configReport, err := call.loadConfigFromFlags("setup-code", nil)
	if err != nil {
		return call.fail(err)
	}
	call.printConfigNotes(configReport)
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return call.fail(err)
	}
	code, err := stateSetupCodes{directories: directories}.Read()
	if err == nil {
		return write(call.stdout, code+"\n", exitSuccess)
	}
	_, database, openErr := openExistingDatabase(ctx, config)
	if openErr != nil {
		return call.fail(openErr)
	}
	count, err := database.Read().CountLiveUsers(ctx)
	if err = errors.Join(err, database.Close()); err != nil {
		return call.fail(err)
	}
	if count > 0 {
		return write(call.stdout, "setup is complete: an admin exists; sign in instead\n", exitFailure)
	}
	return call.fail(errors.New("there is no setup code yet; start the server and it writes one"))
}

// commandOpenAPI prints the API's OpenAPI document, which `make generate` commits (D85).
func commandOpenAPI(_ context.Context, call invocation) int {
	document, err := json.MarshalIndent(api.NewServer(slog.New(slog.DiscardHandler)).OpenAPI(), "", "  ")
	if err != nil {
		return call.fail(err)
	}
	return write(call.stdout, string(document)+"\n", exitSuccess)
}
