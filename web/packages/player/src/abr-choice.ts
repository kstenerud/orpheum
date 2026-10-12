// SPDX-License-Identifier: MPL-2.0

/**
 * The choices of the chunked player's bitrate controller, each a function of what it is told
 * (D265): where it starts, when it gives up a chunk on its way, what it loads next, what voice
 * leaves the film, and what counts as a measurement of the link. abr.ts asks them.
 */

import { haveFutureData } from "./stalls.ts";

/** A quality as the controller reads it: its peak bit rate, BANDWIDTH, and its average, the sound's included. */
export interface ChoiceLevel {
  readonly bitrate: number;
  readonly averageBitrate: number;
}

/**
 * The share of a link's measured speed a quality may take with little loaded ahead, and when a
 * chunk is given up: the core chooses a quality by the same share (D247 §6).
 */
export const linkShareForQuality = 0.7;

/** The share of the link a quality may take with plenty loaded ahead (D265 §4). */
export const linkShareWhenAhead = 0.95;

/** How far ahead, in seconds, the share starts rising from linkShareForQuality, and reaches linkShareWhenAhead. */
export const shareRisesFromS = 10;
export const shareRisesToS = 25;

/** The share of the link the film may take while voice is live (D262 §2). */
export const voiceLinkShare = 0.5;

/**
 * A load quicker than this came from the browser's cache, not over the link (D265 §7): the guess
 * where the browser does not say (LoadedChunk.isFromCache).
 */
export const cacheHitUnderMs = 10;

/**
 * The share of its pace a paced chunk arrives at or below to count: slower than that, the link
 * held it back, not the pace (D265 §7).
 */
export const pacedCountsAtMostShare = 0.9;

/** The highest quality within startAtBps by its BANDWIDTH, or the lowest (D247 §6, D260 §3). */
export function startingLevel(levels: readonly ChoiceLevel[], startAtBps: number): number {
  return Math.max(
    levels.findLastIndex((level) => level.bitrate <= startAtBps),
    0,
  );
}

/** The share of the link a quality may take with bufferAheadS loaded ahead (D265 §4). */
export function linkShare(bufferAheadS: number): number {
  const along = (bufferAheadS - shareRisesFromS) / (shareRisesToS - shareRisesFromS);
  const clamped = Math.min(1, Math.max(0, along));
  return linkShareForQuality + clamped * (linkShareWhenAhead - linkShareForQuality);
}

/** The highest quality no higher than highest whose average is within bps; the lowest when none is. */
export function highestWithin(
  levels: readonly ChoiceLevel[],
  highest: number,
  bps: number,
): number {
  return Math.max(
    levels.findLastIndex((level, index) => index <= highest && level.averageBitrate <= bps),
    0,
  );
}

/** What the controller knows of a chunk on its way. */
export interface ChunkOnItsWay {
  readonly levels: readonly ChoiceLevel[];
  /** The quality the chunk is of, and the lowest the player may play. */
  readonly loadingLevel: number;
  readonly lowestLevel: number;
  /** How long the chunk plays for, in seconds. */
  readonly durationS: number;
  /** How long it has been loading, and when its first byte came, both in ms from when it began. */
  readonly loadingMs: number;
  readonly firstByteMs: number | undefined;
  /** How many of its bytes have arrived, and how many it has; 0 while that is not known. */
  readonly loadedBytes: number;
  readonly totalBytes: number;
  /** The link as measured, bits a second; undefined before anything was. */
  readonly linkBps: number | undefined;
  /** How long what is loaded ahead of the position plays for, in seconds. */
  readonly bufferAheadS: number;
}

/**
 * How fast the chunk is arriving, bits a second: its bytes since its first, or, before any came, the
 * link as measured, as hls.js judges a chunk with none yet; a chunk the core is still making is
 * waited for on a link that carries it (D250 §5). Before anything was measured, nothing.
 */
