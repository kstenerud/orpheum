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
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/node/agent"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
)

// Config is an instance's full config, the file read before the database opens: only what must
// exist by then, or must not be changeable by someone holding only an admin account (D182 §3,
// D200); everything else is a setting in the database. Its keys are those configKeys gives the
// instance's kind, each with its starting value: a key the file lacks runs on it and says so
// (D202). Where the instance's directories are is its top-level config's (D242).
type Config struct {
	Server   ServerConfig   `toml:"server"`
	HTTPS    HTTPSConfig    `toml:"https"`
	TURN     TURNConfig     `toml:"turn"`
	Database DatabaseConfig `toml:"database"`
	Node     NodeConfig     `toml:"node"`
	Log      LogConfig      `toml:"log"`

	// Instance is the instance directory's top-level config: its kind and its directories.
	Instance Instance `toml:"-"`
	// path is the file's absolute path.
	path string
}

// ServerConfig is where the core listens, and its HTTP timeouts.
type ServerConfig struct {
	Listen                   string `toml:"listen"`
	ReadHeaderTimeoutSeconds int64  `toml:"read_header_timeout_seconds"`
	ShutdownGraceSeconds     int64  `toml:"shutdown_grace_seconds"`
}

// HTTPSConfig is the server's public name and its HTTPS listener (D243 §6): its certificate comes by
// ACME DNS-01 through the DNS provider's token, a secret. An empty domain serves no HTTPS.
type HTTPSConfig struct {
	Domain  string `toml:"domain"`
	Contact string `toml:"contact"`
	Listen  string `toml:"listen"`
}

// isOn is whether the server has a public name to serve HTTPS on.
func (https HTTPSConfig) isOn() bool { return https.Domain != "" }

// check refuses a name with no contact or no address to listen on.
func (https HTTPSConfig) check() error {
	if !https.isOn() {
		return nil
	}
	var problems []error
	if _, _, err := net.SplitHostPort(https.Listen); err != nil {
		problems = append(problems, fmt.Errorf("https.listen %q: want host:port", https.Listen))
	}
	if !strings.Contains(https.Contact, "@") {
		problems = append(problems,
			fmt.Errorf("https.contact %q: want the e-mail address the CA writes to", https.Contact))
	}
	return errors.Join(problems...)
}

// TURNConfig is the relay that carries Watch Together's voice when two browsers cannot reach each
// other (D243 §8): where it listens, the public address the browsers are told, and the ports
// relayed media flows through. An empty public address serves no relay.
type TURNConfig struct {
	Listen        string `toml:"listen"`
	PublicAddress string `toml:"public_address"`
	RelayPortMin  int    `toml:"relay_port_min"`
	RelayPortMax  int    `toml:"relay_port_max"`
}

// isOn is whether the server relays voice.
func (relay TURNConfig) isOn() bool { return relay.PublicAddress != "" }

// check refuses a public address that is not an IP address, a listen address with no port, and a
// relay port range that is not one.
func (relay TURNConfig) check() error {
	if !relay.isOn() {
		return nil
	}
	var problems []error
	if net.ParseIP(relay.PublicAddress) == nil {
		problems = append(problems, fmt.Errorf("turn.public_address %q: want an IP address", relay.PublicAddress))
	}
	if _, _, err := net.SplitHostPort(relay.Listen); err != nil {
		problems = append(problems, fmt.Errorf("turn.listen %q: want host:port", relay.Listen))
	}
	if relay.RelayPortMin < 1 || relay.RelayPortMax > 65535 || relay.RelayPortMin > relay.RelayPortMax {
		problems = append(problems, fmt.Errorf("turn.relay_port_min %d and turn.relay_port_max %d: want a range "+
			"of ports, the least first", relay.RelayPortMin, relay.RelayPortMax))
	}
	return errors.Join(problems...)
}

// DatabaseConfig is what the store needs before the database opens.
type DatabaseConfig struct {
	ReadConnectionsMinimum     int `toml:"read_connections_minimum"`
	MigrateFreeSpaceDBMultiple int `toml:"migrate_free_space_db_multiple"`
}

