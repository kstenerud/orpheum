// SPDX-License-Identifier: MPL-2.0

/**
 * The room a chunked playback's picture buffer has for the next chunk (D464): each chunk sized at
 * each quality by its own bytes, the chunks the buffer holds, and the buffer's limit, the device's
 * table's or, after the browser first refused a chunk for its memory, what it was seen to hold,
 * remembered in the browser's storage for its user agent. From them, the quality the next chunk
 * fits at beside the chunk before it (pairLevel), and how far ahead to load so it fits beside what
 * is held (roomAheadS).
 */

import { ownPictureLevel, type ChoiceLevel } from "./abr-choice.ts";
import type { ChoiceWhy } from "./abr-options.ts";

/**
 * The share of the picture buffer the chunks held and the next may take together: the rest is the
 * browser's own room. 23 MB chunks beside small ones played in Silk's 31.5 MB on a 1 GB Fire TV
 * (VQ294's run, 2026-11-11). A justified constant, the owner's (D464 §3).
 */
export const roomShareOfBufferLimit = 0.9;

/**
 * A made quality's encoder buffer, in seconds of its average rate: the node encodes at -maxrate 1.5×
 * and -bufsize 2× the quality's rate (internal/node/encode/encoder.go), so a chunk of it holds at
 * most its peak rate over its length and this much more (D464 §3). A justified constant.
 */
export const madeBufferS = 2;

/** How far past the buffer's end a chunk must run to be the next: hls.js's own look-up tolerance. */
const nextChunkToleranceS = 0.25;

/** A chunk of picture as the room keeps it: its place on the grid, its span, its bytes as loaded. */
export interface HeldChunk {
  readonly sn: number;
  readonly startS: number;
  readonly endS: number;
  readonly bytes: number;
}

/** A chunk as a quality's playlist lists it: its EXT-X-BITRATE, bits a second, where it says one. */
export interface GridChunk {
  readonly sn: number | "initSegment";
  readonly start: number;
  readonly duration: number;
  readonly bitrate: number | null;
}

/** A quality as the room reads it: its rates and, once loaded, its playlist's chunks. */
export interface RoomLevel extends ChoiceLevel {
  readonly details?: { readonly fragments: readonly GridChunk[] } | undefined;
}

/** The span of picture loaded around where it plays (hls.js's mainForwardBufferInfo). */
export interface HeldSpan {
  readonly start: number;
  readonly end: number;
}

/** What the room reads of hls.js. */
export interface RoomHls {
  readonly levels: readonly RoomLevel[];
  readonly minAutoLevel: number;
  readonly mainForwardBufferInfo: HeldSpan | null;
}

/** Where a picture buffer's limit learnt on a device is kept, for the device's next playback. */
export interface LimitMemory {
  readonly read: () => number | undefined;
  readonly write: (bytes: number) => void;
}

/** The browser's storage as LimitMemory reads it: getItem and setItem, either of which may throw. */
export type LimitStorage = Pick<Storage, "getItem" | "setItem">;

/** The storage key a learnt limit is kept under. */
const limitKey = "orpheum.pictureBufferLimit";

/**
 * A learnt limit kept in storage with userAgent, read back only by the same user agent: a browser
 * updated, or another on the same origin, learns its own. Storage refused or throwing keeps nothing.
 */
export function limitMemoryIn(storage: LimitStorage | undefined, userAgent: string): LimitMemory {
  return {
    read: () => {
      try {
        const kept: unknown = JSON.parse(storage?.getItem(limitKey) ?? "null");
        if (typeof kept !== "object" || kept === null) {
          return undefined;
        }
        const bytes: unknown = Reflect.get(kept, "bytes");
        const agent: unknown = Reflect.get(kept, "userAgent");
        return agent === userAgent && typeof bytes === "number" && bytes > 0 ? bytes : undefined;
      } catch {
        return undefined;
      }
    },
    write: (bytes) => {
      try {
        storage?.setItem(limitKey, JSON.stringify({ userAgent, bytes }));
      } catch {
        // Nothing kept: the next playback learns again.
      }
    },
  };
}

/** The browser's local storage, or undefined where it refuses it. */
export function browserStorage(): LimitStorage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

/** Bytes as megabytes, for a note: "28.3 MB". */
export function megabytes(bytes: number): string {
  return `${(bytes / 1_000_000).toFixed(1)} MB`;
}

/**
 * The picture buffer's room: its limit, and the chunks it holds. The limit is the device's table's
 * (assumedBytes), or one learnt and remembered, if lower; at the first refusal of a chunk for the
 * buffer's memory it becomes the most it was seen to hold at once, if lower still.
 */
export class PictureRoom {
  readonly #assumedBytes: number;
  readonly #memory: LimitMemory;
  readonly #note: (note: string) => void;
  #learntBytes: number | undefined;
  #hasLearnt = false;
  #mostHeldBytes = 0;
  readonly #held = new Map<number, HeldChunk>();

