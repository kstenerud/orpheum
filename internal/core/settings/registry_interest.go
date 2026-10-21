// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import "time"

// The settings of work following interest (D324; plans/interest-priority.md).
const (
	InterestSaidEverySeconds   Key = "interest_said_every_seconds"
	InterestLapsesAfterSeconds Key = "interest_lapses_after_seconds"
)

// interestDefinitions are how a person's interest in a film or series is said and how long it
// lasts.
func interestDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(InterestSaidEverySeconds, time.Second, "15", 5, 120,
			"seconds between a shown film's or series' page saying so to the server, which raises its work "+
				"(D324 §1); read by the client"),
			"D324: a few times within interest_lapses_after_seconds"),
		sourced(durationSetting(InterestLapsesAfterSeconds, time.Second, "45", 15, 600,
			"seconds after a page or a playback last said so that its interest lapses and its work falls to "+
				"the shown-interest rung (D324 §1)"),
			"D324: three missed words, as a node's silence (cut-a §4)"),
	}
}
