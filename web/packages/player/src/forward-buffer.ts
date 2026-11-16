// SPDX-License-Identifier: MPL-2.0

/**
 * How far ahead of where it plays a chunked playback loads (forward-buffer-2026-10-29.md P1, P2,
 * P6; the owner, 2026-10-29): a target in seconds that depends on the quality loading, capped by a
 * share of the browser's memory for its picture and of its memory for its sound, each in bytes, never
 * below what holds a quality, lowered by its Watch Together group's runway (D285 §2) and by hls.js's
 * own cut after the browser's buffer ran full, and noted at each change, for the play report.
 */

import { isFireTv } from "@orpheum/sdk";
import Hls, {
  PlaylistLevelType,
  type ErrorData,
  type FragBufferedData,
  type FragLoadedData,
  type HlsConfig,
} from "hls.js";

import { holdsQualityAheadS } from "./abr.ts";
import { deeperForMs, outageText, type LinkOutages, type Outage } from "./link-outage.ts";
import {
  browserStorage,
  limitMemoryIn,
  megabytes,
  PictureRoom,
  type BufferRoom,
} from "./picture-room.ts";

/**
 * How far ahead a playback loads below its top quality, in seconds: the step-up's 20 s loaded ahead
 * (stepUpBufferMs) and room for one chunk of up to 10 s, as 99% are (D414 §5), to load. Less, and the step-up could
 * never open; more only delays the better picture, which shows only once everything loaded below it
 * has played: at the lead's 60 s, up to 90 s from the decision to the change on screen
 * (forward-buffer-2026-10-29.md §1.2, P1; dash.js keeps 18 s below its top). A justified constant,
 * the owner's (2026-10-29).
 */
export const loadsAheadBelowTopS = 30;

/**
 * How far ahead a playback loads at its top quality, in seconds: nothing better can come, so loading
 * ahead there is cover against the link's dips, bounded by the browser's memory
 * (aheadShareOfBufferLimit). dash.js's 60 s for a long film at its top, Media3's 50 s
 * (forward-buffer-2026-10-29.md §2, P1). A justified constant, the owner's (2026-10-29).
 */
export const loadsAheadAtTopS = 60;

/**
 * The least the bytes cap leaves loaded ahead, in seconds: what holds a quality (holdsQualityAheadS).
 * A device whose memory says less cannot hold the top at its rate, which is the ladder's question,
 * not the buffer's (forward-buffer-2026-10-29.md P2). A group's runway and hls.js's own cut may go
 * lower; the memory estimate does not.
 */
export const loadsAheadAtLeastS = holdsQualityAheadS;

/**
 * The share of the browser's memory for a page's picture, and of its memory for its sound, that what
 * is loaded ahead may take: about a third, so the same memory also holds the 30 s kept behind
 * (keptBehindS) and a chunk being appended (forward-buffer-2026-10-29.md P2). A justified constant,
 * the owner's (2026-10-29).
 */
export const aheadShareOfBufferLimit = 1 / 3;

/**
 * How much of what was played a chunked playback keeps in the browser's buffer behind where it plays,
 * in seconds: three of the player's 10 s steps back (seekStepMs) and a Watch Together follower's
 * jump back of a second or less, its chunk of up to 10 s (99% are, D414 §5) kept with it, so a step back plays
 * at once. hls.js keeps everything played by default (Infinity), so a device would hold a film's
 * played minutes up to the browser's memory for a page's picture, and then evict at every append
 * (the efficiency audit's finding 5, web-client-efficiency-audit-2026-10-28.md). That memory is
 * Chromium's 150 MiB on a desktop, which a film's own picture at 20 Mb/s reaches in a minute; on an
 * Android device of 1 GB, as a Fire TV Stick is, Chromium's own source gives 30 MiB, which 30 s
 * behind at 9 Mb/s alone fills, though whether Silk takes that limit is not shown: a staging log had
 * the stick holding more (forward-buffer-2026-10-29.md §3.3; VQ121, its test T4). Lowering it where
 * the limit is 30 MiB is the owner's call (the report's P2). A justified constant.
 */
export const keptBehindS = 30;

