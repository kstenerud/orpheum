// SPDX-License-Identifier: MPL-2.0

/**
 * The events a player fires at its video for the follower of its group: that it moved itself, fell
 * behind, loaded a chunk or switched quality, and what the last two carry.
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
 * The event a player fires at its video as it loads a chunk: a ChunkLoaded in its detail, which a
 * following player tells the server for its log.
 */
export const playerLoadedChunkEvent = "orpheumplayerloadedchunk";

/** One chunk a player loaded, as playerLoadedChunkEvent tells it. */
export interface ChunkLoaded {
  readonly track: "video" | "audio";
  readonly loadMs: number;
  readonly kbps: number;
  /** The quality the chunk is of, in kbit/s. */
  readonly qualityKbps: number;
}

/**
 * The event a player fires at its video as it begins playing another quality: a QualitySwitched
 * in its detail, which a following player tells the server for its log.
 */
export const playerSwitchedQualityEvent = "orpheumplayerswitchedquality";

/** A quality a player began playing, as playerSwitchedQualityEvent tells it. */
export interface QualitySwitched {
  readonly qualityKbps: number;
  readonly estimateKbps: number;
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
  const detail = detailWith(event, ["track", "loadMs", "kbps", "qualityKbps"]);
  const { track, loadMs, kbps, qualityKbps } = detail ?? {};
  return (track === "video" || track === "audio") &&
    typeof loadMs === "number" &&
    typeof kbps === "number" &&
    typeof qualityKbps === "number"
    ? { track, loadMs, kbps, qualityKbps }
    : undefined;
}

/** A playerSwitchedQualityEvent's quality, or undefined when the event carries none. */
export function qualitySwitchedOf(event: Event): QualitySwitched | undefined {
  const { qualityKbps, estimateKbps } = detailWith(event, ["qualityKbps", "estimateKbps"]) ?? {};
  return typeof qualityKbps === "number" && typeof estimateKbps === "number"
    ? { qualityKbps, estimateKbps }
    : undefined;
}
