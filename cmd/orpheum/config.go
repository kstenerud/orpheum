// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
)

// Config is the file read before the database opens: only what must exist by then, or must not
// be changeable by someone holding only an admin account (D182 §3, D200); everything else is a
// setting in the database. Its keys are exactly those of dev/orpheum.dev.toml (cut-a §2a) and
// configKeys, which gives each its starting value: a key the file lacks runs on it and says so
// (D202).
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Database DatabaseConfig `toml:"database"`
	Node     NodeConfig     `toml:"node"`
	Log      LogConfig      `toml:"log"`

	// path is the file's absolute path, which the node child is started with.
	path string
}

// ServerConfig is where the server listens and keeps its state, and its HTTP timeouts.
type ServerConfig struct {
	Listen             string `toml:"listen"`
	DataDir            string `toml:"data_dir"`
	CacheDir           string `toml:"cache_dir"`
	ReadHeaderTimeoutS int64  `toml:"read_header_timeout_s"`
	ShutdownGraceS     int64  `toml:"shutdown_grace_s"`
	SocketDir          string `toml:"socket_dir"`
}

// DatabaseConfig is what the store needs before the database opens.
type DatabaseConfig struct {
	ReadConnectionsMinimum     int `toml:"read_connections_minimum"`
	MigrateFreeSpaceDBMultiple int `toml:"migrate_free_space_db_multiple"`
}

// NodeConfig is the media node's.
type NodeConfig struct {
	Name               string   `toml:"name"`
	RootPaths          []string `toml:"root_paths"`
	FFmpegDir          string   `toml:"ffmpeg_dir"`
	ConfinementPolicy  string   `toml:"confinement_policy"`
	VideoExtensions    []string `toml:"video_extensions"`
	SubtitleExtensions []string `toml:"subtitle_extensions"`
	MaxErrorsPerRoot   int      `toml:"max_errors_per_root"`
	ProbeTimeoutS      int64    `toml:"probe_timeout_s"`
	StatusIntervalS    int64    `toml:"status_interval_s"`
	PreparedDir        string   `toml:"prepared_dir"`
	// PreparedBudgetGiB, PreparedGraceH and PreparedDiskLimitPercent keep the prepared
	// directory in bounds (D217 §5).
	PreparedBudgetGiB            int64 `toml:"prepared_budget_gib"`
	PreparedKeptAfterPlayH       int64 `toml:"prepared_kept_after_play_h"`
	PreparedDiskUsedLimitPercent int64 `toml:"prepared_disk_used_limit_percent"`
}

// LogConfig is how the server logs.
type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

// The file's tables.
const (
	tableServer   = "server"
	tableDatabase = "database"
	tableNode     = "node"
	tableLog      = "log"
)

// configKey is one key of the file. A key with a starting value runs on it when the file lacks it
// (D202); one without is specific to the machine, and the server refuses to start without it.
type configKey struct {
	table         string
	name          string
	startingValue string // the starting value as TOML; "" when there is none
	description   string
}

func (key configKey) path() string {
	return key.table + "." + key.name
}

// configKeys is every key of the file, in the order the file is written.
func configKeys() []configKey {
	return []configKey{
		{tableServer, "listen", "", "the address and port to serve on, which only this machine's owner can choose"},
		{tableServer, "data_dir", "", "where library.db, secret.key and backups live on this machine"},
		{tableServer, "cache_dir", "", "where cache.db and the artwork cache live on this machine"},
		{tableServer, "read_header_timeout_s", "10", "seconds a client has to send its request headers"},
		{tableServer, "shutdown_grace_s", "10", "seconds in-flight requests get to finish when the server stops"},
		{
			tableServer, "socket_dir", `"/run/orpheum"`,
			"where the core's and the node's sockets live; systemd's RuntimeDirectory makes /run/orpheum",
		},
		{
			tableDatabase, "read_connections_minimum", "4",
			"the smallest pool of read connections; a machine with more CPUs gets one per CPU",
		},
		{
			tableDatabase, "migrate_free_space_db_multiple", "3",
			"free space needed before migrating, in multiples of the database: the snapshot plus a rebuild",
		},
		{tableNode, "name", `"local"`, "the name the core knows this node by (D207)"},
		{tableNode, "root_paths", "", "the media directories this node serves; set on the node, never remotely (D107)"},
		{tableNode, "ffmpeg_dir", "", "where the pinned FFmpeg is on this machine"},
		{
			tableNode, "confinement_policy", `"require"`,
			`"require" refuses to run FFmpeg unconfined; "report" runs and says so`,
		},
		{
			tableNode, "video_extensions", videoExtensions,
			"extensions, lower case without the dot, of files the node probes; anything else is skipped",
		},
		{
			tableNode, "subtitle_extensions", `["srt", "ass", "ssa", "vtt"]`,
			"extensions of text subtitle files, reported as sidecars of their video; .nfo always is one",
		},
		{
			tableNode, "max_errors_per_root", "1000",
			"errors one root may produce in a scan before the node stops enumerating it (D40)",
		},
		{tableNode, "probe_timeout_s", "60", "seconds ffprobe gets for one file before it is stopped"},
		{
			tableNode, "status_interval_s", "10",
			"seconds between the node's status records; keep it well under the core's node_silent_after_s",
		},
		{
			tableNode, "prepared_dir", `"/var/cache/orpheum/prepared"`,
			"where this node's prepared renditions live; the node alone deletes there (D217 §5)",
		},
		{
			tableNode, "prepared_budget_gib", "200",
			"GiB the prepared renditions may take; the least recently played go first past it (D217 §5)",
		},
		{
			tableNode, "prepared_kept_after_play_h", "72",
			"hours after its last play a prepared rendition is kept before any past it is evicted (D217 §5)",
		},
		{
			tableNode, "prepared_disk_used_limit_percent", "95",
			"how full the prepared directory's disk may be, whatever the budget; past it, renditions are " +
				"evicted, and a prepare with nothing to evict is refused (D217 §5)",
		},
		{tableLog, "level", `"info"`, "debug, info, warn or error"},
		{tableLog, "format", `"json"`, "json for the service's journal, text for a person's terminal"},
	}
}

