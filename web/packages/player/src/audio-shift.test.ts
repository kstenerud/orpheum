// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import Hls, { LoadStats } from "hls.js";
import type {
  FragmentLoaderContext,
  HlsConfig,
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderResponse,
  LoaderStats,
} from "hls.js";
import { describe, expect, it } from "vitest";

import {
  canMoveSoundEarlier,
  moveSoundEarlier,
  shiftDecodeTime,
  shiftingFragmentLoader,
  SoundShift,
  timescaleOf,
} from "./audio-shift.ts";

/** A box: its size, its type and its payload. */
function box(type: string, ...payload: Uint8Array[]): Uint8Array {
  const size = 8 + payload.reduce((sum, part) => sum + part.length, 0);
  const out = new Uint8Array(size);
  new DataView(out.buffer).setUint32(0, size);
  for (let index = 0; index < 4; index += 1) {
    out[4 + index] = type.charCodeAt(index);
  }
  let at = 8;
  for (const part of payload) {
    out.set(part, at);
    at += part.length;
  }
  return out;
}

/** A full box's version and flags, then bytes. */
function full(version: number, bytes: number[]): Uint8Array {
  return new Uint8Array([version, 0, 0, 0, ...bytes]);
}

/** A big-endian number in n bytes. */
function be(value: number | bigint, n: number): number[] {
  const out: number[] = [];
  let left = BigInt(value);
  for (let index = 0; index < n; index += 1) {
    out.unshift(Number(left & 0xffn));
    left >>= 8n;
  }
  return out;
}

/** A fragment whose tfdt is decodeTime, as version 0 (32-bit) or 1 (64-bit). */
function fragment(decodeTime: number, version: 0 | 1): ArrayBuffer {
  const tfdt = box("tfdt", full(version, be(decodeTime, version === 1 ? 8 : 4)));
  return box(
    "moof",
    box("mfhd", full(0, be(1, 4))),
    box("traf", box("tfhd", full(0, be(1, 4))), tfdt),
  ).buffer as ArrayBuffer;
}

/** The tfdt of a fragment made by fragment(). */
function decodeTimeOf(data: ArrayBuffer, version: 0 | 1): number {
  const view = new DataView(data);
  // moof(8) mfhd(16) traf(8) tfhd(16) tfdt(8) version+flags(4)
  const at = 8 + 16 + 8 + 16 + 8 + 4;
  return version === 1 ? Number(view.getBigUint64(at)) : view.getUint32(at);
}

/** An init segment whose one track's timescale is timescale. */
function init(timescale: number): ArrayBuffer {
  const mdhd = box("mdhd", full(0, [...be(0, 4), ...be(0, 4), ...be(timescale, 4), ...be(0, 4)]));
  return box("moov", box("trak", box("mdia", mdhd))).buffer as ArrayBuffer;
}

describe("moving a chunked playback's sound earlier (the owner's TCL TV, 2026-10-24)", () => {
  it("reads a sound's timescale from its init segment", () => {
    expect(timescaleOf(init(48_000))).toBe(48_000);
    expect(timescaleOf(new ArrayBuffer(8))).toBeUndefined();
  });

  it("takes a fragment's decode time back, 32-bit or 64-bit, never below 0", () => {
    for (const version of [0, 1] as const) {
      const data = fragment(480_000, version);
      shiftDecodeTime(data, 24_000);
      expect(decodeTimeOf(data, version)).toBe(456_000);
      shiftDecodeTime(data, 999_999);
      expect(decodeTimeOf(data, version)).toBe(0);
    }
  });

  it("moves the sound's fragments by the shift as they load, and nothing else", () => {
    const answers = new Map<string, ArrayBuffer>();
    class Stand implements Loader<LoaderContext> {
      context: LoaderContext | null = null;
      stats: LoaderStats = new LoadStats();
      destroy(): void {
        this.context = null;
      }
      abort(): void {
        this.context = null;
      }
      load(
        context: LoaderContext,
        _config: LoaderConfiguration,
        callbacks: LoaderCallbacks<LoaderContext>,
      ): void {
        const data = answers.get(context.url)!;
        const response: LoaderResponse = { url: context.url, data };
        callbacks.onSuccess(response, this.stats, context, null);
      }
    }
    const Shifting = shiftingFragmentLoader(Stand, () => 500);
    const loaded = (url: string, type: string, sn: number | "initSegment"): ArrayBuffer => {
      let got: ArrayBuffer | undefined;
      new Shifting({} as HlsConfig).load(
        { url, frag: { type, sn } } as unknown as FragmentLoaderContext,
        {} as LoaderConfiguration,
        {
          onSuccess: (response) => {
            got = response.data as ArrayBuffer;
          },
          onError: () => undefined,
          onTimeout: () => undefined,
        },
      );
      return got!;
    };
    answers.set("sound/init", init(48_000));
    answers.set("sound/7", fragment(480_000, 0));
    answers.set("picture/7", fragment(160_000, 0));
    loaded("sound/init", "audio", "initSegment");
    expect(decodeTimeOf(loaded("sound/7", "audio", 7), 0)).toBe(456_000);
    expect(decodeTimeOf(loaded("picture/7", "main", 7), 0)).toBe(160_000);
  });

  it("starts each playback where the element's sound is wanted, and throws the sound's buffer away at a change", () => {
    const video = document.createElement("video");
    moveSoundEarlier(video, 300);
    const flushed: unknown[] = [];
    const hls = {
      trigger: (...told: unknown[]) => {
        flushed.push(told);
        return true;
      },
    };
    const shift = new SoundShift();
    const stop = shift.attach(hls, video);
    expect(shift.earlierMs).toBe(300);
    expect(canMoveSoundEarlier(video)).toBe(true);
    moveSoundEarlier(video, 300);
    expect(flushed).toEqual([]);
    moveSoundEarlier(video, 450);
    expect(shift.earlierMs).toBe(450);
    expect(flushed).toEqual([
      [
        Hls.Events.BUFFER_FLUSHING,
        { startOffset: 0, endOffset: Number.POSITIVE_INFINITY, type: "audio" },
      ],
    ]);
    stop();
    expect(canMoveSoundEarlier(video)).toBe(false);
  });
});
