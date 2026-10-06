// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
	"github.com/kstenerud/orpheum/internal/platform/localtime"
)

// Key names a setting. The set is closed: every key is declared in Registry.
type Key string

// The settings. Every opinionated value the server acts on is one of these, so each choice is a
// row that can be read and changed, not a default in code (D200). Units are in the name.
const (
	ShouldCheckForUpdates                          Key = "should_check_for_updates"
	SignInFailureLimit                             Key = "signin_failure_limit"
	SignInFailureWindowSeconds                     Key = "signin_failure_window_seconds"
	SessionIdleExpiryHours                         Key = "session_idle_expiry_hours"
	SessionLastUsedWriteIntervalSeconds            Key = "session_last_used_write_interval_seconds"
	Argon2MemoryKiB                                Key = "argon2_memory_kib"
	Argon2Iterations                               Key = "argon2_iterations"
	Argon2Parallelism                              Key = "argon2_parallelism"
	SecretLastCharsShown                           Key = "secret_last_chars_shown"
	BackupAgeWarningHours                          Key = "backup_age_warning_hours"
	ShouldServeDebugPProf                          Key = "should_serve_debug_pprof"
	ShouldServeDebugMetrics                        Key = "should_serve_debug_metrics"
	NodeSilentAfterSeconds                         Key = "node_silent_after_seconds"
	AssignedJobLimitHours                          Key = "assigned_job_limit_hours"
	ShouldScanOnNodeHello                          Key = "should_scan_on_node_hello"
	SpinningDiskReaderLimit                        Key = "spinning_disk_reader_limit"
	SSDReaderLimit                                 Key = "ssd_reader_limit"
	NetworkShareReaderLimit                        Key = "network_share_reader_limit"
	UnknownDiskReaderLimit                         Key = "unknown_disk_reader_limit"
	NodeJobsBeyondFreeSlots                        Key = "node_jobs_beyond_free_slots"
	PlayPriorityKeptSeconds                        Key = "play_priority_kept_seconds"
	ScanWithholdRemovalsOverPercent                Key = "scan_withhold_removals_over_percent"
	ScanWithholdRemovalsOverFiles                  Key = "scan_withhold_removals_over_files"
	ScanVideoExtensions                            Key = "scan_video_extensions"
	ScanSubtitleExtensions                         Key = "scan_subtitle_extensions"
	ScanVideosPerExamineBatch                      Key = "scan_videos_per_examine_batch"
	JobAttemptLimit                                Key = "job_attempt_limit"
	FinishedJobsKeptDays                           Key = "finished_jobs_kept_days"
	JobFirstRetryDelaySeconds                      Key = "job_first_retry_delay_seconds"
	ProviderTimeoutSeconds                         Key = "provider_timeout_seconds"
	DestinationFirstProbeDelaySeconds              Key = "destination_first_probe_delay_seconds"
	DestinationLongestProbeDelaySeconds            Key = "destination_longest_probe_delay_seconds"
	TMDBRequestsPerSecond                          Key = "tmdb_requests_per_second"
	TMDBBackgroundRateSharePercent                 Key = "tmdb_background_rate_share_percent"
	OpenSubtitlesRequestsPerSecond                 Key = "opensubtitles_requests_per_second"
	OpenSubtitlesDownloadsReservedForPeoplePercent Key = "opensubtitles_downloads_reserved_for_people_percent"
	SortLocale                                     Key = "sort_locale"
	ArtworkMaxSourcePixels                         Key = "artwork_max_source_pixels"
	ArtworkWebPQuality                             Key = "artwork_webp_quality"
	TMDBRefreshAfterDays                           Key = "tmdb_refresh_after_days"
	MetadataRefreshCheckEveryHours                 Key = "metadata_refresh_check_every_hours"
	ArtworkCacheCeilingGiB                         Key = "artwork_cache_ceiling_gib"
	RemovedItemArtworkKeptDays                     Key = "removed_item_artwork_kept_days"
	MediaTokenIdleExpiryHours                      Key = "media_token_idle_expiry_hours"
	HLSSegmentLengthSeconds                        Key = "hls_segment_length_seconds"
	ChunkQualities                                 Key = "chunk_qualities"
	ResumeMinPercent                               Key = "resume_min_percent"
	FinishedAfterPercent                           Key = "finished_after_percent"
	RegistrationInvitationLifeDays                 Key = "registration_invitation_life_days"
	NewLibraryDefaultScanEveryHours                Key = "new_library_default_scan_every_hours"
	LoudnessTargetLUFS                             Key = "loudness_target_lufs"
	LoudnessMaxRaiseDB                             Key = "loudness_max_raise_db"
	ReadThroughSweepEveryHours                     Key = "read_through_sweep_every_hours"
	DerivedWorkSweepEverySeconds                   Key = "derived_work_sweep_every_seconds"
	BackupEveryDays                                Key = "backup_every_days"
	BackupHourLocalTZ                              Key = "backup_hour_local_tz"
	TimeZone                                       Key = "time_zone"
	BackupsKept                                    Key = "backups_kept"
	ArtworkJPEGQuality                             Key = "artwork_jpeg_quality"
	ArtworkSecondsBeforeEachRetry                  Key = "artwork_seconds_before_each_retry"
	ArtworkCacheSweepEveryHours                    Key = "artwork_cache_sweep_every_hours"
	ArtworkServedAtFlushEverySeconds               Key = "artwork_served_at_flush_every_seconds"
	ContinueWatchingMaxWorks                       Key = "continue_watching_max_works"
	JobViewMaxJobs                                 Key = "job_view_max_jobs"
	SystemPageRecentPreparesPerNode                Key = "system_page_recent_prepares_per_node"
	OfflineReminderIntervalDays                    Key = "offline_reminder_interval_days"
	ScreenRefreshFirstWaitSeconds                  Key = "screen_refresh_first_wait_seconds"
	ScreenRefreshLongestWaitSeconds                Key = "screen_refresh_longest_wait_seconds"
	ScreenRefreshFailedWaitSeconds                 Key = "screen_refresh_failed_wait_seconds"
	TogetherDriftLeftAloneMs                       Key = "together_drift_left_alone_ms"
	TogetherNudgeMostPercent                       Key = "together_nudge_most_percent"
	TogetherSeekDriftBeyondMs                      Key = "together_seek_drift_beyond_ms"
	ChunkSoundKbps                                 Key = "chunk_sound_kbps"
	TogetherSyncWaitSeconds                        Key = "together_sync_wait_seconds"
	TogetherCatchUpBufferPercent                   Key = "together_catch_up_buffer_percent"
	ChunkHoldMs                                    Key = "chunk_hold_ms"
)

