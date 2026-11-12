// SPDX-License-Identifier: MPL-2.0

import Hls, { type ErrorData } from "hls.js";

/**
 * Why a video could not play, as a code the app puts in the viewer's words: the player says no
 * English and passes on no browser or library text (standards/typescript.md, Errors).
 */
export type PlaybackFailure =
  /** The page has neither MSE nor the element's own HLS. */
  | "hlsUnsupported"
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
  | "tooBusy"
  /**
   * The server failed on a part of the film the same way each time it tried to make it, which the
   * core says with its own status, 422 (cannotConvert); asking again fails the same way.
   */
  | "cannotMake"
  /** It failed and said nothing more we know. */
  | "unknown";

/**
 * Told when a playback cannot go on: why, as a code for the viewer's words, and, where the player
 * knows it, what failed, in a line for the core's log alone, never for the viewer (failureText).
 */
export type PlaybackFailed = (failure: PlaybackFailure, whatFailed?: string) => void;

/** Every playback failure, for a caller that must have words for each. */
export const playbackFailures: readonly PlaybackFailure[] = [
  "hlsUnsupported",
  "aborted",
  "network",
  "decode",
  "unsupported",
  "tooBusy",
  "cannotMake",
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

/**
 * HTTP's 422: on a chunk's path, the core's word that FFmpeg failed on the film at this chunk each
 * time it was made (cannotConvert), which asking again does not change. Taken for a load that only
 * stopped, it was asked for every two seconds, the film standing black with no word (a film,
 * staging, 2026-11-03; toolkit.md item 12).
 */
export const cannotMakeStatus = 422;

/**
 * HTTP's 503: on a chunk's path, the core's word that its film cannot be read now, its machine away
 * or its disk not answering (media_unreachable, D241 §7), which asking again does not change: the
 * page asks the server why, says so, and opens the film again once it can be read.
 */
export const unreachableStatus = 503;

/**
 * HTTP's 404: on a chunk's or a playlist's path, the core's word that it no longer has the playback
 * (playbackGone: it restarted since the playback began, or ended it) or never offered what was asked,
 * which asking again does not change: the page's report learns why and opens the playback again
 * where it was. Taken for a load that only stopped, a Mac's Chrome asked for one chunk of a gone
 * playback about once a second for 11 hours (staging, 2026-11-12).
 */
export const goneStatus = 404;

/** HTTP's 425 Too Early: on a chunk's path, the core's word that it is being made, and to ask again. */
export const tooEarlyStatus = 425;

/**
 * The status of a load nothing answered: its connection was lost on its way (a WebRTC connection
 * taken as gone, an HTTP connection reset), as XMLHttpRequest says it and fetchLoader says a fetch
 * that failed. Never the core's word: each of those has a status of its own.
 */
export const unansweredStatus = 0;

/**
 * Whether a fatal hls.js error only stopped its loading, which asking it to load on may mend: any but
 * the media's own (MEDIA_ERROR, MUX_ERROR), which alone end a playback, a load failing or timing out,
 * its playlist's or its chunk's, or an exception hls.js caught in its own handling; never the core's
 * 404, 422, 429 or 503, which asking again does not change and the page answers in its own way (D250 §6,
 * D241 §7). A load that failed while the owner's Chrome was offline ended its playback for good,
 * though the link came back (staging, 2026-10-29; VQ125).
 */
export function isLoadStopped(data: Pick<ErrorData, "type" | "fatal" | "response">): boolean {
  const code = data.response?.code;
  return (
    data.fatal &&
    data.type !== Hls.ErrorTypes.MEDIA_ERROR &&
    data.type !== Hls.ErrorTypes.MUX_ERROR &&
    code !== noRoomToMakeStatus &&
    code !== cannotMakeStatus &&
    code !== unreachableStatus &&
    code !== goneStatus
  );
}

/** A load's failure by the core's answer: its own 429 and 422 say why; any other, the network. */
function networkFailure(code: number | undefined): PlaybackFailure {
  if (code === noRoomToMakeStatus) {
    return "tooBusy";
  }
  return code === cannotMakeStatus ? "cannotMake" : "network";
}

/** A fatal hls.js error as a failure, by its type, and a load's by the core's answer. */
export function hlsErrorFailure(data: Pick<ErrorData, "type" | "response">): PlaybackFailure {
  switch (data.type) {
    case Hls.ErrorTypes.NETWORK_ERROR:
      return networkFailure(data.response?.code);
    case Hls.ErrorTypes.MEDIA_ERROR:
    case Hls.ErrorTypes.MUX_ERROR:
      return "decode";
    case Hls.ErrorTypes.KEY_SYSTEM_ERROR:
    case Hls.ErrorTypes.OTHER_ERROR:
      return "unknown";
  }
}
