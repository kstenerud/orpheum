// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/deployment"
	"github.com/kstenerud/orpheum/internal/node/agent"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
)

// Config is an instance's full config, the file read before the database opens: only what must
// exist by then, or must not be changeable by someone holding only an admin account (D182 §3,
// D200); everything else is a setting in the database. Its keys are those configKeys gives the
// instance's kind, each with its starting value: a key the file lacks runs on it and says so
// (D202). Where the instance's directories are is its top-level config's (D242).
type Config struct {
	Server    ServerConfig    `toml:"server"`
	HTTPS     HTTPSConfig     `toml:"https"`
	TURN      TURNConfig      `toml:"turn"`
	Discovery DiscoveryConfig `toml:"discovery"`
	WebRTC    WebRTCConfig    `toml:"webrtc"`
	Database  DatabaseConfig  `toml:"database"`
	Node      NodeConfig      `toml:"node"`
	Log       LogConfig       `toml:"log"`

	// Instance is the instance directory's top-level config: its kind and its directories.
	Instance Instance `toml:"-"`
	// slowWrites, when set, is told each database write that was slow (store.OpenOptions): the
	// serving core's log.
	slowWrites func(store.SlowWrite)
	// slowWriteClock times the writes slowWrites hears of: the system's unless a test names another.
	slowWriteClock clock.Clock
	// serviceClock is the clock the serving core's services read: the system's unless a test names
	// another (see servicesClock).
	serviceClock clock.Clock
	// readerWrites, when set, is told each write a request that only reads made (store.OpenOptions).
	readerWrites func(file, caller string)
	// path is the file's absolute path.
	path string
}

// ServerConfig is where the core listens, and its HTTP timeouts.
type ServerConfig struct {
	Listen                   string `toml:"listen"`
	ReadHeaderTimeoutSeconds int64  `toml:"read_header_timeout_seconds"`
	ShutdownGraceSeconds     int64  `toml:"shutdown_grace_seconds"`
	// HostNames are the public names the house reaches the plain-HTTP listener by, beside its IP
	// addresses and the names public DNS never answers: a request naming any other host is refused,
	// against DNS rebinding (the discovery protocol's §7 item 10; S15).
	HostNames []string `toml:"host_names"`
	// HomeNetworks are networks home besides the private ranges, a VPN's say, from which a request,
	// directly or over a WebRTC connection with no relay between, lets an admin's session administer
	// (D297 §6, D300 §3). Home is always every private address, 10.0.0.0/8, 172.16.0.0/12,
	// 192.168.0.0/16 and fc00::/7, with the core's loopback and the /64 of its own public IPv6 address,
	// but those in NotHomeNetworks or in a network of the core's own interfaces (Docker's bridge, unless
	// it holds the address at home), so a new lease from the router locks no admin out (D452 §1, §2).
	// A listed network is under the same exceptions (the owner, 2026-11-09).
	HomeNetworks []string `toml:"home_networks"`
	// NotHomeNetworks are private networks never home: the tunnel's, through which the internet
	// arrives (D452 §1).
	NotHomeNetworks []string `toml:"not_home_networks"`
	// AddressAtHomeFromDefaultRoute is whether the core shares the host's network, as a deployment
	// with no tunnel runs it (D454 §5), and so takes the address of the interface carrying the default
	// route as the address at home until one is learned or set: without it the house's network, one of
	// its own interfaces', is not home, and no browser at home could teach the address. In a container's
	// own network the default route is Docker's bridge, and taking it would make the internet home
	// (D452 §3). It also has the core learn only an address one of its interfaces holds, since a
	// request's Host is the device's to write.
	AddressAtHomeFromDefaultRoute bool `toml:"address_at_home_from_default_route"`
	// ThisMachineListen is plain HTTP for a browser on the server's own machine alone, which the
	// deployment publishes on that machine's loopback: a request on it is from home, though Docker
	// hands it the core from its gateway (D334 §4). Empty serves none.
	ThisMachineListen string `toml:"this_machine_listen"`
	// InstallRoute is how the server was installed: deployment.ByInstallScript or a compose file's
	// route, as the deployment writes it, or empty when unknown (a config written by hand). The Update
	// page shows only that route's steps, and every one when unknown (D450 §2, D462 §9).
	InstallRoute string `toml:"install_route"`
	// ComposeVersion is the compose file's version, 0 for any other install: the Update page says
	// when a newer one brings something (D461 §3).
	ComposeVersion int `toml:"compose_version"`
	// ImageTag is the compose file's image tag, a pinned one said on the Update page (D462 §10.7).
	ImageTag string `toml:"image_tag"`
	// ComposeMedia are the media folders the compose file names, which the Update page fills into a
	// Synology's newer file (D462 §4); empty for any other install.
	ComposeMedia []string `toml:"compose_media"`
}