  constructor(options: {
    readonly assumedBytes: number;
    readonly memory: LimitMemory;
    readonly note: (note: string) => void;
  }) {
    this.#assumedBytes = options.assumedBytes;
    this.#memory = options.memory;
    this.#note = options.note;
    this.#learntBytes = options.memory.read();
  }

  /** The buffer's limit, in bytes. */
  limitBytes(): number {
    return Math.min(this.#assumedBytes, this.#learntBytes ?? Number.POSITIVE_INFINITY);
  }

  /** What the chunks held and the next may take together, in bytes. */
  roomBytes(): number {
    return this.limitBytes() * roomShareOfBufferLimit;
  }

  /**
   * A chunk of picture was buffered, span now loaded around where it plays: it is held, a chunk
   * that ended before the span is no longer, and the most held at once is counted.
   */
  buffered(chunk: HeldChunk, span: HeldSpan | null): void {
    this.#held.set(chunk.sn, chunk);
    if (span === null) {
      return;
    }
    let heldBytes = 0;
    for (const [sn, each] of this.#held) {
      if (each.endS <= span.start) {
        this.#held.delete(sn);
      } else if (each.startS < span.end) {
        heldBytes += each.bytes;
      }
    }
    this.#mostHeldBytes = Math.max(this.#mostHeldBytes, heldBytes);
  }

  /**
   * The browser refused a chunk of picture for its memory: the first time, the limit is the most it
   * was seen to hold, if lower than the limit, remembered for the device's next playback.
   */
  refused(): void {
    if (this.#hasLearnt) {
      return;
    }
    this.#hasLearnt = true;
    const limitBytes = this.limitBytes();
    if (this.#mostHeldBytes <= 0 || this.#mostHeldBytes >= limitBytes) {
      return;
    }
    this.#learntBytes = this.#mostHeldBytes;
    this.#memory.write(this.#mostHeldBytes);
    this.#note(
      `picture buffer refused: limit ${megabytes(limitBytes)} lowered to ${megabytes(this.#mostHeldBytes)}, the most held`,
    );
  }

  /** The chunks held just before chunk sn, still in span, nearest first. */
  heldBefore(sn: number, span: HeldSpan): readonly HeldChunk[] {
    const before: HeldChunk[] = [];
    for (let at = sn - 1; ; at -= 1) {
      const chunk = this.#held.get(at);
      if (chunk === undefined || chunk.endS <= span.start + nextChunkToleranceS) {
        return before;
      }
      before.push(chunk);
    }
  }
}

/** A picture room as a forward buffer reads it: its limit, and how far ahead it lets a level load. */
export interface BufferRoom {
  readonly limitBytes: () => number;
  /** How far ahead the next chunk, at level, fits beside what is held; undefined for no bound. */
  readonly aheadS: (level: number) => number | undefined;
}

/**
 * room as the forward buffer of hls reads it, the next chunk paced at the quality its controller
 * chose (chosenLevel) before any step-down, else the level the buffer says: a chunk plays at the
 * quality chosen where waiting lets it fit (D464 §4).
 */
export function bufferRoomOf(
  hls: RoomHls,
  room: PictureRoom,
  chosenLevel: () => number | undefined,
): BufferRoom {
  return {
    limitBytes: () => room.limitBytes(),
    aheadS: (level) => roomAheadS(hls, room, chosenLevel() ?? level),
  };
}

/** The chunk hls.js loads next: the first on the grid that runs past the picture loaded. */
export function nextChunkOf(hls: RoomHls): (GridChunk & { readonly sn: number }) | undefined {
  const span = hls.mainForwardBufferInfo;
  const fragments = hls.levels.find((level) => level.details !== undefined)?.details?.fragments;
  if (span === null || fragments === undefined) {
    return undefined;
  }
  const next = fragments.find(
    (chunk) => chunk.start + chunk.duration > span.end + nextChunkToleranceS,
  );
  return next === undefined || next.sn === "initSegment" ? undefined : { ...next, sn: next.sn };
}

/**
 * The bytes of a chunk at level, by the quality's own: its playlist's EXT-X-BITRATE for the chunk
 * where it says one (the film's own picture, D464 §1); else the film's own picture's average rate;
 * else a made quality's bound, its peak rate over the chunk and its encoder's buffer (madeBufferS),
 * since H.264 at the film's height may be no smaller than the film's HEVC.
 */
export function chunkBytesAt(
  levels: readonly RoomLevel[],
  level: number,
  chunk: Pick<GridChunk, "sn" | "duration">,
): number {
  const quality = levels[level];
  if (quality === undefined) {
    return 0;
  }
  const said = quality.details?.fragments.find((each) => each.sn === chunk.sn)?.bitrate ?? 0;
  if (said > 0) {
    return (said * chunk.duration) / 8;
  }
  if (level === ownPictureLevel(levels)) {
    return (quality.averageBitrate * chunk.duration) / 8;
  }
  return (quality.bitrate * chunk.duration + quality.averageBitrate * madeBufferS) / 8;
}

/** The quality pairLevel chose for the next chunk, and that chunk with its bytes then. */
export interface PairChoice {
  readonly level: number;
  readonly chunk?: {
    readonly sn: number;
    /** The chunk's bytes at the quality chosen for it, and at the one it loads at. */
    readonly chosenBytes: number;
    readonly bytes: number;
    /** The bytes of the chunk held before it. */
    readonly beforeBytes: number;
  };
}

