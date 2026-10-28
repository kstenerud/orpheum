// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { megabitsPerSecond, timeLoad } from "./throughput.ts";

/** A clock that reads each of times in turn. */
function clockOf(...times: number[]): () => number {
  let next = 0;
  return () => times[Math.min(next++, times.length - 1)] ?? 0;
}

describe("timeLoad", () => {
  it("counts every byte of a body read to its end, its first byte's wait at the head, and its rate", async () => {
    const pieces = [new Uint8Array(100_000), new Uint8Array(150_000), new Uint8Array(250_000)];
    const body = new ReadableStream<Uint8Array>({
      start: (controller) => {
        for (const piece of pieces) {
          controller.enqueue(piece);
        }
        controller.close();
      },
    });
    const timing = await timeLoad(
      () => Promise.resolve(new Response(body, { status: 200 })),
      clockOf(1000, 1012, 1040),
    );
    expect(timing).toEqual({
      bytes: 500_000,
      firstByteMs: 12,
      totalMs: 40,
      megabitsPerSecond: 100,
    });
  });

  it("fails a refusal with its status, timing nothing", async () => {
    await expect(
      timeLoad(() => Promise.resolve(new Response("off", { status: 404 }))),
    ).rejects.toThrow("the core answered 404");
  });
});

describe("megabitsPerSecond", () => {
  it("is bits over time to a tenth, and nothing for no time", () => {
    expect(megabitsPerSecond(32 * 1024 * 1024, 8000)).toBe(33.6);
    expect(megabitsPerSecond(1000, 0)).toBe(0);
  });
});
