// SPDX-License-Identifier: MPL-2.0

import { playerMovedSoundEvent, type SoundMoved } from "@orpheum/sdk";
import Hls, { PlaylistLevelType } from "hls.js";
import type {
  FragmentLoaderConstructor,
  FragmentLoaderContext,
  HlsConfig,
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderStats,
} from "hls.js";

/**
 * Moving a chunked playback's sound earlier than its picture, for a device that plays its sound
 * late: the owner's TCL TV, in TV Bro, half a second (2026-10-24). A sound can be held back by Web
 * Audio, but never played sooner, so the sound's own chunks are moved: each sound fragment's decode
 * time (its tfdt) is taken back by the shift before hls.js reads it, and hls.js, which places the
 * sound by the picture's initPTS, buffers it that much earlier. A film played as its own file has
 * no chunks of its own sound, and is not moved.
 */

/** hls.js's events, by their names. */
const HlsEvents = Hls.Events;

/** The longest a sound may be moved earlier, in milliseconds. */
export const longestSoundShiftMs = 2000;

/**
 * A chunked playback's way to move its sound earlier, by ms from 0: how far it is moved now, and a
 * move, which says whether it threw the sound loaded away to load it again at the new move.
 */
export interface AudioShift {
  readonly earlierMs: () => number;
  readonly setEarlierMs: (ms: number) => boolean;
}

/** Each element's chunked playback's shift, while one plays it. */
const shiftByElement = new WeakMap<HTMLMediaElement, AudioShift>();

/** How much earlier each element's sound is wanted, kept for the next playback made on it. */
const wantedByElement = new WeakMap<HTMLMediaElement, number>();

/**
 * What moved each element's sound to where it is wanted now, for the log of its next move: a move
 * made before a Watch Together player listened is told no one, and the next says where its from
 * came from.
 */
const whyWantedByElement = new WeakMap<HTMLMediaElement, SoundMoved["why"]>();

/**
 * Why a sound is moved, the latency Web Audio reports then, where the sound is through it, and the
 * latency the device remembered as its levelling began, where it remembered one.
 */
export interface SoundMoveCause {
  readonly why: SoundMoved["why"];
  readonly latencyMs?: number | undefined;
  readonly rememberedMs?: number | undefined;
}

/**
 * Moves element's sound earlier by ms, from 0 to longestSoundShiftMs, now and for any chunked
 * playback made on it after; whether a playback on it can (canMoveSoundEarlier). A move that
 * changes it is told the element (playerMovedSoundEvent) with its cause, for a Watch Together
 * player's log: from where to where, why, what moved it to where it moved from, the latency the
 * device remembered, and whether the playback threw its sound away.
 */
export function moveSoundEarlier(
  element: HTMLMediaElement,
  ms: number,
  cause: SoundMoveCause = { why: "offset" },
): void {
  const earlierMs = Math.min(Math.max(ms, 0), longestSoundShiftMs);
  const playback = shiftByElement.get(element);
  const fromMs = playback?.earlierMs() ?? wantedByElement.get(element) ?? 0;
  wantedByElement.set(element, earlierMs);
  const isFlushed = playback?.setEarlierMs(earlierMs) ?? false;
  if (earlierMs === fromMs) {
    return;
  }
  const fromWhy = whyWantedByElement.get(element);
  whyWantedByElement.set(element, cause.why);
  const moved = soundMoved({ fromMs, earlierMs, isFlushed, fromWhy }, cause);
  element.dispatchEvent(new CustomEvent(playerMovedSoundEvent, { detail: moved }));
}

/**
 * How much earlier element's sound is moved now, in milliseconds: its chunked playback's shift, or
 * the move wanted for the next one; 0 for none.
 */
export function soundEarlierMsOf(element: HTMLMediaElement): number {
  return shiftByElement.get(element)?.earlierMs() ?? wantedByElement.get(element) ?? 0;
}

/** A move as it is told: from and to, whether it threw the sound away, and its cause. */
function soundMoved(
  move: {
    readonly fromMs: number;
    readonly earlierMs: number;
    readonly isFlushed: boolean;
    readonly fromWhy: SoundMoved["why"] | undefined;
  },
  cause: SoundMoveCause,
): SoundMoved {
  const { fromWhy } = move;
  const { why, latencyMs, rememberedMs } = cause;
  return {
    earlierFromMs: move.fromMs,
    earlierMs: move.earlierMs,
    why,
    isFlushed: move.isFlushed,
    ...(latencyMs === undefined ? {} : { latencyMs }),
    ...(fromWhy === undefined ? {} : { fromWhy }),
    ...(rememberedMs === undefined ? {} : { rememberedMs }),
  };
}

