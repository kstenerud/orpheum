// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

// ReadyAheadOfNextChunkPercent is how much film a player waits to have loaded ahead before it plays:
// this percent of the time its next chunk is expected to take to load (the owner, 2026-10-28).
const ReadyAheadOfNextChunkPercent Key = "readyAheadOfNextChunkPercent"

// startDefinitions are when a player starts, and plays on after a seek.
func startDefinitions() []Definition {
	return []Definition{
		sourced(integerSetting(ReadyAheadOfNextChunkPercent, "150", 100, 1000,
			"percent of its next chunk's expected load time, that chunk's size over the link its player "+
				"measures, a player waits to have loaded ahead before it starts and before it plays on after a "+
				"seek, alone or in Watch Together; ten seconds before it can say; read by the client"),
			"the owner, 2026-10-28: outlast the next chunk's load with a margin of 1.5"),
	}
}