// Kind is a setting value's JSON type.
type Kind string

// The kinds a setting may have.
const (
	KindBool    Kind = "bool"
	KindInteger Kind = "integer"
	KindString  Kind = "string"
)

// Definition is one setting: its type and bounds, the value a new server starts with, and
// whether the settings page shows it. The starting value is written into the database on first
// start and read from there ever after (D200); a question starts as JSON null and is asked.
type Definition struct {
	Key       Key
	Kind      Kind
	StartJSON json.RawMessage
	Minimum   int64 // integers only
	Maximum   int64 // integers only
	// Pattern, for a string, is what every value must match; nil takes any string.
	Pattern *regexp.Regexp
	// IsValid, for a string, is what a value must also pass that no pattern can say; nil takes any.
	IsValid func(text string) bool
	// Unit, for an integer that is a length of time, is what one of it is: the unit its key's
	// suffix spells out (_seconds, _hours, _days). Duration reads such a setting; zero is not a length of time.
	Unit        time.Duration
	Question    string
	IsExposed   bool
	Description string
	// StartSource says where the starting value came from: the owner's choice on a date, a best
	// guess and its basis, what was measured and when, or a provider's documentation. The owner
	// tunes values in use, and a good one becomes the starting value.
	StartSource string
}

// LengthOf is count of the setting's unit as a time.Duration; zero for a setting with no unit.
func (definition Definition) LengthOf(count int64) time.Duration {
	return time.Duration(count) * definition.Unit
}

// IsQuestion reports whether the setting starts unanswered.
func (definition Definition) IsQuestion() bool {
	return string(definition.StartJSON) == "null"
}

// Registry is every setting. The starting values are the opinions: each says why it is what it
// is, or names the decision that chose it.
func Registry() []Definition {
	return slices.Clone(built().definitions)
}

// builtRegistry is the registry, and the same by key for Lookup.
type builtRegistry struct {
	definitions []Definition
	byKey       map[Key]Definition
}

