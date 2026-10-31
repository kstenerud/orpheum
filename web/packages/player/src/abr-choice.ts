// SPDX-License-Identifier: MPL-2.0

/**
 * The choices of the chunked player's bitrate controller, each a function of what it is told
 * (D265): where it starts, when it gives up a chunk on its way, what it loads next, what voice
 * leaves the film, and what counts as a measurement of the link. abr.ts asks them.
 */

import type { LinkSample } from "./link-estimate.ts";
import { haveFutureData } from "./stalls.ts";

/** A quality as the controller reads it: its peak bit rate, BANDWIDTH, and its average, the sound's included. */
export interface ChoiceLevel {
  readonly bitrate: number;
  readonly averageBitrate: number;
  /** Its playlist's address, which names the film's own picture ("picture/stream.m3u8"). */
  readonly uri?: string;
  /** Its picture's height, in pixels; 0 or absent where the playlist does not say. */
  readonly height?: number;
}

/**
 * The share of a link's measured speed a quality may take with little loaded ahead, and when a
 * chunk is given up, where the playback's offer does not say: the core chooses a quality by the same
 * share, link_share_for_quality_percent, and its offer carries it (D247 §6). This is that setting's
 * starting value, which a test of the core's settings holds equal.
 */
export const linkShareForQuality = 0.7;

/** The share of the link a quality may take with plenty loaded ahead (D265 §4). */
export const linkShareWhenAhead = 0.95;

/** How far ahead, in seconds, the share starts rising from linkShareForQuality, and reaches linkShareWhenAhead. */
export const shareRisesFromS = 10;
export const shareRisesToS = 25;

/**
 * How recently, in ms, bytes of a chunk given up must have arrived for it to have been arriving, its
 * bytes a measure of the link, rather than stuck (bandwidth-sampling-2026-10-29.md P3): the stuck
 * chunks of a Fire TV's film delivered a few kilobytes and then nothing (staging, 2026-10-28). A
 * starting value, a second, twice the controller's look; to be measured.
 */
export const arrivingWithinMs = 1000;

/**
 * How long nothing of a chunk on its way, answered, may arrive before it is asked for again, the
 * first time, a justified constant: far longer than a live link leaves between its 64 KiB messages
 * at the lowest quality's rate, which a chunk asked again would lose; each time again in a row waits
 * twice as long, up to asksAgainAfterAtMostMs, so a link that comes back plays on within that. Waited for, a chunk
 * stuck after 19 bytes over WebRTC held its player for good, hls.js's own limit two minutes away and
 * no quality given up for it (the owner's Chrome, throttled and unthrottled, staging, 2026-10-29).
 */
export const asksAgainAfterMs = 8000;
export const asksAgainAfterAtMostMs = 16_000;

/**
 * Whether a chunk on its way is to be asked for again, the asksAgain-th time in a row since a chunk
 * last loaded (from 0): answered, its head come, with nothing of it arriving for asksAgainAfterMs,
 * doubled for each time before, up to asksAgainAfterAtMostMs. One not yet answered is hls.js's to
 * time out, since the core may hold it while it is made (chunk_hold_ms).
 */
export function isToAskAgain(
  chunk: { readonly isAnswered: boolean; readonly sinceArrivedMs: number },
  asksAgain: number,
): boolean {
  return chunk.isAnswered && chunk.sinceArrivedMs >= asksAgainWaitMs(asksAgain);
}

/** How long nothing of a chunk may arrive before it is asked for again the asksAgain-th time in a row. */
export function asksAgainWaitMs(asksAgain: number): number {
  return Math.min(asksAgainAfterMs * 2 ** asksAgain, asksAgainAfterAtMostMs);
}

/**
 * How long the playback must have played before it stops starting, and how many chunks that
 * measured the link end it at the latest (D352 §1): by 20 s played the player has filled its first
 * buffer, a dozen chunks or more, so a bad first sample has left the estimate's five. Evidence ends
 * it sooner: samples that settle the ceiling, the same with any one left out (#startingSettles,
 * bandwidth-sampling-2026-10-29.md P5), which a lone sample never does. Four end it however they
 * disagree: a playback still starting never steps up.
 */