// thisMachinePort is the port ThisMachineListen names, 0 for none; check refused one that names none.
func (server ServerConfig) thisMachinePort() int {
	return portOf(server.ThisMachineListen)
}

// lanHostNames are the names the plain-HTTP listener answers beside the ones it always does: the
// config's, and the HTTPS name.
func lanHostNames(config Config) []string {
	return append(slices.Clone(config.Server.HostNames), config.HTTPS.Domain)
}

// HTTPSConfig is the server's public name and its HTTPS listener (D243 §6): its certificate comes by
// ACME DNS-01 through the DNS provider's token, a secret. An empty domain serves no HTTPS.
type HTTPSConfig struct {
	Domain  string `toml:"domain"`
	Contact string `toml:"contact"`
	Listen  string `toml:"listen"`
	// ACMEDirectory is the CA the certificate comes from: Let's Encrypt's production directory, or
	// its staging one, which spends no production quota (D261 §5).
	ACMEDirectory string `toml:"acme_directory"`
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
	if !strings.HasPrefix(https.ACMEDirectory, "https://") {
		problems = append(problems,
			fmt.Errorf("https.acme_directory %q: want the https:// address of an ACME directory", https.ACMEDirectory))
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
	// WhenUnmigratable is what the core does with a persistent.db it cannot bring forward:
	// whenRefuse, or whenStartOver, which staging uses (D249 §4).
	WhenUnmigratable string `toml:"when_unmigratable"`
}

// The values of database.when_unmigratable (D249 §4).
const (
	whenRefuse    = "refuse"
	whenStartOver = "start_over"
)

// NodeConfig is the media node's.
type NodeConfig struct {
	Name string `toml:"name"`
	// CoreSocket is the core's socket, which the node dials (D242 §7).
	CoreSocket          string   `toml:"core_socket"`
	RootPaths           []string `toml:"root_paths"`
	FFmpegDir           string   `toml:"ffmpeg_dir"`
	ConfinementPolicy   string   `toml:"confinement_policy"`
	ProbeTimeoutSeconds int64    `toml:"probe_timeout_seconds"`
	EndedWorkKeptHours  int64    `toml:"ended_work_kept_hours"`
	// ReadHeaderTimeoutSeconds is how long the core has to send a request's headers to the byte
	// socket.
	ReadHeaderTimeoutSeconds int64 `toml:"read_header_timeout_seconds"`
	// PreparedBudgetGiB, PreparedKeptAfterPlayHours and PreparedDiskUsedLimitPercent keep the prepared
	// directory in bounds (D217 §5).
	PreparedBudgetGiB            int64 `toml:"prepared_budget_gib"`
	PreparedKeptAfterPlayHours   int64 `toml:"prepared_kept_after_play_hours"`
	PreparedDiskUsedLimitPercent int64 `toml:"prepared_disk_used_limit_percent"`
	// PreparedWatchedMinutes is how recently a store served a file for it to count as being
	// watched, which eviction never removes.
	PreparedWatchedMinutes int64 `toml:"prepared_watched_minutes"`
	// PreparedDiskMinFreeMiB is the least the prepared directory's disk keeps free, whatever its percentage.
	PreparedDiskMinFreeMiB int64 `toml:"prepared_disk_min_free_mib"`
	// EncodeSlotCount, RemuxSlotCount and SlowestReadMiBPerSecond are what this machine can do,
	// which the node tells the core in its hello (nodeproto.Capacity).
	EncodeSlotCount         int64 `toml:"encode_slot_count"`
	RemuxSlotCount          int64 `toml:"remux_slot_count"`
	SlowestReadMiBPerSecond int64 `toml:"slowest_read_mib_per_second"`
	// WatchQuietSeconds is how long a root goes without a change before what its watcher saw is
	// noticed (D272 §9).
	WatchQuietSeconds int64 `toml:"watch_quiet_seconds"`
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
			config.Discovery.check(), config.WebRTC.check(), config.Database.check())
	}
	return errors.Join(problems...)
}

// check refuses what the core cannot listen with.
func (server ServerConfig) check() error {
	var problems []error
	if _, _, err := net.SplitHostPort(server.Listen); err != nil {
		problems = append(problems, fmt.Errorf("server.listen %q: want host:port", server.Listen))
	}
	if route := deployment.InstallRoute(server.InstallRoute); route != "" && route != deployment.ByInstallScript &&
		!route.IsCompose() {
		problems = append(problems, fmt.Errorf("server.install_route %q: want %q, one of %v or empty",
			server.InstallRoute, deployment.ByInstallScript, deployment.ComposeRoutes()))
	}
	if server.ComposeVersion < 0 {
		problems = append(problems, fmt.Errorf("server.compose_version %d: want 0 or more", server.ComposeVersion))
	}
	if server.ThisMachineListen != "" && server.thisMachinePort() == 0 {
		problems = append(problems, fmt.Errorf("server.this_machine_listen %q: want host:port, or empty",
			server.ThisMachineListen))
	}
	problems = append(problems, atLeastOne(map[string]int64{
		"server.read_header_timeout_seconds": server.ReadHeaderTimeoutSeconds,
		"server.shutdown_grace_seconds":      server.ShutdownGraceSeconds,
	}))
	problems = append(problems, server.checkHomeNetworks()...)
	return errors.Join(problems...)
}

