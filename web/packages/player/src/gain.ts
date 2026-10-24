// SPDX-License-Identifier: MPL-2.0

import { moveSoundEarlier } from "./audio-shift.ts";

/**
 * A gain after a media element's own volume, so a quiet film can be raised past what the element
 * allows, and the volume control still runs to 100, where 100 is the film at the level (D226 §4).
 */
export interface Gain {
  /** Sets the gain, in decibels: positive raises, negative lowers, 0 leaves the sound as it is. */
  readonly setDb: (db: number) => void;
}

/** The part of Web Audio a gain needs, so a test can stand in for the browser's. */
export type GainAudio = Pick<
  AudioContext,
  "destination" | "state" | "resume" | "createMediaElementSource" | "createGain" | "createDelay"
> &
  ReportedLatencies;

/** The latencies an audio context reports, in seconds, where the browser says them. */
interface ReportedLatencies {
  readonly baseLatency?: number;
  readonly outputLatency?: number;
}

/**
 * Whether a page may set this element's volume. iOS lets only its buttons set it, and there a
 * sound routed through Web Audio follows the ring/silent switch, so a phone on silent would play a
 * film mute; so where the volume cannot be set, no gain is applied, and only a prepared copy that
 * carries its gain is levelled (D226 §4).
 */
export function canSetVolume(element: Pick<HTMLMediaElement, "volume">): boolean {
  const before = element.volume;
  const probe = before === 1 ? 0.5 : 1;
  // eslint-disable-next-line no-param-reassign -- the only way to ask is to try
  element.volume = probe;
  const isSettable = element.volume === probe;
  // eslint-disable-next-line no-param-reassign -- and to put it back
  element.volume = before;
  return isSettable;
}

/** A gain in decibels as Web Audio's factor. */
function dbToGainFactor(db: number): number {
  return 10 ** (db / 20);
}

/**
 * One element's levelling: its audio context, the gain wanted, and the gain node once the element's
 * sound is routed through it.
 */
class ElementLevelling {
  private wantedDb: number;
  private wantedOffsetMs = 0;
  private gainNode: GainNode | undefined = undefined;
  private source: MediaElementAudioSourceNode | undefined = undefined;
  private delayNode: DelayNode | undefined = undefined;

  constructor(
    private readonly element: HTMLMediaElement,
    private readonly context: GainAudio,
    db: number,
  ) {
    this.wantedDb = db;
    element.addEventListener("play", () => {
      this.resumeAndRoute();
    });
    // The context's latencies settle once its sound plays: the delay is set again from them.
    element.addEventListener("playing", () => {
      this.applyOffset();
    });
    this.resumeAndRoute();
  }

  /** Whether the element's sound has been handed to Web Audio. */
  isRouted(): boolean {
    return this.gainNode !== undefined;
  }

  /** The latencies the audio context reports, in seconds, where the browser says them. */
  latencies(): ReportedLatencies {
    return this.context;
  }

  /**
   * Moves the sound by ms from its picture, later when positive and earlier when negative, counting
   * what Web Audio's own latency already delays it by (soundOffset).
   */
  setOffsetMs(ms: number): void {
    this.wantedOffsetMs = ms;
    this.applyOffset();
  }

  setDb(db: number): void {
    this.wantedDb = db;
    if (this.gainNode !== undefined) {
      this.gainNode.gain.value = dbToGainFactor(db);
    }
  }

  /**
   * Asks the context to run, and routes the sound once it does; when that is not allowed yet, the
   * next play asks again.
   */
  private resumeAndRoute(): void {
    this.context.resume().then(
      () => {
        this.route();
      },
      () => undefined,
    );
  }

  private route(): void {
    if (this.gainNode !== undefined || this.context.state !== "running") {
      return;
    }
    const { destination } = this.context;
    destination.channelCount = Math.max(destination.maxChannelCount, destination.channelCount);
    const gainNode = this.context.createGain();
    gainNode.gain.value = dbToGainFactor(this.wantedDb);
    const source = this.context.createMediaElementSource(this.element);
    source.connect(gainNode);
    gainNode.connect(destination);
    this.gainNode = gainNode;
    this.source = source;
    this.applyOffset();
  }