/**
 * The quality the next chunk is loaded at (D464 §3): chosen, where it fits in the room beside the
 * chunk held before it; else the highest below at which it fits; else the lowest.
 */
export function pairLevel(hls: RoomHls, room: PictureRoom, chosen: number): PairChoice {
  const next = nextChunkOf(hls);
  const span = hls.mainForwardBufferInfo;
  if (next === undefined || span === null) {
    return { level: chosen };
  }
  const beforeBytes = room.heldBefore(next.sn, span)[0]?.bytes ?? 0;
  const roomBytes = room.roomBytes();
  const chosenBytes = chunkBytesAt(hls.levels, chosen, next);
  let level = chosen;
  let bytes = chosenBytes;
  while (level > hls.minAutoLevel && beforeBytes + bytes > roomBytes) {
    level -= 1;
    bytes = chunkBytesAt(hls.levels, level, next);
  }
  return { level, chunk: { sn: next.sn, chosenBytes, bytes, beforeBytes } };
}

/**
 * A playback's step-down of the one chunk that does not fit (D464 §3), between hls.js and the
 * quality its controller chose: level gives hls.js the quality pairLevel says, keeping the one
 * chosen for the room's pacing (chosen); hls.js sets the quality it loads back on its controller at
 * every tick, and that echo (isEcho) is no quality forced (D266). Each step-down is noted once for
 * its chunk and quality, and kept as why its quality was chosen, for the switch to it (whyChose).
 */
export class StepDown {
  readonly #hls: RoomHls;
  readonly #room: PictureRoom;
  readonly #note: ((step: string) => void) | undefined;
  #chosen: number | undefined;
  #steppedTo: number | undefined;
  /** The step last noted, by its chunk and the quality it stepped down to. */
  #noted: { readonly sn: number; readonly level: number } | undefined;
  readonly #why = new Map<number, ChoiceWhy>();

  constructor(hls: RoomHls, room: PictureRoom, note?: (step: string) => void) {
    this.#hls = hls;
    this.#room = room;
    this.#note = note;
  }

  /** The quality the next chunk loads at, where its controller chose chosen. */
  level(chosen: number): number {
    this.#chosen = chosen;
    const pair = pairLevel(this.#hls, this.#room, chosen);
    if (pair.level === chosen || pair.chunk === undefined) {
      this.#steppedTo = undefined;
      this.#why.delete(chosen);
      return chosen;
    }
    this.#steppedTo = pair.level;
    if (this.#noted?.sn !== pair.chunk.sn || this.#noted.level !== pair.level) {
      this.#noted = { sn: pair.chunk.sn, level: pair.level };
      const why =
        `chunk ${String(pair.chunk.sn)} steps down from level ${String(chosen)}:` +
        ` ${megabytes(pair.chunk.chosenBytes)} beside ${megabytes(pair.chunk.beforeBytes)} held` +
        ` is over the room's ${megabytes(this.#room.roomBytes())};` +
        ` level ${String(pair.level)}'s ${megabytes(pair.chunk.bytes)}`;
      this.#why.set(pair.level, { why, linkSamples: "" });
      this.#note?.(why);
    }
    return pair.level;
  }

  /** The quality its controller last chose, before any step-down; undefined before it chose. */
  chosen(): number | undefined {
    return this.#chosen;
  }

  /** Whether hls.js setting level is its echo of the step-down just given it. */
  isEcho(level: number): boolean {
    return level === this.#steppedTo;
  }

  /** Why level was stepped down to, where it was the last time it was given. */
  whyChose(level: number): ChoiceWhy | undefined {
    return this.#why.get(level);
  }
}

/**
 * How far ahead the picture may be loaded so the next chunk, at level, fits in the room beside what
 * is held (D464 §4): to the end of the chunks held from the earliest whose bytes, with every held
 * chunk after it and the next, fit; hls.js loads the next chunk once playback reaches that chunk,
 * the browser freeing what is before it. Where even the chunk before the next does not fit beside
 * it, that one chunk's length; undefined where everything held fits beside it, or nothing is held.
 */
export function roomAheadS(hls: RoomHls, room: PictureRoom, level: number): number | undefined {
  const next = nextChunkOf(hls);
  const span = hls.mainForwardBufferInfo;
  if (next === undefined || span === null) {
    return undefined;
  }
  const held = room.heldBefore(next.sn, span);
  const nearest = held[0];
  if (nearest === undefined) {
    return undefined;
  }
  const roomBytes = room.roomBytes();
  let bytes = chunkBytesAt(hls.levels, level, next);
  let earliest: HeldChunk | undefined;
  for (const chunk of held) {
    if (bytes + chunk.bytes > roomBytes) {
      break;
    }
    bytes += chunk.bytes;
    earliest = chunk;
  }
  if (earliest === held.at(-1)) {
    return undefined;
  }
  return nearest.endS - (earliest ?? nearest).startS;
}