function arrivingBps(chunk: ChunkOnItsWay): number {
  if (chunk.firstByteMs === undefined || chunk.loadedBytes <= 0) {
    return chunk.linkBps ?? 0;
  }
  const arrivingMs = Math.max(1, chunk.loadingMs - chunk.firstByteMs);
  return (chunk.loadedBytes * 8 * 1000) / arrivingMs;
}

/** How long, in seconds, bytes take to arrive at bps; forever when nothing arrives. */
function secondsFor(bytes: number, bps: number): number {
  return bps > 0 ? (bytes * 8) / bps : Number.POSITIVE_INFINITY;
}

/** Whether a chunk on its way may be given up: above the lowest quality, and loading for half the time it plays. */
function canBeGivenUp(chunk: ChunkOnItsWay): boolean {
  return chunk.loadingLevel > chunk.lowestLevel && chunk.loadingMs >= (chunk.durationS * 1000) / 2;
}

/**
 * The quality a chunk on its way is given up for, or undefined to wait for it (D265 §3): once it
 * has loaded for half the time it plays, a chunk arriving slower than its quality plays, the rest
 * of which would take longer than what is loaded ahead lasts, is given up for the highest quality
 * within linkShareForQuality of the speed it arrives at, the lowest included, when that quality's
 * whole chunk would arrive sooner. hls.js gives up on a chunk only while the video plays, and never
 * for the lowest quality: a film's page filling its hidden player, and a player waiting for its
 * first frame, waited out a whole chunk of the film's own picture (staging, 2026-10-07).
 */
export function giveUpFor(chunk: ChunkOnItsWay): number | undefined {
  const { levels, loadingLevel, durationS } = chunk;
  const loading = levels[loadingLevel];
  if (loading === undefined || !canBeGivenUp(chunk)) {
    return undefined;
  }
  const bps = arrivingBps(chunk);
  if (bps >= loading.averageBitrate) {
    return undefined;
  }
  const wholeBytes =
    chunk.totalBytes > 0 ? chunk.totalBytes : (loading.averageBitrate * durationS) / 8;
  const leftS = secondsFor(Math.max(0, wholeBytes - chunk.loadedBytes), bps);
  if (leftS <= chunk.bufferAheadS) {
    return undefined;
  }
  const chosen = Math.max(
    highestWithin(levels, loadingLevel - 1, linkShareForQuality * bps),
    chunk.lowestLevel,
  );
  const chosenS = secondsFor(((levels[chosen]?.averageBitrate ?? 0) * durationS) / 8, bps);
  return chosenS < leftS || bps === 0 ? chosen : undefined;
}

/**
 * The quality the next chunk is of, as it plays (D265 §4): the highest no higher than ceiling
 * within linkShare of the link as measured; the ceiling while nothing is measured.
 */
export function steadyLevel(choice: {
  readonly levels: readonly ChoiceLevel[];
  readonly ceiling: number;
  readonly linkBps: number | undefined;
  readonly bufferAheadS: number;
}): number {
  const { levels, ceiling, linkBps } = choice;
  return linkBps === undefined
    ? ceiling
    : highestWithin(levels, ceiling, linkShare(choice.bufferAheadS) * linkBps);
}

/**
 * The highest quality the film may play while voice is live: within half the link as measured,
 * never the pace its paced chunks arrive at, or the lowest when none is; any while nothing is
 * measured (D262 §2, D265 §6).
 */
export function voiceCeiling(levels: readonly ChoiceLevel[], linkBps: number | undefined): number {
  return linkBps === undefined
    ? levels.length - 1
    : highestWithin(levels, levels.length - 1, voiceLinkShare * linkBps);
}

/** How a chunk loaded, as a measurement of the link reads it. */
export interface LoadedChunk {
  /** From asking to its last byte, and from its first byte to its last, in ms. */
  readonly loadMs: number;
  readonly arrivingMs: number;
  readonly bytes: number;
  /** The pace it was asked to arrive at, bits a second; undefined when it was not paced. */
  readonly paceBps: number | undefined;
  /** Whether the browser says it came from its cache; undefined where it does not say (cache-hit.ts). */
  readonly isFromCache: boolean | undefined;
}