// built is the registry, built once: building it compiles patterns, which every read of a setting
// paid for while Lookup built it each time (2 ms and 1.5 MB a read, under the owner's stalled
// playback, 2026-10-05).
//
//nolint:gochecknoglobals // the registry is constant, and Go has no constant of this type: built once, never written
var built = sync.OnceValue(func() builtRegistry {
	definitions := buildRegistry()
	byKey := make(map[Key]Definition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	return builtRegistry{definitions: definitions, byKey: byKey}
})

// buildRegistry makes every setting's definition.
func buildRegistry() []Definition {
	return append([]Definition{
		{
			Key: ShouldCheckForUpdates, Kind: KindBool, StartJSON: json.RawMessage("null"),
			IsExposed:   true,
			Question:    "Check GitHub for a new release once a day? The check sends nothing but the request.",
			Description: "whether the server checks for a new release (D184: asked, never assumed)",
			StartSource: "D184: a question, asked and never assumed, so it starts unanswered",
		},
		sourced(integerSetting(SignInFailureLimit, "5", 1, 1000,
			"failed sign-ins per account, and per address, before sign-in is refused for the window (D194 §2)"),
			"best guess (M1): a person mistyping gets several tries; D194 §2 asks only for a limit"),
		sourced(durationSetting(SignInFailureWindowSeconds, time.Second, "900", 1, 7*24*3600,
			"how long, in seconds, a failed sign-in counts toward the limit"),
			"best guess (M1): fifteen minutes, a common lock-out window"),
		sourced(durationSetting(SessionIdleExpiryHours, time.Hour, "2160", 1, 10*365*24,
			"hours a session may go unused before it ends; 90 days, so a television rarely signs out (D154)"),
			"D154: 90 days, so a television rarely signs out"),
		sourced(durationSetting(SessionLastUsedWriteIntervalSeconds, time.Second, "60", 1, 24*3600,
			"how often, in seconds, a session's last use is written; idle expiry needs no finer grain"),
			"best guess (M1): a minute is far finer than a 90-day idle expiry needs"),
		sourced(integerSetting(Argon2MemoryKiB, "65536", 8, math.MaxUint32,
			"argon2id memory per password hash, KiB: RFC 9106's second option (D194 §2); new costs re-hash on sign-in"),
			"D194 §2: RFC 9106's second recommended option, 64 MiB; measured 41 ms on an i3-1315U"),
		sourced(integerSetting(Argon2Iterations, "3", 1, math.MaxUint32, "argon2id passes per password hash (D194 §2)"),
			"D194 §2: RFC 9106's second recommended option, t = 3"),
		sourced(integerSetting(Argon2Parallelism, "4", 1, math.MaxUint8, "argon2id lanes per password hash (D194 §2)"),
			"D194 §2: RFC 9106's second recommended option, p = 4"),
		sourced(integerSetting(SecretLastCharsShown, "4", 0, 8,
			"trailing characters of an API key shown, to tell which key is set; a password shows none (D194 §3)"),
			"D194 §3: the last four characters of an API key"),
		sourced(durationSetting(BackupAgeWarningHours, time.Hour, "168", 1, 365*24,
			"hours after which doctor says the newest backup is old"),
			"best guess (M1): a week, as a nightly backup that stopped would show"),
		{
			Key: ShouldServeDebugPProf, Kind: KindBool, StartJSON: json.RawMessage("false"),
			Description: "serve /debug/pprof, on a loopback listen address only (D91); read at start",
			StartSource: "D91: off by default, local when on",
		},
		{
			Key: ShouldServeDebugMetrics, Kind: KindBool, StartJSON: json.RawMessage("false"),
			Description: "serve /metrics, on a loopback listen address only (D91); not built yet",
			StartSource: "D91: off by default, local when on",
		},
		sourced(durationSetting(NodeSilentAfterSeconds, time.Second, "30", 1, 24*3600,
			"seconds without a record from a node before the core shows it as silent (cut-a §4)"),
			"cut-a §4: silent after 30 s with no record, three status intervals"),
		sourced(durationSetting(AssignedJobLimitHours, time.Hour, "24", 1, 24*30,
			"hours a job given to a node may stay without its outcome before it is its own failure (D234 §7)"),
			"the owner, 2026-10-02: a backstop; silence and the node's own list find a lost job first"),
		{
			Key: ShouldScanOnNodeHello, Kind: KindBool, StartJSON: json.RawMessage("true"),
			Description: "whether a node is asked to scan each of its roots a library covers each time it says hello",
			StartSource: "best guess (M2): a node that says hello may have been away while its files changed",
		},
		sourced(integerSetting(ScanWithholdRemovalsOverPercent, "50", 1, 100,
			"a scan that would remove more than this percentage of a root's files waits for the admin (D208)"),
			"D208 §3: the owner's half, 2026-09-24"),
		sourced(integerSetting(ScanWithholdRemovalsOverFiles, "20", 0, math.MaxInt32,
			"...and more than this many of them; a scan that would remove all of them always waits (D208)"),
			"D208 §3: the owner's twenty, 2026-09-24"),
	}, slices.Concat(
		scanDefinitions(), jobMetadataAndPlaybackDefinitions(), deviceDefinitions(), upkeepAndDisplayDefinitions(),
		backupDefinitions(), togetherDefinitions(),
	)...)
}

// togetherDefinitions are how a Watch Together device follows its group, sent to it in its welcome
// so they are tuned while testing (D250 §1).
func togetherDefinitions() []Definition {
	return []Definition{
		sourced(integerSetting(TogetherDriftLeftAloneMs, "500", 0, 10_000,
			"ms a Watch Together player may be off the group's position before it is corrected (D250 §1)"),
			"D250 §1: the owner, 2026-10-06: \"I'm fine with leaving up to 500ms instead of the 60ms\""),
		sourced(integerSetting(TogetherNudgeMostPercent, "10", 1, 100,
			"percent of the group's rate a Watch Together player's nudge plays faster or slower by at most, "+
				"to close a drift too small to seek across (D250 §1)"),
			"D250 §1: the owner, 2026-10-06: \"10% max speed drift also sounds fine\""),
		sourced(integerSetting(TogetherSeekDriftBeyondMs, "3000", 500, 600_000,
			"ms a Watch Together player may be off the group's position before it seeks rather than nudges "+
				"(D250 §1)"),
			"watch-together.md's Sync: \"seek beyond that\", 3 s, kept by D250 §1"),
		sourced(durationSetting(TogetherSyncWaitSeconds, time.Second, "20", 1, 600,
			"seconds a Watch Together group waits for its players at a sync point (a film put on, the "+
				"host's play after a pause, the host's seek) before playing without those not ready (D250 §3)"),
			"D250 §3: the owner, 2026-10-06: \"30 seconds is too long. Let's start at 20.\""),
		sourced(integerSetting(TogetherCatchUpBufferPercent, "80", 10, 100,
			"percent of what its player loads ahead that a Watch Together player catching up alone aims "+
				"to have loaded when the group reaches it; it jumps ahead by its link's time to load that "+
				"(D250 §4)"),
			"D250 §4: the owner, 2026-10-06: \"target an 80% full buffer by the time the group's playback "+
				"catches up\""),
	}
}

// backupDefinitions are when the server backs itself up, and how many backups it keeps (D243 §11).
func backupDefinitions() []Definition {
	return []Definition{
		exposed(sourced(durationSetting(BackupEveryDays, day, "1", 1, 365,
			"days between the backups the server makes on its own, never holding secret.key (D246 §7)"),
			"D243 §11: one a day")),
		exposed(sourced(integerSetting(BackupHourLocalTZ, "4", 0, 23,
			"the hour of the day, in the server's time_zone, a scheduled backup is made in, a quiet one; a "+
				"backup overdue by a day is made at once (D246 §7)"),
			"best guess: early morning, when no one watches; D243 §11 asks only for a quiet hour")),
		exposed(sourced(integerSetting(BackupsKept, "14", 1, 1000,
			"how many backups are kept, the newest; the copies kept before a restore or a migration are "+
				"never pruned (D243 §11)"),
			"D243 §11: the last 14")),
	}
}

// scanDefinitions are what a scan reads, and how much of it a node reads in one go (D240 §8).
func scanDefinitions() []Definition {
	return []Definition{
		sourced(Definition{
			Key: ScanVideoExtensions, Kind: KindString,
			StartJSON: json.RawMessage(`"mkv,mp4,m4v,avi,mov,wmv,ts,m2ts,mts,webm,mpg,mpeg,vob,flv,ogv,3gp"`),
			Pattern:   regexp.MustCompile(extensionsPattern),
			Description: "the extensions, lower case and comma-separated, of the files a scan reads as " +
				"videos (D240 §8)",
		}, "the containers FFmpeg reads that people keep films in: Matroska and MP4 first, the broadcast and "+
			"disc formats (ts, m2ts, mts, vob, mpg), and the older ones still found in libraries (avi, wmv, flv, "+
			"3gp); a file with another extension is never probed, so a missing one hides films, and an extra "+
			"one costs a probe of each such file. Carried over from the node's video_extensions"),
		sourced(Definition{
			Key: ScanSubtitleExtensions, Kind: KindString, StartJSON: json.RawMessage(`"srt,ass,ssa,vtt"`),
			Pattern: regexp.MustCompile(extensionsPattern),
			Description: "the extensions, lower case and comma-separated, of the text subtitle files a scan " +
				"reads beside their videos (D240 §8)",
		}, "the text subtitle formats the node converts to ASS and WebVTT (D217 §6): SubRip, ASS and SSA, and "+
			"WebVTT; image subtitles are read inside a video, not beside it. Carried over from the node's "+
			"subtitle_extensions"),
		sourced(integerSetting(ScanVideosPerExamineBatch, "10", 1, 1000,
			"how many videos of one folder a node reads in one go, each with the files beside it; a lost one "+
				"is read again (D240 §8)"),
			"the owner's choice, 2026-10-03: a folder of a few dozen loose films is a few of them"),
	}
}

// deviceDefinitions are how many may read a node's device of each kind at once, and how much more
// than it can start the core gives a node (D240 §5).
func deviceDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(PlayPriorityKeptSeconds, time.Second, "600", 10, 24*3600,
			"seconds after a play last asked for its work that it keeps play priority; past them, it is "+
				"background work and pauses nothing (D234 §3)"),
			"the owner, 2026-10-01: question C, ten minutes"),
		sourced(integerSetting(NodeJobsBeyondFreeSlots, "1", 0, 64,
			"how many more jobs than its free slots and readers allow the core gives a node of each kind, "+
				"so the next starts without waiting for the core (D240 §5)"),
			"D234 §3: a window a little beyond its free slots"),
		sourced(integerSetting(SpinningDiskReaderLimit, "1", 1, 64,
			"how many may read a spinning disk at once, or a pool or array with one among its disks (D240 §5)"),
			"D240 §5: a second reader makes a spinning disk seek between them"),
		sourced(integerSetting(SSDReaderLimit, "4", 1, 64,
			"how many may read a solid-state disk at once, or a pool or array of them only (D240 §5)"),
			"best guess (D240 §5): an SSD serves several readers without seeking"),
		sourced(integerSetting(NetworkShareReaderLimit, "2", 1, 64,
			"how many may read a filesystem mounted over the network at once (D240 §5)"),
			"best guess (D240 §5): one reading while another waits on the network"),
		sourced(integerSetting(UnknownDiskReaderLimit, "1", 1, 64,
			"how many may read a device whose disks the node cannot see at once (D240 §5)"),
			"D240 §5: as for a spinning disk, which it may be"),
	}
}

