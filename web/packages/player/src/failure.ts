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
  /** It failed and said nothing more we know. */
  | "unknown";

/** Every playback failure, for a caller that must have words for each. */
export const playbackFailures: readonly PlaybackFailure[] = [
  "hls_unsupported",
  "aborted",
  "network",
  "decode",
  "unsupported",
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

/** A fatal hls.js error as a failure, by its type. */
export function hlsErrorFailure(data: Pick<ErrorData, "type">): PlaybackFailure {
  switch (data.type) {
    case Hls.ErrorTypes.NETWORK_ERROR:
      return "network";
    case Hls.ErrorTypes.MEDIA_ERROR:
    case Hls.ErrorTypes.MUX_ERROR:
      return "decode";
    case Hls.ErrorTypes.KEY_SYSTEM_ERROR:
    case Hls.ErrorTypes.OTHER_ERROR:
      return "unknown";
  }
}
