// SPDX-License-Identifier: MPL-2.0

/**
 * A tone as WAV bytes, which a media element plays from a blob URL: made here, so a test that
 * needs a sound to play loads nothing.
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
