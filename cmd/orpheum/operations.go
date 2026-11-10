// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/backup"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/deployment"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
)

// invocation is one subcommand's run: its arguments, the environment and its streams.
type invocation struct {
	arguments []string
	lookupEnv func(string) (string, bool)
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
}

// fail writes a failure to stderr and returns the failure status.
func (call invocation) fail(err error) int {
	return write(call.stderr, "orpheum: "+err.Error()+"\n", exitFailure)
}

// loadConfigFromFlags parses a subcommand's flags, which always include --instance, and loads and
// checks the configs of the instance of kind it names (D242 §2). When addFlags is set, it adds the
// subcommand's own flags.
func (call invocation) loadConfigFromFlags(
	name string, kind instanceKind, addFlags func(*flag.FlagSet),
) (Config, ConfigReport, error) {
	flags := flag.NewFlagSet("orpheum "+name, flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	instanceDir := flags.String("instance", "", "the instance directory, which holds orpheum.toml (required)")
	if addFlags != nil {
		addFlags(flags)
	}
	if err := flags.Parse(call.arguments); err != nil {
		return Config{}, ConfigReport{}, err
	}
	if *instanceDir == "" {
		return Config{}, ConfigReport{}, errors.New("--instance is required")
	}
	return loadInstanceConfig(*instanceDir, kind)
}

// printConfigNotes writes the config file's report to stderr, for commands with no log.
func (call invocation) printConfigNotes(report ConfigReport) {
	notes := report.Notes()
	if len(notes) > 0 {
		write(call.stderr, strings.Join(notes, "\n")+"\n", exitSuccess)
	}
}

// logger builds the process's logger from the config (D73; standards/go.md §10), writing to
// standard error or appending to the file log.destination names (D242 §3). The returned close
// closes that file.
func (call invocation) logger(config Config) (logger *slog.Logger, closeLog func() error, err error) {
	opened, err := call.leveledLogger(config)
	return opened.logger, opened.closeLog, err
}

// processLog is the process's logger, the level it writes at and the close of its file.
type processLog struct {
	logger *slog.Logger
	// level is the config file's log.level until it is set to another while the process runs (D347).
	level *slog.LevelVar
	// closeLog closes the log's file; standard error's does nothing.
	closeLog func() error
}

// leveledLogger is logger, with the level it writes at.
func (call invocation) leveledLogger(config Config) (processLog, error) {
	fileLevel, err := parseLevel(config.Log.Level)
	if err != nil {
		return processLog{}, err
	}
	level := &slog.LevelVar{}
	level.Set(fileLevel)
	format, err := slogx.ParseFormat(config.Log.Format)
	if err != nil {
		return processLog{}, err
	}
	output, closeLog := call.stderr, func() error { return nil }
	if config.Log.Destination != logToStderr {
		file, openErr := os.OpenFile(config.Log.Destination, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if openErr != nil {
			return processLog{}, fmt.Errorf("open the log, log.destination: %w", openErr)
		}
		output, closeLog = file, file.Close
	}
	logger, err := slogx.New(output, format, level)
	if err != nil {
		return processLog{}, errors.Join(err, closeLog())
	}
	return processLog{logger: logger, level: level, closeLog: closeLog}, nil
}

// openExistingDatabase opens an existing persistent.db, refusing to create one where there is none.
func openExistingDatabase(ctx context.Context, config Config) (*instancedir.Directories, *store.Database, error) {
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		return nil, nil, err
	}
	exists, err := directories.PersistentDatabaseExists()
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, fmt.Errorf("there is no database at %s yet; start the server once with `orpheum core`",
			directories.PersistentDatabasePath())
	}
	database, err := store.Open(ctx, directories.PersistentDatabasePath(), config.openOptions())
	return directories, database, err
}

// commandMigrate migrates the database without starting the server (D181).
func commandMigrate(ctx context.Context, call invocation) int {
	config, configReport, err := call.loadConfigFromFlags("migrate", kindCore, nil)
	if err != nil {
		return call.fail(err)
	}
	call.printConfigNotes(configReport)
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		return call.fail(err)
	}
	var report strings.Builder
	database, err := openAndMigrate(ctx, config, directories, func(step, total int) {
		fmt.Fprintf(&report, "migrating: step %d of %d\n", step, total)
	})
	if err != nil {
		return call.fail(err)
	}
	state, err := database.State(ctx)
	if err = errors.Join(err, database.Close()); err != nil {
		return call.fail(err)
	}
	fmt.Fprintf(&report, "the database is at schema %d\n", state.Version)
	return write(call.stdout, report.String(), exitSuccess)
}