/**
 * How far ahead a playback loads, at any quality, once its link has been out long enough
 * (LinkOutages, D487), in seconds: eight minutes, the 6.4 minutes the owner's phone was out mid-film
 * (staging, 2026-11-14), with a minute and a half to spare; at 700 kb/s of picture and 100 kb/s of
 * sound, about 48 MB. The memory bounds it
 * first where it holds less (deeperShareOfBufferLimit). A justified constant, the lead's (D487 §3).
 */
export const loadsAheadAfterOutageS = 480;

/**
 * The share of the browser's memory for a page's picture, and of its memory for its sound, that all
 * a playback loading deeper holds at once may take (D487 §3): what is loaded ahead, the keptBehindS
 * behind it and one chunk being appended (heldBesideAheadS). The other third is for what the
 * average rate misses: a chunk's peak above it, a sound's rate read only from its chunks so far, a
 * device whose memory is less than its kind's (a Fire TV's sound, not yet shown: VQ121). The
 * picture's room (D464) still holds each chunk of picture to roomShareOfBufferLimit, and hls.js's cut
 * after a refusal still lowers it, kept for the playback, so a refusal is not met again. A justified
 * constant, the lead's.
 */
export const deeperShareOfBufferLimit = 2 / 3;

/**
 * What a playback loading deeper holds besides what is loaded ahead, in seconds: keptBehindS and a
 * chunk of up to 10 s being appended, as 99% are (D414 §5).
 */
const heldBesideAheadS = keptBehindS + 10;

/**
 * How far short of the target in force what is loaded ahead may be for the buffer to be full, in
 * seconds: a chunk of up to 10 s, as 99% are (D414 §5). hls.js asks for the next chunk only once what
 * is loaded ahead falls below its target, so a full buffer stands between the target less the chunk
 * on its way and the target and a chunk past it. A justified constant, the lead's (D501 §1).
 */
export const fullWithinS = 10;

const mebibyte = 1024 * 1024;

/**
 * The browser's memory for a page's picture (its video SourceBuffer), in bytes, by the kind of
 * device, as forward-buffer-2026-10-29.md §3.3 read it: Chromium's 150 MiB on a desktop and an
 * ordinary Android device, 30 MiB on an Android device with 1 GB of memory or less and on a
 * Chromecast playing H.264 (its demuxer_memory_limit*.cc), about 100 MiB in Firefox (Mozilla bug
 * 1760529) and about 100 MB in WebKit (Safari, and every browser on an iPhone). Justified constants,
 * read from the browsers' sources or reports of them. A Fire TV's Silk refused a chunk with 31.5 MB
 * held (VQ294's run, 2026-11-11): lowMemory's 30 MiB.
 */
export const videoLimitBytes = {
  ordinary: 150 * mebibyte,
  lowMemory: 30 * mebibyte,
  cast: 30 * mebibyte,
  firefox: 100 * mebibyte,
  webkit: 100_000_000,
} as const satisfies Record<BufferLimitKind, number>;

/**
 * The browser's memory for a page's sound (its audio SourceBuffer), in bytes, by the same kinds of
 * device, as forward-buffer-2026-10-29.md §3.3 read it: Chromium's 12 MiB on a desktop and an
 * ordinary Android device, 2 MiB on an Android device with 1 GB of memory or less and on a
 * Chromecast playing stereo AAC (its demuxer_memory_limit*.cc; a Chromecast's 5 MiB for AC-3 5.1 is
 * not assumed, the sound's codec not being known before it loads), about 30 MiB in Firefox (Mozilla
 * bug 1760529), and WebKit's 100 MB for each SourceBuffer, as for its picture. A film's own sound,
 * copied at the top of its ladder (D368), runs at up to 640 kb/s for AC-3 and more for E-AC-3, where
 * our made sound is 96 kb/s: a Fire TV's sound buffer ran full at the top twice, with 59 s and 31 s
 * loaded ahead (staging, 2026-10-29). Justified constants, read from the browsers' sources or reports
 * of them; the Fire TV's is not yet shown (VQ121, the report's T4).
 */
export const soundLimitBytes = {
  ordinary: 12 * mebibyte,
  lowMemory: 2 * mebibyte,
  cast: 2 * mebibyte,
  firefox: 30 * mebibyte,
  webkit: 100_000_000,
} as const satisfies Record<BufferLimitKind, number>;

