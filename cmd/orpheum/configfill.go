// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// startingValueMarker ends every line added in memory for a key the file lacks. Nothing writes it
// to the file: a file a person wrote is never rewritten unasked (migrations.md, Files).
const startingValueMarker = "  # starting value; not in the file"

// withStartingValues adds each key, with its starting value, into its table in the file's text:
// after the table's last line, or in a new table at the end. The result is parsed in memory, never
// written (D202). It checks the result still parses and now defines every key.
func withStartingValues(text string, keys []configKey) (string, error) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, table := range tablesOf(keys) {
		var added []string
		for _, key := range keys {
			if key.table == table {
				added = append(added, key.name+" = "+key.startingValue+startingValueMarker)
			}
		}
		lines = insertIntoTable(lines, table, added)
	}
	seeded := strings.Join(lines, "\n") + "\n"
	metadata, err := toml.Decode(seeded, &Config{})
	if err != nil {
		return "", fmt.Errorf("the config with its starting values added does not parse: %w", err)
	}
	for _, key := range keys {
		if !metadata.IsDefined(key.table, key.name) {
			return "", fmt.Errorf("the config with its starting values added still lacks %s", key.path())
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