/**
 * Whether a loaded chunk measures the link (D265 §7): not one from the browser's cache, as the film
 * page's chunks are when Play reads them again, which the browser says, or, where it does not, one
 * that loaded in under cacheHitUnderMs; and a
 * paced one only when it arrived at no more than pacedCountsAtMostShare of its pace, the link, not
 * the pace, holding it back.
 */
export function measuresTheLink(chunk: LoadedChunk): boolean {
  if (chunk.isFromCache ?? chunk.loadMs < cacheHitUnderMs) {
    return false;
  }
  if (chunk.paceBps === undefined) {
    return true;
  }
  const arrivedBps = (chunk.bytes * 8 * 1000) / Math.max(1, chunk.arrivingMs);
  return arrivedBps <= pacedCountsAtMostShare * chunk.paceBps;
}

/** How long a playing video may stand still, its chunks arriving, before its quality is given up as unplayable (D266). */
export const standsStillAtMostMs = 6000;

/** How many chunks of its quality arrive while it stands still that show the link is not why (D266). */
export const chunksWhileStill = 2;

/**
 * How many chunks of picture, appended while the video stands still, may add nothing to what it
 * holds before their quality is given up at once (D266).
 */
export const chunksNotHeld = 2;

/** A video asked to play whose position has not moved, and what arrived meanwhile. */
export interface StandingStill {
  /** How long its position has stood still while it was asked to play, in ms. */
  readonly forMs: number;
  /** How many chunks of picture loaded meanwhile. */
  readonly chunksLoaded: number;
  /** How many of them the browser took and held nothing of (hls.js's bufferAppendNoProgress). */
  readonly chunksNotHeld: number;
}

/**
 * Whether the quality a video stands still at is one this device cannot play (D266): asked to
 * play, its position stood still for standsStillAtMostMs while chunksWhileStill chunks arrived, so
 * the link carries it and the decoder does not. Firefox on macOS took Flow's own picture, 10-bit
 * HEVC whose every keyframe after the first is a CRA, and froze at any start but the film's
 * beginning while its chunks kept arriving (the owner's test, 2026-10-07). Sooner, when the
 * browser holds nothing of chunksNotHeld chunks it took: Firefox on macOS took each of Flow's HEVC
 * chunks after a CRA and held none of it, hls.js reporting "append did not increase buffered
 * coverage" and doing nothing about it (the owner's test, 2026-10-08).
 */
export function isUnplayable(still: StandingStill): boolean {
  return (
    still.chunksNotHeld >= chunksNotHeld ||
    (still.forMs >= standsStillAtMostMs && still.chunksLoaded >= chunksWhileStill)
  );
}

/**
 * Whether what the video plays where it stands, its picture and its sound, is loaded there: only then
 * does its standing still say it cannot play what it has (D266). After a seek the sound's chunk can
 * come seconds after the picture's, made on request, and a video that waited six seconds for its
 * sound, its picture's chunks arriving meanwhile, gave the film's own picture up as unplayable and
 * moved to a quality made for it (the owner's solo jumps, 2026-10-12).
 */
export function isLoadedWhereItStands(
  media: Pick<HTMLMediaElement, "buffered" | "currentTime">,
): boolean {
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index += 1) {
    if (buffered.start(index) <= currentTime && currentTime < buffered.end(index)) {
      return true;
    }
  }
  return false;
}

/**
 * Whether a video is waiting to play at its position (D266): playing, or paused short of the data
 * to play on. An autoplaying video stays paused until it can play, so one whose decoder never
 * starts reads as paused forever; one a person paused, its data there, is not waiting.
 */
export function isWaitingToPlay(media: Pick<HTMLMediaElement, "paused" | "readyState">): boolean {
  return !media.paused || media.readyState < haveFutureData;
}
