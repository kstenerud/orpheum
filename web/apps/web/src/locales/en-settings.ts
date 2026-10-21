// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's settings part: each section of the settings page, and each server setting's
 * description and each key's or password's, and for some a longer one (details), by its name in
 * camelCase (camelKey), kept apart so the catalogue fits in one read
 * (en.ts). A Go test holds a description here for every setting the server has.
 */
export const enSettingsCatalogue = {
  sections: {
    general: "General",
    accounts: "Accounts and sign-in",
    libraries: "Libraries and scanning",
    metadata: "Metadata and artwork",
    playback: "Playback",
    slowLinks: "Slow links",
    upload: "Upload",
    together: "Watch Together and voice",
    nodes: "Media nodes",
    jobs: "Background jobs",
    remoteAccess: "Remote access",
    backups: "Backups and upkeep",
    diagnostics: "Diagnostics and development",
  },
  secretDescriptions: {
    dnsProviderToken:
      "The DNS provider's API token the server proves its name with, to get its HTTPS certificate.",
    opensubtitlesPassword: "The password of the OpenSubtitles account.",
    opensubtitlesUsername: "The OpenSubtitles account everyone's subtitle downloads use.",
    tmdbApiKey: "The TMDB API key films' and series' details and artwork are fetched with.",
  },
  details: {
    interestLapsesAfterSeconds:
      "What someone plays comes first, then the films and series whose pages are open, the latest opened first, then everything else. A page left, or a playback stopped, gives its work back its place after this long; it then stays a rung above work no one has shown interest in.",
    argon2MemoryKib:
      "Passwords are stored as Argon2id hashes, which take memory and time on purpose so that guessing them is slow. More memory makes guessing harder and each sign-in heavier for the server. A change applies to each password the next time its person signs in.",
    artworkCacheCeilingGib:
      "Posters, backdrops and stills are kept in sizes for each kind of screen. Past this limit, those that can be made again from their source are removed, least recently shown first. A large library takes about 5 to 13 GiB. The System page shows what it holds now and what it recommends.",
    artworkSecondsBeforeEachRetry:
      "Making an image's sizes can fail because its source didn't answer. It's tried again after each of these waits, in seconds, in turn. After the last, it's marked failed.",
    assignedJobLimitHours:
      "A backstop: a job a media node took and never answered for, usually because the node restarted, counts as failed after this long and is tried again.",
    backupEveryHours:
      "The server copies its database, with everything people watched and chose and its settings, but not the media. A backup is skipped when nothing changed since the last. The server's own key is never in it, so keep that safely yourself.",
    certificateDnsWaitSeconds:
      "During a renewal, the record that proves the name is the server's takes a moment to reach the DNS servers the certificate authority asks. It is checked after this long.",
    certificateRenewBeforeDays:
      "The server's HTTPS certificate lasts 90 days. It is renewed this long before it expires, so a renewal that fails has many daily retries before it matters.",
    chunkHoldMs:
      "When a player asks for a chunk that's still being made, the server can hold the request until it's ready instead of answering at once. A player counts the wait as download time and may think its link is slower than it is, so the starting value holds nothing.",
    chunkInterestLapsesAfterSeconds:
      "A playing player asks for chunks every few seconds and reports every ten. When one has said nothing for this long, it's taken to have crashed or lost its link, and its encoding stops.",
    chunkKeptAheadSeconds:
      "When a player pauses or leaves, the encoding for its slow link carries on this far past where it stopped, so playing again starts at once.",
    chunkKeptLeftSeconds:
      "When a player leaves the film's page without playing something else, its chunks are kept as maybe played again for this long.",
    chunkKeptLikelySeconds:
      "Chunks made for a slow link take disk space. Those of a player paused or unheard for less than this are likely to be played again, and are removed last when the disk needs room.",
    chunkKeptMaybeSeconds:
      "Past chunk_kept_likely_seconds and up to this, a paused or unheard player's chunks are kept as maybe played again. After it, they're the first removed when the disk needs room.",
    chunkLookaheadSeconds:
      "An encoding for a slow link makes this much film ahead of what its player asked for, then waits for the player to come closer.",
    chunkQualities:
      "When a player's link can't carry a film as it is, the server encodes it into smaller copies, cut in pieces as they're played. Each quality is written as its average bit rate and picture height, such as 2000k@540p. A player moves between them as its link allows.",
    chunkQualitiesHardware:
      "A media node that encodes on its graphics card (Intel Quick Sync or NVIDIA NVENC) can make a higher quality fast enough to play, so it offers more. Written as chunk_qualities.",
    chunkRunSlowestSpeedPercent:
      "Encoding for slow links has to keep ahead of the people watching. When two or more encodings on one media node run slower than this share of playback speed, one is stopped so the others keep up: the oldest that nobody plays from, or else the newest. Its viewers are told why.",
    chunkRunSpanSeconds:
      "One encoding makes at most this much film. A jump elsewhere in the film then starts a new encoding there, instead of waiting for a long one to get to it.",
    deviceCodeRevealWaitSeconds:
      "When you enter a device's code, the server waits for that device to check in and confirm it. Devices check every two seconds.",
    discoveryPendingCheckEverySeconds:
      "A new server is linked to your account at the discovery service by a claim code. Until you enter it there, the server asks this often whether it has been claimed.",
    discoveryRetryFirstSeconds:
      "When the discovery service can't be reached, the server tries again after this long, then waits twice as long after each failure, up to discovery_retry_most_seconds.",
    dnsProviderToken:
      "To be reached from outside the home over HTTPS, the server needs a certificate for its name. It proves the name is its own by putting a record in the name's DNS for a moment, which needs an API token from your DNS provider, such as Cloudflare, allowed to edit that zone's records.",
    finishedAfterPercent:
      "Credits often take the last part of a film. Past this share of its length, the video counts as watched: it leaves Continue Watching and plays from the start next time.",
    hlsSegmentLengthSeconds:
      "A prepared copy is sent in short pieces. Shorter pieces let the dialogue boost and other sound changes start sooner; longer ones make fewer requests. Changing it remakes each prepared copy the next time it's played.",
    inProgressAfterPercent:
      "A video watched only a little, a few minutes of trying it, isn't worth coming back to. Until playback passes this share of its length, it plays from the start next time and stays out of Continue Watching. Past it, it resumes where it stopped.",
    invariantsCheckEveryHours:
      "The server checks that its database agrees with itself, for example that nothing points at a title that's gone. It does this at every start, and this often while it runs, writing each problem it finds to its log.",
    jobAttemptLimit:
      "Background work, such as fetching a title's details or making artwork, is retried when it fails for a reason that may pass. After this many tries it is marked failed and shown on the System page.",
    linkShareForQualityPercent:
      "A link's measured speed varies, and the sound and the picture's peaks need room too. A quality is chosen only when its bit rate fits in this share of what was measured.",
    loudnessMaxRaiseDb:
      "A very quiet film raised all the way to the target would bring its background hiss up too. This is the most the server raises any sound.",
    loudnessTargetLufs:
      "Films are mixed at very different loudness. The server measures each one and raises or lowers it towards this level, in LUFS, the measure broadcasters use. -23 is the European broadcast standard; a higher number, such as -16, is louder.",
    mediaTokenIdleExpiryHours:
      "A playback is given its own address for the media, which works only for it. Once that address has gone unused this long, it stops working.",
    nodeJobsBeyondFreeSlots:
      "The server gives a media node a few more jobs than it can run at once, so when one finishes the next starts at once instead of waiting for the server.",
    nodeSilentAfterSeconds:
      "A media node reports to the server regularly, every third of this time. When nothing has come for this long, the server shows the node as silent and its libraries' folders as unreachable.",
    opensubtitlesDownloadsReservedForPeoplePercent:
      "OpenSubtitles allows an account a number of downloads a day. The server's own work never uses this share of them, so people searching for subtitles still have some.",
    opensubtitlesPassword:
      "With an OpenSubtitles account, people can search for subtitles a film lacks. Every person's downloads use this one account, and its daily download limit is shared among them.",
    pairingCodesPerNetworkMost:
      "A television or other device signs in by showing a code you enter on a device already signed in. This limits how many codes one network can have waiting, so nobody can fill the server with them.",
    playLadderWaitMs:
      "A film is cut for slow links only once it's first played on one. Until its cut points are read, playback waits up to this long, then plays a prepared copy or the original instead.",
    playPriorityKeptSeconds:
      "Work for something being watched goes ahead of background work, such as scanning, and pauses it if needed. Once nobody has asked for that playback's work for this long, it counts as background work again.",
    portMappingLeaseSeconds:
      "A port the router opens on request stays open for a time and is asked for again before it runs out. This is how long it's asked for.",
    readerLimitNetworkShare:
      "A folder mounted from another machine reads over the network, which a few readers share well.",
    readerLimitSpinningDisk:
      "A spinning disk reads fastest one thing at a time: a second reader makes its head move back and forth between them, and both slow down. This includes a pool or array that has a spinning disk among its data disks.",
    readerLimitSsd:
      "A solid-state disk has no head to move, so several can read it at once without slowing each other much.",
    readerLimitUnknownDisk:
      "When a media node can't tell what a disk is, the server treats it as the most careful kind.",
    retentionSweepEveryHours:
      "Ended sign-ins and files gone from their disks are remembered for a time (ended_credentials_kept_days, removed_files_kept_days). This is how often those past their time are deleted.",
    scanVideosPerExamineBatch:
      "A media node reads new videos in groups, each with the files beside it, such as subtitles and posters. A group lost to a restart is read again, so a smaller group repeats less.",
    scanWithholdRemovalsOverPercent:
      "When a disk is unplugged or a network share is down, a scan can find everything gone. A scan that would remove more than this share of a folder's files, and more than scan_withhold_removals_over_files, waits for you to confirm rather than forgetting what was watched.",
    serverUploadMbps:
      "Everything people watch from outside the home is sent over your connection's upload, which is usually much slower than its download. The server keeps the films within this speed: past it, it moves players to lower qualities, and as a last resort removes the newest Watch Together member. Use your connection's upload speed, or less to keep some for other things. Unset, the server never lowers anything for the upload.",
    sessionIdleExpiryHours:
      "A device stays signed in for as long as it is used. Once it has gone unused this long, it is signed out and its person signs in again. A long time suits televisions, which people rarely sign in to.",
    shouldMapPorts:
      "Most home routers let a device inside ask for a port to be opened (UPnP, NAT-PMP or PCP). When on, the server asks for its HTTPS port while it is published to the internet, so it can be reached from outside without you changing the router yourself. Turn it off if you open the port yourself or don't want the router asked.",
    shouldTogetherVoiceBrowserLevelMicrophone:
      "The browser can even out how loud your voice is and clean up background noise. Without it, a quiet partner can be hard to hear over the film. Firefox on Android treats it as a call, turning the film down and taking over its volume buttons.",
    shouldTogetherVoiceCancelEcho:
      "Echo cancellation stops your film's sound, coming out of your speakers, from going back through your microphone to the others. With headphones there is none to cancel. On Android, turning it on makes the phone treat Watch Together as a call, which turns the film down and takes over its volume buttons.",
    signinFailureLimit:
      "After this many wrong passwords, sign-in for that account, or from that address, is refused until signin_failure_window_seconds have passed. It slows down anyone guessing passwords.",
    signinFailureLimitEverywhereMultiple:
      "Someone guessing one person's password from many networks at once escapes the per-network limit. Past this many times the limit in all, each sign-in for that person waits signin_everywhere_pace_seconds first. A device that signed in before is not slowed.",
    signinFailureLimitNetworkMultiple:
      "Many people can share one internet address, such as a household or a mobile carrier. One network may fail this many times the per-account limit before it is refused.",
    sortLocale:
      "Languages sort letters differently: Swedish puts Å, Ä and Ö after Z, German sorts Ä with A. This is the language whose order the libraries' lists follow, as a language tag such as en, sv or de.",
    timeZone:
      "Work the server does at set hours, like backups, runs by this time zone. Use a name from the time zone database, such as Europe/Berlin or America/New_York.",
    tmdbApiKey:
      "TMDB (The Movie Database) is where the server finds what each film and series is: titles, years, plots, cast and artwork. A free API key comes from your TMDB account's settings. Without one, nothing is identified.",
    tmdbBackgroundRateSharePercent:
      "TMDB limits how often it may be asked. The server's own work, such as identifying a new library, may use this share, so a person's search or match never waits behind it.",
    togetherDriftLeftAloneMs:
      "Players in a Watch Together group drift apart a little as they play. Drift under this is left alone, since correcting it would be noticed more than the drift.",
    togetherSeekDriftBeyondMs:
      "A small drift is closed by playing a little faster or slower for a moment. Past this, the player jumps to the group's position instead.",
    togetherSyncWaitSeconds:
      "When a film starts, or the host plays or jumps, every player waits until all are ready. After this long, those that are ready play without the rest, who catch up.",
    uploadCrowdedKeepsPercent:
      "While the upload is crowded, the films' share is lowered in steps, each keeping this much of the share before it.",
    uploadCrowdedSlowPlayersAtLeast:
      "One slow player is more likely its own link's problem, which its quality choice handles. This many slow at once means the house's upload is crowded, and the films' share is lowered.",
    uploadCrowdedSlowUnderPercent:
      "Something else in the house may be using the upload, such as a backup to the cloud. The server notices when several players at once receive less than they need. A player counts as slow when it gets less than this share of what its quality needs.",
    uploadFilmsSharePercent:
      "Of server_upload_mbps, the films take this share. The rest is kept for Watch Together's voices, artwork, and the bursts a player sends ahead of what it plays.",
    uploadRaiseUnderPercent:
      "After a playback was lowered for the upload, it is raised again only when the films' need has stayed under this share of their share for upload_raise_room_seconds, so raising one doesn't push the upload straight back over.",
    uploadSendAwayAfterSeconds:
      "When the upload stays overfull with every player at its lowest quality, nothing else can give. After this long, the newest Watch Together member is sent away, with a message saying why.",
    watchedLibraryScanEveryHours:
      "A watched library notices changes as they happen, but watching can miss one without knowing, for instance across a restart. A full scan this often finds anything missed.",
  },
  descriptions: {
    interestSaidEverySeconds:
      "How often an open film or series page tells the server it is still open, which puts its work first.",
    interestLapsesAfterSeconds:
      "How long after a page or a playback last said so that its work stops coming first.",
    argon2Iterations: "How many passes each password check makes.",
    argon2MemoryKib:
      "The memory each password check uses, in KiB. A change applies at each person's next sign-in.",
    argon2Parallelism: "How many lanes each password check uses.",
    artworkCacheCeilingGib:
      "How much disk the artwork cache may use, in GiB. Past it, images that can be made again are removed.",
    artworkCacheRecommendedHeadroomPercent:
      "The artwork cache limit the System page recommends, as a share of what is never removed.",
    artworkCacheSweepEveryHours: "How often the artwork cache is trimmed to its limit.",
    artworkCacheSweepToPercent:
      "How far under its limit the artwork cache is trimmed, so trimming does not run again at once.",
    artworkJpegQuality:
      "The quality artwork is saved at as JPEG, from 1 to 100, for devices without WebP.",
    artworkMaxSourcePixels:
      "The largest image, in pixels, the server will read. A larger one is refused.",
    artworkSecondsBeforeEachRetry:
      "How long to wait before each retry of an image that failed, comma-separated. After the last it gives up. Read when the server starts.",
    artworkServedAtFlushEverySeconds:
      "How often the server writes down when each image was last shown.",
    artworkWebpQuality: "The quality artwork is saved at as WebP, from 1 to 100.",
    assignedJobLimitHours:
      "How long a job given to a media node may run without an outcome before it counts as failed.",
    backupAgeWarningHours:
      "How old the newest backup may get before the server warns that backups have stopped.",
    backupEveryHours: "How often the server makes a backup. It skips one when nothing changed.",
    backupsKeptDaily: "How many days back one backup a day is kept.",
    backupsKeptHourly: "How many of the newest hourly backups are kept.",
    backupsKeptWeekly: "How many weeks back one backup a week is kept.",
    certificateDnsWaitSeconds:
      "How long a renewal's DNS record is given to spread before it is checked.",
    certificateRenewBeforeDays: "How long before it expires the server's certificate is renewed.",
    chunkCpuCeilingKeptSeconds:
      "How long a playback moved off a quality its node could not keep up with stays below it.",
    chunkHoldMs:
      "How long a request for a chunk not made yet is held before the player is told to ask again.",
    chunkInterestLapsesAfterSeconds:
      "How long a silent player keeps its film being encoded for a slow link.",
    chunkKeptAheadSeconds:
      "How much film past where a player paused or left is still encoded, so it plays again at once.",
    chunkKeptLeftSeconds: "How long chunks are kept after their player left the page.",
    chunkKeptLikelySeconds:
      "How long chunks a paused or silent player played are kept as likely to be played again.",
    chunkKeptMaybeSeconds:
      "How long chunks a paused or silent player played are kept as maybe played again, after which they are the first removed.",
    chunkLookaheadSeconds: "How far ahead of a player the film is encoded for a slow link.",
    chunkQualities:
      "The lower qualities a film is cut into for a slow link, highest first, each as <kbps>k@<height>p.",
    chunkQualitiesHardware:
      "The lower qualities a film is cut into for a slow link where its media node encodes on its graphics card, as chunk_qualities.",
    chunkRunSlowestSpeedPercent:
      "How fast each encoding for a slow link must run, compared to playback speed, before one is stopped to save the others.",
    chunkRunSpanSeconds: "The most film one encoding for a slow link makes before stopping.",
    chunkRunSpeedWindowSeconds: "How far back an encoding's speed is measured over.",
    chunkSoundKbps: "The sound's bit rate, in kbit/s, for films cut for a slow link.",
    continueWatchingMaxWorks: "How many titles Continue Watching shows at most.",
    derivedWorkSweepEverySeconds:
      "How often the server looks for owed matches, refreshes and images.",
    destinationFirstProbeDelaySeconds:
      "How long after an outside service is found down before it is tried again. Each later try waits twice as long.",
    destinationLongestProbeDelaySeconds:
      "The longest wait between tries of an outside service that stays down.",
    deviceCodeRevealWaitSeconds:
      "How long entering a device's code waits for the device to answer.",
    discoveryAddressCheckEverySeconds:
      "How often the server checks whether its addresses changed, telling the discovery service when they did.",
    discoveryAnotherCopyCheckEverySeconds:
      "How often a server refused as a copy of another asks again.",
    discoveryEndedCheckEverySeconds:
      "How often a server whose discovery service has ended asks again.",
    discoveryPendingCheckEverySeconds:
      "While waiting to be claimed, how often the server asks the discovery service whether it is.",
    discoveryRetryFirstSeconds:
      "How long to wait before calling the discovery service again after a failure. Each failure in a row doubles it.",
    discoveryRetryMostSeconds: "The longest wait before calling the discovery service again.",
    endedCredentialsKeptDays: "How long an ended sign-in is remembered before it is deleted.",
    finishedAfterPercent: "How far into a video it counts as watched and leaves Continue Watching.",
    finishedJobsKeptDays: "How long finished jobs stay in the job list.",
    hlsSegmentLengthSeconds:
      "The length of each segment of a prepared copy. Changing it remakes copies when they are next played.",
    invariantsCheckEveryHours:
      "How often the database is checked for consistency, besides at each start.",
    jobAttemptLimit: "How many times a failing background job is tried before it is marked failed.",
    jobFirstRetryDelaySeconds:
      "How long a failed job waits before its first retry. Each later retry waits twice as long.",
    jobHealthWindowHours: "How far back the System page counts failed jobs.",
    jobLongestRetryDelayHours: "The longest a failing job waits between retries.",
    jobViewMaxJobs: "How many of the newest jobs the job list shows.",
    linkShareForQualityPercent:
      "The share of a link's measured speed a quality may use when choosing one.",
    linkSpeedFreshHours: "How long a measured link speed is trusted for its device on its network.",
    loudnessMaxRaiseDb:
      "The most a quiet sound is raised, in dB, so its noise is not raised with it.",
    loudnessTargetLufs: "The loudness, in LUFS, that sound is raised or lowered towards.",
    mediaTokenIdleExpiryHours:
      "How long a playback's media address keeps working after its last use.",
    metadataRefreshCheckEveryHours:
      "How often the server looks for titles whose data is due to be fetched again.",
    readerLimitNetworkShare: "How many may read a network share at once.",
    newLibraryDefaultScanEveryHours:
      "How often a new library checked on a schedule looks for changes. Each library keeps its own once made.",
    nodeJobsBeyondFreeSlots:
      "How many jobs a media node is given beyond what it can run at once, so the next starts without waiting.",
    nodeSilentAfterSeconds:
      "How long a media node may go unheard before it is shown as silent. Each node reports every third of it.",
    opensubtitlesDownloadsReservedForPeoplePercent:
      "The share of the OpenSubtitles account's daily downloads kept back for people asking.",
    opensubtitlesRequestsPerSecond:
      "How many requests a second the server makes to OpenSubtitles at most.",
    pairingCodesPerNetworkMost: "How many pairing codes one network may have waiting at once.",
    pairingCodesPerSiteMost:
      "How many pairing codes one site of many networks may have waiting at once.",
    playLadderWaitMs:
      "How long a film not yet cut for slow links waits for that before playing another way.",
    playPriorityKeptSeconds:
      "How long playback work keeps priority over background work after it was last asked for.",
    portMappingLeaseSeconds: "How long each opening of the router's port is asked for.",
    portMappingPermanentRenewSeconds:
      "How often a permanent opening is asked for again, in case the router forgot it.",
    portMappingRetrySeconds: "How long before asking the router again when it refused.",
    prepareProgressEverySeconds: "How often, at most, a prepare in progress reports how far it is.",
    providerTimeoutSeconds:
      "How long a call to a metadata provider such as TMDB may take. Read when the server starts.",
    readThroughSweepEveryHours:
      "How often the server looks for videos whose loudness or timeline is not known yet.",
    registrationInvitationLifeDays: "How long an invitation to make an account keeps working.",
    removedFilesKeptDays:
      "How long a file gone from its disk is remembered, so putting it back keeps what was learned of it.",
    removedItemArtworkKeptDays:
      "How long a removed title's artwork is kept, in case its disk comes back.",
    inProgressAfterPercent:
      "How far into a video it counts as in progress: it resumes there and shows in Continue Watching. Before it, it plays from the start.",
    retentionSweepEveryHours:
      "How often ended sign-ins and removed files past their keep times are deleted.",
    scanRootErrorLimit:
      "How many errors one folder's scan may meet before it stops, so a broken share does not flood it.",
    scanSubtitleExtensions:
      "The file extensions a scan reads as subtitles beside their videos, comma-separated.",
    scanVideoExtensions: "The file extensions a scan reads as videos, comma-separated.",
    scanVideosPerExamineBatch: "How many videos of one folder a media node reads in one go.",
    scanWithheldRemovalsNamed: "How many files a held-back scan names when it asks you to confirm.",
    scanWithholdRemovalsOverFiles:
      "A scan that would remove more than this many files, and more than scan_withhold_removals_over_percent of a folder's, waits for you to confirm first. One that would remove every file always waits.",
    scanWithholdRemovalsOverPercent:
      "A scan that would remove more than this share of a folder's files waits for you to confirm first.",
    screenRefreshFailedWaitSeconds:
      "How long a screen that follows the server's background work waits before asking again after an ask failed.",
    screenRefreshFirstWaitSeconds:
      "How long a screen that follows the server's background work waits before asking again.",
    screenRefreshLongestWaitSeconds:
      "The longest a screen that follows the server's background work waits between asks, while nothing changes.",
    secretLastCharsShown:
      "How many of an API key's last characters are shown, to tell which key is set. Passwords show none.",
    serverUploadMbps:
      "How fast the server can send, in Mbit/s. Past it, players are moved to lower qualities. Unset, nothing is lowered.",
    sessionIdleExpiryHours: "How long a device may go unused before it is signed out.",
    sessionLastUsedWriteIntervalSeconds: "How often a session's last use is written down.",
    shouldCheckForUpdates:
      "Whether the server checks once a day for a new release. The check sends nothing but the request.",
    shouldMapPorts:
      "Whether the server asks your home router to open its port to the internet, so it can be reached from outside the home.",
    shouldScanOnNodeHello: "Whether a media node scans its library folders each time it connects.",
    shouldServeDebugMetrics: "Serve /metrics, on a loopback address only. Not built yet.",
    shouldServeDebugPprof:
      "Serve Go's profiler at /debug/pprof, on a loopback address only. Read when the server starts.",
    shouldShowDeveloperPages:
      "Whether the menu offers the pages for development, such as the UI gallery.",
    shouldTogetherNudgeKeepPitch:
      "Whether sound keeps its pitch while a Watch Together player is sped up or slowed down.",
    shouldTogetherVoiceBrowserLevelMicrophone:
      "Whether the browser evens out and cleans up the microphone. On Android with Firefox it turns the film down.",
    shouldTogetherVoiceCancelEcho:
      "Whether the browser cancels echo on the microphone. Not needed with headphones; on Android it turns the film down.",
    signinEverywherePaceSeconds:
      "How long each sign-in waits once a person has failed past signin_failure_limit_everywhere_multiple.",
    signinFailureAnswerSeconds:
      "How long a failed sign-in waits before it is answered, so its timing gives nothing away.",
    signinFailureLimit:
      "How many failed sign-ins an account or address may have before sign-in is refused for a while.",
    signinFailureLimitEverywhereMultiple:
      "How many times the limit a person may fail from all networks before each sign-in is slowed.",
    signinFailureLimitNetworkMultiple:
      "How many times the sign-in failure limit one network may fail, since many people can share one address.",
    signinFailureWindowSeconds: "How long a failed sign-in counts toward the limit.",
    sortLocale:
      "The language whose alphabetical order the libraries are sorted in, such as en, sv or de.",
    readerLimitSpinningDisk: "How many may read a spinning disk at once.",
    readerLimitSsd: "How many may read a solid-state disk at once.",
    systemPageRecentPreparesPerNode:
      "How many of a media node's finished prepares the System page lists.",
    timeZone:
      "The server's time zone, such as Europe/Berlin, for scheduled work like backups. Empty uses the machine's own.",
    tmdbBackgroundRateSharePercent:
      "The share of the TMDB rate the server's own background work may use, so a person's request always finds room.",
    tmdbRefreshAfterDays:
      "How long a title's TMDB data is kept before it is fetched again. 0 never fetches again.",
    tmdbRequestsPerSecond: "How many requests a second the server makes to TMDB at most.",
    togetherCatchUpBufferPercent:
      "How much a Watch Together player catching up aims to have loaded when it rejoins.",
    togetherDriftLeftAloneMs:
      "How far a Watch Together player may drift from the group before it is corrected.",
    togetherNudgeMostPercent:
      "The most a drifting Watch Together player is sped up or slowed down by.",
    togetherRunUpMs:
      "How much earlier a new Watch Together player starts, to learn how late it is.",
    togetherSeekDriftBeyondMs:
      "How far a Watch Together player may drift before it jumps instead of speeding up or slowing down.",
    togetherSeekOnlyDriftLeftAloneMs:
      "How far a Watch Together player that can only jump may drift before it does.",
    togetherSettleAfterChangeMs:
      "How long after a play, jump or speed change a Watch Together player's position is not checked.",
    togetherSnapSeeksBeyondSeconds:
      "How far a Watch Together jump must go to be moved to a keyframe.",
    togetherStartLeadMs:
      "How far ahead a Watch Together start is set, so every player starts at the same moment.",
    togetherSyncWaitSeconds:
      "How long a Watch Together group waits for everyone to be ready before playing without those who are not.",
    readerLimitUnknownDisk: "How many may read a disk of an unknown kind at once.",
    uploadCrowdedFloorPercent:
      "The lowest the films' share is lowered to, while the upload is crowded.",
    uploadCrowdedKeepsPercent:
      "The share of the films' upload each lowering keeps, while the upload is crowded.",
    uploadCrowdedLowerEverySeconds: "The least time between lowerings while the upload is crowded.",
    uploadCrowdedSlowPlayersAtLeast:
      "How many players must be slow at once for the upload to count as crowded.",
    uploadCrowdedSlowUnderPercent:
      "A player is slow when its link carries less than this share of what its quality needs.",
    uploadFilmsSharePercent:
      "The share of the server's upload the films may use. The rest is for voice, pictures and bursts.",
    uploadRaiseRoomSeconds:
      "How long the need must stay under upload_raise_under_percent before each raise of a lowered playback.",
    uploadRaiseUnderPercent:
      "How far under the films' share the need must stay before a lowered playback is raised again.",
    uploadSendAwayAfterSeconds:
      "How long the upload may stay overfull, with everything at its lowest, before the newest Watch Together member is sent away.",
    uploadUncrowdedForSeconds: "How long without crowding before the films' share grows back.",
    uploadUncrowdedGrowsPercent: "How much the films' share grows at each step back.",
    uploadUncrowdedRaiseEverySeconds:
      "How long between the films' share's steps back, once the upload is no longer crowded.",
    voiceRelayCredentialHours:
      "How long a Watch Together member's voice relay credentials are good for.",
    watchedLibraryScanEveryHours:
      "How often a watched library is scanned in full anyway, since watching for changes can miss one.",
  },
};
