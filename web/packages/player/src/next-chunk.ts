// SPDX-License-Identifier: MPL-2.0

/**
 * How long a chunked playback's next chunk is expected to take to load, which a player waits to
 * outlast before it starts or plays on after a seek (the SDK's ready-ahead.ts; the owner,
 * 2026-10-28): the chunk after what the video loaded where it is, on the quality hls.js loads, its
 * size from its byte range where the playlist gives one, else its film's length at its quality's
 * average rate, over the next chunk's bound of its controller's estimate of the link, plus the wait
 * its first byte is expected to take (bandwidth-sampling-2026-10-29.md P5). The bound is a lower
 * quantile set by what is loaded ahead, from the link its device remembers before any chunk and
 * blending in each chunk after, so the source moves from remembered to measured gradually, not at
 * a count: two samples on a fresh connection outvoted a link measured minutes before and started a
 * Fire TV on 753 kb/s, where a lone first chunk at 28 Mb/s on a 9.6 Mb/s link had started one with
 * 1 s loaded (staging, 2026-10-28). With nothing remembered and nothing measured it says nothing,
 * and the player waits refillAheadMs.
 */

import { loadedAheadMsOf, tellNextChunkLoadMs, type NextChunkLoad } from "@orpheum/sdk";

/** One chunk in a quality's playlist, as hls.js lists it: where it starts, how long, and its bytes. */
interface ListedChunk {
  readonly start: number;
  readonly duration: number;
  readonly byteRange: readonly number[];
}

/**
 * The part of hls.js the next chunk is read from: the quality it loads, the lowest it may, each
 * quality's chunks, and how far ahead it loads (keepForwardBuffer sets both lengths).
 */
export interface ChunkListing {
  readonly loadLevel: number;
  readonly minAutoLevel: number;
  readonly levels: readonly {
    readonly averageBitrate: number;
    readonly details?: { readonly fragments: readonly ListedChunk[] } | undefined;
  }[];
  readonly config: { readonly maxBufferLength: number; readonly maxMaxBufferLength: number };
}

/** A hair past the end of what is loaded, in seconds, so the chunk ending there is not taken for the next. */
const pastTheEndS = 0.001;

/**
 * The bits of the chunk after what video loaded where it is, on the quality hls.js loads: its byte
 * range's, else its length at the quality's average rate; undefined where that quality's playlist
 * is not loaded yet, or nothing is left to load.
 */
export function nextChunkBits(
  listing: ChunkListing,
  video: Pick<HTMLMediaElement, "buffered" | "currentTime">,
): number | undefined {
  return nextChunkOf(listing, video)?.bits;
}

/** The chunk after what video loaded where it is, on the quality hls.js loads: its bits and its film. */
function nextChunkOf(
  listing: ChunkListing,
  video: Pick<HTMLMediaElement, "buffered" | "currentTime">,
): { readonly bits: number; readonly filmMs: number } | undefined {
  const level = listing.levels[listing.loadLevel];
  const loadedEndS = video.currentTime + loadedAheadMsOf(video) / 1000 + pastTheEndS;
  const next = level?.details?.fragments.find((chunk) => chunk.start + chunk.duration > loadedEndS);
  if (level === undefined || next === undefined) {
    return undefined;
  }
  const [from, to] = next.byteRange;
  const bits =
    from !== undefined && to !== undefined && to > from
      ? (to - from) * 8
      : next.duration * level.averageBitrate;
  return { bits, filmMs: next.duration * 1000 };
}

/**
 * The link a next chunk's load is reckoned over, in kbit/s, the wait its first byte is expected to
 * take, in ms, and what it rests on: the playback's own samples, the prior among them while it
 * counts ("confident"), or the remembered link alone ("remembered").
 */
export interface NextChunkLink {
  readonly kbps: number;
  readonly firstByteMs: number;
  readonly from: Extract<NextChunkLoad["from"], "confident" | "remembered">;
}

/**
 * Tells the SDK, for video, how long its next chunk is expected to take to load and what that rests
 * on (tellNextChunkLoadMs): the wait for its first byte plus its bits over the link nextChunkLink
 * names, its controller's next-chunk bound (Choosing.nextChunkLink); with its bits, which a heavy
 * chunk is waited for by too (heavyChunkLoadKbps), the film it plays, whether
 * it is of the lowest quality hls.js may load, and the most it loads ahead, which a refill after a
 * stall reads (refillAfterStallOf, D414 §2). Returns the end of it.
 */
export function tellNextChunkLoad(
  listing: ChunkListing,
  video: Pick<HTMLMediaElement, "buffered" | "currentTime">,
  nextChunkLink: () => NextChunkLink | undefined,
): () => void {
  return tellNextChunkLoadMs(video, () => {
    const over = nextChunkLink();
    const next = nextChunkOf(listing, video);
    const { maxBufferLength, maxMaxBufferLength } = listing.config;
    // A kilobit a second is a bit a millisecond.
    return over === undefined || next === undefined || over.kbps <= 0
      ? undefined
      : {
          loadMs: over.firstByteMs + next.bits / over.kbps,
          from: over.from,
          filmMs: next.filmMs,
          bits: next.bits,
          isLowestQuality: listing.loadLevel <= listing.minAutoLevel,
          loadsAheadAtMostMs: Math.min(maxBufferLength, maxMaxBufferLength) * 1000,
        };
  });
}