/**
 * The most memory, in GB as navigator.deviceMemory says it, of a device Chromium treats as low-end:
 * 1024 MB (its SysUtils.LOW_MEMORY_DEVICE_THRESHOLD_MB), so 30 MiB for its picture
 * (forward-buffer-2026-10-29.md §3.3). A justified constant.
 */
export const lowMemoryAtMostGb = 1;

/** The kind of device whose memory for a page's picture and sound a playback assumes. */
export type BufferLimitKind = "ordinary" | "lowMemory" | "cast" | "firefox" | "webkit";

/** What a playback reads of its device: its browser's user agent, and its memory where it says. */
export interface DeviceFacts {
  readonly userAgent: string;
  /** navigator.deviceMemory, in GB, in its buckets; undefined where the browser does not say (all but Chromium's). */
  readonly deviceMemoryGb: number | undefined;
}

/** This browser's DeviceFacts. */
export function deviceFactsOf(navigatorLike: { readonly userAgent: string }): DeviceFacts {
  const memory: unknown = Reflect.get(navigatorLike, "deviceMemory");
  return {
    userAgent: navigatorLike.userAgent,
    deviceMemoryGb: typeof memory === "number" && memory > 0 ? memory : undefined,
  };
}

/**
 * The kind of device whose memory for its picture and sound a playback assumes (videoLimitBytes,
 * soundLimitBytes): a Chromecast by its CrKey; a device of lowMemoryAtMostGb or less by its
 * deviceMemory, or a Fire TV by its model code, whose Silk says no deviceMemory over http, a
 * secure context's alone (D464 §2); Firefox; WebKit where no Chromium is named (Safari, or any
 * browser on an iPhone); else Chromium's ordinary.
 */
export function bufferLimitKindOf(device: DeviceFacts): BufferLimitKind {
  const { userAgent } = device;
  if (/\bCrKey\//u.test(userAgent)) {
    return "cast";
  }
  if (
    (device.deviceMemoryGb !== undefined && device.deviceMemoryGb <= lowMemoryAtMostGb) ||
    isFireTv(userAgent)
  ) {
    return "lowMemory";
  }
  if (/\bFirefox\//u.test(userAgent)) {
    return "firefox";
  }
  if (/\bAppleWebKit\//u.test(userAgent) && !/\b(?:Chrome|Chromium|Android)\b/u.test(userAgent)) {
    return "webkit";
  }
  return "ordinary";
}

/**
 * A playback's picture room (D464 §2): its buffer's limit the device kind's (videoLimitBytes), or the
 * one learnt on this device and kept in the browser's storage for its user agent, if lower.
 */
export function pictureRoomOf(device: DeviceFacts, note: (note: string) => void): PictureRoom {
  return new PictureRoom({
    assumedBytes: videoLimitBytes[bufferLimitKindOf(device)],
    memory: limitMemoryIn(browserStorage(), {
      userAgent: device.userAgent,
      nowMs: Date.now,
      told: note,
    }),
    note,
  });
}

/** A quality as the target reads it: its peak and average rates, bits a second. */
export interface AheadLevel {
  readonly bitrate: number;
  readonly averageBitrate: number;
}

/**
 * What bounds the target in force: the quality, the picture's bytes, the sound's bytes, the floor
 * either's bytes were held to, loading deeper after an outage and its picture's or sound's bytes
 * (D487), the next chunk's room, a group's runway, hls.js's cut, the page.
 */
export type AheadBound =
  | "below top"
  | "top"
  | "bytes"
  | "sound bytes"
  | "floor"
  | "sound floor"
  | "deeper"
  | "deeper bytes"
  | "deeper sound bytes"
  | "room"
  | "runway"
  | "cut"
  | "page";

/** The target in force, in seconds ahead, and what bounds it. */
export interface AheadTarget {
  readonly aheadS: number;
  readonly by: AheadBound;
}

/** What bounds the target besides the quality and the bytes, each in seconds ahead; undefined for none. */
export interface AheadLimits {
  /** Where the next chunk fits in the picture's room beside what is held (D464 §4), even below the floor. */
  readonly roomS?: number | undefined;
  /** Its Watch Together group's runway (D285 §2). */
  readonly runwayS?: number | undefined;
  /** hls.js's own cut, after the browser's buffer ran full (its reduceMaxBufferLength). */
  readonly cutS?: number | undefined;
  /** The page's own, as a film page's hidden player's (watching.ts's preloadAheadS). */
  readonly atMostS?: number | undefined;
}

