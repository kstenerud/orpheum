// SPDX-License-Identifier: MPL-2.0

/**
 * How far ahead of where it plays a chunked playback loads (forward-buffer-2026-10-29.md P1, P2,
 * P6; the owner, 2026-10-29): a target in seconds that depends on the quality loading, capped by a
 * share of the browser's memory for its picture in bytes, never below what holds a quality, lowered
 * by its Watch Together group's runway (D285 §2) and by hls.js's own cut after the browser's buffer
 * ran full, and noted at each change, for the play report.
 */

import Hls, { type ErrorData, type HlsConfig } from "hls.js";

import { holdsQualityAheadS } from "./abr.ts";

/**
 * How far ahead a playback loads below its top quality, in seconds: the step-up's 20 s loaded ahead
 * (stepUpBufferMs) and room for one chunk of the ten-second grid to load. Less, and the step-up could
 * never open; more only delays the better picture, which shows only once everything loaded below it
 * has played: at the lead's 60 s, up to 90 s from the decision to the change on screen
 * (forward-buffer-2026-10-29.md §1.2, P1; dash.js keeps 18 s below its top). A justified constant,
 * the owner's (2026-10-29).
 */
export const loadsAheadBelowTopS = 30;

/**
 * How far ahead a playback loads at its top quality, in seconds: nothing better can come, so loading
 * ahead there is cover against the link's dips, bounded by the browser's memory
 * (aheadShareOfVideoLimit). dash.js's 60 s for a long film at its top, Media3's 50 s
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
 * The share of the browser's memory for a page's picture that what is loaded ahead may take: about
 * a third, so the same memory also holds the 30 s kept behind (keptBehindS) and a chunk being
 * appended (forward-buffer-2026-10-29.md P2). A justified constant, the owner's (2026-10-29).
 */
export const aheadShareOfVideoLimit = 1 / 3;

const mebibyte = 1024 * 1024;

/**
 * The browser's memory for a page's picture (its video SourceBuffer), in bytes, by the kind of
 * device, as forward-buffer-2026-10-29.md §3.3 read it: Chromium's 150 MiB on a desktop and an
 * ordinary Android device, 30 MiB on an Android device with 1 GB of memory or less and on a
 * Chromecast playing H.264 (its demuxer_memory_limit*.cc), about 100 MiB in Firefox (Mozilla bug
 * 1760529) and about 100 MB in WebKit (Safari, and every browser on an iPhone). Justified constants,
 * read from the browsers' sources or reports of them; the Fire TV's is not yet shown (VQ121, the
 * report's T4).
 */
export const videoLimitBytes = {
  ordinary: 150 * mebibyte,
  lowMemory: 30 * mebibyte,
  cast: 30 * mebibyte,
  firefox: 100 * mebibyte,
  webkit: 100_000_000,
} as const;

/**
 * The most memory, in GB as navigator.deviceMemory says it, of a device Chromium treats as low-end:
 * 1024 MB (its SysUtils.LOW_MEMORY_DEVICE_THRESHOLD_MB), so 30 MiB for its picture
 * (forward-buffer-2026-10-29.md §3.3). A justified constant.
 */
export const lowMemoryAtMostGb = 1;

/** The kind of device whose memory for a page's picture a playback assumes. */
export type VideoLimitKind = keyof typeof videoLimitBytes;

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
 * The kind of device whose memory for its picture a playback assumes (videoLimitBytes): a
 * Chromecast by its CrKey; a device of lowMemoryAtMostGb or less by its deviceMemory; Firefox; WebKit
 * where no Chromium is named (Safari, or any browser on an iPhone); else Chromium's ordinary.
 */
