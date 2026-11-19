// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"encoding/json"
	"regexp"
)

// LogLevel is the level the core's log and each node's are written at while they run (D347, D525):
// an admin turns it to debug to see more, and back, with no restart.
const LogLevel Key = "logLevel"

// LogLevelFromFile is LogLevel's value that leaves the level each process's config file's
// log.level names.
const LogLevelFromFile = "file"

// loggingDefinitions are the core's and the nodes' logs.
func loggingDefinitions() []Definition {
	return []Definition{
		{
			Key: LogLevel, Kind: KindString, StartJSON: json.RawMessage(`"` + LogLevelFromFile + `"`),
			Pattern: regexp.MustCompile(`^(` + LogLevelFromFile + `|debug|info|warn|error)$`),
			Description: "the level the core's and every node's log are written at, changed at once, a node's " +
				"with its next poll: debug, info, warn or error; " + LogLevelFromFile + " is each one's config " +
				"file's log.level (D347, D525)",
			StartSource: "D347, the owner, 2026-10-15: log depth changed at runtime; the file's level until an " +
				"admin chooses one, so a deployment's own level holds",
		},
	}
}
