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
export { textWhenReady, whenReady } from "./ready.ts";
export {
  ApiProblem,
  dataOrThrow,
  problemFromAnswer,
  reportFailure,
  throwIfRefused,
} from "./problem.ts";
export type { ProblemBody, ProblemCode, ProblemWithCode, UnknownProblemBody } from "./problem.ts";
export type { WaitOptions } from "./ready.ts";
export { browserTogetherSocket, parseTogetherMessage } from "./together.ts";
export type {
  CannotKeepUp,
  GroupEvent,
  GroupState,
  GroupView,
  OpenTogetherSocket,
  RosterMember,
  TogetherEndReason,
} from "./together.ts";
export { createTogetherConnection } from "./together-connection.ts";
export type {
  ConnectionStatus,
  GroupCommand,
  PlayerNote,
  PlayerState,
  TellsCannotKeepUp,
  TogetherConnection,
  TogetherSnapshot,
} from "./together-connection.ts";
export {
  driftCorrection,
  expectedPositionMs,
  fellBehindEvent,
  followGroup,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerSwitchedQualityEvent,
} from "./together-follow.ts";
export type { ChunkLoaded, FollowedMedia, QualitySwitched } from "./together-follow.ts";
export type { MediaProbe, PlayableFormats } from "./playable-formats.ts";

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const sdkPackageName = "@orpheum/sdk";