// jobMetadataAndPlaybackDefinitions are the rest of Registry: jobs, fetching metadata and
// artwork, sorting, playback and resuming.
func jobMetadataAndPlaybackDefinitions() []Definition {
	return append([]Definition{
		sourced(integerSetting(JobAttemptLimit, "5", 1, 100,
			"how many times a background job that failed is tried before it is marked failed"),
			"best guess (M2): enough to ride out a short outage, few enough to stop a hopeless job"),
		sourced(durationSetting(FinishedJobsKeptDays, 24*time.Hour, "30", 1, 3650,
			"days the job view keeps a job that ended, done, failed or cancelled; an item's own failure is kept "+
				"apart from it"),
			"D233 §8, the owner's: a month of what ended"),
		sourced(durationSetting(JobFirstRetryDelaySeconds, time.Second, "30", 1, 86400,
			"seconds before a failed job's first retry; each later retry waits twice as long, up to a day"),
			"best guess (M2): doubling from 30 s reaches the day cap in about a dozen tries"),
		sourced(durationSetting(ProviderTimeoutSeconds, time.Second, "20", 1, 600,
			"seconds a call to a metadata provider such as TMDB may take; read when the server starts"),
			"best guess (M2): well past a slow answer from TMDB, short of a person giving up"),
		sourced(durationSetting(DestinationFirstProbeDelaySeconds, time.Second, "30", 1, 3600,
			"seconds after an outside host is found down before it is looked at again; each later look waits "+
				"twice as long, up to destination_longest_probe_delay_seconds (D231 §6)"),
			"D231 §6: the owner's decision, 2026-09-30"),
		sourced(durationSetting(DestinationLongestProbeDelaySeconds, time.Second, "3600", 1, 24*3600,
			"the longest, in seconds, an outside host that stays down waits between looks: an hour (D231 §6)"),
			"D231 §6: the owner's decision, 2026-09-30"),
		sourced(integerSetting(TMDBRequestsPerSecond, "40", 1, 50,
			"requests a second we keep to with TMDB's API, and apart with its image server (D231 §5)"),
			"TMDB's developer documentation: upper limits \"somewhere in the 40 requests per second range\" "+
				"(developer.themoviedb.org/docs/rate-limiting, read 2026-09-25); its CDN's 50 is the bound"),
		sourced(integerSetting(TMDBBackgroundRateSharePercent, "80", 1, 100,
			"percent of tmdb_requests_per_second the server's own work may use, so a person's request always "+
				"finds room (D231 §5)"),
			"the owner's choice, 2026-09-30"),
		sourced(integerSetting(OpenSubtitlesRequestsPerSecond, "5", 1, 5,
			"requests a second we keep to with OpenSubtitles (D231 §5)"),
			"OpenSubtitles' best practices: \"5 requests per 1 second per IP address\", which is also the bound"),
		sourced(integerSetting(OpenSubtitlesDownloadsReservedForPeoplePercent, "25", 0, 100,
			"percent of the OpenSubtitles account's downloads a day held back for people: the server's own "+
				"work never spends the last of them, rounded up to whole downloads (D231 §5)"),
			"best guess: 5 of a free account's 20 a day (a VIP account has 1000; opensubtitles-api.md); "+
				"the owner's choice of a percentage, 2026-09-30"),
		{
			Key: TimeZone, Kind: KindString, StartJSON: json.RawMessage(`""`), IsValid: isTimeZoneOrMachines,
			IsExposed: true,
			Description: "the server's time zone, a name such as Europe/Berlin, which the hours of scheduled " +
				"work such as backups are in, and the settings whose keys end _local_tz; empty is the " +
				"machine's own, as the server read it at start (D246 §7)",
			StartSource: "the owner, 2026-10-05: the machine's own until one is chosen, for friendliness",
		},
		{
			Key: SortLocale, Kind: KindString, StartJSON: json.RawMessage(`"en"`),
			Pattern:     regexp.MustCompile(langtag.WellFormedPattern),
			IsExposed:   true,
			Description: "the language whose alphabetical order the libraries are listed in (BCP 47: en, sv, de...)",
			StartSource: "best guess: the language the owner reads; any BCP 47 tag works",
		},
	}, artworkAndPlaybackDefinitions()...)
}