// NodeConfig is the media node's.
type NodeConfig struct {
	Name string `toml:"name"`
	// CoreSocket is the core's socket, which the node dials (D242 §7).
	CoreSocket            string   `toml:"core_socket"`
	RootPaths             []string `toml:"root_paths"`
	FFmpegDir             string   `toml:"ffmpeg_dir"`
	ConfinementPolicy     string   `toml:"confinement_policy"`
	MaxErrorsPerRoot      int      `toml:"max_errors_per_root"`
	ProbeTimeoutSeconds   int64    `toml:"probe_timeout_seconds"`
	StatusIntervalSeconds int64    `toml:"status_interval_seconds"`
	ProgressEverySeconds  int64    `toml:"progress_every_seconds"`
	EndedWorkKeptHours    int64    `toml:"ended_work_kept_hours"`
	// ReadHeaderTimeoutSeconds is how long the core has to send a request's headers to the byte
	// socket.
	ReadHeaderTimeoutSeconds int64 `toml:"read_header_timeout_seconds"`
	// PreparedBudgetGiB, PreparedKeptAfterPlayHours and PreparedDiskUsedLimitPercent keep the prepared
	// directory in bounds (D217 §5).
	PreparedBudgetGiB            int64 `toml:"prepared_budget_gib"`
	PreparedKeptAfterPlayHours   int64 `toml:"prepared_kept_after_play_hours"`
	PreparedDiskUsedLimitPercent int64 `toml:"prepared_disk_used_limit_percent"`
	// PreparedDiskMinFreeMiB is the least the prepared directory's disk keeps free, whatever its percentage.
	PreparedDiskMinFreeMiB int64 `toml:"prepared_disk_min_free_mib"`
	// EncodeSlotCount, RemuxSlotCount and SlowestReadMiBPerSecond are what this machine can do,
	// which the node tells the core in its hello (nodeproto.Capacity).
	EncodeSlotCount         int64 `toml:"encode_slot_count"`
	RemuxSlotCount          int64 `toml:"remux_slot_count"`
	SlowestReadMiBPerSecond int64 `toml:"slowest_read_mib_per_second"`
}

// LogConfig is how the instance logs, and where.
type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	// Destination is "stderr", or the absolute path of a file the log is appended to (D242 §3).
	Destination string `toml:"destination"`
}

// logToStderr is the log destination that writes to standard error.
const logToStderr = "stderr"

// keyListen is the key of a table's address and port to listen on: the server's, HTTPS's and the
// voice relay's.
const keyListen = "listen"

