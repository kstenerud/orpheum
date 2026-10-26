// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import "time"

// How long Watch Together's invitations and an idle session last (D346).
const (
	TogetherLinkLifetimeSeconds Key = "together_link_lifetime_seconds"
	TogetherIdleSessionEndHours Key = "together_idle_session_end_hours"
)

// togetherLifetimeDefinitions are how long a Watch Together link admits someone, and how long a session
// lasts with nothing playing and no one doing anything in it (D346). Settings, not constants: they are
// a household's policy, the owner's numbers a start, and another household may want an invitation
// sent earlier in the day or a longer pause kept; nothing in the code depends on their values.
func togetherLifetimeDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(TogetherLinkLifetimeSeconds, time.Second, "3600", 300, 7*24*3600,
			"seconds a Watch Together link admits the person it was made for, from when it was made; then it "+
				"admits no one, and whoever joined by it stays in (D346)"),
			"D346: the owner, 2026-10-26: \"Invitations are good for an hour\""),
		sourced(durationSetting(TogetherIdleSessionEndHours, time.Hour, "6", 1, 7*24,
			"hours a Watch Together session lasts with nothing playing to a player and no one doing anything "+
				"in it (a command, a join, a leave, a link made or withdrawn, the host opening it); then it ends "+
				"as the host's end would, and its links and memberships admit no one (D346)"),
			"D346: the owner, 2026-10-26: \"A host who forgets to close the session has it closed after 6h of "+
				"inactivity\""),
	}
}
