// SPDX-License-Identifier: MPL-2.0

/**
 * The room a chunked playback's picture buffer has for the next chunk (D464): each chunk sized at
 * each quality by its own bytes, the chunks the buffer holds, and the buffer's limit, the device's
 * table's or, after the browser first refused a chunk for its memory, what it was seen to hold,
 * remembered in the browser's storage for its user agent. From them, the quality the next chunk
 * fits at beside the chunk before it (pairLevel), the first step-down after it (foreseenStep), and
 * how far ahead to load so it fits beside what is held (roomAheadS).
 */

import type { ForeseenStepDown } from "@orpheum/sdk";
import { ownPictureLevel, type ChoiceLevel } from "./abr-choice.ts";
import type { ChoiceWhy } from "./abr-options.ts";
import { pictureStreamOf } from "./step-foresight.ts";

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

/**
 * hls.js's own look-up tolerance (maxFragLookUpTolerance): a chunk ending no further than it past
 * the start of the picture loaded is no longer held before the next.
 */
const lookUpToleranceS = 0.25;

/**
 * A chunk of picture as the room keeps it: its place on the grid, the quality it was loaded at, its
 * span, its bytes as loaded.
 */
export interface HeldChunk {
  readonly sn: number;
  readonly level: number;
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
  /** With, from hls.js, the seconds loaded ahead of where it plays (len). */
  readonly mainForwardBufferInfo: (HeldSpan & { readonly len?: number }) | null;
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
 * How long a learnt limit is kept: it only ever falls, so one learnt from a passing refusal would
 * hold the device's buffer low for good (D494); a week on, the device learns it again (the owner,
 * 2026-11-16: "7 days is fine").
 */
export const learntLimitKeptMs: number = 7 * 24 * 60 * 60 * 1000;

/** Of a kept limit, its bytes and when it was learnt; undefined where it is not a limit. */
function keptLimitOf(
  kept: object,
): { readonly bytes: number; readonly learntAtMs: number } | undefined {
  const bytes: unknown = Reflect.get(kept, "bytes");
  const learntAtMs: unknown = Reflect.get(kept, "learntAtMs");
  return typeof bytes === "number" && bytes > 0 && typeof learntAtMs === "number"
    ? { bytes, learntAtMs }
    : undefined;
}

/**
 * A learnt limit kept in storage with userAgent and when it was learnt, read back only by the same
 * user agent within learntLimitKeptMs: a browser updated, or another on the same origin, learns its
 * own. Storage refused or throwing keeps nothing. A kept value that is no limit's is said, through
 * told, and read as none (D494).
 */
