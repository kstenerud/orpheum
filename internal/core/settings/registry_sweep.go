// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import "time"

// The settings that were constants until the constants sweep (the owner, 2026-10-20: "preferring to
// have them configurable db values unless there's a solid reason not to";
// docs/contributors/design/research/constants-sweep-2026-10-20.md), by concern.

// The constants sweep's keys.
const (
	DiscoveryPendingCheckEverySeconds     Key = "discovery_pending_check_every_seconds"
	DiscoveryAddressCheckEverySeconds     Key = "discovery_address_check_every_seconds"
	DiscoveryAnotherCopyCheckEverySeconds Key = "discovery_another_copy_check_every_seconds"
	DiscoveryEndedCheckEverySeconds       Key = "discovery_ended_check_every_seconds"
	DiscoveryRetryFirstSeconds            Key = "discovery_retry_first_seconds"
	DiscoveryRetryMostSeconds             Key = "discovery_retry_most_seconds"
	SignInFailureLimitNetworkMultiple     Key = "signin_failure_limit_network_multiple"
	SignInFailureLimitEverywhereMultiple  Key = "signin_failure_limit_everywhere_multiple"
	SignInEverywherePaceSeconds           Key = "signin_everywhere_pace_seconds"
	PairingCodesPerNetworkMost            Key = "pairing_codes_per_network_most"
	PairingCodesPerSiteMost               Key = "pairing_codes_per_site_most"
	DeviceCodeRevealWaitSeconds           Key = "device_code_reveal_wait_seconds"
	JobLongestRetryDelayHours             Key = "job_longest_retry_delay_hours"
	UploadFilmsSharePercent               Key = "upload_films_share_percent"
	UploadCrowdedSlowUnderPercent         Key = "upload_crowded_slow_under_percent"
	UploadCrowdedSlowPlayersAtLeast       Key = "upload_crowded_slow_players_at_least"
	UploadCrowdedKeepsPercent             Key = "upload_crowded_keeps_percent"
	UploadCrowdedLowerEverySeconds        Key = "upload_crowded_lower_every_seconds"
	UploadCrowdedFloorPercent             Key = "upload_crowded_floor_percent"
	UploadUncrowdedForSeconds             Key = "upload_uncrowded_for_seconds"
	UploadUncrowdedGrowsPercent           Key = "upload_uncrowded_grows_percent"
	UploadUncrowdedRaiseEverySeconds      Key = "upload_uncrowded_raise_every_seconds"
	UploadRaiseUnderPercent               Key = "upload_raise_under_percent"
	UploadRaiseRoomSeconds                Key = "upload_raise_room_seconds"
	UploadSendAwayAfterSeconds            Key = "upload_send_away_after_seconds"
	ChunkInterestLapsesAfterSeconds       Key = "chunk_interest_lapses_after_seconds"
	ChunkCPUCeilingKeptSeconds            Key = "chunk_cpu_ceiling_kept_seconds"
	LinkSpeedFreshHours                   Key = "link_speed_fresh_hours"
	ChunkLookaheadSeconds                 Key = "chunk_lookahead_seconds"
	ChunkRunSpanSeconds                   Key = "chunk_run_span_seconds"
	ChunkRunSpeedWindowSeconds            Key = "chunk_run_speed_window_seconds"
	TogetherSnapSeeksBeyondSeconds        Key = "together_snap_seeks_beyond_seconds"
	PortMappingLeaseSeconds               Key = "port_mapping_lease_seconds"
	PortMappingRetrySeconds               Key = "port_mapping_retry_seconds"
	PortMappingPermanentRenewSeconds      Key = "port_mapping_permanent_renew_seconds"
	CertificateRenewBeforeDays            Key = "certificate_renew_before_days"
	CertificateDNSWaitSeconds             Key = "certificate_dns_wait_seconds"
	//nolint:gosec // G101: a setting's name, not a credential
	VoiceRelayCredentialHours Key = "voice_relay_credential_hours"
	JobHealthWindowHours      Key = "job_health_window_hours"
	RetentionSweepEveryHours  Key = "retention_sweep_every_hours"
	InvariantsCheckEveryHours Key = "invariants_check_every_hours"
)

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

