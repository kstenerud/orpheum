// SPDX-License-Identifier: MPL-2.0

/**
 * How long a chunked playback's next chunk is expected to take to load, which a player waits to
 * outlast before it starts or plays on after a seek (the SDK's ready-ahead.ts; the owner,
 * 2026-10-28): the chunk after what the video loaded where it is, on the quality hls.js loads, its
 * size from its byte range where the playlist gives one, else its film's length at its quality's
 * average rate, over the link as the player measures it once its estimate is confident, or, before
 * it is, over the lower of the link its device last measured on its network and the speed its
 * first chunk of picture loaded at; never over that first chunk alone (D352 §1). A Fire TV's first
 * chunk loaded at 28 Mb/s, its next took 2.6 s at 9.6, and it started with 1 s loaded and ran dry
 * (staging, 2026-10-28).
 */

import {
  chunkLoadedOf,
  loadedAheadMsOf,
  playerLoadedChunkEvent,
  tellNextChunkLoadMs,
  type NextChunkLoad,
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

/** The link a next chunk's load is reckoned over, in kbit/s, and what it rests on. */
export interface NextChunkLink {
  readonly kbps: number;
  readonly from: NextChunkLoad["from"];
}

/**
 * The link to reckon the next chunk's load over: the player's confident estimate where it has one;
 * else the lower of the link its device last measured on its network and its first chunk of
 * picture's speed, or the remembered link before that chunk; undefined with nothing remembered, so
 * the player waits refillAheadMs rather than trust a lone first chunk (D352 §1).
 */
export function nextChunkLinkOf(known: {
  readonly confidentKbps: number | undefined;
  readonly rememberedKbps: number | undefined;
  readonly firstChunkKbps: number | undefined;
}): NextChunkLink | undefined {
  const { confidentKbps, rememberedKbps, firstChunkKbps } = known;
  if (confidentKbps !== undefined && confidentKbps > 0) {
    return { kbps: confidentKbps, from: "confident" };
  }
  if (rememberedKbps === undefined || rememberedKbps <= 0) {
    return undefined;
  }
  return firstChunkKbps !== undefined && firstChunkKbps > 0 && firstChunkKbps < rememberedKbps
    ? { kbps: firstChunkKbps, from: "firstSample" }
    : { kbps: rememberedKbps, from: "remembered" };
}

/**
 * Tells the SDK, for video, how long its next chunk is expected to take to load and what that rests
 * on (tellNextChunkLoadMs): its bits over the link nextChunkLinkOf names, from the link as known, its
 * player's confident estimate and the link the playback's offer says its device last measured on
 * its network, and the first chunk of picture's speed. Returns the end of it.
 */
export function tellNextChunkLoad(
  listing: ChunkListing,
  video: Pick<
    HTMLMediaElement,
    "buffered" | "currentTime" | "addEventListener" | "removeEventListener"
  >,
  link: {
    readonly confidentKbps: () => number | undefined;
    readonly rememberedLinkKbps?: number | undefined;
  },
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
    const over = nextChunkLinkOf({
      confidentKbps: link.confidentKbps(),
      rememberedKbps: link.rememberedLinkKbps,
      firstChunkKbps,
    });
    const bits = nextChunkBits(listing, video);
    // A kilobit a second is a bit a millisecond.
    return over === undefined || bits === undefined
      ? undefined
      : { loadMs: bits / over.kbps, from: over.from };
  });
  return () => {
    video.removeEventListener(playerLoadedChunkEvent, heardChunk);
    stopTelling();
  };
}