// artworkAndPlaybackDefinitions are the rest of jobMetadataAndPlaybackDefinitions: artwork,
// refreshing metadata, playback and resuming.
func artworkAndPlaybackDefinitions() []Definition {
	return []Definition{
		sourced(integerSetting(ArtworkMaxSourcePixels, "40000000", 1_000_000, 400_000_000,
			"the most pixels a source image may have; a larger one is refused before it is decoded (D212 §1)"),
			"best guess (M3): far above any poster or backdrop "+
				"TMDB serves, far below a decompression bomb (D212 §1)"),
		sourced(integerSetting(ArtworkWebPQuality, "82", 1, 100,
			"the quality our WebP encoder writes artwork at, 1 to 100 (D212)"),
			"best guess (M3): libwebp's own default is 75; 82 keeps posters sharp at little cost"),
		sourced(durationSetting(TMDBRefreshAfterDays, day, "180", 0, 36500,
			"days after which a title's TMDB data is fetched again; TMDB's terms allow keeping it six months; "+
				"0 never refreshes (D213)"),
			"D213 §3: 180, under the six months TMDB's terms allow"),
		sourced(durationSetting(MetadataRefreshCheckEveryHours, time.Hour, "24", 1, 8760,
			"hours between looks for titles whose data is due to be fetched again (D213)"),
			"D213 §3: one look a day"),
		sourced(integerSetting(ArtworkCacheCeilingGiB, "20", 1, 1<<20,
			"GiB the artwork cache may hold; past it, what can be made again is evicted (D212 §6)"),
			"D212 §6: 20 GiB, measured against a large library's 5 to 13 GiB"),
		sourced(durationSetting(RemovedItemArtworkKeptDays, day, "30", 0, 3650,
			"days a removed item's artwork is kept, in case its disk comes back (D212 §6)"),
			"best guess (M3): a month, long enough for an unplugged disk to come back (D212 §6)"),
		sourced(durationSetting(MediaTokenIdleExpiryHours, time.Hour, "4", 1, 168,
			"hours a playback's media address lasts after its last use (D217 §2)"),
			"D217 §2: 4 hours"),
		sourced(durationSetting(HLSSegmentLengthSeconds, time.Second,
			strconv.Itoa(nodeproto.StartingSegmentLengthSeconds), nodeproto.MinSegmentLengthSeconds,
			nodeproto.MaxSegmentLengthSeconds,
			"seconds per segment of a prepared copy: shorter switches the dialogue boost sooner, longer "+
				"makes fewer requests; a copy made with another length is made again when next played (D217 §3)"),
			"D217 §3: the owner's two seconds, 2026-09-26, over six"),
		sourced(Definition{
			Key: ChunkQualities, Kind: KindString,
			StartJSON: json.RawMessage(`"3000k@720p,2000k@540p,1200k@480p,700k@432p,400k@360p"`),
			IsValid:   qualities.IsValid,
			Description: "the lower qualities a film is cut into chunks at for a slow link, highest first: each " +
				"<kbps>k@<height>p, its average bit rate, its peaks capped at one and a half times it, and its " +
				"picture's height, the width following the film's shape (D247 §6)",
		}, "D247 §6: Apple's HLS authoring spec's low rungs, to be tuned on the owner's films; 400k@360p "+
			"added by the owner, 2026-10-06, for an airplane's 0.6 Mb/s link"),
		sourced(integerSetting(ChunkHoldMs, "0", 0, 30_000,
			"ms a request for a chunk not made yet is held for it before the player is told to ask again; "+
				"a player counts a hold as the chunk's download, and takes its link for slower (D250 §5)"),
			"D250 §5: answered at once, where it was held 20 s (playback-in-practice.md §3)"),
		sourced(integerSetting(ChunkSoundKbps, "96", 32, 320,
			"kbit/s of the stereo AAC sound a film cut into chunks for a slow link plays with; a change makes "+
				"each film's sound again, as its own store, when next played (D247 §6)"),
			"the owner, 2026-10-06, after an airplane's 0.6 Mb/s link: 192k was a quarter of it; Apple's "+
				"cellular streams use less (playback-in-practice.md §3)"),
		sourced(integerSetting(ResumeMinPercent, "5", 0, 50,
			"percent of a file's length before which a position is not resumed and the file plays from the "+
				"start; a file of unknown length always resumes (play-state.md)"),
			"play-state.md: best guess, from what other players do"),
		sourced(integerSetting(FinishedAfterPercent, "90", 50, 100,
			"percent of a file's length after which it counts as finished: it leaves Continue Watching and plays "+
				"from the start (play-state.md)"),
			"play-state.md: best guess, from what other players do; credits run the last tenth"),
	}
}

