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
 * Where a device keeps the latency of its sound through Web Audio, in whole ms, as last measured
 * there: the app's, in the device's own storage, so the player reads none itself. A later playback
 * on the device moves its sound by it from its first chunk, and its start throws nothing away.
 */
export interface SoundLatencyMemory {
  /** The latency last kept, or undefined where none is. */
  readonly recall: () => number | undefined;
  readonly keep: (ms: number) => void;
}

/** A memory that keeps nothing: each playback learns its latency afresh. */
export const noSoundLatencyMemory: SoundLatencyMemory = {
  recall: () => undefined,
  keep: () => undefined,
};

/**
 * The least change of Web Audio's measured latency, in ms, that moves the sound again: a smaller one
 * leaves the move in effect as it is. Each move of a chunked playback's sound throws its loaded sound
 * away, a wait of seconds on a Fire TV, while a latency read again moves by a few ms: 331 to 347 ms
 * there across casts, and again after its start (the owner's casts, staging, 2026-10-28). 40 ms is
 * the tightest of the broadcast tolerances for sound against its picture: EBU R37 asks that sound
 * lead its picture by no more than 40 ms (and trail by no more than 60 ms), and ITU-R BT.1359-1
 * finds a lead first detectable at about 45 ms (a lag at about 125 ms). A justified constant.
 */
export const soundLatencyChangeIgnoredMs = 40;

/**
 * The Web Audio latency, in ms, above which a device's levelling takes no Web Audio (D436): a
 * lowering is the element's own volume, and a raise is left to the film's ladder, whose sound the
 * server makes with the gain. The owner's TCL TV kept 336 to 426 ms and played its sound through Web
 * Audio 400 to 500 ms behind its picture; a Fire TV keeps 331 to 347 ms; a desktop tens of ms
 * (staging, 2026-11-06). 100 ms sits well above every desktop's and well below every Android TV's
 * seen, and ITU-R BT.1359-1 finds a lag first detectable at about 125 ms, which 100 ms of Web Audio
 * reaches with the element's own lag added. A justified constant, the owner's to confirm (D436).
 */
export const webAudioLateAboveMs = 100;

/**
 * Whether this device's sound through Web Audio plays later than webAudioLateAboveMs, as memory
 * keeps its latency; a device that has never measured one is not, and levels its first film through
 * Web Audio, as it learns the latency (D436).
 */
export function isWebAudioLate(memory: SoundLatencyMemory): boolean {
  const ms = memory.recall();
  return ms !== undefined && ms > webAudioLateAboveMs;
}

/**
 * Each element whose levelling lowers its sound by its own volume (D436): the factor the volume the
 * person set is multiplied by, 1 or less. An element absent plays at the volume set.
 */
const volumeFactorByElement = new WeakMap<HTMLMediaElement, number>();

/**
 * The volume the person set for element, 0 to 1: its own volume, less the lowering levelling puts in
 * it where Web Audio is late (D436). Whatever reads the volume the person set reads this, not the
 * element's, or it would take the lowering for the person's choice.
 */
export function heardVolumeOf(element: HTMLMediaElement): number {
  const factor = volumeFactorByElement.get(element) ?? 1;
  return Math.min(1, element.volume / factor);
}

/**
 * Sets the volume the person chose for element, kept within 0 and 1: its own volume, with the
 * lowering levelling puts in it where Web Audio is late (D436), so 1 is still the film at the level.
 */
export function setHeardVolume(element: HTMLMediaElement, volume: number): void {
  const factor = volumeFactorByElement.get(element) ?? 1;
  // eslint-disable-next-line no-param-reassign -- setting the element's volume is the function's job
  element.volume = Math.min(1, Math.max(0, volume)) * factor;
}

/**
 * Levels element by its own volume: by db where it lowers, and not at all where it raises, which the
 * element cannot. The volume the person set stays as it was (D436).
 */
function levelByVolume(element: HTMLMediaElement, db: number): void {
  const heard = heardVolumeOf(element);
  volumeFactorByElement.set(element, Math.min(1, dbToGainFactor(db)));
  setHeardVolume(element, heard);
}

/** The making of a browser's audio context, the default wherever one is made. */
export function newAudioContext(): GainAudio {
  return new AudioContext();
}

/** HTMLMediaElement.HAVE_FUTURE_DATA, which a plain page may not define. */
const haveFutureData = 3;

/**
 * One element's levelling: its audio context, the gain wanted, the gain node once the element's
 * sound is routed through it, and the latency of Web Audio its sound's move makes up for: the one
 * the device remembers until the context measures one, and after that the one measured, moved only
 * by a change of at least soundLatencyChangeIgnoredMs. A latency is read only as the element's sound
 * plays through the context: one read as the context has just started, before the film plays, is
 * not yet its own. A Fire TV's (Silk) read 83 to 259 ms there and 339 to 347 ms as the film played,
 * so each cast moved its sound from the latency remembered to the early one and back, throwing its
 * sound away each time (the owner's casts, staging, 2026-10-28, 07:47 to 07:51).
 */
