// SPDX-License-Identifier: GPL-3.0-or-later

//go:build dev

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// seedFile is dev/seed.toml (cut-a §2a).
type seedFile struct {
	Users     []seedUser     `toml:"user"`
	Libraries []seedLibrary  `toml:"library"`
	Settings  map[string]any `toml:"settings"`
}

type seedUser struct {
	Name       string `toml:"name"`
	Password   string `toml:"password"`
	Role       string `toml:"role"`
	IsOutsider bool   `toml:"is_outsider"`
}

// seedLibrary is a library the seed creates, on the config's node, with D184's questions answered.
type seedLibrary struct {
	Name            string `toml:"name"`
	Kind            string `toml:"kind"`
	RootPath        string `toml:"root_path"`
	MetadataSource  string `toml:"metadata_source"`
	ChangeDetection string `toml:"change_detection"`
	IsWritable      bool   `toml:"is_writable"`
}

// devSecrets maps the environment variables `make dev` passes on to the secrets they seal
// (dev/seed.toml; D194).
func devSecrets() []struct {
	variable string
	name     secrets.Name
} {
	return []struct {
		variable string
		name     secrets.Name
	}{
		{"ORPHEUM_DEV_TMDB_KEY", secrets.TMDBAPIKey},
		{"ORPHEUM_DEV_OPENSUBTITLES_USERNAME", secrets.OpenSubtitlesUsername},
		{"ORPHEUM_DEV_OPENSUBTITLES_PASSWORD", secrets.OpenSubtitlesPassword},
	}
}

// commandDev runs `orpheum dev seed --config <file> --seed <file>`.
func commandDev(ctx context.Context, call invocation) int {
	if len(call.arguments) == 0 || call.arguments[0] != "seed" {
		return write(call.stderr, "Usage: orpheum dev seed --config <file> --seed <file>\n", exitUsageError)
	}
	call.arguments = call.arguments[1:]
	var seedPath string
	config, configReport, err := call.loadConfigFromFlags("dev seed", func(flags *flag.FlagSet) {
		flags.StringVar(&seedPath, "seed", "", "the seed file, dev/seed.toml rendered (required)")
	})
	if err != nil {
		return call.fail(err)
	}
	call.printConfigNotes(configReport)
	var seed seedFile
	metadata, err := toml.DecodeFile(seedPath, &seed)
	if err != nil {
		return call.fail(fmt.Errorf("seed %s: %w", seedPath, err))
	}
	var report strings.Builder
	for _, key := range metadata.Undecoded() {
		fmt.Fprintf(&report, "seed: key %s is not recognised and is ignored\n", key)
	}
	err = applySeed(ctx, call, config, seed, &report)
	status := write(call.stdout, report.String(), exitSuccess)
	if err != nil {
		return call.fail(err)
	}
	return status
}

// applySeed opens and migrates the dev database and puts the seed into it. It can run again on a
// seeded database: users that exist are left as they are.
func applySeed(ctx context.Context, call invocation, config Config, seed seedFile, report *strings.Builder) (
	err error,
) {
	directories, err := datadir.Prepare(config.Server.DataDir, config.Server.CacheDir)
	if err != nil {
		return err
	}
	database, err := openAndMigrate(ctx, config, directories, nil)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	logger, err := call.logger(config)
	if err != nil {
		return err
	}
	// Every setting gets its starting value first: hashing a password reads argon2id's costs.
	settingsService := settings.NewService(database, clock.System{}, logger)
	if _, err = settingsService.Seed(ctx); err != nil { // the server logs what the seed found at start
		return err
	}
	key, err := loadOrCreateSecretKey(directories, logger)
	if err != nil {
		return err
	}
	err = seedUsers(ctx, database, settingsService, seed.Users, report)
	if err != nil {
		return err
	}
	err = seedSettings(ctx, settingsService, seed.Settings)
	if err != nil {
		return err
	}
	secretStore := secrets.NewStore(database, key, clock.System{}, rand.Reader, settingsService)
	err = seedSecrets(ctx, call.lookupEnv, secretStore, report)
	if err != nil {
		return err
	}
	return seedLibraries(ctx, database, config.Node.Name, seed.Libraries, report)
}

