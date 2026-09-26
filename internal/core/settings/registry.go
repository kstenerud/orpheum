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
	UpdateCheck                Key = "update_check"
	SubtitlesOnByDefault       Key = "subtitles_on_by_default"
	DialogueBoostDefault       Key = "dialogue_boost_default"
	SignInFailureLimit         Key = "signin_failure_limit"
	SignInFailureWindowS       Key = "signin_failure_window_s"
	SessionIdleExpiryH         Key = "session_idle_expiry_h"
	SessionLastSeenResolutionS Key = "session_last_seen_resolution_s"
	Argon2MemoryKiB            Key = "argon2_memory_kib"
	Argon2Iterations           Key = "argon2_iterations"
	Argon2Parallelism          Key = "argon2_parallelism"
	SecretHintLength           Key = "secret_hint_length"
	BackupAgeWarningH          Key = "backup_age_warning_h"
	DebugPProf                 Key = "debug_pprof"
	DebugMetrics               Key = "debug_metrics"
	NodeSilentAfterS           Key = "node_silent_after_s"
	NodeRestartAfterS          Key = "node_restart_after_s"
	NodeRestartDelayS          Key = "node_restart_delay_s"
	ScanOnNodeHello            Key = "scan_on_node_hello"
	RemovalWithholdPercent     Key = "removal_withhold_percent"
	RemovalWithholdMinFiles    Key = "removal_withhold_min_files"
	JobRetryLimit              Key = "job_retry_limit"
	JobRetryBackoffS           Key = "job_retry_backoff_s"
	ProviderTimeoutS           Key = "provider_timeout_s"
	SortLocale                 Key = "sort_locale"
	ArtworkMaxSourcePixels     Key = "artwork_max_source_pixels"
	ArtworkWebPQuality         Key = "artwork_webp_quality"
	TMDBRefreshAfterD          Key = "tmdb_refresh_after_d"
	RefreshCheckEveryH         Key = "refresh_check_every_h"
	ArtworkCacheCeilingGiB     Key = "artwork_cache_ceiling_gib"
	ArtworkRemovedGraceD       Key = "artwork_removed_grace_d"
	RemoteBitRateCeilingKbps   Key = "remote_bit_rate_ceiling_kbps"
	MediaTokenIdleH            Key = "media_token_idle_h"
	HLSSegmentS                Key = "hls_segment_s"
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
	Key     Key
	Scope   Scope
	Kind    Kind
	Start   json.RawMessage
	Minimum int64 // integers only
	Maximum int64 // integers only
	// Pattern, for a string, is what every value must match; nil takes any string.
	Pattern  *regexp.Regexp
	Question string
	Exposed  bool
	Describe string
}

// IsQuestion reports whether the setting starts unanswered.
func (definition Definition) IsQuestion() bool {
	return string(definition.Start) == "null"
}