/** Whether element plays a chunked playback whose sound can be moved earlier. */
export function canMoveSoundEarlier(element: HTMLMediaElement): boolean {
  return shiftByElement.has(element);
}

/** The box path to a fragment's decode time: moof, traf, tfdt. */
const tfdtPath = ["moof", "traf", "tfdt"] as const;

/** The box path to a track's timescale in an init segment: moov, trak, mdia, mdhd. */
const mdhdPath = ["moov", "trak", "mdia", "mdhd"] as const;

/** The offset of the payload of the first box at path within view, or undefined where there is none. */
function payloadAt(
  view: DataView,
  path: readonly string[],
  within: { readonly from: number; readonly to: number } = { from: 0, to: view.byteLength },
): number | undefined {
  const { to } = within;
  let offset = within.from;
  while (offset + 8 <= to) {
    let size = view.getUint32(offset);
    let header = 8;
    if (size === 1) {
      if (offset + 16 > to) {
        return undefined;
      }
      size = Number(view.getBigUint64(offset + 8));
      header = 16;
    } else if (size === 0) {
      size = to - offset;
    }
    if (size < header || offset + size > to) {
      return undefined;
    }
    const type = String.fromCharCode(
      view.getUint8(offset + 4),
      view.getUint8(offset + 5),
      view.getUint8(offset + 6),
      view.getUint8(offset + 7),
    );
    if (type === path[0]) {
      return path.length === 1
        ? offset + header
        : payloadAt(view, path.slice(1), { from: offset + header, to: offset + size });
    }
    offset += size;
  }
  return undefined;
}

/** The timescale of the first track an init segment describes, or undefined where it says none. */
export function timescaleOf(init: ArrayBuffer): number | undefined {
  const view = new DataView(init);
  const at = payloadAt(view, mdhdPath);
  if (at === undefined) {
    return undefined;
  }
  // A full box: version 1 has 64-bit times before the timescale, version 0 32-bit ones.
  const at2 = view.getUint8(at) === 1 ? at + 4 + 16 : at + 4 + 8;
  return at2 + 4 <= view.byteLength ? view.getUint32(at2) : undefined;
}

/**
 * Takes a fragment's decode time back by units of its timescale, never below 0, in place; a
 * fragment with no tfdt is left as it is.
 */
export function shiftDecodeTime(fragment: ArrayBuffer, units: number): void {
  const view = new DataView(fragment);
  const at = payloadAt(view, tfdtPath);
  if (at === undefined || units <= 0) {
    return;
  }
  if (view.getUint8(at) === 1) {
    const time = view.getBigUint64(at + 4);
    const by = BigInt(Math.round(units));
    view.setBigUint64(at + 4, time > by ? time - by : 0n);
  } else {
    const time = view.getUint32(at + 4);
    view.setUint32(at + 4, Math.max(0, time - Math.round(units)));
  }
}

/** The loader class hls.js makes its loaders from: its default, for any context. */
type LoaderClass = new (config: HlsConfig) => Loader<LoaderContext>;

/** What a sound fragment's loader keeps between its fragments: the sound's timescale, once known. */
interface SoundTimescale {
  units: number | undefined;
}

/**
 * Moves a fragment's data as it arrives, when it is a sound's: its init segment says the sound's
 * timescale, and each fragment after is taken back by earlierMs of it.
 */
function shiftOnArrival(
  context: FragmentLoaderContext,
  data: unknown,
  shift: { readonly timescale: SoundTimescale; readonly earlierMs: () => number },
): void {
  if (context.frag.type !== PlaylistLevelType.AUDIO || !(data instanceof ArrayBuffer)) {
    return;
  }
  if (context.frag.sn === "initSegment") {
    // eslint-disable-next-line no-param-reassign -- the timescale this loader keeps
    shift.timescale.units = timescaleOf(data) ?? shift.timescale.units;
  } else if (shift.timescale.units !== undefined) {
    shiftDecodeTime(data, (shift.earlierMs() / 1000) * shift.timescale.units);
  }
}