// videoExtensions is the starting list of video extensions: what real libraries hold. The
// extension is only a hint about whether probing is worthwhile (scanner.md).
const videoExtensions = `["mkv", "mp4", "m4v", "avi", "mov", "wmv", "ts", "m2ts", "mts", "webm", "mpg", "mpeg", ` +
	`"vob", "flv", "ogv", "3gp"]`

// ConfigReport is what loading the file found besides its values: the keys it lacks, which run on
// their starting values, and the keys nothing reads. The doctor lists both; run logs them (D202).
type ConfigReport struct {
	OnStartingValue []configKey
	Unknown         []string
}

// loadConfig reads and checks the config file. A missing key that has a starting value runs on it,
// in memory — the file is never rewritten — and is reported; a missing key with none is refused,
// because it is specific to this machine (D202). Keys nothing reads are reported and ignored,
// never fatal (packaging-history.md lesson 11).
func loadConfig(path string) (Config, ConfigReport, error) {
	var report ConfigReport
	text, err := os.ReadFile(path) //nolint:gosec // G304: the operator names the config file
	if err != nil {
		return Config{}, report, fmt.Errorf("read the config file: %w", err)
	}
	metadata, err := toml.Decode(string(text), &Config{})
	if err != nil {
		return Config{}, report, fmt.Errorf("config %s: %w", path, err)
	}
	report.Unknown = unknownKeys(metadata)
	var missingRequired []error
	for _, key := range missingKeys(metadata) {
		if key.startingValue == "" {
			missingRequired = append(missingRequired,
				fmt.Errorf("%s is required and has no starting value: it is %s; add it to "+
					"the file", key.path(), key.description))
			continue
		}
		report.OnStartingValue = append(report.OnStartingValue, key)
	}
	if len(missingRequired) > 0 {
		return Config{}, report, fmt.Errorf("config %s: %w", path, errors.Join(missingRequired...))
	}
	full := string(text)
	if len(report.OnStartingValue) > 0 {
		if full, err = withStartingValues(full, report.OnStartingValue); err != nil {
			return Config{}, report, fmt.Errorf("config %s: %w", path, err)
		}
	}
	var config Config
	if _, err = toml.Decode(full, &config); err != nil {
		return Config{}, report, fmt.Errorf("config %s: %w", path, err)
	}
	if err = config.check(); err != nil {
		return Config{}, report, fmt.Errorf("config %s: %w", path, err)
	}
	if config.path, err = filepath.Abs(path); err != nil {
		return Config{}, report, fmt.Errorf("config %s: %w", path, err)
	}
	return config, report, nil
}

// Notes renders the report as lines, one per key.
func (report ConfigReport) Notes() []string {
	var notes []string
	for _, key := range report.OnStartingValue {
		notes = append(notes, fmt.Sprintf("config: %s is not in the file; using its starting value, %s (%s)",
			key.path(), key.startingValue, key.description))
	}
	for _, key := range report.Unknown {
		notes = append(notes, fmt.Sprintf("config: %s is in the file, but nothing reads it: a typo, or a key a "+
			"release removed. It is ignored", key))
	}
	return notes
}

// unknownKeys lists the keys the file has and nothing reads: a typo, or a key a release removed.
func unknownKeys(metadata toml.MetaData) []string {
	undecoded := metadata.Undecoded()
	var unknown []string
	for index, key := range undecoded {
		// An unknown table is listed before its keys; report the keys, which say more.
		if index+1 < len(undecoded) && strings.HasPrefix(undecoded[index+1].String(), key.String()+".") {
			continue
		}
		unknown = append(unknown, key.String())
	}
	return unknown
}

