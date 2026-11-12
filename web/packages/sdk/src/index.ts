// SPDX-License-Identifier: MPL-2.0

/**
 * The public entry point of the Orpheum SDK: the only module other packages and apps import
 * from this package (`exports` in package.json forbids deeper paths).
 *
 * The SDK is React-free. It holds the generated API types and client, the playable-formats probe
 * (what this browser can play), the wait for a file the server is still making (whenReady,
 * textWhenReady), the reading of refusals, and the Watch Together client: the session socket's
 * connection and a player following its group (plans/cut-a.md §3, §5).
 */

export { apiPrefix, artUrl, createApi } from "./client.ts";
export type { Api, ApiOptions, ArtSize } from "./client.ts";
export type { components, operations, paths } from "./gen/api.ts";
export { allPages, pageLimitMax } from "./pages.ts";
export type { ListPage, PageQuery } from "./pages.ts";
export { browserPlayableFormats, playableFormatsFrom } from "./playable-formats.ts";
export { canFollowAGroup } from "./follow-support.ts";
export type { GlobalOf } from "./follow-support.ts";
export {
  audioContextClass,
  browserDeviceFacts,
  codecAnswersFrom,
  countAudioOutputs,
  deviceReportFrom,
  fireTvHintOf,
  fireTvPattern,
  isFireTv,
  labCodecs,
  labFeatureNames,
  measureAudioLatency,
  nativeHlsTypes,
} from "./device-report.ts";
export type {
  AudioLatency,
  CodecAnswer,
  CodecProbe,
  DeviceFacts,
  DeviceReport,
  LabFeature,
} from "./device-report.ts";
export { megabitsPerSecond, timeLoad } from "./throughput.ts";
export type { LoadTiming } from "./throughput.ts";
export {
  failureText,
  openLoopback,
  receivedLevelOf,
  testLoopback,
  toneTrack,
} from "./rtc-loopback.ts";
export type { Loopback, LoopbackResult } from "./rtc-loopback.ts";
export { browserFetch, textWhenReady, whenReady } from "./ready.ts";
export {
  connectRtc,
  isSecureUnfinished,
  isTurnedAway,
  RtcUnconnected,
  TurnedAway,
} from "./rtc-connect.ts";
export { connectThrough, provingOf, signallerOf, targetOfSession } from "./core-target.ts";
export type { HeldSession, KeptCore, Target } from "./core-target.ts";
export { isUnreached, reaching, unreachedRetryMs } from "./rtc-reopen.ts";
export { RtcTransport, SwitchableFetch } from "./rtc-transport.ts";
export { SilentRequestError } from "./rtc-request.ts";
export {
  directFetch,
  directPathWaitMs,
  endingOf,
  firstDirectPath,
  notWaitedFor,
  type DirectEnding,
  type DirectOutcome,
} from "./direct-path.ts";
export { ipv6ProbeFastMs, probeIpv6 } from "./ipv6-probe.ts";
export { isNetworkFailure, namingFailures, PathKeeper } from "./path-keeper.ts";
export type { PathWays } from "./path-keeper.ts";
export {
  isRealm,
  masterRealm,
  realmAsk,
  realmKey,
  realmName,
  realmPerson,
  realmSignIn,
  realmTag,
} from "./realm.ts";
export type { RealmFound, RealmTyped } from "./realm.ts";
export {
  deviceCheckCode,
  deviceCodeState,
  deviceCommitment,
  deviceKeyHash,
  newDeviceKey,
  newDeviceNonce,
  openDeviceOutcome,
  requestDeviceCode,
  revealDeviceNonce,
} from "./device-code.ts";
export type {
  ClaimedBy,
  DeviceAsking,
  DeviceCode,
  DeviceCodeState,
  DeviceKey,
  DeviceOutcome,
} from "./device-code.ts";
export {
  base64url,
  clientManifestMessage,
  discoverySignaller,
  fromBase64,
  handOffOfferKey,
  locate,
  locateHash,
  locatorHash,
  NotFound,
  TooMany,
  offerKey,
  pathsOf,
  CoreOffline,
  coreNotConnectedRefusal,
  isCoreOffline,
  isKeyRefused,
  KeyRefused,
  proofRefusal,
  protocolVersion,
  readLinkFragment,
  refusedClientVersions,
  reportDeadEnd,
  sessionOfferKey,
  verifyClientManifest,
} from "./discovery.ts";
export type {
  DeadEndCause,
  DeadEndReason,
  DiscoveryClient,
  DiscoverySignalling,
  Families,
  Family,
  HasIpv6,
  Held,
  Proving,
  LinkFragment,
  Located,
  ManifestToCheck,
} from "./discovery.ts";
export { rtcTogetherSocket } from "./rtc-together.ts";
export { mediaCarrierOf, readMediaCarrierFrom, tellMediaCarriers } from "./media-carrier.ts";
export {
  answerLoad,
  askPathOf,
  carryMediaOver,
  isClaimMessage,
  isFetchMessage,
  isPathMessage,
  isPathNoteMessage,
  isPathQueryMessage,
  loadAfterAsking,
  mediaPathPrefix,
  networkFetch,
  relayedAheadBytes,
  reportPathNotes,
  responseFromPort,
  sayMediaGoesOverHttp,
} from "./rtc-media.ts";
export {
  controlWaitMs,
  isPageWorkerBypassed,
  isWorkerBypassed,
  isWorkerProbe,
  NoServiceWorker,
  PageNotControlled,
  workerProbeAnswer,
  workerProbePath,
  workerProbeResponse,
} from "./worker-control.ts";
export { forgetSteps, noteStep, stepsSoFar } from "./steps.ts";
export type {
  ClaimMessage,
  FetchMessage,
  MediaCredentials,
  PathMessage,
  PathNoteMessage,
  PathQueryMessage,
  PortLike,
} from "./rtc-media.ts";
export type { Description, Signaller } from "./rtc-connect.ts";
export type { CarrierSeen, FetchRequest } from "./rtc-transport.ts";
export { candidatePairsOf, pairsReportedMost, type CandidatePairSeen } from "./rtc-pairs.ts";
export {
  ApiProblem,
  dataOrThrow,
  noteForTheLog,
  problemFromAnswer,
  reportFailure,
  sendFailuresTo,
  throwIfRefused,
} from "./problem.ts";
export type { SentLevel } from "./problem.ts";
export type { ProblemBody, ProblemCode, ProblemWithCode, UnknownProblemBody } from "./problem.ts";
export type { WaitOptions } from "./ready.ts";
export {
  browserTogetherSocket,
  everyRight,
  noRight,
  parseTogetherMessage,
  startingTuning,
  togetherProtocolVersion,
} from "./together.ts";
export type {
  CannotKeepUp,
  DeviceRole,
  DeviceStatus,
  GroupEvent,
  GroupState,
  GroupView,
  OpenTogetherSocket,
  PlaybackReport,
  PlaybackRights,
  RosterDevice,
  RosterMember,
  Standing,
  SyncTuning,
  TogetherEndReason,
  VoiceSignal,
} from "./together.ts";
export { createTogetherConnection, expectedGroupPositionMs } from "./together-connection.ts";
export { isLeaderOf, leaderOf } from "./together-roster.ts";
export { createLiveConnection } from "./live-connection.ts";
export { scrubPauseWindowMs } from "./together-lead.ts";
export type {
  CastSending,
  CastToReceive,
  LiveConnection,
  LiveSnapshot,
} from "./live-connection.ts";
export type { PictureChoice, PlaybackPicture, SubtitleOption } from "./playback-picture.ts";
export type { PlaybackSound, SoundChannel, SoundChoice, SoundTrack } from "./playback-sound.ts";
export type {
  CarriesPictureChoices,
  CarriesSoundChoices,
  ConnectionStatus,
  GroupCommand,
  PlayerNote,
  PlayerState,
  Press,
  RelaysVoiceSignals,
  TellsCannotKeepUp,
  TellsPlaybackAlone,
  TellsStanding,
  TogetherConnection,
  TogetherSnapshot,
  VoiceCallStep,
} from "./together-connection.ts";
export { createTogetherVoice, partnerGainMost } from "./together-voice.ts";
export type {
  TogetherVoice,
  TogetherVoiceOptions,
  VoiceSnapshot,
  VoiceStatus,
} from "./together-voice.ts";
export { microphoneFailureOf } from "./together-voice-call.ts";
export type {
  MicrophoneFailure,
  MicrophoneProcessing,
  VoiceFailure,
  VoiceCall,
  VoiceCallEvents,
  VoiceCallSetup,
  VoiceCallState,
  VoiceDevices,
  VoiceMicrophone,
  VoicePartner,
} from "./together-voice-call.ts";
export { browserVoiceDevices } from "./together-voice-browser.ts";
export type { BrowserVoiceDevices } from "./together-voice-browser.ts";
export { isVoiceLive } from "./voice-live.ts";
export { isFollowingAGroup } from "./following-a-group.ts";
export { driftCorrection, expectedPositionMs } from "./together-drift.ts";
export { followGroup, tellsNoFilmHere } from "./together-follow-group.ts";
export { waitAheadAtMostMs } from "./together-follow.ts";
export type { FollowedMedia } from "./together-media.ts";
export { isAskedToPlay, loadedAheadMsOf, startGateOf } from "./start-gate.ts";
export {
  readyAheadOf,
  refillAfterStallOf,
  runAfterStallMs,
  setReadyAheadPercent,
  tellNextChunkLoadMs,
} from "./ready-ahead.ts";
export type { NextChunkLoad, NextChunkLoadFrom, ReadyAhead } from "./ready-ahead.ts";
export type { GatedMedia, StartGate, StartWait } from "./start-gate.ts";
export {
  catchUpSeekEvent,
  chunkLoadedOf,
  groupJumpedEvent,
  fellBehindEvent,
  holdLoadingEvent,
  loadingHoldOf,
  loadingRunwayEvent,
  loadingRunwayOf,
  playerBufferedFirstEvent,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerMovedSoundEvent,
  playerSwitchedQualityEvent,
  soundMovedOf,
} from "./together-player-events.ts";
export { PlaybackTold } from "./together-leader.ts";
export type {
  ChunkLoaded,
  LoadingHold,
  LoadingRunway,
  QualitySwitched,
  SoundMoved,
} from "./together-player-events.ts";
export {
  pageActivation,
  playedMutedEvent,
  readyForSoundAtTap,
  soundOnAtTap,
} from "./together-sound.ts";
export { healPageAudio, keepPageAudio, setPageAudio } from "./page-audio.ts";
export type { PageAudio } from "./page-audio.ts";
export type { ShownPage } from "./shown-page.ts";
export type { MediaProbe, PlayableFormats } from "./playable-formats.ts";

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const sdkPackageName = "@orpheum/sdk";
export {
  causeOfFrame,
  howFrozen,
  isPageHidden,
  longTasksOf,
  mainThreadSampleMs,
  mainThreadStallAtLeastMs,
  watchFreezes,
} from "./main-thread-freezes.ts";
export type {
  FrameScript,
  Freeze,
  FreezeWatch,
  LongTaskEntry,
  LongTaskObserverClass,
  LongTaskScope,
  LongTasks,
} from "./main-thread-freezes.ts";
