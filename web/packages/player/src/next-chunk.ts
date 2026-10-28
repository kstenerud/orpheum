// SPDX-License-Identifier: MPL-2.0

/**
 * How long a chunked playback's next chunk is expected to take to load, which a player waits to
 * outlast before it starts or plays on after a seek (the SDK's ready-ahead.ts; the owner,
 * 2026-10-28): the chunk after what the video loaded where it is, on the quality hls.js loads, its
 * size from its byte range where the playlist gives one, else its film's length at its quality's
 * average rate, over the link as the player measures it, or, before it has, as the first chunk of
 * picture loaded.
 */

import {
  chunkLoadedOf,
  loadedAheadMsOf,
  playerLoadedChunkEvent,
  tellNextChunkLoadMs,
} from "@orpheum/sdk";

/** One chunk in a quality's playlist, as hls.js lists it: where it starts, how long, and its bytes. */
interface ListedChunk {
  readonly start: number;
  readonly duration: number;
  readonly byteRange: readonly number[];
}

/** The part of hls.js the next chunk is read from: the quality it loads and each quality's chunks. */
export interface ChunkListing {
  readonly loadLevel: number;
  readonly levels: readonly {
    readonly averageBitrate: number;
    readonly details?: { readonly fragments: readonly ListedChunk[] } | undefined;
  }[];
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
  const level = listing.levels[listing.loadLevel];
  const loadedEndS = video.currentTime + loadedAheadMsOf(video) / 1000 + pastTheEndS;
  const next = level?.details?.fragments.find((chunk) => chunk.start + chunk.duration > loadedEndS);
  if (level === undefined || next === undefined) {
    return undefined;
  }
  const [from, to] = next.byteRange;
  return from !== undefined && to !== undefined && to > from
    ? (to - from) * 8
    : next.duration * level.averageBitrate;
}

/**
 * Tells the SDK, for video, how long its next chunk is expected to take to load (tellNextChunkLoadMs):
 * its bits over linkKbps, or, while that says nothing, over the rate the first chunk of picture
 * loaded at. Returns the end of it.
 */
export function tellNextChunkLoad(
  listing: ChunkListing,
  video: Pick<
    HTMLMediaElement,
    "buffered" | "currentTime" | "addEventListener" | "removeEventListener"
  >,
  linkKbps: () => number | undefined,
): () => void {
  let firstChunkKbps: number | undefined;
  const heardChunk = (event: Event): void => {
    const loaded = chunkLoadedOf(event);
    if (loaded?.track === "video" && loaded.loadedKbps > 0) {
      firstChunkKbps ??= loaded.loadedKbps;
    }
  };
  video.addEventListener(playerLoadedChunkEvent, heardChunk);
  const stopTelling = tellNextChunkLoadMs(video, () => {
    const kbps = linkKbps() ?? firstChunkKbps;
    const bits = nextChunkBits(listing, video);
    // A kilobit a second is a bit a millisecond.
    return kbps === undefined || kbps <= 0 || bits === undefined ? undefined : bits / kbps;
  });
  return () => {
    video.removeEventListener(playerLoadedChunkEvent, heardChunk);
    stopTelling();
  };
}
