// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// seededMarker ends every line seed-settings writes, so a person can see which values were never
// chosen by anyone.
const seededMarker = "  # starting value, written by orpheum seed-settings"

// configSeedReport is what seeding the config file did, and what it found and left alone.
type configSeedReport struct {
	Written   []configKey // missing keys it added, with their starting values
	NeedsHand []configKey // missing keys with no starting value: specific to this machine
	Unknown   []string    // keys the file has and nothing reads: a typo, or a key a release removed
	Backup    string      // the copy of the file as it was, when anything was written
}

// seedConfigFile adds every missing key that has a starting value to the config file, keeping
// everything else — comments, order, spacing — as it was, after writing a copy of the file as it
// was beside it. A file a person wrote is rewritten only because they asked (migrations.md,
// Files), and never loses anything.
func seedConfigFile(path string, now time.Time) (configSeedReport, error) {
	var report configSeedReport
	original, err := os.ReadFile(path) //nolint:gosec // G304: the operator names the config file
	if err != nil {
		return report, fmt.Errorf("read the config file: %w", err)
	}
	metadata, err := toml.Decode(string(original), &Config{})
	if err != nil {
		return report, fmt.Errorf("config %s: %w", path, err)
	}
	report.Unknown = unknownKeys(metadata)
	for _, key := range missingKeys(metadata) {
		if key.start == "" {
			report.NeedsHand = append(report.NeedsHand, key)
		} else {
			report.Written = append(report.Written, key)
		}
	}
	if len(report.Written) == 0 {
		return report, nil
	}
	seeded, err := insertKeys(string(original), report.Written)
	if err != nil {
		return report, err
	}
	report.Backup = path + ".before-seed-" + now.UTC().Format("20060102T150405Z")
	err = writeNew(report.Backup, original)
	if err != nil {
		return report, err
	}
	if err = os.WriteFile(path, []byte(seeded), 0o600); err != nil { //nolint:gosec // G703: the operator's own file
		return report, fmt.Errorf("write the config file (the original is in %s): %w", report.Backup, err)
	}
	return report, nil
}

// insertKeys adds each key into its table: after the table's last line, or in a new table at the
// end when the file has none. It checks the result still parses and now defines every key.
func insertKeys(text string, keys []configKey) (string, error) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, table := range tablesOf(keys) {
		var added []string
		for _, key := range keys {
			if key.table == table {
				added = append(added, key.name+" = "+key.start+seededMarker)
			}
		}
		lines = insertIntoTable(lines, table, added)
	}
	seeded := strings.Join(lines, "\n") + "\n"
	metadata, err := toml.Decode(seeded, &Config{})
	if err != nil {
		return "", fmt.Errorf("the seeded config does not parse, so nothing was written: %w", err)
	}
	for _, key := range keys {
		if !metadata.IsDefined(key.table, key.name) {
			return "", fmt.Errorf("the seeded config still lacks %s, so nothing was written", key.path())
		}
	}
	return seeded, nil
}

// tablesOf lists the tables keys belong to, in the order configKeys gives them.
func tablesOf(keys []configKey) []string {
	var tables []string
	for _, key := range keys {
		if len(tables) == 0 || tables[len(tables)-1] != key.table {
			tables = append(tables, key.table)
		}
	}
	return tables
}

var tableHeader = regexp.MustCompile(`^\s*\[\s*([A-Za-z0-9_.-]+)\s*\]\s*(#.*)?$`)

// insertIntoTable puts added after the last non-blank line of table, or appends the table.
func insertIntoTable(lines []string, table string, added []string) []string {
	start := -1
	for index, line := range lines {
		if match := tableHeader.FindStringSubmatch(line); len(match) > 1 && match[1] == table {
			start = index
			break
		}
	}
	if start < 0 {
		return append(append(lines, "", "["+table+"]"), added...)
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "[") {
			end = index
			break
		}
	}
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	result := append([]string{}, lines[:end]...)
	result = append(result, added...)
	return append(result, lines[end:]...)
}

// writeNew writes a file that must not exist yet, mode 0600.
func writeNew(path string, contents []byte) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: beside the config
	if err != nil {
		return fmt.Errorf("keep a copy of the config file: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err = file.Write(contents); err != nil {
		return fmt.Errorf("keep a copy of the config file: %w", err)
	}
	return nil
}
