// SPDX-License-Identifier: MPL-2.0

import Hls, { type ErrorData } from "hls.js";

/**
 * Why a video could not play, as a code the app puts in the viewer's words: the player says no
 * English and passes on no browser or library text (standards/typescript.md, Errors).
 */
export type PlaybackFailure =
  /** The page has neither MSE nor the element's own HLS. */
  | "hls_unsupported"
  /** Loading stopped before it was done. */
  | "aborted"
  /** The media could not be fetched. */
  | "network"
  /** The media was fetched but could not be decoded. */
  | "decode"
  /** The browser does not play this format at all. */
  | "unsupported"
  /**
   * The server has no room to make this quality as fast as it plays: its run was evicted for the
   * other films its machine makes (D250 §6), which the core says with its own status, 429.
   */
  | "too_busy"
  /** It failed and said nothing more we know. */
  | "unknown";

/** Every playback failure, for a caller that must have words for each. */
export const playbackFailures: readonly PlaybackFailure[] = [
  "hls_unsupported",
  "aborted",
  "network",
  "decode",
  "unsupported",
  "too_busy",
  "unknown",
];

/** HTML's MediaError codes: MEDIA_ERR_ABORTED, _NETWORK, _DECODE and _SRC_NOT_SUPPORTED. */
const failureByMediaErrorCode: ReadonlyMap<number, PlaybackFailure> = new Map([
  [1, "aborted"],
  [2, "network"],
  [3, "decode"],
  [4, "unsupported"],
]);

/** A media element's error as a failure, by its code (HTML's MediaError); none is unknown. */
export function mediaErrorFailure(error: Pick<MediaError, "code"> | null): PlaybackFailure {
  return (error === null ? undefined : failureByMediaErrorCode.get(error.code)) ?? "unknown";
}

/** HTTP's 429: on a chunk's path, the core's word that it has no room to make it (D250 §6). */
export const noRoomToMakeStatus = 429;

/** HTTP's 425 Too Early: on a chunk's path, the core's word that it is being made, and to ask again. */
export const tooEarlyStatus = 425;

/** A fatal hls.js error as a failure, by its type, and a load's by the core's answer. */
export function hlsErrorFailure(data: Pick<ErrorData, "type" | "response">): PlaybackFailure {
  switch (data.type) {
    case Hls.ErrorTypes.NETWORK_ERROR:
      return data.response?.code === noRoomToMakeStatus ? "too_busy" : "network";
    case Hls.ErrorTypes.MEDIA_ERROR:
    case Hls.ErrorTypes.MUX_ERROR:
      return "decode";
    case Hls.ErrorTypes.KEY_SYSTEM_ERROR:
    case Hls.ErrorTypes.OTHER_ERROR:
      return "unknown";
  }
}
