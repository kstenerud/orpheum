// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// Config is the file read before the database opens: only what must exist by then, or must not
// be changeable by someone holding only an admin account (D182 §3, D200). Every key is required
// and none has a default in code; everything else is a setting in the database. Its keys are
// exactly those of dev/orpheum.dev.toml (cut-a §2a).
type Config struct {
	Server ServerConfig `toml:"server"`
	Node   NodeConfig   `toml:"node"`
	Log    LogConfig    `toml:"log"`
}

// ServerConfig is where the server listens and keeps its state.
type ServerConfig struct {
	Listen   string `toml:"listen"`
	DataDir  string `toml:"data_dir"`
	CacheDir string `toml:"cache_dir"`
}

// NodeConfig is the media node's; M2 starts the node, and M1 reads and checks it.
type NodeConfig struct {
	Roots       []string `toml:"roots"`
	FFmpegDir   string   `toml:"ffmpeg_dir"`
	Confinement string   `toml:"confinement"`
}

// LogConfig is how the server logs.
type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

// loadConfig reads and checks the config file. Keys it does not know are returned, to be
// reported and ignored, never fatal (packaging-history.md lesson 11).
func loadConfig(path string) (Config, []string, error) {
	var config Config
	metadata, err := toml.DecodeFile(path, &config)
	if err != nil {
		return Config{}, nil, fmt.Errorf("config %s: %w", path, err)
	}
	undecoded := metadata.Undecoded()
	var unknown []string
	for index, key := range undecoded {
		// An unknown table is listed before its keys; report the keys, which say more.
		if index+1 < len(undecoded) && strings.HasPrefix(undecoded[index+1].String(), key.String()+".") {
			continue
		}
		unknown = append(unknown, key.String())
	}
	if err = requireEveryKey(metadata); err != nil {
		return Config{}, unknown, fmt.Errorf("config %s: %w", path, err)
	}
	if err = config.check(); err != nil {
		return Config{}, unknown, fmt.Errorf("config %s: %w", path, err)
	}
	return config, unknown, nil
}

// The file's tables.
const (
	tableServer = "server"
	tableNode   = "node"
	tableLog    = "log"
)

// requiredKeys is every key of the file. None has a default in code (D200); a missing one is
// refused by name.
func requiredKeys() [][]string {
	return [][]string{
		{tableServer, "listen"},
		{tableServer, "data_dir"},
		{tableServer, "cache_dir"},
		{tableNode, "roots"},
		{tableNode, "ffmpeg_dir"},
		{tableNode, "confinement"},
		{tableLog, "level"},
		{tableLog, "format"},
	}
}

func requireEveryKey(metadata toml.MetaData) error {
	var missing []error
	for _, key := range requiredKeys() {
		if !metadata.IsDefined(key...) {
			missing = append(missing, fmt.Errorf("%s is required", strings.Join(key, ".")))
		}
	}
	return errors.Join(missing...)
}

// check refuses, naming the key, a value the server cannot start without or cannot use.
func (config *Config) check() error {
	var problems []error
	for key, value := range map[string]string{
		"server.data_dir": config.Server.DataDir, "server.cache_dir": config.Server.CacheDir,
	} {
		if value != "" && !filepath.IsAbs(value) {
			problems = append(problems, fmt.Errorf("%s must be an absolute path, not %q", key, value))
		}
	}
	if _, _, err := net.SplitHostPort(config.Server.Listen); config.Server.Listen != "" && err != nil {
		problems = append(problems, fmt.Errorf("server.listen %q: want host:port", config.Server.Listen))
	}
	if config.Node.Confinement != "require" && config.Node.Confinement != "report" {
		problems = append(problems, fmt.Errorf("node.confinement %q: want \"require\" or \"report\"",
			config.Node.Confinement))
	}
	if _, err := parseLevel(config.Log.Level); err != nil {
		problems = append(problems, err)
	}
	if _, err := slogx.ParseFormat(config.Log.Format); err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
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
