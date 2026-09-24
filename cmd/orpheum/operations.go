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
	"github.com/kstenerud/orpheum/internal/core/settings"
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

// failAfter writes what was done so far to stdout, then the failure to stderr.
func (call invocation) failAfter(done string, err error) int {
	if write(call.stdout, done, exitSuccess) != exitSuccess {
		return exitWriteFailure
	}
	return call.fail(err)
}

// configFlags parses a subcommand's flags, which always include --config.
func (call invocation) configFlags(name string, extra func(*flag.FlagSet)) (Config, error) {
	flags := flag.NewFlagSet("orpheum "+name, flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	configPath := flags.String("config", "", "the config file (required)")
	if extra != nil {
		extra(flags)
	}
	if err := flags.Parse(call.arguments); err != nil {
		return Config{}, err
	}
	if *configPath == "" {
		return Config{}, errors.New("--config is required")
	}
	config, unknown, err := loadConfig(*configPath)
	var report strings.Builder
	for _, key := range unknown {
		fmt.Fprintf(&report, "orpheum: config key %s is not recognised and is ignored\n", key)
	}
	if write(call.stderr, report.String(), exitSuccess) != exitSuccess {
		return config, errors.Join(err, errors.New("write to stderr failed"))
	}
	return config, err
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

// openExisting opens an existing library.db, refusing to create one where there is none.
func openExisting(ctx context.Context, config Config) (*datadir.Directories, *store.Database, error) {
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
			directories.LibraryDatabase())
	}
	database, err := store.Open(ctx, directories.LibraryDatabase(), config.openOptions())
	return directories, database, err
}

// commandMigrate migrates the database without starting the server (D181).
func commandMigrate(ctx context.Context, call invocation) int {
	config, err := call.configFlags("migrate", nil)
	if err != nil {
		return call.fail(err)
	}
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return call.fail(err)
	}
	database, err := store.Open(ctx, directories.LibraryDatabase(), config.openOptions())
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
	var includeKey bool
	config, err := call.configFlags("backup", func(flags *flag.FlagSet) {
		flags.BoolVar(&includeKey, "include-key", false,
			"also write secret.key beside the backup, for moving to a new machine")
	})
	if err != nil {
		return call.fail(err)
	}
	directories, database, err := openExisting(ctx, config)
	if err != nil {
		return call.fail(err)
	}
	written, err := backup.Write(ctx, database, directories, clock.System{}.Now(), includeKey)
	if err = errors.Join(err, database.Close()); err != nil {
		return call.fail(err)
	}
	report := fmt.Sprintf("wrote %s (sealed secrets are ciphertext in it)\n", written.Database)
	if written.Key != "" {
		report += fmt.Sprintf("wrote %s: whoever has both files has every secret; keep them apart, or safe\n",
			written.Key)
	}
	return write(call.stdout, report, exitSuccess)
}

// commandRestore restores a backup, with the server stopped. Without --yes it only says what it
// would do (backup-restore.md: restore says what it will overwrite before doing it).
func commandRestore(ctx context.Context, call invocation) int {
	var source, keyPath string
	var confirmed bool
	config, err := call.configFlags("restore", func(flags *flag.FlagSet) {
		flags.StringVar(&source, "from", "", "the backup file to restore (required)")
		flags.StringVar(&keyPath, "key", "", "a secret.key to install with it, from `backup --include-key`")
		flags.BoolVar(&confirmed, "yes", false, "restore; without it, only say what would happen")
	})
	if err != nil {
		return call.fail(err)
	}
	if source == "" {
		return call.fail(errors.New("--from is required"))
	}
	var key []byte
	if keyPath != "" {
		// The operator names the key file on the command line; reading it is the point.
		if key, err = os.ReadFile(keyPath); err != nil { //nolint:gosec // G304: see above
			return call.fail(fmt.Errorf("read the key: %w", err))
		}
	}
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return call.fail(err)
	}
	plan, err := backup.MakePlan(ctx, directories, source, key, clock.System{}.Now())
	if err != nil {
		return call.fail(err)
	}
	if !confirmed {
		return write(call.stdout, plan.Describe()+"Nothing was changed. Run again with --yes to restore.\n",
			exitUsageError)
	}
	if write(call.stdout, plan.Describe(), exitSuccess) != exitSuccess {
		return exitWriteFailure
	}
	if err = backup.Restore(ctx, directories, plan, key, config.openOptions()); err != nil {
		return call.fail(err)
	}
	return write(call.stdout, "Restored. Start the server; it migrates an older backup on the way up.\n", exitSuccess)
}