// The file's tables.
const (
	tableServer   = "server"
	tableHTTPS    = "https"
	tableTURN     = "turn"
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

// configKeys is every key of the full config of an instance of kind, in the order the file is
// written: the core's server and database, or the node's, and the log.
func configKeys(kind instanceKind) []configKey {
	if kind == kindNode {
		return append(nodeConfigKeys(), logConfigKeys()...)
	}
	return append(coreConfigKeys(), logConfigKeys()...)
}

// coreConfigKeys are the full config's keys for the core, in the order the file is written.
func coreConfigKeys() []configKey {
	return []configKey{
		{tableServer, keyListen, "", "the address and port to serve on, which only this machine's owner can choose"},
		{tableServer, "read_header_timeout_seconds", "10", "seconds a client has to send its request headers"},
		{tableServer, "shutdown_grace_seconds", "10", "seconds in-flight requests get to finish when the server stops"},
		{
			tableHTTPS, "domain", `""`,
			`the public name served over HTTPS, "media.orpheum-app.com"; empty serves none (D243 §6)`,
		},
		{tableHTTPS, "contact", `""`, "the e-mail address the certificate's CA writes to about it"},
		{tableHTTPS, keyListen, `":443"`, "the address and port HTTPS is served on"},
		{
			tableTURN, "public_address", `""`,
			"the IP address browsers reach the voice relay at, the VPS's; empty serves none (D243 §7, §8)",
		},
		{tableTURN, keyListen, `":3478"`, "the UDP address and port the voice relay is served on (D243 §8)"},
		{tableTURN, "relay_port_min", "49160", "the least UDP port relayed voice flows through (D243 §8)"},
		{tableTURN, "relay_port_max", "49199", "the greatest UDP port relayed voice flows through (D243 §8)"},
		{
			tableDatabase, "read_connections_minimum", "4",
			"the smallest pool of read connections; a machine with more CPUs gets one per CPU",
		},
		{
			tableDatabase, "migrate_free_space_db_multiple", "3",
			"free space needed before migrating, in multiples of the database: the snapshot plus a rebuild",
		},
	}
}

// nodeConfigKeys are the file's keys for the media node, in the order the file is written.
func nodeConfigKeys() []configKey {
	return []configKey{
		{tableNode, "name", `"local"`, "the name the core knows this node by (D207)"},
		{
			tableNode, "core_socket", "",
			"the core's socket, in its instance's run directory, which this node dials (D242 §7)",
		},
		{tableNode, "root_paths", "", "the media directories this node serves; set on the node, never remotely (D107)"},
		{tableNode, "ffmpeg_dir", "", "where the pinned FFmpeg is on this machine"},
		{
			tableNode, "confinement_policy", `"require"`,
			`"require" refuses to run FFmpeg unconfined; "report" runs and says so`,
		},
		{
			tableNode, "max_errors_per_root", "1000",
			"errors one root may produce in a scan before the node stops enumerating it (D40)",
		},
		{tableNode, "probe_timeout_seconds", "60", "seconds ffprobe gets for one file before it is stopped"},
		{
			tableNode, "status_interval_seconds", "10",
			"seconds between the node's status records; keep it well under the core's node_silent_after_seconds",
		},
		{
			tableNode, "progress_every_seconds", "2",
			"seconds, at least, between a running prepare's words to the core on how far it is (D234 §5)",
		},
		{
			tableNode, "ended_work_kept_hours", "168",
			"hours the node remembers work whose result the core took, so the same command is not run twice (D238 §2)",
		},
		{
			tableNode, "read_header_timeout_seconds", "10",
			"seconds the core has to send a request's headers to this node's byte socket",
		},
		{
			tableNode, "prepared_budget_gib", "200",
			"GiB the prepared renditions may take; the least recently played go first past it (D217 §5)",
		},
		{
			tableNode, "prepared_kept_after_play_hours", "72",
			"hours after its last play a prepared rendition is kept before any past it is evicted (D217 §5)",
		},
		{
			tableNode, "prepared_disk_used_limit_percent", "95",
			"how full the prepared directory's disk may be, whatever the budget; past it, renditions are " +
				"evicted, and a prepare with nothing to evict is refused (D217 §5)",
		},
		{
			tableNode, "prepared_disk_min_free_mib", "100",
			"MiB the prepared directory's disk keeps free, whatever its fullness limit; below it, renditions are " +
				"evicted, and a prepare with nothing to evict is refused",
		},
		{
			tableNode, "encode_slot_count", "1",
			"encodes this node runs at once, which its processor or GPU bounds (cut-a §5.3)",
		},
		{
			tableNode, "remux_slot_count", "2",
			"remuxes this node runs at once, which its disk bounds (cut-a §5.3)",
		},
		{
			tableNode, "slowest_read_mib_per_second", "20",
			"the slowest this node reads a whole file, a busy spinning disk or a network mount: how long a " +
				"run that reads one is allowed, and how long the core waits for a loudness measurement",
		},
	}
}

// logConfigKeys are the file's keys for logging.
func logConfigKeys() []configKey {
	return []configKey{
		{tableLog, "level", `"info"`, "debug, info, warn or error"},
		{tableLog, "format", `"json"`, "json for the service's journal, text for a person's terminal"},
		{
			tableLog, "destination", `"stderr"`,
			`"stderr", or the absolute path of a file the log is appended to (D242 §3)`,
		},
	}
}

// ConfigReport is what loading the file found besides its values: the keys it lacks, which run on
// their starting values, and the keys nothing reads. The doctor lists both; run logs them (D202).
type ConfigReport struct {
	OnStartingValue []configKey
	Unknown         []string
}

// loadInstanceConfig reads the instance directory dir's top-level config, refusing an instance of
// another kind, and then its full config (loadConfig).
func loadInstanceConfig(dir string, kind instanceKind) (Config, ConfigReport, error) {
	instance, err := loadInstance(dir, kind)
	if err != nil {
		return Config{}, ConfigReport{}, err
	}
	config, report, err := loadConfig(instance.fullConfigPath(), kind)
	config.Instance = instance
	return config, report, err
}

// loadConfig reads and checks the full config of an instance of kind. A missing key that has a
// starting value runs on it, in memory — the file is never rewritten — and is reported; a missing
// key with none is refused, because it is specific to this machine (D202). Keys nothing reads are
// reported and ignored, never fatal (packaging-history.md lesson 11).
func loadConfig(path string, kind instanceKind) (Config, ConfigReport, error) {
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
	for _, key := range missingKeys(metadata, kind) {
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
	if err = config.check(kind); err != nil {
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

// missingKeys lists the keys of kind the file lacks.
func missingKeys(metadata toml.MetaData, kind instanceKind) []configKey {
	var missing []configKey
	for _, key := range configKeys(kind) {
		if !metadata.IsDefined(key.table, key.name) {
			missing = append(missing, key)
		}
	}
	return missing
}

// check refuses, naming the key, a value an instance of kind cannot start without or cannot use.
func (config *Config) check(kind instanceKind) error {
	problems := []error{config.Log.check()}
	if kind == kindNode {
		problems = append(problems, config.Node.check())
	} else {
		problems = append(problems, config.Server.check(), config.HTTPS.check(), config.TURN.check(),
			config.Database.check())
	}
	return errors.Join(problems...)
}

// check refuses what the core cannot listen with.
func (server ServerConfig) check() error {
	var problems []error
	if _, _, err := net.SplitHostPort(server.Listen); err != nil {
		problems = append(problems, fmt.Errorf("server.listen %q: want host:port", server.Listen))
	}
	problems = append(problems, atLeastOne(map[string]int64{
		"server.read_header_timeout_seconds": server.ReadHeaderTimeoutSeconds,
		"server.shutdown_grace_seconds":      server.ShutdownGraceSeconds,
	}))
	return errors.Join(problems...)
}

// check refuses database choices the store cannot use.
func (database DatabaseConfig) check() error {
	return atLeastOne(map[string]int64{
		"database.read_connections_minimum":       int64(database.ReadConnectionsMinimum),
		"database.migrate_free_space_db_multiple": int64(database.MigrateFreeSpaceDBMultiple),
	})
}

// check refuses what a node cannot run with.
func (node NodeConfig) check() error {
	problems := []error{atLeastOne(map[string]int64{
		"node.max_errors_per_root":            int64(node.MaxErrorsPerRoot),
		"node.probe_timeout_seconds":          node.ProbeTimeoutSeconds,
		"node.status_interval_seconds":        node.StatusIntervalSeconds,
		"node.progress_every_seconds":         node.ProgressEverySeconds,
		"node.ended_work_kept_hours":          node.EndedWorkKeptHours,
		"node.read_header_timeout_seconds":    node.ReadHeaderTimeoutSeconds,
		"node.prepared_disk_min_free_mib":     node.PreparedDiskMinFreeMiB,
		"node.prepared_budget_gib":            node.PreparedBudgetGiB,
		"node.prepared_kept_after_play_hours": node.PreparedKeptAfterPlayHours,
		"node.encode_slot_count":              node.EncodeSlotCount,
		"node.remux_slot_count":               node.RemuxSlotCount,
		"node.slowest_read_mib_per_second":    node.SlowestReadMiBPerSecond,
	})}
	// The core stops work its node says has not moved for nodes.MovementWindow, so the node's own
	// read window, which a probe's timeout lengthens, must end first, or the core stops work the
	// node still judges sound.
	if readWindow := agent.ReadStallWindowFor(time.Duration(node.ProbeTimeoutSeconds) * time.Second); readWindow >=
		nodes.MovementWindow {
		problems = append(problems, fmt.Errorf("node.probe_timeout_seconds is %d; with the node's read window it "+
			"must stay under %d, so the core does not stop work the node still judges moving",
			node.ProbeTimeoutSeconds, int64((nodes.MovementWindow-agent.ReadStallWindowFor(0))/time.Second)))
	}
	if !filepath.IsAbs(node.CoreSocket) {
		problems = append(problems, fmt.Errorf("node.core_socket must be an absolute path, not %q", node.CoreSocket))
	}
	if limit := node.PreparedDiskUsedLimitPercent; limit < 1 || limit > 100 {
		problems = append(problems, fmt.Errorf("node.prepared_disk_used_limit_percent is %d; want 1 to 100", limit))
	}
	if node.Name == "" {
		problems = append(problems, errors.New("node.name is empty; the core knows a node by its name"))
	}
	if node.ConfinementPolicy != "require" && node.ConfinementPolicy != "report" {
		problems = append(problems, fmt.Errorf("node.confinement_policy %q: want \"require\" or \"report\"",
			node.ConfinementPolicy))
	}
	return errors.Join(problems...)
}

// check refuses a log level, format or destination the logger cannot use.
func (log LogConfig) check() error {
	var problems []error
	if _, err := parseLevel(log.Level); err != nil {
		problems = append(problems, err)
	}
	if _, err := slogx.ParseFormat(log.Format); err != nil {
		problems = append(problems, err)
	}
	if log.Destination != logToStderr && !filepath.IsAbs(log.Destination) {
		problems = append(problems, fmt.Errorf("log.destination %q: want \"stderr\" or an absolute path",
			log.Destination))
	}
	return errors.Join(problems...)
}

// atLeastOne refuses, naming each, the keys whose value is below 1.
func atLeastOne(values map[string]int64) error {
	var problems []error
	for key, value := range values {
		if value < 1 {
			problems = append(problems, fmt.Errorf("%s is %d; it must be at least 1", key, value))
		}
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
	return time.Duration(config.Server.ReadHeaderTimeoutSeconds) * time.Second
}

func (config *Config) shutdownGrace() time.Duration {
	return time.Duration(config.Server.ShutdownGraceSeconds) * time.Second
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
	for _, key := range configKeys(config.Instance.Kind) {
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