class ElementLevelling {
  private wantedDb: number;
  private wantedOffsetMs = 0;
  private gainNode: GainNode | undefined = undefined;
  private source: MediaElementAudioSourceNode | undefined = undefined;
  private delayNode: DelayNode | undefined = undefined;
  /** Web Audio's latency the sound's move makes up for, in ms. */
  private latencyInEffectMs: number;
  /** What last changed latencyInEffectMs, until the move it makes is told; undefined after. */
  private latencyChangedBy: "remembered" | "measured" | "direct" | undefined;
  /** The latency the device remembered as this levelling began, for the log; undefined for none. */
  private readonly rememberedMs: number | undefined;

  private readonly context: GainAudio;
  private readonly memory: SoundLatencyMemory;

  constructor(
    private readonly element: HTMLMediaElement,
    audio: { readonly context: GainAudio; readonly memory: SoundLatencyMemory },
    db: number,
  ) {
    this.context = audio.context;
    this.memory = audio.memory;
    this.wantedDb = db;
    const rememberedMs = this.memory.recall();
    this.rememberedMs = rememberedMs;
    this.latencyInEffectMs = rememberedMs ?? 0;
    this.latencyChangedBy = rememberedMs === undefined ? undefined : "remembered";
    element.addEventListener("play", () => {
      this.resumeAndRoute();
    });
    // The context's latencies settle once its sound plays: the move is set again from them. A sound
    // still not through Web Audio as it plays has no latency to make up, remembered or not.
    element.addEventListener("playing", () => {
      this.route();
      if (this.source === undefined && this.latencyInEffectMs !== 0) {
        this.latencyInEffectMs = 0;
        this.latencyChangedBy = "direct";
      }
      this.applyOffset({ isLatencyRead: true });
    });
    this.applyOffset({ isLatencyRead: false });
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
   * How the sound is moved now: the delay Web Audio holds it back by and the latency counted toward
   * the wanted offset, in milliseconds (applyOffset).
   */
  applied(): { readonly delayMs: number; readonly latencyCountedMs: number } {
    const delaySeconds = this.delayNode?.delayTime.value ?? 0;
    return {
      delayMs: Math.round(delaySeconds * 1000),
      latencyCountedMs: Math.round(this.latencyInEffectMs),
    };
  }

  /**
   * Moves the sound by ms from its picture, later when positive and earlier when negative, counting
   * what Web Audio's own latency already delays it by (soundOffset).
   */
  setOffsetMs(ms: number): void {
    this.wantedOffsetMs = ms;
    this.applyOffset({ isLatencyRead: false });
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
    // Routed as the film already plays, as levelling turned on during it, its sound plays through
    // the context at once; routed before, the latency waits for the film's playing.
    this.applyOffset({ isLatencyRead: this.isElementPlaying() });
  }

  /** Whether the element plays now: not paused, with the data to go on. */
  private isElementPlaying(): boolean {
    return !this.element.paused && this.element.readyState >= haveFutureData;
  }

  /**
   * The latency the context measures, in ms, once the sound is routed through it; undefined before,
   * and while it reports an output latency of 0, which no output has: not yet measured.
   */
  private measuredLatencyMs(): number | undefined {
    const { baseLatency, outputLatency } = this.context;
    if (this.source === undefined || outputLatency === 0) {
      return undefined;
    }
    const ms = ((baseLatency ?? 0) + (outputLatency ?? 0)) * 1000;
    return Number.isFinite(ms) ? Math.round(ms) : undefined;
  }

  /**
   * Takes the latency measured into effect, where it is read (isLatencyRead) and differs from the
   * one in effect by at least soundLatencyChangeIgnoredMs, and keeps it in the device's memory; a
   * smaller change, or one not read, leaves the move as it is. Returns the latency the context
   * reports, read or not, for the log.
   */
  private takeMeasuredLatency(isLatencyRead: boolean): number | undefined {
    const measuredMs = this.measuredLatencyMs();
    if (
      isLatencyRead &&
      measuredMs !== undefined &&
      Math.abs(measuredMs - this.latencyInEffectMs) >= soundLatencyChangeIgnoredMs
    ) {
      this.latencyInEffectMs = measuredMs;
      this.latencyChangedBy = "measured";
      this.memory.keep(measuredMs);
    }
    return measuredMs;
  }

  /**
   * Moves the sound as wanted: what is left of the offset once Web Audio's own latency, which the
   * sound takes once routed, is counted, as remembered until it is measured, the latency read only
   * where isLatencyRead says the element's sound plays through the context. A sound still later
   * than wanted is moved earlier by its chunks (moveSoundEarlier); one still earlier is held back by
   * a delay node, put between the element and the gain the first time one is needed, so a sound
   * never delayed keeps its route.
   */
  private applyOffset(at: { readonly isLatencyRead: boolean }): void {
    const { source, gainNode } = this;
    const measuredMs = this.takeMeasuredLatency(at.isLatencyRead);
    const leftMs = this.wantedOffsetMs - this.latencyInEffectMs;
    moveSoundEarlier(this.element, Math.round(Math.max(0, -leftMs)), {
      why: this.latencyChangedBy ?? "offset",
      latencyMs: measuredMs,
      rememberedMs: this.rememberedMs,
    });
    this.latencyChangedBy = undefined;
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
 * too, from the start by the latency memory keeps for the device, until one is measured. Where the
 * page cannot set the element's volume, as on iOS, nothing is delayed (canSetVolume).
 */
export function soundOffset(
  element: HTMLMediaElement,
  createAudioContext: () => GainAudio = newAudioContext,
  memory: SoundLatencyMemory = noSoundLatencyMemory,
): SoundOffset {
  return {
    setMs: (wanted) => {
      const ms = Math.min(Math.max(wanted, -longestSoundOffsetMs), longestSoundOffsetMs);
      const levelling = levellingByElement.get(element);
      if (levelling !== undefined) {
        levelling.setOffsetMs(ms);
      } else if (ms > 0 && canSetVolume(element)) {
        const made = new ElementLevelling(element, { context: createAudioContext(), memory }, 0);
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
  /**
   * Through Web Audio, how long its delay holds the sound back and the latency the player counted
   * toward the device's sound offset, in milliseconds, each absent for none: the owner's TCL TV heard films' timing change
   * from one run to the next with Level loudness on, and the log said only the latencies
   * (2026-11-06).
   */
  readonly delayMs?: number;
  readonly latencyCountedMs?: number;
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
    ...movedOnly(levelling.applied()),
  };
}

/** A route's delay and counted latency, each only where it moves the sound. */
function movedOnly(applied: {
  readonly delayMs: number;
  readonly latencyCountedMs: number;
}): Pick<SoundRoute, "delayMs" | "latencyCountedMs"> {
  const { delayMs, latencyCountedMs } = applied;
  return {
    ...(delayMs === 0 ? {} : { delayMs }),
    ...(latencyCountedMs === 0 ? {} : { latencyCountedMs }),
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
 * channel is kept, so a receiver still gets surround. A sound through Web Audio is moved earlier by
 * its latency, from the start by the one memory keeps for the device, until one is measured. Where
 * the page cannot set the element's volume, as on iOS, no gain is applied (canSetVolume). Where the
 * latency memory keeps is above webAudioLateAboveMs and the element plays its film's own file
 * (isOwnFile), no Web Audio is taken: a lowering is the element's own volume (heardVolumeOf), and a
 * raise is not applied, the film's ladder's sound carrying it (D436). A chunked playback keeps Web
 * Audio on every device, its sound's chunks moved earlier by the latency (audio-shift.ts), which an
 * own file's sound cannot be. An element is levelled the way it first was for its life.
 */
export function gainAfterVolume(
  element: HTMLMediaElement,
  createAudioContext: () => GainAudio = newAudioContext,
  memory: SoundLatencyMemory = noSoundLatencyMemory,
): Gain {
  return gainOf(element, { createAudioContext, memory, isOwnFile: false });
}

/** The gain for element's sound where it plays its film's own file (gainAfterVolume, D436). */
export function ownFileGainAfterVolume(
  element: HTMLMediaElement,
  createAudioContext: () => GainAudio = newAudioContext,
  memory: SoundLatencyMemory = noSoundLatencyMemory,
): Gain {
  return gainOf(element, { createAudioContext, memory, isOwnFile: true });
}

/** The gain for element's sound, by gainAfterVolume's rules, isOwnFile saying what element plays. */
function gainOf(
  element: HTMLMediaElement,
  how: {
    readonly createAudioContext: () => GainAudio;
    readonly memory: SoundLatencyMemory;
    readonly isOwnFile: boolean;
  },
): Gain {
  const { createAudioContext, memory, isOwnFile } = how;
  return {
    setDb: (db) => {
      const levelling = levellingByElement.get(element);
      if (levelling !== undefined) {
        levelling.setDb(db);
      } else if (volumeFactorByElement.has(element)) {
        levelByVolume(element, db);
      } else if (db !== 0 && canSetVolume(element)) {
        if (isOwnFile && isWebAudioLate(memory)) {
          levelByVolume(element, db);
          return;
        }
        levellingByElement.set(
          element,
          new ElementLevelling(element, { context: createAudioContext(), memory }, db),
        );
      }
    },
  };
}