// Registry is every setting. The starting values are the opinions: each says why it is what it
// is, or names the decision that chose it.
func Registry() []Definition {
	return append([]Definition{
		{
			Key: UpdateCheck, Scope: ScopeServer, Kind: KindBool, Start: json.RawMessage("null"), Exposed: true,
			Question: "Check GitHub for a new release once a day? The check sends nothing but the request.",
			Describe: "whether the server checks for a new release (D184: asked, never assumed)",
		},
		{
			Key: SubtitlesOnByDefault, Scope: ScopeServer, Kind: KindBool, Start: json.RawMessage("true"),
			Exposed: true, Describe: "whether subtitles are on when a person has chosen nothing (cut-a §1)",
		},
		{
			Key: DialogueBoostDefault, Scope: ScopeServer, Kind: KindBool, Start: json.RawMessage("false"),
			Exposed:  true,
			Describe: "whether dialogue boost is on when a person has chosen nothing; off plays the original mix",
		},
		integer(SignInFailureLimit, "5", 1, 1000,
			"failed sign-ins per account, and per address, before sign-in is refused for the window (D194 §2)"),
		integer(SignInFailureWindowS, "900", 1, 7*24*3600,
			"how long, in seconds, a failed sign-in counts toward the limit"),
		integer(SessionIdleExpiryH, "2160", 1, 10*365*24,
			"hours a session may go unused before it ends; 90 days, so a television rarely signs out (D154)"),
		integer(SessionLastSeenResolutionS, "60", 1, 24*3600,
			"how often, in seconds, a session's last use is written; idle expiry needs no finer grain"),
		integer(Argon2MemoryKiB, "65536", 8, math.MaxUint32,
			"argon2id memory per password hash, KiB: RFC 9106's second option (D194 §2); new costs re-hash on sign-in"),
		integer(Argon2Iterations, "3", 1, math.MaxUint32, "argon2id passes per password hash (D194 §2)"),
		integer(Argon2Parallelism, "4", 1, math.MaxUint8, "argon2id lanes per password hash (D194 §2)"),
		integer(SecretHintLength, "4", 0, 8,
			"trailing characters of an API key shown as its hint; a password never has one (D194 §3)"),
		integer(BackupAgeWarningH, "168", 1, 365*24, "hours after which doctor says the newest backup is old"),
		{
			Key: DebugPProf, Scope: ScopeServer, Kind: KindBool, Start: json.RawMessage("false"),
			Describe: "serve /debug/pprof, on a loopback listen address only (D91); read at start",
		},
		{
			Key: DebugMetrics, Scope: ScopeServer, Kind: KindBool, Start: json.RawMessage("false"),
			Describe: "serve /metrics, on a loopback listen address only (D91); not built yet",
		},
		integer(NodeSilentAfterS, "30", 1, 24*3600,
			"seconds without a record from a node before the core shows it as silent (cut-a §4)"),
		integer(NodeRestartAfterS, "120", 1, 24*3600,
			"seconds of silence before the core restarts the node it supervises (cut-a §4)"),
		integer(NodeRestartDelayS, "5", 1, 3600,
			"seconds the core waits before starting its node again after it exited, so a crash cannot spin"),
		{
			Key: ScanOnNodeHello, Scope: ScopeServer, Kind: KindBool, Start: json.RawMessage("true"),
			Describe: "whether the core asks a node to scan every root it names each time it says hello",
		},
		integer(RemovalWithholdPercent, "50", 1, 100,
			"a scan that would remove more than this percentage of a root's files waits for the admin (D208)"),
		integer(RemovalWithholdMinFiles, "20", 0, math.MaxInt32,
			"...and more than this many of them; a scan that would remove all of them always waits (D208)"),
	}, metadataDefinitions()...)
}

// metadataDefinitions are the settings of fetching metadata and artwork.
func metadataDefinitions() []Definition {
	return []Definition{
		integer(JobRetryLimit, "5", 1, 100,
			"how many times a background job that failed is tried before it is marked failed"),
		integer(JobRetryBackoffS, "30", 1, 86400,
			"seconds before a failed job's first retry; each later retry waits twice as long"),
		integer(ProviderTimeoutS, "20", 1, 600,
			"seconds a call to a metadata provider such as TMDB may take; read when the server starts"),
		{
			Key: SortLocale, Scope: ScopeServer, Kind: KindString, Start: json.RawMessage(`"en"`),
			Pattern:  regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`),
			Exposed:  true,
			Describe: "the language whose alphabetical order the libraries are listed in (BCP 47: en, sv, de...)",
		},
		integer(ArtworkMaxSourcePixels, "40000000", 1_000_000, 400_000_000,
			"the most pixels a source image may have; a larger one is refused before it is decoded (D212 §1)"),
		integer(ArtworkWebPQuality, "82", 1, 100, "the quality our WebP encoder writes artwork at, 1 to 100 (D212)"),
		integer(TMDBRefreshAfterD, "180", 0, 36500,
			"days after which a title's TMDB data is fetched again; TMDB's terms allow keeping it six months; "+
				"0 never refreshes (D213)"),
		integer(RefreshCheckEveryH, "24", 1, 8760,
			"hours between looks for titles whose data is due to be fetched again (D213)"),
		integer(ArtworkCacheCeilingGiB, "20", 1, 1<<20,
			"GiB the artwork cache may hold; past it, what can be made again is evicted (D212 §6)"),
		integer(ArtworkRemovedGraceD, "30", 0, 3650,
			"days a removed item's artwork is kept, in case its disk comes back (D212 §6)"),
		integer(RemoteBitRateCeilingKbps, "10000", 500, 1_000_000,
			"kb/s above which a file played over a remote link is re-encoded (cut-a §6, reason bandwidth)"),
		integer(MediaTokenIdleH, "4", 1, 168,
			"hours a playback's media address lasts after its last use (D217 §2)"),
		integer(HLSSegmentS, "2", 1, 10,
			"seconds per segment of a prepared copy: shorter switches the dialogue boost sooner, longer "+
				"makes fewer requests; a copy made with another length is made again when next played (D217 §3)"),
	}
}

func integer(key Key, start string, minimum, maximum int64, describe string) Definition {
	return Definition{
		Key: key, Scope: ScopeServer, Kind: KindInteger, Start: json.RawMessage(start),
		Minimum: minimum, Maximum: maximum, Describe: describe,
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