// uploadDefinitions are how the server keeps its films within its upload, server_upload_mbps: their
// share of it, how players tell the house's upload is crowded by other traffic and how far the share
// then falls and grows back, when a lowered player is raised, and when the last resort sends a Watch
// Together member away (D286 §4, §5; plans/upload-budget.md §4c). One controller: tune them together.
func uploadDefinitions() []Definition {
	return []Definition{
		sourced(integerSetting(UploadFilmsSharePercent, "85", 10, 100,
			"percent of server_upload_mbps the films may use: the rest is for voice, playlists, pictures and "+
				"the bursts a player loading ahead sends above its quality's average (D286 §5)"),
			"D286 §5: 85%"),
		sourced(integerSetting(UploadCrowdedSlowUnderPercent, "150", 100, 500,
			"a player is slow when the link it reports carries less than this percent of what its quality "+
				"needs: too little to load ahead of what it plays (upload-budget.md §4c)"),
			"upload-budget.md §4c: one and a half times"),
		sourced(integerSetting(UploadCrowdedSlowPlayersAtLeast, "2", 1, 20,
			"how many players must be slow at once for the house's upload to count as crowded by other "+
				"traffic: one alone is its own link, which its quality controller handles"),
			"upload-budget.md §4c: two"),
		sourced(integerSetting(UploadCrowdedKeepsPercent, "80", 10, 99,
			"percent of the films' share each lowering for a crowded upload keeps"),
			"upload-budget.md §4c: best guess"),
		sourced(durationSetting(UploadCrowdedLowerEverySeconds, time.Second, "10", 1, 300,
			"the fewest seconds between lowerings of the films' share for a crowded upload"),
			"upload-budget.md §4c: best guess, a player's report interval"),
		sourced(integerSetting(UploadCrowdedFloorPercent, "10", 1, 100,
			"the lowest a crowded upload lowers the films' share to, in percent of their whole share"),
			"upload-budget.md §4c: best guess"),
		sourced(durationSetting(UploadUncrowdedForSeconds, time.Second, "60", 5, 3600,
			"seconds without a sign of crowding before the films' share grows back"),
			"upload-budget.md §4c: best guess"),
		sourced(integerSetting(UploadUncrowdedGrowsPercent, "10", 1, 100,
			"percent the films' share grows by at each step back once the upload is no longer crowded"),
			"upload-budget.md §4c: best guess"),
		sourced(durationSetting(UploadUncrowdedRaiseEverySeconds, time.Second, "30", 1, 3600,
			"seconds between the films' share's steps back once the upload is no longer crowded"),
			"upload-budget.md §4c: best guess"),
		sourced(integerSetting(UploadRaiseUnderPercent, "75", 10, 99,
			"percent of the films' share the need must stay under, for upload_raise_room_seconds, before a "+
				"lowered playback is raised a quality: room enough that raising one does not lower it again "+
				"at once (D286 §4)"),
			"D286 §4: three quarters"),
		sourced(durationSetting(UploadRaiseRoomSeconds, time.Second, "30", 1, 600,
			"seconds the need must have had room before each raise of a lowered playback (D286 §4)"),
			"D286 §4: thirty seconds"),
		sourced(durationSetting(UploadSendAwayAfterSeconds, time.Second, "20", 5, 600,
			"seconds the need must stay over the films' share with every quality at its lowest before the "+
				"newest Watch Together member is sent away, the last resort (D286 §5)"),
			"D286 §5: twenty seconds"),
	}
}