  /**
   * Moves the sound as wanted: what is left of the offset once Web Audio's own latency, which the
   * sound takes once routed, is counted. A sound still later than wanted is moved earlier by its
   * chunks (moveSoundEarlier); one still earlier is held back by a delay node, put between the
   * element and the gain the first time one is needed, so a sound never delayed keeps its route.
   */
  private applyOffset(): void {
    const { source, gainNode } = this;
    const latencyMs =
      source === undefined
        ? 0
        : ((this.context.baseLatency ?? 0) + (this.context.outputLatency ?? 0)) * 1000;
    const leftMs = this.wantedOffsetMs - latencyMs;
    moveSoundEarlier(this.element, Math.round(Math.max(0, -leftMs)));
    if (source === undefined || gainNode === undefined) {
      return;
    }
    if (this.delayNode === undefined) {
      if (leftMs <= 0) {
        return;
      }
      const delayNode = this.context.createDelay(longestSoundOffsetMs / 1000);
      source.disconnect();
      source.connect(delayNode);
      delayNode.connect(gainNode);
      this.delayNode = delayNode;
    }
    this.delayNode.delayTime.value = Math.max(0, leftMs / 1000);
  }
}

/** The furthest a sound may be moved from its picture either way, in milliseconds. */
export const longestSoundOffsetMs = 2000;

/** Each element's levelling: Web Audio takes an element's sound once, for the element's life. */
const levellingByElement = new WeakMap<HTMLMediaElement, ElementLevelling>();

/** A move of an element's sound from its picture. */
export interface SoundOffset {
  /**
   * Sets the move, in milliseconds: later when positive, earlier when negative, at most
   * longestSoundOffsetMs either way.
   */
  readonly setMs: (ms: number) => void;
}

/**
 * A move of element's sound from its picture, for a device that plays one of them late: the owner's
 * TCL TV, in TV Bro, plays its sound half a second late, and the browser cannot tell (2026-10-24).
 * Later is Web Audio's delay: the sound is handed to Web Audio, as a gain hands it, and its own
 * latency counts toward the delay. Earlier is the chunks' (moveSoundEarlier), where a chunked
 * playback plays the element: a sound already through Web Audio is moved earlier by its latency
 * too. Where the page cannot set the element's volume, as on iOS, nothing is delayed (canSetVolume).
 */
export function soundOffset(
  element: HTMLMediaElement,
  createAudioContext: () => GainAudio = () => new AudioContext(),
): SoundOffset {
  return {
    setMs: (wanted) => {
      const ms = Math.min(Math.max(wanted, -longestSoundOffsetMs), longestSoundOffsetMs);
      const levelling = levellingByElement.get(element);
      if (levelling !== undefined) {
        levelling.setOffsetMs(ms);
      } else if (ms > 0 && canSetVolume(element)) {
        const made = new ElementLevelling(element, createAudioContext(), 0);
        made.setOffsetMs(ms);
        levellingByElement.set(element, made);
      } else {
        moveSoundEarlier(element, Math.max(0, -ms));
      }
    },
  };
}

/**
 * How an element's sound reaches the speakers: through Web Audio, for its gain, with the latencies
 * its audio context reports in milliseconds where the browser says them; or straight from the
 * element. A sound through Web Audio may play later than its picture where the context's output is
 * slow, as Android's can be (the owner's TCL TV, 2026-10-24).
 */
export interface SoundRoute {
  readonly isThroughWebAudio: boolean;
  readonly baseLatencyMs?: number;
  readonly outputLatencyMs?: number;
}

/** How element's sound reaches the speakers now (SoundRoute). */
export function soundRouteOf(element: HTMLMediaElement): SoundRoute {
  const levelling = levellingByElement.get(element);
  if (levelling?.isRouted() !== true) {
    return { isThroughWebAudio: false };
  }
  const { baseLatency, outputLatency } = levelling.latencies();
  const base = inMs(baseLatency);
  const output = inMs(outputLatency);
  return {
    isThroughWebAudio: true,
    ...(base === undefined ? {} : { baseLatencyMs: base }),
    ...(output === undefined ? {} : { outputLatencyMs: output }),
  };
}

/** Seconds as whole milliseconds; undefined for none, or for what is not a number. */
function inMs(seconds: number | undefined): number | undefined {
  return seconds === undefined || !Number.isFinite(seconds)
    ? undefined
    : Math.round(seconds * 1000);
}

/**
 * The gain for element's sound. The sound is handed to Web Audio only once a gain other than 0 dB
 * is asked for and the audio context is running: a browser holds a context until the page is
 * played with, and one would hold the video with it, so until then the film plays as it is. Every
 * channel is kept, so a receiver still gets surround. Where the page cannot set the element's
 * volume, as on iOS, no gain is applied (canSetVolume).
 */
export function gainAfterVolume(
  element: HTMLMediaElement,
  createAudioContext: () => GainAudio = () => new AudioContext(),
): Gain {
  return {
    setDb: (db) => {
      const levelling = levellingByElement.get(element);
      if (levelling !== undefined) {
        levelling.setDb(db);
      } else if (db !== 0 && canSetVolume(element)) {
        levellingByElement.set(element, new ElementLevelling(element, createAudioContext(), db));
      }
    },
  };
}