export function videoLimitKindOf(device: DeviceFacts): VideoLimitKind {
  const { userAgent } = device;
  if (/\bCrKey\//u.test(userAgent)) {
    return "cast";
  }
  if (device.deviceMemoryGb !== undefined && device.deviceMemoryGb <= lowMemoryAtMostGb) {
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

/** A quality as the target reads it: its peak and average rates, bits a second. */
export interface AheadLevel {
  readonly bitrate: number;
  readonly averageBitrate: number;
}

/** What bounds the target in force: the quality, the bytes, the bytes' floor, a group's runway, hls.js's cut, the page. */
export type AheadBound = "below top" | "top" | "bytes" | "floor" | "runway" | "cut" | "page";

/** The target in force, in seconds ahead, and what bounds it. */
export interface AheadTarget {
  readonly aheadS: number;
  readonly by: AheadBound;
}

/** What bounds the target besides the quality and the bytes, each in seconds ahead; undefined for none. */
export interface AheadLimits {
  /** Its Watch Together group's runway (D285 §2). */
  readonly runwayS?: number | undefined;
  /** hls.js's own cut, after the browser's buffer ran full (its reduceMaxBufferLength). */
  readonly cutS?: number | undefined;
  /** The page's own, as a film page's hidden player's (watching.ts's preloadAheadS). */
  readonly atMostS?: number | undefined;
}

/**
 * The target for loading level ahead (P1, P2): loadsAheadAtTopS at the top, loadsAheadBelowTopS
 * below it; no more than aheadShareOfVideoLimit of limitBytes at the level's average rate, but not
 * below loadsAheadAtLeastS for that; then no more than each of limits.
 */
export function aheadTargetOf(
  loading: { readonly level: AheadLevel; readonly isTop: boolean; readonly limitBytes: number },
  limits: AheadLimits = {},
): AheadTarget {
  let target: AheadTarget = loading.isTop
    ? { aheadS: loadsAheadAtTopS, by: "top" }
    : { aheadS: loadsAheadBelowTopS, by: "below top" };
  const rateBps = rateOf(loading.level);
  if (rateBps > 0) {
    const bytesS = (loading.limitBytes * aheadShareOfVideoLimit * 8) / rateBps;
    if (bytesS < target.aheadS) {
      target =
        bytesS < loadsAheadAtLeastS
          ? { aheadS: Math.min(loadsAheadAtLeastS, target.aheadS), by: "floor" }
          : { aheadS: bytesS, by: "bytes" };
    }
  }
  const lower: readonly [number | undefined, AheadBound][] = [
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

/** A level's rate as its bytes fill memory: its average, the sound's with it, else its peak. */
function rateOf(level: AheadLevel): number {
  return level.averageBitrate > 0 ? level.averageBitrate : level.bitrate;
}

/** What keeping the target needs of hls.js. */
export interface AheadHls {
  readonly config: Pick<HlsConfig, "maxBufferLength" | "maxMaxBufferLength">;
  readonly levels: readonly AheadLevel[];
  readonly media: Pick<HTMLMediaElement, "buffered" | "currentTime"> | null;
}

/**
 * Keeps a chunked playback's hls.js loading as far ahead as aheadTargetOf says for the quality it
 * loads: hls.js reads maxBufferLength and maxMaxBufferLength each time it decides whether to load
 * the next chunk, just after it sets the quality (LEVEL_SWITCHING), so both are set to the target
 * there, and again when a runway is told or lifted, or the buffer runs full. hls.js's own cut, a
 * maxMaxBufferLength it lowered after the browser refused a chunk for its memory, is kept for the
 * rest of the playback, never undone (forward-buffer-2026-10-29.md §1.3, §3.3). Each change of the
 * target is noted, with its bytes and what bounds it; each buffer run full, with what was loaded;
 * and, once, the device's memory and the limit assumed (P6).
 */
export class ForwardBuffer {
  readonly #hls: AheadHls;
  readonly #limitBytes: number;
  readonly #isTop: (level: number) => boolean;
  readonly #note: (note: string) => void;
  readonly #atMostS: number | undefined;
  #level = -1;
  #runwayS: number | undefined;
  #cutS: number | undefined;
  /** The maxMaxBufferLength last set, by which a lower one is hls.js's own cut. */
  #setS: number | undefined;
  #noted: AheadTarget | undefined;

  constructor(
    hls: AheadHls,
    options: {
      readonly device: DeviceFacts;
      /** Whether level is the top this playback may load: nothing better is to come. */
      readonly isTop: (level: number) => boolean;
      readonly note: (note: string) => void;
      readonly atMostS?: number | undefined;
    },
  ) {
    this.#hls = hls;
    this.#isTop = options.isTop;
    this.#note = options.note;
    this.#atMostS = options.atMostS;
    const kind = videoLimitKindOf(options.device);
    this.#limitBytes = videoLimitBytes[kind];
    const memory = options.device.deviceMemoryGb;
    this.#note(
      `device memory ${memory === undefined ? "unsaid" : `${String(memory)} GB`},` +
        ` video buffer ${megabytes(this.#limitBytes)} assumed (${kind}); ${options.device.userAgent}`,
    );
  }

  /** The target in force now; below the top at the first quality's rate before one loads. */
  target(): AheadTarget {
    const level = this.#hls.levels[this.#level] ?? { bitrate: 0, averageBitrate: 0 };
    return aheadTargetOf(
      { level, isTop: this.#level >= 0 && this.#isTop(this.#level), limitBytes: this.#limitBytes },
      { runwayS: this.#runwayS, cutS: this.#cutS, atMostS: this.#atMostS },
    );
  }

  /** hls.js begins loading level (LEVEL_SWITCHING): its target is set before hls.js reads it. */
  loads(level: number): void {
    this.#level = level;
    this.#keep();
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
    const rateBps = level === undefined ? 0 : rateOf(level);
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

  /** Sets hls.js's target to the one in force, noting it when it changed. */
  #keep(): void {
    this.#takeCut();
    const target = this.target();
    const { config } = this.#hls;
    config.maxBufferLength = target.aheadS;
    config.maxMaxBufferLength = target.aheadS;
    this.#setS = target.aheadS;
    if (this.#noted?.aheadS !== target.aheadS || this.#noted.by !== target.by) {
      this.#noted = target;
      this.#note(this.#targetText(target));
    }
  }

  /** Keeps hls.js's own cut: a maxMaxBufferLength lower than the one last set. */
  #takeCut(): void {
    const nowS = this.#hls.config.maxMaxBufferLength;
    if (this.#setS !== undefined && nowS < this.#setS) {
      this.#cutS = Math.min(nowS, this.#cutS ?? Number.POSITIVE_INFINITY);
    }
  }

  /** A target's note: "loads ahead 30.0 s (4.9 MB), by below top: level 1 of 0-3, 1896/1296 kb/s; cap 52.4 MB". */
  #targetText(target: AheadTarget): string {
    const level = this.#hls.levels[this.#level];
    const rateBps = level === undefined ? 0 : rateOf(level);
    const levelText =
      level === undefined
        ? "no level yet"
        : `level ${String(this.#level)} of 0-${String(this.#hls.levels.length - 1)},` +
          ` ${String(Math.round(level.bitrate / 1000))}/${String(Math.round(rateBps / 1000))} kb/s`;
    return (
      `loads ahead ${target.aheadS.toFixed(1)} s (${megabytes((target.aheadS * rateBps) / 8)})` +
      `, by ${target.by}: ${levelText}; cap ${megabytes(this.#limitBytes * aheadShareOfVideoLimit)}`
    );
  }
}

/** Bytes as megabytes, for a note: "52.4 MB". */
function megabytes(bytes: number): string {
  return `${(bytes / 1_000_000).toFixed(1)} MB`;
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
 * load, and at each buffer run full. Returns it, and the stopping.
 */
export function keepForwardBuffer(
  hls: AheadHls & Pick<Hls, "on" | "off">,
  options: ConstructorParameters<typeof ForwardBuffer>[1],
): { readonly buffer: ForwardBuffer; readonly stop: () => void } {
  const buffer = new ForwardBuffer(hls, options);
  const onSwitching = (_event: unknown, data: { readonly level: number }): void => {
    buffer.loads(data.level);
  };
  const onError = (_event: unknown, data: ErrorData): void => {
    if (data.details === Hls.ErrorDetails.BUFFER_FULL_ERROR) {
      buffer.bufferFull(data);
    }
  };
  hls.on(Hls.Events.LEVEL_SWITCHING, onSwitching);
  hls.on(Hls.Events.ERROR, onError);
  return {
    buffer,
    stop: () => {
      hls.off(Hls.Events.LEVEL_SWITCHING, onSwitching);
      hls.off(Hls.Events.ERROR, onError);
    },
  };
}
