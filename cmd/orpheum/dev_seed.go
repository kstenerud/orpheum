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
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/datadir"
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
	Name     string `toml:"name"`
	Password string `toml:"password"`
	Role     string `toml:"role"`
	Outsider bool   `toml:"outsider"`
}

// seedLibrary is read so the file parses whole; libraries are seeded from M2, with the scanner.
type seedLibrary struct {
	Name            string `toml:"name"`
	Kind            string `toml:"kind"`
	Root            string `toml:"root"`
	MetadataSource  string `toml:"metadata_source"`
	ChangeDetection string `toml:"change_detection"`
	Writable        bool   `toml:"writable"`
}

// devSecrets maps the environment variables `make dev` passes on to the secrets they seal
// (dev/seed.toml; D194). A user-scope secret goes to the first admin the seed creates.
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
		{"ORPHEUM_DEV_OPENSUBTITLES_API_KEY", secrets.OpenSubtitlesAPIKey},
	}
}

// commandDev runs `orpheum dev seed --config <file> --seed <file>`.
func commandDev(ctx context.Context, call invocation) int {
	if len(call.arguments) == 0 || call.arguments[0] != "seed" {
		return write(call.stderr, "Usage: orpheum dev seed --config <file> --seed <file>\n", exitUsageError)
	}
	call.arguments = call.arguments[1:]
	var seedPath string
	config, err := call.configFlags("dev seed", func(flags *flag.FlagSet) {
		flags.StringVar(&seedPath, "seed", "", "the seed file, dev/seed.toml rendered (required)")
	})
	if err != nil {
		return call.fail(err)
	}
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
	database, err := store.Open(ctx, directories.LibraryDatabase())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	err = database.Migrate(ctx, store.MigrateOptions{SnapshotDirectory: directories.BackupDirectory()})
	if err != nil {
		return err
	}
	logger, err := call.logger(config)
	if err != nil {
		return err
	}
	key, err := loadOrCreateKey(directories, logger)
	if err != nil {
		return err
	}
	firstAdmin, err := seedUsers(ctx, database, seed.Users, report)
	if err != nil {
		return err
	}
	err = seedSettings(ctx, database, seed.Settings)
	if err != nil {
		return err
	}
	sealed := secrets.NewStore(database, key, clock.System{}, rand.Reader)
	err = seedSecrets(ctx, call.lookupEnv, sealed, firstAdmin, report)
	if err != nil {
		return err
	}
	if len(seed.Libraries) > 0 {
		fmt.Fprintf(report, "seed: %d libraries are in the seed; they are created from M2, with the scanner\n",
			len(seed.Libraries))
	}
	return nil
}

func seedUsers(
	ctx context.Context, database *store.Database, users []seedUser, report *strings.Builder,
) (*int64, error) {
	service, err := auth.NewService(database, clock.System{}, rand.Reader, auth.DefaultParameters())
	if err != nil {
		return nil, err
	}
	var firstAdmin *int64
	for _, user := range users {
		id, role, seedErr := seedUserOnce(ctx, service, database, user, report)
		if seedErr != nil {
			return nil, fmt.Errorf("seed user %q: %w", user.Name, seedErr)
		}
		if role == auth.RoleAdmin && firstAdmin == nil {
			firstAdmin = &id
		}
	}
	return firstAdmin, nil
}

// seedUserOnce creates a user, or finds the one a previous seed created.
func seedUserOnce(
	ctx context.Context, service *auth.Service, database *store.Database, user seedUser, report *strings.Builder,
) (int64, auth.Role, error) {
	role, err := auth.ParseRole(user.Role)
	if err != nil {
		return 0, "", err
	}
	id, err := service.CreateUser(ctx, auth.NewUser{
		Name: user.Name, Password: user.Password, Role: role, IsOutsider: user.Outsider,
	})
	if err == nil {
		fmt.Fprintf(report, "seed: user %s (%s)\n", user.Name, role)
		return id, role, nil
	}
	if !errors.Is(err, auth.ErrNameTaken) {
		return 0, "", err
	}
	fmt.Fprintf(report, "seed: user %s exists; left as it is\n", user.Name)
	existing, err := store.One(database.Read().GetLiveUserByName(ctx, user.Name))
	if err != nil {
		return 0, "", err
	}
	return existing.ID, auth.Role(existing.RoleName), nil
}

func seedSettings(ctx context.Context, database *store.Database, values map[string]any) error {
	encoded := map[string]json.RawMessage{}
	for key, value := range values {
		text, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("seed setting %s: %w", key, err)
		}
		encoded[key] = text
	}
	return settings.NewService(database, clock.System{}).Set(ctx, nil, encoded, nil)
}

func seedSecrets(
	ctx context.Context, lookupEnv func(string) (string, bool), sealed *secrets.Store, firstAdmin *int64,
	report *strings.Builder,
) error {
	for _, source := range devSecrets() {
		text, isSet := lookupEnv(source.variable)
		if !isSet || text == "" {
			continue
		}
		definition, _ := secrets.Lookup(source.name)
		owner := (*int64)(nil)
		if definition.Scope == secrets.ScopeUser {
			if firstAdmin == nil {
				return fmt.Errorf("%s is set, but the seed created no admin to own it", source.variable)
			}
			owner = firstAdmin
		}
		if err := sealed.Set(ctx, source.name, owner, secrets.NewValue(text), nil); err != nil {
			return fmt.Errorf("seal %s: %w", source.variable, err)
		}
		fmt.Fprintf(report, "seed: sealed %s from %s\n", source.name, source.variable)
	}
	return nil
}