// playbackTimingDefinitions are how long the server holds what its players tell it.
func playbackTimingDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(ChunkInterestLapsesAfterSeconds, time.Second, "120", 30, 1800,
			"seconds a playing player's interest in its film's chunks lasts with it silent: no word, no "+
				"chunk asked for, no report. A playing player asks every few seconds and reports every ten, "+
				"so a crash or a lost link holds an encoder at most this long (D309 §3)"),
			"D309 §3: two minutes"),
		sourced(durationSetting(ChunkCPUCeilingKeptSeconds, time.Second, "120", 10, 3600,
			"seconds a playback moved off a quality its node could not make fast enough keeps to the one it "+
				"was moved to: long enough for the node's other runs to settle, after which it may climb "+
				"again (D309 §6, D286 §4)"),
			"D286 §4: two minutes"),
		sourced(durationSetting(LinkSpeedFreshHours, time.Hour, "24", 1, 720,
			"hours a measured link speed stands for its player on its network, after which a link may have "+
				"changed for a reason no address shows, and a new measurement is waited for (D257 §1)"),
			"D257 §1: a day, \"counting for less after a day\""),
	}
}

// chunkTimingDefinitions are how far chunks are made ahead of a player and in one run, and over how
// long a run's speed is measured.
func chunkTimingDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(ChunkLookaheadSeconds, time.Second, "60", 10, 600,
			"seconds of film past a chunk a player asks for that are made or on their way before the run "+
				"waits for the player to come closer: about what hls.js buffers ahead, so a player never asks "+
				"for a chunk no run is making (D247 §10)"),
			"D247 §10: a minute, Kyoo's look-ahead (slow-links.md §4)"),
		sourced(durationSetting(ChunkRunSpanSeconds, time.Second, "300", 30, 3600,
			"the most seconds of film one run of FFmpeg makes, and at least chunk_lookahead_seconds: a seek "+
				"starts encoding where the player is rather than waiting for a long run elsewhere (D247 §10)"),
			"D247 §10: five minutes, as Kyoo's runs"),
		sourced(durationSetting(ChunkRunSpeedWindowSeconds, time.Second, "20", 5, 300,
			"seconds back a run's speed is measured over, half of them sampled before it is judged, and "+
				"how long after an eviction its node is left to settle before another (D250 §6)"),
			"D250 §6: twenty seconds"),
	}
}

// togetherSweepDefinitions are Watch Together's server-side values the constants sweep made settings.
func togetherSweepDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(TogetherSnapSeeksBeyondSeconds, time.Second, "30", 5, 600,
			"seconds a Watch Together seek must jump to be snapped to a keyframe: a jump along the timeline "+
				"is imprecise, but a short skip by a key or a button snapped back would never move on (D285 §1)"),
			"D285 §1: thirty seconds"),
	}
}

// portMappingDefinitions are how the core asks the home router to open its HTTPS port (D273 §7).
func portMappingDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(PortMappingLeaseSeconds, time.Second, "3600", 120, 86400,
			"seconds each port mapping is asked of the router for, renewed at half of what it grants (D273 §7)"),
			"best guess: an hour, a common lease"),
		sourced(durationSetting(PortMappingRetrySeconds, time.Second, "300", 10, 86400,
			"seconds before asking the router again when none took a mapping (D273 §7)"),
			"best guess: five minutes"),
		sourced(durationSetting(PortMappingPermanentRenewSeconds, time.Second, "1800", 60, 86400,
			"seconds between asks again for a mapping the router made for good, in case it restarted and "+
				"forgot it (D273 §7)"),
			"best guess: half an hour"),
	}
}

// certificateDefinitions are how the server's certificates are obtained and renewed (D243 §6).
func certificateDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(CertificateRenewBeforeDays, 24*time.Hour, "30", 1, 60,
			"days before it expires a certificate is renewed, so a renewal that fails has days of daily "+
				"tries before it matters (D243 §6)"),
			"Let's Encrypt's advice: a third of its ninety days"),
		sourced(durationSetting(CertificateDNSWaitSeconds, time.Second, "30", 5, 600,
			"seconds a DNS-01 challenge's record is given to reach the name's servers before the CA is "+
				"asked to look: Cloudflare's answer within seconds, and a margin costs nothing every two "+
				"months (D243 §6)"),
			"D243 §6: thirty seconds"),
		sourced(durationSetting(VoiceRelayCredentialHours, time.Hour, "24", 1, 168,
			"hours a Watch Together member's voice relay credentials are good for: longer than any "+
				"evening's film, and a membership that ends first ends them with it (D243 §8)"),
			"D243 §8: a day"),
	}
}