// missingKeys lists the keys the file lacks.
func missingKeys(metadata toml.MetaData) []configKey {
	var missing []configKey
	for _, key := range configKeys() {
		if !metadata.IsDefined(key.table, key.name) {
			missing = append(missing, key)
		}
	}
	return missing
}

// check refuses, naming the key, a value the server cannot start without or cannot use.
func (config *Config) check() error {
	var problems []error
	for key, value := range map[string]string{
		"server.data_dir": config.Server.DataDir, "server.cache_dir": config.Server.CacheDir,
		"server.socket_dir": config.Server.SocketDir,
	} {
		if !filepath.IsAbs(value) {
			problems = append(problems, fmt.Errorf("%s must be an absolute path, not %q", key, value))
		}
	}
	if _, _, err := net.SplitHostPort(config.Server.Listen); err != nil {
		problems = append(problems, fmt.Errorf("server.listen %q: want host:port", config.Server.Listen))
	}
	for key, value := range map[string]int64{
		"server.read_header_timeout_s":            config.Server.ReadHeaderTimeoutS,
		"server.shutdown_grace_s":                 config.Server.ShutdownGraceS,
		"database.read_connections_minimum":       int64(config.Database.ReadConnectionsMinimum),
		"database.migrate_free_space_db_multiple": int64(config.Database.MigrateFreeSpaceDBMultiple),
		"node.max_errors_per_root":                int64(config.Node.MaxErrorsPerRoot),
		"node.probe_timeout_s":                    config.Node.ProbeTimeoutS,
		"node.status_interval_s":                  config.Node.StatusIntervalS,
		"node.prepared_budget_gib":                config.Node.PreparedBudgetGiB,
		"node.prepared_kept_after_play_h":         config.Node.PreparedKeptAfterPlayH,
	} {
		if value < 1 {
			problems = append(problems, fmt.Errorf("%s is %d; it must be at least 1", key, value))
		}
	}
	if limit := config.Node.PreparedDiskUsedLimitPercent; limit < 1 || limit > 100 {
		problems = append(problems, fmt.Errorf("node.prepared_disk_used_limit_percent is %d; want 1 to 100", limit))
	}
	if config.Node.Name == "" {
		problems = append(problems, errors.New("node.name is empty; the core knows a node by its name"))
	}
	if config.Node.ConfinementPolicy != "require" && config.Node.ConfinementPolicy != "report" {
		problems = append(problems, fmt.Errorf("node.confinement_policy %q: want \"require\" or \"report\"",
			config.Node.ConfinementPolicy))
	}
	if _, err := parseLevel(config.Log.Level); err != nil {
		problems = append(problems, err)
	}
	if _, err := slogx.ParseFormat(config.Log.Format); err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}

// openOptions are the store's choices from the file.
func (config *Config) openOptions() store.OpenOptions {
	return store.OpenOptions{MinimumReaders: config.Database.ReadConnectionsMinimum}
}

// migrateOptions are the runner's choices from the file.
func (config *Config) migrateOptions(snapshotDirectory string, beforeStep func(step, total int)) store.MigrateOptions {
	return store.MigrateOptions{
		SnapshotDirectory: snapshotDirectory, OnBeforeStep: beforeStep,
		FreeSpaceDatabaseMultiple: config.Database.MigrateFreeSpaceDBMultiple,
	}
}

func (config *Config) readHeaderTimeout() time.Duration {
	return time.Duration(config.Server.ReadHeaderTimeoutS) * time.Second
}

func (config *Config) shutdownGrace() time.Duration {
	return time.Duration(config.Server.ShutdownGraceS) * time.Second
}

func parseLevel(name string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(name)); err != nil {
		return 0, fmt.Errorf("log.level %q: want debug, info, warn or error", name)
	}
	return level, nil
}

// isLoopback reports whether a listen address accepts only this machine's connections.
func isLoopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

// configFileKeys renders every key as the server runs with it, for /config to show read-only.
func configFileKeys(config Config, report ConfigReport) ([]api.ConfigFileKey, error) {
	encoded, err := toml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("render the config: %w", err)
	}
	var tables map[string]map[string]any
	if _, err = toml.Decode(string(encoded), &tables); err != nil {
		return nil, fmt.Errorf("render the config: %w", err)
	}
	isOnStartingValue := map[string]bool{}
	for _, key := range report.OnStartingValue {
		isOnStartingValue[key.path()] = true
	}
	keys := []api.ConfigFileKey{}
	for _, key := range configKeys() {
		value, err := json.Marshal(tables[key.table][key.name])
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", key.path(), err)
		}
		startingValue := json.RawMessage("null")
		if key.startingValue != "" {
			startingValue = json.RawMessage(key.startingValue)
		}
		keys = append(keys, api.ConfigFileKey{
			Key: key.path(), Value: value, StartingValue: startingValue, IsFromFile: !isOnStartingValue[key.path()],
			Description: key.description,
		})
	}
	return keys, nil
}