export function limitMemoryIn(
  storage: LimitStorage | undefined,
  device: {
    readonly userAgent: string;
    readonly nowMs: () => number;
    readonly told: (note: string) => void;
  },
): LimitMemory {
  const { userAgent, nowMs, told } = device;
  return {
    read: () => {
      let kept: unknown;
      try {
        kept = JSON.parse(storage?.getItem(limitKey) ?? "null");
      } catch {
        told("picture buffer limit: the kept one is not JSON, and is none");
        return undefined;
      }
      if (kept === null) {
        return undefined;
      }
      if (typeof kept !== "object") {
        told("picture buffer limit: the kept one is not a limit, and is none");
        return undefined;
      }
      if (Reflect.get(kept, "userAgent") !== userAgent) {
        return undefined;
      }
      const limit = keptLimitOf(kept);
      if (limit === undefined) {
        told("picture buffer limit: the kept one is not a limit, and is none");
        return undefined;
      }
      return nowMs() - limit.learntAtMs < learntLimitKeptMs ? limit.bytes : undefined;
    },
    write: (bytes) => {
      try {
        storage?.setItem(limitKey, JSON.stringify({ userAgent, bytes, learntAtMs: nowMs() }));
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

  /** Whether chunk sn is held, at any quality, and not yet played past what is loaded in span. */
  holds(sn: number, span: HeldSpan): boolean {
    const chunk = this.#held.get(sn);
    return chunk !== undefined && chunk.endS > span.start + lookUpToleranceS;
  }

  /** The chunks held just before chunk sn, still in span, nearest first. */
  heldBefore(sn: number, span: HeldSpan): readonly HeldChunk[] {
    const before: HeldChunk[] = [];
    for (let at = sn - 1; ; at -= 1) {
      const chunk = this.#held.get(at);
      if (chunk === undefined || chunk.endS <= span.start + lookUpToleranceS) {
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

/**
 * The chunk hls.js loads next, at any quality: the chunk the picture loaded ends in, but where the
 * room holds it, at whatever quality, the one after it. hls.js 1.7.3 takes the next chunk on the
 * quality it last loaded, and loadsNoChunkAgain makes it do so on every other (D503 §4): a chunk
 * whose picture ends short of its playlist's end, a made quality's chunk 40 ending at 200.77 s and
 * its playlist at 200.784, was loaded again at the quality above, 22.8 MB, where the room had
 * judged chunk 41 (file 1504 on the Fire TV, 2026-11-15; idiosyncrasies.md).
 */
export function nextChunkOf(
  hls: RoomHls,
  room: PictureRoom,
): (GridChunk & { readonly sn: number }) | undefined {
  const span = hls.mainForwardBufferInfo;
  const fragments = gridOf(hls);
  if (span === null || fragments === undefined) {
    return undefined;
  }
  const endsIn = fragments.find((chunk) => chunk.start + chunk.duration > span.end);
  const isHeld = typeof endsIn?.sn === "number" && room.holds(endsIn.sn, span);
  return numbered(isHeld ? fragments.find((chunk) => chunk.sn === Number(endsIn.sn) + 1) : endsIn);
}

/** The film's chunks, the same at every quality, from the first quality whose playlist is loaded. */
function gridOf(hls: RoomHls): readonly GridChunk[] | undefined {
  return hls.levels.find((each) => each.details !== undefined)?.details?.fragments;
}

/** A chunk of picture with its number on the grid; undefined for none, or an init segment. */
function numbered(chunk: GridChunk | undefined): (GridChunk & { readonly sn: number }) | undefined {
  return chunk === undefined || chunk.sn === "initSegment" ? undefined : { ...chunk, sn: chunk.sn };
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
    /** The chunk hls.js loads next (nextChunkOf). */
    readonly sn: number;
    /** Its bytes at the quality chosen, and at the one it loads at. */
    readonly chosenBytes: number;
    readonly bytes: number;
    /** The bytes of the chunk held before it. */
    readonly beforeBytes: number;
  };
}

/**
 * The quality the next chunk is loaded at (D464 §3): chosen, where it fits in the room beside the
 * chunk held before it; else the highest below at which it fits; else the lowest. The chunk judged
 * is the one hls.js loads next, the same at every quality (nextChunkOf).
 */
export function pairLevel(hls: RoomHls, room: PictureRoom, chosen: number): PairChoice {
  const span = hls.mainForwardBufferInfo;
  const next = nextChunkOf(hls, room);
  if (span === null || next === undefined) {
    return { level: chosen };
  }
  const beforeBytes = room.heldBefore(next.sn, span)[0]?.bytes ?? 0;
  const chosenBytes = chunkBytesAt(hls.levels, chosen, next);
  const roomBytes = room.roomBytes();
  let level = chosen;
  let bytes = chosenBytes;
  while (level > hls.minAutoLevel && beforeBytes + bytes > roomBytes) {
    level -= 1;
    bytes = chunkBytesAt(hls.levels, level, next);
  }
  return { level, chunk: { sn: next.sn, chosenBytes, bytes, beforeBytes } };
}

/**
 * The fewest chunks after the next that foreseenStep looks over, where the buffer holds fewer: a
 * quality's run, started as it says, takes FFmpeg's open of 5 to 8 s and its first chunk to make
 * (file 15497, production, 2026-11-17), about one chunk of film at 2x, so two give it one to spare.
 * A justified constant.
 */
export const foreseenChunksAtLeast = 2;

/** A step-down foreseen: the quality a chunk after the next will step down to, and that chunk. */
export interface ForeseenStep {
  readonly level: number;
  readonly sn: number;
  readonly startS: number;
}

/**
 * The first chunk after the next that will step down from chosen, and the quality it will (D532 §1),
 * judged as pairLevel judges the next beside the chunk before it, each at chosen until the first
 * that steps, the next at the quality pair loads it at; over as many chunks as the buffer holds
 * before the next, and at least foreseenChunksAtLeast. Undefined for none, or one to the quality
 * the next loads at, which the player asks for already.
 */
export function foreseenStep(
  hls: RoomHls,
  room: PictureRoom,
  { chosen, pair }: { readonly chosen: number; readonly pair: PairChoice },
): ForeseenStep | undefined {
  const span = hls.mainForwardBufferInfo;
  const fragments = gridOf(hls);
  const next = pair.chunk;
  if (span === null || fragments === undefined || next === undefined) {
    return undefined;
  }
  const roomBytes = room.roomBytes();
  const lastSn = next.sn + Math.max(foreseenChunksAtLeast, room.heldBefore(next.sn, span).length);
  let beforeBytes = next.bytes;
  for (let sn = next.sn + 1; sn <= lastSn; sn += 1) {
    const chunk = numbered(fragments.find((each) => each.sn === sn));
    if (chunk === undefined) {
      return undefined;
    }
    let level = chosen;
    let bytes = chunkBytesAt(hls.levels, level, chunk);
    while (level > hls.minAutoLevel && beforeBytes + bytes > roomBytes) {
      level -= 1;
      bytes = chunkBytesAt(hls.levels, level, chunk);
    }
    if (level < chosen) {
      return level === pair.level ? undefined : { level, sn, startS: chunk.start };
    }
    beforeBytes = bytes;
  }
  return undefined;
}

/** step, as the server is told it: by its quality's stream; undefined where its playlist names none. */
function saidStepOf(hls: RoomHls, step: ForeseenStep): ForeseenStepDown | undefined {
  const pictureStream = pictureStreamOf(hls.levels[step.level]?.uri);
  return pictureStream === undefined
    ? undefined
    : { pictureStream, fromMs: Math.round(step.startS * 1000) };
}

/** Whether two step-downs told are the same, none being the same as none. */
function isSameStep(a: ForeseenStepDown | undefined, b: ForeseenStepDown | undefined): boolean {
  return a?.pictureStream === b?.pictureStream && a?.fromMs === b?.fromMs;
}

/**
 * A playback's step-down of the one chunk that does not fit (D464 §3), between hls.js and the
 * quality its controller chose: level gives hls.js the quality pairLevel says, keeping the one
 * chosen for the room's pacing (chosen); hls.js sets the quality it loads back on its controller at
 * every tick, and that echo (isEcho) is no quality forced (D266). Each step-down is noted once for
 * its chunk and quality, and kept as why its quality was chosen, for the switch to it (whyChose).
 * The first step-down after the next chunk (foreseenStep) is told to foresee, by its quality's
 * stream, each time it changes, and undefined once there is none (D532 §1).
 */
export class StepDown {
  readonly #hls: RoomHls;
  readonly #room: PictureRoom;
  readonly #note: ((step: string) => void) | undefined;
  #chosen: number | undefined;
  #steppedTo: number | undefined;
  /** The step last noted, by its chunk and the quality it stepped down to. */
  #noted: { readonly sn: number; readonly level: number } | undefined;
  /** The chunk last stepped down, and the quality it was stepped down to. */
  #stepped: { readonly sn: number; readonly level: number } | undefined;
  /** The chunk last noted kept at its step-down (keptDown). */
  #keptNoted: number | undefined;
  readonly #why = new Map<number, ChoiceWhy>();
  readonly #foresee: ((step: ForeseenStepDown | undefined) => void) | undefined;
  /** The step-down last told to foresee; undefined for none. */
  #foreseen: ForeseenStepDown | undefined;

  constructor(
    hls: RoomHls,
    room: PictureRoom,
    tell: {
      readonly note?: ((step: string) => void) | undefined;
      readonly foresee?: ((step: ForeseenStepDown | undefined) => void) | undefined;
    } = {},
  ) {
    this.#hls = hls;
    this.#room = room;
    this.#note = tell.note;
    this.#foresee = tell.foresee;
  }

  /** The quality the next chunk loads at, where its controller chose chosen. */
  level(chosen: number): number {
    this.#chosen = chosen;
    const pair = pairLevel(this.#hls, this.#room, chosen);
    this.#tellForeseen(foreseenStep(this.#hls, this.#room, { chosen, pair }));
    const kept = this.#keptDown(pair);
    if (kept !== undefined) {
      this.#steppedTo = kept;
      return kept;
    }
    if (pair.level === chosen || pair.chunk === undefined) {
      this.#steppedTo = undefined;
      this.#why.delete(chosen);
      return chosen;
    }
    this.#steppedTo = pair.level;
    this.#stepped = { sn: pair.chunk.sn, level: pair.level };
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

  /**
   * The quality a chunk stepped down and asked for again, as after it failed or was not made yet,
   * keeps while less than its length is loaded ahead, where the pair would now go higher: the
   * quality it stepped down to (D503 §3). Its retry was judged afresh, and with the heavy chunk
   * before it played out, chunk 41, stepped down to level 3 and answered 425, was asked again at
   * level 5 with nothing ahead, 21.4 MB, and play stood 6.7 s (file 1504 on the Fire TV,
   * 2026-11-15). Undefined where it does not hold.
   */
  #keptDown(pair: PairChoice): number | undefined {
    const stepped = this.#stepped;
    if (stepped === undefined || pair.chunk?.sn !== stepped.sn || pair.level <= stepped.level) {
      return undefined;
    }
    const aheadS = this.#littleAheadS();
    if (aheadS === undefined) {
      return undefined;
    }
    if (this.#keptNoted !== stepped.sn) {
      this.#keptNoted = stepped.sn;
      this.#note?.(
        `chunk ${String(stepped.sn)} asked again at level ${String(stepped.level)}, its step-down,` +
          ` with ${aheadS.toFixed(1)} s ahead`,
      );
    }
    return stepped.level;
  }

  /** The seconds loaded ahead, where they are fewer than the next chunk's; undefined otherwise. */
  #littleAheadS(): number | undefined {
    const aheadS = this.#hls.mainForwardBufferInfo?.len;
    const next = nextChunkOf(this.#hls, this.#room);
    return aheadS !== undefined && next !== undefined && aheadS < next.duration
      ? aheadS
      : undefined;
  }

  /**
   * Tells foresee the step-down foreseen, by its quality's stream, where it is not the one told
   * last; one to a quality whose playlist names no stream is none.
   */
  #tellForeseen(step: ForeseenStep | undefined): void {
    const said = step === undefined ? undefined : saidStepOf(this.#hls, step);
    if (isSameStep(this.#foreseen, said)) {
      return;
    }
    this.#foreseen = said;
    this.#foresee?.(said);
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
  const next = nextChunkOf(hls, room);
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
