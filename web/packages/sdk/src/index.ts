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
export { browserFetch, textWhenReady, whenReady } from "./ready.ts";
export { connectRtc, RtcTransport, SwitchableFetch } from "./rtc-transport.ts";
export { directFetch, directPathWaitMs, firstDirectPath } from "./direct-path.ts";
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
  discoverySignaller,
  fromBase64,
  locate,
  locateHash,
  locatorHash,
  NotFound,
  offerKey,
  pathsOf,
  protocolVersion,
  readLinkFragment,
  reportDeadEnd,
  sessionOfferKey,
} from "./discovery.ts";
export type {
  DiscoveryClient,
  DiscoverySignalling,
  Held,
  Proving,
  LinkFragment,
  Located,
} from "./discovery.ts";
export { rtcTogetherSocket } from "./rtc-together.ts";
export {
  answerLoad,
  carryMediaOver,
  isClaimMessage,
  isFetchMessage,
  isPathMessage,
  isPathQueryMessage,
  loadAfterAsking,
  mediaPathPrefix,
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
export type { Description, FetchRequest, Signaller } from "./rtc-transport.ts";
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
  GroupEvent,
  GroupState,
  GroupView,
  OpenTogetherSocket,
  PlaybackRights,
  RosterMember,
  SyncTuning,
  TogetherEndReason,
  VoiceSignal,
} from "./together.ts";
export { createTogetherConnection } from "./together-connection.ts";
export type {
  ConnectionStatus,
  GroupCommand,
  PlayerNote,
  PlayerState,
  RelaysVoiceSignals,
  TellsCannotKeepUp,
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
export type { StartLagMemory } from "./together-start.ts";
export type { FollowedMedia } from "./together-media.ts";
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
  playerSwitchedQualityEvent,
} from "./together-player-events.ts";
export type {
  ChunkLoaded,
  LoadingHold,
  LoadingRunway,
  QualitySwitched,
} from "./together-player-events.ts";
export { playedMutedEvent, readyForSoundAtTap, soundOnAtTap } from "./together-sound.ts";
export { healPageAudio, keepPageAudio, setPageAudio } from "./page-audio.ts";
export type { PageAudio, ShownAgainPage } from "./page-audio.ts";
export type { MediaProbe, PlayableFormats } from "./playable-formats.ts";

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const sdkPackageName = "@orpheum/sdk";