/** The sound loading ahead as the target reads it: its rate, bits a second, and the browser's memory for it. */
export interface AheadSound {
  readonly rateBps: number;
  readonly limitBytes: number;
}

/**
 * The target for loading level ahead (P1, P2): loadsAheadAtTopS at the top, loadsAheadBelowTopS
 * below it; no more than aheadShareOfBufferLimit of limitBytes at the level's average rate, nor of
 * the sound's limitBytes at the sound's own rate where it is known, but not below loadsAheadAtLeastS
 * for either; loading deeper (isDeeper, D487), the deeper target where it is further (deeperOf);
 * then no more than each of limits.
 */
export function aheadTargetOf(
  loading: {
    readonly level: AheadLevel;
    readonly isTop: boolean;
    readonly limitBytes: number;
    readonly sound?: AheadSound | undefined;
    /** Whether it loads deeper, its link having been out (LinkOutages, D487). */
    readonly isDeeper?: boolean | undefined;
  },
  limits: AheadLimits = {},
): AheadTarget {
  let target: AheadTarget = loading.isTop
    ? { aheadS: loadsAheadAtTopS, by: "top" }
    : { aheadS: loadsAheadBelowTopS, by: "below top" };
  const picture = { limitBytes: loading.limitBytes, rateBps: rateOf(loading.level) };
  target = cappedByBytes(target, picture, ["bytes", "floor"]);
  if (loading.sound !== undefined) {
    target = cappedByBytes(target, loading.sound, ["sound bytes", "sound floor"]);
  }
  if (loading.isDeeper === true) {
    target = deeperOf(target, { picture, sound: loading.sound });
  }
  const lower: readonly [number | undefined, AheadBound][] = [
    [limits.roomS, "room"],
    [limits.cutS, "cut"],
    [limits.runwayS, "runway"],
    [limits.atMostS, "page"],
  ];
  for (const [aheadS, by] of lower) {
    if (aheadS !== undefined && aheadS < target.aheadS) {
      target = { aheadS, by };
    }
  }
  return target;
}

/**
 * target, no further than aheadShareOfBufferLimit of a buffer's limitBytes at rateBps, bounded by
 * the first of bounds, but not below loadsAheadAtLeastS for that, bounded by the second.
 */
function cappedByBytes(
  target: AheadTarget,
  buffer: { readonly limitBytes: number; readonly rateBps: number },
  bounds: readonly [bytes: AheadBound, floor: AheadBound],
): AheadTarget {
  if (buffer.rateBps <= 0) {
    return target;
  }
  const bytesS = (buffer.limitBytes * aheadShareOfBufferLimit * 8) / buffer.rateBps;
  if (bytesS >= target.aheadS) {
    return target;
  }
  return bytesS < loadsAheadAtLeastS
    ? { aheadS: Math.min(loadsAheadAtLeastS, target.aheadS), by: bounds[1] }
    : { aheadS: bytesS, by: bounds[0] };
}

/**
 * The target loading deeper after an outage (D487 §3), where it is further than target, today's:
 * loadsAheadAfterOutageS, no more than all held at once fits in deeperShareOfBufferLimit of the
 * picture's memory at its rate, nor of the sound's at its own where it is known. Today's where the
 * picture's rate is not yet known, before a quality loads: its bytes would bound nothing.
 */
function deeperOf(
  target: AheadTarget,
  buffers: {
    readonly picture: { readonly limitBytes: number; readonly rateBps: number };
    readonly sound: AheadSound | undefined;
  },
): AheadTarget {
  if (buffers.picture.rateBps <= 0) {
    return target;
  }
  let deeper = heldWithin(
    { aheadS: loadsAheadAfterOutageS, by: "deeper" },
    buffers.picture,
    "deeper bytes",
  );
  if (buffers.sound !== undefined) {
    deeper = heldWithin(deeper, buffers.sound, "deeper sound bytes");
  }
  return deeper.aheadS > target.aheadS ? deeper : target;
}

/**
 * target, no further than lets what is loaded ahead, with the heldBesideAheadS beside it, fit in
 * deeperShareOfBufferLimit of a buffer's limitBytes at rateBps, bounded by by.
 */