// upkeepAndDisplayDefinitions are the rest of Registry: how long an invitation lasts, a new
// library's check interval, the loudness level, artwork's quality and upkeep, and how many rows
// the screens list. The owner made each a setting on 2026-09-30, answering the sweep for
// hardcoded values ("We're basically giving full tweak capability"), each with its basis.
func upkeepAndDisplayDefinitions() []Definition {
	return []Definition{
		sourced(durationSetting(RegistrationInvitationLifeDays, day, "7", 1, 365,
			"days a registration invitation works before its person needs a new one (D218 §1)"),
			"D218 §1: the owner's flow, 2026-09-25"),
		sourced(durationSetting(ScreenRefreshFirstWaitSeconds, time.Second, "2", 1, 60,
			"seconds a screen showing what the server changes in the background waits before asking again, "+
				"and after an answer that changed it (D237)"),
			"D237: the owner, 2026-10-02"),
		sourced(durationSetting(ScreenRefreshLongestWaitSeconds, time.Second, "30", 1, 3600,
			"the longest such a screen waits before asking again; the wait doubles to it while nothing "+
				"changes (D237)"),
			"D237: the owner, 2026-10-02: \"max 30s\""),
		sourced(durationSetting(ScreenRefreshFailedWaitSeconds, time.Second, "5", 1, 3600,
			"seconds such a screen waits before asking again after a read that failed, every time, so it "+
				"shows the server's answer soon after it is back (D237)"),
			"D237's amendment: the owner, 2026-10-06: \"Make it every 5 seconds\""),
		sourced(durationSetting(OfflineReminderIntervalDays, day, "7", 1, 365,
			"days an admin's dismissal hides the reminder that folders or disks are offline; it comes back "+
				"after them while any is still offline (D231 §7)"),
			"best guess: a weekly reminder, the owner 2026-09-30"),
		sourced(durationSetting(NewLibraryDefaultScanEveryHours, time.Hour, "24", 1, 8760,
			"hours between looks for changes that a new library checked on an interval starts with; each "+
				"library keeps its own once made"),
			"best guess: daily, as metadata_refresh_check_every_hours; the web form had it as its own default"),
		sourced(integerSetting(LoudnessTargetLUFS, "-23", -40, -5,
			"LUFS a measured sound is raised or lowered towards (D226 §3)"),
			"D226 §3: EBU R128's target, chosen from the owner's films (-18.8 to -30.0 LUFS measured "+
				"2026-09-28) so typical ones change little"),
		sourced(integerSetting(LoudnessMaxRaiseDB, "20", 0, 60,
			"the most dB a quiet sound is raised, so its own noise is not brought up with it (D226 §3)"),
			"D226 §3: the owner's decision, 2026-09-28; best guess, not measured"),
		sourced(durationSetting(ReadThroughSweepEveryHours, time.Hour, "1", 1, 8760,
			"hours between looks for videos to read through, whose loudness or picture's timeline is not "+
				"known yet; a scan also starts one (D226 §2, D247 §10)"),
			"best guess: hourly, so a file a scan missed waits no longer than that"),
		sourced(durationSetting(DerivedWorkSweepEverySeconds, time.Second, "3600", 60, 86400,
			"seconds between the sweeps that find the first matches owed, the refreshes people asked for, "+
				"the images owed a recheck and the libraries owed an identification pass; each also runs at "+
				"start, after a pass and when a destination comes back (D231 §3)"),
			"D231 §3: hourly by default"),
		sourced(integerSetting(ArtworkJPEGQuality, "85", 1, 100,
			"the quality the core cuts JPEG artwork at, 1 to 100, for clients without WebP (D212 §4)"),
			"best guess: a common JPEG quality where artifacts are rarely seen; not measured"),
		sourced(Definition{
			Key: ArtworkSecondsBeforeEachRetry, Kind: KindString,
			StartJSON: json.RawMessage(`"60,300,1800,7200,43200,86400,86400,86400,86400,86400,86400,86400"`),
			Pattern:   regexp.MustCompile(`^[1-9]\d{0,6}(,[1-9]\d{0,6}){0,99}$`),
			Description: "seconds before each retry of making an image's sizes that failed through no fault of " +
				"the image, comma-separated; after the last it fails. Read when the server starts (D213 §5)",
		}, "D213 §5: a minute, five, half an hour, two hours, twelve, then daily for a week"),
		sourced(durationSetting(ArtworkCacheSweepEveryHours, time.Hour, "24", 1, 8760,
			"hours between sweeps that hold the artwork cache to its ceiling; a write past it also starts one "+
				"(D212 §6)"),
			"best guess: daily; a write past the ceiling sweeps at once anyway"),
		sourced(durationSetting(ArtworkServedAtFlushEverySeconds, time.Second, "3600", 1, 86400,
			"seconds between writes of when each image was last served, which eviction orders by (D212 §6)"),
			"best guess: hourly; eviction needs no finer grain, and a stop writes them too"),
		sourced(integerSetting(ContinueWatchingMaxWorks, "20", 1, 500,
			"how many works Continue Watching shows at most"),
			"best guess: a row or two on a screen"),
		sourced(integerSetting(JobViewMaxJobs, "200", 1, 10000,
			"how many of the newest jobs the job view lists"),
			"best guess: enough to see a scan's work, few enough to load at once"),
		sourced(integerSetting(SystemPageRecentPreparesPerNode, "10", 1, 1000,
			"how many of a node's finished prepares the System page lists"),
			"best guess: enough to compare recent encode speeds"),
	}
}

