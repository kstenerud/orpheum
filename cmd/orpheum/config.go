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

// Config is the file read before the database opens: only what must exist by then (D182 §3).
// Everything changeable at runtime is a setting in the database. Its keys are exactly those of
// dev/orpheum.dev.toml (cut-a §2a).
type Config struct {
	Server ServerConfig `toml:"server"`
	Node   NodeConfig   `toml:"node"`
	Log    LogConfig    `toml:"log"`
	Debug  DebugConfig  `toml:"debug"`
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

// DebugConfig turns on profiling and metrics, served on the listen address only when it is
// loopback (cut-a §2a; D91 keeps both off by default).
type DebugConfig struct {
	PProf   bool `toml:"pprof"`
	Metrics bool `toml:"metrics"`
}

// The defaults a missing key takes. Each is safe to be wrong and is printed at start (D183).
const (
	defaultLogLevel    = "info"
	defaultLogFormat   = "json"
	defaultConfinement = "require" // a production install refuses to run unconfined (cut-a §2a)
)

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
	config.applyDefaults()
	if err = config.check(); err != nil {
		return Config{}, unknown, fmt.Errorf("config %s: %w", path, err)
	}
	return config, unknown, nil
}

func (config *Config) applyDefaults() {
	if config.Log.Level == "" {
		config.Log.Level = defaultLogLevel
	}
	if config.Log.Format == "" {
		config.Log.Format = defaultLogFormat
	}
	if config.Node.Confinement == "" {
		config.Node.Confinement = defaultConfinement
	}
}

// check refuses, naming the key, a value the server cannot start without or cannot use.
func (config *Config) check() error {
	var problems []error
	for key, value := range map[string]string{
		"server.listen": config.Server.Listen, "server.data_dir": config.Server.DataDir,
		"server.cache_dir": config.Server.CacheDir,
	} {
		if value == "" {
			problems = append(problems, fmt.Errorf("%s is required", key))
		}
	}
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