function heldWithin(
  target: AheadTarget,
  buffer: { readonly limitBytes: number; readonly rateBps: number },
  by: AheadBound,
): AheadTarget {
  if (buffer.rateBps <= 0) {
    return target;
  }
  const aheadS =
    (buffer.limitBytes * deeperShareOfBufferLimit * 8) / buffer.rateBps - heldBesideAheadS;
  return aheadS < target.aheadS ? { aheadS, by } : target;
}

/** A level's rate as its bytes fill memory: its average, the sound's with it, else its peak. */
function rateOf(level: AheadLevel): number {
  return level.averageBitrate > 0 ? level.averageBitrate : level.bitrate;
}

/** The page's outages as a playback's target reads them, by its own monotonic clock (D487). */
export interface AheadOutages {
  readonly memory: Pick<LinkOutages, "back" | "deeperAt">;
  readonly monotonicMs: () => number;
}

/** What keeping the target needs of hls.js. */
export interface AheadHls {
  readonly config: Pick<HlsConfig, "maxBufferLength" | "maxMaxBufferLength">;
  readonly levels: readonly AheadLevel[];
  readonly media: Pick<HTMLMediaElement, "buffered" | "currentTime"> | null;
}

/**
 * The least length of a chunk of sound whose rate is read, in seconds: an init segment has none, and
 * a film's last chunk may be a sliver whose rate says little.
 */
const soundRateFromAtLeastS = 1;

/**
 * Keeps a chunked playback's hls.js loading as far ahead as aheadTargetOf says for the quality it
 * loads: hls.js reads maxBufferLength and maxMaxBufferLength each time it decides whether to load
 * the next chunk, just after it sets the quality (LEVEL_SWITCHING), so both are set to the target
 * there, and again when a runway is told or lifted, or the buffer runs full. hls.js's own cut, a
 * maxMaxBufferLength it lowered after the browser refused a chunk for its memory, is kept for the
 * rest of the playback, never undone (forward-buffer-2026-10-29.md §1.3, §3.3). The sound's own
 * rate is read from its chunks as they load, the highest of the sound loading, since a master
 * playlist's EXT-X-MEDIA says none (hls.js gives each audio track a bitrate of 0), and the target is
 * set again when it rises. Each change of the target is noted, with its bytes and what bounds it;
 * each buffer run full, with what was loaded; and, once, the device's memory and the limits assumed
 * (P6).
 */
export class ForwardBuffer {
  readonly #hls: AheadHls;
  readonly #limitBytes: number;
  readonly #soundLimitBytes: number;
  readonly #isTop: (level: number) => boolean;
  readonly #note: (note: string) => void;
  readonly #atMostS: number | undefined;
  readonly #room: BufferRoom | undefined;
  #level = -1;
  #runwayS: number | undefined;
  #cutS: number | undefined;
  /** The maxMaxBufferLength last set, by which a lower one is hls.js's own cut. */
  #setS: number | undefined;
  #noted: AheadTarget | undefined;
  /** The sound loading, by its playlist, and the highest rate its chunks loaded at, bits a second. */
  #sound: { readonly playlist: string; rateBps: number } | undefined;
  readonly #outages: AheadOutages | undefined;
  /** The outage it was last noted to load deeper for (D487); undefined while it loads as before. */
  #deeperFor: Outage | undefined;

