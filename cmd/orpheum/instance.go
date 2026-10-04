// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// instanceFileName is the top-level config's name in an instance directory (D242 §3).
const instanceFileName = "orpheum.toml"

// fullConfigFileName is the full config's name in an instance's persistent directory (D242 §3).
const fullConfigFileName = "config.toml"

// instanceKind is what an instance runs: a core or a node (D242 §6).
type instanceKind string

// The kinds of instance.
const (
	kindCore instanceKind = "core"
	kindNode instanceKind = "node"
)

// instanceDirectoryMode is the mode default-config makes an instance's directories with: they hold
// the database, the key and the sockets, which are the instance's alone.
const instanceDirectoryMode = 0o700

// Instance is an instance directory's top-level config: what wrote it, which kind it is, and where
// its directories are, as absolute paths (D242 §3). A distribution may point them anywhere, the
// cache on another disk say; default-config puts them inside the instance directory.
type Instance struct {
	// OrpheumVersion is the release that wrote the file.
	OrpheumVersion string       `toml:"orpheum_version"`
	Kind           instanceKind `toml:"kind"`
	// PersistentDir holds what a backup keeps: the full config and, for a core, persistent.db and
	// secret.key.
	PersistentDir string `toml:"persistent_dir"`
	// CacheDir holds what can be rebuilt: cache.db and the artwork for a core, the prepared
	// renditions for a node.
	CacheDir string `toml:"cache_dir"`
	// BackupsDir is where a core's backups go, outside PersistentDir so a backup holds no earlier
	// ones; a node has none.
	BackupsDir string `toml:"backups_dir,omitempty"`
	// RunDir holds what only a running instance needs: its sockets, the server's lock and the setup
	// code.
	RunDir string `toml:"run_dir"`

	// directory is the instance directory, absolute.
	directory string
}

// paths are a core instance's directories, for instancedir.
func (instance Instance) paths() instancedir.Paths {
	return instancedir.Paths{
		Persistent: instance.PersistentDir, Cache: instance.CacheDir, Backups: instance.BackupsDir,
		Run: instance.RunDir,
	}
}

// fullConfigPath is where the instance's full config is.
func (instance Instance) fullConfigPath() string {
	return filepath.Join(instance.PersistentDir, fullConfigFileName)
}

// preparedDir is where a node instance keeps its prepared renditions (D242 §3).
func (instance Instance) preparedDir() string {
	return filepath.Join(instance.CacheDir, "prepared")
}

// loadInstance reads the top-level config of the instance directory dir and checks it is of kind.
func loadInstance(dir string, kind instanceKind) (Instance, error) {
	if !filepath.IsAbs(dir) {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return Instance{}, fmt.Errorf("instance %s: %w", dir, err)
		}
		dir = absolute
	}
	path := filepath.Join(dir, instanceFileName)
	text, err := os.ReadFile(path) //nolint:gosec // G304: the operator names the instance directory
	if errors.Is(err, os.ErrNotExist) {
		return Instance{}, fmt.Errorf("%s has no %s: it is not an instance directory; make one with "+
			"`orpheum default-config%s %s`", dir, instanceFileName, kindFlag(kind), dir)
	}
	if err != nil {
		return Instance{}, fmt.Errorf("read the instance's config: %w", err)
	}
	var instance Instance
	metadata, err := toml.Decode(string(text), &instance)
	if err != nil {
		return Instance{}, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return Instance{}, fmt.Errorf("%s: %s is not a key of the top-level config", path, undecoded[0])
	}
	instance.directory = dir
	if err = instance.check(kind); err != nil {
		return Instance{}, fmt.Errorf("%s: %w", path, err)
	}
	return instance, nil
}

// check refuses an instance of another kind, or one whose directories are not absolute.
func (instance Instance) check(kind instanceKind) error {
	if instance.Kind != kind {
		return fmt.Errorf("this is a %q instance, and `orpheum %s` runs on a %q one", instance.Kind, kind, kind)
	}
	directories := map[string]string{
		"persistent_dir": instance.PersistentDir, "cache_dir": instance.CacheDir, "run_dir": instance.RunDir,
	}
	if kind == kindCore {
		directories["backups_dir"] = instance.BackupsDir
	}
	var problems []error
	for key, value := range directories {
		if !filepath.IsAbs(value) {
			problems = append(problems, fmt.Errorf("%s must be an absolute path, not %q", key, value))
		}
	}
	return errors.Join(problems...)
}

