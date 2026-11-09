// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"encoding/json"
	"regexp"
)

// LogLevel is the level the core's log is written at while it runs (D347): an admin turns it to
// debug to see more, and back, with no restart.
const LogLevel Key = "logLevel"

// LogLevelFromFile is LogLevel's value that leaves the level the config file's log.level names.
const LogLevelFromFile = "file"

// loggingDefinitions are the core's log.
func loggingDefinitions() []Definition {
	return []Definition{
		{
			Key: LogLevel, Kind: KindString, StartJSON: json.RawMessage(`"` + LogLevelFromFile + `"`),
			Pattern: regexp.MustCompile(`^(` + LogLevelFromFile + `|debug|info|warn|error)$`),
			Description: "the level the core's log is written at, changed at once: debug, info, warn or error; " +
				LogLevelFromFile + " is the config file's log.level (D347)",
			StartSource: "D347, the owner, 2026-10-15: log depth changed at runtime; the file's level until an " +
				"admin chooses one, so a deployment's own level holds",
		},
	}
}
