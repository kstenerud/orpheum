// SPDX-License-Identifier: MPL-2.0

/**
 * How far ahead of where it plays a chunked playback loads (forward-buffer-2026-10-29.md P1, P2,
 * P6; the owner, 2026-10-29): a target in seconds that depends on the quality loading, capped by a
 * share of the browser's memory for its picture and of its memory for its sound, each in bytes, never
 * below what holds a quality, lowered by its Watch Together group's runway (D285 §2) and by hls.js's
 * own cut after the browser's buffer ran full, and noted at each change, for the play report.
 */

import Hls, {
  PlaylistLevelType,
  type ErrorData,
  type FragLoadedData,
  type HlsConfig,
} from "hls.js";

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
 * soundLimitBytes): a
 * Chromecast by its CrKey; a device of lowMemoryAtMostGb or less by its deviceMemory; Firefox; WebKit
 * where no Chromium is named (Safari, or any browser on an iPhone); else Chromium's ordinary.
 */
export function bufferLimitKindOf(device: DeviceFacts): BufferLimitKind {
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

/**
 * What bounds the target in force: the quality, the picture's bytes, the sound's bytes, the floor
 * either's bytes were held to, a group's runway, hls.js's cut, the page.
 */
export type AheadBound =
  | "below top"
  | "top"
  | "bytes"
  | "sound bytes"
  | "floor"
  | "sound floor"
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
 * for either; then no more than each of limits.
 */
export function aheadTargetOf(
  loading: {
    readonly level: AheadLevel;
    readonly isTop: boolean;
    readonly limitBytes: number;
    readonly sound?: AheadSound | undefined;
  },
  limits: AheadLimits = {},
): AheadTarget {
  let target: AheadTarget = loading.isTop
    ? { aheadS: loadsAheadAtTopS, by: "top" }
    : { aheadS: loadsAheadBelowTopS, by: "below top" };
  target = cappedByBytes(
    target,
    { limitBytes: loading.limitBytes, rateBps: rateOf(loading.level) },
    ["bytes", "floor"],
  );
  if (loading.sound !== undefined) {
    target = cappedByBytes(target, loading.sound, ["sound bytes", "sound floor"]);
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
  #level = -1;
  #runwayS: number | undefined;
  #cutS: number | undefined;
  /** The maxMaxBufferLength last set, by which a lower one is hls.js's own cut. */
  #setS: number | undefined;
  #noted: AheadTarget | undefined;
  /** The sound loading, by its playlist, and the highest rate its chunks loaded at, bits a second. */
  #sound: { readonly playlist: string; rateBps: number } | undefined;

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
    const kind = bufferLimitKindOf(options.device);
    this.#limitBytes = videoLimitBytes[kind];
    this.#soundLimitBytes = soundLimitBytes[kind];
    const memory = options.device.deviceMemoryGb;
    this.#note(
      `device memory ${memory === undefined ? "unsaid" : `${String(memory)} GB`},` +
        ` video buffer ${megabytes(this.#limitBytes)}, sound ${megabytes(this.#soundLimitBytes)}` +
        ` assumed (${kind}); ${options.device.userAgent}`,
    );
  }

  /** The target in force now; below the top at the first quality's rate before one loads. */
  target(): AheadTarget {
    const level = this.#hls.levels[this.#level] ?? { bitrate: 0, averageBitrate: 0 };
    return aheadTargetOf(
      {
        level,
        isTop: this.#level >= 0 && this.#isTop(this.#level),
        limitBytes: this.#limitBytes,
        sound: this.#soundOf(),
      },
      { runwayS: this.#runwayS, cutS: this.#cutS, atMostS: this.#atMostS },
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
          ` cap ${megabytes(sound.limitBytes * aheadShareOfBufferLimit)}`;
    const [floorRateBps, floorLimitBytes] =
      target.by === "sound floor" && sound !== undefined
        ? [sound.rateBps, sound.limitBytes]
        : [rateBps, this.#limitBytes];
    const isFloor = target.by === "floor" || target.by === "sound floor";
    const overflowText =
      isFloor && (target.aheadS * floorRateBps) / 8 > floorLimitBytes
        ? `; over its ${megabytes(floorLimitBytes)}, left to hls.js's cut`
        : "";
    return (
      `loads ahead ${target.aheadS.toFixed(1)} s (${megabytes((target.aheadS * rateBps) / 8)})` +
      `, by ${target.by}: ${levelText}; cap ${megabytes(this.#limitBytes * aheadShareOfBufferLimit)}` +
      soundText +
      overflowText
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
 * load, at each chunk of sound loaded, and at each buffer run full. Returns it, and the stopping.
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
  const onError = (_event: unknown, data: ErrorData): void => {
    if (data.details === Hls.ErrorDetails.BUFFER_FULL_ERROR) {
      buffer.bufferFull(data);
    }
  };
  hls.on(Hls.Events.LEVEL_SWITCHING, onSwitching);
  hls.on(Hls.Events.FRAG_LOADED, onLoaded);
  hls.on(Hls.Events.ERROR, onError);
  return {
    buffer,
    stop: () => {
      hls.off(Hls.Events.LEVEL_SWITCHING, onSwitching);
      hls.off(Hls.Events.FRAG_LOADED, onLoaded);
      hls.off(Hls.Events.ERROR, onError);
    },
  };
}
