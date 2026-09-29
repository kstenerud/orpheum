// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"math"
	"regexp"
)

// Key names a setting. The set is closed: every key is declared in Registry.
type Key string

// The settings. Every opinionated value the server acts on is one of these, so each choice is a
// row that can be read and changed, not a default in code (D200). Units are in the name.
const (
	ShouldCheckForUpdates           Key = "should_check_for_updates"
	SignInFailureLimit              Key = "signin_failure_limit"
	SignInFailureWindowS            Key = "signin_failure_window_s"
	SessionIdleExpiryH              Key = "session_idle_expiry_h"
	SessionLastSeenWriteIntervalS   Key = "session_last_seen_write_interval_s"
	Argon2MemoryKiB                 Key = "argon2_memory_kib"
	Argon2Iterations                Key = "argon2_iterations"
	Argon2Parallelism               Key = "argon2_parallelism"
	SecretLastCharsShown            Key = "secret_last_chars_shown"
	BackupAgeWarningH               Key = "backup_age_warning_h"
	ShouldServeDebugPProf           Key = "should_serve_debug_pprof"
	ShouldServeDebugMetrics         Key = "should_serve_debug_metrics"
	NodeSilentAfterS                Key = "node_silent_after_s"
	NodeRestartAfterSilentS         Key = "node_restart_after_silent_s"
	NodeRestartAfterExitS           Key = "node_restart_after_exit_s"
	ShouldScanOnNodeHello           Key = "should_scan_on_node_hello"
	ScanWithholdRemovalsOverPercent Key = "scan_withhold_removals_over_percent"
	ScanWithholdRemovalsOverFiles   Key = "scan_withhold_removals_over_files"
	JobAttemptLimit                 Key = "job_attempt_limit"
	JobFirstRetryDelayS             Key = "job_first_retry_delay_s"
	ProviderTimeoutS                Key = "provider_timeout_s"
	SortLocale                      Key = "sort_locale"
	ArtworkMaxSourcePixels          Key = "artwork_max_source_pixels"
	ArtworkWebPQuality              Key = "artwork_webp_quality"
	TMDBRefreshAfterD               Key = "tmdb_refresh_after_d"
	MetadataRefreshCheckEveryH      Key = "metadata_refresh_check_every_h"
	ArtworkCacheCeilingGiB          Key = "artwork_cache_ceiling_gib"
	RemovedItemArtworkKeptD         Key = "removed_item_artwork_kept_d"
	RemoteBitRateCeilingKbps        Key = "remote_bit_rate_ceiling_kbps"
	MediaTokenIdleExpiryH           Key = "media_token_idle_expiry_h"
	HLSSegmentLengthS               Key = "hls_segment_length_s"
	ResumeMinPercent                Key = "resume_min_percent"
	FinishedAfterPercent            Key = "finished_after_percent"
)

// Scope is whose a setting is.
type Scope string

// The scopes, as the setting table's CHECK names them.
const (
	ScopeServer Scope = "server"
	ScopeUser   Scope = "user"
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
	Scope     Scope
	Kind      Kind
	StartJSON json.RawMessage
	Minimum   int64 // integers only
	Maximum   int64 // integers only
	// Pattern, for a string, is what every value must match; nil takes any string.
	Pattern     *regexp.Regexp
	Question    string
	IsExposed   bool
	Description string
}

// IsQuestion reports whether the setting starts unanswered.
func (definition Definition) IsQuestion() bool {
	return string(definition.StartJSON) == "null"
}

