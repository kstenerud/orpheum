// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

// Section is the part of the settings page a setting is listed in: the web client names each, in
// its catalogue (D46, D51).
type Section string

// The sections, in the order the settings page lists them.
const (
	// SectionGeneral is the server's language, time and the home screen.
	SectionGeneral Section = "general"
	// SectionAccounts is people, sign-in, sessions and pairing devices.
	SectionAccounts Section = "accounts"
	// SectionLibraries is scanning the library folders.
	SectionLibraries Section = "libraries"
	// SectionMetadata is TMDB, OpenSubtitles and artwork.
	SectionMetadata Section = "metadata"
	// SectionPlayback is how a film plays: resuming, loudness, its chunks.
	SectionPlayback Section = "playback"
	// SectionSlowLinks is the lower qualities cut in chunks for a slow link.
	SectionSlowLinks Section = "slowLinks"
	// SectionUpload is keeping the films within the house's upload.
	SectionUpload Section = "upload"
	// SectionTogether is Watch Together and its voice.
	SectionTogether Section = "together"
	// SectionNodes is the media nodes and their disks.
	SectionNodes Section = "nodes"
	// SectionJobs is background work.
	SectionJobs Section = "jobs"
	// SectionRemoteAccess is reaching the server from outside the home.
	SectionRemoteAccess Section = "remoteAccess"
	// SectionBackups is backups and the database upkeep.
	SectionBackups Section = "backups"
	// SectionDiagnostics is diagnosing and developing.
	SectionDiagnostics Section = "diagnostics"
)

// Sections are every section, in the order the settings page lists them.
func Sections() []Section {
	return []Section{
		SectionGeneral, SectionAccounts, SectionLibraries, SectionMetadata, SectionPlayback, SectionSlowLinks,
		SectionUpload, SectionTogether, SectionNodes, SectionJobs, SectionRemoteAccess, SectionBackups,
		SectionDiagnostics,
	}
}

