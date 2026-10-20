// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import "time"

// The settings that were constants until the constants sweep (the owner, 2026-10-20: "preferring to
// have them configurable db values unless there's a solid reason not to";
// docs/contributors/design/research/constants-sweep-2026-10-20.md), by concern.

// upkeepSweepDefinitions are how often the core's upkeep looks, and how far back the job view counts:
// each was a constant until the constants sweep (the owner, 2026-10-20).
func upkeepSweepDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(JobLongestRetryDelayHours, time.Hour, "24", 1, 168,
			"the longest a failing job's retry waits: its doubling back-off stops there, and a job that "+
				"keeps failing retries about this often until job_attempt_limit (D231 §10)"),
			"D231: a day, as job_first_retry_delay_seconds' description says"),
		sourced(durationSetting(JobHealthWindowHours, time.Hour, "24", 1, 720,
			"hours back the System page's job health counts each kind's failed jobs (D233 §8)"),
			"D233 §8: a day"),
		sourced(durationSetting(RetentionSweepEveryHours, time.Hour, "24", 1, 168,
			"hours between the sweeps that forget what is kept a number of days past its days: ended "+
				"credentials and removed files (D304 §6)"),
			"D304 §6: daily, as the limits are in days"),
		sourced(durationSetting(InvariantsCheckEveryHours, time.Hour, "24", 1, 720,
			"hours between the checks of the database's invariants, beside the one at start (D303)"),
			"D303 (M7): daily, a few full reads of tables at this scale"),
	}
}

// discoveryDefinitions are the core's side of its discovery: how often it looks, in each state. The
// minimums of those that call the discovery protect the service from a core asking too often.
func discoveryDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(DiscoveryPendingCheckEverySeconds, time.Second, "2", 1, 600,
			"seconds between a core's asks of its discovery whether it is claimed yet, while it waits for "+
				"its owner to type its claim code there: the longest the owner waits after typing it"),
			"the owner, 2026-10-20: 10 s kept each claim waiting, the owner and the browser tests alike; a "+
				"status call is small, and a code waits minutes at most"),
		sourced(durationSetting(DiscoveryAddressCheckEverySeconds, time.Second, "60", 10, 300,
			"seconds between a claimed core's looks whether its addresses changed, sending them to its "+
				"discovery at once when they did; at most the 5 minutes between its heartbeats"),
			"discovery-core.md C2: a minute; a look sends only on a change, so it costs the discovery nothing"),
		sourced(durationSetting(DiscoveryAnotherCopyCheckEverySeconds, time.Second, "180", 30, 3600,
			"seconds between the asks of a core its discovery refused as a copy of another still "+
				"connected: when the other stops, this one takes its place (D293 §1)"),
			"D293 §1: three minutes"),
		sourced(durationSetting(DiscoveryEndedCheckEverySeconds, time.Second, "3600", 300, 86400,
			"seconds between the asks of a core whose discovery has ended, or that has none, in case its "+
				"owner pays again"),
			"best guess: hourly, as the owner paying again is rare and a minute's wait no one notices"),
		sourced(durationSetting(DiscoveryRetryFirstSeconds, time.Second, "1", 1, 60,
			"seconds before calling the discovery again after a failed call, or dialling its signalling "+
				"connection again after it dropped; each failure in a row doubles it, jittered, to "+
				"discovery_retry_most_seconds (the protocol's §2; C7)"),
			"the config file's retry_first_milliseconds until the constants sweep, 2026-10-20"),
		sourced(durationSetting(DiscoveryRetryMostSeconds, time.Second, "300", 1, 3600,
			"the longest wait before calling the discovery again after failures in a row; below "+
				"discovery_retry_first_seconds it waits that. The development stack keeps it a second "+
				"(D292 §9)"),
			"the config file's retry_most_milliseconds until the constants sweep, 2026-10-20"),
	}
}

// signInStretchDefinitions are how far the failed sign-in limit stretches beyond one person on one
// network (the second audit's K2, the third's A13).
func signInStretchDefinitions() []Definition {
	return []Definition{
		sourced(integerSetting(SignInFailureLimitNetworkMultiple, "4", 1, 100,
			"how many times signin_failure_limit one network may fail to sign in within its window: a "+
				"household or a carrier's shared address is many people's failures (K2)"),
			"the second audit's K2, 2026-10-13: four"),
		sourced(integerSetting(SignInFailureLimitEverywhereMultiple, "10", 1, 1000,
			"how many times signin_failure_limit a person may fail to sign in from every network at once "+
				"within its window before each of their sign-ins waits signin_everywhere_pace_seconds; a "+
				"device that signed in before is not held to it (A13)"),
			"the second audit's K2, 2026-10-13: ten"),
		sourced(durationSetting(SignInEverywherePaceSeconds, time.Second, "10", 1, 300,
			"seconds each sign-in of a person waits once they have failed past "+
				"signin_failure_limit_everywhere_multiple: a guesser on many networks makes a few guesses a "+
				"minute, and the person, who signs in once, waits seconds (A13)"),
			"the third audit's A13: ten seconds"),
	}
}

// pairingDefinitions are how a device not signed in gets to be: the codes one network and one site may
// hold, and how long a typed code waits for its device.
func pairingDefinitions() []Definition {
	return []Definition{
		sourced(integerSetting(PairingCodesPerNetworkMost, "5", 1, 100,
			"how many pairing codes not yet entered one network may hold at once (an IPv6 /64 or an IPv4 "+
				"address); past it a new one is refused, never an older one dropped (S13, D288 §6)"),
			"the security audit's S13, 2026-10-13: five"),
		sourced(integerSetting(PairingCodesPerSiteMost, "20", 1, 1000,
			"how many pairing codes not yet entered one site may hold at once (an IPv6 /48, which holds "+
				"many networks): twenty networks' five codes each left no television a code (K5)"),
			"the second audit's K5, 2026-10-13: twenty"),
		sourced(durationSetting(DeviceCodeRevealWaitSeconds, time.Second, "15", 3, 120,
			"seconds entering a typed device code waits for the device to reveal its nonce; the device "+
				"asks every two seconds, so at least three (D321)"),
			"D321: fifteen seconds"),
	}
}
