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
>;

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
function factor(db: number): number {
  return 10 ** (db / 20);
}

/**
 * One element's levelling: its audio context, the gain wanted, and the stage once the element's
 * sound is routed through it.
 */
class Levelled {
  private wantedDb: number;
  private stage: GainNode | undefined = undefined;

  constructor(
    private readonly element: HTMLMediaElement,
    private readonly context: GainAudio,
    db: number,
  ) {
    this.wantedDb = db;
    element.addEventListener("play", () => {
      this.start();
    });
    this.start();
  }

  setDb(db: number): void {
    this.wantedDb = db;
    if (this.stage !== undefined) {
      this.stage.gain.value = factor(db);
    }
  }

  /**
   * Asks the context to run, and routes the sound once it does; when that is not allowed yet, the
   * next play asks again.
   */
  private start(): void {
    this.context.resume().then(
      () => {
        this.route();
      },
      () => undefined,
    );
  }

  private route(): void {
    if (this.stage !== undefined || this.context.state !== "running") {
      return;
    }
    const { destination } = this.context;
    destination.channelCount = Math.max(destination.maxChannelCount, destination.channelCount);
    const stage = this.context.createGain();
    stage.gain.value = factor(this.wantedDb);
    this.context.createMediaElementSource(this.element).connect(stage);
    stage.connect(destination);
    this.stage = stage;
  }
}

/** Each element's levelling: Web Audio takes an element's sound once, for the element's life. */
const levelled = new WeakMap<HTMLMediaElement, Levelled>();

/**
 * The gain for element's sound. The sound is handed to Web Audio only once a gain other than 0 dB
 * is asked for and the audio context is running: a browser holds a context until the page is
 * played with, and one would hold the video with it, so until then the film plays as it is. Every
 * channel is kept, so a receiver still gets surround.
 */
export function gainAfterVolume(
  element: HTMLMediaElement,
  audio: () => GainAudio = () => new AudioContext(),
): Gain {
  return {
    setDb: (db) => {
      const level = levelled.get(element);
      if (level !== undefined) {
        level.setDb(db);
      } else if (db !== 0) {
        levelled.set(element, new Levelled(element, audio(), db));
      }
    },
  };
}
