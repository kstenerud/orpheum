// SPDX-License-Identifier: MPL-2.0

/**
 * Tones for the lab pages (plans/tv-device-tests.md), as WAV bytes a media element plays from a
 * blob URL: the extend probe sends one to each of two outputs by setSinkId, which only a media
 * element has; the button test plays one, once its switch is on, so the page counts as playing
 * media, which a desktop browser asks before it gives a page a keyboard's media keys; and the sound
 * timing test plays its beeps through a video element, as a film's sound plays. Made here, so the
 * probes load nothing.
 */

/** The bytes of a WAV file of samples, each from -1 to 1: 16-bit mono PCM at sampleRate. */
function pcmWav(samples: Float32Array, sampleRate: number): Uint8Array<ArrayBuffer> {
  const bytes = new Uint8Array(44 + samples.length * 2);
  const view = new DataView(bytes.buffer);
  const text = (at: number, value: string): void => {
    for (let index = 0; index < value.length; index += 1) {
      view.setUint8(at + index, value.codePointAt(index) ?? 0);
    }
  };
  text(0, "RIFF");
  view.setUint32(4, 36 + samples.length * 2, true);
  text(8, "WAVE");
  text(12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, 1, true); // mono
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  text(36, "data");
  view.setUint32(40, samples.length * 2, true);
  samples.forEach((value, index) => {
    view.setInt16(44 + index * 2, Math.round(value * 32_767), true);
  });
  return bytes;
}

/** The bytes of a WAV file of a sine tone: 16-bit mono PCM, looping cleanly at whole cycles. */
export function toneWav(
  frequencyHz: number,
  options: {
    readonly seconds?: number;
    readonly sampleRate?: number;
    readonly amplitude?: number;
  } = {},
): Uint8Array<ArrayBuffer> {
  const sampleRate = options.sampleRate ?? 22_050;
  const amplitude = options.amplitude ?? 0.3;
  // Whole cycles, so a looping element plays no click at the seam.
  const cycles = Math.max(1, Math.round((options.seconds ?? 1) * frequencyHz));
  const samples = new Float32Array(Math.round((cycles * sampleRate) / frequencyHz));
  for (let index = 0; index < samples.length; index += 1) {
    samples[index] = Math.sin((2 * Math.PI * frequencyHz * index) / sampleRate) * amplitude;
  }
  return pcmWav(samples, sampleRate);
}

/** A blob URL of a looping tone's WAV, for an <audio> element; revoke it when done. */
export function toneUrl(frequencyHz: number, amplitude?: number): string {
  return wavUrl(toneWav(frequencyHz, amplitude === undefined ? {} : { amplitude }));
}

/** A blob URL of a WAV's bytes, for a media element; revoke it when done. */
export function wavUrl(bytes: Uint8Array<ArrayBuffer>): string {
  return URL.createObjectURL(new Blob([bytes], { type: "audio/wav" }));
}

/** One beep of a run: its pitch, when it starts and how long it lasts, in milliseconds. */
export interface Beep {
  readonly frequencyHz: number;
  readonly atMs: number;
  readonly lastsMs: number;
}

/**
 * How long a beep fades in and out, in milliseconds: long enough that its edges make no click, short
 * beside the beep, so its start is heard where it is. A justified constant.
 */
export const beepFadeMs = 3;

/**
 * The bytes of a WAV file lastingMs long, silent but for beeps: 16-bit mono PCM, each beep a sine
 * faded in and out over beepFadeMs.
 */
export function beepsWav(
  beeps: readonly Beep[],
  options: {
    readonly lastingMs: number;
    readonly sampleRate?: number;
    readonly amplitude?: number;
  },
): Uint8Array<ArrayBuffer> {
  const sampleRate = options.sampleRate ?? 22_050;
  const amplitude = options.amplitude ?? 0.3;
  const perMs = sampleRate / 1000;
  const samples = new Float32Array(Math.round(options.lastingMs * perMs));
  const fade = beepFadeMs * perMs;
  for (const beep of beeps) {
    const from = Math.round(beep.atMs * perMs);
    const length = Math.round(beep.lastsMs * perMs);
    for (let index = 0; index < length && from + index < samples.length; index += 1) {
      const edge = Math.min(1, index / fade, (length - 1 - index) / fade);
      const wave = Math.sin((2 * Math.PI * beep.frequencyHz * index) / sampleRate);
      samples[from + index] = wave * amplitude * Math.max(0, edge);
    }
  }
  return pcmWav(samples, sampleRate);
}