// Registry is every setting. The starting values are the opinions: each says why it is what it
// is, or names the decision that chose it.
func Registry() []Definition {
	return append([]Definition{
		{
			Key: ShouldCheckForUpdates, Scope: ScopeServer, Kind: KindBool, StartJSON: json.RawMessage("null"),
			IsExposed:   true,
			Question:    "Check GitHub for a new release once a day? The check sends nothing but the request.",
			Description: "whether the server checks for a new release (D184: asked, never assumed)",
		},
		integerSetting(SignInFailureLimit, "5", 1, 1000,
			"failed sign-ins per account, and per address, before sign-in is refused for the window (D194 §2)"),
		integerSetting(SignInFailureWindowS, "900", 1, 7*24*3600,
			"how long, in seconds, a failed sign-in counts toward the limit"),
		integerSetting(SessionIdleExpiryH, "2160", 1, 10*365*24,
			"hours a session may go unused before it ends; 90 days, so a television rarely signs out (D154)"),
		integerSetting(SessionLastSeenWriteIntervalS, "60", 1, 24*3600,
			"how often, in seconds, a session's last use is written; idle expiry needs no finer grain"),
		integerSetting(Argon2MemoryKiB, "65536", 8, math.MaxUint32,
			"argon2id memory per password hash, KiB: RFC 9106's second option (D194 §2); new costs re-hash on sign-in"),
		integerSetting(Argon2Iterations, "3", 1, math.MaxUint32, "argon2id passes per password hash (D194 §2)"),
		integerSetting(Argon2Parallelism, "4", 1, math.MaxUint8, "argon2id lanes per password hash (D194 §2)"),
		integerSetting(SecretLastCharsShown, "4", 0, 8,
			"trailing characters of an API key shown, to tell which key is set; a password shows none (D194 §3)"),
		integerSetting(BackupAgeWarningH, "168", 1, 365*24, "hours after which doctor says the newest backup is old"),
		{
			Key: ShouldServeDebugPProf, Scope: ScopeServer, Kind: KindBool, StartJSON: json.RawMessage("false"),
			Description: "serve /debug/pprof, on a loopback listen address only (D91); read at start",
		},
		{
			Key: ShouldServeDebugMetrics, Scope: ScopeServer, Kind: KindBool, StartJSON: json.RawMessage("false"),
			Description: "serve /metrics, on a loopback listen address only (D91); not built yet",
		},
		integerSetting(NodeSilentAfterS, "30", 1, 24*3600,
			"seconds without a record from a node before the core shows it as silent (cut-a §4)"),
		integerSetting(NodeRestartAfterSilentS, "120", 1, 24*3600,
			"seconds of silence before the core restarts the node it supervises (cut-a §4)"),
		integerSetting(NodeRestartAfterExitS, "5", 1, 3600,
			"seconds the core waits before starting its node again after it exited, so a crash cannot spin"),
		{
			Key: ShouldScanOnNodeHello, Scope: ScopeServer, Kind: KindBool, StartJSON: json.RawMessage("true"),
			Description: "whether the core asks a node to scan every root it names each time it says hello",
		},
		integerSetting(ScanWithholdRemovalsOverPercent, "50", 1, 100,
			"a scan that would remove more than this percentage of a root's files waits for the admin (D208)"),
		integerSetting(ScanWithholdRemovalsOverFiles, "20", 0, math.MaxInt32,
			"...and more than this many of them; a scan that would remove all of them always waits (D208)"),
	}, jobMetadataAndPlaybackDefinitions()...)
}

// jobMetadataAndPlaybackDefinitions are the rest of Registry: jobs, fetching metadata and
// artwork, sorting, playback and resuming.
func jobMetadataAndPlaybackDefinitions() []Definition {
	return []Definition{
		integerSetting(JobAttemptLimit, "5", 1, 100,
			"how many times a background job that failed is tried before it is marked failed"),
		integerSetting(JobFirstRetryDelayS, "30", 1, 86400,
			"seconds before a failed job's first retry; each later retry waits twice as long"),
		integerSetting(ProviderTimeoutS, "20", 1, 600,
			"seconds a call to a metadata provider such as TMDB may take; read when the server starts"),
		{
			Key: SortLocale, Scope: ScopeServer, Kind: KindString, StartJSON: json.RawMessage(`"en"`),
			Pattern:     regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`),
			IsExposed:   true,
			Description: "the language whose alphabetical order the libraries are listed in (BCP 47: en, sv, de...)",
		},
		integerSetting(ArtworkMaxSourcePixels, "40000000", 1_000_000, 400_000_000,
			"the most pixels a source image may have; a larger one is refused before it is decoded (D212 §1)"),
		integerSetting(ArtworkWebPQuality, "82", 1, 100,
			"the quality our WebP encoder writes artwork at, 1 to 100 (D212)"),
		integerSetting(TMDBRefreshAfterD, "180", 0, 36500,
			"days after which a title's TMDB data is fetched again; TMDB's terms allow keeping it six months; "+
				"0 never refreshes (D213)"),
		integerSetting(MetadataRefreshCheckEveryH, "24", 1, 8760,
			"hours between looks for titles whose data is due to be fetched again (D213)"),
		integerSetting(ArtworkCacheCeilingGiB, "20", 1, 1<<20,
			"GiB the artwork cache may hold; past it, what can be made again is evicted (D212 §6)"),
		integerSetting(RemovedItemArtworkKeptD, "30", 0, 3650,
			"days a removed item's artwork is kept, in case its disk comes back (D212 §6)"),
		integerSetting(RemoteBitRateCeilingKbps, "10000", 500, 1_000_000,
			"kb/s above which a file played over a remote link is re-encoded (cut-a §6, reason bandwidth)"),
		integerSetting(MediaTokenIdleExpiryH, "4", 1, 168,
			"hours a playback's media address lasts after its last use (D217 §2)"),
		integerSetting(HLSSegmentLengthS, "2", 1, 10,
			"seconds per segment of a prepared copy: shorter switches the dialogue boost sooner, longer "+
				"makes fewer requests; a copy made with another length is made again when next played (D217 §3)"),
		integerSetting(ResumeMinPercent, "5", 0, 50,
			"percent of a file's length before which a position is not resumed and the file plays from the "+
				"start; a file of unknown length always resumes (play-state.md)"),
		integerSetting(FinishedAfterPercent, "90", 50, 100,
			"percent of a file's length after which it counts as finished: it leaves Continue Watching and plays "+
				"from the start (play-state.md)"),
	}
}

func integerSetting(key Key, start string, minimum, maximum int64, description string) Definition {
	return Definition{
		Key: key, Scope: ScopeServer, Kind: KindInteger, StartJSON: json.RawMessage(start),
		Minimum: minimum, Maximum: maximum, Description: description,
	}
}

// Lookup finds a setting's definition.
func Lookup(key Key) (Definition, bool) {
	for _, definition := range Registry() {
		if definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}
