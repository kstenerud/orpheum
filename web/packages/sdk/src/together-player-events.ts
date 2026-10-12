// SPDX-License-Identifier: MPL-2.0

/**
 * The events a player fires at its video for the follower of its group: that it moved itself, fell
 * behind, loaded a chunk or switched quality, and what the last two carry; and the one the follower
 * fires at it, to hold its loading while the group waits.
 */

/**
 * The event a player fires at its video as it moves the video itself, before the seeking that
 * follows: hls.js steps over a gap in what it loaded, or nudges a stalled video on. That seek is
 * the player's, not its person's, and a leading player does not send it to the group (the owner's
 * test, 2026-10-05, where each such step became a jump for everyone).
 */
export const playerMovedItselfEvent = "orpheumplayermoveditself";

/**
 * The event a player fires at its video as the first chunk of each of its playlists, the picture's
 * ("main") and the sound's ("audio"), is in its buffer: the follower's page notes what the player
 * holds then and how hls.js placed it, since an iPhone that jumped 10 s at its start, the chunks'
 * offset, left only a guess at whether that placing was wrong (the sync review, 2026-10-08).
 */
export const playerBufferedFirstEvent = "orpheumplayerbufferedfirst";

/**
 * The event a following player fires at its video as it seeks to catch up with a group it fell
 * behind, having once been in step: a stall in all but name, which a player on a slow link never
 * otherwise counts, every load being a seek's (D247 §2; the owner's test, 2026-10-05).
 */
export const fellBehindEvent = "orpheumfellbehind";

/**
 * The event a following player fires at its video just before it seeks to catch up with its group:
 * that seek is no person's or group's jump, and its player does not say a seek took it there, so it
 * waits for the run already making its quality (D250 §5, D284 §2).
 */
export const catchUpSeekEvent = "orpheumcatchupseek";

/**
 * The event a following player fires at its video as its group jumps: a player on a quality the
 * server makes starts the jump on the group's common one, one encode for all, and settles after
 * (D286 §3).
 */
export const groupJumpedEvent = "orpheumgroupjumped";

/**
 * The event a player fires at its video as it loads a chunk: a ChunkLoaded in its detail, which a
 * following player tells the server for its log.
 */
export const playerLoadedChunkEvent = "orpheumplayerloadedchunk";

/** One chunk a player loaded, as playerLoadedChunkEvent tells it. */
export interface ChunkLoaded {
  readonly track: "video" | "audio";
  readonly loadMs: number;
  readonly loadedKbps: number;
  /** The quality the chunk is of, in kbit/s. */
  readonly qualityKbps: number;
  /** How much film the chunk plays, in ms: with loadMs, whether the link keeps up with the film. */
  readonly filmMs: number;
}

/**
 * The event a player fires at its video as it begins playing another quality: a QualitySwitched
 * in its detail, which a following player tells the server for its log.
 */
export const playerSwitchedQualityEvent = "orpheumplayerswitchedquality";

/**
 * A quality a player began playing, as playerSwitchedQualityEvent tells it, with the link's
 * estimate then: undefined before a chunk measured it, when the estimate is only hls.js's starting
 * value.
 */
export interface QualitySwitched {
  readonly qualityKbps: number;
  readonly linkEstimateKbps: number | undefined;
}

/** The detail an event carries, when it is a CustomEvent whose detail has every one of fields. */
function detailWith(event: Event, fields: readonly string[]): Record<string, unknown> | undefined {
  if (!(event instanceof CustomEvent)) {
    return undefined;
  }
  const detail: unknown = event.detail;
  if (typeof detail !== "object" || detail === null) {
    return undefined;
  }
  const record: Record<string, unknown> = { ...detail };
  return fields.every((field) => field in record) ? record : undefined;
}

/** A playerLoadedChunkEvent's chunk, or undefined when the event carries none. */
export function chunkLoadedOf(event: Event): ChunkLoaded | undefined {
  const detail = detailWith(event, ["track", "loadMs", "loadedKbps", "qualityKbps", "filmMs"]);
  const { track, loadMs, loadedKbps, qualityKbps, filmMs } = detail ?? {};
  return (track === "video" || track === "audio") &&
    typeof loadMs === "number" &&
    typeof loadedKbps === "number" &&
    typeof qualityKbps === "number" &&
    typeof filmMs === "number"
    ? { track, loadMs, loadedKbps, qualityKbps, filmMs }
    : undefined;
}

/** A playerSwitchedQualityEvent's quality, or undefined when the event carries none. */
export function qualitySwitchedOf(event: Event): QualitySwitched | undefined {
  const { qualityKbps, linkEstimateKbps } = detailWith(event, ["qualityKbps"]) ?? {};
  return typeof qualityKbps === "number" &&
    (linkEstimateKbps === undefined || typeof linkEstimateKbps === "number")
    ? { qualityKbps, linkEstimateKbps }
    : undefined;
}

/**
 * The event a following player fires at its video to hold its loading, or let it go on: a
 * LoadingHold in its detail. While its group waits at a sync point and it is ready, it holds, so
 * the players not yet ready have the server's upload to themselves; it goes on as the group plays
 * (the owner's jumps, 2026-10-12, where the ready players went on filling 30 s ahead).
 */
export const holdLoadingEvent = "orpheumholdloading";

/** Whether a player is to hold its loading, as holdLoadingEvent tells it. */
export interface LoadingHold {
  readonly isHeld: boolean;
}

/** The LoadingHold an event carries, if it is one. */
export function loadingHoldOf(event: Event): LoadingHold | undefined {
  const { isHeld } = detailWith(event, ["isHeld"]) ?? {};
  return typeof isHeld === "boolean" ? { isHeld } : undefined;
}

/**
 * The event a following player fires at its video to have it load no more than a runway ahead, or
 * as far as it would: a LoadingRunway in its detail. While another player in its group is still
 * loading, it keeps a short runway, so the one behind has the server's upload (D285 §2).
 */
export const loadingRunwayEvent = "orpheumloadingrunway";

/** How far ahead a player is to load at most, in seconds; undefined for as far as it would. */
export interface LoadingRunway {
  readonly aheadS: number | undefined;
}

/** The LoadingRunway an event carries, if it is one. */
export function loadingRunwayOf(event: Event): LoadingRunway | undefined {
  const detail = detailWith(event, ["aheadS"]);
  if (detail === undefined) {
    return undefined;
  }
  const { aheadS } = detail;
  return aheadS === undefined || typeof aheadS === "number" ? { aheadS } : undefined;
}