// commandBackup writes a backup of the instance's persistent directory into its backups directory
// (D242 §5); --include-key puts secret.key in it.
func commandBackup(ctx context.Context, call invocation) int {
	var shouldIncludeKey bool
	config, configReport, err := call.loadConfigFromFlags("backup", kindCore, func(flags *flag.FlagSet) {
		flags.BoolVar(&shouldIncludeKey, "include-key", false,
			"put secret.key in the backup too, for moving to a new machine")
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
	report := fmt.Sprintf("wrote %s (sealed secrets are ciphertext in it)\n", written.Path)
	if written.IncludesKey {
		report += "it holds secret.key: whoever has it has every secret; keep it safe\n"
	}
	return write(call.stdout, report, exitSuccess)
}

// commandRestore restores a backup, with the server stopped. Without --yes it only says what it
// would do (backup-restore.md: restore says what it will overwrite before doing it).
func commandRestore(ctx context.Context, call invocation) int {
	var backupPath string
	var isConfirmed bool
	config, configReport, err := call.loadConfigFromFlags("restore", kindCore, func(flags *flag.FlagSet) {
		flags.StringVar(&backupPath, "from", "", "the backup to restore, a .tar.gz `orpheum backup` wrote (required)")
		flags.BoolVar(&isConfirmed, "yes", false, "restore; without it, only say what would happen")
	})
	if err != nil {
		return call.fail(err)
	}
	call.printConfigNotes(configReport)
	if backupPath == "" {
		return call.fail(errors.New("--from is required"))
	}
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		return call.fail(err)
	}
	now := clock.System{}.Now()
	plan, err := backup.MakePlan(ctx, directories, backupPath, now)
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
	if err = backup.Restore(ctx, directories, plan, now, config.openOptions()); err != nil {
		return call.fail(err)
	}
	return write(call.stdout, "Restored. Start the server; it migrates an older backup on the way up.\n", exitSuccess)
}

// commandStartOver sets persistent.db aside in the backups directory, with the server stopped, so
// its next start makes an empty one: what an install needs once when the new version cannot bring
// its database forward (D303 §2: a fresh baseline, no migration from before it). Every other
// persistent file stays, the instance's identity, its certificate, its discovery claim and the key
// that seals its secrets, so it keeps its names and its discovery; its people, libraries, settings
// and secrets start again. Without --yes it only says what it would do.
func commandStartOver(_ context.Context, call invocation) int {
	var isConfirmed bool
	config, configReport, err := call.loadConfigFromFlags("start-over", kindCore, func(flags *flag.FlagSet) {
		flags.BoolVar(&isConfirmed, "yes", false, "set the database aside; without it, only say what would happen")
	})
	if err != nil {
		return call.fail(err)
	}
	call.printConfigNotes(configReport)
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		return call.fail(err)
	}
	asideName := "persistent-set-aside-" + clock.System{}.Now().UTC().Format("20060102T150405Z") + ".db"
	plan := fmt.Sprintf("The database %s is set aside as %s in %s. The server then starts with an empty "+
		"one: a new setup code, and its people, libraries, settings and secrets made again. Its identity, "+
		"certificate and discovery claim stay.\n", directories.PersistentDatabasePath(), asideName,
		config.Instance.BackupsDir)
	if !isConfirmed {
		return write(call.stdout, plan+"Nothing was changed. Run again with --yes to set it aside.\n", exitUsageError)
	}
	lock, err := directories.LockInstance()
	if err != nil {
		return call.fail(err)
	}
	aside, err := directories.SetPersistentDatabaseAside(asideName)
	if err = errors.Join(err, lock.Release()); err != nil {
		return call.fail(err)
	}
	return write(call.stdout, fmt.Sprintf("Set aside as %s. Start the server; it makes an empty database, and "+
		"`orpheum setup-code` prints the code that makes its first admin.\n", aside), exitSuccess)
}

// commandSetupCode prints the one-time setup code, for a server whose log has scrolled away.
func commandSetupCode(ctx context.Context, call invocation) int {
	config, configReport, err := call.loadConfigFromFlags("setup-code", kindCore, nil)
	if err != nil {
		return call.fail(err)
	}
	call.printConfigNotes(configReport)
	directories, err := instancedir.Open(config.Instance.paths())
	if err != nil {
		return call.fail(err)
	}
	code, err := runSetupCodes{directories: directories}.Read()
	if err == nil {
		return write(call.stdout, code+"\n", exitSuccess)
	}
	logger, closeLog, err := call.logger(config)
	if err != nil {
		return call.fail(err)
	}
	defer closeLog() //nolint:errcheck // the log of a command that has written its answer
	_, database, openErr := openExistingDatabase(ctx, config)
	if openErr != nil {
		return call.fail(openErr)
	}
	authService := auth.NewService(database, clock.System{}, rand.Reader,
		settings.NewService(database, clock.System{}, logger))
	count, err := authService.CountUsers(ctx)
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

// composeFilesUsage says how `make generate` writes the compose files households paste.
const composeFilesUsage = "usage: orpheum compose-files <repository>\n"

// commandComposeFiles writes every route's published compose file under the repository it is given,
// from the one template (D462 §10.2), touching only a file that differs.
func commandComposeFiles(_ context.Context, call invocation) int {
	if len(call.arguments) != 1 || call.arguments[0] == "" {
		return write(call.stderr, composeFilesUsage, exitUsageError)
	}
	if err := deployment.WriteHouseholdComposeFiles(call.arguments[0]); err != nil {
		return call.fail(err)
	}
	return exitSuccess
}

// commandDefaultConfig makes an instance in the directory it is given, a core's or with --node a
// node's, and refuses one that already holds an instance (D242 §4).
func commandDefaultConfig(_ context.Context, call invocation) int {
	flags := flag.NewFlagSet("orpheum default-config", flag.ContinueOnError)
	flags.SetOutput(call.stderr)
	isNode := flags.Bool("node", false, "make a node's instance rather than a core's")
	runDir := flags.String("run-dir", "",
		"put its run directory, which holds its socket, here rather than inside it: a socket's path is short")
	if err := flags.Parse(call.arguments); err != nil {
		return exitUsageError
	}
	if flags.NArg() != 1 {
		return write(call.stderr, "usage: orpheum default-config [--node] [--run-dir <dir>] <instance directory>\n",
			exitUsageError)
	}
	kind := kindCore
	if *isNode {
		kind = kindNode
	}
	instance, err := makeInstance(flags.Arg(0), kind, *runDir, version)
	if err != nil {
		return call.fail(err)
	}
	return write(call.stdout, fmt.Sprintf("made a %s instance in %s\n"+
		"set the keys commented out as required in %s, then run `orpheum %s --instance %s`\n",
		kind, instance.directory, instance.fullConfigPath(), kind, instance.directory), exitSuccess)
}