// isTimeZoneOrMachines is whether name is a time zone the server knows, or empty, the machine's.
func isTimeZoneOrMachines(name string) bool { return name == "" || localtime.IsName(name) }

// exposed is definition shown on the settings page.
func exposed(definition Definition) Definition {
	definition.IsExposed = true
	return definition
}

// sourced is definition with where its starting value came from.
func sourced(definition Definition, startSource string) Definition {
	definition.StartSource = startSource
	return definition
}

// extensionsPattern is a setting's list of file extensions: lower case, without the dot,
// comma-separated.
const extensionsPattern = `^[a-z0-9]{1,10}(,[a-z0-9]{1,10}){0,99}$`

// day is the unit of a setting whose key ends _days.
const day = 24 * time.Hour

func durationSetting(
	key Key, unit time.Duration, start string, minimum, maximum int64, description string,
) Definition {
	definition := integerSetting(key, start, minimum, maximum, description)
	definition.Unit = unit
	return definition
}

func integerSetting(key Key, start string, minimum, maximum int64, description string) Definition {
	return Definition{
		Key: key, Kind: KindInteger, StartJSON: json.RawMessage(start),
		Minimum: minimum, Maximum: maximum, Description: description,
	}
}

// Lookup finds a setting's definition.
func Lookup(key Key) (Definition, bool) {
	definition, known := built().byKey[key]
	return definition, known
}

// StartingDuration is a length-of-time setting's starting value: what a reader that must go on
// when the setting cannot be read falls back on, so the fallback is the registry's and never a
// copy of it. Zero for a key that is not a length of time.
func StartingDuration(key Key) time.Duration {
	definition, known := Lookup(key)
	var count int64
	if !known || json.Unmarshal(definition.StartJSON, &count) != nil {
		return 0
	}
	return definition.LengthOf(count)
}
