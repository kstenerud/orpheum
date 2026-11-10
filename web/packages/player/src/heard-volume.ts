// SPDX-License-Identifier: MPL-2.0

/**
 * What of a film's sound the app reads without playing one: whether a page may set an element's
 * volume, the volume the person set, whether this device's Web Audio is late, and how far a sound
 * may be moved. Apart from the levelling (gain.ts), which moves a chunked playback's sound by its
 * chunks and so loads hls.js, so a screen that reads them does not wait on hls.js (the web
 * client's architecture audit of 2026-11-10, Playback 2).
 */

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
export function dbToGainFactor(db: number): number {
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
 * reaches with the element's own lag added. The owner confirmed it, 2026-11-06 (D436).
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

/** Whether element's levelling is by its own volume, Web Audio being late here (D436). */
export function isLevelledByVolume(element: HTMLMediaElement): boolean {
  return volumeFactorByElement.has(element);
}

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
export function levelByVolume(element: HTMLMediaElement, db: number): void {
  const heard = heardVolumeOf(element);
  volumeFactorByElement.set(element, Math.min(1, dbToGainFactor(db)));
  setHeardVolume(element, heard);
}

/** The furthest a sound may be moved from its picture either way, in milliseconds. */
export const longestSoundOffsetMs = 2000;