// sectionByKey is each setting's section: the exhaustive linter refuses a key left out.
//
//nolint:gochecknoglobals // a constant table, which Go has no constant of: read, never written
var sectionByKey = map[Key]Section{
	ContinueWatchingMaxWorks:                       SectionGeneral,
	ShouldCheckForUpdates:                          SectionGeneral,
	SortLocale:                                     SectionGeneral,
	TimeZone:                                       SectionGeneral,
	Argon2Iterations:                               SectionAccounts,
	Argon2MemoryKiB:                                SectionAccounts,
	Argon2Parallelism:                              SectionAccounts,
	DeviceCodeRevealWaitSeconds:                    SectionAccounts,
	EndedCredentialsKeptDays:                       SectionAccounts,
	PairingCodesPerNetworkMost:                     SectionAccounts,
	PairingCodesPerSiteMost:                        SectionAccounts,
	RegistrationInvitationLifeDays:                 SectionAccounts,
	SecretLastCharsShown:                           SectionAccounts,
	SessionIdleExpiryHours:                         SectionAccounts,
	SessionLastUsedWriteIntervalSeconds:            SectionAccounts,
	SignInEverywherePaceSeconds:                    SectionAccounts,
	SignInFailureAnswerSeconds:                     SectionAccounts,
	SignInFailureLimit:                             SectionAccounts,
	SignInFailureLimitEverywhereMultiple:           SectionAccounts,
	SignInFailureLimitNetworkMultiple:              SectionAccounts,
	SignInFailureWindowSeconds:                     SectionAccounts,
	NewLibraryDefaultScanEveryHours:                SectionLibraries,
	ReadThroughSweepEveryHours:                     SectionLibraries,
	RemovedFilesKeptDays:                           SectionLibraries,
	ScanRootErrorLimit:                             SectionLibraries,
	ScanVideosPerExamineBatch:                      SectionLibraries,
	ScanWithheldRemovalsNamed:                      SectionLibraries,
	ScanWithholdRemovalsOverFiles:                  SectionLibraries,
	ScanWithholdRemovalsOverPercent:                SectionLibraries,
	ShouldScanOnNodeHello:                          SectionLibraries,
	WatchedLibraryScanEveryHours:                   SectionLibraries,
	ArtworkCacheCeilingGiB:                         SectionMetadata,
	ArtworkCacheRecommendedHeadroomPercent:         SectionMetadata,
	ArtworkCacheSweepEveryHours:                    SectionMetadata,
	ArtworkCacheSweepToPercent:                     SectionMetadata,
	ArtworkJPEGQuality:                             SectionMetadata,
	ArtworkMaxSourcePixels:                         SectionMetadata,
	ArtworkSecondsBeforeEachRetry:                  SectionMetadata,
	ArtworkServedAtFlushEverySeconds:               SectionMetadata,
	ArtworkWebPQuality:                             SectionMetadata,
	DerivedWorkSweepEverySeconds:                   SectionMetadata,
	DestinationFirstProbeDelaySeconds:              SectionMetadata,
	DestinationLongestProbeDelaySeconds:            SectionMetadata,
	MetadataRefreshCheckEveryHours:                 SectionMetadata,
	OpenSubtitlesDownloadsReservedForPeoplePercent: SectionMetadata,
	OpenSubtitlesRequestsPerSecond:                 SectionMetadata,
	ProviderTimeoutSeconds:                         SectionMetadata,
	RemovedItemArtworkKeptDays:                     SectionMetadata,
	TMDBBackgroundRateSharePercent:                 SectionMetadata,
	TMDBRefreshAfterDays:                           SectionMetadata,
	TMDBRequestsPerSecond:                          SectionMetadata,
	FinishedAfterPercent:                           SectionPlayback,
	HLSSegmentLengthSeconds:                        SectionPlayback,
	ReadyAheadOfNextChunkPercent:                   SectionPlayback,
	LoudnessMaxRaiseDB:                             SectionPlayback,
	LoudnessTargetLUFS:                             SectionPlayback,
	MediaTokenIdleExpiryHours:                      SectionPlayback,
	InProgressAfterPercent:                         SectionPlayback,
	ChunkCPUCeilingKeptSeconds:                     SectionSlowLinks,
	ChunkHoldMs:                                    SectionSlowLinks,
	ChunkInterestLapsesAfterSeconds:                SectionSlowLinks,
	ChunkKeptAheadSeconds:                          SectionSlowLinks,
	ChunkKeptLeftSeconds:                           SectionSlowLinks,
	ChunkKeptLikelySeconds:                         SectionSlowLinks,
	ChunkKeptMaybeSeconds:                          SectionSlowLinks,
	ChunkLookaheadSeconds:                          SectionSlowLinks,
	ChunkQualities:                                 SectionSlowLinks,
	ChunkQualitiesHardware:                         SectionSlowLinks,
	ChunkRunSlowestSpeedPercent:                    SectionSlowLinks,
	ChunkRunSpanSeconds:                            SectionSlowLinks,
	ChunkRunSpeedWindowSeconds:                     SectionSlowLinks,
	ChunkSoundKbps:                                 SectionSlowLinks,
	LinkShareForQualityPercent:                     SectionSlowLinks,
	LinkSpeedFreshHours:                            SectionSlowLinks,
	LinkSteadinessRelaxesSeconds:                   SectionSlowLinks,
	PlayLadderWaitMs:                               SectionSlowLinks,
	ServerUploadMbps:                               SectionUpload,
	WebRTCConnectionsPerNetwork:                    SectionRemoteAccess,
	UploadCrowdedFloorPercent:                      SectionUpload,
	UploadCrowdedKeepsPercent:                      SectionUpload,
	UploadCrowdedLowerEverySeconds:                 SectionUpload,
	UploadCrowdedSlowPlayersAtLeast:                SectionUpload,
	UploadCrowdedSlowUnderPercent:                  SectionUpload,
	UploadFilmsSharePercent:                        SectionUpload,
	UploadRaiseRoomSeconds:                         SectionUpload,
	UploadRaiseUnderPercent:                        SectionUpload,
	UploadSendAwayAfterSeconds:                     SectionUpload,
	UploadUncrowdedForSeconds:                      SectionUpload,
	UploadUncrowdedGrowsPercent:                    SectionUpload,
	UploadUncrowdedRaiseEverySeconds:               SectionUpload,
	ShouldTogetherNudgeKeepPitch:                   SectionTogether,
	ShouldTogetherVoiceBrowserLevelMicrophone:      SectionTogether,
	ShouldTogetherVoiceCancelEcho:                  SectionTogether,
	TogetherCatchUpBufferPercent:                   SectionTogether,
	TogetherDriftLeftAloneMs:                       SectionTogether,
	TogetherNudgeMostPercent:                       SectionTogether,
	TogetherSeekDriftBeyondMs:                      SectionTogether,
	TogetherSeekOnlyDriftLeftAloneMs:               SectionTogether,
	TogetherSettleAfterChangeMs:                    SectionTogether,
	TogetherSnapSeeksBeyondSeconds:                 SectionTogether,
	TogetherLinkLifetimeSeconds:                    SectionTogether,
	TogetherIdleSessionEndHours:                    SectionTogether,
	VoiceRelayCredentialHours:                      SectionTogether,
	AssignedJobLimitHours:                          SectionNodes,
	ReaderLimitNetworkShare:                        SectionNodes,
	NodeJobsBeyondFreeSlots:                        SectionNodes,
	NodeSilentAfterSeconds:                         SectionNodes,
	PlayPriorityKeptSeconds:                        SectionNodes,
	InterestSaidEverySeconds:                       SectionJobs,
	InterestLapsesAfterSeconds:                     SectionJobs,
	UnavailableRootRescanSeconds:                   SectionLibraries,
	SeriesEpisodesAhead:                            SectionJobs,
	NodeProgressEverySeconds:                       SectionNodes,
	ReaderLimitSpinningDisk:                        SectionNodes,
	ReaderLimitSSD:                                 SectionNodes,
	ReaderLimitUnknownDisk:                         SectionNodes,
	FinishedJobsKeptDays:                           SectionJobs,
	JobAttemptLimit:                                SectionJobs,
	JobFirstRetryDelaySeconds:                      SectionJobs,
	JobHealthWindowHours:                           SectionJobs,
	JobLongestRetryDelayHours:                      SectionJobs,
	JobViewMaxJobs:                                 SectionJobs,
	CertificateDNSWaitSeconds:                      SectionRemoteAccess,
	CertificateRenewBeforeDays:                     SectionRemoteAccess,
	DiscoveryAddressCheckEverySeconds:              SectionRemoteAccess,
	DiscoveryEndedCheckEverySeconds:                SectionRemoteAccess,
	DiscoveryPendingCheckEverySeconds:              SectionRemoteAccess,
	DiscoveryRetryFirstSeconds:                     SectionRemoteAccess,
	DiscoveryRetryMostSeconds:                      SectionRemoteAccess,
	PortMappingLeaseSeconds:                        SectionRemoteAccess,
	PortMappingPermanentRenewSeconds:               SectionRemoteAccess,
	PortMappingRetrySeconds:                        SectionRemoteAccess,
	ShouldMapPorts:                                 SectionRemoteAccess,
	BackupAgeWarningHours:                          SectionBackups,
	BackupEveryHours:                               SectionBackups,
	BackupsKeptDaily:                               SectionBackups,
	BackupsKeptHourly:                              SectionBackups,
	BackupsKeptWeekly:                              SectionBackups,
	InvariantsCheckEveryHours:                      SectionBackups,
	RetentionSweepEveryHours:                       SectionBackups,
	ScreenRefreshFailedWaitSeconds:                 SectionDiagnostics,
	ScreenRefreshFirstWaitSeconds:                  SectionDiagnostics,
	ScreenRefreshLongestWaitSeconds:                SectionDiagnostics,
	LogLevel:                                       SectionDiagnostics,
	ShouldServeDebugMetrics:                        SectionDiagnostics,
	ShouldServeDebugPProf:                          SectionDiagnostics,
	ShouldShowDeveloperPages:                       SectionDiagnostics,
	ShouldOfferDeviceTests:                         SectionDiagnostics,
	AddressAtHome:                                  SectionRemoteAccess,
	AddressAtHomeLearnedFrom:                       SectionRemoteAccess,
}