  constructor(
    hls: AheadHls,
    options: {
      readonly device: DeviceFacts;
      /** Whether level is the top this playback may load: nothing better is to come. */
      readonly isTop: (level: number) => boolean;
      readonly note: (note: string) => void;
      readonly atMostS?: number | undefined;
      /** Its picture's room, whose limit is the picture's and which paces it (D464); undefined for none. */
      readonly room?: BufferRoom | undefined;
      /** The page's outages, by which it loads deeper (D487); undefined for a playback that never does. */
      readonly outages?: AheadOutages | undefined;
    },
  ) {
    this.#hls = hls;
    this.#isTop = options.isTop;
    this.#note = options.note;
    this.#atMostS = options.atMostS;
    this.#room = options.room;
    this.#outages = options.outages;
    const kind = bufferLimitKindOf(options.device);
    this.#limitBytes = videoLimitBytes[kind];
    this.#soundLimitBytes = soundLimitBytes[kind];
    const memory = options.device.deviceMemoryGb;
    const limitBytes = this.#pictureLimitBytes();
    const learntText = limitBytes < this.#limitBytes ? `, ${megabytes(limitBytes)} learnt` : "";
    this.#note(
      `device memory ${memory === undefined ? "unsaid" : `${String(memory)} GB`},` +
        ` video buffer ${megabytes(this.#limitBytes)}${learntText},` +
        ` sound ${megabytes(this.#soundLimitBytes)} assumed (${kind}); ${options.device.userAgent}`,
    );
  }

  /** The picture's buffer's limit: its room's, else its kind's. */
  #pictureLimitBytes(): number {
    return this.#room?.limitBytes() ?? this.#limitBytes;
  }

  /** The target in force now; below the top at the first quality's rate before one loads. */
  target(): AheadTarget {
    const level = this.#hls.levels[this.#level] ?? { bitrate: 0, averageBitrate: 0 };
    return aheadTargetOf(
      {
        level,
        isTop: this.#level >= 0 && this.#isTop(this.#level),
        limitBytes: this.#pictureLimitBytes(),
        sound: this.#soundOf(),
        isDeeper: this.#deeperNow() !== undefined,
      },
      {
        roomS: this.#level >= 0 ? this.#room?.aheadS(this.#level) : undefined,
        runwayS: this.#runwayS,
        cutS: this.#cutS,
        atMostS: this.#atMostS,
      },
    );
  }

  /** The sound loading as the target reads it; undefined before a chunk of it measured its rate. */
  #soundOf(): AheadSound | undefined {
    return this.#sound === undefined
      ? undefined
      : { rateBps: this.#sound.rateBps, limitBytes: this.#soundLimitBytes };
  }

  /**
   * A chunk of sound loaded (hls.js's FRAG_LOADED for an audio fragment), of playlist, bytes long and
   * durationS of sound: the sound loading's rate is the highest of its chunks', a new playlist's
   * starting afresh, and the target is set again when it changes.
   */
  soundLoaded(chunk: {
    readonly playlist: string;
    readonly bytes: number;
    readonly durationS: number;
  }): void {
    if (chunk.durationS < soundRateFromAtLeastS || chunk.bytes <= 0) {
      return;
    }
    const rateBps = (chunk.bytes * 8) / chunk.durationS;
    if (this.#sound?.playlist !== chunk.playlist) {
      this.#sound = { playlist: chunk.playlist, rateBps };
    } else if (rateBps > this.#sound.rateBps) {
      this.#sound.rateBps = rateBps;
    } else {
      return;
    }
    this.#keep();
  }

  /** hls.js begins loading level (LEVEL_SWITCHING): its target is set before hls.js reads it. */
  loads(level: number): void {
    this.#level = level;
    this.#keep();
  }

  /** The outage that has it load deeper now (LinkOutages.deeperAt); undefined for none. */
  #deeperNow(): Outage | undefined {
    const outages = this.#outages;
    return outages?.memory.deeperAt(outages.monotonicMs());
  }

  /**
   * A chunk of picture buffered: its link is back, which ends an outage under way (LinkOutages.back),
   * and the next chunk's room is read again.
   */
  pictureBuffered(): void {
    this.#linkBack();
    this.#keep();
  }

  /**
   * Whether the buffer is full: where the video plays, what is loaded ahead reaches the target in
   * force less fullWithinS, so hls.js waits for it to play down before it asks for more. A film
   * started below the original its person chose moves to it only with its buffer full (D501 §1).
   * Nothing loaded where it plays is never full.
   */
  isFull(): boolean {
    const media = this.#hls.media;
    const around = media === null ? undefined : loadedAround(media);
    return around !== undefined && around.aheadS >= this.target().aheadS - fullWithinS;
  }

  /** The playback ends: an outage it waited through ends with it, as for a playback opened again. */
  ended(): void {
    this.#linkBack();
  }

  #linkBack(): void {
    const outages = this.#outages;
    outages?.memory.back(outages.monotonicMs());
  }

  /**
   * Its group's runway (D285 §2): no more than aheadS ahead while one in the group is loading;
   * undefined lifts it, back to the target in force, hls.js's own cut kept.
   */
  keepRunway(aheadS: number | undefined): void {
    this.#runwayS = aheadS;
    this.#keep();
  }

  /**
   * The browser refused a chunk for its memory (hls.js's bufferFullError, after it freed what it
   * could behind): noted with what was loaded, and hls.js's cut, made before this is told, kept.
   */
  bufferFull(data: Pick<ErrorData, "sourceBufferName">): void {
    this.#takeCut();
    const media = this.#hls.media;
    const around = media === null ? undefined : loadedAround(media);
    const level = this.#hls.levels[this.#level];
    const pictureRateBps = level === undefined ? 0 : rateOf(level);
    const rateBps =
      data.sourceBufferName === "audio" ? (this.#sound?.rateBps ?? 0) : pictureRateBps;
    const loadedText =
      around === undefined
        ? "nothing loaded where it plays"
        : `${around.behindS.toFixed(1)} s behind, ${around.aheadS.toFixed(1)} s ahead` +
          ` (about ${megabytes(((around.behindS + around.aheadS) * rateBps) / 8)}` +
          ` at ${String(Math.round(rateBps / 1000))} kb/s)`;
    const cutText = this.#cutS === undefined ? "no cut" : `cut to ${this.#cutS.toFixed(1)} s`;
    this.#note(`buffer full on ${data.sourceBufferName ?? "?"}: ${loadedText}; ${cutText}`);
    this.#keep();
  }

  /** Sets hls.js's target to the one in force, noting it when it changed, after a change of loading deeper. */
  #keep(): void {
    this.#takeCut();
    this.#noteDeeper();
    const target = this.target();
    const { config } = this.#hls;
    config.maxBufferLength = target.aheadS;
    config.maxMaxBufferLength = target.aheadS;
    this.#setS = target.aheadS;
    // The room moves at every chunk while it bounds the target: noted as it begins to, not at each.
    const isMoved = this.#noted?.aheadS !== target.aheadS && target.by !== "room";
    if (isMoved || this.#noted?.by !== target.by) {
      this.#noted = target;
      this.#note(this.#targetText(target));
    }
  }

  /**
   * Notes each change of the outage it loads deeper for (D487): "loads deeper ahead: its link was out
   * 381.3 s (its loading stopped), back 2.0 s ago; until it stays good 10 min", or, after it,
   * "loads ahead as before: its link good 10 min since it was out 381.3 s (its loading stopped)".
   */
  #noteDeeper(): void {
    const deeper = this.#deeperNow();
    const was = this.#deeperFor;
    if (deeper === was) {
      return;
    }
    this.#deeperFor = deeper;
    const goodText = `${String(deeperForMs / 60_000)} min`;
    if (deeper !== undefined) {
      const agoS = ((this.#outages?.monotonicMs() ?? deeper.endedAtMs) - deeper.endedAtMs) / 1000;
      this.#note(
        `loads deeper ahead: its link was ${outageText(deeper)}, back ${agoS.toFixed(1)} s ago;` +
          ` until it stays good ${goodText}`,
      );
    } else if (was !== undefined) {
      this.#note(
        `loads ahead as before: its link good ${goodText} since it was ${outageText(was)}`,
      );
    }
  }

  /** Keeps hls.js's own cut: a maxMaxBufferLength lower than the one last set. */
  #takeCut(): void {
    const nowS = this.#hls.config.maxMaxBufferLength;
    if (this.#setS !== undefined && nowS < this.#setS) {
      this.#cutS = Math.min(nowS, this.#cutS ?? Number.POSITIVE_INFINITY);
    }
  }

  /**
   * A target's note: "loads ahead 30.0 s (4.9 MB), by below top: level 1 of 0-3, 1896/1296 kb/s; cap
   * 52.4 MB", then the sound's rate and cap where its rate is known, and, at a floor whose bytes are
   * more than the browser's whole memory for them, that its buffer will run full and hls.js's cut
   * lower it: "; sound 2600 kb/s, cap 0.7 MB; over its 2.1 MB, left to hls.js's cut".
   */
  #targetText(target: AheadTarget): string {
    const level = this.#hls.levels[this.#level];
    const rateBps = level === undefined ? 0 : rateOf(level);
    const levelText =
      level === undefined
        ? "no level yet"
        : `level ${String(this.#level)} of 0-${String(this.#hls.levels.length - 1)},` +
          ` ${String(Math.round(level.bitrate / 1000))}/${String(Math.round(rateBps / 1000))} kb/s`;
    const sound = this.#soundOf();
    const soundText =
      sound === undefined
        ? ""
        : `; sound ${String(Math.round(sound.rateBps / 1000))} kb/s,` +
          ` cap ${megabytes(sound.limitBytes * capShareOf(target))}`;
    const [floorRateBps, floorLimitBytes] =
      target.by === "sound floor" && sound !== undefined
        ? [sound.rateBps, sound.limitBytes]
        : [rateBps, this.#pictureLimitBytes()];
    const isFloor = target.by === "floor" || target.by === "sound floor";
    const overflowText =
      isFloor && (target.aheadS * floorRateBps) / 8 > floorLimitBytes
        ? `; over its ${megabytes(floorLimitBytes)}, left to hls.js's cut`
        : "";
    return (
      `loads ahead ${target.aheadS.toFixed(1)} s (${megabytes((target.aheadS * rateBps) / 8)})` +
      `, by ${target.by}: ${levelText}; cap ${megabytes(this.#pictureLimitBytes() * capShareOf(target))}` +
      soundText +
      overflowText
    );
  }
}

