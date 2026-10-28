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
export {
  failureText,
  openLoopback,
  receivedLevelOf,
  testLoopback,
  toneTrack,
} from "./rtc-loopback.ts";
export type { Loopback, LoopbackResult } from "./rtc-loopback.ts";
export { browserFetch, textWhenReady, whenReady } from "./ready.ts";
export { connectRtc, isTurnedAway, TurnedAway } from "./rtc-connect.ts";
export { RtcTransport, SwitchableFetch } from "./rtc-transport.ts";
export {
  directFetch,
  directPathWaitMs,
  firstDirectPath,
  notWaitedFor,
  type DirectOutcome,
} from "./direct-path.ts";
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
  DiscoveryClient,
  DiscoverySignalling,
  Held,
  Proving,
  LinkFragment,
  Located,
  ManifestToCheck,
} from "./discovery.ts";
export { rtcTogetherSocket } from "./rtc-together.ts";
export {
  answerLoad,
  askPathOf,
  carryMediaOver,
  isClaimMessage,
  isFetchMessage,
  isPathMessage,
  isPathQueryMessage,
  loadAfterAsking,
  mediaPathPrefix,
  networkFetch,
  responseFromPort,
} from "./rtc-media.ts";
export type {
  ClaimMessage,
  FetchMessage,
  MediaCredentials,
  PathMessage,
  PathQueryMessage,
  PortLike,
} from "./rtc-media.ts";
export type { Description, Signaller } from "./rtc-connect.ts";
export type { FetchRequest } from "./rtc-transport.ts";
export { candidatePairsOf, pairsReportedMost, type CandidatePairSeen } from "./rtc-pairs.ts";
export {
  ApiProblem,
  dataOrThrow,
  problemFromAnswer,
  reportFailure,
  sendFailuresTo,
  throwIfRefused,
} from "./problem.ts";
export type { ProblemBody, ProblemCode, ProblemWithCode, UnknownProblemBody } from "./problem.ts";
export type { WaitOptions } from "./ready.ts";
export {
  browserTogetherSocket,
  everyRight,
  noRight,
  parseTogetherMessage,
  startingTuning,
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
export {
  createTogetherConnection,
  expectedGroupPositionMs,
  isLeaderOf,
  leaderOf,
} from "./together-connection.ts";
export { createLiveConnection } from "./live-connection.ts";
export { scrubPauseWindowMs } from "./together-lead.ts";
export type {
  CastSending,
  CastToReceive,
  LiveConnection,
  LiveSnapshot,
} from "./live-connection.ts";
export type {
  ConnectionStatus,
  GroupCommand,
  PlayerNote,
  PlayerState,
  Press,
  RelaysVoiceSignals,
  TellsCannotKeepUp,
  TellsStanding,
  TogetherConnection,
  TogetherSnapshot,
  VoiceCallStep,
} from "./together-connection.ts";
export { createTogetherVoice, microphoneFailureOf, partnerGainMost } from "./together-voice.ts";
export type {
  MicrophoneFailure,
  MicrophoneProcessing,
  VoiceFailure,
  TogetherVoice,
  TogetherVoiceOptions,
  VoiceCall,
  VoiceCallEvents,
  VoiceCallSetup,
  VoiceCallState,
  VoiceDevices,
  VoiceMicrophone,
  VoicePartner,
  VoiceSnapshot,
  VoiceStatus,
} from "./together-voice.ts";
export { browserVoiceDevices } from "./together-voice-browser.ts";
export type { BrowserVoiceDevices } from "./together-voice-browser.ts";
export { isVoiceLive } from "./voice-live.ts";
export { isFollowingAGroup } from "./following-a-group.ts";
export { driftCorrection, expectedPositionMs } from "./together-drift.ts";
export { followGroup, waitAheadAtMostMs } from "./together-follow.ts";
export type { FollowedMedia } from "./together-media.ts";
export { loadedAheadMsOf, startGateOf } from "./start-gate.ts";
export { readyAheadOf, setReadyAheadPercent, tellNextChunkLoadMs } from "./ready-ahead.ts";
export type { ReadyAhead } from "./ready-ahead.ts";
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
} from "./together-player-events.ts";
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
export type { PageAudio, ShownAgainPage } from "./page-audio.ts";
export type { MediaProbe, PlayableFormats } from "./playable-formats.ts";

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const sdkPackageName = "@orpheum/sdk";