// commandSetupCode prints the one-time setup code, for a server whose log has scrolled away.
func commandSetupCode(ctx context.Context, call invocation) int {
	config, err := call.configFlags("setup-code", nil)
	if err != nil {
		return call.fail(err)
	}
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return call.fail(err)
	}
	code, err := stateSetupCodes{directories: directories}.Read()
	if err == nil {
		return write(call.stdout, code+"\n", exitSuccess)
	}
	_, database, openErr := openExisting(ctx, config)
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

// commandSeedSettings fills every gap in the configuration and changes nothing else: first the
// config file, where it adds every missing key that has a starting value, then the database,
// where it writes the starting value of every setting with no row. It reports what it wrote,
// what only a person can fill, and what it found that nothing reads, which it leaves alone (D200).
// It works with the server stopped, when a missing value keeps the server from starting.
func commandSeedSettings(ctx context.Context, call invocation) int {
	flags := flag.NewFlagSet("orpheum seed-settings", flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	configPath := flags.String("config", "", "the config file (required)")
	if err := flags.Parse(call.arguments); err != nil {
		return call.fail(err)
	}
	if *configPath == "" {
		return call.fail(errors.New("--config is required"))
	}
	var report strings.Builder
	fileReport, err := seedConfigFile(*configPath, clock.System{}.Now())
	describeConfigSeed(&report, *configPath, fileReport)
	if err != nil {
		return call.failAfter(report.String(), err)
	}
	config, _, err := loadConfig(*configPath)
	if err != nil {
		return call.failAfter(report.String(), err)
	}
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return call.failAfter(report.String(), err)
	}
	exists, err := directories.LibraryDatabaseExists()
	if err != nil {
		return call.failAfter(report.String(), err)
	}
	if !exists {
		report.WriteString("settings: there is no database yet; the server writes every setting at its first start\n")
		return write(call.stdout, report.String(), exitSuccess)
	}
	_, database, err := openExisting(ctx, config)
	if err != nil {
		return call.failAfter(report.String(), err)
	}
	seeded, err := settings.NewService(database, clock.System{}).Seed(ctx)
	if err = errors.Join(err, database.Close()); err != nil {
		return call.failAfter(report.String(), err)
	}
	describeSettingsSeed(&report, seeded)
	return write(call.stdout, report.String(), exitSuccess)
}

func describeConfigSeed(report *strings.Builder, path string, seeded configSeedReport) {
	for _, key := range seeded.Written {
		fmt.Fprintf(report, "config: wrote %s = %s, its starting value (%s)\n", key.path(), key.start, key.describe)
	}
	if seeded.Backup != "" {
		fmt.Fprintf(report, "config: the file as it was is kept in %s\n", seeded.Backup)
	}
	for _, key := range seeded.NeedsHand {
		fmt.Fprintf(report, "config: %s is missing and has no starting value: it is %s; add it to %s by hand\n",
			key.path(), key.describe, path)
	}
	if len(seeded.Written) == 0 && len(seeded.NeedsHand) == 0 {
		report.WriteString("config: every key is there; nothing was written\n")
	}
	for _, key := range seeded.Unknown {
		fmt.Fprintf(report, "config: %s is in the file, but nothing reads it: a typo, or a key a release removed. "+
			"It is left alone\n", key)
	}
}

func describeSettingsSeed(report *strings.Builder, seeded settings.SeedReport) {
	for _, key := range seeded.Written {
		definition, _ := settings.Lookup(key)
		fmt.Fprintf(report, "settings: wrote %s = %s, its starting value\n", key, definition.Start)
	}
	for _, key := range seeded.Updated {
		definition, _ := settings.Lookup(key)
		fmt.Fprintf(report, "settings: moved %s to %s, the starting value this release gives it; nobody had set it\n",
			key, definition.Start)
	}
	if len(seeded.Written) == 0 && len(seeded.Updated) == 0 {
		report.WriteString("settings: every setting already has a value; nothing was written\n")
	}
	for _, unknown := range seeded.Unknown {
		fmt.Fprintf(report, "settings: %s is in the database, but nothing reads it: a typo, or a key a release "+
			"removed. It is left alone\n", unknown)
	}
}

// commandOpenAPI prints the API's OpenAPI document, which `make generate` commits (D85).
func commandOpenAPI(_ context.Context, call invocation) int {
	document, err := json.MarshalIndent(api.NewServer(slog.New(slog.DiscardHandler)).OpenAPI(), "", "  ")
	if err != nil {
		return call.fail(err)
	}
	return write(call.stdout, string(document)+"\n", exitSuccess)
}
