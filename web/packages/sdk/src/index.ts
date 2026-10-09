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
export { rtcTogetherSocket } from "./rtc-together.ts";
export {
  answerLoad,
  carryMediaOver,
  isFetchMessage,
  isPathMessage,
  mediaPathPrefix,
  responseFromPort,
} from "./rtc-media.ts";
export type { FetchMessage, PathMessage, PortLike } from "./rtc-media.ts";
export type { Description, FetchRequest, Signaller } from "./rtc-transport.ts";
export {
  ApiProblem,
  dataOrThrow,
  problemFromAnswer,
  reportFailure,
  throwIfRefused,
} from "./problem.ts";
export type { ProblemBody, ProblemCode, ProblemWithCode, UnknownProblemBody } from "./problem.ts";
export type { WaitOptions } from "./ready.ts";
export { browserTogetherSocket, parseTogetherMessage, startingTuning } from "./together.ts";
export type {
  CannotKeepUp,
  DeviceRole,
  GroupEvent,
  GroupState,
  GroupView,
  OpenTogetherSocket,
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
export { driftCorrection, expectedPositionMs } from "./together-drift.ts";
export { followGroup, waitAheadAtMostMs } from "./together-follow.ts";
export type { FollowedMedia } from "./together-media.ts";
export {
  fellBehindEvent,
  playerBufferedFirstEvent,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerSwitchedQualityEvent,
} from "./together-player-events.ts";
export type { ChunkLoaded, QualitySwitched } from "./together-player-events.ts";
export { playedMutedEvent, soundOnAtTap } from "./together-sound.ts";
export type { MediaProbe, PlayableFormats } from "./playable-formats.ts";

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const sdkPackageName = "@orpheum/sdk";
