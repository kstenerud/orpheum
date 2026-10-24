// SPDX-License-Identifier: MPL-2.0

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
  "destination" | "state" | "resume" | "createMediaElementSource" | "createGain"
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
  private gainNode: GainNode | undefined = undefined;

  constructor(
    private readonly element: HTMLMediaElement,
    private readonly context: GainAudio,
    db: number,
  ) {
    this.wantedDb = db;
    element.addEventListener("play", () => {
      this.resumeAndRoute();
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
    this.context.createMediaElementSource(this.element).connect(gainNode);
    gainNode.connect(destination);
    this.gainNode = gainNode;
  }
}

/** Each element's levelling: Web Audio takes an element's sound once, for the element's life. */
const levellingByElement = new WeakMap<HTMLMediaElement, ElementLevelling>();

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