export const startingSamples = 4;
export const startingPlayedMs = 20_000;

/**
 * How many chunks must have measured the link before it is reported, and so remembered for the
 * device's next start (D257 §1): never a lone first sample (D352 §1).
 */
export const reportsLinkFromSamples = 2;

/**
 * How far above its quality's rate, its BANDWIDTH, a chunk's download is paced while voice is
 * live: enough that a busy chunk still arrives well before it plays (D262 §2).
 */
export const paceAboveQuality = 1.25;

/** The share of the link the film may take while voice is live (D262 §2). */
export const voiceLinkShare = 0.5;

/**
 * A load quicker than this came from the browser's cache, not over the link (D265 §7): the guess
 * where the browser does not say (LoadedChunk.isFromCache).
 */
export const cacheHitUnderMs = 10;

/**
 * The fewest bytes a chunk measures the link with, counted after its first piece (D382 §2): a
 * smaller one's time is mostly its request's wait, not its bytes arriving. A film opens on black, a first chunk of 44.8 KB, which
 * measured a LAN as 676 kbit/s and started the film at its lowest quality (the owner's test on
 * staging, 2026-10-18). The lowest quality's 10 s chunk is about 500 KB.
 */
export const measuresFromBytesAtLeast = 128 * 1024;

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

/**
 * The share of the link a quality may take with bufferAheadS loaded ahead (D265 §4): rising from
 * forQuality, the share with little loaded ahead, to linkShareWhenAhead.
 */