// check refuses database choices the store cannot use.
func (database DatabaseConfig) check() error {
	problems := []error{atLeastOne(map[string]int64{
		"database.read_connections_minimum":       int64(database.ReadConnectionsMinimum),
		"database.migrate_free_space_db_multiple": int64(database.MigrateFreeSpaceDBMultiple),
	})}
	if database.WhenUnmigratable != whenRefuse && database.WhenUnmigratable != whenStartOver {
		problems = append(problems, fmt.Errorf("database.when_unmigratable %q: want %q or %q",
			database.WhenUnmigratable, whenRefuse, whenStartOver))
	}
	return errors.Join(problems...)
}

// check refuses what a node cannot run with.
func (node NodeConfig) check() error {
	problems := []error{atLeastOne(map[string]int64{
		"node.probe_timeout_seconds":          node.ProbeTimeoutSeconds,
		"node.ended_work_kept_hours":          node.EndedWorkKeptHours,
		"node.read_header_timeout_seconds":    node.ReadHeaderTimeoutSeconds,
		"node.prepared_disk_min_free_mib":     node.PreparedDiskMinFreeMiB,
		"node.prepared_budget_gib":            node.PreparedBudgetGiB,
		"node.prepared_kept_after_play_hours": node.PreparedKeptAfterPlayHours,
		"node.prepared_watched_minutes":       node.PreparedWatchedMinutes,
		"node.encode_slot_count":              node.EncodeSlotCount,
		"node.remux_slot_count":               node.RemuxSlotCount,
		"node.slowest_read_mib_per_second":    node.SlowestReadMiBPerSecond,
		"node.watch_quiet_seconds":            node.WatchQuietSeconds,
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
	if !slices.Contains(confinementPolicies(), node.ConfinementPolicy) {
		problems = append(problems, fmt.Errorf("node.confinement_policy %q: want \"require\", \"available\" or "+
			"\"report\"", node.ConfinementPolicy))
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
	options := store.OpenOptions{
		MinimumReaders: config.Database.ReadConnectionsMinimum, OnReaderWrite: config.readerWrites,
	}
	if config.slowWrites != nil {
		options.Clock, options.OnSlowWrite = config.slowWriteClock, config.slowWrites
		if options.Clock == nil {
			options.Clock = clock.System{}
		}
	}
	return options
}

// servicesClock is the clock the core's services read: the system's unless a test named another.
func (config *Config) servicesClock() clock.Clock {
	if config.serviceClock == nil {
		return clock.System{}
	}
	return config.serviceClock
}

// loggingSlowWrites logs each database write that waited for the writer, or held it, past
// store.SlowWriteAfter, and the write that held the writer when it asked: what a slow request
// that writes waited behind (the owner's production NAS, 2026-10-15; D303, DL17).
func loggingSlowWrites(ctx context.Context, logger *slog.Logger) func(store.SlowWrite) {
	return func(write store.SlowWrite) {
		attributes := []any{
			"file", write.File, "caller", write.Caller,
			"waitedMs", write.Waited.Milliseconds(), "heldMs", write.Held.Milliseconds(),
		}
		if write.Behind != "" {
			attributes = append(attributes, "behind", write.Behind)
		}
		logger.InfoContext(ctx, "a database write was slow", attributes...)
	}
}

// loggingReaderWrites logs each database write a request that only reads made: reading never
// writes (D303, DL10), and the log names what did. A write the store allows a reading request, as a
// play's GET queuing the chunks it plays, is logged at debug, with why; any other is a warning.
func loggingReaderWrites(ctx context.Context, logger *slog.Logger) func(file, caller string) {
	return func(file, caller string) {
		if why, isAllowed := store.ReaderWriteAllowed(caller); isAllowed {
			logger.DebugContext(ctx, "a request that only reads wrote to the database, as it may",
				"file", file, "caller", caller, "why", why)
			return
		}
		logger.WarnContext(ctx, "a request that only reads wrote to the database", "file", file, "caller", caller)
	}
}

// migrateOptions are the runner's choices from the file, its snapshots in the backup directory.
func (config *Config) migrateOptions(
	directories *instancedir.Directories, beforeStep func(step, total int),
) store.MigrateOptions {
	return store.MigrateOptions{
		SnapshotDirectory: directories.BackupDirectory(), RemoveSnapshotFile: directories.RemoveBackupFile,
		OnBeforeStep:              beforeStep,
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