// kindFlag is the flag default-config makes an instance of kind with.
func kindFlag(kind instanceKind) string {
	if kind == kindNode {
		return " --node"
	}
	return ""
}

// makeInstance makes an instance of kind in dir (D242 §4): its directories, the top-level config
// pointing at them, and the full config with every key, each at its starting value, and those
// with none left for the admin to fill in. Its run directory is runDir when that is set, else
// inside dir. It refuses a directory that already holds an instance, and a run directory whose
// socket's path would be too long for a Unix socket, before it makes anything.
func makeInstance(dir string, kind instanceKind, runDir, writtenBy string) (Instance, error) {
	if !filepath.IsAbs(dir) {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return Instance{}, fmt.Errorf("instance %s: %w", dir, err)
		}
		dir = absolute
	}
	if _, err := os.Stat(filepath.Join(dir, instanceFileName)); err == nil {
		return Instance{}, fmt.Errorf("%s already holds an instance (%s); default-config never overwrites one",
			dir, instanceFileName)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Instance{}, fmt.Errorf("look for an instance in %s: %w", dir, err)
	}
	inside := instancedir.PathsIn(dir)
	instance := Instance{
		OrpheumVersion: writtenBy, Kind: kind, PersistentDir: inside.Persistent, CacheDir: inside.Cache,
		RunDir: cmp.Or(runDir, inside.Run), directory: dir,
	}
	if !filepath.IsAbs(instance.RunDir) {
		return Instance{}, fmt.Errorf("the run directory %q: want an absolute path", instance.RunDir)
	}
	if err := unixsocket.CheckPath(instance.RunDir, socketNameOf(kind)); err != nil {
		return Instance{}, fmt.Errorf("%w; make the instance with --run-dir somewhere shorter, such as "+
			"/run/orpheum", err)
	}
	if kind == kindCore {
		instance.BackupsDir = inside.Backups
	}
	for _, directory := range []string{
		dir, instance.PersistentDir, instance.CacheDir, instance.BackupsDir, instance.RunDir,
	} {
		if directory == "" {
			continue
		}
		if err := os.MkdirAll(directory, instanceDirectoryMode); err != nil {
			return Instance{}, fmt.Errorf("make %s: %w", directory, err)
		}
	}
	if err := writeNewFile(instance.fullConfigPath(), defaultFullConfig(kind)); err != nil {
		return Instance{}, err
	}
	topLevel, err := toml.Marshal(instance)
	if err != nil {
		return Instance{}, fmt.Errorf("render the top-level config: %w", err)
	}
	header := "# An Orpheum instance (D242). Start it with `orpheum " + string(kind) + " --instance " + dir +
		"`.\n# The full config is " + instance.fullConfigPath() + ".\n"
	return instance, writeNewFile(filepath.Join(dir, instanceFileName), header+string(topLevel))
}

// socketNameOf is the socket an instance of kind listens on in its run directory.
func socketNameOf(kind instanceKind) string {
	if kind == kindNode {
		return nodeSocketName
	}
	return coreSocketName
}

// defaultFullConfig is a new instance's full config: every key of kind, by table, with its
// description; a key with a starting value is set to it, and one without is left commented out
// for the admin to fill in, so the instance refuses to start until it is (D202).
func defaultFullConfig(kind instanceKind) string {
	var text strings.Builder
	text.WriteString("# Orpheum's full config: what must exist before the database opens (D200).\n" +
		"# A key commented out below has no starting value and must be set before the " +
		string(kind) + " starts.\n")
	table := ""
	for _, key := range configKeys(kind) {
		if key.table != table {
			table = key.table
			fmt.Fprintf(&text, "\n[%s]\n", table)
		}
		fmt.Fprintf(&text, "# %s\n", key.description)
		if key.startingValue == "" {
			fmt.Fprintf(&text, "# %s =   (required)\n", key.name)
			continue
		}
		fmt.Fprintf(&text, "%s = %s\n", key.name, key.startingValue)
	}
	return text.String()
}

// writeNewFile writes contents to path, mode 0600, refusing a file already there.
func writeNewFile(path, contents string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: see makeInstance
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err = file.WriteString(contents); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", path, err), file.Close())
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