/**
 * hls.js's callbacks for one fragment, told of it as the fragment's own context: the wrapped
 * loader, made for any context, answers with the one it was given.
 */
function forFragment(
  context: FragmentLoaderContext,
  callbacks: LoaderCallbacks<FragmentLoaderContext>,
  onArrival: (data: unknown) => void,
): LoaderCallbacks<LoaderContext> {
  const { onAbort, onProgress } = callbacks;
  return {
    // eslint-disable-next-line max-params -- hls.js's LoaderOnSuccess
    onSuccess: (response, stats, _context, networkDetails) => {
      onArrival(response.data);
      callbacks.onSuccess(response, stats, context, networkDetails);
    },
    // eslint-disable-next-line max-params -- hls.js's LoaderOnError
    onError: (error, _context, networkDetails, stats) => {
      callbacks.onError(error, context, networkDetails, stats);
    },
    onTimeout: (stats, _context, networkDetails) => {
      callbacks.onTimeout(stats, context, networkDetails);
    },
    ...(onAbort === undefined
      ? {}
      : {
          onAbort: (stats, _context, networkDetails) => {
            onAbort(stats, context, networkDetails);
          },
        }),
    ...(onProgress === undefined
      ? {}
      : {
          // eslint-disable-next-line max-params -- hls.js's LoaderOnProgress
          onProgress: (stats, _context, data, networkDetails) => {
            onProgress(stats, context, data, networkDetails);
          },
        }),
  };
}

/**
 * A fragment loader that moves the sound's fragments earlier by earlierMs() as they arrive, reading
 * the sound's timescale from its init segment; every other fragment, and a sound whose timescale
 * is not yet known, arrives as it is. It wraps loaderClass, hls.js's own loader, and forwards the
 * rest.
 */
export function shiftingFragmentLoader(
  loaderClass: LoaderClass,
  earlierMs: () => number,
): FragmentLoaderConstructor {
  const shift = { timescale: { units: undefined }, earlierMs };
  return class ShiftingFragmentLoader implements Loader<FragmentLoaderContext> {
    context: FragmentLoaderContext | null = null;

    private readonly inner: Loader<LoaderContext>;

    constructor(config: HlsConfig) {
      this.inner = new loaderClass(config);
    }

    get stats(): LoaderStats {
      return this.inner.stats;
    }

    destroy(): void {
      this.inner.destroy();
    }

    abort(): void {
      this.inner.abort();
    }

    getCacheAge(): number | null {
      return this.inner.getCacheAge?.() ?? null;
    }

    getResponseHeader(name: string): string | null {
      return this.inner.getResponseHeader?.(name) ?? null;
    }

    load(
      context: FragmentLoaderContext,
      config: LoaderConfiguration,
      callbacks: LoaderCallbacks<FragmentLoaderContext>,
    ): void {
      this.context = context;
      this.inner.load(
        context,
        config,
        forFragment(context, callbacks, (data) => {
          shiftOnArrival(context, data, shift);
        }),
      );
    }
  };
}

/**
 * One chunked playback's shift of its sound: what its fragment loader reads (shiftingFragmentLoader),
 * and, once attached to its hls.js and element, what moveSoundEarlier sets. A change throws the
 * sound's buffer away, so the sound loads again at its new time.
 */
export class SoundShift {
  /** How much earlier the sound is moved, in ms. */
  earlierMs = 0;

  /** Lets element's sound be moved earlier while hls plays it, starting where it is wanted; returns the stop. */
  attach(hls: Pick<Hls, "trigger">, element: HTMLMediaElement): () => void {
    this.earlierMs = wantedByElement.get(element) ?? 0;
    const own: AudioShift = {
      earlierMs: () => this.earlierMs,
      setEarlierMs: (ms) => {
        if (ms === this.earlierMs) {
          return false;
        }
        this.earlierMs = ms;
        hls.trigger(HlsEvents.BUFFER_FLUSHING, {
          startOffset: 0,
          endOffset: Number.POSITIVE_INFINITY,
          type: "audio",
        });
        return true;
      },
    };
    shiftByElement.set(element, own);
    return () => {
      if (shiftByElement.get(element) === own) {
        shiftByElement.delete(element);
      }
    };
  }
}