export function linkShare(bufferAheadS: number, forQuality = linkShareForQuality): number {
  const along = (bufferAheadS - shareRisesFromS) / (shareRisesToS - shareRisesFromS);
  const clamped = Math.min(1, Math.max(0, along));
  return forQuality + clamped * (Math.max(forQuality, linkShareWhenAhead) - forQuality);
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
  /** The share of a link a quality may take, as the offer says; linkShareForQuality when absent. */
  readonly shareForQuality?: number | undefined;
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

/** The share of the link a quality may take, as the offer said it to the chunk's player. */
function shareFor(chunk: ChunkOnItsWay): number {
  return chunk.shareForQuality ?? linkShareForQuality;
}

/**
 * The quality a chunk on its way is given up for, or undefined to wait for it (D265 §3): once it
 * has loaded for half the time it plays, a chunk arriving slower than its quality plays, the rest
 * of which would take longer than what is loaded ahead lasts, is given up for the highest quality
 * within the share for quality of the speed it arrives at, the lowest included, when that quality's
 * whole chunk would arrive sooner. It is the one giving up: hls.js's own, which gave up only while
 * the video played and never for the lowest, left a film's page filling its hidden player, and a
 * player waiting for its first frame, waiting out a whole chunk of the film's own picture (staging,
 * 2026-10-07), and took a chunk the core was making for a stalled one, so it is off (abr.ts).
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
    highestWithin(levels, loadingLevel - 1, shareFor(chunk) * bps),
    chunk.lowestLevel,
  );
  const chosenS = secondsFor(((levels[chosen]?.averageBitrate ?? 0) * durationS) / 8, bps);
  return chosenS < leftS || bps === 0 ? chosen : undefined;
}

/**
 * A chunk's first arriving piece, its load's first progress: when it came, in ms on its stats'
 * clock (performance.now's), and how many bytes it brought.
 */
export interface FirstPiece {
  readonly atMs: number;
  readonly bytes: number;
}

/** What of a chunk measures the link: its bytes after its first piece, over the ms after it. */
export interface AfterFirstPiece {
  readonly bytes: number;
  readonly ms: number;
}

/**
 * What of a chunk's arrival measures the link (D382 §2): its bytes after its first piece, over the
 * time from it to toMs, its last byte's or the give-up's. A transport that delivers in pieces, the
 * WebRTC path's 128 KiB, brings the first with the first byte, so a chunk given up with only that
 * measured a 20 Mb/s link at 680 kb/s (the Fire TV, staging, 2026-10-30). Where its first piece is
 * not known, or is an earlier load's, before fromMs, its bytes since its first byte, fromMs.
 */
export function afterFirstPiece(
  arrived: { readonly bytes: number; readonly fromMs: number; readonly toMs: number },
  piece: FirstPiece | undefined,
): AfterFirstPiece {
  if (piece === undefined || piece.atMs < arrived.fromMs) {
    return { bytes: arrived.bytes, ms: Math.max(1, arrived.toMs - arrived.fromMs) };
  }
  return {
    bytes: Math.max(0, arrived.bytes - piece.bytes),
    ms: Math.max(1, arrived.toMs - piece.atMs),
  };
}

/** A chunk given up on its way, as what it says of the link reads it. */
export interface GivenUpChunk {
  readonly level: number;
  /** How long it had been loading, and when its first byte came, both in ms from when it began. */
  readonly loadingMs: number;
  readonly firstByteMs: number | undefined;
  /** How many of its bytes had arrived, none of them the core's answers that it is being made. */
  readonly arrivedBytes: number;
  /** How long since its last bytes arrived, in ms. */
  readonly sinceArrivedMs: number;
  /** What of it measures the link (afterFirstPiece); its bytes since its first byte where absent. */
  readonly afterFirstPiece?: AfterFirstPiece | undefined;
}

/**
 * What a chunk given up says of the link's speed (bandwidth-sampling-2026-10-29.md P3): one that was
 * arriving, its bytes coming within arrivingWithinMs of the give-up and at least
 * measuresFromBytesAtLeast of them after its first piece, is a sample of those bytes over the time
 * from its first piece to the give-up (D382 §2), as arrivalMs times a whole chunk; one stuck, nothing
 * arriving or only its first piece, says nothing of it: undefined. It is a
 * censored measurement, "slower than this, or the transport stopped", which belongs to the link's
 * reliability, a drop (D352 §4), never to its speed. Counted as one byte over the whole wait, one
 * such chunk among four at 14–18 Mb/s made the estimate 69 kb/s, and a Fire TV played a film at its
 * lowest quality for 75 s (staging, 2026-10-28). hls.js samples only the wait of a fragment it
 * abandons with nothing loaded.
 */
export function givenUpSample(chunk: GivenUpChunk): LinkSample | undefined {
  const { firstByteMs } = chunk;
  if (firstByteMs === undefined) {
    return undefined;
  }
  const measured = chunk.afterFirstPiece ?? {
    bytes: chunk.arrivedBytes,
    ms: Math.max(1, chunk.loadingMs - firstByteMs),
  };
  if (measured.bytes < measuresFromBytesAtLeast || chunk.sinceArrivedMs > arrivingWithinMs) {
    return undefined;
  }
  return { level: chunk.level, bytes: measured.bytes, arrivedMs: measured.ms, firstByteMs };
}

/**
 * Whether a chunk on its way is stuck, worth a note though it is not given up: loading for half the
 * time it plays, as one is before it may be given up, with nothing arriving for arrivingWithinMs.
 */
export function isStuck(chunk: {
  readonly chunk: { readonly duration: number };
  readonly loadingMs: number;
  readonly sinceArrivedMs: number;
}): boolean {
  return (
    chunk.loadingMs >= (chunk.chunk.duration * 1000) / 2 && chunk.sinceArrivedMs > arrivingWithinMs
  );
}

/**
 * How a chunk given up came, for its note: "750000 B in 6000 ms", a sample; "stuck: 5000 B in 6000 ms,
 * none for 4500 ms", nothing arriving within arrivingWithinMs; "stuck: 131072 B in 2028 ms, only its
 * first piece", nothing after it (D382 §2); or "100000 B in 6000 ms, too few to measure".
 */
export function givenUpText(chunk: {
  readonly loadedBytes: number;
  readonly loadingMs: number;
  readonly sinceArrivedMs: number;
  readonly isSample: boolean;
  readonly afterFirstPiece?: AfterFirstPiece | undefined;
}): string {
  const came = `${String(chunk.loadedBytes)} B in ${String(chunk.loadingMs)} ms`;
  if (chunk.isSample) {
    return came;
  }
  if (chunk.sinceArrivedMs > arrivingWithinMs) {
    return `stuck: ${came}, none for ${String(chunk.sinceArrivedMs)} ms`;
  }
  return chunk.afterFirstPiece?.bytes === 0 && chunk.loadedBytes > 0
    ? `stuck: ${came}, only its first piece`
    : `${came}, too few to measure`;
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
  /** The share of a link a quality may take, as the offer says; linkShareForQuality when absent. */
  readonly shareForQuality?: number | undefined;
}): number {
  const { levels, ceiling, linkBps } = choice;
  return linkBps === undefined
    ? ceiling
    : highestWithin(
        levels,
        ceiling,
        linkShare(choice.bufferAheadS, choice.shareForQuality) * linkBps,
      );
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

/**
 * How long, in ms, a loaded chunk took to arrive: from its answer's first byte, its head, to its
 * last; at least 1. This, not the time from asking, is what its bytes measure the link over: the
 * wait for the first byte is the request's, queued behind others on a WebRTC connection, opening a
 * channel, the core finding or making the chunk, and none of it the link carrying bytes (D247 §9:
 * the speed measured while chunks download). hls.js's own sample runs from the ask to the chunk
 * parsed, less its estimate of that wait, which weights a long wait as an outlier and leaves most
 * of it in. A film's first chunk of its own picture, made and on disk, measured a path that
 * had carried 12.5 Mb/s at 1008 kb/s, its page having opened two more WebRTC connections in the
 * seconds before, as it does only while its requests crowd the ones it has; that one sample took
 * the player to the lowest quality (the owner, on staging, 2026-10-26).
 */
export function arrivalMs(loading: { readonly first: number; readonly end: number }): number {
  return Math.max(1, loading.end - loading.first);
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
 * that loaded in under cacheHitUnderMs; not one under measuresFromBytesAtLeast, whose time is its
 * request's wait; and a paced one only when it arrived at no more than pacedCountsAtMostShare of its pace, the link, not
 * the pace, holding it back.
 */
export function measuresTheLink(chunk: LoadedChunk): boolean {
  if (chunk.isFromCache ?? chunk.loadMs < cacheHitUnderMs) {
    return false;
  }
  if (chunk.bytes < measuresFromBytesAtLeast) {
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
  /** How far ahead of its position it holds the film, in seconds. */
  readonly loadedAheadS: number;
}

/**
 * How far ahead a video standing still must hold the film for its standing still to be its decoder's,
 * not its link's, with no chunk arriving: its buffer full, nothing more is asked for (D266).
 */
export const heldAheadAtLeastS = 4;

/**
 * Whether the quality a video stands still at is one this device cannot play (D266): asked to
 * play, its position stood still for standsStillAtMostMs while chunksWhileStill chunks arrived, so
 * the link carries it and the decoder does not. Firefox on macOS took Flow's own picture, 10-bit
 * HEVC whose every keyframe after the first is a CRA, and froze at any start but the film's
 * beginning while its chunks kept arriving (the owner's test, 2026-10-07). Sooner, when the
 * browser holds nothing of chunksNotHeld chunks it took: Firefox on macOS took each of Flow's HEVC
 * chunks after a CRA and held none of it, hls.js reporting "append did not increase buffered
 * coverage" and doing nothing about it (the owner's test, 2026-10-08). And with no chunk arriving,
 * when it holds heldAheadAtLeastS ahead and still stands still: an older Android phone's Chrome took a
 * film's own 10-bit AV1 and HEVC pictures, filled its buffer and stood at 0 "playing", its decoder
 * unable to keep up (the owner's friend, 2026-10-12).
 */
export function isUnplayable(still: StandingStill): boolean {
  return (
    still.chunksNotHeld >= chunksNotHeld ||
    (still.forMs >= standsStillAtMostMs &&
      (still.chunksLoaded >= chunksWhileStill || still.loadedAheadS >= heldAheadAtLeastS))
  );
}

/**
 * How far ahead of where it stands a video must hold what it plays, its picture and its sound both,
 * for its standing still to say it cannot play what it has, in seconds (D266): with less, it waits for
 * loads, the link's to bring. A minute's outage left Chrome standing 0.07 s short of the end of what
 * it held, and its quality was given up as unplayable (a local reproduction, 2026-10-29).
 */
export const standsWithAheadAtLeastS = 1;

/**
 * How far ahead of where it stands the video holds what it plays there, its picture and its sound
 * both, in seconds (the element's own buffered ranges, which hold only what both have); 0 when
 * nothing is loaded where it stands. After a seek the sound's chunk can come seconds after the
 * picture's, made on request, and a video that waited six seconds for its sound, its picture's chunks
 * arriving meanwhile, gave the film's own picture up as unplayable and moved to a quality made for it
 * (the owner's solo jumps, 2026-10-12). Not hls.js's mainForwardBufferInfo, its picture's alone: with
 * its loading stopped by an outage it said 33.9 s while the video held 0.07 s, and the quality was
 * given up as one the decoder could not keep up with (a local reproduction, 2026-10-29).
 */
export function loadedAheadOf(media: Pick<HTMLMediaElement, "buffered" | "currentTime">): number {
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index += 1) {
    if (buffered.start(index) <= currentTime && currentTime < buffered.end(index)) {
      return buffered.end(index) - currentTime;
    }
  }
  return 0;
}

/**
 * Whether a video is waiting to play at its position (D266): playing, or paused short of the data
 * to play on. An autoplaying video stays paused until it can play, so one whose decoder never
 * starts reads as paused forever; one a person paused, its data there, is not waiting.
 */
export function isWaitingToPlay(media: Pick<HTMLMediaElement, "paused" | "readyState">): boolean {
  return !media.paused || media.readyState < haveFutureData;
}

/** The quality that is the film's own picture, copied rather than made (its playlist picture/), or -1. */
export function ownPictureLevel(levels: readonly ChoiceLevel[]): number {
  return levels.findIndex(
    (level) =>
      level.uri === "picture/stream.m3u8" || level.uri?.endsWith("/picture/stream.m3u8") === true,
  );
}

/**
 * The film's own picture, when a player in a Watch Together group keeps it: while its link carries
 * the picture's average rate, whatever share the controller keeps back for voice or a growing
 * buffer. Such a player stepped down to a quality the server made for it, adding an encode the
 * group's encoding devices could not spare, on a link only crowded by the group sharing the server's
 * upload (D286 §2); undefined when it does not keep it.
 */
export function keptOwnPicture(
  levels: readonly ChoiceLevel[],
  linkBps: number | undefined,
): number | undefined {
  const own = ownPictureLevel(levels);
  const picture = levels[own];
  return picture !== undefined && linkBps !== undefined && linkBps >= picture.averageBitrate
    ? own
    : undefined;
}

/** The height of the quality a group's encoded devices start on after a jump (D286 §3). */
export const commonStartHeight = 720;

/** How long after a jump a group's encoded device keeps the common quality before it settles. */
export const commonStartForMs = 30_000;

/**
 * The quality every device of a group that needs an encoded one starts on after a jump: the highest
 * the server makes at or under commonStartHeight, one encode for all of them where each had wanted
 * its own (D286 §3); -1 where there is none.
 */
export function commonStartLevel(levels: readonly ChoiceLevel[]): number {
  const own = ownPictureLevel(levels);
  return levels.findLastIndex(
    (level, index) =>
      index !== own && (level.height ?? 0) > 0 && (level.height ?? 0) <= commonStartHeight,
  );
}

/**
 * The highest quality whose average rate, its sound's with it, is at most ceilingKbps, the lowest
 * when none is; the highest of all with no ceiling (D286 §5).
 */
export function underCeiling(
  levels: readonly ChoiceLevel[],
  ceilingKbps: number | undefined,
): number {
  if (ceilingKbps === undefined) {
    return levels.length - 1;
  }
  let highest = 0;
  levels.forEach((level, index) => {
    if (level.averageBitrate <= ceilingKbps * 1000) {
      highest = Math.max(highest, index);
    }
  });
  return highest;
}