// seedLibraries creates each library that does not exist yet, on the node the config names,
// which it enrols if the node has not said hello. A root is recorded as the node reports it:
// with symlinks resolved.
func seedLibraries(
	ctx context.Context, database *store.Database, nodeName string, libraries []seedLibrary, report *strings.Builder,
) error {
	if len(libraries) == 0 {
		return nil
	}
	nodeID, err := nodes.Enrol(ctx, database, clock.System{}, nodeName)
	if err != nil {
		return err
	}
	for _, wantedLibrary := range libraries {
		root := wantedLibrary.RootPath
		if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil {
			root = resolved
		}
		_, err = library.Create(ctx, database, clock.System{}, library.NewLibrary{
			Name: wantedLibrary.Name, Kind: wantedLibrary.Kind, MetadataSource: wantedLibrary.MetadataSource,
			ChangeDetection: wantedLibrary.ChangeDetection, IsWritable: wantedLibrary.IsWritable, NodeID: nodeID,
			Roots: []string{root},
		})
		switch {
		case errors.Is(err, library.ErrNameTaken):
			fmt.Fprintf(report, "seed: library %q exists; left as it is\n", wantedLibrary.Name)
		case err != nil:
			return fmt.Errorf("seed library %q: %w", wantedLibrary.Name, err)
		default:
			fmt.Fprintf(report, "seed: library %q covers %s on node %q\n", wantedLibrary.Name, root, nodeName)
		}
	}
	return nil
}

func seedUsers(
	ctx context.Context, database *store.Database, settingsService *settings.Service, users []seedUser,
	report *strings.Builder,
) error {
	service := auth.NewService(database, clock.System{}, rand.Reader, settingsService)
	for _, user := range users {
		if err := seedUserOnce(ctx, service, user, report); err != nil {
			return fmt.Errorf("seed user %q: %w", user.Name, err)
		}
	}
	return nil
}

// seedUserOnce creates a user, or leaves the one a previous seed created.
func seedUserOnce(ctx context.Context, service *auth.Service, user seedUser, report *strings.Builder) error {
	role, err := auth.ParseRole(user.Role)
	if err != nil {
		return err
	}
	_, err = service.CreateUser(ctx, auth.NewUser{
		Name: user.Name, Password: user.Password, Role: role, IsOutsider: user.IsOutsider,
	})
	switch {
	case err == nil:
		fmt.Fprintf(report, "seed: user %s (%s)\n", user.Name, role)
	case errors.Is(err, auth.ErrNameTaken):
		fmt.Fprintf(report, "seed: user %s exists; left as it is\n", user.Name)
	default:
		return err
	}
	return nil
}

// seedSettings sets the seed's values through the raw path, as /config does: the dev seed may set
// any setting, shown on the settings page or not.
func seedSettings(ctx context.Context, settingsService *settings.Service, values map[string]any) error {
	encoded := map[string]json.RawMessage{}
	for key, value := range values {
		text, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("seed setting %s: %w", key, err)
		}
		encoded[key] = text
	}
	return settingsService.Set(ctx, nil, encoded, nil, false)
}

func seedSecrets(
	ctx context.Context, lookupEnv func(string) (string, bool), sealed *secrets.Store, report *strings.Builder,
) error {
	for _, source := range devSecrets() {
		text, isSet := lookupEnv(source.variable)
		if !isSet || text == "" {
			continue
		}
		if err := sealed.Set(ctx, source.name, secrets.NewValue(text), nil); err != nil {
			return fmt.Errorf("seal %s: %w", source.variable, err)
		}
		fmt.Fprintf(report, "seed: sealed %s from %s\n", source.name, source.variable)
	}
	return nil
}
