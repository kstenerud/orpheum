// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { latestWrite } from "./latest-write.ts";

/** A send whose writes answer when the test says, and what it was asked to write. */
function heldSend(): {
  readonly send: (value: number) => Promise<void>;
  readonly sent: number[];
  readonly answer: (failure?: Error) => void;
} {
  const sent: number[] = [];
  const answers: ((failure?: Error) => void)[] = [];
  return {
    sent,
    send: async (value) => {
      sent.push(value);
      await new Promise<void>((kept, failed) => {
        answers.push((failure) => {
          if (failure === undefined) {
            kept();
          } else {
            failed(failure);
          }
        });
      });
    },
    answer: (failure) => {
      answers.shift()?.(failure);
    },
  };
}

describe("a write of the latest value asked for (the audit of 2026-10-25, W9)", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("writes only the last of a run of asks, once they stop for the wait", async () => {
    vi.useFakeTimers();
    const held = heldSend();
    const writes = latestWrite(held.send, 500, { kept: () => undefined, failed: () => undefined });
    writes.write(1);
    await vi.advanceTimersByTimeAsync(400);
    writes.write(2);
    await vi.advanceTimersByTimeAsync(400);
    writes.write(3);
    await vi.advanceTimersByTimeAsync(499);
    expect(held.sent).toEqual([]);
    await vi.advanceTimersByTimeAsync(1);
    expect(held.sent).toEqual([3]);
  });

  it("never sends two at once: one asked for during a write goes after it, the latest only", async () => {
    vi.useFakeTimers();
    const held = heldSend();
    const writes = latestWrite(held.send, 500, { kept: () => undefined, failed: () => undefined });
    writes.write(1);
    await vi.advanceTimersByTimeAsync(500);
    writes.write(2);
    writes.write(3);
    await vi.advanceTimersByTimeAsync(5000);
    expect(held.sent).toEqual([1]);
    held.answer();
    await vi.advanceTimersByTimeAsync(0);
    expect(held.sent).toEqual([1, 3]);
  });

  it("tells each write kept or failed, and sends what waits at once when flushed", async () => {
    vi.useFakeTimers();
    const held = heldSend();
    const told: string[] = [];
    const writes = latestWrite(held.send, 500, {
      kept: () => told.push("kept"),
      failed: (cause) => told.push(`failed: ${String(cause)}`),
    });
    writes.write(1);
    writes.flush();
    expect(held.sent).toEqual([1]);
    held.answer(new Error("refused"));
    await vi.advanceTimersByTimeAsync(0);
    writes.write(2);
    writes.flush();
    held.answer();
    await vi.advanceTimersByTimeAsync(0);
    expect(told).toEqual(["failed: Error: refused", "kept"]);
    writes.flush();
    expect(held.sent).toEqual([1, 2]);
  });
});