/**
 * The share of a buffer's memory a target's note says its cap is: all held at once loading deeper
 * (deeperShareOfBufferLimit), else what is loaded ahead (aheadShareOfBufferLimit).
 */
function capShareOf(target: AheadTarget): number {
  return target.by.startsWith("deeper") ? deeperShareOfBufferLimit : aheadShareOfBufferLimit;
}

/** What media holds loaded behind and ahead of where it plays, in seconds; undefined where nothing is loaded there. */
function loadedAround(
  media: Pick<HTMLMediaElement, "buffered" | "currentTime">,
): { readonly behindS: number; readonly aheadS: number } | undefined {
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index += 1) {
    const start = buffered.start(index);
    const end = buffered.end(index);
    if (start <= currentTime && currentTime <= end) {
      return { behindS: currentTime - start, aheadS: end - currentTime };
    }
  }
  return undefined;
}

/**
 * Keeps hls's loading ahead to a ForwardBuffer's target (P1, P2): at each quality it begins to
 * load, at each chunk of sound loaded and of picture buffered, and at each buffer run full. Returns it, and the stopping.
 */
export function keepForwardBuffer(
  hls: AheadHls & Pick<Hls, "on" | "off">,
  options: ConstructorParameters<typeof ForwardBuffer>[1],
): { readonly buffer: ForwardBuffer; readonly stop: () => void } {
  const buffer = new ForwardBuffer(hls, options);
  const onSwitching = (_event: unknown, data: { readonly level: number }): void => {
    buffer.loads(data.level);
  };
  const onLoaded = (_event: unknown, data: FragLoadedData): void => {
    if (data.frag.type === PlaylistLevelType.AUDIO) {
      buffer.soundLoaded({
        playlist: data.frag.baseurl,
        bytes: data.payload.byteLength,
        durationS: data.frag.duration,
      });
    }
  };
  const onBuffered = (_event: unknown, data: FragBufferedData): void => {
    if (data.frag.type === PlaylistLevelType.MAIN) {
      buffer.pictureBuffered();
    }
  };
  const onError = (_event: unknown, data: ErrorData): void => {
    if (data.details === Hls.ErrorDetails.BUFFER_FULL_ERROR) {
      buffer.bufferFull(data);
    }
  };
  hls.on(Hls.Events.LEVEL_SWITCHING, onSwitching);
  hls.on(Hls.Events.FRAG_LOADED, onLoaded);
  hls.on(Hls.Events.FRAG_BUFFERED, onBuffered);
  hls.on(Hls.Events.ERROR, onError);
  return {
    buffer,
    stop: () => {
      buffer.ended();
      hls.off(Hls.Events.LEVEL_SWITCHING, onSwitching);
      hls.off(Hls.Events.FRAG_LOADED, onLoaded);
      hls.off(Hls.Events.FRAG_BUFFERED, onBuffered);
      hls.off(Hls.Events.ERROR, onError);
    },
  };
}
